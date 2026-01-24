package smtp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/metrics"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/ratelimit"
)

// authFailure tracks failed auth attempts
type authFailure struct {
	count     int
	lastFail  time.Time
	blockedAt time.Time
}

// Backend implements smtp.Backend for go-smtp
type Backend struct {
	queue       queue.Queue
	auth        *config.AuthConfig
	logger      *slog.Logger
	rateLimiter *ratelimit.Limiter
	serverType  string

	// Auth brute force protection
	authFailures map[string]*authFailure
	authMu       sync.RWMutex
}

// NewBackend creates a new SMTP backend
func NewBackend(q queue.Queue, auth *config.AuthConfig, logger *slog.Logger) *Backend {
	return &Backend{
		queue:        q,
		auth:         auth,
		logger:       logger,
		serverType:   "smtp",
		authFailures: make(map[string]*authFailure),
	}
}

// SetServerType sets the server type for metrics
func (b *Backend) SetServerType(serverType string) {
	b.serverType = serverType
}

// SetRateLimiter sets the rate limiter for the backend
func (b *Backend) SetRateLimiter(rl *ratelimit.Limiter) {
	b.rateLimiter = rl
}

// CheckRateLimit checks if the request is within rate limits
func (b *Backend) CheckRateLimit(ctx context.Context, req *ratelimit.Request) error {
	if b.rateLimiter == nil {
		return nil
	}

	result, err := b.rateLimiter.Allow(ctx, req)
	if err != nil {
		b.logger.Error("rate limit check error", "error", err)
		return nil // Don't block on errors
	}

	if !result.Allowed {
		b.logger.Warn("rate limit exceeded",
			"level", result.DeniedBy,
			"key", result.DeniedKey,
			"retry_after", result.RetryAfter,
		)

		// Track rate limit metrics
		metrics.IncRateLimitExceeded(string(result.DeniedBy))

		return &smtp.SMTPError{
			Code:         452,
			EnhancedCode: smtp.EnhancedCode{4, 7, 1},
			Message:      "Rate limit exceeded, try again later",
		}
	}

	return nil
}

// Auth brute force protection constants
const (
	maxAuthFailures   = 5               // Max failures before blocking
	authBlockDuration = 15 * time.Minute // How long to block
	authFailureWindow = 5 * time.Minute  // Window for counting failures
)

// CheckAuthBlocked checks if IP is blocked due to too many auth failures
func (b *Backend) CheckAuthBlocked(ip string) bool {
	b.authMu.RLock()
	defer b.authMu.RUnlock()

	if f, ok := b.authFailures[ip]; ok {
		// Check if still blocked
		if !f.blockedAt.IsZero() && time.Since(f.blockedAt) < authBlockDuration {
			return true
		}
	}
	return false
}

// RecordAuthFailure records a failed auth attempt
func (b *Backend) RecordAuthFailure(ip string) bool {
	b.authMu.Lock()
	defer b.authMu.Unlock()

	now := time.Now()
	f, ok := b.authFailures[ip]
	if !ok {
		f = &authFailure{}
		b.authFailures[ip] = f
	}

	// Reset counter if outside window
	if time.Since(f.lastFail) > authFailureWindow {
		f.count = 0
		f.blockedAt = time.Time{}
	}

	f.count++
	f.lastFail = now

	// Block if too many failures
	if f.count >= maxAuthFailures {
		f.blockedAt = now
		b.logger.Warn("IP blocked due to auth failures", "ip", ip, "failures", f.count)
		return true // Now blocked
	}

	return false
}

// ClearAuthFailure clears auth failure record on successful auth
func (b *Backend) ClearAuthFailure(ip string) {
	b.authMu.Lock()
	defer b.authMu.Unlock()
	delete(b.authFailures, ip)
}

// NewSession is called when a new SMTP connection is established
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return NewSession(b, c), nil
}
