package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/foxzi/sendry/internal/queue"
)

// SendRequest is the request body for POST /send
type SendRequest struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	Subject string            `json:"subject"`
	Body    string            `json:"body"`
	HTML    string            `json:"html,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SendResponse is the response for POST /send
type SendResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// StatusResponse is the response for GET /status/{id}
type StatusResponse struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	From       string    `json:"from"`
	To         []string  `json:"to"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	RetryCount int       `json:"retry_count"`
	LastError  string    `json:"last_error,omitempty"`
}

// QueueResponse is the response for GET /queue
type QueueResponse struct {
	Stats    *queue.QueueStats `json:"stats"`
	Messages []*MessageSummary `json:"messages,omitempty"`
}

// MessageSummary is a summary of a message
type MessageSummary struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// HealthResponse is the response for GET /health
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Uptime  string            `json:"uptime"`
	Queue   *queue.QueueStats `json:"queue"`
}

// ErrorResponse is the error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// handleSend handles POST /api/v1/send
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.From == "" {
		s.sendError(w, http.StatusBadRequest, "from is required")
		return
	}
	if len(req.To) == 0 {
		s.sendError(w, http.StatusBadRequest, "to is required")
		return
	}
	if req.Subject == "" && req.Body == "" && req.HTML == "" {
		s.sendError(w, http.StatusBadRequest, "subject, body or html is required")
		return
	}

	// Build email data (RFC 5322)
	data := s.buildEmailData(&req)

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
		s.logger.Error("failed to enqueue message", "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to queue message")
		return
	}

	s.logger.Info("message queued via API",
		"id", msg.ID,
		"from", msg.From,
		"to", msg.To,
	)

	s.sendJSON(w, http.StatusAccepted, SendResponse{
		ID:     msg.ID,
		Status: string(msg.Status),
	})
}

// handleStatus handles GET /api/v1/status/{id}
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	msg, err := s.queue.Get(r.Context(), id)
	if err != nil {
		s.logger.Error("failed to get message", "id", id, "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to get message")
		return
	}

	if msg == nil {
		s.sendError(w, http.StatusNotFound, "Message not found")
		return
	}

	s.sendJSON(w, http.StatusOK, StatusResponse{
		ID:         msg.ID,
		Status:     string(msg.Status),
		From:       msg.From,
		To:         msg.To,
		CreatedAt:  msg.CreatedAt,
		UpdatedAt:  msg.UpdatedAt,
		RetryCount: msg.RetryCount,
		LastError:  msg.LastError,
	})
}

// handleQueue handles GET /api/v1/queue
func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	stats, err := s.queue.Stats(r.Context())
	if err != nil {
		s.logger.Error("failed to get queue stats", "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to get queue stats")
		return
	}

	// Get recent messages
	messages, err := s.queue.List(r.Context(), queue.ListFilter{Limit: 100})
	if err != nil {
		s.logger.Error("failed to list messages", "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to list messages")
		return
	}

	summaries := make([]*MessageSummary, len(messages))
	for i, msg := range messages {
		summaries[i] = &MessageSummary{
			ID:        msg.ID,
			From:      msg.From,
			To:        msg.To,
			Status:    string(msg.Status),
			CreatedAt: msg.CreatedAt,
		}
	}

	s.sendJSON(w, http.StatusOK, QueueResponse{
		Stats:    stats,
		Messages: summaries,
	})
}

// handleDeleteMessage handles DELETE /api/v1/queue/{id}
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.queue.Delete(r.Context(), id); err != nil {
		s.logger.Error("failed to delete message", "id", id, "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to delete message")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	stats, _ := s.queue.Stats(r.Context())

	s.sendJSON(w, http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: "0.1.1",
		Uptime:  time.Since(s.startTime).String(),
		Queue:   stats,
	})
}

// buildEmailData constructs RFC 5322 email data
func (s *Server) buildEmailData(req *SendRequest) []byte {
	var buf bytes.Buffer

	// Headers
	buf.WriteString(fmt.Sprintf("From: %s\r\n", req.From))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(req.To, ", ")))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", req.Subject))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString(fmt.Sprintf("Message-ID: <%s@%s>\r\n", uuid.New().String(), extractDomain(req.From)))

	// Custom headers
	for k, v := range req.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	// MIME headers
	if req.HTML != "" {
		boundary := uuid.New().String()
		buf.WriteString("MIME-Version: 1.0\r\n")
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
		buf.WriteString("\r\n")

		// Plain text part
		if req.Body != "" {
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
			buf.WriteString("\r\n")
			buf.WriteString(req.Body)
			buf.WriteString("\r\n")
		}

		// HTML part
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(req.HTML)
		buf.WriteString("\r\n")

		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(req.Body)
	}

	return buf.Bytes()
}

