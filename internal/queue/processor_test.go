package queue

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/ratelimit"
)

// mockSender implements Sender for testing
type mockSender struct {
	sendFunc func(ctx context.Context, msg *Message) error
	sent     []*Message
}

func (m *mockSender) Send(ctx context.Context, msg *Message) error {
	m.sent = append(m.sent, msg)
	if m.sendFunc != nil {
		return m.sendFunc(ctx, msg)
	}
	return nil
}

func TestProcessorWithRateLimiter(t *testing.T) {
	// Create temp dir for DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "queue.db")

	// Create storage
	storage, err := NewBoltStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	// Create rate limiter with strict limit for gmail.com
	rlCfg := &ratelimit.Config{
		RecipientDomains: map[string]*ratelimit.LimitConfig{
			"gmail.com": {
				MessagesPerHour: 1,
				MessagesPerDay:  5,
			},
		},
	}
	limiter, err := ratelimit.NewLimiter(storage.DB(), rlCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer limiter.Stop()

	// Create mock sender
	sender := &mockSender{}

	// Create processor
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := ProcessorConfig{
		Workers:         1,
		RetryInterval:   time.Second,
		MaxRetries:      3,
		ProcessInterval: 100 * time.Millisecond,
	}
	processor := NewProcessor(storage, sender, cfg, nil, logger)
	processor.SetRateLimiter(limiter)

	// Enqueue two messages to gmail.com
	msg1 := &Message{
		ID:        "msg1",
		From:      "test@example.com",
		To:        []string{"user1@gmail.com"},
		Data:      []byte("test email 1"),
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	msg2 := &Message{
		ID:        "msg2",
		From:      "test@example.com",
		To:        []string{"user2@gmail.com"},
		Data:      []byte("test email 2"),
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	if err := storage.Enqueue(context.Background(), msg1); err != nil {
		t.Fatal(err)
	}
	if err := storage.Enqueue(context.Background(), msg2); err != nil {
		t.Fatal(err)
	}

	// Start processor
	ctx, cancel := context.WithCancel(context.Background())
	processor.Start(ctx)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)
	cancel()
	processor.Stop()

	// First message should be sent
	if len(sender.sent) != 1 {
		t.Errorf("expected 1 message sent, got %d", len(sender.sent))
	}

	// Check second message was deferred
	msg2Updated, _ := storage.Get(context.Background(), "msg2")
	if msg2Updated != nil && msg2Updated.Status != StatusDeferred {
		t.Errorf("expected msg2 status deferred, got %s", msg2Updated.Status)
	}
}

func TestProcessorRetryOnError(t *testing.T) {
	// Create temp dir for DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "queue.db")

	storage, err := NewBoltStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	// Create sender that fails first time
	attempts := 0
	sender := &mockSender{
		sendFunc: func(ctx context.Context, msg *Message) error {
			attempts++
			if attempts == 1 {
				return errors.New("temporary error")
			}
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := ProcessorConfig{
		Workers:         1,
		RetryInterval:   10 * time.Millisecond, // Fast retry for test
		MaxRetries:      3,
		ProcessInterval: 50 * time.Millisecond,
	}
	isTemp := func(err error) bool { return true }
	processor := NewProcessor(storage, sender, cfg, isTemp, logger)

	// Enqueue message
	msg := &Message{
		ID:        "retry-test",
		From:      "test@example.com",
		To:        []string{"user@test.com"},
		Data:      []byte("test"),
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	if err := storage.Enqueue(context.Background(), msg); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	processor.Start(ctx)

	// Wait for retry
	time.Sleep(300 * time.Millisecond)
	cancel()
	processor.Stop()

	// Should have attempted at least twice
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts)
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, "unknown"},
		{"connection refused", errors.New("connection refused"), "connection_refused"},
		{"timeout", errors.New("connection timeout"), "timeout"},
		{"dns error", errors.New("no such host"), "dns_error"},
		{"smtp 550", errors.New("550 user unknown"), "recipient_rejected"},
		{"smtp 554", errors.New("554 spam detected"), "spam_rejected"},
		{"relay denied 550", errors.New("550 relay access denied"), "relay_denied"},
		{"tls error", errors.New("tls handshake failed"), "tls_error"},
		{"generic", errors.New("some other error"), "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			if result != tt.expected {
				t.Errorf("classifyError(%v) = %s, want %s", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsBounceMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		expected bool
	}{
		{"normal message", &Message{ID: "msg1", From: "user@example.com"}, false},
		{"bounce suffix", &Message{ID: "msg1-bounce", From: "user@example.com"}, true},
		{"empty from", &Message{ID: "msg1", From: ""}, true},
		{"null sender", &Message{ID: "msg1", From: "<>"}, true},
		{"mailer-daemon", &Message{ID: "msg1", From: "MAILER-DAEMON@example.com"}, true},
		{"postmaster", &Message{ID: "msg1", From: "postmaster@example.com"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBounceMessage(tt.msg)
			if result != tt.expected {
				t.Errorf("isBounceMessage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	p := &Processor{
		retryInterval: 5 * time.Minute,
		logger:        logger,
	}

	tests := []struct {
		retryCount int
		minBackoff time.Duration
		maxBackoff time.Duration
	}{
		{1, 5 * time.Minute, 5 * time.Minute},
		{2, 10 * time.Minute, 10 * time.Minute},
		{3, 20 * time.Minute, 20 * time.Minute},
		{4, 40 * time.Minute, 40 * time.Minute},
		{5, 60 * time.Minute, 60 * time.Minute}, // Should cap at 1 hour
		{10, 60 * time.Minute, 60 * time.Minute},
	}

	for _, tt := range tests {
		backoff := p.calculateBackoff(tt.retryCount)
		if backoff < tt.minBackoff || backoff > tt.maxBackoff {
			t.Errorf("calculateBackoff(%d) = %v, want between %v and %v",
				tt.retryCount, backoff, tt.minBackoff, tt.maxBackoff)
		}
	}
}
