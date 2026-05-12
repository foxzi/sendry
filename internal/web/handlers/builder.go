package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/foxzi/sendry/internal/web/blocks"
	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	emailtpl "github.com/foxzi/sendry/internal/web/template"
)

func (h *Handlers) BuilderPage(w http.ResponseWriter, r *http.Request) {
	folders, _ := h.templates.GetFolders()

	wrapper, err := blocks.GetWrapper()
	if err != nil {
		h.logger.Error("failed to load wrapper", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load email wrapper")
		return
	}

	categories, err := h.blocks.ListGroupedByCategory()
	if err != nil {
		h.logger.Error("failed to load blocks", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load blocks")
		return
	}

	data := map[string]any{
		"Title":      "Email Builder",
		"Active":     "templates",
		"User":       h.getUserFromContext(r),
		"Folders":    folders,
		"Wrapper":    wrapper,
		"Categories": categories,
	}

	if id := r.PathValue("id"); id != "" {
		t, err := h.templates.GetByID(id)
		if err != nil || t == nil {
			h.error(w, http.StatusNotFound, "Template not found")
			return
		}
		refs, _ := h.templates.GetBlockRefs(id)
		recovered := false

		if len(refs) == 0 && t.HTML != "" {
			if rebuilt := h.detectBlocksInHTML(t.HTML); len(rebuilt) > 0 {
				refs = rebuilt
				recovered = true
			}
		}
		initial := make([]map[string]any, 0, len(refs))
		for _, r := range refs {
			initial = append(initial, map[string]any{
				"block_id":   r.BlockID,
				"gap_height": r.GapHeight,
				"gap_color":  r.GapColor,
				"condition":  r.Condition,
			})
		}
		data["Title"] = "Edit: " + t.Name
		data["Template"] = t
		data["InitialBlocks"] = initial
		data["LegacyRecovered"] = recovered
		data["LegacyEmpty"] = len(refs) == 0 && t.HTML != ""
	}

	h.render(w, "template_builder", data)
}

func (h *Handlers) BuilderUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	t, err := h.templates.GetByID(id)
	if err != nil || t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	name := r.FormValue("name")
	subject := r.FormValue("subject")
	description := r.FormValue("description")
	folder := r.FormValue("folder")
	html := r.FormValue("html")
	variables := r.FormValue("variables")
	if name == "" || subject == "" {
		h.error(w, http.StatusBadRequest, "Name and subject are required")
		return
	}
	if html == "" {
		h.error(w, http.StatusBadRequest, "Please add at least one block to the email")
		return
	}
	cs := parseContainerSettings(r)
	t.Name = name
	t.Description = description
	t.Subject = subject
	t.Variables = variables
	t.Folder = folder
	t.ContainerRadius = cs.Radius
	t.ContainerRadiusTop = cs.RadiusTop
	t.ContainerRadiusBottom = cs.RadiusBottom
	t.ContainerTransparent = cs.Transparent
	t.ContainerWidth = cs.Width
	t.ContainerPaddingV = cs.PaddingV
	t.ContainerPaddingH = cs.PaddingH
	t.PageBackground = cs.PageBG

	blockRefs := parseBlockRefs(r.FormValue("block_refs"))
	t.UseBlocks = len(blockRefs) > 0

	if len(blockRefs) > 0 {
		opts := WrapperOpts{
			Radius:       t.ContainerRadius,
			RadiusTop:    t.ContainerRadiusTop,
			RadiusBottom: t.ContainerRadiusBottom,
			Transparent:  t.ContainerTransparent,
			Width:        t.ContainerWidth,
			PaddingV:     t.ContainerPaddingV,
			PaddingH:     t.ContainerPaddingH,
			PageBG:       t.PageBackground,
		}
		if rebuilt, rerr := h.rebuildTemplateHTML(blockRefs, opts); rerr == nil && rebuilt != "" {
			html = rebuilt
		}
	}
	html = makeAbsoluteURLs(html, h.cfg.Server.PublicURL, h.cfg.Server.PublicUploadURL)
	text := stripHTMLTags(html)
	t.HTML = html
	t.Text = text

	user := h.getUserFromContext(r)
	if err := h.templates.Update(t, "Edited via builder", user["Email"].(string)); err != nil {
		h.logger.Error("failed to update template via builder", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save template")
		return
	}
	if err := h.templates.SetBlockRefs(t.ID, blockRefs); err != nil {
		h.logger.Error("failed to update block refs", "template_id", t.ID, "error", err)
	}

	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"update", "template", t.ID, auditJSON(map[string]any{"name": t.Name, "source": "builder"}))
	http.Redirect(w, r, "/templates/"+t.ID, http.StatusSeeOther)
}

func (h *Handlers) BuilderCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	name := r.FormValue("name")
	subject := r.FormValue("subject")
	description := r.FormValue("description")
	folder := r.FormValue("folder")
	html := r.FormValue("html")
	variables := r.FormValue("variables")

	if name == "" || subject == "" {
		h.error(w, http.StatusBadRequest, "Name and subject are required")
		return
	}

	if html == "" {
		h.error(w, http.StatusBadRequest, "Please add at least one block to the email")
		return
	}

	html = makeAbsoluteURLs(html, h.cfg.Server.PublicURL, h.cfg.Server.PublicUploadURL)
	text := stripHTMLTags(html)

	cs := parseContainerSettings(r)
	blockRefs := parseBlockRefs(r.FormValue("block_refs"))
	t := &models.Template{
		Name:                 name,
		Description:          description,
		Subject:              subject,
		HTML:                 html,
		Text:                 text,
		Variables:            variables,
		Folder:               folder,
		UseBlocks:            len(blockRefs) > 0,
		ContainerRadius:       cs.Radius,
		ContainerRadiusTop:    cs.RadiusTop,
		ContainerRadiusBottom: cs.RadiusBottom,
		ContainerTransparent: cs.Transparent,
		ContainerWidth:       cs.Width,
		ContainerPaddingV:    cs.PaddingV,
		ContainerPaddingH:    cs.PaddingH,
		PageBackground:       cs.PageBG,
	}

	user := h.getUserFromContext(r)
	if err := h.templates.Create(t, user["Email"].(string)); err != nil {
		h.logger.Error("failed to create template from builder", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create template")
		return
	}

	if len(blockRefs) > 0 {
		if err := h.templates.SetBlockRefs(t.ID, blockRefs); err != nil {
			h.logger.Error("failed to save block refs on create", "template_id", t.ID, "error", err)
		}
	}

	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"create", "template", t.ID, auditJSON(map[string]any{"name": t.Name, "source": "builder"}))
	http.Redirect(w, r, "/templates/"+t.ID, http.StatusSeeOther)
}

