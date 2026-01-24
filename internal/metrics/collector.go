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
	MessagesSent     map[string]float64 `json:"messages_sent"`
	MessagesFailed   map[string]float64 `json:"messages_failed"`
	MessagesBounced  map[string]float64 `json:"messages_bounced"`
	MessagesDeferred map[string]float64 `json:"messages_deferred"`
	SMTPConnections  map[string]float64 `json:"smtp_connections"`
	SMTPAuthSuccess  float64            `json:"smtp_auth_success"`
	SMTPAuthFailed   float64            `json:"smtp_auth_failed"`
	SMTPTLS          float64            `json:"smtp_tls"`
	APIRequests      map[string]float64 `json:"api_requests"`
	APIErrors        map[string]float64 `json:"api_errors"`
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

	shadow ShadowCounters
	mu     sync.Mutex
	stopCh chan struct{}
	wg     sync.WaitGroup
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
		shadow: ShadowCounters{
			MessagesSent:      make(map[string]float64),
			MessagesFailed:    make(map[string]float64),
			MessagesBounced:   make(map[string]float64),
			MessagesDeferred:  make(map[string]float64),
			SMTPConnections:   make(map[string]float64),
			APIRequests:       make(map[string]float64),
			APIErrors:         make(map[string]float64),
			RateLimitExceeded: make(map[string]float64),
		},
		stopCh: make(chan struct{}),
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

		c.mu.Lock()
		defer c.mu.Unlock()

		// Restore message counters
		for k, v := range shadow.MessagesSent {
			c.shadow.MessagesSent[k] = v
			c.metrics.MessagesSentTotal.WithLabelValues(k).Add(v)
		}
		for k, v := range shadow.MessagesFailed {
			domain, errorType := splitLabelKey(k)
			c.shadow.MessagesFailed[k] = v
			c.metrics.MessagesFailedTotal.WithLabelValues(domain, errorType).Add(v)
		}
		for k, v := range shadow.MessagesBounced {
			c.shadow.MessagesBounced[k] = v
			c.metrics.MessagesBouncedTotal.WithLabelValues(k).Add(v)
		}
		for k, v := range shadow.MessagesDeferred {
			c.shadow.MessagesDeferred[k] = v
			c.metrics.MessagesDeferredTotal.WithLabelValues(k).Add(v)
		}

		// Restore SMTP counters
		for k, v := range shadow.SMTPConnections {
			c.shadow.SMTPConnections[k] = v
			c.metrics.SMTPConnectionsTotal.WithLabelValues(k).Add(v)
		}
		c.shadow.SMTPAuthSuccess = shadow.SMTPAuthSuccess
		c.shadow.SMTPAuthFailed = shadow.SMTPAuthFailed
		c.shadow.SMTPTLS = shadow.SMTPTLS
		c.metrics.SMTPAuthSuccessTotal.Add(shadow.SMTPAuthSuccess)
		c.metrics.SMTPAuthFailedTotal.Add(shadow.SMTPAuthFailed)
		c.metrics.SMTPTLSTotal.Add(shadow.SMTPTLS)

		// Restore API counters
		for k, v := range shadow.APIRequests {
			method, path, status := splitTripleLabelKey(k)
			c.shadow.APIRequests[k] = v
			c.metrics.APIRequestsTotal.WithLabelValues(method, path, status).Add(v)
		}
		for k, v := range shadow.APIErrors {
			c.shadow.APIErrors[k] = v
			c.metrics.APIErrorsTotal.WithLabelValues(k).Add(v)
		}

		// Restore rate limit counters
		for k, v := range shadow.RateLimitExceeded {
			c.shadow.RateLimitExceeded[k] = v
			c.metrics.RateLimitExceededTotal.WithLabelValues(k).Add(v)
		}

		return nil
	})
}

