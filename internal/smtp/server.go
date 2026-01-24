package smtp

import (
	"context"
	"log/slog"

	"github.com/emersion/go-smtp"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
)

// Server wraps go-smtp server with configuration
type Server struct {
	server *smtp.Server
	addr   string
	logger *slog.Logger
}

// NewServer creates a new SMTP server
func NewServer(cfg *config.SMTPConfig, q queue.Queue, logger *slog.Logger) *Server {
	backend := NewBackend(q, &cfg.Auth, logger)

	srv := smtp.NewServer(backend)
	srv.Domain = cfg.Domain
	srv.MaxMessageBytes = int64(cfg.MaxMessageBytes)
	srv.MaxRecipients = cfg.MaxRecipients
	srv.ReadTimeout = cfg.ReadTimeout
	srv.WriteTimeout = cfg.WriteTimeout
	srv.AllowInsecureAuth = true // TLS will be added later

	return &Server{
		server: srv,
		addr:   cfg.ListenAddr,
		logger: logger,
	}
}

// ListenAndServe starts the SMTP server
func (s *Server) ListenAndServe() error {
	s.server.Addr = s.addr
	s.logger.Info("starting SMTP server", "addr", s.addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down SMTP server")
	return s.server.Shutdown(ctx)
}

// Close immediately closes the server
func (s *Server) Close() error {
	return s.server.Close()
}
