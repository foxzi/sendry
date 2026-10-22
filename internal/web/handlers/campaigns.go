package handlers

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"regexp"
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

func (h *Handlers) CampaignNew(w http.ResponseWriter, r *http.Request) {
	// Tracking checkboxes default to the panel-wide tracking settings
	trackOpens, trackClicks := false, false
	if ts, err := h.settings.GetTrackingSettings(); err != nil {
		h.logger.Error("failed to load tracking settings", "error", err)
	} else if ts != nil {
		trackOpens, trackClicks = ts.OpensDefault, ts.ClicksDefault
	}

	data := map[string]any{
		"Title":       "New Campaign",
		"Active":      "campaigns",
		"User":        h.getUserFromContext(r),
		"TrackOpens":  trackOpens,
		"TrackClicks": trackClicks,
	}

	h.render(w, "campaign_new", data)
}

func (h *Handlers) CampaignCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	c := &models.Campaign{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		FromEmail:   r.FormValue("from_email"),
		FromName:    r.FormValue("from_name"),
		ReplyTo:     r.FormValue("reply_to"),
		Variables:   r.FormValue("variables"),
		Tags:        r.FormValue("tags"),
		TrackOpens:  r.FormValue("track_opens") != "",
		TrackClicks: r.FormValue("track_clicks") != "",
		AddUTM:      r.FormValue("add_utm") != "",
	}

	if c.Name == "" || c.FromEmail == "" {
		h.error(w, http.StatusBadRequest, "Name and From Email are required")
		return
	}

	if err := h.campaigns.Create(c); err != nil {
		h.logger.Error("failed to create campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create campaign")
		return
	}

	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r),
		"create", "campaign", c.ID, `{"name":"`+c.Name+`"}`)
	http.Redirect(w, r, "/campaigns/"+c.ID, http.StatusSeeOther)
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

	variants, err := h.campaigns.GetVariants(id)
	if err != nil {
		h.logger.Error("failed to get variants", "error", err)
	}

	// Get recipient lists for send page
	recipientLists, _, _ := h.recipients.ListLists(models.RecipientListFilter{Limit: 100})

	trackingStats, err := h.tracking.CampaignStats(id)
	if err != nil {
		h.logger.Error("failed to load campaign tracking stats", "error", err)
	}

	data := map[string]any{
		"Title":          c.Name,
		"Active":         "campaigns",
		"User":           h.getUserFromContext(r),
		"Tracking":       trackingCards(c, trackingStats, err),
		"Campaign":       c,
		"Variants":       variants,
		"RecipientLists": recipientLists,
		"Servers":        h.cfg.Sendry.Servers,
	}

	h.render(w, "campaign_view", data)
}

func (h *Handlers) CampaignEdit(w http.ResponseWriter, r *http.Request) {
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

	data := map[string]any{
		"Title":    "Edit " + c.Name,
		"Active":   "campaigns",
		"User":     h.getUserFromContext(r),
		"Campaign": c,
	}

	h.render(w, "campaign_edit", data)
}

func (h *Handlers) CampaignUpdate(w http.ResponseWriter, r *http.Request) {
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

	c.Name = r.FormValue("name")
	c.Description = r.FormValue("description")
	c.FromEmail = r.FormValue("from_email")
	c.FromName = r.FormValue("from_name")
	c.ReplyTo = r.FormValue("reply_to")
	c.Tags = r.FormValue("tags")
	c.TrackOpens = r.FormValue("track_opens") != ""
	c.TrackClicks = r.FormValue("track_clicks") != ""
	c.AddUTM = r.FormValue("add_utm") != ""

	if err := h.campaigns.Update(c); err != nil {
		h.logger.Error("failed to update campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update campaign")
		return
	}

	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r),
		"update", "campaign", id, `{"name":"`+c.Name+`"}`)
	http.Redirect(w, r, "/campaigns/"+id, http.StatusSeeOther)
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

func (h *Handlers) CampaignVariables(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	data := map[string]any{
		"Title":    c.Name + " - Variables",
		"Active":   "campaigns",
		"User":     h.getUserFromContext(r),
		"Campaign": c,
	}

	h.render(w, "campaign_variables", data)
}

func (h *Handlers) CampaignVariablesUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	variables := r.FormValue("variables")

	if err := h.campaigns.UpdateVariables(id, variables); err != nil {
		h.logger.Error("failed to update variables", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update variables")
		return
	}

	http.Redirect(w, r, "/campaigns/"+id, http.StatusSeeOther)
}

