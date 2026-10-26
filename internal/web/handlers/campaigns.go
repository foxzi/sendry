package handlers

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/tracking"
)

func (h *Handlers) CampaignList(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	filter := models.CampaignListFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	}

	campaigns, total, err := h.campaigns.List(filter)
	if err != nil {
		h.logger.Error("failed to list campaigns", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load campaigns")
		return
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Campaigns",
		"Active":     "campaigns",
		"User":       h.getUserFromContext(r),
		"Campaigns":  campaigns,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
	}

	h.render(w, "campaigns", data)
}

func (h *Handlers) CampaignView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load campaign")
		return
	}
	if c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	data, err := h.campaignFlowData(r, "edit", c)
	if err != nil {
		h.logger.Error("failed to build campaign page", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load campaign")
		return
	}

	recentJobs, _, _ := h.jobs.List(models.JobListFilter{CampaignID: id, Limit: 10})
	trackingStats, err := h.tracking.CampaignStats(id)
	if err != nil {
		h.logger.Error("failed to load campaign tracking stats", "error", err)
	}
	data["Tracking"] = trackingCards(c, trackingStats, err)
	data["Jobs"] = h.jobsWithStats(recentJobs)

	h.render(w, "campaign_view", data)
}

// CampaignRedirect sends the old edit, variables and variants pages to the
// campaign page.
func (h *Handlers) CampaignRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/campaigns/"+r.PathValue("id"), http.StatusMovedPermanently)
}

func (h *Handlers) CampaignDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.campaigns.Delete(id); err != nil {
		h.logger.Error("failed to delete campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete campaign")
		return
	}
	// email_events has no foreign key to campaigns
	if err := h.tracking.DeleteCampaignEvents(id); err != nil {
		h.logger.Error("failed to delete campaign events", "campaign_id", id, "error", err)
	}

	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r),
		"delete", "campaign", id, "")
	http.Redirect(w, r, "/campaigns", http.StatusSeeOther)
}

// publicOrigin returns the public origin of unsubscribe and tracking links:
// the tracking base URL setting or server.public_url; "" if neither is set.
func (h *Handlers) publicOrigin() (string, error) {
	ts, err := h.settings.GetTrackingSettings()
	if err != nil {
		return "", err
	}
	return tracking.ResolveBaseURL(ts.BaseURL, h.cfg.Server.PublicURL), nil
}

// hasPublicOrigin reports whether unsubscribe and tracking links can be built.
// A settings load error is logged and counts as configured.
func (h *Handlers) hasPublicOrigin() bool {
	origin, err := h.publicOrigin()
	if err != nil {
		h.logger.Error("failed to load tracking settings", "error", err)
		return true
	}
	return origin != ""
}

func (h *Handlers) CampaignSendPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	data, err := h.campaignFlowData(r, "send", c)
	if err != nil {
		h.logger.Error("failed to build campaign send page", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load campaign")
		return
	}
	h.render(w, "campaign_send", data)
}

// activeVariants returns the variants that are sent (weight > 0), in order
func activeVariants(variants []models.CampaignVariant) []models.CampaignVariant {
	active := make([]models.CampaignVariant, 0, len(variants))
	for _, v := range variants {
		if v.Weight > 0 {
			active = append(active, v)
		}
	}
	return active
}

func (h *Handlers) CampaignSend(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	params, perr := parseJobParams(r)
	if perr != nil {
		h.error(w, perr.status, perr.message)
		return
	}

	// Get variants
	variants, err := h.campaigns.GetVariants(id)
	variants = activeVariants(variants)
	if err != nil || len(variants) == 0 {
		h.error(w, http.StatusBadRequest, "Campaign has no variants configured")
		return
	}

	job, perr := h.startJob(r, c, variants, params)
	if perr != nil {
		h.error(w, perr.status, perr.message)
		return
	}
	http.Redirect(w, r, "/jobs/"+job.ID, http.StatusSeeOther)
}

// statusError is an error to answer a request with
type statusError struct {
	status  int
	message string
}

// jobParams are the send settings of a campaign send form
type jobParams struct {
	RecipientListID string
	Servers         []string
	Strategy        string
	DryRun          bool
	DryRunLimit     int
	ScheduledAt     *time.Time
}

