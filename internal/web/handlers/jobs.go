package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/stats"
)

func (h *Handlers) JobList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	filter := models.JobListFilter{
		Status: status,
		Limit:  limit,
		Offset: offset,
	}

	jobs, total, err := h.jobs.List(filter)
	if err != nil {
		h.logger.Error("failed to list jobs", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load jobs")
		return
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Runs",
		"Active":     "jobs",
		"User":       h.getUserFromContext(r),
		"Jobs":       h.jobsWithStats(jobs),
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Status":     status,
	}

	h.render(w, "jobs", data)
}

// jobsWithStats builds template rows (Job, Stats, Progress) for a list of jobs.
func (h *Handlers) jobsWithStats(jobs []models.SendJob) []map[string]any {
	ids := make([]string, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	all, err := h.jobs.GetStatsBatch(ids)
	if err != nil {
		h.logger.Error("failed to get job stats", "error", err)
	}
	rows := make([]map[string]any, len(jobs))
	for i, job := range jobs {
		stats := all[job.ID]
		rows[i] = map[string]any{
			"Job":      job,
			"Stats":    stats,
			"Progress": stats.Progress(),
		}
	}
	return rows
}

func (h *Handlers) JobView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

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

	stats, _ := h.jobs.GetStats(id)

	errorCounts, err := h.jobs.ErrorCounts(id)
	if err != nil {
		h.logger.Error("failed to count job errors", "error", err)
	}

	// Get recent items
	items, _, _ := h.jobs.ListItems(models.JobItemFilter{
		JobID: id,
		Limit: 50,
	})

	trackingStats, statsErr := h.tracking.JobStats(id)
	if statsErr != nil {
		h.logger.Error("failed to load tracking stats", "error", statsErr)
	}
	campaign, err := h.campaigns.GetByID(job.CampaignID)
	if err != nil {
		h.logger.Error("failed to load job campaign", "error", err)
	}

	// Parse servers from JSON
	var servers []string
	json.Unmarshal([]byte(job.Servers), &servers)

	data := map[string]any{
		"Title":    "Job: " + job.ID[:8],
		"Active":   "jobs",
		"User":     h.getUserFromContext(r),
		"Job":      job,
		"Stats":    stats,
		"Progress": stats.Progress(),
		"Items":    items,
		"Servers":  servers,
		"Errors":   summarizeErrors(errorCounts),
		"Tracking": trackingCards(campaign, trackingStats, statsErr),
	}
	if n, _ := strconv.Atoi(r.URL.Query().Get("retried")); n > 0 {
		data["Retried"] = n
	}
	// Without the public origin the worker sends with no unsubscribe links and
	// no tracking, so warn while the run still has mail to send
	if stats.Pending > 0 && (job.Status == "running" || job.Status == "scheduled" || job.Status == "paused") {
		if origin, err := h.publicOrigin(); err == nil && origin == "" {
			data["NoPublicURL"] = true
		}
	}

	h.render(w, "job_view", data)
}

// trackingCards returns the stats for the engagement cards of a job or
// campaign page, or nil to hide them: on a load error (statsErr), or when the
// campaign tracks neither opens nor clicks and nobody unsubscribed or was
// skipped as unsubscribed (unsubscribe links are always on).
func trackingCards(c *models.Campaign, st models.TrackingStats, statsErr error) *models.TrackingStats {
	if statsErr != nil {
		return nil
	}
	if c != nil && !c.TrackOpens && !c.TrackClicks && st.Unsubscribed == 0 && st.SkippedUnsub == 0 {
		return nil
	}
	return &st
}

func (h *Handlers) JobItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status := r.URL.Query().Get("status")
	errorType := stats.ParseErrorType(r.URL.Query().Get("error_type"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 100
	offset := (page - 1) * limit

	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}

	opened := yesNo(r.URL.Query().Get("opened"))
	clicked := yesNo(r.URL.Query().Get("clicked"))

	filter := models.JobItemFilter{
		JobID:     id,
		Status:    status,
		ErrorType: errorType,
		Opened:    opened,
		Clicked:   clicked,
		Limit:     limit,
		Offset:    offset,
	}

	items, total, err := h.jobs.ListItems(filter)
	if err != nil {
		h.logger.Error("failed to list items", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load items")
		return
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Job Items",
		"Active":     "jobs",
		"User":       h.getUserFromContext(r),
		"Job":        job,
		"Items":      items,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Status":     status,
		"ErrorType":  errorType,
		"ErrorTypes": stats.ErrorTypes,
		"Opened":     opened,
		"Clicked":    clicked,
	}

	h.render(w, "job_items", data)
}

func (h *Handlers) JobPause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}

	if job.Status != "running" {
		h.error(w, http.StatusBadRequest, "Can only pause running jobs")
		return
	}

	if err := h.jobs.UpdateStatus(id, "paused"); err != nil {
		h.logger.Error("failed to pause job", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to pause job")
		return
	}

	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

func (h *Handlers) JobResume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}

	if job.Status != "paused" {
		h.error(w, http.StatusBadRequest, "Can only resume paused jobs")
		return
	}

	if err := h.jobs.UpdateStatus(id, "running"); err != nil {
		h.logger.Error("failed to resume job", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to resume job")
		return
	}

	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

func (h *Handlers) JobCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}

	if job.Status != "running" && job.Status != "paused" && job.Status != "scheduled" {
		h.error(w, http.StatusBadRequest, "Cannot cancel job in status: "+job.Status)
		return
	}

	if err := h.jobs.UpdateStatus(id, "cancelled"); err != nil {
		h.logger.Error("failed to cancel job", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to cancel job")
		return
	}

	// the campaign page's cancel button returns there
	http.Redirect(w, r, localReturnPath(r.FormValue("return"), "/jobs/"+id), http.StatusSeeOther)
}

func (h *Handlers) JobRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}

	if job.Status != "completed" && job.Status != "failed" {
		h.error(w, http.StatusBadRequest, "Can only retry completed or failed jobs")
		return
	}

	n, err := h.jobs.RetryFailedItems(id)
	if err != nil {
		h.logger.Error("failed to reset failed items", "job_id", id, "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to retry job")
		return
	}
	if n == 0 {
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
		return
	}

	if err := h.jobs.UpdateStatus(id, "running"); err != nil {
		h.logger.Error("failed to retry job", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to retry job")
		return
	}

	http.Redirect(w, r, "/jobs/"+id+"?retried="+strconv.FormatInt(n, 10), http.StatusSeeOther)
}

// yesNo normalises an engagement filter value to "yes", "no" or "".
func yesNo(v string) string {
	if v == "yes" || v == "no" {
		return v
	}
	return ""
}
