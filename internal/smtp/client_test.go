package smtp

import (
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/dkim"
	"github.com/foxzi/sendry/internal/dns"
)

func TestNewClient(t *testing.T) {
	resolver := dns.NewResolver(0)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test with default timeout
	client := NewClient(resolver, "mail.example.com", 0, logger)
	if client.timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", client.timeout)
	}
	if client.hostname != "mail.example.com" {
		t.Errorf("expected hostname mail.example.com, got %s", client.hostname)
	}

	// Test with custom timeout
	client = NewClient(resolver, "mail.example.com", 60*time.Second, logger)
	if client.timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", client.timeout)
	}
}

func TestDeliveryError(t *testing.T) {
	// Test temporary error
	tempErr := &DeliveryError{
		Temporary: true,
		Message:   "Connection refused",
	}
	if tempErr.Error() != "Connection refused" {
		t.Errorf("expected 'Connection refused', got %s", tempErr.Error())
	}

	// Test permanent error
	permErr := &DeliveryError{
		Temporary: false,
		Message:   "User not found",
	}
	if permErr.Error() != "User not found" {
		t.Errorf("expected 'User not found', got %s", permErr.Error())
	}
}

func TestIsTemporaryError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "temporary delivery error",
			err:      &DeliveryError{Temporary: true, Message: "temp"},
			expected: true,
		},
		{
			name:     "permanent delivery error",
			err:      &DeliveryError{Temporary: false, Message: "perm"},
			expected: false,
		},
		{
			name:     "unknown error",
			err:      errors.New("unknown error"),
			expected: true, // Assume temporary for unknown
		},
		{
			name:     "wrapped temporary error",
			err:      errors.New("wrap: " + (&DeliveryError{Temporary: true}).Error()),
			expected: true, // Can't unwrap, assumes temporary
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsTemporaryError(tc.err)
			if result != tc.expected {
				t.Errorf("IsTemporaryError() = %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestCategorizeError(t *testing.T) {
	resolver := dns.NewResolver(0)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(resolver, "mail.example.com", 30*time.Second, logger)

	tests := []struct {
		name          string
		err           error
		stage         string
		wantTemporary bool
	}{
		{
			name:          "550 user not found",
			err:           errors.New("550 5.1.1 User not found"),
			stage:         "RCPT TO",
			wantTemporary: false,
		},
		{
			name:          "551 user not local",
			err:           errors.New("551 User not local"),
			stage:         "RCPT TO",
			wantTemporary: false,
		},
		{
			name:          "552 mailbox full",
			err:           errors.New("552 Mailbox full"),
			stage:         "DATA",
			wantTemporary: false,
		},
		{
			name:          "553 invalid address",
			err:           errors.New("553 Invalid mailbox"),
			stage:         "RCPT TO",
			wantTemporary: false,
		},
		{
			name:          "554 transaction failed",
			err:           errors.New("554 Transaction failed"),
			stage:         "DATA",
			wantTemporary: false,
		},
		{
			name:          "421 try again later",
			err:           errors.New("421 Service not available"),
			stage:         "HELO",
			wantTemporary: true,
		},
		{
			name:          "450 mailbox unavailable",
			err:           errors.New("450 Mailbox temporarily unavailable"),
			stage:         "RCPT TO",
			wantTemporary: true,
		},
		{
			name:          "connection timeout",
			err:           errors.New("i/o timeout"),
			stage:         "connection",
			wantTemporary: true,
		},
		{
			name:          "generic error",
			err:           errors.New("something went wrong"),
			stage:         "unknown",
			wantTemporary: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := client.categorizeError(tc.err, tc.stage)
			if result.Temporary != tc.wantTemporary {
				t.Errorf("categorizeError() temporary = %v, want %v", result.Temporary, tc.wantTemporary)
			}
			if result.Message == "" {
				t.Error("expected non-empty message")
			}
		})
	}
}

// Mock DKIM provider for testing
type mockDKIMProvider struct {
	signers map[string]*dkim.Signer
}

func (m *mockDKIMProvider) GetSignerForEmail(email string) *dkim.Signer {
	domain := extractDomainFromEmail(email)
	return m.signers[domain]
}

func TestSetDKIMProvider(t *testing.T) {
	resolver := dns.NewResolver(0)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(resolver, "mail.example.com", 30*time.Second, logger)

	// Initially no provider
	if client.dkimProvider != nil {
		t.Error("expected nil DKIM provider initially")
	}

	// Set provider
	provider := &mockDKIMProvider{
		signers: make(map[string]*dkim.Signer),
	}
	client.SetDKIMProvider(provider)

	if client.dkimProvider == nil {
		t.Error("expected DKIM provider to be set")
	}
}

func TestGetDKIMSignerPriority(t *testing.T) {
	resolver := dns.NewResolver(0)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(resolver, "mail.example.com", 30*time.Second, logger)

	// No provider or signer - should return nil
	signer := client.getDKIMSigner("user@example.com")
	if signer != nil {
		t.Error("expected nil signer when nothing configured")
	}

	// Set provider that returns nil
	provider := &mockDKIMProvider{
		signers: make(map[string]*dkim.Signer),
	}
	client.SetDKIMProvider(provider)

	signer = client.getDKIMSigner("user@example.com")
	if signer != nil {
		t.Error("expected nil signer when provider has no signer for domain")
	}
}

func TestExtractDomainFromEmail(t *testing.T) {
	tests := []struct {
		email    string
		expected string
	}{
		{"user@example.com", "example.com"},
		{"user@EXAMPLE.COM", "example.com"},
		{"user@Sub.Domain.Com", "sub.domain.com"},
		{"user@", ""},
		{"nodomain", ""},
		{"@domain.com", "domain.com"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.email, func(t *testing.T) {
			result := extractDomainFromEmail(tc.email)
			if result != tc.expected {
				t.Errorf("extractDomainFromEmail(%q) = %q, want %q", tc.email, result, tc.expected)
			}
		})
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		addr     string
		expected string
	}{
		{"192.168.1.1:25", "192.168.1.1"},
		{"10.0.0.1:587", "10.0.0.1"},
		{"[::1]:25", "::1"},
		{"192.168.1.1", "192.168.1.1"}, // No port
		{"invalid", "invalid"},
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			result := extractIP(tc.addr)
			if result != tc.expected {
				t.Errorf("extractIP(%q) = %q, want %q", tc.addr, result, tc.expected)
			}
		})
	}
}
