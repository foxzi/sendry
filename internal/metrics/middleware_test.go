package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := wrapResponseWriter(w)

	// Test initial status
	if rw.status != http.StatusOK {
		t.Errorf("Expected initial status %d, got %d", http.StatusOK, rw.status)
	}

	// Test WriteHeader
	rw.WriteHeader(http.StatusNotFound)
	if rw.status != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, rw.status)
	}

	// Test double WriteHeader (should be ignored)
	rw.WriteHeader(http.StatusInternalServerError)
	if rw.status != http.StatusNotFound {
		t.Errorf("Expected status to remain %d, got %d", http.StatusNotFound, rw.status)
	}
}

func TestResponseWriterWrite(t *testing.T) {
	w := httptest.NewRecorder()
	rw := wrapResponseWriter(w)

	// Write without explicit WriteHeader
	_, err := rw.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Status should default to 200
	if rw.status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rw.status)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	m := New()
	SetGlobal(m)
	defer SetGlobal(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	wrapped := HTTPMiddleware(handler)

	req := httptest.NewRequest("GET", "/api/v1/status/123", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHTTPMiddlewareNoMetrics(t *testing.T) {
	SetGlobal(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTPMiddleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic when global metrics is nil
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"550E8400-E29B-41D4-A716-446655440000", true},
		{"not-a-uuid", false},
		{"550e8400e29b41d4a716446655440000", false}, // missing dashes
		{"", false},
		{"550e8400-e29b-41d4-a716-44665544000", false}, // too short
		{"550e8400-e29b-41d4-a716-4466554400000", false}, // too long
	}

	for _, tt := range tests {
		result := isUUID(tt.input)
		if result != tt.expected {
			t.Errorf("isUUID(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestCategorizeStatus(t *testing.T) {
	tests := []struct {
		status   int
		expected string
	}{
		{500, "server_error"},
		{503, "server_error"},
		{429, "rate_limited"},
		{401, "auth_error"},
		{403, "auth_error"},
		{404, "not_found"},
		{400, "bad_request"},
		{422, "client_error"},
		{200, "unknown"},
		{201, "unknown"},
	}

	for _, tt := range tests {
		result := categorizeStatus(tt.status)
		if result != tt.expected {
			t.Errorf("categorizeStatus(%d) = %q, expected %q", tt.status, result, tt.expected)
		}
	}
}
