package metrics

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	globalMetrics *Metrics
	globalMu      sync.RWMutex
)

// Metrics holds all Prometheus metrics for Sendry
type Metrics struct {
	// Message counters
	MessagesSentTotal     *prometheus.CounterVec
	MessagesFailedTotal   *prometheus.CounterVec
	MessagesBouncedTotal  *prometheus.CounterVec
	MessagesDeferredTotal *prometheus.CounterVec

	// Queue gauges
	QueueSize          prometheus.Gauge
	QueueOldestSeconds prometheus.Gauge
	QueueActive        prometheus.Gauge
	QueueDeferred      prometheus.Gauge

	// SMTP counters/gauges
	SMTPConnectionsTotal  *prometheus.CounterVec
	SMTPConnectionsActive prometheus.Gauge
	SMTPAuthSuccessTotal  prometheus.Counter
	SMTPAuthFailedTotal   prometheus.Counter
	SMTPTLSTotal          prometheus.Counter

	// API metrics
	APIRequestsTotal          *prometheus.CounterVec
	APIRequestDurationSeconds *prometheus.HistogramVec
	APIErrorsTotal            *prometheus.CounterVec

	// Rate limiting
	RateLimitExceededTotal *prometheus.CounterVec

	// System metrics
	UptimeSeconds    prometheus.Gauge
	Goroutines       prometheus.Gauge
	StorageUsedBytes prometheus.Gauge

	registry *prometheus.Registry

	// shadow mirrors counter values for persistence (see Collector)
	shadow   ShadowCounters
	shadowMu sync.Mutex
}

// New creates a new Metrics instance with all metrics registered
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		// Message counters
		MessagesSentTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_messages_sent_total",
				Help: "Total number of successfully delivered messages",
			},
			[]string{"domain"},
		),
		MessagesFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_messages_failed_total",
				Help: "Total number of permanently failed messages",
			},
			[]string{"domain", "error_type"},
		),
		MessagesBouncedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_messages_bounced_total",
				Help: "Total number of bounce messages sent",
			},
			[]string{"domain"},
		),
		MessagesDeferredTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_messages_deferred_total",
				Help: "Total number of messages deferred for retry",
			},
			[]string{"domain"},
		),

		// Queue gauges
		QueueSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_queue_size",
				Help: "Total number of pending and deferred messages in queue",
			},
		),
		QueueOldestSeconds: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_queue_oldest_seconds",
				Help: "Age of the oldest message in seconds",
			},
		),
		QueueActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_queue_active",
				Help: "Number of messages currently being processed",
			},
		),
		QueueDeferred: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_queue_deferred",
				Help: "Number of messages awaiting retry",
			},
		),

		// SMTP counters/gauges
		SMTPConnectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_smtp_connections_total",
				Help: "Total number of SMTP connections",
			},
			[]string{"server_type"},
		),
		SMTPConnectionsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_smtp_connections_active",
				Help: "Number of currently active SMTP connections",
			},
		),
		SMTPAuthSuccessTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "sendry_smtp_auth_success_total",
				Help: "Total number of successful SMTP authentications",
			},
		),
		SMTPAuthFailedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "sendry_smtp_auth_failed_total",
				Help: "Total number of failed SMTP authentications",
			},
		),
		SMTPTLSTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "sendry_smtp_tls_connections_total",
				Help: "Total number of TLS-upgraded SMTP connections",
			},
		),

		// API metrics
		APIRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_api_requests_total",
				Help: "Total number of API requests",
			},
			[]string{"method", "path", "status"},
		),
		APIRequestDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "sendry_api_request_duration_seconds",
				Help:    "API request duration in seconds",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		APIErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_api_errors_total",
				Help: "Total number of API errors",
			},
			[]string{"error_type"},
		),

		// Rate limiting
		RateLimitExceededTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sendry_ratelimit_exceeded_total",
				Help: "Total number of rate limit exceeded events",
			},
			[]string{"level"},
		),

		// System metrics
		UptimeSeconds: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_uptime_seconds",
				Help: "Server uptime in seconds",
			},
		),
		Goroutines: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_goroutines",
				Help: "Number of active goroutines",
			},
		),
		StorageUsedBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sendry_storage_used_bytes",
				Help: "BoltDB file size in bytes",
			},
		),

		registry: reg,
		shadow:   newShadowCounters(),
	}

	// Register all metrics
	reg.MustRegister(
		m.MessagesSentTotal,
		m.MessagesFailedTotal,
		m.MessagesBouncedTotal,
		m.MessagesDeferredTotal,
		m.QueueSize,
		m.QueueOldestSeconds,
		m.QueueActive,
		m.QueueDeferred,
		m.SMTPConnectionsTotal,
		m.SMTPConnectionsActive,
		m.SMTPAuthSuccessTotal,
		m.SMTPAuthFailedTotal,
		m.SMTPTLSTotal,
		m.APIRequestsTotal,
		m.APIRequestDurationSeconds,
		m.APIErrorsTotal,
		m.RateLimitExceededTotal,
		m.UptimeSeconds,
		m.Goroutines,
		m.StorageUsedBytes,
	)

	return m
}

