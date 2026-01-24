package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/foxzi/sendry/internal/api"
	"github.com/foxzi/sendry/internal/bounce"
	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/dns"
	"github.com/foxzi/sendry/internal/domain"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/ratelimit"
	"github.com/foxzi/sendry/internal/sandbox"
	"github.com/foxzi/sendry/internal/smtp"
	sendryTLS "github.com/foxzi/sendry/internal/tls"
)

// App is the main application
type App struct {
	config          *config.Config
	queue           queue.Queue
	smtpServer      *smtp.Server
	smtpSubmission  *smtp.Server
	smtpsServer     *smtp.Server
	apiServer       *api.Server
	processor       *queue.Processor
	logger          *slog.Logger
	tlsConfig       *tls.Config
	acmeManager     *sendryTLS.ACMEManager
	acmeServer      *http.Server
	domainManager   *domain.Manager
	rateLimiter     *ratelimit.Limiter
	sandboxStorage  *sandbox.Storage
	sandboxSender   *sandbox.Sender
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

	// Create rate limiter if enabled
	var rateLimiter *ratelimit.Limiter
	if cfg.RateLimit.Enabled {
		rlConfig := &ratelimit.Config{}
		if cfg.RateLimit.Global != nil {
			rlConfig.Global = &ratelimit.LimitConfig{
				MessagesPerHour: cfg.RateLimit.Global.MessagesPerHour,
				MessagesPerDay:  cfg.RateLimit.Global.MessagesPerDay,
			}
		}
		if cfg.RateLimit.DefaultDomain != nil {
			rlConfig.DefaultDomain = &ratelimit.LimitConfig{
				MessagesPerHour: cfg.RateLimit.DefaultDomain.MessagesPerHour,
				MessagesPerDay:  cfg.RateLimit.DefaultDomain.MessagesPerDay,
			}
		}
		if cfg.RateLimit.DefaultSender != nil {
			rlConfig.DefaultSender = &ratelimit.LimitConfig{
				MessagesPerHour: cfg.RateLimit.DefaultSender.MessagesPerHour,
				MessagesPerDay:  cfg.RateLimit.DefaultSender.MessagesPerDay,
			}
		}
		if cfg.RateLimit.DefaultIP != nil {
			rlConfig.DefaultIP = &ratelimit.LimitConfig{
				MessagesPerHour: cfg.RateLimit.DefaultIP.MessagesPerHour,
				MessagesPerDay:  cfg.RateLimit.DefaultIP.MessagesPerDay,
			}
		}
		if cfg.RateLimit.DefaultAPIKey != nil {
			rlConfig.DefaultAPIKey = &ratelimit.LimitConfig{
				MessagesPerHour: cfg.RateLimit.DefaultAPIKey.MessagesPerHour,
				MessagesPerDay:  cfg.RateLimit.DefaultAPIKey.MessagesPerDay,
			}
		}

		rateLimiter, err = ratelimit.NewLimiter(storage.DB(), rlConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create rate limiter: %w", err)
		}
		logger.Info("rate limiting enabled")
	}

	// Create DNS resolver
	resolver := dns.NewResolver(5 * time.Minute)

	// Create Domain Manager for multi-domain support
	domainMgr, err := domain.NewManager(cfg, logger.With("component", "domain_manager"))
	if err != nil {
		return nil, fmt.Errorf("failed to create domain manager: %w", err)
	}

	// Create SMTP client
	smtpClient := smtp.NewClient(resolver, cfg.Server.Hostname, 30*time.Second, logger.With("component", "smtp_client"))

	// Setup DKIM provider for multi-domain signing
	if domainMgr.HasDKIM() {
		smtpClient.SetDKIMProvider(domainMgr)
		logger.Info("DKIM signing enabled", "domains", domainMgr.ListDomains())
	}

	// Create sandbox storage
	sandboxStorage, err := sandbox.NewStorage(storage.DB())
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox storage: %w", err)
	}
	logger.Info("sandbox storage enabled")

	// Create sandbox sender that wraps the real sender
	sandboxSender := sandbox.NewSender(
		smtpClient,
		domainMgr,
		sandboxStorage,
		logger.With("component", "sandbox_sender"),
	)

	// Create queue processor with sandbox sender
	processor := queue.NewProcessor(
		storage,
		sandboxSender,
		queue.ProcessorConfig{
			Workers:         cfg.Queue.Workers,
			RetryInterval:   cfg.Queue.RetryInterval,
			MaxRetries:      cfg.Queue.MaxRetries,
			ProcessInterval: cfg.Queue.ProcessInterval,
		},
		smtp.IsTemporaryError,
		logger.With("component", "processor"),
	)

	// Setup bounce generator for NDR messages
	bounceGen := bounce.NewGenerator(cfg.Server.Hostname)
	processor.SetBounceGenerator(bounceGen)
	logger.Info("bounce handling enabled", "hostname", cfg.Server.Hostname)

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
		Config:      &cfg.SMTP,
		Queue:       storage,
		Logger:      logger.With("component", "smtp_server"),
		TLSConfig:   tlsConfig,
		Implicit:    false,
		Addr:        cfg.SMTP.ListenAddr,
		RateLimiter: rateLimiter,
	})

	// Create SMTP submission server (port 587) with STARTTLS
	submissionCfg := cfg.SMTP
	smtpSubmission := smtp.NewServerWithOptions(smtp.ServerOptions{
		Config:      &submissionCfg,
		Queue:       storage,
		Logger:      logger.With("component", "smtp_submission"),
		TLSConfig:   tlsConfig,
		Implicit:    false,
		Addr:        cfg.SMTP.SubmissionAddr,
		RateLimiter: rateLimiter,
	})

	// Create SMTPS server (port 465) with implicit TLS
	var smtpsServer *smtp.Server
	if tlsConfig != nil {
		smtpsServer = smtp.NewServerWithOptions(smtp.ServerOptions{
			Config:      &cfg.SMTP,
			Queue:       storage,
			Logger:      logger.With("component", "smtps_server"),
			TLSConfig:   tlsConfig,
			Implicit:    true,
			Addr:        cfg.SMTP.SMTPSAddr,
			RateLimiter: rateLimiter,
		})
	}

	// Create API server with full options
	apiServer := api.NewServerWithOptions(api.ServerOptions{
		Queue:          storage,
		Config:         &cfg.API,
		FullConfig:     cfg,
		Logger:         logger.With("component", "api"),
		DomainManager:  domainMgr,
		RateLimiter:    rateLimiter,
		SandboxStorage: sandboxStorage,
	})

	return &App{
		config:          cfg,
		queue:           storage,
		smtpServer:      smtpServer,
		smtpSubmission:  smtpSubmission,
		smtpsServer:     smtpsServer,
		apiServer:       apiServer,
		processor:       processor,
		logger:          logger,
		tlsConfig:       tlsConfig,
		sandboxStorage:  sandboxStorage,
		sandboxSender:   sandboxSender,
		acmeManager:    acmeManager,
		domainManager:  domainMgr,
		rateLimiter:    rateLimiter,
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

	// Start ACME HTTP challenge server on port 80 if ACME is enabled
	if a.acmeManager != nil {
		a.acmeServer = &http.Server{
			Addr:    ":80",
			Handler: a.acmeManager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Redirect all non-ACME requests to HTTPS
				target := "https://" + r.Host + r.URL.Path
				if r.URL.RawQuery != "" {
					target += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			})),
		}
		go func() {
			a.logger.Info("starting ACME HTTP challenge server", "addr", ":80")
			if err := a.acmeServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				a.logger.Warn("ACME HTTP server error", "error", err)
			}
		}()
	}

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

	// Shutdown ACME server if running
	if a.acmeServer != nil {
		if err := a.acmeServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("acme server shutdown error", "error", err)
		}
	}

	// Stop rate limiter (persists counters)
	if a.rateLimiter != nil {
		if err := a.rateLimiter.Stop(); err != nil {
			a.logger.Error("rate limiter stop error", "error", err)
		}
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
