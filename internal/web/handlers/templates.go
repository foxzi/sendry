package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/sendry"
)

func (h *Handlers) TemplateList(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	search := r.URL.Query().Get("search")
	folder := r.URL.Query().Get("folder")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	filter := models.TemplateListFilter{
		Search: search,
		Folder: folder,
		Limit:  limit,
		Offset: offset,
	}

	templates, total, err := h.templates.List(filter)
	if err != nil {
		h.logger.Error("failed to list templates", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load templates")
		return
	}

	folders, err := h.templates.GetFolders()
	if err != nil {
		h.logger.Error("failed to get folders", "error", err)
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Templates",
		"Active":     "templates",
		"User":       h.getUserFromContext(r),
		"Templates":  templates,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
		"Folder":     folder,
		"Folders":    folders,
	}

	h.render(w, "templates", data)
}

func (h *Handlers) TemplateNew(w http.ResponseWriter, r *http.Request) {
	folders, _ := h.templates.GetFolders()

	data := map[string]any{
		"Title":   "New Template",
		"Active":  "templates",
		"User":    h.getUserFromContext(r),
		"Folders": folders,
	}

	h.render(w, "template_new", data)
}

func (h *Handlers) TemplateCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	t := &models.Template{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		Subject:     r.FormValue("subject"),
		HTML:        r.FormValue("html"),
		Text:        r.FormValue("text"),
		Variables:   r.FormValue("variables"),
		Folder:      r.FormValue("folder"),
	}

	// Validate required fields
	if t.Name == "" || t.Subject == "" {
		h.error(w, http.StatusBadRequest, "Name and subject are required")
		return
	}

	user := h.getUserFromContext(r)
	if err := h.templates.Create(t, user["Email"]); err != nil {
		h.logger.Error("failed to create template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create template")
		return
	}

	http.Redirect(w, r, "/templates/"+t.ID, http.StatusSeeOther)
}

func (h *Handlers) TemplateView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := h.templates.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load template")
		return
	}
	if t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	deployments, err := h.templates.GetDeployments(id)
	if err != nil {
		h.logger.Error("failed to get deployments", "error", err)
	}

	// Get available servers from config
	servers := make([]map[string]any, 0)
	for _, s := range h.cfg.Sendry.Servers {
		deployed := false
		deployedVersion := 0
		for _, d := range deployments {
			if d.ServerName == s.Name {
				deployed = true
				deployedVersion = d.DeployedVersion
				break
			}
		}
		servers = append(servers, map[string]any{
			"Name":            s.Name,
			"Env":             s.Env,
			"Deployed":        deployed,
			"DeployedVersion": deployedVersion,
			"OutOfSync":       deployed && deployedVersion < t.CurrentVersion,
		})
	}

	data := map[string]any{
		"Title":       t.Name,
		"Active":      "templates",
		"User":        h.getUserFromContext(r),
		"Template":    t,
		"Deployments": deployments,
		"Servers":     servers,
	}

	h.render(w, "template_view", data)
}

func (h *Handlers) TemplateEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := h.templates.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load template")
		return
	}
	if t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	folders, _ := h.templates.GetFolders()

	data := map[string]any{
		"Title":    "Edit " + t.Name,
		"Active":   "templates",
		"User":     h.getUserFromContext(r),
		"Template": t,
		"Folders":  folders,
	}

	h.render(w, "template_edit", data)
}

func (h *Handlers) TemplateUpdate(w http.ResponseWriter, r *http.Request) {
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

	t.Name = r.FormValue("name")
	t.Description = r.FormValue("description")
	t.Subject = r.FormValue("subject")
	t.HTML = r.FormValue("html")
	t.Text = r.FormValue("text")
	t.Variables = r.FormValue("variables")
	t.Folder = r.FormValue("folder")

	changeNote := r.FormValue("change_note")
	if changeNote == "" {
		changeNote = "Updated template"
	}

	user := h.getUserFromContext(r)
	if err := h.templates.Update(t, changeNote, user["Email"]); err != nil {
		h.logger.Error("failed to update template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update template")
		return
	}

	http.Redirect(w, r, "/templates/"+id, http.StatusSeeOther)
}

func (h *Handlers) TemplateDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.templates.Delete(id); err != nil {
		h.logger.Error("failed to delete template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete template")
		return
	}

	http.Redirect(w, r, "/templates", http.StatusSeeOther)
}

