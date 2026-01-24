package queue

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Sender is an interface for sending messages
type Sender interface {
	Send(ctx context.Context, msg *Message) error
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

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// ProcessorConfig contains processor configuration
type ProcessorConfig struct {
	Workers         int
	RetryInterval   time.Duration
	MaxRetries      int
	ProcessInterval time.Duration
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
		stopCh:          make(chan struct{}),
	}
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

		logger.Info("message deferred",
			"retry_count", msg.RetryCount,
			"next_retry_at", msg.NextRetryAt,
			"backoff", backoff,
		)
	} else {
		// Permanent failure or max retries exceeded
		msg.Status = StatusFailed
		logger.Error("message failed permanently",
			"retry_count", msg.RetryCount,
			"max_retries", p.maxRetries,
		)
	}

	if err := p.queue.Update(ctx, msg); err != nil {
		logger.Error("failed to update message status", "error", err)
	}
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
