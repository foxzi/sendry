package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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

// Requests that match no route share one path label instead of one series
// per probed URL; matched requests use the route pattern.
func TestNormalizePath(t *testing.T) {
	r := chi.NewRouter()
	var got []string
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			got = append(got, normalizePath(req))
		})
	})
	r.Get("/api/v1/status/{id}", func(w http.ResponseWriter, r *http.Request) {})
	for _, p := range []string{"/api/v1/status/123", "/wp-login.php", "/.env"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}
	want := []string{"/api/v1/status/{id}", unmatchedPath, unmatchedPath}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("paths = %v, want %v", got, want)
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
