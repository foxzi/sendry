package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/foxzi/sendry/internal/web/auth"
	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/handlers"
	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/static"
	"github.com/foxzi/sendry/internal/web/views"
	"github.com/foxzi/sendry/internal/web/worker"
)

type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *db.DB
	views  *views.Engine
	http   *http.Server
	worker *worker.Worker
	oidc   *auth.OIDCProvider
}

func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	// Initialize database
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Run migrations
	if err := database.Migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize views
	viewEngine, err := views.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize views: %w", err)
	}

	// Initialize OIDC provider if enabled
	var oidcProvider *auth.OIDCProvider
	if cfg.Auth.OIDC.Enabled {
		oidcProvider, err = auth.NewOIDCProvider(context.Background(), &cfg.Auth.OIDC)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OIDC provider: %w", err)
		}
		logger.Info("OIDC provider initialized", "issuer", cfg.Auth.OIDC.IssuerURL)
	}

	// Create server
	s := &Server{
		cfg:    cfg,
		logger: logger,
		db:     database,
		views:  viewEngine,
		oidc:   oidcProvider,
	}

	// Setup HTTP server
	s.http = &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      s.setupRoutes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Initialize worker
	s.worker = worker.New(cfg, database.DB, logger, worker.DefaultConfig())

	return s, nil
}

