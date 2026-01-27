package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/models"
)

func (h *Handlers) RecipientListList(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	filter := models.RecipientListFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	}

	lists, total, err := h.recipients.ListLists(filter)
	if err != nil {
		h.logger.Error("failed to list recipient lists", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load recipient lists")
		return
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Recipients",
		"Active":     "recipients",
		"User":       h.getUserFromContext(r),
		"Lists":      lists,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
	}

	h.render(w, "recipients", data)
}

func (h *Handlers) RecipientListNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":  "New Recipient List",
		"Active": "recipients",
		"User":   h.getUserFromContext(r),
	}

	h.render(w, "recipient_list_new", data)
}

func (h *Handlers) RecipientListCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	list := &models.RecipientList{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		SourceType:  "manual",
	}

	if list.Name == "" {
		h.error(w, http.StatusBadRequest, "Name is required")
		return
	}

	if err := h.recipients.CreateList(list); err != nil {
		h.logger.Error("failed to create recipient list", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create recipient list")
		return
	}

	http.Redirect(w, r, "/recipients/"+list.ID, http.StatusSeeOther)
}

func (h *Handlers) RecipientListView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	list, err := h.recipients.GetListByID(id)
	if err != nil {
		h.logger.Error("failed to get recipient list", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load recipient list")
		return
	}
	if list == nil {
		h.error(w, http.StatusNotFound, "Recipient list not found")
		return
	}

	// Get recipients with pagination
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	tag := r.URL.Query().Get("tag")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	filter := models.RecipientFilter{
		ListID: id,
		Search: search,
		Status: status,
		Tag:    tag,
		Limit:  limit,
		Offset: offset,
	}

	recipients, total, err := h.recipients.ListRecipients(filter)
	if err != nil {
		h.logger.Error("failed to list recipients", "error", err)
	}

	tags, _ := h.recipients.GetTags(id)

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      list.Name,
		"Active":     "recipients",
		"User":       h.getUserFromContext(r),
		"List":       list,
		"Recipients": recipients,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
		"Status":     status,
		"Tag":        tag,
		"Tags":       tags,
	}

	h.render(w, "recipient_list_view", data)
}

func (h *Handlers) RecipientListEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	list, err := h.recipients.GetListByID(id)
	if err != nil {
		h.logger.Error("failed to get recipient list", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load recipient list")
		return
	}
	if list == nil {
		h.error(w, http.StatusNotFound, "Recipient list not found")
		return
	}

	data := map[string]any{
		"Title":  "Edit " + list.Name,
		"Active": "recipients",
		"User":   h.getUserFromContext(r),
		"List":   list,
	}

	h.render(w, "recipient_list_edit", data)
}

func (h *Handlers) RecipientListUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	list, err := h.recipients.GetListByID(id)
	if err != nil || list == nil {
		h.error(w, http.StatusNotFound, "Recipient list not found")
		return
	}

	list.Name = r.FormValue("name")
	list.Description = r.FormValue("description")

	if err := h.recipients.UpdateList(list); err != nil {
		h.logger.Error("failed to update recipient list", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update recipient list")
		return
	}

	http.Redirect(w, r, "/recipients/"+id, http.StatusSeeOther)
}

func (h *Handlers) RecipientListDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.recipients.DeleteList(id); err != nil {
		h.logger.Error("failed to delete recipient list", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete recipient list")
		return
	}

	http.Redirect(w, r, "/recipients", http.StatusSeeOther)
}

func (h *Handlers) RecipientImportPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	list, err := h.recipients.GetListByID(id)
	if err != nil || list == nil {
		h.error(w, http.StatusNotFound, "Recipient list not found")
		return
	}

	data := map[string]any{
		"Title":  "Import to " + list.Name,
		"Active": "recipients",
		"User":   h.getUserFromContext(r),
		"List":   list,
	}

	h.render(w, "recipient_import", data)
}

