package queue

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CleanerConfig contains cleanup settings
type CleanerConfig struct {
	// Delivered messages retention
	DeliveredMaxAge   time.Duration
	DeliveredInterval time.Duration

	// DLQ retention
	DLQMaxAge   time.Duration
	DLQMaxCount int
	DLQInterval time.Duration
}

// Cleaner handles automatic cleanup of old messages
type Cleaner struct {
	storage *BoltStorage
	cfg     CleanerConfig
	logger  *slog.Logger
	wg      sync.WaitGroup
	done    chan struct{}
}

// NewCleaner creates a new cleaner service
func NewCleaner(storage *BoltStorage, cfg CleanerConfig, logger *slog.Logger) *Cleaner {
	return &Cleaner{
		storage: storage,
		cfg:     cfg,
		logger:  logger,
		done:    make(chan struct{}),
	}
}

// Start starts the cleanup goroutines
func (c *Cleaner) Start(ctx context.Context) {
	// Delivered messages cleanup
	if c.cfg.DeliveredMaxAge > 0 && c.cfg.DeliveredInterval > 0 {
		c.wg.Add(1)
		go c.cleanupDeliveredLoop(ctx)
	}

	// DLQ cleanup
	if (c.cfg.DLQMaxAge > 0 || c.cfg.DLQMaxCount > 0) && c.cfg.DLQInterval > 0 {
		c.wg.Add(1)
		go c.cleanupDLQLoop(ctx)
	}

	c.logger.Info("cleaner started",
		"delivered_max_age", c.cfg.DeliveredMaxAge,
		"delivered_interval", c.cfg.DeliveredInterval,
		"dlq_max_age", c.cfg.DLQMaxAge,
		"dlq_max_count", c.cfg.DLQMaxCount,
		"dlq_interval", c.cfg.DLQInterval,
	)
}

// Stop stops the cleaner and waits for goroutines to finish
func (c *Cleaner) Stop() {
	close(c.done)
	c.wg.Wait()
	c.logger.Info("cleaner stopped")
}

func (c *Cleaner) cleanupDeliveredLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.DeliveredInterval)
	defer ticker.Stop()

	// Run cleanup immediately on start
	c.runDeliveredCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			c.runDeliveredCleanup(ctx)
		}
	}
}

func (c *Cleaner) runDeliveredCleanup(ctx context.Context) {
	deleted, err := c.storage.CleanupDelivered(ctx, c.cfg.DeliveredMaxAge)
	if err != nil {
		c.logger.Error("failed to cleanup delivered messages", "error", err)
		return
	}

	if deleted > 0 {
		c.logger.Info("cleaned up delivered messages", "deleted", deleted)
	}
}

func (c *Cleaner) cleanupDLQLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.DLQInterval)
	defer ticker.Stop()

	// Run cleanup immediately on start
	c.runDLQCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			c.runDLQCleanup(ctx)
		}
	}
}

func (c *Cleaner) runDLQCleanup(ctx context.Context) {
	deleted, err := c.storage.CleanupDLQ(ctx, c.cfg.DLQMaxAge, c.cfg.DLQMaxCount)
	if err != nil {
		c.logger.Error("failed to cleanup DLQ", "error", err)
		return
	}

	if deleted > 0 {
		c.logger.Info("cleaned up DLQ messages", "deleted", deleted)
	}
}