// persistCounters saves counter values to BoltDB
func (c *Collector) persistCounters() error {
	c.mu.Lock()
	shadow := c.shadow
	c.mu.Unlock()

	return c.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketMetrics)
		if bucket == nil {
			return nil
		}

		data, err := json.Marshal(shadow)
		if err != nil {
			return err
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

// TrackMessageSent tracks a sent message and updates shadow counter
func (c *Collector) TrackMessageSent(domain string) {
	c.mu.Lock()
	c.shadow.MessagesSent[domain]++
	c.mu.Unlock()
	c.metrics.MessagesSentTotal.WithLabelValues(domain).Inc()
}

// TrackMessageFailed tracks a failed message and updates shadow counter
func (c *Collector) TrackMessageFailed(domain, errorType string) {
	key := makeLabelKey(domain, errorType)
	c.mu.Lock()
	c.shadow.MessagesFailed[key]++
	c.mu.Unlock()
	c.metrics.MessagesFailedTotal.WithLabelValues(domain, errorType).Inc()
}

// TrackMessageBounced tracks a bounced message and updates shadow counter
func (c *Collector) TrackMessageBounced(domain string) {
	c.mu.Lock()
	c.shadow.MessagesBounced[domain]++
	c.mu.Unlock()
	c.metrics.MessagesBouncedTotal.WithLabelValues(domain).Inc()
}

// TrackMessageDeferred tracks a deferred message and updates shadow counter
func (c *Collector) TrackMessageDeferred(domain string) {
	c.mu.Lock()
	c.shadow.MessagesDeferred[domain]++
	c.mu.Unlock()
	c.metrics.MessagesDeferredTotal.WithLabelValues(domain).Inc()
}

// TrackSMTPConnection tracks an SMTP connection and updates shadow counter
func (c *Collector) TrackSMTPConnection(serverType string) {
	c.mu.Lock()
	c.shadow.SMTPConnections[serverType]++
	c.mu.Unlock()
	c.metrics.SMTPConnectionsTotal.WithLabelValues(serverType).Inc()
	c.metrics.SMTPConnectionsActive.Inc()
}

// TrackSMTPAuthSuccess tracks successful SMTP auth and updates shadow counter
func (c *Collector) TrackSMTPAuthSuccess() {
	c.mu.Lock()
	c.shadow.SMTPAuthSuccess++
	c.mu.Unlock()
	c.metrics.SMTPAuthSuccessTotal.Inc()
}

// TrackSMTPAuthFailed tracks failed SMTP auth and updates shadow counter
func (c *Collector) TrackSMTPAuthFailed() {
	c.mu.Lock()
	c.shadow.SMTPAuthFailed++
	c.mu.Unlock()
	c.metrics.SMTPAuthFailedTotal.Inc()
}

// TrackSMTPTLS tracks TLS connection and updates shadow counter
func (c *Collector) TrackSMTPTLS() {
	c.mu.Lock()
	c.shadow.SMTPTLS++
	c.mu.Unlock()
	c.metrics.SMTPTLSTotal.Inc()
}

// TrackAPIRequest tracks an API request and updates shadow counter
func (c *Collector) TrackAPIRequest(method, path, status string) {
	key := makeTripleLabelKey(method, path, status)
	c.mu.Lock()
	c.shadow.APIRequests[key]++
	c.mu.Unlock()
	c.metrics.APIRequestsTotal.WithLabelValues(method, path, status).Inc()
}

// TrackAPIError tracks an API error and updates shadow counter
func (c *Collector) TrackAPIError(errorType string) {
	c.mu.Lock()
	c.shadow.APIErrors[errorType]++
	c.mu.Unlock()
	c.metrics.APIErrorsTotal.WithLabelValues(errorType).Inc()
}

// TrackRateLimitExceeded tracks rate limit exceeded and updates shadow counter
func (c *Collector) TrackRateLimitExceeded(level string) {
	c.mu.Lock()
	c.shadow.RateLimitExceeded[level]++
	c.mu.Unlock()
	c.metrics.RateLimitExceededTotal.WithLabelValues(level).Inc()
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
