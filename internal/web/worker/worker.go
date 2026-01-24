package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
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
	sendry    *sendry.Manager

	batchSize    int
	pollInterval time.Duration
	concurrency  int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

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

	// Parse campaign variables
	var campaignVars map[string]string
	if campaign.Variables != "" {
		json.Unmarshal([]byte(campaign.Variables), &campaignVars)
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

			w.processItem(&item, campaign, variantMap, templateMap, campaignVars)
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

	// Build email subject
	subject := tmpl.Subject
	if variant.SubjectOverride != "" {
		subject = variant.SubjectOverride
	}

	// Build email request
	req := &sendry.SendRequest{
		From:    formatFrom(campaign.FromEmail, campaign.FromName),
		To:      []string{item.Email},
		Subject: subject,
		Body:    tmpl.Text,
		HTML:    tmpl.HTML,
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
