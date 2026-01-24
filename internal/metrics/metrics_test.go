package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}

	if m.Registry() == nil {
		t.Error("Registry() returned nil")
	}

	// Check that all metrics are registered
	if m.MessagesSentTotal == nil {
		t.Error("MessagesSentTotal is nil")
	}
	if m.MessagesFailedTotal == nil {
		t.Error("MessagesFailedTotal is nil")
	}
	if m.MessagesBouncedTotal == nil {
		t.Error("MessagesBouncedTotal is nil")
	}
	if m.MessagesDeferredTotal == nil {
		t.Error("MessagesDeferredTotal is nil")
	}
	if m.QueueSize == nil {
		t.Error("QueueSize is nil")
	}
	if m.SMTPConnectionsTotal == nil {
		t.Error("SMTPConnectionsTotal is nil")
	}
	if m.SMTPConnectionsActive == nil {
		t.Error("SMTPConnectionsActive is nil")
	}
	if m.APIRequestsTotal == nil {
		t.Error("APIRequestsTotal is nil")
	}
	if m.APIRequestDurationSeconds == nil {
		t.Error("APIRequestDurationSeconds is nil")
	}
}

func TestGlobalMetrics(t *testing.T) {
	// Initially global should be nil
	if Global() != nil {
		t.Error("Global() should be nil before SetGlobal")
	}

	m := New()
	SetGlobal(m)

	if Global() != m {
		t.Error("Global() did not return the set metrics")
	}

	// Cleanup
	SetGlobal(nil)
}

func TestIncMessagesSent(t *testing.T) {
	m := New()
	SetGlobal(m)
	defer SetGlobal(nil)

	IncMessagesSent("example.com")
	IncMessagesSent("example.com")
	IncMessagesSent("other.com")

	// Check counter value
	counter, err := m.MessagesSentTotal.GetMetricWithLabelValues("example.com")
	if err != nil {
		t.Fatalf("Failed to get counter: %v", err)
	}

	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 2 {
		t.Errorf("Expected counter value 2, got %f", metric.Counter.GetValue())
	}
}

func TestIncMessagesFailed(t *testing.T) {
	m := New()
	SetGlobal(m)
	defer SetGlobal(nil)

	IncMessagesFailed("example.com", "timeout")
	IncMessagesFailed("example.com", "dns_error")
	IncMessagesFailed("example.com", "timeout")

	counter, err := m.MessagesFailedTotal.GetMetricWithLabelValues("example.com", "timeout")
	if err != nil {
		t.Fatalf("Failed to get counter: %v", err)
	}

	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 2 {
		t.Errorf("Expected counter value 2, got %f", metric.Counter.GetValue())
	}
}

func TestIncSMTPConnections(t *testing.T) {
	m := New()
	SetGlobal(m)
	defer SetGlobal(nil)

	IncSMTPConnections("smtp")
	IncSMTPConnections("smtp")
	IncSMTPConnections("smtps")

	// Check total connections counter
	counter, err := m.SMTPConnectionsTotal.GetMetricWithLabelValues("smtp")
	if err != nil {
		t.Fatalf("Failed to get counter: %v", err)
	}

	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 2 {
		t.Errorf("Expected smtp connections 2, got %f", metric.Counter.GetValue())
	}

	// Check active connections gauge
	var activeMetric dto.Metric
	if err := m.SMTPConnectionsActive.(prometheus.Gauge).Write(&activeMetric); err != nil {
		t.Fatalf("Failed to write active metric: %v", err)
	}

	if activeMetric.Gauge.GetValue() != 3 {
		t.Errorf("Expected active connections 3, got %f", activeMetric.Gauge.GetValue())
	}
}

func TestDecSMTPConnectionsActive(t *testing.T) {
	m := New()
	SetGlobal(m)
	defer SetGlobal(nil)

	// Start with 2 connections
	IncSMTPConnections("smtp")
	IncSMTPConnections("smtp")

	// Decrement one
	DecSMTPConnectionsActive()

	var metric dto.Metric
	if err := m.SMTPConnectionsActive.(prometheus.Gauge).Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Gauge.GetValue() != 1 {
		t.Errorf("Expected active connections 1, got %f", metric.Gauge.GetValue())
	}
}

func TestIncSMTPAuth(t *testing.T) {
	m := New()
	SetGlobal(m)
	defer SetGlobal(nil)

	IncSMTPAuthSuccess()
	IncSMTPAuthSuccess()
	IncSMTPAuthFailed()

	var successMetric dto.Metric
	if err := m.SMTPAuthSuccessTotal.(prometheus.Counter).Write(&successMetric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if successMetric.Counter.GetValue() != 2 {
		t.Errorf("Expected auth success 2, got %f", successMetric.Counter.GetValue())
	}

	var failedMetric dto.Metric
	if err := m.SMTPAuthFailedTotal.(prometheus.Counter).Write(&failedMetric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if failedMetric.Counter.GetValue() != 1 {
		t.Errorf("Expected auth failed 1, got %f", failedMetric.Counter.GetValue())
	}
}

func TestIncRateLimitExceeded(t *testing.T) {
	m := New()
	SetGlobal(m)
	defer SetGlobal(nil)

	IncRateLimitExceeded("global")
	IncRateLimitExceeded("domain")
	IncRateLimitExceeded("global")

	counter, err := m.RateLimitExceededTotal.GetMetricWithLabelValues("global")
	if err != nil {
		t.Fatalf("Failed to get counter: %v", err)
	}

	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 2 {
		t.Errorf("Expected rate limit exceeded 2, got %f", metric.Counter.GetValue())
	}
}

func TestGlobalNilSafe(t *testing.T) {
	SetGlobal(nil)

	// These should not panic when global is nil
	IncMessagesSent("example.com")
	IncMessagesFailed("example.com", "timeout")
	IncMessagesBounced("example.com")
	IncMessagesDeferred("example.com")
	IncSMTPConnections("smtp")
	DecSMTPConnectionsActive()
	IncSMTPAuthSuccess()
	IncSMTPAuthFailed()
	IncSMTPTLS()
	IncRateLimitExceeded("global")
	IncAPIErrors("server_error")
}
