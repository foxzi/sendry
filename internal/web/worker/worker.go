package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/sendry"
)

// Worker processes send jobs in the background
type Worker struct {
	cfg       *config.Config
	logger    *slog.Logger
	jobs      *repository.JobRepository
	campaigns *repository.CampaignRepository
	templates *repository.TemplateRepository
	settings  *repository.SettingsRepository
	sendry    *sendry.Manager

	batchSize    int
	pollInterval time.Duration
	concurrency  int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// variable pattern for template substitution: {{variable_name}}
var varPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// Config holds worker configuration
type Config struct {
	BatchSize    int
	PollInterval time.Duration
	Concurrency  int
}

// DefaultConfig returns default worker configuration
func DefaultConfig() Config {
	return Config{
		BatchSize:    10,
		PollInterval: 5 * time.Second,
		Concurrency:  5,
	}
}

// New creates a new worker
func New(cfg *config.Config, db *sql.DB, logger *slog.Logger, workerCfg Config) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		cfg:          cfg,
		logger:       logger.With("component", "worker"),
		jobs:         repository.NewJobRepository(db),
		campaigns:    repository.NewCampaignRepository(db),
		templates:    repository.NewTemplateRepository(db),
		settings:     repository.NewSettingsRepository(db),
		sendry:       sendry.NewManager(cfg.Sendry.Servers),
		batchSize:    workerCfg.BatchSize,
		pollInterval: workerCfg.PollInterval,
		concurrency:  workerCfg.Concurrency,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the worker
func (w *Worker) Start() {
	w.wg.Add(1)
	go w.run()
	w.logger.Info("worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval, "concurrency", w.concurrency)
}

// Stop stops the worker gracefully
func (w *Worker) Stop() {
	w.logger.Info("stopping worker...")
	w.cancel()
	w.wg.Wait()
	w.logger.Info("worker stopped")
}

func (w *Worker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processJobs()
		}
	}
}

func (w *Worker) processJobs() {
	// Check for scheduled jobs that are due to run
	w.startScheduledJobs()

	// Track status of queued items
	w.trackQueuedItems()

	// Get all running jobs
	jobs, err := w.jobs.GetRunningJobs()
	if err != nil {
		w.logger.Error("failed to get running jobs", "error", err)
		return
	}

	for _, job := range jobs {
		select {
		case <-w.ctx.Done():
			return
		default:
			w.processJob(&job)
		}
	}
}

// trackQueuedItems checks status of queued items via Sendry API
func (w *Worker) trackQueuedItems() {
	items, err := w.jobs.GetQueuedItems(w.batchSize * 2)
	if err != nil {
		w.logger.Error("failed to get queued items", "error", err)
		return
	}

	for _, item := range items {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		client, err := w.sendry.GetClient(item.ServerName)
		if err != nil {
			continue
		}

		status, err := client.GetStatus(w.ctx, item.SendryMsgID)
		if err != nil {
			w.logger.Debug("failed to get status", "item_id", item.ID, "sendry_id", item.SendryMsgID, "error", err)
			continue
		}

		// Map Sendry status to local status
		newStatus := mapSendryStatus(status.Status)
		if newStatus != "" && newStatus != item.Status {
			errorMsg := ""
			if newStatus == "failed" {
				errorMsg = status.LastError
			}
			if err := w.jobs.UpdateItemStatus(item.ID, newStatus, item.SendryMsgID, errorMsg); err != nil {
				w.logger.Error("failed to update item status", "item_id", item.ID, "error", err)
			} else {
				w.logger.Debug("status updated", "item_id", item.ID, "old", item.Status, "new", newStatus)
			}
		}
	}
}

// mapSendryStatus maps Sendry API status to local status
func mapSendryStatus(status string) string {
	switch status {
	case "queued", "pending", "processing":
		return "queued"
	case "sent", "delivered":
		return "sent"
	case "failed", "rejected":
		return "failed"
	case "bounced":
		return "failed"
	default:
		return ""
	}
}

