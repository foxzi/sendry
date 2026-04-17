package handlers

import (
	"net/http"
	"strings"

	"github.com/foxzi/sendry/internal/web/blocks"
	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
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

	h.render(w, "template_builder", data)
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

	t := &models.Template{
		Name:        name,
		Description: description,
		Subject:     subject,
		HTML:        html,
		Text:        text,
		Variables:   variables,
		Folder:      folder,
	}

	user := h.getUserFromContext(r)
	if err := h.templates.Create(t, user["Email"].(string)); err != nil {
		h.logger.Error("failed to create template from builder", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create template")
		return
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
	result := b.String()
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