func (s *Server) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Create handlers
	h := handlers.New(s.cfg, s.db, s.logger, s.views, s.oidc)

	// Health check
	mux.HandleFunc("GET /health", h.Health)

	// Static files (embedded)
	mux.Handle("GET /static/", http.StripPrefix("/static/", static.Handler()))

	// Auth routes (public)
	mux.HandleFunc("GET /auth/login", h.LoginPage)
	mux.HandleFunc("POST /auth/login", h.Login)
	mux.HandleFunc("GET /auth/logout", h.Logout)
	mux.HandleFunc("GET /auth/oidc/login", h.OIDCLogin)
	mux.HandleFunc("GET /auth/callback", h.OIDCCallback)

	// Protected routes
	protected := http.NewServeMux()

	// Dashboard
	protected.HandleFunc("GET /", h.Dashboard)

	// Templates
	protected.HandleFunc("GET /templates", h.TemplateList)
	protected.HandleFunc("GET /templates/new", h.TemplateNew)
	protected.HandleFunc("GET /templates/import", h.TemplateImportPage)
	protected.HandleFunc("POST /templates/import", h.TemplateImport)
	protected.HandleFunc("POST /templates", h.TemplateCreate)
	protected.HandleFunc("GET /templates/{id}", h.TemplateView)
	protected.HandleFunc("GET /templates/{id}/edit", h.TemplateEdit)
	protected.HandleFunc("PUT /templates/{id}", h.TemplateUpdate)
	protected.HandleFunc("DELETE /templates/{id}", h.TemplateDelete)
	protected.HandleFunc("GET /templates/{id}/versions", h.TemplateVersions)
	protected.HandleFunc("GET /templates/{id}/diff", h.TemplateDiff)
	protected.HandleFunc("GET /templates/{id}/export", h.TemplateExport)
	protected.HandleFunc("GET /templates/{id}/test", h.TemplateTestPage)
	protected.HandleFunc("POST /templates/{id}/test", h.TemplateTest)
	protected.HandleFunc("POST /templates/{id}/deploy", h.TemplateDeploy)
	protected.HandleFunc("GET /templates/{id}/preview", h.TemplatePreview)

	// Recipients
	protected.HandleFunc("GET /recipients", h.RecipientListList)
	protected.HandleFunc("GET /recipients/new", h.RecipientListNew)
	protected.HandleFunc("POST /recipients", h.RecipientListCreate)
	protected.HandleFunc("GET /recipients/{id}", h.RecipientListView)
	protected.HandleFunc("GET /recipients/{id}/edit", h.RecipientListEdit)
	protected.HandleFunc("PUT /recipients/{id}", h.RecipientListUpdate)
	protected.HandleFunc("DELETE /recipients/{id}", h.RecipientListDelete)
	protected.HandleFunc("GET /recipients/{id}/import", h.RecipientImportPage)
	protected.HandleFunc("POST /recipients/{id}/import", h.RecipientImport)
	protected.HandleFunc("GET /recipients/{id}/export", h.RecipientListExport)
	protected.HandleFunc("GET /recipients/{id}/recipients", h.RecipientsList)
	protected.HandleFunc("POST /recipients/{id}/add", h.RecipientAdd)
	protected.HandleFunc("DELETE /recipients/{id}/recipients/{recipientId}", h.RecipientDelete)

	// Campaigns
	protected.HandleFunc("GET /campaigns", h.CampaignList)
	protected.HandleFunc("GET /campaigns/new", h.CampaignNew)
	protected.HandleFunc("POST /campaigns", h.CampaignCreate)
	protected.HandleFunc("GET /campaigns/{id}", h.CampaignView)
	protected.HandleFunc("GET /campaigns/{id}/edit", h.CampaignEdit)
	protected.HandleFunc("PUT /campaigns/{id}", h.CampaignUpdate)
	protected.HandleFunc("DELETE /campaigns/{id}", h.CampaignDelete)
	protected.HandleFunc("GET /campaigns/{id}/variables", h.CampaignVariables)
	protected.HandleFunc("PUT /campaigns/{id}/variables", h.CampaignVariablesUpdate)
	protected.HandleFunc("GET /campaigns/{id}/variants", h.CampaignVariants)
	protected.HandleFunc("POST /campaigns/{id}/variants", h.CampaignVariantCreate)
	protected.HandleFunc("DELETE /campaigns/{id}/variants/{variantId}", h.CampaignVariantDelete)
	protected.HandleFunc("GET /campaigns/{id}/send", h.CampaignSendPage)
	protected.HandleFunc("POST /campaigns/{id}/send", h.CampaignSend)
	protected.HandleFunc("GET /campaigns/{id}/jobs", h.CampaignJobs)

	// Jobs
	protected.HandleFunc("GET /jobs", h.JobList)
	protected.HandleFunc("GET /jobs/{id}", h.JobView)
	protected.HandleFunc("GET /jobs/{id}/items", h.JobItems)
	protected.HandleFunc("POST /jobs/{id}/pause", h.JobPause)
	protected.HandleFunc("POST /jobs/{id}/resume", h.JobResume)
	protected.HandleFunc("POST /jobs/{id}/cancel", h.JobCancel)
	protected.HandleFunc("POST /jobs/{id}/retry", h.JobRetry)

	// Servers
	protected.HandleFunc("GET /servers", h.ServerList)
	protected.HandleFunc("GET /servers/{name}", h.ServerView)
	protected.HandleFunc("GET /servers/{name}/queue", h.ServerQueue)
	protected.HandleFunc("GET /servers/{name}/dlq", h.ServerDLQ)
	protected.HandleFunc("GET /servers/{name}/sandbox", h.ServerSandbox)

	// Domains (per server)
	protected.HandleFunc("GET /servers/{server}/domains", h.DomainsList)
	protected.HandleFunc("GET /servers/{server}/domains/new", h.DomainsNew)
	protected.HandleFunc("POST /servers/{server}/domains", h.DomainsCreate)
	protected.HandleFunc("GET /servers/{server}/domains/{domain}", h.DomainsView)
	protected.HandleFunc("GET /servers/{server}/domains/{domain}/edit", h.DomainsEdit)
	protected.HandleFunc("POST /servers/{server}/domains/{domain}", h.DomainsUpdate)
	protected.HandleFunc("POST /servers/{server}/domains/{domain}/delete", h.DomainsDelete)

	// Monitoring
	protected.HandleFunc("GET /monitoring", h.Monitoring)

	// Settings
	protected.HandleFunc("GET /settings", h.Settings)
	protected.HandleFunc("GET /settings/variables", h.GlobalVariables)
	protected.HandleFunc("PUT /settings/variables", h.GlobalVariablesUpdate)
	protected.HandleFunc("GET /settings/users", h.UserList)
	protected.HandleFunc("GET /settings/audit", h.AuditLog)

	// DKIM (per server)
	protected.HandleFunc("GET /servers/{server}/dkim", h.DKIMList)
	protected.HandleFunc("GET /servers/{server}/dkim/new", h.DKIMNew)
	protected.HandleFunc("POST /servers/{server}/dkim", h.DKIMCreate)
	protected.HandleFunc("GET /servers/{server}/dkim/{id}", h.DKIMView)
	protected.HandleFunc("DELETE /servers/{server}/dkim/{id}", h.DKIMDelete)
	protected.HandleFunc("POST /servers/{server}/dkim/{id}/deploy", h.DKIMDeploy)
	protected.HandleFunc("DELETE /servers/{server}/dkim/{id}/deployments", h.DKIMDeploymentDelete)

	// Wrap protected routes with auth middleware
	authMiddleware := middleware.Auth(s.cfg, s.db, s.logger)
	mux.Handle("/", authMiddleware(protected))

	// Apply global middleware
	handler := middleware.MethodOverride(mux)
	handler = middleware.Logger(s.logger)(handler)
	handler = middleware.Recovery(s.logger)(handler)

	return handler
}

func (s *Server) Run(ctx context.Context) error {
	// Start background worker
	s.worker.Start()

	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("starting web server", "addr", s.cfg.Server.ListenAddr)
		if s.cfg.Server.TLS.Enabled {
			errCh <- s.http.ListenAndServeTLS(s.cfg.Server.TLS.CertFile, s.cfg.Server.TLS.KeyFile)
		} else {
			errCh <- s.http.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		s.worker.Stop()
		return err
	case <-ctx.Done():
		// Stop worker first
		s.worker.Stop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("shutdown error", "error", err)
		}
		s.db.Close()
		return nil
	}
}
