package smtp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/ipfilter"
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

	// Configurable limits (from auth config)
	maxAuthFailures   int
	authBlockDuration time.Duration
	authFailureWindow time.Duration

	// Cleanup goroutine control
	cleanupStopCh chan struct{}

	// Anti-relay protection: only allow sending from configured domains
	allowedDomains map[string]bool

	// IP filtering
	ipFilter *ipfilter.Filter
}

// NewBackend creates a new SMTP backend
func NewBackend(q queue.Queue, auth *config.AuthConfig, logger *slog.Logger) *Backend {
	// Use configured values or defaults
	maxFailures := auth.MaxFailures
	if maxFailures == 0 {
		maxFailures = 5
	}
	blockDuration := auth.BlockDuration
	if blockDuration == 0 {
		blockDuration = 15 * time.Minute
	}
	failureWindow := auth.FailureWindow
	if failureWindow == 0 {
		failureWindow = 5 * time.Minute
	}

	b := &Backend{
		queue:             q,
		auth:              auth,
		logger:            logger,
		serverType:        "smtp",
		authFailures:      make(map[string]*authFailure),
		maxAuthFailures:   maxFailures,
		authBlockDuration: blockDuration,
		authFailureWindow: failureWindow,
		cleanupStopCh:     make(chan struct{}),
	}

	// Start background cleanup of expired auth failures
	go b.cleanupLoop()

	return b
}

// Stop stops the backend cleanup goroutine
func (b *Backend) Stop() {
	close(b.cleanupStopCh)
}

// cleanupLoop periodically removes expired auth failure entries to prevent memory leaks
func (b *Backend) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.cleanupStopCh:
			return
		case <-ticker.C:
			b.cleanupExpiredAuthFailures()
		}
	}
}

// cleanupExpiredAuthFailures removes auth failure entries that are no longer relevant
func (b *Backend) cleanupExpiredAuthFailures() {
	b.authMu.Lock()
	defer b.authMu.Unlock()

	now := time.Now()
	expireThreshold := b.authBlockDuration + b.authFailureWindow

	for ip, f := range b.authFailures {
		// Remove if both block expired and last failure is old enough
		blockExpired := f.blockedAt.IsZero() || now.Sub(f.blockedAt) > b.authBlockDuration
		failureExpired := now.Sub(f.lastFail) > expireThreshold

		if blockExpired && failureExpired {
			delete(b.authFailures, ip)
		}
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

// SetAllowedDomains sets the list of domains allowed for sending (anti-relay protection)
func (b *Backend) SetAllowedDomains(domains []string) {
	b.allowedDomains = make(map[string]bool, len(domains))
	for _, d := range domains {
		b.allowedDomains[d] = true
	}
	b.logger.Info("allowed domains configured", "count", len(domains), "domains", domains)
}

// IsDomainAllowed checks if the sender domain is allowed
func (b *Backend) IsDomainAllowed(domain string) bool {
	// If no allowed domains configured, allow all (backwards compatibility)
	if len(b.allowedDomains) == 0 {
		return true
	}
	return b.allowedDomains[domain]
}

// SetIPFilter sets the IP filter for connection filtering
func (b *Backend) SetIPFilter(filter *ipfilter.Filter) {
	b.ipFilter = filter
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


// CheckAuthBlocked checks if IP is blocked due to too many auth failures
func (b *Backend) CheckAuthBlocked(ip string) bool {
	b.authMu.RLock()
	defer b.authMu.RUnlock()

	if f, ok := b.authFailures[ip]; ok {
		// Check if still blocked
		if !f.blockedAt.IsZero() && time.Since(f.blockedAt) < b.authBlockDuration {
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
	if time.Since(f.lastFail) > b.authFailureWindow {
		f.count = 0
		f.blockedAt = time.Time{}
	}

	f.count++
	f.lastFail = now

	// Block if too many failures
	if f.count >= b.maxAuthFailures {
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
	// Check IP filter if configured
	if b.ipFilter != nil && b.ipFilter.Enabled() {
		if !b.ipFilter.IsAllowedAddr(c.Conn().RemoteAddr().String()) {
			b.logger.Warn("SMTP connection rejected by IP filter",
				"remote_addr", c.Conn().RemoteAddr().String(),
			)
			return nil, &smtp.SMTPError{
				Code:         554,
				EnhancedCode: smtp.EnhancedCode{5, 7, 1},
				Message:      "Connection refused",
			}
		}
	}

	return NewSession(b, c), nil
}
