package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	emailtpl "github.com/foxzi/sendry/internal/web/template"
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
	borderRadius := parseIntField(r.FormValue("border_radius"), 0, 0, 64)
	padV := parseIntField(r.FormValue("padding_v"), 0, 0, 200)
	padH := parseIntField(r.FormValue("padding_h"), 0, 0, 200)
	background := strings.TrimSpace(r.FormValue("background"))

	if name == "" || category == "" || html == "" {
		h.error(w, http.StatusBadRequest, "Name, category and HTML are required")
		return
	}

	b := &models.EmailBlock{
		Name:         name,
		Category:     category,
		HTML:         html,
		PreviewText:  previewText,
		BorderRadius: borderRadius,
		PaddingV:     padV,
		PaddingH:     padH,
		Background:   background,
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

	usedIn, err := h.templates.GetTemplatesByBlockID(id)
	if err != nil {
		h.logger.Error("failed to list templates using block", "block_id", id, "error", err)
	}

	sampleJSON, _ := json.MarshalIndent(buildSampleData(b.HTML), "", "  ")

	data := map[string]any{
		"Title":      b.Name,
		"Active":     "blocks",
		"User":       h.getUserFromContext(r),
		"Block":      b,
		"UsedIn":     usedIn,
		"SampleJSON": string(sampleJSON),
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

	sampleJSON, _ := json.MarshalIndent(buildSampleData(b.HTML), "", "  ")

	data := map[string]any{
		"Title":      "Edit Block",
		"Active":     "blocks",
		"User":       h.getUserFromContext(r),
		"Block":      b,
		"Categories": categories,
		"IsEdit":     true,
		"SampleJSON": string(sampleJSON),
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
	b.BorderRadius = parseIntField(r.FormValue("border_radius"), b.BorderRadius, 0, 64)
	b.PaddingV = parseIntField(r.FormValue("padding_v"), b.PaddingV, 0, 200)
	b.PaddingH = parseIntField(r.FormValue("padding_h"), b.PaddingH, 0, 200)
	if bg := strings.TrimSpace(r.FormValue("background")); bg != "" || r.FormValue("background_clear") == "1" {
		b.Background = bg
	}

	if err := h.blocks.Update(b); err != nil {
		h.logger.Error("failed to update block", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update block")
		return
	}

	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"update", "block", b.ID, auditJSON(map[string]any{"name": b.Name}))

	if rebuilt, err := h.rebuildTemplatesUsingBlock(b.ID, user["Email"].(string)); err != nil {
		h.logger.Error("auto-rebuild templates", "block_id", b.ID, "error", err)
	} else if rebuilt > 0 {
		h.logger.Info("auto-rebuilt templates after block change", "block_id", b.ID, "count", rebuilt)
	}

	http.Redirect(w, r, "/blocks/"+b.ID, http.StatusSeeOther)
}

func (h *Handlers) BlockInlineEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := h.blocks.GetByID(id)
	if err != nil {
		http.Error(w, "load failed", http.StatusInternalServerError)
		return
	}
	if b == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		HTML string `json:"html"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	newHTML := strings.TrimSpace(payload.HTML)
	if newHTML == "" {
		http.Error(w, "html cannot be empty", http.StatusBadRequest)
		return
	}
	oldOpen := strings.Count(b.HTML, "{{")
	oldClose := strings.Count(b.HTML, "}}")
	newOpen := strings.Count(newHTML, "{{")
	newClose := strings.Count(newHTML, "}}")
	if newOpen != oldOpen || newClose != oldClose || newOpen != newClose {
		http.Error(w, "template variables changed — refusing to save", http.StatusBadRequest)
		return
	}
	if err := emailtpl.ValidateHTML("inline-edit-validate", newHTML); err != nil {
		h.logger.Warn("inline edit rejected: template parse failed", "block_id", id, "error", err)
		http.Error(w, "edited HTML is not a valid template: "+err.Error(), http.StatusBadRequest)
		return
	}
	b.HTML = newHTML
	if err := h.blocks.Update(b); err != nil {
		h.logger.Error("inline edit", "block_id", id, "error", err)
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	user := h.getUserFromContext(r)
	email, _ := user["Email"].(string)
	h.settings.LogAction(r, middleware.GetUserID(r), email,
		"update", "block", b.ID, auditJSON(map[string]any{"inline_edit": true}))
	if rebuilt, err := h.rebuildTemplatesUsingBlock(b.ID, email); err != nil {
		h.logger.Error("auto-rebuild templates", "block_id", b.ID, "error", err)
	} else if rebuilt > 0 {
		h.logger.Info("auto-rebuilt templates after inline edit", "block_id", b.ID, "count", rebuilt)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *Handlers) BlockUpdateAppearance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := h.blocks.GetByID(id)
	if err != nil {
		http.Error(w, "load failed", http.StatusInternalServerError)
		return
	}
	if b == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		BorderRadius int    `json:"border_radius"`
		PaddingV     int    `json:"padding_v"`
		PaddingH     int    `json:"padding_h"`
		Background   string `json:"background"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	clamp := func(n, min, max int) int {
		if n < min {
			return min
		}
		if n > max {
			return max
		}
		return n
	}
	b.BorderRadius = clamp(payload.BorderRadius, 0, 64)
	b.PaddingV = clamp(payload.PaddingV, 0, 200)
	b.PaddingH = clamp(payload.PaddingH, 0, 200)
	b.Background = strings.TrimSpace(payload.Background)

	if err := h.blocks.Update(b); err != nil {
		h.logger.Error("update appearance", "block_id", id, "error", err)
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}

	user := h.getUserFromContext(r)
	email, _ := user["Email"].(string)
	h.settings.LogAction(r, middleware.GetUserID(r), email,
		"update", "block", b.ID, auditJSON(map[string]any{"appearance": true}))
	if rebuilt, err := h.rebuildTemplatesUsingBlock(b.ID, email); err != nil {
		h.logger.Error("auto-rebuild templates", "block_id", b.ID, "error", err)
	} else if rebuilt > 0 {
		h.logger.Info("auto-rebuilt templates after appearance change", "block_id", b.ID, "count", rebuilt)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":            true,
		"border_radius": b.BorderRadius,
		"padding_v":     b.PaddingV,
		"padding_h":     b.PaddingH,
		"background":    b.Background,
	})
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

	used, err := h.templates.GetTemplatesByBlockID(id)
	if err != nil {
		h.logger.Error("check block usage", "block_id", id, "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to check block usage")
		return
	}
	if len(used) > 0 {
		names := make([]string, 0, len(used))
		for _, t := range used {
			names = append(names, t.Name)
		}
		h.error(w, http.StatusConflict, "Block is used in templates: "+strings.Join(names, ", ")+". Remove it from those templates first.")
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

func (h *Handlers) BlockPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := h.blocks.GetByID(id)
	if err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to load block")
		return
	}
	if b == nil {
		h.error(w, http.StatusNotFound, "Block not found")
		return
	}

	var data map[string]any
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			if err := json.Unmarshal(body, &data); err != nil {
				http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	if data == nil {
		data = map[string]any{}
	}

	out, err := emailtpl.RenderHTML("block", b.HTML, data)
	if err != nil {
		http.Error(w, "Render error: "+err.Error(), http.StatusBadRequest)
		return
	}

	if wrapped, werr := ApplyWrapper(WrapBlockShell(out, b.BorderRadius, b.PaddingV, b.PaddingH, b.Background), WrapperOpts{Transparent: true}); werr == nil {
		out = strings.ReplaceAll(wrapped, "{{.Subject}}", "Preview")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(out))
}

func parseIntField(raw string, def, min, max int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < min || n > max {
		return def
	}
	return n
}
