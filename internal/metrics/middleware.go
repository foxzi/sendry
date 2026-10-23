package metrics

import (
	"net/http"
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

		// Normalize path to avoid high cardinality
		m.observeAPIRequest(r.Method, normalizePath(r), wrapped.status, time.Since(start).Seconds())
	})
}

// unmatchedPath is the path label of requests that matched no route
// (scanners probing /wp-login.php, /.env, ...): the raw path would add a
// series per probe, and the counters are persisted across restarts.
const unmatchedPath = "unmatched"

// normalizePath returns the chi route pattern of the request, so the path
// label has one value per route, or unmatchedPath.
func normalizePath(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
		return rctx.RoutePattern()
	}
	return unmatchedPath
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
