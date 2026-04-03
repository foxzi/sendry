package handlers

import (
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
)

func (h *Handlers) BlockList(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	category := r.URL.Query().Get("category")
	pageStr := r.URL.Query().Get("page")

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	limit := 25
	offset := (page - 1) * limit

	blocks, total, err := h.blocks.List(models.BlockListFilter{
		Search:   search,
		Category: category,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		h.logger.Error("failed to list blocks", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to list blocks")
		return
	}

	categories, _ := h.blocks.GetCategories()

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	data := map[string]any{
		"Title":      "Blocks",
		"Active":     "blocks",
		"User":       h.getUserFromContext(r),
		"Blocks":     blocks,
		"Categories": categories,
		"Search":     search,
		"Category":   category,
		"Page":       page,
		"TotalPages": totalPages,
		"Total":      total,
	}

	h.render(w, "blocks", data)
}

func (h *Handlers) BlockNew(w http.ResponseWriter, r *http.Request) {
	categories, _ := h.blocks.GetCategories()

	data := map[string]any{
		"Title":      "New Block",
		"Active":     "blocks",
		"User":       h.getUserFromContext(r),
		"Categories": categories,
	}

	h.render(w, "block_form", data)
}

func (h *Handlers) BlockCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	name := r.FormValue("name")
	category := r.FormValue("category")
	html := r.FormValue("html")
	previewText := r.FormValue("preview_text")

	if name == "" || category == "" || html == "" {
		h.error(w, http.StatusBadRequest, "Name, category and HTML are required")
		return
	}

	b := &models.EmailBlock{
		Name:        name,
		Category:    category,
		HTML:        html,
		PreviewText: previewText,
	}

	if err := h.blocks.Create(b); err != nil {
		h.logger.Error("failed to create block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create block")
		return
	}

	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"create", "block", b.ID, auditJSON(map[string]any{"name": b.Name, "category": b.Category}))

	http.Redirect(w, r, "/blocks/"+b.ID, http.StatusSeeOther)
}

func (h *Handlers) BlockView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := h.blocks.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load block")
		return
	}
	if b == nil {
		h.error(w, http.StatusNotFound, "Block not found")
		return
	}

	data := map[string]any{
		"Title":  b.Name,
		"Active": "blocks",
		"User":   h.getUserFromContext(r),
		"Block":  b,
	}

	h.render(w, "block_view", data)
}

func (h *Handlers) BlockEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := h.blocks.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load block")
		return
	}
	if b == nil {
		h.error(w, http.StatusNotFound, "Block not found")
		return
	}

	categories, _ := h.blocks.GetCategories()

	data := map[string]any{
		"Title":      "Edit Block",
		"Active":     "blocks",
		"User":       h.getUserFromContext(r),
		"Block":      b,
		"Categories": categories,
		"IsEdit":     true,
	}

	h.render(w, "block_form", data)
}

func (h *Handlers) BlockUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	b, err := h.blocks.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load block")
		return
	}
	if b == nil {
		h.error(w, http.StatusNotFound, "Block not found")
		return
	}

	name := r.FormValue("name")
	category := r.FormValue("category")
	html := r.FormValue("html")
	previewText := r.FormValue("preview_text")

	if name == "" || category == "" || html == "" {
		h.error(w, http.StatusBadRequest, "Name, category and HTML are required")
		return
	}

	b.Name = name
	b.Category = category
	b.HTML = html
	b.PreviewText = previewText

	if err := h.blocks.Update(b); err != nil {
		h.logger.Error("failed to update block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update block")
		return
	}

	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"update", "block", b.ID, auditJSON(map[string]any{"name": b.Name}))

	http.Redirect(w, r, "/blocks/"+b.ID, http.StatusSeeOther)
}

func (h *Handlers) BlockDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	b, err := h.blocks.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load block")
		return
	}
	if b == nil {
		h.error(w, http.StatusNotFound, "Block not found")
		return
	}

	if err := h.blocks.Delete(id); err != nil {
		h.logger.Error("failed to delete block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete block")
		return
	}

	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"delete", "block", id, auditJSON(map[string]any{"name": b.Name}))

	http.Redirect(w, r, "/blocks", http.StatusSeeOther)
}
