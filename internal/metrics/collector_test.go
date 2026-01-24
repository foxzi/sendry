package metrics

import (
	"context"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

type mockQueueStatsProvider struct {
	stats *QueueStats
}

func (m *mockQueueStatsProvider) Stats(ctx context.Context) (*QueueStats, error) {
	return m.stats, nil
}

func TestNewCollector(t *testing.T) {
	// Create temp database
	f, err := os.CreateTemp("", "metrics_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	f.Close()
	defer os.Remove(f.Name())

	db, err := bolt.Open(f.Name(), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	m := New()
	queueStats := &mockQueueStatsProvider{
		stats: &QueueStats{
			Pending:  10,
			Sending:  2,
			Deferred: 5,
		},
	}

	c, err := NewCollector(db, m, queueStats, f.Name(), 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to create collector: %v", err)
	}

	if c == nil {
		t.Fatal("Collector is nil")
	}

	if err := c.Stop(); err != nil {
		t.Errorf("Failed to stop collector: %v", err)
	}
}

func TestCollectorPersistence(t *testing.T) {
	// Create temp database
	f, err := os.CreateTemp("", "metrics_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	f.Close()
	defer os.Remove(f.Name())

	db, err := bolt.Open(f.Name(), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	m := New()
	queueStats := &mockQueueStatsProvider{
		stats: &QueueStats{
			Pending: 10,
		},
	}

	c, err := NewCollector(db, m, queueStats, f.Name(), 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to create collector: %v", err)
	}

	// Track some metrics
	c.TrackMessageSent("example.com")
	c.TrackMessageSent("example.com")
	c.TrackMessageFailed("example.com", "timeout")
	c.TrackSMTPConnection("smtp")
	c.TrackSMTPAuthSuccess()
	c.TrackRateLimitExceeded("global")

	// Stop collector (should persist)
	if err := c.Stop(); err != nil {
		t.Errorf("Failed to stop collector: %v", err)
	}
	db.Close()

	// Reopen database and create new collector
	db2, err := bolt.Open(f.Name(), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer db2.Close()

	m2 := New()
	c2, err := NewCollector(db2, m2, queueStats, f.Name(), 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to recreate collector: %v", err)
	}
	defer c2.Stop()

	// Check that counters were restored
	if c2.shadow.MessagesSent["example.com"] != 2 {
		t.Errorf("Expected MessagesSent[example.com] = 2, got %f", c2.shadow.MessagesSent["example.com"])
	}

	if c2.shadow.SMTPAuthSuccess != 1 {
		t.Errorf("Expected SMTPAuthSuccess = 1, got %f", c2.shadow.SMTPAuthSuccess)
	}
}

func TestCollectorTrackMethods(t *testing.T) {
	f, err := os.CreateTemp("", "metrics_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	f.Close()
	defer os.Remove(f.Name())

	db, err := bolt.Open(f.Name(), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	m := New()
	queueStats := &mockQueueStatsProvider{stats: &QueueStats{}}

	c, err := NewCollector(db, m, queueStats, f.Name(), 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to create collector: %v", err)
	}
	defer c.Stop()

	// Test all track methods
	c.TrackMessageSent("example.com")
	if c.shadow.MessagesSent["example.com"] != 1 {
		t.Error("TrackMessageSent failed")
	}

	c.TrackMessageFailed("example.com", "timeout")
	if c.shadow.MessagesFailed["example.com|timeout"] != 1 {
		t.Error("TrackMessageFailed failed")
	}

	c.TrackMessageBounced("example.com")
	if c.shadow.MessagesBounced["example.com"] != 1 {
		t.Error("TrackMessageBounced failed")
	}

	c.TrackMessageDeferred("example.com")
	if c.shadow.MessagesDeferred["example.com"] != 1 {
		t.Error("TrackMessageDeferred failed")
	}

	c.TrackSMTPConnection("smtp")
	if c.shadow.SMTPConnections["smtp"] != 1 {
		t.Error("TrackSMTPConnection failed")
	}

	c.TrackSMTPAuthSuccess()
	if c.shadow.SMTPAuthSuccess != 1 {
		t.Error("TrackSMTPAuthSuccess failed")
	}

	c.TrackSMTPAuthFailed()
	if c.shadow.SMTPAuthFailed != 1 {
		t.Error("TrackSMTPAuthFailed failed")
	}

	c.TrackSMTPTLS()
	if c.shadow.SMTPTLS != 1 {
		t.Error("TrackSMTPTLS failed")
	}

	c.TrackAPIRequest("GET", "/api/v1/status", "200")
	if c.shadow.APIRequests["GET|/api/v1/status|200"] != 1 {
		t.Error("TrackAPIRequest failed")
	}

	c.TrackAPIError("server_error")
	if c.shadow.APIErrors["server_error"] != 1 {
		t.Error("TrackAPIError failed")
	}

	c.TrackRateLimitExceeded("global")
	if c.shadow.RateLimitExceeded["global"] != 1 {
		t.Error("TrackRateLimitExceeded failed")
	}
}

func TestLabelKeyHelpers(t *testing.T) {
	// Test makeLabelKey and splitLabelKey
	key := makeLabelKey("domain", "errortype")
	a, b := splitLabelKey(key)
	if a != "domain" || b != "errortype" {
		t.Errorf("Expected (domain, errortype), got (%s, %s)", a, b)
	}

	// Test makeTripleLabelKey and splitTripleLabelKey
	tripleKey := makeTripleLabelKey("GET", "/api", "200")
	m, p, s := splitTripleLabelKey(tripleKey)
	if m != "GET" || p != "/api" || s != "200" {
		t.Errorf("Expected (GET, /api, 200), got (%s, %s, %s)", m, p, s)
	}
}