func (h *Handlers) RecipientImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	list, err := h.recipients.GetListByID(id)
	if err != nil || list == nil {
		h.error(w, http.StatusNotFound, "Recipient list not found")
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.error(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		h.error(w, http.StatusBadRequest, "No file uploaded")
		return
	}
	defer file.Close()

	// Update source type to CSV
	list.SourceType = "csv"
	h.recipients.UpdateList(list)

	result, err := h.recipients.ImportCSV(id, file)
	if err != nil {
		h.logger.Error("failed to import CSV", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to import: "+err.Error())
		return
	}

	data := map[string]any{
		"Title":  "Import Results",
		"Active": "recipients",
		"User":   h.getUserFromContext(r),
		"List":   list,
		"Result": result,
	}

	h.render(w, "recipient_import_result", data)
}

func (h *Handlers) RecipientsList(w http.ResponseWriter, r *http.Request) {
	// This endpoint returns just the recipients table for HTMX updates
	id := r.PathValue("id")

	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	tag := r.URL.Query().Get("tag")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	filter := models.RecipientFilter{
		ListID: id,
		Search: search,
		Status: status,
		Tag:    tag,
		Limit:  limit,
		Offset: offset,
	}

	recipients, total, err := h.recipients.ListRecipients(filter)
	if err != nil {
		h.logger.Error("failed to list recipients", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load recipients")
		return
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"ListID":     id,
		"Recipients": recipients,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
		"Status":     status,
		"Tag":        tag,
	}

	h.views.RenderPartial(w, "recipient_table", data)
}

// RecipientAdd handles adding a single recipient
func (h *Handlers) RecipientAdd(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	recipient := &models.Recipient{
		ListID:    id,
		Email:     r.FormValue("email"),
		Name:      r.FormValue("name"),
		Variables: r.FormValue("variables"),
		Tags:      r.FormValue("tags"),
		Status:    "active",
	}

	if recipient.Email == "" {
		h.error(w, http.StatusBadRequest, "Email is required")
		return
	}

	if err := h.recipients.AddRecipient(recipient); err != nil {
		h.logger.Error("failed to add recipient", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to add recipient")
		return
	}

	http.Redirect(w, r, "/recipients/"+id, http.StatusSeeOther)
}

// RecipientDelete handles deleting a single recipient
func (h *Handlers) RecipientDelete(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	recipientID := r.PathValue("recipientId")

	if err := h.recipients.DeleteRecipient(recipientID, listID); err != nil {
		h.logger.Error("failed to delete recipient", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete recipient")
		return
	}

	http.Redirect(w, r, "/recipients/"+listID, http.StatusSeeOther)
}

// RecipientListExport exports recipients to CSV using streaming to avoid memory issues
func (h *Handlers) RecipientListExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	list, err := h.recipients.GetListByID(id)
	if err != nil || list == nil {
		h.error(w, http.StatusNotFound, "Recipient list not found")
		return
	}

	// Stream recipients directly from database to avoid loading all into memory
	rows, err := h.recipients.StreamRecipients(id)
	if err != nil {
		h.logger.Error("failed to stream recipients for export", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to export recipients")
		return
	}
	defer rows.Close()

	// Set headers for CSV download
	filename := fmt.Sprintf("%s-recipients.csv", sanitizeFilename(list.Name))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{"email", "name", "status", "variables", "tags"}
	if err := writer.Write(header); err != nil {
		h.logger.Error("failed to write CSV header", "error", err)
		return
	}

	// Stream and write recipients one by one
	count := 0
	for rows.Next() {
		rec, err := h.recipients.ScanRecipient(rows)
		if err != nil {
			h.logger.Error("failed to scan recipient", "error", err)
			continue
		}

		row := []string{
			rec.Email,
			rec.Name,
			rec.Status,
			rec.Variables,
			rec.Tags,
		}
		if err := writer.Write(row); err != nil {
			h.logger.Error("failed to write CSV row", "error", err)
			return
		}
		count++
	}

	if err := rows.Err(); err != nil {
		h.logger.Error("error iterating recipients", "error", err)
	}

	h.logger.Info("recipients exported", "list_id", id, "count", count)
}
