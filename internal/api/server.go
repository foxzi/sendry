package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
)

// Server is the HTTP API server
type Server struct {
	router     *chi.Mux
	httpServer *http.Server
	queue      queue.Queue
	config     *config.APIConfig
	logger     *slog.Logger
	startTime  time.Time
}

// NewServer creates a new API server
func NewServer(q queue.Queue, cfg *config.APIConfig, logger *slog.Logger) *Server {
	s := &Server{
		router:    chi.NewRouter(),
		queue:     q,
		config:    cfg,
		logger:    logger,
		startTime: time.Now(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	// Middleware
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
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
	})
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe() error {
	s.httpServer = &http.Server{
		Addr:         s.config.ListenAddr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
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
