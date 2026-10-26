package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
)

// JobSchedule changes a scheduled launch (POST /jobs/{id}/schedule): list,
// servers, strategy and time, from the same form as the send. The items are
// built again, so the list is read now. An empty scheduled_at starts the
// launch right away.
func (h *Handlers) JobSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	job, err := h.jobs.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get job", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load job")
		return
	}
	if job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}
	if job.Status != "scheduled" {
		h.error(w, http.StatusBadRequest, "Only a scheduled job can be changed, status: "+job.Status)
		return
	}

	params, perr := parseJobParams(r)
	if perr != nil {
		h.error(w, perr.status, perr.message)
		return
	}
	// the dry-run settings are fixed when the launch is created
	params.DryRun = job.DryRun
	params.DryRunLimit = job.DryRunLimit

	variants, err := h.campaigns.GetVariants(job.CampaignID)
	variants = activeVariants(variants)
	if err != nil || len(variants) == 0 {
		h.error(w, http.StatusBadRequest, "Campaign has no variants configured")
		return
	}

	items, recipients, unsubscribed, perr := h.buildJobItems(variants, params)
	if perr != nil {
		h.error(w, perr.status, perr.message)
		return
	}
	for i := range items {
		items[i].JobID = id
	}

	serversJSON, _ := json.Marshal(params.Servers)
	err = h.jobs.RescheduleJob(id, params.RecipientListID, string(serversJSON), params.Strategy, params.ScheduledAt, items, unsubscribed)
	if errors.Is(err, repository.ErrJobNotScheduled) {
		h.error(w, http.StatusBadRequest, "The launch has already started or was cancelled")
		return
	}
	if err != nil {
		h.logger.Error("failed to reschedule job", "job_id", id, "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update job")
		return
	}

	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r),
		"reschedule", "campaign", job.CampaignID, `{"job_id":"`+id+`","recipients":`+strconv.Itoa(recipients)+`}`)
	http.Redirect(w, r, "/campaigns/"+job.CampaignID, http.StatusSeeOther)
}

// scheduledJobs returns the scheduled launches of a campaign with the number
// of recipients each will be sent to.
func (h *Handlers) scheduledJobs(campaignID string) []map[string]any {
	jobs, _, err := h.jobs.List(models.JobListFilter{CampaignID: campaignID, Status: "scheduled", Limit: 50})
	if err != nil {
		h.logger.Error("failed to list scheduled jobs", "error", err)
		return nil
	}
	out := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		stats, _ := h.jobs.GetStats(job.ID)
		out = append(out, map[string]any{"Job": job, "Recipients": stats.Pending})
	}
	return out
}

// localReturnPath returns the form's return value when it is a path inside
// the campaigns section of the panel, otherwise fallback. Only plain local
// paths pass: no scheme, host, backslash or protocol-relative "//".
func localReturnPath(value, fallback string) string {
	if !strings.HasPrefix(value, "/campaigns/") || strings.ContainsAny(value, "\\\r\n") {
		return fallback
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/campaigns/") {
		return fallback
	}
	return value
}
