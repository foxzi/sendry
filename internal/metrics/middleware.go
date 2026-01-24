package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ResponseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// HTTPMiddleware creates a middleware that records HTTP request metrics
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := Global()
		if m == nil {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := wrapResponseWriter(w)

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(wrapped.status)

		// Normalize path to avoid high cardinality
		path := normalizePath(r)

		m.APIRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		m.APIRequestDurationSeconds.WithLabelValues(r.Method, path).Observe(duration)

		// Track errors
		if wrapped.status >= 400 {
			errorType := categorizeStatus(wrapped.status)
			m.APIErrorsTotal.WithLabelValues(errorType).Inc()
		}
	})
}

// HTTPMiddlewareWithCollector creates a middleware that uses the collector for persistence
func HTTPMiddlewareWithCollector(c *Collector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			wrapped := wrapResponseWriter(w)

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(wrapped.status)

			// Normalize path to avoid high cardinality
			path := normalizePath(r)

			c.TrackAPIRequest(r.Method, path, status)
			c.metrics.APIRequestDurationSeconds.WithLabelValues(r.Method, path).Observe(duration)

			// Track errors
			if wrapped.status >= 400 {
				errorType := categorizeStatus(wrapped.status)
				c.TrackAPIError(errorType)
			}
		})
	}
}

// normalizePath extracts route pattern from chi router to avoid high cardinality
func normalizePath(r *http.Request) string {
	// Try to get the route pattern from chi
	rctx := chi.RouteContext(r.Context())
	if rctx != nil && rctx.RoutePattern() != "" {
		return rctx.RoutePattern()
	}

	// Fallback: normalize the path manually
	path := r.URL.Path

	// Replace UUIDs with placeholder
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if isUUID(part) {
			parts[i] = "{id}"
		}
	}

	return strings.Join(parts, "/")
}

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	// Simple check for UUID format: 8-4-4-4-12
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// categorizeStatus categorizes HTTP status codes into error types
func categorizeStatus(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status == 429:
		return "rate_limited"
	case status == 401 || status == 403:
		return "auth_error"
	case status == 404:
		return "not_found"
	case status == 400:
		return "bad_request"
	case status >= 400:
		return "client_error"
	default:
		return "unknown"
	}
}
