package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/sandbox"
)

// SandboxServer handles sandbox API endpoints
type SandboxServer struct {
	storage *sandbox.Storage
	queue   queue.Queue
}

// NewSandboxServer creates a new sandbox server
func NewSandboxServer(storage *sandbox.Storage, q queue.Queue) *SandboxServer {
	return &SandboxServer{
		storage: storage,
		queue:   q,
	}
}

// RegisterRoutes registers sandbox API routes
func (s *SandboxServer) RegisterRoutes(r chi.Router) {
	r.Route("/sandbox", func(r chi.Router) {
		r.Get("/messages", s.handleList)
		r.Get("/messages/{id}", s.handleGet)
		r.Get("/messages/{id}/raw", s.handleGetRaw)
		r.Delete("/messages", s.handleClear)
		r.Delete("/messages/{id}", s.handleDelete)
		r.Post("/messages/{id}/resend", s.handleResend)
		r.Get("/stats", s.handleStats)
	})
}

// SandboxMessageResponse represents a sandbox message in API responses
type SandboxMessageResponse struct {
	ID           string    `json:"id"`
	From         string    `json:"from"`
	To           []string  `json:"to"`
	OriginalTo   []string  `json:"original_to,omitempty"`
	Subject      string    `json:"subject"`
	Domain       string    `json:"domain"`
	Mode         string    `json:"mode"`
	CapturedAt   time.Time `json:"captured_at"`
	ClientIP     string    `json:"client_ip,omitempty"`
	SimulatedErr string    `json:"simulated_error,omitempty"`
}

// SandboxListResponse is the response for GET /api/v1/sandbox/messages
type SandboxListResponse struct {
	Messages []*SandboxMessageResponse `json:"messages"`
	Total    int                       `json:"total"`
}

// handleList handles GET /api/v1/sandbox/messages
func (s *SandboxServer) handleList(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		sendError(w, http.StatusServiceUnavailable, "Sandbox storage not available")
		return
	}

	filter := sandbox.ListFilter{
		Domain: r.URL.Query().Get("domain"),
		Mode:   r.URL.Query().Get("mode"),
		From:   r.URL.Query().Get("from"),
		Limit:  100, // Default limit
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
			if filter.Limit > 1000 {
				filter.Limit = 1000 // Prevent DoS via excessive limit
			}
		}
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filter.Offset = o
			if filter.Offset > 1000000 {
				filter.Offset = 1000000 // Prevent excessive offset
			}
		}
	}

	messages, err := s.storage.List(r.Context(), filter)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to list messages")
		return
	}

	response := SandboxListResponse{
		Messages: make([]*SandboxMessageResponse, len(messages)),
		Total:    len(messages),
	}

	for i, msg := range messages {
		response.Messages[i] = &SandboxMessageResponse{
			ID:           msg.ID,
			From:         msg.From,
			To:           msg.To,
			OriginalTo:   msg.OriginalTo,
			Subject:      msg.Subject,
			Domain:       msg.Domain,
			Mode:         msg.Mode,
			CapturedAt:   msg.CapturedAt,
			ClientIP:     msg.ClientIP,
			SimulatedErr: msg.SimulatedErr,
		}
	}

	sendJSON(w, http.StatusOK, response)
}

// SandboxMessageDetailResponse is the response for GET /api/v1/sandbox/messages/{id}
type SandboxMessageDetailResponse struct {
	SandboxMessageResponse
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	HTML    string            `json:"html,omitempty"`
	Size    int               `json:"size"`
}

// handleGet handles GET /api/v1/sandbox/messages/{id}
func (s *SandboxServer) handleGet(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		sendError(w, http.StatusServiceUnavailable, "Sandbox storage not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	msg, err := s.storage.Get(r.Context(), id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get message")
		return
	}

	if msg == nil {
		sendError(w, http.StatusNotFound, "Message not found")
		return
	}

	// Parse headers and body
	headers, body, html := parseEmailData(msg.Data)

	response := SandboxMessageDetailResponse{
		SandboxMessageResponse: SandboxMessageResponse{
			ID:           msg.ID,
			From:         msg.From,
			To:           msg.To,
			OriginalTo:   msg.OriginalTo,
			Subject:      msg.Subject,
			Domain:       msg.Domain,
			Mode:         msg.Mode,
			CapturedAt:   msg.CapturedAt,
			ClientIP:     msg.ClientIP,
			SimulatedErr: msg.SimulatedErr,
		},
		Headers: headers,
		Body:    body,
		HTML:    html,
		Size:    len(msg.Data),
	}

	sendJSON(w, http.StatusOK, response)
}

// handleGetRaw handles GET /api/v1/sandbox/messages/{id}/raw
func (s *SandboxServer) handleGetRaw(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		sendError(w, http.StatusServiceUnavailable, "Sandbox storage not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	msg, err := s.storage.Get(r.Context(), id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get message")
		return
	}

	if msg == nil {
		sendError(w, http.StatusNotFound, "Message not found")
		return
	}

	// Return raw email data
	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(id)+".eml\"")
	w.WriteHeader(http.StatusOK)
	w.Write(msg.Data)
}

