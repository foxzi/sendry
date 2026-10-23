package metrics

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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

	// Increment counters through the global helpers used by the application
	SetGlobal(m)
	defer SetGlobal(nil)

	IncMessagesSent("example.com")
	IncMessagesSent("example.com")
	IncMessagesFailed("example.com", "timeout")
	IncMessagesBounced("example.com")
	IncMessagesDeferred("example.com")
	IncSMTPConnections("smtp")
	IncSMTPAuthSuccess()
	IncSMTPAuthFailed()
	IncSMTPTLS()
	IncRateLimitExceeded("global")
	m.observeAPIRequest("GET", "/api/v1/status", 200, 0.01)
	m.observeAPIRequest("POST", "/api/v1/send", 500, 0.01)

	// Stop collector (should persist)
	if err := c.Stop(); err != nil {
		t.Errorf("Failed to stop collector: %v", err)
	}
	db.Close()

	// Reopen database and create new collector (simulates restart)
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

	// Check that Prometheus counters were restored
	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"messages_sent", counterValue(t, m2.MessagesSentTotal.WithLabelValues("example.com")), 2},
		{"messages_failed", counterValue(t, m2.MessagesFailedTotal.WithLabelValues("example.com", "timeout")), 1},
		{"messages_bounced", counterValue(t, m2.MessagesBouncedTotal.WithLabelValues("example.com")), 1},
		{"messages_deferred", counterValue(t, m2.MessagesDeferredTotal.WithLabelValues("example.com")), 1},
		{"smtp_connections", counterValue(t, m2.SMTPConnectionsTotal.WithLabelValues("smtp")), 1},
		{"smtp_auth_success", counterValue(t, m2.SMTPAuthSuccessTotal), 1},
		{"smtp_auth_failed", counterValue(t, m2.SMTPAuthFailedTotal), 1},
		{"smtp_tls", counterValue(t, m2.SMTPTLSTotal), 1},
		{"ratelimit_exceeded", counterValue(t, m2.RateLimitExceededTotal.WithLabelValues("global")), 1},
		{"api_requests", counterValue(t, m2.APIRequestsTotal.WithLabelValues("GET", "/api/v1/status", "200")), 1},
		{"api_errors", counterValue(t, m2.APIErrorsTotal.WithLabelValues("server_error")), 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: expected %v after restart, got %v", c.name, c.want, c.got)
		}
	}

	// Counters keep growing on top of restored values
	SetGlobal(m2)
	IncMessagesSent("example.com")
	if got := counterValue(t, m2.MessagesSentTotal.WithLabelValues("example.com")); got != 3 {
		t.Errorf("expected messages_sent = 3 after increment, got %v", got)
	}
	m2.shadowMu.Lock()
	shadowSent := m2.shadow.MessagesSent["example.com"]
	m2.shadowMu.Unlock()
	if shadowSent != 3 {
		t.Errorf("expected shadow messages_sent = 3, got %v", shadowSent)
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

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var metric dto.Metric
	if err := c.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	return metric.Counter.GetValue()
}
