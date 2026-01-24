package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/models"
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

	// Get stats for each job
	jobsWithStats := make([]map[string]any, len(jobs))
	for i, job := range jobs {
		stats, _ := h.jobs.GetStats(job.ID)
		progress := 0
		if stats.Total > 0 {
			progress = (stats.Sent + stats.Failed) * 100 / stats.Total
		}
		jobsWithStats[i] = map[string]any{
			"Job":      job,
			"Stats":    stats,
			"Progress": progress,
		}
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Jobs",
		"Active":     "jobs",
		"User":       h.getUserFromContext(r),
		"Jobs":       jobsWithStats,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Status":     status,
	}

	h.render(w, "jobs", data)
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
	progress := 0
	if stats.Total > 0 {
		progress = (stats.Sent + stats.Failed) * 100 / stats.Total
	}

	// Get recent items
	items, _, _ := h.jobs.ListItems(models.JobItemFilter{
		JobID: id,
		Limit: 50,
	})

	// Parse servers from JSON
	var servers []string
	json.Unmarshal([]byte(job.Servers), &servers)

	data := map[string]any{
		"Title":    "Job: " + job.ID[:8],
		"Active":   "jobs",
		"User":     h.getUserFromContext(r),
		"Job":      job,
		"Stats":    stats,
		"Progress": progress,
		"Items":    items,
		"Servers":  servers,
	}

	h.render(w, "job_view", data)
}

func (h *Handlers) JobItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status := r.URL.Query().Get("status")
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

	filter := models.JobItemFilter{
		JobID:  id,
		Status: status,
		Limit:  limit,
		Offset: offset,
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

	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

func (h *Handlers) JobRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}

	// TODO: Implement retry logic - reset failed items to pending
	// For now, just update status back to running

	if err := h.jobs.UpdateStatus(id, "running"); err != nil {
		h.logger.Error("failed to retry job", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to retry job")
		return
	}

	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}