func (h *Handlers) TemplateVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := h.templates.GetByID(id)
	if err != nil || t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	versions, err := h.templates.GetVersions(id)
	if err != nil {
		h.logger.Error("failed to get versions", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load versions")
		return
	}

	data := map[string]any{
		"Title":    t.Name + " - Versions",
		"Active":   "templates",
		"User":     h.getUserFromContext(r),
		"Template": t,
		"Versions": versions,
	}

	h.render(w, "template_versions", data)
}

func (h *Handlers) TemplateDeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	serverName := r.FormValue("server")
	if serverName == "" {
		h.error(w, http.StatusBadRequest, "Server name is required")
		return
	}

	t, err := h.templates.GetByID(id)
	if err != nil || t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	// Get Sendry client for this server
	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusBadRequest, "Server not found: "+serverName)
		return
	}

	// Check if template was already deployed to this server
	existingDeployment, _ := h.templates.GetDeployment(id, serverName)

	// Build template request for Sendry API
	// Convert {{variable}} to {{.variable}} for Go templates compatibility
	req := &sendry.TemplateCreateRequest{
		Name:        t.Name,
		Description: t.Description,
		Subject:     convertToGoTemplate(t.Subject),
		HTML:        convertToGoTemplate(t.HTML),
		Text:        convertToGoTemplate(t.Text),
	}

	var remoteID string
	ctx := r.Context()

	if existingDeployment != nil && existingDeployment.RemoteID != "" {
		// Update existing template on Sendry
		resp, err := client.UpdateTemplate(ctx, existingDeployment.RemoteID, req)
		if err != nil {
			h.logger.Error("failed to update template on Sendry", "server", serverName, "error", err)
			h.error(w, http.StatusInternalServerError, "Failed to deploy template: "+err.Error())
			return
		}
		remoteID = resp.ID
	} else {
		// Create new template on Sendry
		resp, err := client.CreateTemplate(ctx, req)
		if err != nil {
			h.logger.Error("failed to create template on Sendry", "server", serverName, "error", err)
			h.error(w, http.StatusInternalServerError, "Failed to deploy template: "+err.Error())
			return
		}
		remoteID = resp.ID
	}

	// Save deployment record
	deployment := &models.TemplateDeployment{
		TemplateID:      id,
		ServerName:      serverName,
		RemoteID:        remoteID,
		DeployedVersion: t.CurrentVersion,
	}

	if err := h.templates.SaveDeployment(deployment); err != nil {
		h.logger.Error("failed to save deployment", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save deployment record")
		return
	}

	h.logger.Info("template deployed", "template_id", id, "server", serverName, "remote_id", remoteID, "version", t.CurrentVersion)
	http.Redirect(w, r, "/templates/"+id, http.StatusSeeOther)
}

func (h *Handlers) TemplateDiff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v1Str := r.URL.Query().Get("v1")
	v2Str := r.URL.Query().Get("v2")

	t, err := h.templates.GetByID(id)
	if err != nil || t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	versions, err := h.templates.GetVersions(id)
	if err != nil {
		h.logger.Error("failed to get versions", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load versions")
		return
	}

	// If no versions selected, show version selector
	if v1Str == "" || v2Str == "" {
		data := map[string]any{
			"Title":    t.Name + " - Compare Versions",
			"Active":   "templates",
			"User":     h.getUserFromContext(r),
			"Template": t,
			"Versions": versions,
		}
		h.render(w, "template_diff", data)
		return
	}

	v1, _ := strconv.Atoi(v1Str)
	v2, _ := strconv.Atoi(v2Str)

	// Get version 1 content
	ver1, err := h.templates.GetVersion(id, v1)
	if err != nil {
		h.logger.Error("failed to get version", "version", v1, "error", err)
		h.error(w, http.StatusNotFound, "Version not found")
		return
	}

	// Get version 2 content
	ver2, err := h.templates.GetVersion(id, v2)
	if err != nil {
		h.logger.Error("failed to get version", "version", v2, "error", err)
		h.error(w, http.StatusNotFound, "Version not found")
		return
	}

	data := map[string]any{
		"Title":    t.Name + " - Compare v" + v1Str + " vs v" + v2Str,
		"Active":   "templates",
		"User":     h.getUserFromContext(r),
		"Template": t,
		"Versions": versions,
		"V1":       v1,
		"V2":       v2,
		"Version1": ver1,
		"Version2": ver2,
	}

	h.render(w, "template_diff", data)
}

