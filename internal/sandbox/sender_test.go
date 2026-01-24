package sandbox

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/foxzi/sendry/internal/queue"
)

// mockSender is a mock sender for testing
type mockSender struct {
	sentMessages []*queue.Message
	shouldError  bool
	errorMsg     string
}

func (m *mockSender) Send(ctx context.Context, msg *queue.Message) error {
	if m.shouldError {
		return &SimulatedError{Message: m.errorMsg, Temporary: true}
	}
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

// mockDomainProvider is a mock domain provider for testing
type mockDomainProvider struct {
	modes         map[string]string
	redirectAddrs map[string][]string
	bccAddrs      map[string][]string
}

func (m *mockDomainProvider) GetDomainMode(domain string) string {
	if mode, ok := m.modes[domain]; ok {
		return mode
	}
	return "production"
}

func (m *mockDomainProvider) GetRedirectAddresses(domain string) []string {
	if addrs, ok := m.redirectAddrs[domain]; ok {
		return addrs
	}
	return nil
}

func (m *mockDomainProvider) GetBCCAddresses(domain string) []string {
	if addrs, ok := m.bccAddrs[domain]; ok {
		return addrs
	}
	return nil
}

func TestSenderProductionMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	mock := &mockSender{}
	provider := &mockDomainProvider{
		modes: map[string]string{
			"production.com": "production",
		},
	}

	sender := NewSender(mock, provider, storage, nil)

	msg := &queue.Message{
		ID:        "test-1",
		From:      "sender@production.com",
		To:        []string{"recipient@example.com"},
		Data:      []byte("test message"),
		CreatedAt: time.Now(),
	}

	err = sender.Send(context.Background(), msg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Message should be sent to real sender
	if len(mock.sentMessages) != 1 {
		t.Errorf("expected 1 sent message, got %d", len(mock.sentMessages))
	}
}

func TestSenderSandboxMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	mock := &mockSender{}
	provider := &mockDomainProvider{
		modes: map[string]string{
			"sandbox.com": "sandbox",
		},
	}

	sender := NewSender(mock, provider, storage, nil)

	msg := &queue.Message{
		ID:        "test-sandbox",
		From:      "sender@sandbox.com",
		To:        []string{"recipient@example.com"},
		Data:      []byte("Subject: Test\r\n\r\ntest message"),
		CreatedAt: time.Now(),
	}

	err = sender.Send(context.Background(), msg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Message should NOT be sent to real sender
	if len(mock.sentMessages) != 0 {
		t.Errorf("expected 0 sent messages in sandbox mode, got %d", len(mock.sentMessages))
	}

	// Message should be in storage
	stored, err := storage.Get(context.Background(), "test-sandbox")
	if err != nil {
		t.Fatalf("failed to get stored message: %v", err)
	}
	if stored == nil {
		t.Error("expected message to be stored in sandbox")
	}
	if stored.Mode != "sandbox" {
		t.Errorf("expected mode sandbox, got %s", stored.Mode)
	}
}

func TestSenderRedirectMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	mock := &mockSender{}
	provider := &mockDomainProvider{
		modes: map[string]string{
			"redirect.com": "redirect",
		},
		redirectAddrs: map[string][]string{
			"redirect.com": {"test@testing.com"},
		},
	}

	sender := NewSender(mock, provider, storage, nil)

	msg := &queue.Message{
		ID:        "test-redirect",
		From:      "sender@redirect.com",
		To:        []string{"original@example.com"},
		Data:      []byte("Subject: Test\r\n\r\ntest message"),
		CreatedAt: time.Now(),
	}

	err = sender.Send(context.Background(), msg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Message should be sent to redirect address
	if len(mock.sentMessages) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(mock.sentMessages))
	}

	sent := mock.sentMessages[0]
	if len(sent.To) != 1 || sent.To[0] != "test@testing.com" {
		t.Errorf("expected redirect to test@testing.com, got %v", sent.To)
	}

	// Message should also be in storage with original_to
	stored, err := storage.Get(context.Background(), "test-redirect")
	if err != nil {
		t.Fatalf("failed to get stored message: %v", err)
	}
	if stored == nil {
		t.Error("expected message to be stored")
	}
	if stored.Mode != "redirect" {
		t.Errorf("expected mode redirect, got %s", stored.Mode)
	}
	if len(stored.OriginalTo) != 1 || stored.OriginalTo[0] != "original@example.com" {
		t.Errorf("expected original_to to be original@example.com, got %v", stored.OriginalTo)
	}
}

func TestSenderBCCMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	mock := &mockSender{}
	provider := &mockDomainProvider{
		modes: map[string]string{
			"bcc.com": "bcc",
		},
		bccAddrs: map[string][]string{
			"bcc.com": {"archive@testing.com"},
		},
	}

	sender := NewSender(mock, provider, storage, nil)

	msg := &queue.Message{
		ID:        "test-bcc",
		From:      "sender@bcc.com",
		To:        []string{"recipient@example.com"},
		Data:      []byte("Subject: Test\r\n\r\ntest message"),
		CreatedAt: time.Now(),
	}

	err = sender.Send(context.Background(), msg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Both original and BCC should be sent
	if len(mock.sentMessages) != 2 {
		t.Fatalf("expected 2 sent messages (original + BCC), got %d", len(mock.sentMessages))
	}

	// First should be original recipient
	if mock.sentMessages[0].To[0] != "recipient@example.com" {
		t.Errorf("expected first message to original recipient")
	}

	// Second should be BCC
	if mock.sentMessages[1].To[0] != "archive@testing.com" {
		t.Errorf("expected second message to BCC recipient")
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		email    string
		expected string
	}{
		{"user@example.com", "example.com"},
		{"User Name <user@example.com>", "example.com"},
		{"user@EXAMPLE.COM", "example.com"},
		{"invalid", ""},
		{"@example.com", ""},
		{"user@", ""},
	}

	for _, tc := range tests {
		result := extractDomain(tc.email)
		if result != tc.expected {
			t.Errorf("extractDomain(%q) = %q, expected %q", tc.email, result, tc.expected)
		}
	}
}

func TestExtractSubject(t *testing.T) {
	tests := []struct {
		data     []byte
		expected string
	}{
		{[]byte("Subject: Test Subject\r\n\r\nBody"), "Test Subject"},
		{[]byte("From: sender@example.com\r\nSubject: Hello World\r\n\r\nBody"), "Hello World"},
		{[]byte("From: sender@example.com\r\n\r\nBody"), ""},
		{[]byte("subject: lowercase\r\n\r\nBody"), "lowercase"},
	}

	for _, tc := range tests {
		result := extractSubject(tc.data)
		if result != tc.expected {
			t.Errorf("extractSubject(%q) = %q, expected %q", string(tc.data), result, tc.expected)
		}
	}
}
