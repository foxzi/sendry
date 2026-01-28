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
	"github.com/foxzi/sendry/internal/headers"
	"github.com/foxzi/sendry/internal/metrics"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/ratelimit"
	"github.com/foxzi/sendry/internal/sandbox"
	"github.com/foxzi/sendry/internal/smtp"
	"github.com/foxzi/sendry/internal/template"
	sendryTLS "github.com/foxzi/sendry/internal/tls"
)

// App is the main application
type App struct {
	config           *config.Config
	queue            queue.Queue
	smtpServer       *smtp.Server
	smtpSubmission   *smtp.Server
	smtpsServer      *smtp.Server
	apiServer        *api.Server
	processor        *queue.Processor
	cleaner          *queue.Cleaner
	logger           *slog.Logger
	tlsConfig        *tls.Config
	acmeManager      *sendryTLS.ACMEManager
	acmeServer       *http.Server
	domainManager    *domain.Manager
	rateLimiter      *ratelimit.Limiter
	sandboxStorage   *sandbox.Storage
	sandboxSender    *sandbox.Sender
	metricsServer    *metrics.Server
	metricsCollector *metrics.Collector
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
		if cfg.RateLimit.DefaultRecipientDomain != nil {
			rlConfig.DefaultRecipientDomain = &ratelimit.LimitConfig{
				MessagesPerHour: cfg.RateLimit.DefaultRecipientDomain.MessagesPerHour,
				MessagesPerDay:  cfg.RateLimit.DefaultRecipientDomain.MessagesPerDay,
			}
		}
		if cfg.RateLimit.RecipientDomains != nil {
			rlConfig.RecipientDomains = make(map[string]*ratelimit.LimitConfig)
			for domain, limit := range cfg.RateLimit.RecipientDomains {
				rlConfig.RecipientDomains[domain] = &ratelimit.LimitConfig{
					MessagesPerHour: limit.MessagesPerHour,
					MessagesPerDay:  limit.MessagesPerDay,
				}
			}
		}

		rateLimiter, err = ratelimit.NewLimiter(storage.DB(), rlConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create rate limiter: %w", err)
		}
		logger.Info("rate limiting enabled")
	}

	// Initialize metrics if enabled
	var metricsInstance *metrics.Metrics
	var metricsCollector *metrics.Collector
	var metricsServer *metrics.Server

	if cfg.Metrics.Enabled {
		metricsInstance = metrics.New()
		metrics.SetGlobal(metricsInstance)

		// Create queue stats adapter for metrics
		queueStatsAdapter := &queueStatsAdapter{queue: storage}

		metricsCollector, err = metrics.NewCollector(
			storage.DB(),
			metricsInstance,
			queueStatsAdapter,
			cfg.Storage.Path,
			cfg.Metrics.FlushInterval,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create metrics collector: %w", err)
		}

		metricsServer = metrics.NewServerWithAllowedIPs(
			metricsInstance,
			cfg.Metrics.ListenAddr,
			cfg.Metrics.Path,
			cfg.Metrics.AllowedIPs,
			logger.With("component", "metrics"),
		)

		logger.Info("metrics enabled", "addr", cfg.Metrics.ListenAddr, "path", cfg.Metrics.Path)
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

	// Create template storage
	templateStorage, err := template.NewStorage(storage.DB())
	if err != nil {
		return nil, fmt.Errorf("failed to create template storage: %w", err)
	}
	logger.Info("template storage enabled")

	// Create sandbox sender that wraps the real sender
	sandboxSender := sandbox.NewSender(
		smtpClient,
		domainMgr,
		sandboxStorage,
		logger.With("component", "sandbox_sender"),
	)

	// Setup header rules processor if configured
	if cfg.HeaderRules != nil && cfg.HeaderRules.HasRules() {
		headerProcessor := headers.NewProcessor(cfg.HeaderRules)
		sandboxSender.SetHeaderProcessor(headerProcessor)
		logger.Info("header rules enabled")
	}

	// Create queue processor with sandbox sender
	processor := queue.NewProcessor(
		storage,
		sandboxSender,
		queue.ProcessorConfig{
			Workers:         cfg.Queue.Workers,
			RetryInterval:   cfg.Queue.RetryInterval,
			MaxRetries:      cfg.Queue.MaxRetries,
			ProcessInterval: cfg.Queue.ProcessInterval,
			DLQEnabled:      cfg.DLQ.Enabled,
		},
		smtp.IsTemporaryError,
		logger.With("component", "processor"),
	)

	// Create cleaner for automatic cleanup
	cleaner := queue.NewCleaner(
		storage,
		queue.CleanerConfig{
			DeliveredMaxAge:   cfg.Storage.Retention.DeliveredMaxAge,
			DeliveredInterval: cfg.Storage.Retention.CleanupInterval,
			DLQMaxAge:         cfg.DLQ.MaxAge,
			DLQMaxCount:       cfg.DLQ.MaxCount,
			DLQInterval:       cfg.DLQ.CleanupInterval,
		},
		logger.With("component", "cleaner"),
	)

	// Setup bounce generator for NDR messages
	bounceGen := bounce.NewGenerator(cfg.Server.Hostname)
	processor.SetBounceGenerator(bounceGen)
	logger.Info("bounce handling enabled", "hostname", cfg.Server.Hostname)

	// Setup rate limiter for recipient domain limiting
	if rateLimiter != nil {
		processor.SetRateLimiter(rateLimiter)
	}

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

	// Get allowed domains for anti-relay protection
	allowedDomains := cfg.GetAllDomains()

	// Create SMTP server (port 25) with STARTTLS
	smtpServer := smtp.NewServerWithOptions(smtp.ServerOptions{
		Config:         &cfg.SMTP,
		Queue:          storage,
		Logger:         logger.With("component", "smtp_server"),
		TLSConfig:      tlsConfig,
		Implicit:       false,
		Addr:           cfg.SMTP.ListenAddr,
		RateLimiter:    rateLimiter,
		ServerType:     "smtp",
		AllowedDomains: allowedDomains,
		AllowedIPs:     cfg.SMTP.AllowedIPs,
	})

	// Create SMTP submission server (port 587) with STARTTLS
	submissionCfg := cfg.SMTP
	smtpSubmission := smtp.NewServerWithOptions(smtp.ServerOptions{
		Config:         &submissionCfg,
		Queue:          storage,
		Logger:         logger.With("component", "smtp_submission"),
		TLSConfig:      tlsConfig,
		Implicit:       false,
		Addr:           cfg.SMTP.SubmissionAddr,
		RateLimiter:    rateLimiter,
		ServerType:     "submission",
		AllowedDomains: allowedDomains,
		AllowedIPs:     cfg.SMTP.AllowedIPs,
	})

	// Create SMTPS server (port 465) with implicit TLS
	var smtpsServer *smtp.Server
	if tlsConfig != nil {
		smtpsServer = smtp.NewServerWithOptions(smtp.ServerOptions{
			Config:         &cfg.SMTP,
			Queue:          storage,
			Logger:         logger.With("component", "smtps_server"),
			TLSConfig:      tlsConfig,
			Implicit:       true,
			Addr:           cfg.SMTP.SMTPSAddr,
			RateLimiter:    rateLimiter,
			ServerType:     "smtps",
			AllowedDomains: allowedDomains,
			AllowedIPs:     cfg.SMTP.AllowedIPs,
		})
	}

	// Create API server with full options
	apiServer := api.NewServerWithOptions(api.ServerOptions{
		Queue:           storage,
		Config:          &cfg.API,
		FullConfig:      cfg,
		Logger:          logger.With("component", "api"),
		DomainManager:   domainMgr,
		RateLimiter:     rateLimiter,
		SandboxStorage:  sandboxStorage,
		TemplateStorage: templateStorage,
		TLSConfig:       tlsConfig,
	})

	return &App{
		config:           cfg,
		queue:            storage,
		smtpServer:       smtpServer,
		smtpSubmission:   smtpSubmission,
		smtpsServer:      smtpsServer,
		apiServer:        apiServer,
		processor:        processor,
		cleaner:          cleaner,
		logger:           logger,
		tlsConfig:        tlsConfig,
		sandboxStorage:   sandboxStorage,
		sandboxSender:    sandboxSender,
		acmeManager:      acmeManager,
		domainManager:    domainMgr,
		rateLimiter:      rateLimiter,
		metricsServer:    metricsServer,
		metricsCollector: metricsCollector,
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

	// Start cleaner for automatic cleanup
	a.cleaner.Start(ctx)

	// Start metrics collector and server if enabled
	if a.metricsCollector != nil {
		a.metricsCollector.Start(ctx)
	}
	if a.metricsServer != nil {
		go func() {
			if err := a.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				a.logger.Error("metrics server error", "error", err)
			}
		}()
	}

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

	// Handle ACME certificates if enabled
	if a.acmeManager != nil {
		if a.config.SMTP.TLS.ACME.OnDemand {
			// On-demand mode: check cached certificates, don't start HTTP server
			a.logger.Info("ACME on-demand mode enabled, checking cached certificates")
			valid, certs := a.acmeManager.HasValidCachedCertificates()
			if !valid {
				if len(certs) == 0 {
					return fmt.Errorf("no ACME certificates found in cache; run 'sendry tls renew' to obtain certificates")
				}
				// Check if any certificate is expired
				for _, cert := range certs {
					if cert.DaysLeft < 0 {
						return fmt.Errorf("certificate for %s has expired; run 'sendry tls renew' to renew", cert.Domain)
					}
					if cert.DaysLeft < 7 {
						a.logger.Warn("certificate expiring soon, run 'sendry tls renew'",
							"domain", cert.Domain,
							"days_left", cert.DaysLeft)
					}
				}
			}
			for _, cert := range certs {
				a.logger.Info("using cached certificate",
					"domain", cert.Domain,
					"expires", cert.NotAfter.Format("2006-01-02"),
					"days_left", cert.DaysLeft)
			}
		} else {
			// Always-on mode: start HTTP server on port 80 for ACME challenges
			a.acmeServer = &http.Server{
				Addr: ":80",
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

			// Wait for HTTP server to start
			time.Sleep(100 * time.Millisecond)

			// Ensure certificates are obtained/validated at startup
			certCtx, certCancel := context.WithTimeout(ctx, 2*time.Minute)
			certs, err := a.acmeManager.EnsureCertificates(certCtx)
			certCancel()
			if err != nil {
				a.logger.Error("failed to ensure certificates", "error", err)
				// Check if we have any valid certificates to continue with
				if len(certs) == 0 {
					return fmt.Errorf("ACME certificate acquisition failed and no valid certificates available: %w", err)
				}
				a.logger.Warn("continuing with existing certificates despite renewal failure")
			}
			for _, cert := range certs {
				if cert.IsNew {
					a.logger.Info("obtained new certificate",
						"domain", cert.Domain,
						"expires", cert.NotAfter.Format("2006-01-02"),
						"days_left", cert.DaysLeft)
				} else {
					a.logger.Info("certificate valid",
						"domain", cert.Domain,
						"expires", cert.NotAfter.Format("2006-01-02"),
						"days_left", cert.DaysLeft)
				}
			}
		}
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

	// Stop cleaner
	a.cleaner.Stop()

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

	// Stop metrics collector (persists counters)
	if a.metricsCollector != nil {
		if err := a.metricsCollector.Stop(); err != nil {
			a.logger.Error("metrics collector stop error", "error", err)
		}
	}

	// Shutdown metrics server
	if a.metricsServer != nil {
		if err := a.metricsServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("metrics server shutdown error", "error", err)
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

// queueStatsAdapter adapts queue.Queue to metrics.QueueStatsProvider
type queueStatsAdapter struct {
	queue queue.Queue
}

func (a *queueStatsAdapter) Stats(ctx context.Context) (*metrics.QueueStats, error) {
	stats, err := a.queue.Stats(ctx)
	if err != nil {
		return nil, err
	}
	return &metrics.QueueStats{
		Pending:   stats.Pending,
		Sending:   stats.Sending,
		Deferred:  stats.Deferred,
		Delivered: stats.Delivered,
		Failed:    stats.Failed,
		Total:     stats.Total,
	}, nil
}
