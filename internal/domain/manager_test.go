package domain

import (
	"log/slog"
	"os"
	"testing"

	"github.com/foxzi/sendry/internal/config"
)

func TestNewManagerWithoutDKIM(t *testing.T) {
	cfg := &config.Config{
		DKIM: config.DKIMConfig{
			Enabled: false,
		},
		Domains: map[string]config.DomainConfig{
			"example.com": {
				Mode: "production",
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m, err := NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if m.HasDKIM() {
		t.Error("expected HasDKIM() = false when no DKIM configured")
	}
}

func TestGetDomainMode(t *testing.T) {
	cfg := &config.Config{
		Domains: map[string]config.DomainConfig{
			"production.com": {
				Mode: "production",
			},
			"sandbox.com": {
				Mode: "sandbox",
			},
			"redirect.com": {
				Mode: "redirect",
			},
			"bcc.com": {
				Mode: "bcc",
			},
			"nomode.com": {},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m, err := NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	tests := []struct {
		domain   string
		expected string
	}{
		{"production.com", "production"},
		{"sandbox.com", "sandbox"},
		{"redirect.com", "redirect"},
		{"bcc.com", "bcc"},
		{"nomode.com", "production"},      // default
		{"unknown.com", "production"},     // not configured, default
	}

	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			mode := m.GetDomainMode(tc.domain)
			if mode != tc.expected {
				t.Errorf("GetDomainMode(%s) = %s, want %s", tc.domain, mode, tc.expected)
			}
		})
	}
}

func TestGetRedirectAddresses(t *testing.T) {
	cfg := &config.Config{
		Domains: map[string]config.DomainConfig{
			"redirect.com": {
				Mode:       "redirect",
				RedirectTo: []string{"admin@example.com", "test@example.com"},
			},
			"noreirect.com": {
				Mode: "production",
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m, err := NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test domain with redirect
	addrs := m.GetRedirectAddresses("redirect.com")
	if len(addrs) != 2 {
		t.Errorf("expected 2 redirect addresses, got %d", len(addrs))
	}
	if addrs[0] != "admin@example.com" {
		t.Errorf("expected admin@example.com, got %s", addrs[0])
	}

	// Test domain without redirect
	addrs = m.GetRedirectAddresses("noreirect.com")
	if len(addrs) != 0 {
		t.Errorf("expected no redirect addresses, got %d", len(addrs))
	}

	// Test unknown domain
	addrs = m.GetRedirectAddresses("unknown.com")
	if addrs != nil {
		t.Error("expected nil for unknown domain")
	}
}

func TestGetBCCAddresses(t *testing.T) {
	cfg := &config.Config{
		Domains: map[string]config.DomainConfig{
			"bcc.com": {
				Mode:  "bcc",
				BCCTo: []string{"archive@example.com"},
			},
			"nobcc.com": {
				Mode: "production",
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m, err := NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test domain with BCC
	addrs := m.GetBCCAddresses("bcc.com")
	if len(addrs) != 1 {
		t.Errorf("expected 1 BCC address, got %d", len(addrs))
	}
	if addrs[0] != "archive@example.com" {
		t.Errorf("expected archive@example.com, got %s", addrs[0])
	}

	// Test domain without BCC
	addrs = m.GetBCCAddresses("nobcc.com")
	if len(addrs) != 0 {
		t.Errorf("expected no BCC addresses, got %d", len(addrs))
	}
}

func TestListDomains(t *testing.T) {
	cfg := &config.Config{
		Domains: map[string]config.DomainConfig{
			"a.com": {},
			"b.com": {},
			"c.com": {},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m, err := NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	domains := m.ListDomains()
	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}

	// Check all domains are present
	domainMap := make(map[string]bool)
	for _, d := range domains {
		domainMap[d] = true
	}
	for _, expected := range []string{"a.com", "b.com", "c.com"} {
		if !domainMap[expected] {
			t.Errorf("missing domain %s", expected)
		}
	}
}

func TestGetDomainConfig(t *testing.T) {
	cfg := &config.Config{
		Domains: map[string]config.DomainConfig{
			"example.com": {
				Mode: "production",
				RateLimit: &config.DomainRateLimitConfig{
					MessagesPerHour: 1000,
				},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m, err := NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test existing domain
	dc := m.GetDomainConfig("example.com")
	if dc == nil {
		t.Fatal("expected domain config, got nil")
	}
	if dc.Mode != "production" {
		t.Errorf("expected mode production, got %s", dc.Mode)
	}
	if dc.RateLimit.MessagesPerHour != 1000 {
		t.Errorf("expected 1000 messages per hour, got %d", dc.RateLimit.MessagesPerHour)
	}

	// Test unknown domain
	dc = m.GetDomainConfig("unknown.com")
	if dc != nil {
		t.Error("expected nil for unknown domain")
	}
}

func TestGetSignerWithoutDKIM(t *testing.T) {
	cfg := &config.Config{
		DKIM: config.DKIMConfig{
			Enabled: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m, err := NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Should return nil when no DKIM configured
	signer := m.GetSigner("example.com")
	if signer != nil {
		t.Error("expected nil signer when DKIM not configured")
	}

	signer = m.GetSignerForEmail("user@example.com")
	if signer != nil {
		t.Error("expected nil signer for email when DKIM not configured")
	}
}
