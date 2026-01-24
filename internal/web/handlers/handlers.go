package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/views"
)

type Handlers struct {
	cfg        *config.Config
	db         *db.DB
	logger     *slog.Logger
	views      *views.Engine
	templates  *repository.TemplateRepository
	recipients *repository.RecipientRepository
	campaigns  *repository.CampaignRepository
}

func New(cfg *config.Config, db *db.DB, logger *slog.Logger, v *views.Engine) *Handlers {
	return &Handlers{
		cfg:        cfg,
		db:         db,
		logger:     logger,
		views:      v,
		templates:  repository.NewTemplateRepository(db.DB),
		recipients: repository.NewRecipientRepository(db.DB),
		campaigns:  repository.NewCampaignRepository(db.DB),
	}
}

// Health check
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// Dashboard
func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	// Get stats from DB
	var templates, campaigns, recipients, activeJobs int
	h.db.QueryRow("SELECT COUNT(*) FROM templates").Scan(&templates)
	h.db.QueryRow("SELECT COUNT(*) FROM campaigns").Scan(&campaigns)
	h.db.QueryRow("SELECT COUNT(*) FROM recipients").Scan(&recipients)
	h.db.QueryRow("SELECT COUNT(*) FROM send_jobs WHERE status = 'running'").Scan(&activeJobs)

	data := map[string]any{
		"Title":  "Dashboard",
		"Active": "dashboard",
		"User":   h.getUserFromContext(r),
		"Stats": map[string]int{
			"Templates":  templates,
			"Campaigns":  campaigns,
			"Recipients": recipients,
			"ActiveJobs": activeJobs,
		},
		"Servers":    h.getServersStatus(),
		"RecentJobs": []any{},
	}

	h.render(w, "dashboard", data)
}

// Helper to render templates
func (h *Handlers) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.views.Render(w, name, data); err != nil {
		h.logger.Error("failed to render template", "name", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Helper for JSON responses
func (h *Handlers) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode JSON", "error", err)
	}
}

// Helper for errors
func (h *Handlers) error(w http.ResponseWriter, status int, message string) {
	h.logger.Error("request error", "status", status, "message", message)
	http.Error(w, message, status)
}

// Get user from request context
func (h *Handlers) getUserFromContext(r *http.Request) map[string]string {
	// TODO: implement proper user context
	return map[string]string{
		"Email": "admin@example.com",
	}
}

// Get servers status from config
func (h *Handlers) getServersStatus() []map[string]any {
	servers := make([]map[string]any, 0, len(h.cfg.Sendry.Servers))
	for _, s := range h.cfg.Sendry.Servers {
		servers = append(servers, map[string]any{
			"Name":      s.Name,
			"Env":       s.Env,
			"Online":    true, // TODO: check actual status
			"QueueSize": 0,
		})
	}
	return servers
}