// startScheduledJobs checks for scheduled jobs that are due and starts them
func (w *Worker) startScheduledJobs() {
	scheduledJobs, err := w.jobs.GetScheduledJobsDue()
	if err != nil {
		w.logger.Error("failed to get scheduled jobs", "error", err)
		return
	}

	for _, job := range scheduledJobs {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		// Update job status to running
		if err := w.jobs.UpdateStatus(job.ID, "running"); err != nil {
			w.logger.Error("failed to start scheduled job", "job_id", job.ID, "error", err)
			continue
		}

		w.logger.Info("started scheduled job", "job_id", job.ID, "campaign", job.CampaignName, "scheduled_at", job.ScheduledAt)
	}
}

func (w *Worker) processJob(job *models.SendJob) {
	// Get pending items
	items, err := w.jobs.GetPendingItems(job.ID, w.batchSize)
	if err != nil {
		w.logger.Error("failed to get pending items", "job_id", job.ID, "error", err)
		return
	}

	if len(items) == 0 {
		// No more pending items, check if job is complete
		stats, err := w.jobs.GetStats(job.ID)
		if err != nil {
			w.logger.Error("failed to get job stats", "job_id", job.ID, "error", err)
			return
		}

		if stats.Pending == 0 {
			// Job complete
			status := "completed"
			if stats.Failed > 0 && stats.Sent == 0 {
				status = "failed"
			}
			if err := w.jobs.UpdateStatus(job.ID, status); err != nil {
				w.logger.Error("failed to update job status", "job_id", job.ID, "error", err)
			} else {
				w.logger.Info("job completed", "job_id", job.ID, "status", status, "sent", stats.Sent, "failed", stats.Failed)
			}
		}
		return
	}

	// Get campaign data for email sending
	campaign, err := w.campaigns.GetByID(job.CampaignID)
	if err != nil || campaign == nil {
		w.logger.Error("failed to get campaign", "job_id", job.ID, "campaign_id", job.CampaignID, "error", err)
		return
	}

	// Get variants for this campaign
	variants, err := w.campaigns.GetVariants(job.CampaignID)
	if err != nil {
		w.logger.Error("failed to get variants", "job_id", job.ID, "error", err)
		return
	}

	variantMap := make(map[string]*models.CampaignVariant)
	for i := range variants {
		variantMap[variants[i].ID] = &variants[i]
	}

	// Load templates for variants
	templateMap := make(map[string]*models.Template)
	for _, v := range variants {
		if _, exists := templateMap[v.TemplateID]; !exists {
			tmpl, err := w.templates.GetByID(v.TemplateID)
			if err != nil {
				w.logger.Error("failed to get template", "template_id", v.TemplateID, "error", err)
				continue
			}
			templateMap[v.TemplateID] = tmpl
		}
	}

	// Get global variables
	globalVars, err := w.settings.GetGlobalVariablesMap()
	if err != nil {
		w.logger.Error("failed to get global variables", "job_id", job.ID, "error", err)
		globalVars = make(map[string]string)
	}

	// Parse campaign variables
	var campaignVars map[string]string
	if campaign.Variables != "" {
		if err := json.Unmarshal([]byte(campaign.Variables), &campaignVars); err != nil {
			w.logger.Error("failed to parse campaign variables", "job_id", job.ID, "error", err)
		}
	}

	// Process items concurrently
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup

	for _, item := range items {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(item models.SendJobItem) {
			defer func() {
				<-sem
				wg.Done()
			}()

			w.processItem(&item, campaign, variantMap, templateMap, globalVars, campaignVars)
		}(item)
	}

	wg.Wait()

	// Update job stats
	stats, err := w.jobs.GetStats(job.ID)
	if err == nil {
		if err := w.jobs.UpdateStats(job.ID, stats); err != nil {
			w.logger.Error("failed to update job stats", "job_id", job.ID, "error", err)
		}
	}
}