func (h *Handlers) CampaignVariants(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	variants, err := h.campaigns.GetVariants(id)
	if err != nil {
		h.logger.Error("failed to get variants", "error", err)
	}

	// Get templates for selection
	templateList, _, _ := h.templates.List(models.TemplateListFilter{Limit: 100})

	data := map[string]any{
		"Title":     c.Name + " - Variants",
		"Active":    "campaigns",
		"User":      h.getUserFromContext(r),
		"Campaign":  c,
		"Variants":  variants,
		"Templates": templateList,
	}

	h.render(w, "campaign_variants", data)
}

func (h *Handlers) CampaignVariantCreate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	weight, _ := strconv.Atoi(r.FormValue("weight"))
	if weight <= 0 {
		weight = 100
	}

	v := &models.CampaignVariant{
		CampaignID:      id,
		Name:            r.FormValue("name"),
		TemplateID:      r.FormValue("template_id"),
		SubjectOverride: r.FormValue("subject_override"),
		Weight:          weight,
	}

	if v.Name == "" || v.TemplateID == "" {
		h.error(w, http.StatusBadRequest, "Name and template are required")
		return
	}

	if err := h.campaigns.AddVariant(v); err != nil {
		h.logger.Error("failed to add variant", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to add variant")
		return
	}

	http.Redirect(w, r, "/campaigns/"+id+"/variants", http.StatusSeeOther)
}