func makeAbsoluteURLs(html, publicURL, publicUploadURL string) string {
	if publicURL == "" && publicUploadURL == "" {
		return html
	}
	uploadsBase := strings.TrimRight(publicUploadURL, "/")
	if uploadsBase == "" {
		uploadsBase = strings.TrimRight(publicURL, "/")
	}
	if uploadsBase != "" {
		html = strings.ReplaceAll(html, `src="/uploads/`, `src="`+uploadsBase+`/uploads/`)
	}
	staticBase := strings.TrimRight(publicURL, "/")
	if staticBase != "" {
		html = strings.ReplaceAll(html, `src="/static/`, `src="`+staticBase+`/static/`)
	}
	return html
}

func stripHTMLTags(s string) string {
	s = removeTagBlock(s, "script")
	s = removeTagBlock(s, "style")

	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	result := html.UnescapeString(b.String())
	result = strings.ReplaceAll(result, " ", " ")

	lines := strings.Split(result, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, "\n")
}

func removeTagBlock(s, tagName string) string {
	lower := strings.ToLower(s)
	openTag := "<" + tagName
	closeTag := "</" + tagName

	for {
		start := strings.Index(lower, openTag)
		if start == -1 {
			break
		}
		afterOpen := start + len(openTag)
		if afterOpen < len(lower) && lower[afterOpen] != ' ' && lower[afterOpen] != '>' && lower[afterOpen] != '\t' && lower[afterOpen] != '\n' {
			lower = lower[:start] + strings.Repeat("x", len(openTag)) + lower[afterOpen:]
			continue
		}
		end := strings.Index(lower[start:], closeTag)
		if end == -1 {
			s = s[:start]
			break
		}
		endAbs := start + end
		closeEnd := strings.Index(lower[endAbs:], ">")
		if closeEnd == -1 {
			s = s[:start]
			break
		}
		cutEnd := endAbs + closeEnd + 1
		s = s[:start] + s[cutEnd:]
		lower = strings.ToLower(s)
	}
	return s
}

