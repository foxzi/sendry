package metrics

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// QueueStats contains queue statistics for metrics
type QueueStats struct {
	Pending   int64
	Sending   int64
	Deferred  int64
	Delivered int64
	Failed    int64
	Total     int64
}

// QueueStatsProvider provides queue statistics for metrics
type QueueStatsProvider interface {
	Stats(ctx context.Context) (*QueueStats, error)
}

var bucketMetrics = []byte("metrics")

// ShadowCounters stores counter values for persistence
type ShadowCounters struct {
	MessagesSent      map[string]float64 `json:"messages_sent"`
	MessagesFailed    map[string]float64 `json:"messages_failed"`
	MessagesBounced   map[string]float64 `json:"messages_bounced"`
	MessagesDeferred  map[string]float64 `json:"messages_deferred"`
	SMTPConnections   map[string]float64 `json:"smtp_connections"`
	SMTPAuthSuccess   float64            `json:"smtp_auth_success"`
	SMTPAuthFailed    float64            `json:"smtp_auth_failed"`
	SMTPTLS           float64            `json:"smtp_tls"`
	APIRequests       map[string]float64 `json:"api_requests"`
	APIErrors         map[string]float64 `json:"api_errors"`
	RateLimitExceeded map[string]float64 `json:"ratelimit_exceeded"`
}

// Collector handles metrics persistence and system gauge updates
type Collector struct {
	db            *bolt.DB
	metrics       *Metrics
	queueStats    QueueStatsProvider
	storagePath   string
	flushInterval time.Duration
	startTime     time.Time

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func newShadowCounters() ShadowCounters {
	return ShadowCounters{
		MessagesSent:      make(map[string]float64),
		MessagesFailed:    make(map[string]float64),
		MessagesBounced:   make(map[string]float64),
		MessagesDeferred:  make(map[string]float64),
		SMTPConnections:   make(map[string]float64),
		APIRequests:       make(map[string]float64),
		APIErrors:         make(map[string]float64),
		RateLimitExceeded: make(map[string]float64),
	}
}

// NewCollector creates a new metrics collector
func NewCollector(db *bolt.DB, m *Metrics, queueStats QueueStatsProvider, storagePath string, flushInterval time.Duration) (*Collector, error) {
	if flushInterval == 0 {
		flushInterval = 10 * time.Second
	}

	// Create bucket if not exists
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketMetrics)
		return err
	})
	if err != nil {
		return nil, err
	}

	c := &Collector{
		db:            db,
		metrics:       m,
		queueStats:    queueStats,
		storagePath:   storagePath,
		flushInterval: flushInterval,
		startTime:     time.Now(),
		stopCh:        make(chan struct{}),
	}

	// Load persisted counters
	if err := c.loadCounters(); err != nil {
		return nil, err
	}

	return c, nil
}

// Start begins the collector background tasks
func (c *Collector) Start(ctx context.Context) {
	c.wg.Add(2)
	go c.persistLoop(ctx)
	go c.updateSystemMetrics(ctx)
}

// Stop stops the collector and persists final values
func (c *Collector) Stop() error {
	close(c.stopCh)
	c.wg.Wait()
	return c.persistCounters()
}

