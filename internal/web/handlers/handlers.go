package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/foxzi/sendry/internal/web/auth"
	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/router"
	"github.com/foxzi/sendry/internal/web/sendry"
	"github.com/foxzi/sendry/internal/web/views"
)

type Handlers struct {
	cfg        *config.Config
	db         *db.DB
	logger     *slog.Logger
	views      *views.Engine
	sendry     *sendry.Manager
	oidc       *auth.OIDCProvider
	templates  *repository.TemplateRepository
	recipients *repository.RecipientRepository
	campaigns  *repository.CampaignRepository
	jobs       *repository.JobRepository
	settings   *repository.SettingsRepository
	dkim       *repository.DKIMRepository
	domains    *repository.DomainRepository
	sends      *repository.SendRepository
	apiKeys    *repository.APIKeyRepository
	router     *router.EmailRouter
}

func New(cfg *config.Config, db *db.DB, logger *slog.Logger, v *views.Engine, oidcProvider *auth.OIDCProvider) *Handlers {
	sendryMgr := sendry.NewManager(cfg.Sendry.Servers)
	templates := repository.NewTemplateRepository(db.DB)
	settings := repository.NewSettingsRepository(db.DB)
	domains := repository.NewDomainRepository(db.DB)
	sends := repository.NewSendRepository(db.DB)
	apiKeys := repository.NewAPIKeyRepository(db.DB)

	emailRouter := router.NewEmailRouter(router.RouterConfig{
		Domains:   domains,
		Templates: templates,
		Sends:     sends,
		Settings:  settings,
		Sendry:    sendryMgr,
		MultiSend: &cfg.Sendry.MultiSend,
		Logger:    logger.With("component", "router"),
	})

	return &Handlers{
		cfg:        cfg,
		db:         db,
		logger:     logger,
		views:      v,
		sendry:     sendryMgr,
		oidc:       oidcProvider,
		templates:  templates,
		recipients: repository.NewRecipientRepository(db.DB),
		campaigns:  repository.NewCampaignRepository(db.DB),
		jobs:       repository.NewJobRepository(db.DB),
		settings:   settings,
		dkim:       repository.NewDKIMRepository(db.DB),
		domains:    domains,
		sends:      sends,
		apiKeys:    apiKeys,
		router:     emailRouter,
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

// Get servers status from config (quick, no API calls)
func (h *Handlers) getServersStatus() []map[string]any {
	servers := make([]map[string]any, 0, len(h.cfg.Sendry.Servers))
	for _, s := range h.cfg.Sendry.Servers {
		servers = append(servers, map[string]any{
			"Name":      s.Name,
			"Env":       s.Env,
			"Online":    true,
			"QueueSize": 0,
		})
	}
	return servers
}

// sanitizeFilename removes dangerous characters from filename for Content-Disposition header.
// Prevents HTTP header injection via CRLF and quote characters.
func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "\\", "")
	s = strings.ReplaceAll(s, "/", "")
	return strings.TrimSpace(s)
}

// Get servers status with actual health checks (slow, makes API calls)
func (h *Handlers) getServersStatusLive(r *http.Request) []map[string]any {
	statuses := h.sendry.GetAllStatus(r.Context())
	servers := make([]map[string]any, 0, len(statuses))
	for _, s := range statuses {
		servers = append(servers, map[string]any{
			"Name":      s.Name,
			"Env":       s.Env,
			"Online":    s.Online,
			"Version":   s.Version,
			"Uptime":    s.Uptime,
			"QueueSize": s.QueueSize,
			"Error":     s.Error,
		})
	}
	return servers
}