type containerForm struct {
	Radius       int
	RadiusTop    int
	RadiusBottom int
	Transparent  bool
	Width        int
	PaddingV     int
	PaddingH     int
	PageBG       string
}

func parseContainerSettings(r *http.Request) containerForm {
	out := containerForm{Radius: 8, Width: 600, PaddingV: 20, PaddingH: 0}
	if raw := r.FormValue("container_radius"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed >= 0 && parsed <= 64 {
			out.Radius = parsed
		}
	}
	if raw := r.FormValue("container_radius_top"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed >= 0 && parsed <= 64 {
			out.RadiusTop = parsed
		}
	}
	if raw := r.FormValue("container_radius_bottom"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed >= 0 && parsed <= 64 {
			out.RadiusBottom = parsed
		}
	}
	out.Transparent = r.FormValue("container_transparent") == "on" || r.FormValue("container_transparent") == "true" || r.FormValue("container_transparent") == "1"
	if raw := r.FormValue("container_width"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed >= 320 && parsed <= 1200 {
			out.Width = parsed
		}
	}
	if raw := r.FormValue("container_padding_v"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed >= 0 && parsed <= 200 {
			out.PaddingV = parsed
		}
	}
	if raw := r.FormValue("container_padding_h"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed >= 0 && parsed <= 200 {
			out.PaddingH = parsed
		}
	}
	if bg := strings.TrimSpace(r.FormValue("page_background")); bg != "" {
		out.PageBG = bg
	}

	if v := r.FormValue("page_background_transparent"); v == "on" || v == "true" || v == "1" {
		out.PageBG = "transparent"
	}
	return out
}

func (h *Handlers) BuilderRenderPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		HTML string         `json:"html"`
		Data map[string]any `json:"data"`
	}
	raw, _ := io.ReadAll(r.Body)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.Data == nil {
		body.Data = map[string]any{}
	}
	out, err := emailtpl.RenderHTML("builder-preview", body.HTML, body.Data)
	if err != nil {
		http.Error(w, "Render error: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(out))
}

func (h *Handlers) detectBlocksInHTML(html string) []models.TemplateBlockRef {
	blocks, _, err := h.blocks.List(models.BlockListFilter{Limit: 1000})
	if err != nil || len(blocks) == 0 {
		return nil
	}
	type hit struct {
		idx     int
		blockID string
	}
	var hits []hit
	for _, b := range blocks {
		body := strings.TrimSpace(b.HTML)
		if body == "" {
			continue
		}
		if i := strings.Index(html, body); i >= 0 {
			hits = append(hits, hit{idx: i, blockID: b.ID})
		}
	}
	if len(hits) == 0 {
		return nil
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].idx < hits[j].idx })
	refs := make([]models.TemplateBlockRef, 0, len(hits))
	for i, h := range hits {
		refs = append(refs, models.TemplateBlockRef{
			BlockID:  h.blockID,
			Position: i + 1,
		})
	}
	return refs
}
