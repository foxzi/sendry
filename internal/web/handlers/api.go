package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/router"
)

// APIErrorResponse represents an API error
type APIErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// APISend handles POST /api/v1/send
func (h *Handlers) APISend(w http.ResponseWriter, r *http.Request) {
	var req router.APISendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.apiError(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}

	// Get API key from context
	apiKey := middleware.GetAPIKeyFromContext(r)
	apiKeyID := ""
	if apiKey != nil {
		apiKeyID = apiKey.ID
	}

	// Get client IP
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = forwarded
	}

	// Send through router
	resp, err := h.router.Send(r.Context(), &req, apiKeyID, clientIP)
	if err != nil {
		switch {
		case errors.Is(err, router.ErrDomainNotFound):
			h.apiError(w, http.StatusBadRequest, "Domain not configured", "DOMAIN_NOT_FOUND")
		case errors.Is(err, router.ErrNoServersAvailable):
			h.apiError(w, http.StatusServiceUnavailable, "No healthy servers available", "NO_SERVERS")
		case errors.Is(err, router.ErrAllServersFailed):
			h.apiError(w, http.StatusBadGateway, "All servers failed to send", "SEND_FAILED")
		case errors.Is(err, router.ErrTemplateNotFound):
			h.apiError(w, http.StatusNotFound, "Template not found", "TEMPLATE_NOT_FOUND")
		case errors.Is(err, router.ErrInvalidRequest), strings.Contains(err.Error(), "invalid request"):
			h.apiError(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		default:
			h.logger.Error("send failed", "error", err)
			h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	h.apiJSON(w, http.StatusAccepted, resp)
}

// APISendTemplate handles POST /api/v1/send/template
func (h *Handlers) APISendTemplate(w http.ResponseWriter, r *http.Request) {
	// Same as APISend - template is handled by the router based on request fields
	h.APISend(w, r)
}

// APIGetStatus handles GET /api/v1/send/{id}/status
func (h *Handlers) APIGetStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.apiError(w, http.StatusBadRequest, "Send ID is required", "MISSING_ID")
		return
	}

	send, err := h.sends.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get send", "id", id, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Failed to get send status", "INTERNAL_ERROR")
		return
	}

	if send == nil {
		h.apiError(w, http.StatusNotFound, "Send not found", "NOT_FOUND")
		return
	}

	h.apiJSON(w, http.StatusOK, map[string]any{
		"id":            send.ID,
		"status":        send.Status,
		"server_name":   send.ServerName,
		"server_msg_id": send.ServerMsgID,
		"from":          send.FromAddress,
		"to":            send.ToAddresses,
		"subject":       send.Subject,
		"created_at":    send.CreatedAt,
		"sent_at":       send.SentAt,
		"error":         send.ErrorMessage,
	})
}

// apiJSON sends a JSON response
func (h *Handlers) apiJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode JSON", "error", err)
	}
}

// apiError sends an error response
func (h *Handlers) apiError(w http.ResponseWriter, status int, message, code string) {
	h.apiJSON(w, status, APIErrorResponse{
		Error: message,
		Code:  code,
	})
}

// GetAPIKeysRepository returns the API keys repository for middleware
func (h *Handlers) GetAPIKeysRepository() interface{} {
	return h.apiKeys
}