func (h *Handlers) CampaignVariantDelete(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("id")
	variantID := r.PathValue("variantId")

	if err := h.campaigns.DeleteVariant(variantID); err != nil {
		h.logger.Error("failed to delete variant", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete variant")
		return
	}

	http.Redirect(w, r, "/campaigns/"+campaignID+"/variants", http.StatusSeeOther)
}

// unsubscribeVarRE matches the {{unsubscribe_url}} template variable
var unsubscribeVarRE = regexp.MustCompile(`\{\{-?\s*\.?unsubscribe_url\s*-?\}\}`)

// publicOrigin returns the public origin of unsubscribe and tracking links:
// the tracking base URL setting or server.public_url; "" if neither is set.
func (h *Handlers) publicOrigin() (string, error) {
	ts, err := h.settings.GetTrackingSettings()
	if err != nil {
		return "", err
	}
	return tracking.ResolveBaseURL(ts.BaseURL, h.cfg.Server.PublicURL), nil
}

// campaignSendData returns the data of the campaign send page
func (h *Handlers) campaignSendData(r *http.Request, c *models.Campaign) map[string]any {
	variants, _ := h.campaigns.GetVariants(c.ID)
	recipientLists, _, _ := h.recipients.ListLists(models.RecipientListFilter{Limit: 100})

	// Warn when no variant template contains the unsubscribe link variable
	hasUnsubscribeLink := false
	checked := map[string]bool{}
	for _, v := range variants {
		if checked[v.TemplateID] {
			continue
		}
		checked[v.TemplateID] = true
		tmpl, err := h.templates.GetByID(v.TemplateID)
		if err != nil || tmpl == nil {
			continue
		}
		if unsubscribeVarRE.MatchString(tmpl.HTML) || unsubscribeVarRE.MatchString(tmpl.Text) {
			hasUnsubscribeLink = true
			break
		}
	}

	// Campaign mail cannot be sent without a public origin for unsubscribe links
	origin, err := h.publicOrigin()
	if err != nil {
		h.logger.Error("failed to load tracking settings", "error", err)
	}

	return map[string]any{
		"NoUnsubscribeLink": len(variants) > 0 && !hasUnsubscribeLink,
		"NoPublicURL":       err == nil && origin == "",
		"Title":             "Send " + c.Name,
		"Active":            "campaigns",
		"User":              h.getUserFromContext(r),
		"Campaign":          c,
		"Variants":          variants,
		"RecipientLists":    recipientLists,
		"Servers":           h.cfg.Sendry.Servers,
	}
}

func (h *Handlers) CampaignSendPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	h.render(w, "campaign_send", h.campaignSendData(r, c))
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

	// Every campaign message carries an unsubscribe link (and List-Unsubscribe
	// headers) built from the public origin: without it the job must not start
	origin, err := h.publicOrigin()
	if err != nil {
		h.logger.Error("failed to load tracking settings", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load tracking settings")
		return
	}
	if origin == "" {
		data := h.campaignSendData(r, c)
		data["Error"] = "Cannot send: the public URL for unsubscribe links is not configured. " +
			"Set the tracking domain in Settings > Tracking or server.public_url in the config."
		h.render(w, "campaign_send", data)
		return
	}

	recipientListID := r.FormValue("recipient_list_id")
	if recipientListID == "" {
		h.error(w, http.StatusBadRequest, "Recipient list is required")
		return
	}

	servers := r.Form["servers"]
	if len(servers) == 0 {
		h.error(w, http.StatusBadRequest, "At least one server is required")
		return
	}

	strategy := r.FormValue("strategy")
	if strategy == "" {
		strategy = jobStrategyRoundRobin
	}
	if strategy != jobStrategyRoundRobin && strategy != jobStrategyRandom && strategy != jobStrategyWeighted {
		h.error(w, http.StatusBadRequest, "Unknown distribution strategy")
		return
	}

	// Get variants
	variants, err := h.campaigns.GetVariants(id)
	if err != nil || len(variants) == 0 {
		h.error(w, http.StatusBadRequest, "Campaign has no variants configured")
		return
	}

	// Handle dry-run mode
	dryRun := r.FormValue("dry_run") == "on"
	dryRunLimit := 0
	if dryRun {
		dryRunLimit, _ = strconv.Atoi(r.FormValue("dry_run_limit"))
		if dryRunLimit <= 0 {
			dryRunLimit = 10 // default
		}
	}

	// Create servers JSON
	serversJSON, _ := json.Marshal(servers)

	// Create job
	job := &models.SendJob{
		CampaignID:      id,
		RecipientListID: recipientListID,
		Servers:         string(serversJSON),
		Strategy:        strategy,
		DryRun:          dryRun,
		DryRunLimit:     dryRunLimit,
	}

	// Handle scheduled_at
	if scheduledAt := r.FormValue("scheduled_at"); scheduledAt != "" {
		t, err := parseScheduledAt(scheduledAt, r.FormValue("tz"), r.FormValue("tz_offset"))
		if err == nil {
			job.ScheduledAt = &t
			job.Status = "scheduled"
		}
	}

	if err := h.jobs.Create(job); err != nil {
		h.logger.Error("failed to create job", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create job")
		return
	}

	// Get recipients: active ones up to the dry-run limit (all without it)
	recipients, unsubscribed, err := h.jobRecipients(recipientListID, dryRunLimit)
	if err != nil {
		h.logger.Error("failed to get recipients", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to get recipients")
		return
	}

	// Create job items
	items := make([]models.SendJobItem, len(recipients), len(recipients)+len(unsubscribed))
	nextServer := h.serverPicker(strategy, servers)
	nextVariant := weightedPicker(variants)

	for i, recipient := range recipients {
		items[i] = models.SendJobItem{
			JobID:       job.ID,
			RecipientID: recipient.ID,
			VariantID:   variants[nextVariant()].ID,
			ServerName:  servers[nextServer()],
		}
	}

	// Unsubscribed recipients get skipped items, so skips are counted per
	// recipient like everything else (and campaign stats can count them once)
	for _, recipient := range unsubscribed {
		items = append(items, models.SendJobItem{
			JobID:       job.ID,
			RecipientID: recipient.ID,
			VariantID:   variants[0].ID,
			ServerName:  servers[0],
			Status:      models.JobItemStatusSkipped,
			Error:       models.SkipReasonUnsubscribed,
		})
	}

	if err := h.jobs.CreateItems(items); err != nil {
		h.logger.Error("failed to create job items", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create job items")
		return
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
		"send", "campaign", id, `{"job_id":"`+job.ID+`","recipients":`+strconv.Itoa(len(recipients))+`}`)
	http.Redirect(w, r, "/jobs/"+job.ID, http.StatusSeeOther)
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

	// Get stats for each job
	jobsWithStats := make([]map[string]any, len(jobs))
	for i, job := range jobs {
		stats, _ := h.jobs.GetStats(job.ID)
		jobsWithStats[i] = map[string]any{
			"Job":      job,
			"Stats":    stats,
			"Progress": stats.Progress(),
		}
	}

	data := map[string]any{
		"Title":    c.Name + " - Jobs",
		"Active":   "campaigns",
		"User":     h.getUserFromContext(r),
		"Campaign": c,
		"Jobs":     jobsWithStats,
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
