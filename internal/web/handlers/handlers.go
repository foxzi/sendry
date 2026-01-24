package handlers

import (
	"log/slog"
	"net/http"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
)

type Handlers struct {
	cfg    *config.Config
	db     *db.DB
	logger *slog.Logger
}

func New(cfg *config.Config, db *db.DB, logger *slog.Logger) *Handlers {
	return &Handlers{
		cfg:    cfg,
		db:     db,
		logger: logger,
	}
}

// Health check
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// Dashboard
func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	h.render(w, "dashboard", nil)
}

// Helper to render templates
func (h *Handlers) render(w http.ResponseWriter, name string, data any) {
	// TODO: implement template rendering
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte("<html><body><h1>" + name + "</h1><p>Page under construction</p></body></html>"))
}

// Helper for JSON responses
func (h *Handlers) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// TODO: implement JSON encoding
}

// Helper for errors
func (h *Handlers) error(w http.ResponseWriter, status int, message string) {
	h.logger.Error("request error", "status", status, "message", message)
	http.Error(w, message, status)
}
