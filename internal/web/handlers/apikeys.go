package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
)

// APIKeysList shows the API keys management page
func (h *Handlers) APIKeysList(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	filter := models.APIKeyFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	}

	keys, total, err := h.apiKeys.List(filter)
	if err != nil {
		h.logger.Error("failed to list API keys", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load API keys")
		return
	}

	totalPages := (total + limit - 1) / limit

	// Check for newly created key in query params
	newKey := r.URL.Query().Get("new_key")

	data := map[string]any{
		"Title":      "API Keys",
		"Active":     "settings",
		"User":       h.getUserFromContext(r),
		"APIKeys":    keys,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
		"NewKey":     newKey,
	}

	h.render(w, "apikeys_list", data)
}

// APIKeyCreate creates a new API key
func (h *Handlers) APIKeyCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	name := r.FormValue("name")
	if name == "" {
		h.error(w, http.StatusBadRequest, "Name is required")
		return
	}

	// Parse expiration
	var expiresAt *time.Time
	if exp := r.FormValue("expires_at"); exp != "" {
		t, err := time.Parse("2006-01-02", exp)
		if err == nil {
			expiresAt = &t
		}
	}

	// Parse rate limits
	rateLimitMinute, _ := strconv.Atoi(r.FormValue("rate_limit_minute"))
	rateLimitHour, _ := strconv.Atoi(r.FormValue("rate_limit_hour"))

	user := h.getUserFromContext(r)
	opts := repository.APIKeyCreateOptions{
		Name:            name,
		CreatedBy:       user["Email"],
		Permissions:     []string{"send"},
		ExpiresAt:       expiresAt,
		RateLimitMinute: rateLimitMinute,
		RateLimitHour:   rateLimitHour,
	}

	result, err := h.apiKeys.Create(opts)
	if err != nil {
		h.logger.Error("failed to create API key", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create API key")
		return
	}

	h.logger.Info("API key created", "id", result.ID, "name", name, "user", user["Email"])

	// Redirect with the new key shown (only time it's visible)
	http.Redirect(w, r, "/settings/api-keys?new_key="+result.Key, http.StatusSeeOther)
}

// APIKeyDelete deletes an API key
func (h *Handlers) APIKeyDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.error(w, http.StatusBadRequest, "API key ID is required")
		return
	}

	if err := h.apiKeys.Delete(id); err != nil {
		h.logger.Error("failed to delete API key", "id", id, "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete API key")
		return
	}

	user := h.getUserFromContext(r)
	h.logger.Info("API key deleted", "id", id, "user", user["Email"])

	http.Redirect(w, r, "/settings/api-keys", http.StatusSeeOther)
}

// APIKeyToggle toggles the active status of an API key
func (h *Handlers) APIKeyToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.error(w, http.StatusBadRequest, "API key ID is required")
		return
	}

	newActive, err := h.apiKeys.ToggleActive(id)
	if err != nil {
		h.logger.Error("failed to toggle API key", "id", id, "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to toggle API key")
		return
	}

	user := h.getUserFromContext(r)
	action := "deactivated"
	if newActive {
		action = "activated"
	}
	h.logger.Info("API key "+action, "id", id, "user", user["Email"])

	http.Redirect(w, r, "/settings/api-keys", http.StatusSeeOther)
}
