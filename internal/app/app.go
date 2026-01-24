package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/foxzi/sendry/internal/api"
	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/dns"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/smtp"
)

// App is the main application
type App struct {
	config        *config.Config
	queue         queue.Queue
	smtpServer    *smtp.Server
	smtpSubmission *smtp.Server
	apiServer     *api.Server
	processor     *queue.Processor
	logger        *slog.Logger
}

// New creates a new application
func New(cfg *config.Config) (*App, error) {
	// Setup logger
	logger := setupLogger(cfg.Logging)

	// Create storage
	storage, err := queue.NewBoltStorage(cfg.Storage.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create DNS resolver
	resolver := dns.NewResolver(5 * time.Minute)

	// Create SMTP client
	smtpClient := smtp.NewClient(resolver, cfg.Server.Hostname, 30*time.Second, logger.With("component", "smtp_client"))

	// Create queue processor
	processor := queue.NewProcessor(
		storage,
		smtpClient,
		queue.ProcessorConfig{
			Workers:         cfg.Queue.Workers,
			RetryInterval:   cfg.Queue.RetryInterval,
			MaxRetries:      cfg.Queue.MaxRetries,
			ProcessInterval: cfg.Queue.ProcessInterval,
		},
		smtp.IsTemporaryError,
		logger.With("component", "processor"),
	)

	// Create SMTP server (port 25)
	smtpServer := smtp.NewServer(&cfg.SMTP, storage, logger.With("component", "smtp_server"))

	// Create SMTP submission server (port 587)
	submissionCfg := cfg.SMTP
	submissionCfg.ListenAddr = cfg.SMTP.SubmissionAddr
	smtpSubmission := smtp.NewServer(&submissionCfg, storage, logger.With("component", "smtp_submission"))

	// Create API server
	apiServer := api.NewServer(storage, &cfg.API, logger.With("component", "api"))

	return &App{
		config:        cfg,
		queue:         storage,
		smtpServer:    smtpServer,
		smtpSubmission: smtpSubmission,
		apiServer:     apiServer,
		processor:     processor,
		logger:        logger,
	}, nil
}

// Run starts all components and waits for shutdown
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("starting sendry",
		"hostname", a.config.Server.Hostname,
		"smtp_addr", a.config.SMTP.ListenAddr,
		"submission_addr", a.config.SMTP.SubmissionAddr,
		"api_addr", a.config.API.ListenAddr,
	)

	// Create context that listens for signals
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start queue processor
	a.processor.Start(ctx)

	// Channel to collect errors
	errCh := make(chan error, 3)

	// Start SMTP server
	go func() {
		if err := a.smtpServer.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("smtp server: %w", err)
		}
	}()

	// Start SMTP submission server
	go func() {
		if err := a.smtpSubmission.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("smtp submission: %w", err)
		}
	}()

	// Start API server
	go func() {
		if err := a.apiServer.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("api server: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	case err := <-errCh:
		a.logger.Error("server error", "error", err)
		cancel()
	}

	// Graceful shutdown
	return a.Shutdown(context.Background())
}

// Shutdown gracefully shuts down all components
func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("shutting down")

	// Create timeout context
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Stop processor first (stop accepting new work)
	a.processor.Stop()

	// Shutdown servers
	if err := a.smtpServer.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("smtp server shutdown error", "error", err)
	}

	if err := a.smtpSubmission.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("smtp submission shutdown error", "error", err)
	}

	if err := a.apiServer.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("api server shutdown error", "error", err)
	}

	// Close storage
	if err := a.queue.Close(); err != nil {
		a.logger.Error("storage close error", "error", err)
	}

	a.logger.Info("shutdown complete")
	return nil
}

// setupLogger creates a logger based on configuration
func setupLogger(cfg config.LoggingConfig) *slog.Logger {
	var handler slog.Handler

	level := slog.LevelInfo
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
