package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/foxzi/sendry/internal/web/models"
)

// CampaignSegmentCount answers the launch form how many active recipients of
// a list match a segment (POST /campaigns/segment/count, JSON
// {"list_id": "...", "segment": {...}}): {"count": N}, or {"error": "..."}
// with 400 for a segment that does not validate.
func (h *Handlers) CampaignSegmentCount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ListID  string          `json:"list_id"`
		Segment json.RawMessage `json:"segment"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err := json.Unmarshal(raw, &body); err != nil {
		h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	seg, err := models.ParseSegment(string(body.Segment))
	if err != nil {
		h.jsonError(w, "Invalid segment: "+err.Error(), http.StatusBadRequest)
		return
	}
	list, err := h.recipients.GetListByID(body.ListID)
	if err != nil {
		h.logger.Error("failed to get recipient list", "error", err)
		h.jsonError(w, "Failed to load the list", http.StatusInternalServerError)
		return
	}
	if list == nil {
		h.jsonError(w, "Recipient list not found", http.StatusNotFound)
		return
	}
	n, err := h.recipients.CountSegment(list.ID, seg)
	if err != nil {
		h.logger.Error("failed to count segment", "list_id", list.ID, "error", err)
		h.jsonError(w, "Failed to count recipients", http.StatusInternalServerError)
		return
	}
	h.json(w, http.StatusOK, map[string]int{"count": n})
}

// RecipientSegmentFields returns what the segment conditions of a list can
// refer to (GET /recipients/{id}/segment-fields): the tags and the variable
// names its recipients have. They are suggestions for the launch form.
func (h *Handlers) RecipientSegmentFields(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tags, err := h.recipients.GetTags(id)
	if err != nil {
		h.logger.Error("failed to get list tags", "list_id", id, "error", err)
		tags = []string{}
	}
	vars, err := h.recipients.GetVariableKeys(id)
	if err != nil {
		h.logger.Error("failed to get list variables", "list_id", id, "error", err)
		vars = []string{}
	}
	h.json(w, http.StatusOK, map[string][]string{"tags": tags, "variables": vars})
}