// loadCounters loads persisted counter values from BoltDB
func (c *Collector) loadCounters() error {
	return c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketMetrics)
		if bucket == nil {
			return nil
		}

		data := bucket.Get([]byte("counters"))
		if data == nil {
			return nil
		}

		var shadow ShadowCounters
		if err := json.Unmarshal(data, &shadow); err != nil {
			return nil // Skip invalid data
		}

		m := c.metrics
		m.shadowMu.Lock()
		defer m.shadowMu.Unlock()

		// Restore message counters
		for k, v := range shadow.MessagesSent {
			m.shadow.MessagesSent[k] += v
			m.MessagesSentTotal.WithLabelValues(k).Add(v)
		}
		for k, v := range shadow.MessagesFailed {
			domain, errorType := splitLabelKey(k)
			m.shadow.MessagesFailed[k] += v
			m.MessagesFailedTotal.WithLabelValues(domain, errorType).Add(v)
		}
		for k, v := range shadow.MessagesBounced {
			m.shadow.MessagesBounced[k] += v
			m.MessagesBouncedTotal.WithLabelValues(k).Add(v)
		}
		for k, v := range shadow.MessagesDeferred {
			m.shadow.MessagesDeferred[k] += v
			m.MessagesDeferredTotal.WithLabelValues(k).Add(v)
		}

		// Restore SMTP counters
		for k, v := range shadow.SMTPConnections {
			m.shadow.SMTPConnections[k] += v
			m.SMTPConnectionsTotal.WithLabelValues(k).Add(v)
		}
		m.shadow.SMTPAuthSuccess += shadow.SMTPAuthSuccess
		m.shadow.SMTPAuthFailed += shadow.SMTPAuthFailed
		m.shadow.SMTPTLS += shadow.SMTPTLS
		m.SMTPAuthSuccessTotal.Add(shadow.SMTPAuthSuccess)
		m.SMTPAuthFailedTotal.Add(shadow.SMTPAuthFailed)
		m.SMTPTLSTotal.Add(shadow.SMTPTLS)

		// Restore API counters
		for k, v := range shadow.APIRequests {
			method, path, status := splitTripleLabelKey(k)
			m.shadow.APIRequests[k] += v
			m.APIRequestsTotal.WithLabelValues(method, path, status).Add(v)
		}
		for k, v := range shadow.APIErrors {
			m.shadow.APIErrors[k] += v
			m.APIErrorsTotal.WithLabelValues(k).Add(v)
		}

		// Restore rate limit counters
		for k, v := range shadow.RateLimitExceeded {
			m.shadow.RateLimitExceeded[k] += v
			m.RateLimitExceededTotal.WithLabelValues(k).Add(v)
		}

		return nil
	})
}

// persistCounters saves counter values to BoltDB
func (c *Collector) persistCounters() error {
	c.metrics.shadowMu.Lock()
	data, err := json.Marshal(c.metrics.shadow)
	c.metrics.shadowMu.Unlock()
	if err != nil {
		return err
	}

	return c.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketMetrics)
		if bucket == nil {
			return nil
		}
		return bucket.Put([]byte("counters"), data)
	})
}

// persistLoop periodically persists counter values
func (c *Collector) persistLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.persistCounters()
		}
	}
}

// updateSystemMetrics periodically updates system gauges
func (c *Collector) updateSystemMetrics(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.collectSystemMetrics(ctx)
		}
	}
}

// collectSystemMetrics collects current system state
func (c *Collector) collectSystemMetrics(ctx context.Context) {
	// Update uptime
	c.metrics.UptimeSeconds.Set(time.Since(c.startTime).Seconds())

	// Update goroutines
	c.metrics.Goroutines.Set(float64(runtime.NumGoroutine()))

	// Update storage size
	if c.storagePath != "" {
		if info, err := os.Stat(c.storagePath); err == nil {
			c.metrics.StorageUsedBytes.Set(float64(info.Size()))
		}
	}

	// Update queue stats
	if c.queueStats != nil {
		stats, err := c.queueStats.Stats(ctx)
		if err == nil {
			c.metrics.QueueSize.Set(float64(stats.Pending + stats.Deferred))
			c.metrics.QueueActive.Set(float64(stats.Sending))
			c.metrics.QueueDeferred.Set(float64(stats.Deferred))
		}
	}
}

// Helper functions for label key serialization
func makeLabelKey(a, b string) string {
	return a + "|" + b
}

func splitLabelKey(key string) (string, string) {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '|' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

func makeTripleLabelKey(a, b, c string) string {
	return a + "|" + b + "|" + c
}

func splitTripleLabelKey(key string) (string, string, string) {
	parts := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])

	if len(parts) >= 3 {
		return parts[0], parts[1], parts[2]
	}
	if len(parts) == 2 {
		return parts[0], parts[1], ""
	}
	if len(parts) == 1 {
		return parts[0], "", ""
	}
	return "", "", ""
}