// handleDelete handles DELETE /api/v1/sandbox/messages/{id}
func (s *SandboxServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		sendError(w, http.StatusServiceUnavailable, "Sandbox storage not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.storage.Delete(r.Context(), id); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to delete message")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SandboxClearRequest is the request for DELETE /api/v1/sandbox/messages
type SandboxClearRequest struct {
	Domain   string `json:"domain,omitempty"`
	OlderThan string `json:"older_than,omitempty"` // Duration string like "7d", "24h"
}

// handleClear handles DELETE /api/v1/sandbox/messages
func (s *SandboxServer) handleClear(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		sendError(w, http.StatusServiceUnavailable, "Sandbox storage not available")
		return
	}

	domain := r.URL.Query().Get("domain")
	olderThanStr := r.URL.Query().Get("older_than")

	var olderThan time.Duration
	if olderThanStr != "" {
		d, err := time.ParseDuration(olderThanStr)
		if err != nil {
			sendError(w, http.StatusBadRequest, "Invalid older_than format (use Go duration: 24h, 7d)")
			return
		}
		olderThan = d
	}

	count, err := s.storage.Clear(r.Context(), domain, olderThan)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to clear messages")
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"cleared": count,
	})
}

// handleResend handles POST /api/v1/sandbox/messages/{id}/resend
func (s *SandboxServer) handleResend(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		sendError(w, http.StatusServiceUnavailable, "Sandbox storage not available")
		return
	}

	if s.queue == nil {
		sendError(w, http.StatusServiceUnavailable, "Queue not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	msg, err := s.storage.Get(r.Context(), id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get message")
		return
	}

	if msg == nil {
		sendError(w, http.StatusNotFound, "Message not found")
		return
	}

	// Determine recipients - use original if available (for redirect mode)
	recipients := msg.To
	if len(msg.OriginalTo) > 0 {
		recipients = msg.OriginalTo
	}

	// Create new queue message
	queueMsg := &queue.Message{
		ID:        id + "-resend-" + time.Now().Format("20060102150405"),
		From:      msg.From,
		To:        recipients,
		Data:      msg.Data,
		Status:    queue.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ClientIP:  msg.ClientIP,
	}

	if err := s.queue.Enqueue(r.Context(), queueMsg); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to enqueue message")
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{
		"status":     "queued",
		"message_id": queueMsg.ID,
	})
}

// SandboxStatsResponse is the response for GET /api/v1/sandbox/stats
type SandboxStatsResponse struct {
	Total     int64            `json:"total"`
	ByDomain  map[string]int64 `json:"by_domain"`
	ByMode    map[string]int64 `json:"by_mode"`
	OldestAt  *time.Time       `json:"oldest_at,omitempty"`
	NewestAt  *time.Time       `json:"newest_at,omitempty"`
	TotalSize int64            `json:"total_size"`
}

// handleStats handles GET /api/v1/sandbox/stats
func (s *SandboxServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		sendError(w, http.StatusServiceUnavailable, "Sandbox storage not available")
		return
	}

	stats, err := s.storage.Stats(r.Context())
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get stats")
		return
	}

	response := SandboxStatsResponse{
		Total:     stats.Total,
		ByDomain:  stats.ByDomain,
		ByMode:    stats.ByMode,
		TotalSize: stats.TotalSize,
	}

	if !stats.OldestAt.IsZero() {
		response.OldestAt = &stats.OldestAt
	}
	if !stats.NewestAt.IsZero() {
		response.NewestAt = &stats.NewestAt
	}

	sendJSON(w, http.StatusOK, response)
}

// parseEmailData extracts headers and body from raw email data
func parseEmailData(data []byte) (headers map[string]string, body string, html string) {
	headers = make(map[string]string)
	content := string(data)

	// Split headers and body
	parts := splitHeadersBody(content)
	headerSection := parts[0]
	bodySection := ""
	if len(parts) > 1 {
		bodySection = parts[1]
	}

	// Parse headers
	var currentHeader string
	var currentValue string
	for _, line := range splitLines(headerSection) {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			// Continuation of previous header
			currentValue += " " + trimSpace(line)
		} else if idx := indexByte(line, ':'); idx > 0 {
			// Save previous header
			if currentHeader != "" {
				headers[currentHeader] = currentValue
			}
			currentHeader = line[:idx]
			currentValue = trimSpace(line[idx+1:])
		}
	}
	// Save last header
	if currentHeader != "" {
		headers[currentHeader] = currentValue
	}

	// Simple body extraction (not handling MIME properly for simplicity)
	body = bodySection
	if contentType, ok := headers["Content-Type"]; ok {
		if contains(contentType, "text/html") {
			html = bodySection
			body = ""
		}
	}

	return headers, body, html
}

func splitHeadersBody(s string) []string {
	// Find empty line that separates headers from body
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '\n' && (s[i+1] == '\n' || (i+2 < len(s) && s[i+1] == '\r' && s[i+2] == '\n')) {
			if s[i+1] == '\n' {
				return []string{s[:i], s[i+2:]}
			}
			return []string{s[:i], s[i+3:]}
		}
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			if i+2 < len(s) && s[i+2] == '\r' && i+3 < len(s) && s[i+3] == '\n' {
				return []string{s[:i], s[i+4:]}
			}
		}
	}
	return []string{s}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			end := i
			if end > start && s[end-1] == '\r' {
				end--
			}
			lines = append(lines, s[start:end])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr) >= 0)
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
