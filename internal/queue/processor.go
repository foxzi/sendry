package queue

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/foxzi/sendry/internal/email"
	"github.com/foxzi/sendry/internal/metrics"
)

// Sender is an interface for sending messages
type Sender interface {
	Send(ctx context.Context, msg *Message) error
}

// BounceGenerator generates bounce messages
type BounceGenerator interface {
	GenerateDSN(msg *Message, errorMsg string, permanent bool) ([]byte, error)
}

// DLQStorage is an interface for dead letter queue operations
type DLQStorage interface {
	MoveToDLQ(ctx context.Context, msg *Message) error
}

// IsTemporaryError checks if the error is temporary
type ErrorChecker func(err error) bool

// Processor processes the message queue
type Processor struct {
	queue           Queue
	sender          Sender
	workers         int
	retryInterval   time.Duration
	maxRetries      int
	processInterval time.Duration
	isTemporary     ErrorChecker
	logger          *slog.Logger
	bounceGenerator BounceGenerator
	bounceEnabled   bool
	dlqEnabled      bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// ProcessorConfig contains processor configuration
type ProcessorConfig struct {
	Workers         int
	RetryInterval   time.Duration
	MaxRetries      int
	ProcessInterval time.Duration
	DLQEnabled      bool // Enable dead letter queue (if false, failed messages are deleted)
}

// NewProcessor creates a new queue processor
func NewProcessor(q Queue, sender Sender, cfg ProcessorConfig, isTemp ErrorChecker, logger *slog.Logger) *Processor {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 5 * time.Minute
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.ProcessInterval <= 0 {
		cfg.ProcessInterval = 10 * time.Second
	}
	if isTemp == nil {
		isTemp = func(err error) bool { return true }
	}

	return &Processor{
		queue:           q,
		sender:          sender,
		workers:         cfg.Workers,
		retryInterval:   cfg.RetryInterval,
		maxRetries:      cfg.MaxRetries,
		processInterval: cfg.ProcessInterval,
		isTemporary:     isTemp,
		logger:          logger,
		dlqEnabled:      cfg.DLQEnabled,
		stopCh:          make(chan struct{}),
	}
}

// SetBounceGenerator sets the bounce generator for sending NDRs
func (p *Processor) SetBounceGenerator(bg BounceGenerator) {
	p.bounceGenerator = bg
	p.bounceEnabled = true
}

// Start starts the processor workers
func (p *Processor) Start(ctx context.Context) {
	p.logger.Info("starting queue processor", "workers", p.workers)

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

// Stop stops the processor gracefully
func (p *Processor) Stop() {
	p.logger.Info("stopping queue processor")
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("queue processor stopped")
}

// worker is the main processing loop
func (p *Processor) worker(ctx context.Context, id int) {
	defer p.wg.Done()

	logger := p.logger.With("worker_id", id)
	logger.Debug("worker started")

	ticker := time.NewTicker(p.processInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("worker stopped by context")
			return
		case <-p.stopCh:
			logger.Debug("worker stopped by signal")
			return
		case <-ticker.C:
			p.processOne(ctx, logger)
		}
	}
}

// processOne processes a single message from the queue
func (p *Processor) processOne(ctx context.Context, logger *slog.Logger) {
	msg, err := p.queue.Dequeue(ctx)
	if err != nil {
		logger.Error("failed to dequeue message", "error", err)
		return
	}

	if msg == nil {
		return // Queue is empty
	}

	logger = logger.With("message_id", msg.ID)
	logger.Debug("processing message")

	// Try to send
	sendCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	err = p.sender.Send(sendCtx, msg)
	cancel()

	if err == nil {
		// Success
		msg.Status = StatusDelivered
		msg.UpdatedAt = time.Now()

		if err := p.queue.Update(ctx, msg); err != nil {
			logger.Error("failed to update message status", "error", err)
		}

		// Track metrics
		metrics.IncMessagesSent(email.ExtractDomain(msg.From))

		logger.Info("message delivered", "from", msg.From, "to", msg.To)
		return
	}

	// Handle error
	logger.Warn("delivery failed", "error", err, "retry_count", msg.RetryCount)

	msg.RetryCount++
	msg.LastError = err.Error()
	msg.UpdatedAt = time.Now()

	if p.isTemporary(err) && msg.RetryCount < p.maxRetries {
		// Schedule retry with exponential backoff
		backoff := p.calculateBackoff(msg.RetryCount)
		msg.Status = StatusDeferred
		msg.NextRetryAt = time.Now().Add(backoff)

		// Track metrics
		metrics.IncMessagesDeferred(email.ExtractDomain(msg.From))

		logger.Info("message deferred",
			"retry_count", msg.RetryCount,
			"next_retry_at", msg.NextRetryAt,
			"backoff", backoff,
		)
	} else {
		// Permanent failure or max retries exceeded
		msg.Status = StatusFailed

		// Track metrics
		metrics.IncMessagesFailed(email.ExtractDomain(msg.From), classifyError(err))

		logger.Error("message failed permanently",
			"retry_count", msg.RetryCount,
			"max_retries", p.maxRetries,
		)

		// Generate and send bounce message
		p.sendBounce(ctx, msg, err.Error(), logger)

		// Move to dead letter queue or delete
		if p.dlqEnabled {
			if dlq, ok := p.queue.(DLQStorage); ok {
				if err := dlq.MoveToDLQ(ctx, msg); err != nil {
					logger.Error("failed to move message to DLQ", "error", err)
				} else {
					logger.Info("message moved to DLQ", "id", msg.ID)
					return // Already saved to DLQ, no need to Update
				}
			}
		} else {
			// DLQ disabled - delete the message
			if err := p.queue.Delete(ctx, msg.ID); err != nil {
				logger.Error("failed to delete failed message", "error", err)
			} else {
				logger.Info("failed message deleted (DLQ disabled)", "id", msg.ID)
				return
			}
		}
	}

	if err := p.queue.Update(ctx, msg); err != nil {
		logger.Error("failed to update message status", "error", err)
	}
}

// sendBounce generates and queues a bounce (NDR) message
func (p *Processor) sendBounce(ctx context.Context, msg *Message, errorMsg string, logger *slog.Logger) {
	if !p.bounceEnabled || p.bounceGenerator == nil {
		return
	}

	// Don't send bounce for bounce messages (prevent loops)
	if isBounceMessage(msg) {
		logger.Debug("skipping bounce for bounce message")
		return
	}

	// Generate DSN
	bounceData, err := p.bounceGenerator.GenerateDSN(msg, errorMsg, true)
	if err != nil {
		logger.Error("failed to generate bounce", "error", err)
		return
	}

	// Create bounce message with unique ID
	bounceMsg := &Message{
		ID:        uuid.New().String(),
		From:      "", // Will be set by sender (postmaster)
		To:        []string{msg.From},
		Data:      bounceData,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Enqueue bounce message
	if err := p.queue.Enqueue(ctx, bounceMsg); err != nil {
		logger.Error("failed to enqueue bounce message", "error", err)
		return
	}

	// Track bounce metrics
	metrics.IncMessagesBounced(email.ExtractDomain(msg.From))

	logger.Info("bounce message queued", "bounce_id", bounceMsg.ID, "original_sender", msg.From)
}

// isBounceMessage checks if message is a bounce (to prevent loops)
func isBounceMessage(msg *Message) bool {
	// Check if message ID contains -bounce suffix
	if strings.HasSuffix(msg.ID, "-bounce") {
		return true
	}

	// Check for empty From (null sender)
	if msg.From == "" || msg.From == "<>" {
		return true
	}

	// Check for MAILER-DAEMON or postmaster
	from := strings.ToLower(msg.From)
	if strings.Contains(from, "mailer-daemon") || strings.Contains(from, "postmaster") {
		return true
	}

	return false
}

// calculateBackoff calculates exponential backoff duration
func (p *Processor) calculateBackoff(retryCount int) time.Duration {
	// Exponential backoff: retry_interval * 2^(retry_count-1)
	// But cap it at a reasonable maximum (1 hour)
	multiplier := 1 << (retryCount - 1) // 2^(n-1)
	if multiplier > 12 {
		multiplier = 12 // Cap at ~12x retry_interval
	}

	backoff := time.Duration(multiplier) * p.retryInterval

	// Max 1 hour
	maxBackoff := time.Hour
	if backoff > maxBackoff {
		return maxBackoff
	}

	return backoff
}

// smtpCodePattern matches SMTP response codes at word boundaries
var smtpCodePattern = regexp.MustCompile(`\b(4\d{2}|5\d{2})\b`)

// classifyError classifies delivery error into category for metrics
func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}

	errStr := strings.ToLower(err.Error())

	// First check for SMTP codes
	matches := smtpCodePattern.FindStringSubmatch(err.Error())
	if len(matches) > 1 {
		code := matches[1]
		// Check for relay denial first (more specific)
		if (code == "550" || code == "551") && strings.Contains(errStr, "relay") {
			return "relay_denied"
		}
		switch code {
		case "550", "551", "552", "553":
			return "recipient_rejected"
		case "554":
			return "spam_rejected"
		case "530", "535":
			return "auth_failed"
		}
	}

	// Fall back to string matching for non-SMTP errors
	switch {
	case strings.Contains(errStr, "connection refused"):
		return "connection_refused"
	case strings.Contains(errStr, "connection timeout") || strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "dns"):
		return "dns_error"
	case strings.Contains(errStr, "user unknown"):
		return "recipient_rejected"
	case strings.Contains(errStr, "spam"):
		return "spam_rejected"
	case strings.Contains(errStr, "relay"):
		return "relay_denied"
	case strings.Contains(errStr, "authentication"):
		return "auth_failed"
	case strings.Contains(errStr, "tls") || strings.Contains(errStr, "certificate"):
		return "tls_error"
	default:
		return "other"
	}
}
