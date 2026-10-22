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
	emailtpl "github.com/foxzi/sendry/internal/web/template"
	"github.com/foxzi/sendry/internal/web/tracking"
)

// Worker processes send jobs in the background
type Worker struct {
	cfg       *config.Config
	logger    *slog.Logger
	jobs      *repository.JobRepository
	sends     *repository.SendRepository
	campaigns *repository.CampaignRepository
	templates *repository.TemplateRepository
	settings  *repository.SettingsRepository
	tracking  *repository.TrackingRepository
	unsubs    *repository.UnsubscribeRepository
	sendry    *sendry.Manager

	batchSize    int
	pollInterval time.Duration
	concurrency  int

	// rowid of the last queued job item and send checked, so status tracking cycles through all of them
	queuedCursor     int64
	queuedSendCursor int64

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
		sends:        repository.NewSendRepository(db),
		campaigns:    repository.NewCampaignRepository(db),
		templates:    repository.NewTemplateRepository(db),
		settings:     repository.NewSettingsRepository(db),
		tracking:     repository.NewTrackingRepository(db),
		unsubs:       repository.NewUnsubscribeRepository(db),
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

	// Track status of queued items and API sends
	w.trackQueuedItems()
	w.trackQueuedSends()

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
	limit := w.batchSize * 2
	items, lastRowID, err := w.jobs.GetQueuedItems(w.queuedCursor, limit)
	if err != nil {
		w.logger.Error("failed to get queued items", "error", err)
		return
	}

	// Start over from the oldest item once the end of the queue is reached
	if len(items) < limit {
		w.queuedCursor = 0
	} else {
		w.queuedCursor = lastRowID
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

		newStatus, errorMsg, changed := deliveryUpdate(item.Status, item.Error, status)
		if changed {
			if err := w.jobs.UpdateItemStatus(item.ID, newStatus, item.SendryMsgID, errorMsg); err != nil {
				w.logger.Error("failed to update item status", "item_id", item.ID, "error", err)
			} else {
				w.logger.Debug("status updated", "item_id", item.ID, "old", item.Status, "new", newStatus)
			}
		}
	}
}

// trackQueuedSends checks delivery status of API sends accepted by Sendry
func (w *Worker) trackQueuedSends() {
	limit := w.batchSize * 2
	sends, lastRowID, err := w.sends.GetQueuedSends(w.queuedSendCursor, limit)
	if err != nil {
		w.logger.Error("failed to get queued sends", "error", err)
		return
	}

	// Start over from the oldest send once the end of the queue is reached
	if len(sends) < limit {
		w.queuedSendCursor = 0
	} else {
		w.queuedSendCursor = lastRowID
	}

	for _, send := range sends {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		client, err := w.sendry.GetClient(send.ServerName)
		if err != nil {
			continue
		}

		status, err := client.GetStatus(w.ctx, send.ServerMsgID)
		if err != nil {
			w.logger.Debug("failed to get send status", "send_id", send.ID, "sendry_id", send.ServerMsgID, "error", err)
			continue
		}

		newStatus, errorMsg, changed := deliveryUpdate(send.Status, send.ErrorMessage, status)
		if !changed {
			continue
		}

		var sentAt *time.Time
		if newStatus == models.SendStatusSent {
			now := time.Now()
			sentAt = &now
		}
		if err := w.sends.UpdateStatus(send.ID, newStatus, errorMsg, send.ServerMsgID, sentAt); err != nil {
			w.logger.Error("failed to update send status", "send_id", send.ID, "error", err)
		} else {
			w.logger.Debug("send status updated", "send_id", send.ID, "old", send.Status, "new", newStatus)
		}
	}
}

// deliveryUpdate returns the local status and error for a queued message from its Sendry status,
// and whether the record has to be updated. Deferred messages stay queued but keep the last
// delivery error, so problems are visible before the message finally fails.
func deliveryUpdate(curStatus, curError string, status *sendry.StatusResponse) (string, string, bool) {
	newStatus := mapSendryStatus(status.Status)
	if newStatus == "" {
		return "", "", false
	}

	errorMsg := ""
	if newStatus != "sent" {
		errorMsg = status.LastError
	}

	return newStatus, errorMsg, newStatus != curStatus || errorMsg != curError
}

