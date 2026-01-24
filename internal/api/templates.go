package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/template"
)

// TemplateServer handles template API endpoints
type TemplateServer struct {
	storage *template.Storage
	engine  *template.Engine
	queue   queue.Queue
}

// NewTemplateServer creates a new template server
func NewTemplateServer(storage *template.Storage, q queue.Queue) *TemplateServer {
	return &TemplateServer{
		storage: storage,
		engine:  template.NewEngine(),
		queue:   q,
	}
}

// RegisterRoutes registers template API routes
func (s *TemplateServer) RegisterRoutes(r chi.Router) {
	r.Route("/templates", func(r chi.Router) {
		r.Get("/", s.handleList)
		r.Post("/", s.handleCreate)
		r.Get("/{id}", s.handleGet)
		r.Put("/{id}", s.handleUpdate)
		r.Delete("/{id}", s.handleDelete)
		r.Post("/{id}/preview", s.handlePreview)
	})

	r.Post("/send/template", s.handleSendTemplate)
}

// Request/Response types

// TemplateCreateRequest is the request for creating a template
type TemplateCreateRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Subject     string                  `json:"subject"`
	HTML        string                  `json:"html,omitempty"`
	Text        string                  `json:"text,omitempty"`
	Variables   []template.VariableInfo `json:"variables,omitempty"`
}

// TemplateUpdateRequest is the request for updating a template
type TemplateUpdateRequest struct {
	Name        string                  `json:"name,omitempty"`
	Description string                  `json:"description,omitempty"`
	Subject     string                  `json:"subject,omitempty"`
	HTML        string                  `json:"html,omitempty"`
	Text        string                  `json:"text,omitempty"`
	Variables   []template.VariableInfo `json:"variables,omitempty"`
}

