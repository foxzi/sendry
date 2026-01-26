package smtp

import (
	"context"
	"crypto/tls"
	"log/slog"

	"github.com/emersion/go-smtp"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/ratelimit"
)

// Server wraps go-smtp server with configuration
type Server struct {
	server    *smtp.Server
	backend   *Backend
	addr      string
	tlsConfig *tls.Config
	implicit  bool // true for SMTPS (implicit TLS on port 465)
	logger    *slog.Logger
}

// ServerOptions contains options for creating SMTP server
type ServerOptions struct {
	Config         *config.SMTPConfig
	Queue          queue.Queue
	Logger         *slog.Logger
	TLSConfig      *tls.Config
	Implicit       bool     // true for SMTPS (implicit TLS)
	Addr           string
	RateLimiter    *ratelimit.Limiter
	ServerType     string   // smtp, submission, smtps - for metrics
	AllowedDomains []string // Domains allowed for sending (anti-relay protection)
}

// NewServer creates a new SMTP server
func NewServer(cfg *config.SMTPConfig, q queue.Queue, logger *slog.Logger) *Server {
	return NewServerWithOptions(ServerOptions{
		Config: cfg,
		Queue:  q,
		Logger: logger,
		Addr:   cfg.ListenAddr,
	})
}

// NewServerWithOptions creates a new SMTP server with custom options
func NewServerWithOptions(opts ServerOptions) *Server {
	backend := NewBackend(opts.Queue, &opts.Config.Auth, opts.Logger)
	if opts.RateLimiter != nil {
		backend.SetRateLimiter(opts.RateLimiter)
	}
	if len(opts.AllowedDomains) > 0 {
		backend.SetAllowedDomains(opts.AllowedDomains)
	}

	// Set server type for metrics
	serverType := opts.ServerType
	if serverType == "" {
		if opts.Implicit {
			serverType = "smtps"
		} else {
			serverType = "smtp"
		}
	}
	backend.SetServerType(serverType)

	srv := smtp.NewServer(backend)
	srv.Domain = opts.Config.Domain
	srv.MaxMessageBytes = int64(opts.Config.MaxMessageBytes)
	srv.MaxRecipients = opts.Config.MaxRecipients
	srv.ReadTimeout = opts.Config.ReadTimeout
	srv.WriteTimeout = opts.Config.WriteTimeout

	// Configure TLS
	if opts.TLSConfig != nil {
		srv.TLSConfig = opts.TLSConfig
		if opts.Implicit {
			// SMTPS: require TLS from start
			srv.AllowInsecureAuth = false
		} else {
			// STARTTLS: allow upgrade
			srv.AllowInsecureAuth = true
		}
	} else {
		srv.AllowInsecureAuth = true
	}

	return &Server{
		server:    srv,
		backend:   backend,
		addr:      opts.Addr,
		tlsConfig: opts.TLSConfig,
		implicit:  opts.Implicit,
		logger:    opts.Logger,
	}
}

// ListenAndServe starts the SMTP server
func (s *Server) ListenAndServe() error {
	s.server.Addr = s.addr
	if s.implicit && s.tlsConfig != nil {
		s.logger.Info("starting SMTPS server (implicit TLS)", "addr", s.addr)
		return s.server.ListenAndServeTLS()
	}
	s.logger.Info("starting SMTP server", "addr", s.addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down SMTP server")
	s.backend.Stop()
	return s.server.Shutdown(ctx)
}

// Close immediately closes the server
func (s *Server) Close() error {
	return s.server.Close()
}