// mapSendryStatus maps Sendry API status to local status
func mapSendryStatus(status string) string {
	switch status {
	case "queued", "pending", "processing", "sending", "deferred":
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

	// Tracking environment is loaded once per batch
	env := w.loadTrackingEnv()

	// Number template links once; the numbers are stable across recipients
	numberedMap := make(map[string]string, len(templateMap))
	for id, tmpl := range templateMap {
		if tmpl != nil {
			numberedMap[id] = tracking.NumberLinks(tmpl.HTML)
		}
	}

	w.saveSnapshots(job, items, variantMap, templateMap, numberedMap, globalVars, campaignVars)

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

			w.processItem(&item, campaign, variantMap, templateMap, numberedMap, env, globalVars, campaignVars)
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
	numberedMap map[string]string,
	env *trackingEnv,
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

	// Built-in unsubscribe link; set last so recipient or campaign variables cannot override it
	vars["unsubscribe_url"] = env.unsubscribeURL(item.ID)

	// Build email subject with variable substitution
	subjectSrc := tmpl.Subject
	if variant.SubjectOverride != "" {
		subjectSrc = variant.SubjectOverride
	}
	subject, err := renderTextTemplate("subject", subjectSrc, vars)
	if err != nil {
		w.updateItemFailed(item.ID, "render subject: "+err.Error())
		return
	}

	// Render HTML and text with variable substitution
	numbered, ok := numberedMap[variant.TemplateID]
	if !ok {
		numbered = tmpl.HTML
	}
	html, err := buildCampaignHTML(numbered, vars, env, campaign, item.ID)
	if err != nil {
		w.updateItemFailed(item.ID, "render html: "+err.Error())
		return
	}
	text, err := renderTextTemplate("text", tmpl.Text, vars)
	if err != nil {
		w.updateItemFailed(item.ID, "render text: "+err.Error())
		return
	}

	// Build email request
	req := &sendry.SendRequest{
		From:    formatFrom(campaign.FromEmail, campaign.FromName),
		To:      []string{item.Email},
		Subject: subject,
		Body:    text,
		HTML:    html,
	}

	req.Headers = campaignHeaders(campaign, env, item.ID)

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

func mergeVariables(global, campaign map[string]string, recipientJSON string) map[string]any {
	result := make(map[string]any)

	// Start with global variables (lowest priority)
	for k, v := range global {
		result[k] = v
	}

	// Add campaign variables (medium priority)
	for k, v := range campaign {
		result[k] = v
	}

	if recipientJSON != "" {
		var recipientVars map[string]any
		if err := json.Unmarshal([]byte(recipientJSON), &recipientVars); err == nil {
			for k, v := range recipientVars {
				result[k] = v
			}
		} else {
			var flat map[string]string
			if err := json.Unmarshal([]byte(recipientJSON), &flat); err == nil {
				for k, v := range flat {
					result[k] = v
				}
			}
		}
	}

	return result
}

func renderTemplate(name, src string, vars map[string]any) (string, error) {
	return emailtpl.RenderHTML(name, src, vars)
}

func renderTextTemplate(name, src string, vars map[string]any) (string, error) {
	return emailtpl.RenderText(name, src, vars)
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

// trackingEnv is what the worker needs to build tracked mail. A nil env means
// tracking is unavailable and mail is sent as is.
type trackingEnv struct {
	baseURL string
	signer  *tracking.Signer
}

// loadTrackingEnv loads tracking settings and the token signer. It returns nil
// (and logs) when they are unavailable: a send must never fail because of tracking.
func (w *Worker) loadTrackingEnv() *trackingEnv {
	ts, err := w.settings.GetTrackingSettings()
	if err != nil || ts == nil || ts.Secret == "" {
		w.logger.Error("tracking settings unavailable, sending without tracking", "error", err)
		return nil
	}
	base := tracking.ResolveBaseURL(ts.BaseURL, w.cfg.Server.PublicURL)
	if base == "" {
		w.logger.Warn("tracking base URL is not configured, sending without tracking")
		return nil
	}
	return &trackingEnv{baseURL: base, signer: tracking.NewSigner([]byte(ts.Secret))}
}

// unsubscribeURL returns the signed unsubscribe link of a job item, or "" without env
func (e *trackingEnv) unsubscribeURL(itemID string) string {
	if e == nil {
		return ""
	}
	return tracking.UnsubscribeURL(e.baseURL, e.signer, models.SourceJobItem, itemID)
}

// buildCampaignHTML renders numbered template HTML with the recipient variables
// and applies open/click tracking (and UTM when the campaign asks for it).
func buildCampaignHTML(numbered string, vars map[string]any, env *trackingEnv, campaign *models.Campaign, itemID string) (string, error) {
	html, err := renderTemplate("html", numbered, vars)
	if err != nil {
		return "", err
	}
	if env == nil {
		// Without tracking only strip the numbering attributes
		return tracking.Apply(html, tracking.Options{}), nil
	}

	opts := tracking.Options{
		BaseURL:     env.baseURL,
		Signer:      env.signer,
		SourceType:  models.SourceJobItem,
		SourceID:    itemID,
		TrackOpens:  campaign.TrackOpens,
		TrackClicks: campaign.TrackClicks,
	}
	if campaign.AddUTM {
		opts.UTM = &tracking.UTM{
			Source:   "email",
			Medium:   "email",
			Campaign: campaign.Name,
			SC:       campaign.ID,
			SR:       itemID,
			AddSL:    true,
		}
	}
	return tracking.Apply(html, opts), nil
}

// campaignHeaders returns the extra headers of campaign mail: Reply-To and the
// List-Unsubscribe pair. Returns nil when there are none.
func campaignHeaders(campaign *models.Campaign, env *trackingEnv, itemID string) map[string]string {
	headers := map[string]string{}
	if campaign.ReplyTo != "" {
		headers["Reply-To"] = campaign.ReplyTo
	}
	if env != nil {
		for k, v := range tracking.ListUnsubscribeHeaders(env.unsubscribeURL(itemID)) {
			headers[k] = v
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// saveSnapshots stores, for every variant used by this batch that has no
// snapshot yet, the numbered HTML and its link list. The first item of the batch
// for the variant supplies the recipient variables. Failures are only logged.
func (w *Worker) saveSnapshots(
	job *models.SendJob,
	items []models.SendJobItem,
	variantMap map[string]*models.CampaignVariant,
	templateMap map[string]*models.Template,
	numberedMap map[string]string,
	globalVars map[string]string,
	campaignVars map[string]string,
) {
	existing, err := w.tracking.GetSnapshots(job.ID)
	if err != nil {
		w.logger.Error("failed to load snapshots", "job_id", job.ID, "error", err)
		return
	}
	done := make(map[string]bool, len(existing))
	for _, sn := range existing {
		done[sn.VariantID] = true
	}

	for _, item := range items {
		if done[item.VariantID] {
			continue
		}
		done[item.VariantID] = true // try once per batch

		variant := variantMap[item.VariantID]
		if variant == nil {
			continue
		}
		tmpl := templateMap[variant.TemplateID]
		numbered, ok := numberedMap[variant.TemplateID]
		if tmpl == nil || !ok {
			continue
		}

		vars := mergeVariables(globalVars, campaignVars, item.RecipientVariables)
		vars["email"] = item.Email
		vars["recipient_email"] = item.Email
		if item.RecipientName != "" {
			vars["name"] = item.RecipientName
			vars["recipient_name"] = item.RecipientName
		}
		vars["unsubscribe_url"] = "#"

		html, err := renderTemplate("html", numbered, vars)
		if err != nil {
			w.logger.Error("failed to render snapshot", "job_id", job.ID, "variant_id", variant.ID, "error", err)
			continue
		}

		snap := &models.JobVariantSnapshot{
			JobID:           job.ID,
			VariantID:       variant.ID,
			VariantName:     variant.Name,
			TemplateID:      tmpl.ID,
			TemplateVersion: tmpl.CurrentVersion,
			HTML:            html,
			Links:           tracking.ExtractLinks(numbered),
		}
		if err := w.tracking.SaveSnapshot(snap); err != nil {
			w.logger.Error("failed to save snapshot", "job_id", job.ID, "variant_id", variant.ID, "error", err)
		}
	}
}