// parseJobParams reads and validates the send settings of a parsed form:
// recipient_list_id, servers, strategy, dry_run, dry_run_limit, scheduled_at,
// tz and tz_offset.
func parseJobParams(r *http.Request) (jobParams, *statusError) {
	p := jobParams{
		RecipientListID: r.FormValue("recipient_list_id"),
		Servers:         r.Form["servers"],
		Strategy:        r.FormValue("strategy"),
	}
	if p.RecipientListID == "" {
		return p, &statusError{http.StatusBadRequest, "Recipient list is required"}
	}
	if len(p.Servers) == 0 {
		return p, &statusError{http.StatusBadRequest, "At least one server is required"}
	}

	if p.Strategy == "" {
		p.Strategy = jobStrategyRoundRobin
	}
	if p.Strategy != jobStrategyRoundRobin && p.Strategy != jobStrategyRandom && p.Strategy != jobStrategyWeighted {
		return p, &statusError{http.StatusBadRequest, "Unknown distribution strategy"}
	}

	// Handle dry-run mode
	p.DryRun = r.FormValue("dry_run") == "on"
	if p.DryRun {
		p.DryRunLimit, _ = strconv.Atoi(r.FormValue("dry_run_limit"))
		if p.DryRunLimit <= 0 {
			p.DryRunLimit = 10 // default
		}
	}

	// Handle scheduled_at
	if scheduledAt := r.FormValue("scheduled_at"); scheduledAt != "" {
		t, err := parseScheduledAt(scheduledAt, r.FormValue("tz"), r.FormValue("tz_offset"))
		if err != nil {
			// Never fall back to sending right away
			return p, &statusError{http.StatusBadRequest, "Invalid schedule time"}
		}
		p.ScheduledAt = &t
	}
	return p, nil
}

// startJob creates the send job of a campaign with an item per recipient of
// the list, and starts it unless it is scheduled. variants must not be empty.
func (h *Handlers) startJob(r *http.Request, c *models.Campaign, variants []models.CampaignVariant, p jobParams) (*models.SendJob, *statusError) {
	// Get recipients: active ones up to the dry-run limit (all without it)
	recipients, unsubscribed, err := h.jobRecipients(p.RecipientListID, p.DryRunLimit)
	if err != nil {
		h.logger.Error("failed to get recipients", "error", err)
		return nil, &statusError{http.StatusInternalServerError, "Failed to get recipients"}
	}
	if len(recipients) == 0 {
		// Nobody to send to: an empty run would only show up as "completed"
		return nil, &statusError{http.StatusBadRequest, "The recipient list has no active recipients"}
	}

	// Create servers JSON
	serversJSON, _ := json.Marshal(p.Servers)

	// Create job
	job := &models.SendJob{
		CampaignID:      c.ID,
		RecipientListID: p.RecipientListID,
		Servers:         string(serversJSON),
		Strategy:        p.Strategy,
		DryRun:          p.DryRun,
		DryRunLimit:     p.DryRunLimit,
		TrackOpens:      c.TrackOpens && h.hasPublicOrigin(), // no pixel without the public origin
		ScheduledAt:     p.ScheduledAt,
	}
	if p.ScheduledAt != nil {
		job.Status = "scheduled"
	}

	if err := h.jobs.Create(job); err != nil {
		h.logger.Error("failed to create job", "error", err)
		return nil, &statusError{http.StatusInternalServerError, "Failed to create job"}
	}

	// Create job items
	items := make([]models.SendJobItem, len(recipients), len(recipients)+len(unsubscribed))
	nextServer := h.serverPicker(p.Strategy, p.Servers)
	nextVariant := weightedPicker(variants)

	for i, recipient := range recipients {
		items[i] = models.SendJobItem{
			JobID:       job.ID,
			RecipientID: recipient.ID,
			VariantID:   variants[nextVariant()].ID,
			ServerName:  p.Servers[nextServer()],
		}
	}

	// Unsubscribed recipients get skipped items, so skips are counted per
	// recipient like everything else (and campaign stats can count them once)
	for _, recipient := range unsubscribed {
		items = append(items, models.SendJobItem{
			JobID:       job.ID,
			RecipientID: recipient.ID,
			VariantID:   variants[0].ID,
			ServerName:  p.Servers[0],
			Status:      models.JobItemStatusSkipped,
			Error:       models.SkipReasonUnsubscribed,
		})
	}

	if err := h.jobs.CreateItems(items); err != nil {
		h.logger.Error("failed to create job items", "error", err)
		return nil, &statusError{http.StatusInternalServerError, "Failed to create job items"}
	}

	if len(unsubscribed) > 0 {
		if err := h.jobs.SetSkippedUnsubscribed(job.ID, len(unsubscribed)); err != nil {
			h.logger.Error("failed to store skipped count", "job_id", job.ID, "error", err)
		}
	}

	// Start job if not scheduled
	if job.ScheduledAt == nil {
		// the background worker picks up running jobs
		h.jobs.UpdateStatus(job.ID, "running")
	}

	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r),
		"send", "campaign", c.ID, `{"job_id":"`+job.ID+`","recipients":`+strconv.Itoa(len(recipients))+`}`)
	return job, nil
}

