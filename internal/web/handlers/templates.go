package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/sendry"
	smtpclient "github.com/foxzi/sendry/internal/web/smtp"
	emailtpl "github.com/foxzi/sendry/internal/web/template"
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

	blockRefs := parseBlockRefs(r.FormValue("block_refs"))
	if len(blockRefs) > 0 {
		t.UseBlocks = true
	}

	user := h.getUserFromContext(r)
	if err := h.templates.Create(t, user["Email"].(string)); err != nil {
		h.logger.Error("failed to create template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create template")
		return
	}

	if len(blockRefs) > 0 {
		if err := h.templates.SetBlockRefs(t.ID, blockRefs); err != nil {
			h.logger.Error("failed to save block refs", "template_id", t.ID, "error", err)
		}
	}

	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"create", "template", t.ID, `{"name":"`+t.Name+`"}`)
	http.Redirect(w, r, "/templates/"+t.ID, http.StatusSeeOther)
}

func parseBlockRefs(raw string) []models.TemplateBlockRef {
	if raw == "" {
		return nil
	}
	type wireRef struct {
		BlockID   string `json:"block_id"`
		GapHeight int    `json:"gap_height"`
		GapColor  string `json:"gap_color"`
		Condition string `json:"condition"`
	}
	var rich []wireRef
	if err := json.Unmarshal([]byte(raw), &rich); err == nil {
		out := make([]models.TemplateBlockRef, 0, len(rich))
		for _, r := range rich {
			if r.BlockID == "" {
				continue
			}
			out = append(out, models.TemplateBlockRef{
				BlockID:   r.BlockID,
				GapHeight: r.GapHeight,
				GapColor:  r.GapColor,
				Condition: sanitizeIdentifier(r.Condition),
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	out := make([]models.TemplateBlockRef, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, models.TemplateBlockRef{BlockID: id})
		}
	}
	return out
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

	skeleton, _ := json.MarshalIndent(buildSampleData(t.HTML+" "+t.Subject), "", "  ")
	data := map[string]any{
		"Title":          t.Name,
		"Active":         "templates",
		"User":           h.getUserFromContext(r),
		"Template":       t,
		"Deployments":    deployments,
		"Servers":        servers,
		"VariablesShape": string(skeleton),
	}

	h.render(w, "template_view", data)
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

	rawRefs := strings.TrimSpace(r.FormValue("block_refs"))
	blockRefs := parseBlockRefs(rawRefs)
	if rawRefs != "" && rawRefs != "null" {
		t.UseBlocks = len(blockRefs) > 0
	}

	changeNote := r.FormValue("change_note")
	if changeNote == "" {
		changeNote = "Updated template"
	}

	user := h.getUserFromContext(r)
	if err := h.templates.Update(t, changeNote, user["Email"].(string)); err != nil {
		h.logger.Error("failed to update template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update template")
		return
	}

	if rawRefs != "" {
		if err := h.templates.SetBlockRefs(t.ID, blockRefs); err != nil {
			h.logger.Error("failed to update block refs", "template_id", t.ID, "error", err)
		}
	}

	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"update", "template", id, `{"name":"`+t.Name+`"}`)
	http.Redirect(w, r, "/templates/"+id, http.StatusSeeOther)
}

func (h *Handlers) TemplateDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.templates.Delete(id); err != nil {
		h.logger.Error("failed to delete template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete template")
		return
	}

	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"delete", "template", id, "")
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
	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"deploy", "template", id, `{"server":"`+serverName+`"}`)
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
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(t.Name)+".json\"")

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
	if err := h.templates.Create(t, user["Email"].(string)); err != nil {
		h.logger.Error("failed to import template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to import template")
		return
	}

	h.logger.Info("template imported", "id", t.ID, "name", t.Name, "user", user["Email"].(string))
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"create", "template", t.ID, `{"name":"`+t.Name+`","source":"import"}`)
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

	userSMTP, err := h.userSMTP.ListByUser(middleware.GetUserID(r))
	if err != nil {
		h.logger.Error("list user smtp", "error", err)
	}

	sampleJSON, _ := json.MarshalIndent(buildSampleData(t.HTML+" "+t.Subject), "", "  ")

	data := map[string]any{
		"Title":      "Send Test Email - " + t.Name,
		"Active":     "templates",
		"User":       h.getUserFromContext(r),
		"Template":   t,
		"Servers":    servers,
		"UserSMTP":   userSMTP,
		"SampleJSON": string(sampleJSON),
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

	transport := r.FormValue("transport")
	if transport == "" {
		transport = "sendry"
	}
	to := strings.TrimSpace(r.FormValue("to"))
	if to == "" {
		h.error(w, http.StatusBadRequest, "Recipient (to) is required")
		return
	}

	sampleData := map[string]any{}
	if varsJSON := strings.TrimSpace(r.FormValue("variables")); varsJSON != "" {
		if err := json.Unmarshal([]byte(varsJSON), &sampleData); err != nil {
			h.error(w, http.StatusBadRequest, "Invalid JSON in 'Sample data': "+err.Error())
			return
		}
	}

	if transport == "smtp" {
		h.testSendViaSMTP(w, r, t, to, sampleData)
		return
	}

	serverName := r.FormValue("server")
	from := strings.TrimSpace(r.FormValue("from"))
	if serverName == "" || from == "" {
		h.error(w, http.StatusBadRequest, "Server and from are required for Sendry transport")
		return
	}
	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusBadRequest, "Server not found: "+serverName)
		return
	}

	globalVars, err := h.settings.GetGlobalVariablesMap()
	if err != nil {
		h.logger.Error("failed to get global variables", "error", err)
		globalVars = make(map[string]string)
	}
	for k, v := range sampleData {
		if s, ok := v.(string); ok {
			globalVars[k] = s
		} else {
			globalVars[k] = fmt.Sprintf("%v", v)
		}
	}

	subject := renderTemplateVars(t.Subject, globalVars)
	html := renderTemplateVars(t.HTML, globalVars)
	text := renderTemplateVars(t.Text, globalVars)
	html = makeAbsoluteURLs(html, h.cfg.Server.PublicURL, h.cfg.Server.PublicUploadURL)

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
	h.logger.Info("test email sent", "template_id", id, "server", serverName, "to", to, "message_id", resp.ID, "user", user["Email"].(string))
	http.Redirect(w, r, "/templates/"+id+"?test_sent=1", http.StatusSeeOther)
}