// sendJSON sends a JSON response
func (s *Server) sendJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// sendError sends an error response
func (s *Server) sendError(w http.ResponseWriter, status int, message string) {
	s.sendJSON(w, status, ErrorResponse{Error: message})
}

// extractDomain extracts domain from email address
func extractDomain(email string) string {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		parts := strings.Split(email, "@")
		if len(parts) == 2 {
			return parts[1]
		}
		return "localhost"
	}
	parts := strings.Split(addr.Address, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return "localhost"
}

// Dead Letter Queue handlers

// DLQResponse is the response for GET /api/v1/dlq
type DLQResponse struct {
	Stats    *queue.DLQStats  `json:"stats"`
	Messages []*MessageSummary `json:"messages,omitempty"`
}

// handleDLQ handles GET /api/v1/dlq
func (s *Server) handleDLQ(w http.ResponseWriter, r *http.Request) {
	storage, ok := s.queue.(*queue.BoltStorage)
	if !ok {
		s.sendError(w, http.StatusInternalServerError, "DLQ not supported")
		return
	}

	stats, err := storage.DLQStats(r.Context())
	if err != nil {
		s.logger.Error("failed to get DLQ stats", "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to get DLQ stats")
		return
	}

	messages, err := storage.ListDLQ(r.Context(), 100, 0)
	if err != nil {
		s.logger.Error("failed to list DLQ messages", "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to list DLQ messages")
		return
	}

	summaries := make([]*MessageSummary, len(messages))
	for i, msg := range messages {
		summaries[i] = &MessageSummary{
			ID:        msg.ID,
			From:      msg.From,
			To:        msg.To,
			Status:    string(msg.Status),
			CreatedAt: msg.CreatedAt,
		}
	}

	s.sendJSON(w, http.StatusOK, DLQResponse{
		Stats:    stats,
		Messages: summaries,
	})
}

// handleDLQGet handles GET /api/v1/dlq/{id}
func (s *Server) handleDLQGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	storage, ok := s.queue.(*queue.BoltStorage)
	if !ok {
		s.sendError(w, http.StatusInternalServerError, "DLQ not supported")
		return
	}

	msg, err := storage.GetFromDLQ(r.Context(), id)
	if err != nil {
		s.logger.Error("failed to get DLQ message", "id", id, "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to get DLQ message")
		return
	}

	if msg == nil {
		s.sendError(w, http.StatusNotFound, "Message not found in DLQ")
		return
	}

	s.sendJSON(w, http.StatusOK, StatusResponse{
		ID:         msg.ID,
		Status:     string(msg.Status),
		From:       msg.From,
		To:         msg.To,
		CreatedAt:  msg.CreatedAt,
		UpdatedAt:  msg.UpdatedAt,
		RetryCount: msg.RetryCount,
		LastError:  msg.LastError,
	})
}

// handleDLQRetry handles POST /api/v1/dlq/{id}/retry
func (s *Server) handleDLQRetry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	storage, ok := s.queue.(*queue.BoltStorage)
	if !ok {
		s.sendError(w, http.StatusInternalServerError, "DLQ not supported")
		return
	}

	if err := storage.RetryFromDLQ(r.Context(), id); err != nil {
		s.logger.Error("failed to retry DLQ message", "id", id, "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to retry message")
		return
	}

	s.logger.Info("message retried from DLQ", "id", id)
	s.sendJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Message moved to pending queue",
	})
}

// handleDLQDelete handles DELETE /api/v1/dlq/{id}
func (s *Server) handleDLQDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		s.sendError(w, http.StatusBadRequest, "id is required")
		return
	}

	storage, ok := s.queue.(*queue.BoltStorage)
	if !ok {
		s.sendError(w, http.StatusInternalServerError, "DLQ not supported")
		return
	}

	if err := storage.DeleteFromDLQ(r.Context(), id); err != nil {
		s.logger.Error("failed to delete DLQ message", "id", id, "error", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to delete message")
		return
	}

	s.logger.Info("message deleted from DLQ", "id", id)
	w.WriteHeader(http.StatusNoContent)
}
