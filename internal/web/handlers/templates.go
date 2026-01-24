package handlers

import (
	"net/http"
	"strconv"

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
	req := &sendry.TemplateCreateRequest{
		Name:        t.Name,
		Description: t.Description,
		Subject:     t.Subject,
		HTML:        t.HTML,
		Text:        t.Text,
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