func (h *Handlers) TemplatePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	versionStr := r.URL.Query().Get("version")

	t, err := h.templates.GetByID(id)
	if err != nil || t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	html := t.HTML
	subject := t.Subject

	// If version specified, get that version
	if versionStr != "" {
		version, _ := strconv.Atoi(versionStr)
		if version > 0 && version != t.CurrentVersion {
			v, err := h.templates.GetVersion(id, version)
			if err == nil && v != nil {
				html = v.HTML
				subject = v.Subject
			}
		}
	}

	data := map[string]any{
		"Title":    "Preview: " + t.Name,
		"Active":   "templates",
		"User":     h.getUserFromContext(r),
		"Template": t,
		"HTML":     html,
		"Subject":  subject,
	}

	h.render(w, "template_preview", data)
}

// TemplateExportData represents template data for export/import
type TemplateExportData struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Subject     string `json:"subject"`
	HTML        string `json:"html"`
	Text        string `json:"text"`
	Variables   string `json:"variables"`
	Folder      string `json:"folder"`
	ExportedAt  string `json:"exported_at"`
	Version     int    `json:"version"`
}

func (h *Handlers) TemplateExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := h.templates.GetByID(id)
	if err != nil || t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	export := TemplateExportData{
		Name:        t.Name,
		Description: t.Description,
		Subject:     t.Subject,
		HTML:        t.HTML,
		Text:        t.Text,
		Variables:   t.Variables,
		Folder:      t.Folder,
		ExportedAt:  time.Now().UTC().Format(time.RFC3339),
		Version:     t.CurrentVersion,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+t.Name+".json\"")

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(export); err != nil {
		h.logger.Error("failed to encode template export", "error", err)
	}
}

func (h *Handlers) TemplateImportPage(w http.ResponseWriter, r *http.Request) {
	folders, _ := h.templates.GetFolders()

	data := map[string]any{
		"Title":   "Import Template",
		"Active":  "templates",
		"User":    h.getUserFromContext(r),
		"Folders": folders,
	}

	h.render(w, "template_import", data)
}

func (h *Handlers) TemplateImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		// Try regular form parse
		if err := r.ParseForm(); err != nil {
			h.error(w, http.StatusBadRequest, "Invalid form data")
			return
		}
	}

	var importData TemplateExportData

	// Check for file upload
	file, _, err := r.FormFile("file")
	if err == nil {
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&importData); err != nil {
			h.error(w, http.StatusBadRequest, "Invalid JSON file: "+err.Error())
			return
		}
	} else {
		// Try JSON text input
		jsonText := r.FormValue("json")
		if jsonText == "" {
			h.error(w, http.StatusBadRequest, "No file or JSON provided")
			return
		}
		if err := json.Unmarshal([]byte(jsonText), &importData); err != nil {
			h.error(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
	}

	// Override folder if specified in form
	if folder := r.FormValue("folder"); folder != "" {
		importData.Folder = folder
	}

	// Override name if specified in form
	if name := r.FormValue("name"); name != "" {
		importData.Name = name
	}

	// Validate required fields
	if importData.Name == "" || importData.Subject == "" {
		h.error(w, http.StatusBadRequest, "Name and subject are required")
		return
	}

	// Create template
	t := &models.Template{
		Name:        importData.Name,
		Description: importData.Description,
		Subject:     importData.Subject,
		HTML:        importData.HTML,
		Text:        importData.Text,
		Variables:   importData.Variables,
		Folder:      importData.Folder,
	}

	user := h.getUserFromContext(r)
	if err := h.templates.Create(t, user["Email"]); err != nil {
		h.logger.Error("failed to import template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to import template")
		return
	}

	h.logger.Info("template imported", "id", t.ID, "name", t.Name, "user", user["Email"])
	http.Redirect(w, r, "/templates/"+t.ID, http.StatusSeeOther)
}

func (h *Handlers) TemplateTestPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := h.templates.GetByID(id)
	if err != nil || t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}

	// Get available servers from config
	servers := make([]map[string]any, 0)
	for _, s := range h.cfg.Sendry.Servers {
		servers = append(servers, map[string]any{
			"Name": s.Name,
			"Env":  s.Env,
		})
	}

	data := map[string]any{
		"Title":    "Send Test Email - " + t.Name,
		"Active":   "templates",
		"User":     h.getUserFromContext(r),
		"Template": t,
		"Servers":  servers,
	}

	h.render(w, "template_test", data)
}

