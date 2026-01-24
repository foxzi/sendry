package metrics

import (
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
	QueueSize           prometheus.Gauge
	QueueOldestSeconds  prometheus.Gauge
	QueueActive         prometheus.Gauge
	QueueDeferred       prometheus.Gauge

	// SMTP counters/gauges
	SMTPConnectionsTotal  *prometheus.CounterVec
	SMTPConnectionsActive prometheus.Gauge
	SMTPAuthSuccessTotal  prometheus.Counter
	SMTPAuthFailedTotal   prometheus.Counter
	SMTPTLSTotal          prometheus.Counter

	// API metrics
	APIRequestsTotal         *prometheus.CounterVec
	APIRequestDurationSeconds *prometheus.HistogramVec
	APIErrorsTotal           *prometheus.CounterVec

	// Rate limiting
	RateLimitExceededTotal *prometheus.CounterVec

	// System metrics
	UptimeSeconds     prometheus.Gauge
	Goroutines        prometheus.Gauge
	StorageUsedBytes  prometheus.Gauge

	registry *prometheus.Registry
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
	m := Global()
	if m != nil {
		m.MessagesSentTotal.WithLabelValues(domain).Inc()
	}
}

// IncMessagesFailed increments the failed message counter
func IncMessagesFailed(domain, errorType string) {
	m := Global()
	if m != nil {
		m.MessagesFailedTotal.WithLabelValues(domain, errorType).Inc()
	}
}

// IncMessagesBounced increments the bounced message counter
func IncMessagesBounced(domain string) {
	m := Global()
	if m != nil {
		m.MessagesBouncedTotal.WithLabelValues(domain).Inc()
	}
}

// IncMessagesDeferred increments the deferred message counter
func IncMessagesDeferred(domain string) {
	m := Global()
	if m != nil {
		m.MessagesDeferredTotal.WithLabelValues(domain).Inc()
	}
}

// IncSMTPConnections increments the SMTP connection counter
func IncSMTPConnections(serverType string) {
	m := Global()
	if m != nil {
		m.SMTPConnectionsTotal.WithLabelValues(serverType).Inc()
		m.SMTPConnectionsActive.Inc()
	}
}

// DecSMTPConnectionsActive decrements active SMTP connections
func DecSMTPConnectionsActive() {
	m := Global()
	if m != nil {
		m.SMTPConnectionsActive.Dec()
	}
}

// IncSMTPAuthSuccess increments successful auth counter
func IncSMTPAuthSuccess() {
	m := Global()
	if m != nil {
		m.SMTPAuthSuccessTotal.Inc()
	}
}

// IncSMTPAuthFailed increments failed auth counter
func IncSMTPAuthFailed() {
	m := Global()
	if m != nil {
		m.SMTPAuthFailedTotal.Inc()
	}
}

// IncSMTPTLS increments TLS connection counter
func IncSMTPTLS() {
	m := Global()
	if m != nil {
		m.SMTPTLSTotal.Inc()
	}
}

// IncRateLimitExceeded increments rate limit exceeded counter
func IncRateLimitExceeded(level string) {
	m := Global()
	if m != nil {
		m.RateLimitExceededTotal.WithLabelValues(level).Inc()
	}
}

// IncAPIErrors increments API error counter
func IncAPIErrors(errorType string) {
	m := Global()
	if m != nil {
		m.APIErrorsTotal.WithLabelValues(errorType).Inc()
	}
}