func (w *Worker) processItem(
	item *models.SendJobItem,
	campaign *models.Campaign,
	variantMap map[string]*models.CampaignVariant,
	templateMap map[string]*models.Template,
	globalVars map[string]string,
	campaignVars map[string]string,
) {
	// Get variant
	variant, ok := variantMap[item.VariantID]
	if !ok {
		w.updateItemFailed(item.ID, "variant not found")
		return
	}

	// Get template
	tmpl, ok := templateMap[variant.TemplateID]
	if !ok || tmpl == nil {
		w.updateItemFailed(item.ID, "template not found")
		return
	}

	// Get Sendry client
	client, err := w.sendry.GetClient(item.ServerName)
	if err != nil {
		w.updateItemFailed(item.ID, "server not found: "+item.ServerName)
		return
	}

	// Build merged variables map (priority: recipient > campaign > global)
	vars := mergeVariables(globalVars, campaignVars, item.RecipientVariables)

	// Add built-in variables
	vars["email"] = item.Email
	vars["recipient_email"] = item.Email
	if item.RecipientName != "" {
		vars["name"] = item.RecipientName
		vars["recipient_name"] = item.RecipientName
	}

	// Build email subject with variable substitution
	subject := tmpl.Subject
	if variant.SubjectOverride != "" {
		subject = variant.SubjectOverride
	}
	subject = renderTemplate(subject, vars)

	// Render HTML and text with variable substitution
	html := renderTemplate(tmpl.HTML, vars)
	text := renderTemplate(tmpl.Text, vars)

	// Build email request
	req := &sendry.SendRequest{
		From:    formatFrom(campaign.FromEmail, campaign.FromName),
		To:      []string{item.Email},
		Subject: subject,
		Body:    text,
		HTML:    html,
	}

	if campaign.ReplyTo != "" {
		req.Headers = map[string]string{"Reply-To": campaign.ReplyTo}
	}

	// Send email
	resp, err := client.Send(w.ctx, req)
	if err != nil {
		w.updateItemFailed(item.ID, err.Error())
		w.logger.Debug("failed to send email", "item_id", item.ID, "email", item.Email, "error", err)
		return
	}

	// Update item as queued
	if err := w.jobs.UpdateItemStatus(item.ID, "queued", resp.ID, ""); err != nil {
		w.logger.Error("failed to update item status", "item_id", item.ID, "error", err)
		return
	}

	w.logger.Debug("email queued", "item_id", item.ID, "email", item.Email, "sendry_id", resp.ID)
}

// mergeVariables merges variable maps with priority: recipient > campaign > global
func mergeVariables(global, campaign map[string]string, recipientJSON string) map[string]string {
	result := make(map[string]string)

	// Start with global variables (lowest priority)
	for k, v := range global {
		result[k] = v
	}

	// Add campaign variables (medium priority)
	for k, v := range campaign {
		result[k] = v
	}

	// Add recipient variables (highest priority)
	if recipientJSON != "" {
		var recipientVars map[string]string
		if err := json.Unmarshal([]byte(recipientJSON), &recipientVars); err == nil {
			for k, v := range recipientVars {
				result[k] = v
			}
		}
	}

	return result
}

// renderTemplate substitutes {{variable}} patterns in template string
func renderTemplate(template string, vars map[string]string) string {
	if template == "" {
		return template
	}

	return varPattern.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable name (remove {{ and }})
		varName := strings.TrimSpace(match[2 : len(match)-2])
		if value, ok := vars[varName]; ok {
			return value
		}
		// Keep original if variable not found
		return match
	})
}

func (w *Worker) updateItemFailed(itemID, errorMsg string) {
	if err := w.jobs.UpdateItemStatus(itemID, "failed", "", errorMsg); err != nil {
		w.logger.Error("failed to update item status", "item_id", itemID, "error", err)
	}
}

func formatFrom(email, name string) string {
	if name == "" {
		return email
	}
	return name + " <" + email + ">"
}
