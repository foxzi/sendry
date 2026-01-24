package bounce

import (
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/queue"
)

func TestNewGenerator(t *testing.T) {
	g := NewGenerator("mail.example.com")

	if g.hostname != "mail.example.com" {
		t.Errorf("expected hostname mail.example.com, got %s", g.hostname)
	}
	if g.postmaster != "postmaster@mail.example.com" {
		t.Errorf("expected postmaster postmaster@mail.example.com, got %s", g.postmaster)
	}
	if g.reportingMTA != "mail.example.com" {
		t.Errorf("expected reportingMTA mail.example.com, got %s", g.reportingMTA)
	}
}

func TestSetPostmaster(t *testing.T) {
	g := NewGenerator("mail.example.com")
	g.SetPostmaster("admin@example.com")

	if g.postmaster != "admin@example.com" {
		t.Errorf("expected postmaster admin@example.com, got %s", g.postmaster)
	}
}

func TestGenerateDSN(t *testing.T) {
	g := NewGenerator("mail.example.com")

	msg := &queue.Message{
		ID:   "test-msg-123",
		From: "sender@example.com",
		To:   []string{"recipient1@test.com", "recipient2@test.com"},
		Data: []byte("Subject: Test Message\r\n\r\nTest body"),
	}

	tests := []struct {
		name      string
		errorMsg  string
		permanent bool
		wantAction string
		wantStatus string
	}{
		{
			name:       "permanent failure",
			errorMsg:   "550 User not found",
			permanent:  true,
			wantAction: "failed",
			wantStatus: "5.0.0",
		},
		{
			name:       "temporary failure",
			errorMsg:   "421 Try again later",
			permanent:  false,
			wantAction: "delayed",
			wantStatus: "4.0.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dsn, err := g.GenerateDSN(msg, tc.errorMsg, tc.permanent)
			if err != nil {
				t.Fatalf("GenerateDSN failed: %v", err)
			}

			dsnStr := string(dsn)

			// Check required headers
			if !strings.Contains(dsnStr, "From: Mail Delivery System <postmaster@mail.example.com>") {
				t.Error("missing or incorrect From header")
			}
			if !strings.Contains(dsnStr, "To: <sender@example.com>") {
				t.Error("missing or incorrect To header")
			}
			if !strings.Contains(dsnStr, "Subject: Delivery Status Notification (Failure)") {
				t.Error("missing Subject header")
			}
			if !strings.Contains(dsnStr, "Content-Type: multipart/report") {
				t.Error("missing multipart/report Content-Type")
			}
			if !strings.Contains(dsnStr, "Auto-Submitted: auto-replied") {
				t.Error("missing Auto-Submitted header")
			}

			// Check DSN content
			if !strings.Contains(dsnStr, "message/delivery-status") {
				t.Error("missing delivery-status part")
			}
			if !strings.Contains(dsnStr, "Reporting-MTA: dns; mail.example.com") {
				t.Error("missing Reporting-MTA")
			}
			if !strings.Contains(dsnStr, "Action: "+tc.wantAction) {
				t.Errorf("expected Action: %s", tc.wantAction)
			}
			if !strings.Contains(dsnStr, "Status: "+tc.wantStatus) {
				t.Errorf("expected Status: %s", tc.wantStatus)
			}

			// Check recipients
			if !strings.Contains(dsnStr, "Final-Recipient: rfc822; recipient1@test.com") {
				t.Error("missing recipient1 in DSN")
			}
			if !strings.Contains(dsnStr, "Final-Recipient: rfc822; recipient2@test.com") {
				t.Error("missing recipient2 in DSN")
			}

			// Check error message
			if !strings.Contains(dsnStr, tc.errorMsg) {
				t.Errorf("DSN should contain error message: %s", tc.errorMsg)
			}

			// Check original subject
			if !strings.Contains(dsnStr, "Subject: Test Message") {
				t.Error("missing original subject")
			}
		})
	}
}

func TestGenerateSimpleBounce(t *testing.T) {
	g := NewGenerator("mail.example.com")

	msg := &queue.Message{
		ID:   "test-msg-456",
		From: "sender@example.com",
		To:   []string{"recipient@test.com"},
		Data: []byte("Subject: Hello World\r\n\r\nBody"),
	}

	bounce, err := g.GenerateSimpleBounce(msg, "Connection refused")
	if err != nil {
		t.Fatalf("GenerateSimpleBounce failed: %v", err)
	}

	bounceStr := string(bounce)

	// Check headers
	if !strings.Contains(bounceStr, "From: Mail Delivery System <postmaster@mail.example.com>") {
		t.Error("missing From header")
	}
	if !strings.Contains(bounceStr, "To: <sender@example.com>") {
		t.Error("missing To header")
	}
	if !strings.Contains(bounceStr, "Subject: Undelivered Mail Returned to Sender") {
		t.Error("missing Subject header")
	}
	if !strings.Contains(bounceStr, "Content-Type: text/plain") {
		t.Error("missing Content-Type header")
	}
	if !strings.Contains(bounceStr, "Auto-Submitted: auto-replied") {
		t.Error("missing Auto-Submitted header")
	}

	// Check content
	if !strings.Contains(bounceStr, "To: recipient@test.com") {
		t.Error("missing recipient in body")
	}
	if !strings.Contains(bounceStr, "Subject: Hello World") {
		t.Error("missing original subject")
	}
	if !strings.Contains(bounceStr, "Connection refused") {
		t.Error("missing error message")
	}
}

func TestExtractSubject(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "normal subject",
			data:     []byte("From: test@example.com\r\nSubject: Test Subject\r\n\r\nBody"),
			expected: "Test Subject",
		},
		{
			name:     "no subject",
			data:     []byte("From: test@example.com\r\n\r\nBody"),
			expected: "(no subject)",
		},
		{
			name:     "empty subject",
			data:     []byte("From: test@example.com\r\nSubject: \r\n\r\nBody"),
			expected: "(no subject)",
		},
		{
			name:     "invalid email",
			data:     []byte("not a valid email"),
			expected: "(unknown)",
		},
		{
			name:     "empty data",
			data:     []byte{},
			expected: "(unknown)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractSubject(tc.data)
			if result != tc.expected {
				t.Errorf("extractSubject() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestGenerateDSNWithCustomPostmaster(t *testing.T) {
	g := NewGenerator("mail.example.com")
	g.SetPostmaster("noreply@example.com")

	msg := &queue.Message{
		ID:   "test-123",
		From: "sender@example.com",
		To:   []string{"rcpt@test.com"},
		Data: []byte("Subject: Test\r\n\r\nBody"),
	}

	dsn, err := g.GenerateDSN(msg, "Error", true)
	if err != nil {
		t.Fatalf("GenerateDSN failed: %v", err)
	}

	if !strings.Contains(string(dsn), "From: Mail Delivery System <noreply@example.com>") {
		t.Error("custom postmaster not used in From header")
	}
	if !strings.Contains(string(dsn), "contact <noreply@example.com>") {
		t.Error("custom postmaster not used in body")
	}
}
