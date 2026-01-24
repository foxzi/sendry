package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/foxzi/sendry/internal/api"
	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/dkim"
	"github.com/foxzi/sendry/internal/dns"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/smtp"
	sendryTLS "github.com/foxzi/sendry/internal/tls"
)

// App is the main application
type App struct {
	config         *config.Config
	queue          queue.Queue
	smtpServer     *smtp.Server
	smtpSubmission *smtp.Server
	smtpsServer    *smtp.Server
	apiServer      *api.Server
	processor      *queue.Processor
	logger         *slog.Logger
	tlsConfig      *tls.Config
	acmeManager    *sendryTLS.ACMEManager
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

	// Setup DKIM signer if configured
	if cfg.DKIM.Enabled {
		dkimSigner, err := dkim.NewSignerFromFile(cfg.DKIM.KeyFile, cfg.DKIM.Domain, cfg.DKIM.Selector)
		if err != nil {
			return nil, fmt.Errorf("failed to create DKIM signer: %w", err)
		}
		smtpClient.SetDKIMSigner(dkimSigner)
		logger.Info("DKIM signing enabled", "domain", cfg.DKIM.Domain, "selector", cfg.DKIM.Selector)
	}

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

	// Setup TLS configuration
	var tlsConfig *tls.Config
	var acmeManager *sendryTLS.ACMEManager

	if cfg.SMTP.TLS.ACME.Enabled {
		acmeManager = sendryTLS.NewACMEManager(
			cfg.SMTP.TLS.ACME.Email,
			cfg.SMTP.TLS.ACME.Domains,
			cfg.SMTP.TLS.ACME.CacheDir,
		)
		tlsConfig = acmeManager.TLSConfig()
		logger.Info("ACME (Let's Encrypt) enabled", "domains", cfg.SMTP.TLS.ACME.Domains)
	} else if cfg.SMTP.TLS.CertFile != "" && cfg.SMTP.TLS.KeyFile != "" {
		tlsConfig, err = sendryTLS.LoadCertificate(cfg.SMTP.TLS.CertFile, cfg.SMTP.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
		}
		logger.Info("TLS enabled with manual certificates")
	}

	// Create SMTP server (port 25) with STARTTLS
	smtpServer := smtp.NewServerWithOptions(smtp.ServerOptions{
		Config:    &cfg.SMTP,
		Queue:     storage,
		Logger:    logger.With("component", "smtp_server"),
		TLSConfig: tlsConfig,
		Implicit:  false,
		Addr:      cfg.SMTP.ListenAddr,
	})

	// Create SMTP submission server (port 587) with STARTTLS
	submissionCfg := cfg.SMTP
	smtpSubmission := smtp.NewServerWithOptions(smtp.ServerOptions{
		Config:    &submissionCfg,
		Queue:     storage,
		Logger:    logger.With("component", "smtp_submission"),
		TLSConfig: tlsConfig,
		Implicit:  false,
		Addr:      cfg.SMTP.SubmissionAddr,
	})

	// Create SMTPS server (port 465) with implicit TLS
	var smtpsServer *smtp.Server
	if tlsConfig != nil {
		smtpsServer = smtp.NewServerWithOptions(smtp.ServerOptions{
			Config:    &cfg.SMTP,
			Queue:     storage,
			Logger:    logger.With("component", "smtps_server"),
			TLSConfig: tlsConfig,
			Implicit:  true,
			Addr:      cfg.SMTP.SMTPSAddr,
		})
	}

	// Create API server
	apiServer := api.NewServer(storage, &cfg.API, logger.With("component", "api"))

	return &App{
		config:         cfg,
		queue:          storage,
		smtpServer:     smtpServer,
		smtpSubmission: smtpSubmission,
		smtpsServer:    smtpsServer,
		apiServer:      apiServer,
		processor:      processor,
		logger:         logger,
		tlsConfig:      tlsConfig,
		acmeManager:    acmeManager,
	}, nil
}

// Run starts all components and waits for shutdown
func (a *App) Run(ctx context.Context) error {
	logAttrs := []any{
		"hostname", a.config.Server.Hostname,
		"smtp_addr", a.config.SMTP.ListenAddr,
		"submission_addr", a.config.SMTP.SubmissionAddr,
		"api_addr", a.config.API.ListenAddr,
	}
	if a.smtpsServer != nil {
		logAttrs = append(logAttrs, "smtps_addr", a.config.SMTP.SMTPSAddr)
	}
	a.logger.Info("starting sendry", logAttrs...)

	// Create context that listens for signals
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start queue processor
	a.processor.Start(ctx)

	// Channel to collect errors
	errCh := make(chan error, 4)

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

	// Start SMTPS server if TLS is configured
	if a.smtpsServer != nil {
		go func() {
			if err := a.smtpsServer.ListenAndServe(); err != nil {
				errCh <- fmt.Errorf("smtps server: %w", err)
			}
		}()
	}

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

	if a.smtpsServer != nil {
		if err := a.smtpsServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("smtps server shutdown error", "error", err)
		}
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
