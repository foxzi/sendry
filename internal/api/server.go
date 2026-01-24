package api

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/domain"
	"github.com/foxzi/sendry/internal/metrics"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/ratelimit"
	"github.com/foxzi/sendry/internal/sandbox"
	"github.com/foxzi/sendry/internal/template"
)

// Server is the HTTP API server
type Server struct {
	router           *chi.Mux
	httpServer       *http.Server
	queue            queue.Queue
	boltStorage      *queue.BoltStorage // typed reference for DLQ operations
	config           *config.APIConfig
	fullConfig       *config.Config
	logger           *slog.Logger
	startTime        time.Time
	domainManager    *domain.Manager
	rateLimiter      *ratelimit.Limiter
	managementServer *ManagementServer
	sandboxServer    *SandboxServer
	sandboxStorage   *sandbox.Storage
	tlsConfig        *tls.Config
	templateServer   *TemplateServer
}

// ServerOptions contains options for creating an API server
type ServerOptions struct {
	Queue           queue.Queue
	Config          *config.APIConfig
	FullConfig      *config.Config
	Logger          *slog.Logger
	DomainManager   *domain.Manager
	RateLimiter     *ratelimit.Limiter
	SandboxStorage  *sandbox.Storage
	TemplateStorage *template.Storage
	DKIMKeysDir     string
	TLSCertsDir     string
	TLSConfig       *tls.Config
}

// NewServer creates a new API server
func NewServer(q queue.Queue, cfg *config.APIConfig, logger *slog.Logger) *Server {
	return NewServerWithOptions(ServerOptions{
		Queue:  q,
		Config: cfg,
		Logger: logger,
	})
}

// NewServerWithOptions creates a new API server with full options
func NewServerWithOptions(opts ServerOptions) *Server {
	s := &Server{
		router:         chi.NewRouter(),
		queue:          opts.Queue,
		config:         opts.Config,
		fullConfig:     opts.FullConfig,
		logger:         opts.Logger,
		startTime:      time.Now(),
		domainManager:  opts.DomainManager,
		rateLimiter:    opts.RateLimiter,
		sandboxStorage: opts.SandboxStorage,
		tlsConfig:      opts.TLSConfig,
	}

	// Store typed reference for DLQ operations
	if bs, ok := opts.Queue.(*queue.BoltStorage); ok {
		s.boltStorage = bs
	}

	// Create management server if we have full config
	if opts.FullConfig != nil {
		dkimDir := opts.DKIMKeysDir
		if dkimDir == "" {
			dkimDir = "/var/lib/sendry/dkim"
		}
		tlsDir := opts.TLSCertsDir
		if tlsDir == "" {
			tlsDir = "/var/lib/sendry/certs"
		}
		s.managementServer = NewManagementServer(
			opts.DomainManager,
			opts.RateLimiter,
			opts.FullConfig,
			dkimDir,
			tlsDir,
		)
	}

	// Create sandbox server if storage is available
	if opts.SandboxStorage != nil {
		s.sandboxServer = NewSandboxServer(opts.SandboxStorage, opts.Queue)
	}

	// Create template server if storage is available
	if opts.TemplateStorage != nil {
		s.templateServer = NewTemplateServer(opts.TemplateStorage, opts.Queue)
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	// Middleware
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(metrics.HTTPMiddleware)
	s.router.Use(s.loggingMiddleware)
	s.router.Use(middleware.Recoverer)

	// Health check (no auth required)
	s.router.Get("/health", s.handleHealth)

	// API v1 routes (auth required)
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Post("/send", s.handleSend)
		r.Get("/status/{id}", s.handleStatus)
		r.Get("/queue", s.handleQueue)
		r.Delete("/queue/{id}", s.handleDeleteMessage)

		// Dead Letter Queue routes
		r.Get("/dlq", s.handleDLQ)
		r.Get("/dlq/{id}", s.handleDLQGet)
		r.Post("/dlq/{id}/retry", s.handleDLQRetry)
		r.Delete("/dlq/{id}", s.handleDLQDelete)

		// Management routes (DKIM, TLS, domains, rate limits)
		if s.managementServer != nil {
			s.managementServer.RegisterRoutes(r)
		}

		// Sandbox routes
		if s.sandboxServer != nil {
			s.sandboxServer.RegisterRoutes(r)
		}

		// Template routes
		if s.templateServer != nil {
			s.templateServer.RegisterRoutes(r)
		}
	})
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe() error {
	s.httpServer = &http.Server{
		Addr:           s.config.ListenAddr,
		Handler:        s.router,
		ReadTimeout:    s.config.ReadTimeout,
		WriteTimeout:   s.config.WriteTimeout,
		IdleTimeout:    s.config.IdleTimeout,
		MaxHeaderBytes: s.config.MaxHeaderBytes,
	}

	if s.tlsConfig != nil {
		s.httpServer.TLSConfig = s.tlsConfig
		s.logger.Info("starting HTTPS API server", "addr", s.config.ListenAddr)
		return s.httpServer.ListenAndServeTLS("", "")
	}

	s.logger.Info("starting HTTP API server", "addr", s.config.ListenAddr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP API server")
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}