func (h *Handlers) CampaignJobs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	// Get jobs for this campaign
	jobs, _, _ := h.jobs.List(models.JobListFilter{
		CampaignID: id,
		Limit:      50,
	})

	data := map[string]any{
		"Title":    c.Name + " - Runs",
		"Active":   "campaigns",
		"User":     h.getUserFromContext(r),
		"Campaign": c,
		"Jobs":     h.jobsWithStats(jobs),
	}

	h.render(w, "campaign_jobs", data)
}

// jobRecipients returns the active recipients of a list to send a job to, in
// list order, at most limit of them (0 = all), and the recipients unsubscribed
// in the list that were passed over to get them. With a dry-run limit only the
// unsubscribed recipients met before the limit is reached are returned.
func (h *Handlers) jobRecipients(listID string, limit int) ([]models.Recipient, []models.Recipient, error) {
	rows, err := h.recipients.StreamRecipients(listID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	recipients := []models.Recipient{}
	var unsubscribed []models.Recipient
	for rows.Next() {
		if limit > 0 && len(recipients) >= limit {
			break
		}
		rec, err := h.recipients.ScanRecipient(rows)
		if err != nil {
			return nil, nil, err
		}
		switch models.SkipReason(rec.Status) {
		case "":
			recipients = append(recipients, *rec)
		case models.SkipReasonUnsubscribed:
			unsubscribed = append(unsubscribed, *rec)
		}
	}
	return recipients, unsubscribed, rows.Err()
}

// Server distribution strategies of the campaign send form
const (
	jobStrategyRoundRobin = "round-robin"
	jobStrategyRandom     = "random"
	jobStrategyWeighted   = "weighted"
)

// serverPicker returns a function that yields server indexes for the job
// items: plain rotation, uniformly random, or in proportion to the
// sendry.multi_send.weights of the config.
func (h *Handlers) serverPicker(strategy string, servers []string) func() int {
	switch strategy {
	case jobStrategyRandom:
		return func() int { return rand.IntN(len(servers)) }
	case jobStrategyWeighted:
		weights := make([]int, len(servers))
		for i, name := range servers {
			weights[i] = h.cfg.Sendry.MultiSend.Weights[name]
		}
		return smoothWeightedPicker(weights)
	default:
		return smoothWeightedPicker(make([]int, len(servers)))
	}
}

// weightedPicker returns a function that yields variant indexes in proportion
// to the variant weights (see smoothWeightedPicker).
func weightedPicker(variants []models.CampaignVariant) func() int {
	weights := make([]int, len(variants))
	for i, v := range variants {
		weights[i] = v.Weight
	}
	return smoothWeightedPicker(weights)
}

// smoothWeightedPicker returns a function that yields indexes in proportion
// to the weights, deterministically (smooth weighted round-robin):
// weights 3:1 give 0,0,1,0, 0,0,1,0, ...; equal weights give plain rotation.
// An index with weight <= 0 is never picked, unless no weight is positive,
// then all are rotated equally.
func smoothWeightedPicker(in []int) func() int {
	weights := make([]int, len(in))
	total := 0
	for i, w := range in {
		if w > 0 {
			weights[i] = w
			total += w
		}
	}
	if total == 0 {
		for i := range weights {
			weights[i] = 1
		}
		total = len(weights)
	}

	current := make([]int, len(weights))
	return func() int {
		best := 0
		for i, w := range weights {
			current[i] += w
			if current[i] > current[best] {
				best = i
			}
		}
		current[best] -= total
		return best
	}
}

// parseScheduledAt parses the datetime-local value of the send form, entered
// in the browser's time zone: the IANA name tz, or else offset, the minutes
// of JS getTimezoneOffset (UTC minus local time) at that date; UTC when
// neither is usable. The result is in UTC, as scheduled jobs are compared
// with the database's UTC clock.
func parseScheduledAt(value, tz, offset string) (time.Time, error) {
	loc := time.UTC
	if l, err := time.LoadLocation(tz); tz != "" && tz != "Local" && err == nil {
		loc = l
	} else if m, err := strconv.Atoi(offset); err == nil && m > -16*60 && m < 16*60 {
		loc = time.FixedZone("", -m*60)
	}
	t, err := time.ParseInLocation("2006-01-02T15:04", value, loc)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