func (h *Handlers) testSendViaSMTP(w http.ResponseWriter, r *http.Request, t *models.Template, to string, sampleData map[string]any) {
	if h.cipher == nil {
		h.error(w, http.StatusServiceUnavailable, "Encryption not configured. Set auth.encryption_key in web.yaml.")
		return
	}
	smtpID := r.FormValue("smtp_id")
	if smtpID == "" {
		h.error(w, http.StatusBadRequest, "SMTP server is required")
		return
	}
	userID := middleware.GetUserID(r)
	srv, err := h.userSMTP.GetByID(smtpID, userID)
	if err != nil || srv == nil {
		h.error(w, http.StatusBadRequest, "SMTP server not found")
		return
	}
	password, err := h.cipher.Decrypt(srv.PasswordEnc)
	if err != nil {
		h.logger.Error("decrypt smtp password", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to decrypt SMTP password")
		return
	}

	subject, err := emailtpl.RenderHTML("subject", t.Subject, sampleData)
	if err != nil {
		h.error(w, http.StatusBadRequest, "Render subject: "+err.Error())
		return
	}
	html, err := emailtpl.RenderHTML("body", t.HTML, sampleData)
	if err != nil {
		h.error(w, http.StatusBadRequest, "Render HTML: "+err.Error())
		return
	}
	html = makeAbsoluteURLs(html, h.cfg.Server.PublicURL, h.cfg.Server.PublicUploadURL)

	err = smtpclient.Send(smtpclient.Server{
		Host:       srv.Host,
		Port:       srv.Port,
		Username:   srv.Username,
		Password:   password,
		Encryption: smtpclient.Encryption(srv.Encryption),
	}, smtpclient.Message{
		From:     srv.FromAddress,
		FromName: srv.FromName,
		To:       []string{to},
		Subject:  "[TEST] " + subject,
		HTML:     html,
	})
	if err != nil {
		h.logger.Error("smtp test send", "template_id", t.ID, "smtp_id", smtpID, "error", err)
		h.error(w, http.StatusInternalServerError, "SMTP send failed: "+err.Error())
		return
	}
	user := h.getUserFromContext(r)
	email, _ := user["Email"].(string)
	h.logger.Info("smtp test email sent", "template_id", t.ID, "smtp_id", smtpID, "to", to, "user", email)
	h.settings.LogAction(r, userID, email, "test_send", "template", t.ID,
		auditJSON(map[string]any{"to": to, "smtp_id": smtpID}))
	http.Redirect(w, r, "/templates/"+t.ID+"?test_sent=1", http.StatusSeeOther)
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

func buildSampleData(body string) map[string]any {
	out := map[string]any{}
	collectIntoMap(body, out)
	return out
}

const identPathPat = `[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*`

var (
	templateIfRE  = regexp.MustCompile(`\{\{\s*if\s+\.?(` + identPathPat + `)\s*\}\}`)
	templateVarRE = regexp.MustCompile(`\{\{\s*\.?(` + identPathPat + `)\s*\}\}`)
)

var templateReservedKeywords = map[string]bool{
	"end": true, "else": true, "with": true, "range": true,
	"if": true, "block": true, "define": true, "template": true,
}

func setNestedKey(out map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := out
	for i, p := range parts {
		if i == len(parts)-1 {
			if _, exists := cur[p]; !exists {
				cur[p] = value
			}
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
}

func collectIntoMap(body string, out map[string]any) {
	stripped := stripTopLevelRanges(body, out)
	for _, m := range templateIfRE.FindAllStringSubmatch(stripped, -1) {
		name := m[1]
		if templateReservedKeywords[name] {
			continue
		}
		setNestedKey(out, name, sampleValueFor(lastSegment(name)))
	}
	for _, m := range templateVarRE.FindAllStringSubmatch(stripped, -1) {
		name := m[1]
		if templateReservedKeywords[name] {
			continue
		}
		setNestedKey(out, name, sampleValueFor(lastSegment(name)))
	}
}

func stripTopLevelRanges(body string, out map[string]any) string {
	var sb strings.Builder
	i := 0
	for i < len(body) {
		idx := strings.Index(body[i:], "{{")
		if idx < 0 {
			sb.WriteString(body[i:])
			break
		}
		idx += i
		sb.WriteString(body[i:idx])
		end := strings.Index(body[idx:], "}}")
		if end < 0 {
			sb.WriteString(body[idx:])
			break
		}
		end += idx + 2
		action := body[idx:end]
		rmatch := topRangeStart.FindStringSubmatch(action)
		if rmatch == nil {
			sb.WriteString(action)
			i = end
			continue
		}
		listName := rmatch[1]
		bodyStart := end
		bodyEnd, blockEnd := findMatchingEnd(body, bodyStart)
		if bodyEnd < 0 {
			sb.WriteString(action)
			i = end
			continue
		}
		inner := body[bodyStart:bodyEnd]
		element := map[string]any{}
		collectIntoMap(inner, element)
		setNestedKey(out, listName, []any{element})
		i = blockEnd
	}
	return sb.String()
}

var topRangeStart = regexp.MustCompile(`^\{\{\s*range\s+\.?(` + identPathPat + `)\s*\}\}$`)

var (
	blockOpenRE  = regexp.MustCompile(`\{\{\s*(range|if|with|block|define)\b[^}]*\}\}`)
	blockCloseRE = regexp.MustCompile(`\{\{\s*end\s*\}\}`)
)

func findMatchingEnd(body string, start int) (int, int) {
	depth := 1
	pos := start
	for pos < len(body) {
		nextOpen := blockOpenRE.FindStringIndex(body[pos:])
		nextClose := blockCloseRE.FindStringIndex(body[pos:])
		if nextClose == nil {
			return -1, -1
		}
		if nextOpen != nil && nextOpen[0] < nextClose[0] {
			depth++
			pos += nextOpen[1]
			continue
		}
		depth--
		if depth == 0 {
			return pos + nextClose[0], pos + nextClose[1]
		}
		pos += nextClose[1]
	}
	return -1, -1
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}

func sampleValueFor(name string) string {
	low := strings.ToLower(name)
	switch {
	case strings.Contains(low, "email"):
		return "user@example.com"
	case strings.Contains(low, "url") || strings.Contains(low, "link"):
		return "https://example.com"
	case strings.Contains(low, "phone"):
		return "+7 999 123-45-67"
	case strings.Contains(low, "year"):
		return "2026"
	case strings.Contains(low, "date"):
		return "2026-01-01"
	case strings.Contains(low, "price") || strings.Contains(low, "amount") || strings.Contains(low, "total") || strings.Contains(low, "sum"):
		return "100 ₽"
	case strings.Contains(low, "percent"):
		return "10%"
	case strings.Contains(low, "quantity") || strings.Contains(low, "count"):
		return "1"
	case strings.Contains(low, "number") || strings.HasSuffix(low, "id"):
		return "12345"
	case strings.Contains(low, "name"):
		return "Sample " + name
	}
	return "Sample " + name
}

var identifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func sanitizeIdentifier(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || !identifierRE.MatchString(s) {
		return ""
	}
	return s
}
