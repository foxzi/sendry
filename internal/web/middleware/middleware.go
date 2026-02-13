package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
)

// RateLimiter provides in-memory rate limiting
type RateLimiter struct {
	mu       sync.RWMutex
	counters map[string]*rateLimitCounter
}

type rateLimitCounter struct {
	minuteCount int
	hourCount   int
	minuteReset time.Time
	hourReset   time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		counters: make(map[string]*rateLimitCounter),
	}
	// Start cleanup goroutine
	go rl.cleanup()
	return rl
}

// Allow checks if a request is allowed and increments counters
func (rl *RateLimiter) Allow(keyID string, limitMinute, limitHour int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	counter, exists := rl.counters[keyID]
	if !exists {
		counter = &rateLimitCounter{
			minuteReset: now.Add(time.Minute),
			hourReset:   now.Add(time.Hour),
		}
		rl.counters[keyID] = counter
	}

	// Reset counters if window expired
	if now.After(counter.minuteReset) {
		counter.minuteCount = 0
		counter.minuteReset = now.Add(time.Minute)
	}
	if now.After(counter.hourReset) {
		counter.hourCount = 0
		counter.hourReset = now.Add(time.Hour)
	}

	// Check limits
	if limitMinute > 0 && counter.minuteCount >= limitMinute {
		return false
	}
	if limitHour > 0 && counter.hourCount >= limitHour {
		return false
	}

	// Increment counters
	counter.minuteCount++
	counter.hourCount++
	return true
}

// cleanup removes expired counters periodically
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, counter := range rl.counters {
			// Remove if both windows have expired
			if now.After(counter.minuteReset) && now.After(counter.hourReset) {
				delete(rl.counters, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Global rate limiter instance
var globalRateLimiter = NewRateLimiter()

type ctxKey string

const ctxKeyAPIKey ctxKey = "api_key"

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

// APIAuth middleware authenticates API requests using API keys
func APIAuth(apiKeys *repository.APIKeyRepository, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get API key from header
			auth := r.Header.Get("Authorization")
			if auth == "" {
				auth = r.Header.Get("X-API-Key")
			}

			// Strip Bearer prefix if present
			if strings.HasPrefix(auth, "Bearer ") {
				auth = strings.TrimPrefix(auth, "Bearer ")
			}

			if auth == "" {
				sendAPIError(w, http.StatusUnauthorized, "API key required")
				return
			}

			// Hash and lookup
			keyHash := repository.HashKey(auth)
			apiKey, err := apiKeys.GetByHash(keyHash)
			if err != nil {
				logger.Error("API key lookup failed", "error", err)
				sendAPIError(w, http.StatusInternalServerError, "Authentication failed")
				return
			}

			if apiKey == nil {
				sendAPIError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			if !apiKey.Active {
				sendAPIError(w, http.StatusUnauthorized, "API key is inactive")
				return
			}

			// Check expiration
			if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
				sendAPIError(w, http.StatusUnauthorized, "API key expired")
				return
			}

			// Check rate limits
			if apiKey.RateLimitMinute > 0 || apiKey.RateLimitHour > 0 {
				if !globalRateLimiter.Allow(apiKey.ID, apiKey.RateLimitMinute, apiKey.RateLimitHour) {
					sendAPIError(w, http.StatusTooManyRequests, "Rate limit exceeded")
					return
				}
			}

			// Update last used (async to not slow down request)
			go func() {
				if err := apiKeys.UpdateLastUsed(apiKey.ID); err != nil {
					logger.Warn("failed to update API key last used", "error", err)
				}
			}()

			// Add to context
			ctx := context.WithValue(r.Context(), ctxKeyAPIKey, apiKey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAPIKeyFromContext returns the API key from request context
func GetAPIKeyFromContext(r *http.Request) *models.APIKey {
	if key, ok := r.Context().Value(ctxKeyAPIKey).(*models.APIKey); ok {
		return key
	}
	return nil
}

// sendAPIError sends a JSON error response
func sendAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// IPFilter middleware restricts access to allowed IPs
func IPFilter(allowedIPs []string, logger *slog.Logger) func(http.Handler) http.Handler {
	// Parse allowed IPs/CIDRs
	var allowedNets []*net.IPNet
	var allowedAddrs []net.IP

	for _, ip := range allowedIPs {
		if strings.Contains(ip, "/") {
			_, ipNet, err := net.ParseCIDR(ip)
			if err != nil {
				logger.Warn("invalid CIDR in allowed_ips", "cidr", ip, "error", err)
				continue
			}
			allowedNets = append(allowedNets, ipNet)
		} else {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				logger.Warn("invalid IP in allowed_ips", "ip", ip)
				continue
			}
			allowedAddrs = append(allowedAddrs, parsed)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no IPs configured, allow all
			if len(allowedNets) == 0 && len(allowedAddrs) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			clientIP := getClientIP(r)
			ip := net.ParseIP(clientIP)
			if ip == nil {
				logger.Warn("could not parse client IP", "ip", clientIP)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Check against allowed addresses
			for _, allowed := range allowedAddrs {
				if allowed.Equal(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Check against allowed networks
			for _, ipNet := range allowedNets {
				if ipNet.Contains(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}

			logger.Warn("access denied by IP filter", "ip", clientIP, "path", r.URL.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
		})
	}
}

// getClientIP extracts client IP from request, handling proxies
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (first IP in chain)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
