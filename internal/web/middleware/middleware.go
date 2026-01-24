package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
)

// Logger middleware logs HTTP requests
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"duration", time.Since(start),
				"ip", r.RemoteAddr,
			)
		})
	}
}

// Recovery middleware recovers from panics
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"error", err,
						"stack", string(debug.Stack()),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Auth middleware checks authentication
func Auth(cfg *config.Config, database *db.DB, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: implement proper session checking
			// For now, allow all requests (development mode)

			// Get session cookie
			cookie, err := r.Cookie("session")
			if err != nil {
				// No session, redirect to login
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}

			// Validate session
			var userID string
			err = database.QueryRow(
				"SELECT user_id FROM sessions WHERE id = ? AND expires_at > datetime('now')",
				cookie.Value,
			).Scan(&userID)

			if err != nil {
				// Invalid session, redirect to login
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}

			// Session valid, continue
			next.ServeHTTP(w, r)
		})
	}
}

// MethodOverride middleware allows overriding HTTP method via _method form field
func MethodOverride(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			method := r.FormValue("_method")
			if method != "" {
				r.Method = method
			}
		}
		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