// TemplateResponse is the response for a template
type TemplateResponse struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Subject     string                  `json:"subject"`
	HTML        string                  `json:"html,omitempty"`
	Text        string                  `json:"text,omitempty"`
	Variables   []template.VariableInfo `json:"variables,omitempty"`
	Version     int                     `json:"version"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// TemplateListResponse is the response for listing templates
type TemplateListResponse struct {
	Templates []*TemplateResponse `json:"templates"`
	Total     int                 `json:"total"`
}

// TemplatePreviewRequest is the request for previewing a template
type TemplatePreviewRequest struct {
	Data map[string]interface{} `json:"data"`
}

// TemplatePreviewResponse is the response for previewing a template
type TemplatePreviewResponse struct {
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

// SendTemplateRequest is the request for sending via template
type SendTemplateRequest struct {
	TemplateID   string                 `json:"template_id,omitempty"`
	TemplateName string                 `json:"template_name,omitempty"`
	From         string                 `json:"from"`
	To           []string               `json:"to"`
	CC           []string               `json:"cc,omitempty"`
	BCC          []string               `json:"bcc,omitempty"`
	Data         map[string]interface{} `json:"data"`
	Headers      map[string]string      `json:"headers,omitempty"`
}

// handleList handles GET /api/v1/templates
func (s *TemplateServer) handleList(w http.ResponseWriter, r *http.Request) {
	filter := template.ListFilter{
		Search: r.URL.Query().Get("search"),
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset > 0 {
			filter.Offset = offset
		}
	}

	templates, err := s.storage.List(r.Context(), filter)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to list templates")
		return
	}

	response := TemplateListResponse{
		Templates: make([]*TemplateResponse, len(templates)),
		Total:     len(templates),
	}

	for i, tmpl := range templates {
		response.Templates[i] = templateToResponse(tmpl)
	}

	sendJSON(w, http.StatusOK, response)
}

// handleCreate handles POST /api/v1/templates
func (s *TemplateServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req TemplateCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		sendError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.Subject == "" {
		sendError(w, http.StatusBadRequest, "subject is required")
		return
	}

	if req.HTML == "" && req.Text == "" {
		sendError(w, http.StatusBadRequest, "html or text is required")
		return
	}

	tmpl := &template.Template{
		Name:        req.Name,
		Description: req.Description,
		Subject:     req.Subject,
		HTML:        req.HTML,
		Text:        req.Text,
		Variables:   req.Variables,
	}

	// Validate template syntax
	if err := s.engine.Validate(tmpl); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid template syntax: %v", err))
		return
	}

	if err := s.storage.Create(r.Context(), tmpl); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			sendError(w, http.StatusConflict, err.Error())
			return
		}
		sendError(w, http.StatusInternalServerError, "Failed to create template")
		return
	}

	sendJSON(w, http.StatusCreated, templateToResponse(tmpl))
}

// handleGet handles GET /api/v1/templates/{id}
func (s *TemplateServer) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	tmpl, err := s.storage.Get(r.Context(), id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get template")
		return
	}

	if tmpl == nil {
		// Try by name
		tmpl, err = s.storage.GetByName(r.Context(), id)
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Failed to get template")
			return
		}
	}

	if tmpl == nil {
		sendError(w, http.StatusNotFound, "Template not found")
		return
	}

	sendJSON(w, http.StatusOK, templateToResponse(tmpl))
}

// handleUpdate handles PUT /api/v1/templates/{id}
func (s *TemplateServer) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req TemplateUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tmpl, err := s.storage.Get(r.Context(), id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get template")
		return
	}

	if tmpl == nil {
		sendError(w, http.StatusNotFound, "Template not found")
		return
	}

	// Update fields
	if req.Name != "" {
		tmpl.Name = req.Name
	}
	if req.Description != "" {
		tmpl.Description = req.Description
	}
	if req.Subject != "" {
		tmpl.Subject = req.Subject
	}
	if req.HTML != "" {
		tmpl.HTML = req.HTML
	}
	if req.Text != "" {
		tmpl.Text = req.Text
	}
	if req.Variables != nil {
		tmpl.Variables = req.Variables
	}

	// Validate template syntax
	if err := s.engine.Validate(tmpl); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid template syntax: %v", err))
		return
	}

	if err := s.storage.Update(r.Context(), tmpl); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			sendError(w, http.StatusConflict, err.Error())
			return
		}
		sendError(w, http.StatusInternalServerError, "Failed to update template")
		return
	}

	sendJSON(w, http.StatusOK, templateToResponse(tmpl))
}

// handleDelete handles DELETE /api/v1/templates/{id}
func (s *TemplateServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.storage.Delete(r.Context(), id); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to delete template")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handlePreview handles POST /api/v1/templates/{id}/preview
func (s *TemplateServer) handlePreview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req TemplatePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tmpl, err := s.storage.Get(r.Context(), id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get template")
		return
	}

	if tmpl == nil {
		// Try by name
		tmpl, err = s.storage.GetByName(r.Context(), id)
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Failed to get template")
			return
		}
	}

	if tmpl == nil {
		sendError(w, http.StatusNotFound, "Template not found")
		return
	}

	result, err := s.engine.Render(tmpl, req.Data)
	if err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to render template: %v", err))
		return
	}

	sendJSON(w, http.StatusOK, TemplatePreviewResponse{
		Subject: result.Subject,
		HTML:    result.HTML,
		Text:    result.Text,
	})
}

// handleSendTemplate handles POST /api/v1/send/template
func (s *TemplateServer) handleSendTemplate(w http.ResponseWriter, r *http.Request) {
	var req SendTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TemplateID == "" && req.TemplateName == "" {
		sendError(w, http.StatusBadRequest, "template_id or template_name is required")
		return
	}

	if req.From == "" {
		sendError(w, http.StatusBadRequest, "from is required")
		return
	}

	if len(req.To) == 0 {
		sendError(w, http.StatusBadRequest, "to is required")
		return
	}

	// Get template
	var tmpl *template.Template
	var err error

	if req.TemplateID != "" {
		tmpl, err = s.storage.Get(r.Context(), req.TemplateID)
	} else {
		tmpl, err = s.storage.GetByName(r.Context(), req.TemplateName)
	}

	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get template")
		return
	}

	if tmpl == nil {
		sendError(w, http.StatusNotFound, "Template not found")
		return
	}

	// Render template
	result, err := s.engine.Render(tmpl, req.Data)
	if err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to render template: %v", err))
		return
	}

	// Build email data
	data := s.buildEmailData(req.From, req.To, result.Subject, result.Text, result.HTML, req.Headers)

	// Create message
	msg := &queue.Message{
		ID:        uuid.New().String(),
		From:      req.From,
		To:        req.To,
		Data:      data,
		Status:    queue.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ClientIP:  r.RemoteAddr,
	}

	// Enqueue
	if err := s.queue.Enqueue(r.Context(), msg); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to queue message")
		return
	}

	sendJSON(w, http.StatusAccepted, SendResponse{
		ID:     msg.ID,
		Status: string(msg.Status),
	})
}

// buildEmailData constructs RFC 5322 email data
func (s *TemplateServer) buildEmailData(from string, to []string, subject, text, html string, headers map[string]string) []byte {
	var buf bytes.Buffer

	// Headers
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString(fmt.Sprintf("Message-ID: <%s@%s>\r\n", uuid.New().String(), extractDomainFromEmail(from)))

	// Custom headers
	for k, v := range headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	// MIME headers
	if html != "" {
		boundary := uuid.New().String()
		buf.WriteString("MIME-Version: 1.0\r\n")
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
		buf.WriteString("\r\n")

		// Plain text part
		if text != "" {
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
			buf.WriteString("\r\n")
			buf.WriteString(text)
			buf.WriteString("\r\n")
		}

		// HTML part
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(html)
		buf.WriteString("\r\n")

		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(text)
	}

	return buf.Bytes()
}

func extractDomainFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return "localhost"
}

func templateToResponse(tmpl *template.Template) *TemplateResponse {
	return &TemplateResponse{
		ID:          tmpl.ID,
		Name:        tmpl.Name,
		Description: tmpl.Description,
		Subject:     tmpl.Subject,
		HTML:        tmpl.HTML,
		Text:        tmpl.Text,
		Variables:   tmpl.Variables,
		Version:     tmpl.Version,
		CreatedAt:   tmpl.CreatedAt,
		UpdatedAt:   tmpl.UpdatedAt,
	}
}
