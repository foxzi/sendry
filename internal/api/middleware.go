package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration", time.Since(start),
			"bytes", ww.BytesWritten(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// authMiddleware checks API key authentication
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.config.APIKey == "" {
			// No API key configured, allow all
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Also check X-API-Key header
			auth = r.Header.Get("X-API-Key")
		}

		// Parse Bearer token
		if strings.HasPrefix(auth, "Bearer ") {
			auth = strings.TrimPrefix(auth, "Bearer ")
		}

		if auth != s.config.APIKey {
			s.logger.Warn("unauthorized API request",
				"remote_addr", r.RemoteAddr,
				"path", r.URL.Path,
			)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