// Registry returns the Prometheus registry
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// SetGlobal sets the global metrics instance
func SetGlobal(m *Metrics) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalMetrics = m
}

// Global returns the global metrics instance
func Global() *Metrics {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalMetrics
}

// IncMessagesSent increments the sent message counter
func IncMessagesSent(domain string) {
	if m := Global(); m != nil {
		m.MessagesSentTotal.WithLabelValues(domain).Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.MessagesSent[domain]++ })
	}
}

// IncMessagesFailed increments the failed message counter
func IncMessagesFailed(domain, errorType string) {
	if m := Global(); m != nil {
		m.MessagesFailedTotal.WithLabelValues(domain, errorType).Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.MessagesFailed[makeLabelKey(domain, errorType)]++ })
	}
}

// IncMessagesBounced increments the bounced message counter
func IncMessagesBounced(domain string) {
	if m := Global(); m != nil {
		m.MessagesBouncedTotal.WithLabelValues(domain).Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.MessagesBounced[domain]++ })
	}
}

// IncMessagesDeferred increments the deferred message counter
func IncMessagesDeferred(domain string) {
	if m := Global(); m != nil {
		m.MessagesDeferredTotal.WithLabelValues(domain).Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.MessagesDeferred[domain]++ })
	}
}

// IncSMTPConnections increments the SMTP connection counter
func IncSMTPConnections(serverType string) {
	if m := Global(); m != nil {
		m.SMTPConnectionsTotal.WithLabelValues(serverType).Inc()
		m.SMTPConnectionsActive.Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.SMTPConnections[serverType]++ })
	}
}

// DecSMTPConnectionsActive decrements active SMTP connections
func DecSMTPConnectionsActive() {
	if m := Global(); m != nil {
		m.SMTPConnectionsActive.Dec()
	}
}

// IncSMTPAuthSuccess increments successful auth counter
func IncSMTPAuthSuccess() {
	if m := Global(); m != nil {
		m.SMTPAuthSuccessTotal.Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.SMTPAuthSuccess++ })
	}
}

// IncSMTPAuthFailed increments failed auth counter
func IncSMTPAuthFailed() {
	if m := Global(); m != nil {
		m.SMTPAuthFailedTotal.Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.SMTPAuthFailed++ })
	}
}

// IncSMTPTLS increments TLS connection counter
func IncSMTPTLS() {
	if m := Global(); m != nil {
		m.SMTPTLSTotal.Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.SMTPTLS++ })
	}
}

// IncRateLimitExceeded increments rate limit exceeded counter
func IncRateLimitExceeded(level string) {
	if m := Global(); m != nil {
		m.RateLimitExceededTotal.WithLabelValues(level).Inc()
		m.updateShadow(func(sc *ShadowCounters) { sc.RateLimitExceeded[level]++ })
	}
}

// observeAPIRequest records an API request (and an API error for 4xx/5xx statuses)
func (m *Metrics) observeAPIRequest(method, path string, status int, durationSeconds float64) {
	statusStr := strconv.Itoa(status)
	m.APIRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.APIRequestDurationSeconds.WithLabelValues(method, path).Observe(durationSeconds)

	var errorType string
	if status >= 400 {
		errorType = categorizeStatus(status)
		m.APIErrorsTotal.WithLabelValues(errorType).Inc()
	}

	m.updateShadow(func(sc *ShadowCounters) {
		sc.APIRequests[makeTripleLabelKey(method, path, statusStr)]++
		if errorType != "" {
			sc.APIErrors[errorType]++
		}
	})
}

// updateShadow applies fn to the shadow counters under lock.
// Shadow counters mirror Prometheus counters so that the Collector
// can persist them to BoltDB and restore them after restart.
func (m *Metrics) updateShadow(fn func(sc *ShadowCounters)) {
	m.shadowMu.Lock()
	fn(&m.shadow)
	m.shadowMu.Unlock()
}