func (h *Handlers) TemplateTest(w http.ResponseWriter, r *http.Request) {
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

	serverName := r.FormValue("server")
	to := r.FormValue("to")
	from := r.FormValue("from")

	if serverName == "" || to == "" || from == "" {
		h.error(w, http.StatusBadRequest, "Server, from, and to are required")
		return
	}

	// Get Sendry client
	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusBadRequest, "Server not found: "+serverName)
		return
	}

	// Get global variables for substitution
	globalVars, err := h.settings.GetGlobalVariablesMap()
	if err != nil {
		h.logger.Error("failed to get global variables", "error", err)
		globalVars = make(map[string]string)
	}

	// Render template with variables
	subject := renderTemplateVars(t.Subject, globalVars)
	html := renderTemplateVars(t.HTML, globalVars)
	text := renderTemplateVars(t.Text, globalVars)

	// Send test email
	req := &sendry.SendRequest{
		From:    from,
		To:      []string{to},
		Subject: "[TEST] " + subject,
		HTML:    html,
		Body:    text,
	}

	ctx := r.Context()
	resp, err := client.Send(ctx, req)
	if err != nil {
		h.logger.Error("failed to send test email", "error", err, "template_id", id, "server", serverName)
		h.error(w, http.StatusInternalServerError, "Failed to send test email: "+err.Error())
		return
	}

	user := h.getUserFromContext(r)
	h.logger.Info("test email sent", "template_id", id, "server", serverName, "to", to, "message_id", resp.ID, "user", user["Email"])

	// Redirect back to template with success message
	http.Redirect(w, r, "/templates/"+id+"?test_sent=1", http.StatusSeeOther)
}

// renderTemplateVars replaces {{var}} with values from vars map
func renderTemplateVars(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		result = replaceVar(result, key, value)
	}
	return result
}

// replaceVar replaces {{key}} with value in template
func replaceVar(template, key, value string) string {
	placeholder := "{{" + key + "}}"
	for i := 0; i < len(template); {
		idx := indexString(template[i:], placeholder)
		if idx == -1 {
			break
		}
		template = template[:i+idx] + value + template[i+idx+len(placeholder):]
		i = i + idx + len(value)
	}
	return template
}

// indexString returns the index of substr in s, or -1 if not found
func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// convertToGoTemplate converts {{variable}} syntax to {{.variable}} for Go templates
// This is needed because Sendry MTA uses Go's text/template which requires dot notation
func convertToGoTemplate(template string) string {
	if template == "" {
		return template
	}

	result := template
	i := 0
	for i < len(result) {
		// Find {{
		start := indexString(result[i:], "{{")
		if start == -1 {
			break
		}
		start += i

		// Find }}
		end := indexString(result[start:], "}}")
		if end == -1 {
			break
		}
		end += start

		// Extract content between {{ and }}
		content := result[start+2 : end]

		// Skip if already has a dot prefix or is a Go template function/action
		trimmed := content
		for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
			trimmed = trimmed[1:]
		}

		// Skip if:
		// - already starts with . (e.g., {{.name}})
		// - is a range/if/else/end/define/template/block/with action
		// - contains a pipe | (function call)
		// - is empty
		needsConvert := len(trimmed) > 0 &&
			trimmed[0] != '.' &&
			trimmed[0] != '$' &&
			!startsWithKeyword(trimmed, "range") &&
			!startsWithKeyword(trimmed, "if") &&
			!startsWithKeyword(trimmed, "else") &&
			!startsWithKeyword(trimmed, "end") &&
			!startsWithKeyword(trimmed, "define") &&
			!startsWithKeyword(trimmed, "template") &&
			!startsWithKeyword(trimmed, "block") &&
			!startsWithKeyword(trimmed, "with") &&
			!containsChar(content, '|')

		if needsConvert {
			// Add dot prefix: {{name}} -> {{.name}}
			// Preserve any leading whitespace
			leadingSpace := ""
			for j := 0; j < len(content); j++ {
				if content[j] == ' ' || content[j] == '\t' {
					leadingSpace += string(content[j])
				} else {
					break
				}
			}
			newContent := leadingSpace + "." + trimmed
			result = result[:start+2] + newContent + result[end:]
			i = start + 2 + len(newContent) + 2
		} else {
			i = end + 2
		}
	}

	return result
}

// startsWithKeyword checks if s starts with keyword followed by space or end
func startsWithKeyword(s, keyword string) bool {
	if len(s) < len(keyword) {
		return false
	}
	if s[:len(keyword)] != keyword {
		return false
	}
	if len(s) == len(keyword) {
		return true
	}
	return s[len(keyword)] == ' ' || s[len(keyword)] == '\t'
}

// containsChar checks if s contains char c
func containsChar(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}
