package smtp

import (
	"context"
	"log/slog"

	"github.com/emersion/go-smtp"
	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/ratelimit"
)

// Backend implements smtp.Backend for go-smtp
type Backend struct {
	queue       queue.Queue
	auth        *config.AuthConfig
	logger      *slog.Logger
	rateLimiter *ratelimit.Limiter
}

// NewBackend creates a new SMTP backend
func NewBackend(q queue.Queue, auth *config.AuthConfig, logger *slog.Logger) *Backend {
	return &Backend{
		queue:  q,
		auth:   auth,
		logger: logger,
	}
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
		return &smtp.SMTPError{
			Code:         452,
			EnhancedCode: smtp.EnhancedCode{4, 7, 1},
			Message:      "Rate limit exceeded, try again later",
		}
	}

	return nil
}

// NewSession is called when a new SMTP connection is established
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return NewSession(b, c), nil
}
