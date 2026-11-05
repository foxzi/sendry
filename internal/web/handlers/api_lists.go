package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
)

const (
	apiMaxRecipientsPerRequest = 1000
	apiMaxRecipientsBody       = 5 << 20 // 5 MiB
	apiMaxEmailLen             = 254
)

type apiRecipientIn struct {
	Email     string         `json:"email"`
	Name      string         `json:"name"`
	Variables map[string]any `json:"variables"`
}

type apiAddRecipientsRequest struct {
	Recipients     []apiRecipientIn `json:"recipients"`
	UpdateExisting bool             `json:"update_existing"`
}

type apiInvalidRecipient struct {
	Email string `json:"email"`
	Error string `json:"error"`
}

// validateAPIEmail checks an already normalised address: a bare addr-spec
// with a domain, no display name, no spaces.
func validateAPIEmail(email string) string {
	if email == "" {
		return "email is empty"
	}
	if len(email) > apiMaxEmailLen {
		return "email is too long"
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" {
		return "invalid email address"
	}
	at := strings.LastIndexByte(email, '@')
	if at < 1 || !strings.Contains(email[at+1:], ".") {
		return "invalid email address"
	}
	return ""
}

// APIListLists handles GET /api/v1/lists
func (h *Handlers) APIListLists(w http.ResponseWriter, r *http.Request) {
	lists, _, err := h.recipients.ListLists(models.RecipientListFilter{})
	if err != nil {
		h.logger.Error("api: failed to list recipient lists", "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	counts, err := h.recipients.ListCountsByList()
	if err != nil {
		h.logger.Error("api: failed to count recipients", "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	out := make([]map[string]any, 0, len(lists))
	for _, l := range lists {
		c := counts[l.ID]
		item := map[string]any{
			"id":           l.ID,
			"name":         l.Name,
			"total":        c.Total,
			"active":       c.Active,
			"unsubscribed": c.Unsubscribed,
		}
		if l.Description != "" {
			item["description"] = l.Description
		}
		out = append(out, item)
	}
	h.apiJSON(w, http.StatusOK, map[string]any{"lists": out})
}

// APIAddRecipients handles POST /api/v1/lists/{id}/recipients
func (h *Handlers) APIAddRecipients(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	list, err := h.recipients.GetListByID(listID)
	if err != nil {
		h.logger.Error("api: failed to get recipient list", "id", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	if list == nil {
		h.apiError(w, http.StatusNotFound, "List not found", "LIST_NOT_FOUND")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, apiMaxRecipientsBody)
	var req apiAddRecipientsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			h.apiError(w, http.StatusRequestEntityTooLarge, "Request body is too large", "BODY_TOO_LARGE")
			return
		}
		h.apiError(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}
	if len(req.Recipients) == 0 {
		h.apiError(w, http.StatusBadRequest, "recipients is required", "INVALID_REQUEST")
		return
	}
	if len(req.Recipients) > apiMaxRecipientsPerRequest {
		h.apiError(w, http.StatusRequestEntityTooLarge, "Too many recipients: at most 1000 per request", "TOO_MANY_RECIPIENTS")
		return
	}

	invalid := []apiInvalidRecipient{}
	order := []string{}
	byEmail := map[string]repository.RecipientUpsert{}
	for _, in := range req.Recipients {
		email := repository.NormalizeEmail(in.Email)
		if msg := validateAPIEmail(email); msg != "" {
			invalid = append(invalid, apiInvalidRecipient{Email: in.Email, Error: msg})
			continue
		}
		if _, seen := byEmail[email]; !seen {
			order = append(order, email)
		}
		// last occurrence in the request wins
		byEmail[email] = repository.RecipientUpsert{
			Email:     email,
			Name:      strings.TrimSpace(in.Name),
			Variables: in.Variables,
		}
	}
	items := make([]repository.RecipientUpsert, 0, len(order))
	for _, e := range order {
		items = append(items, byEmail[e])
	}

	res, err := h.recipients.UpsertRecipients(listID, items, req.UpdateExisting)
	if err != nil {
		h.logger.Error("api: failed to add recipients", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}

	h.auditAPI(r, "api_recipients_add", listID,
		"added="+itoa(res.Added)+" updated="+itoa(res.Updated)+" existing="+itoa(res.Existing)+
			" unsubscribed="+itoa(res.Unsubscribed)+" invalid="+itoa(len(invalid)))

	h.apiJSON(w, http.StatusOK, map[string]any{
		"added":        res.Added,
		"updated":      res.Updated,
		"existing":     res.Existing,
		"unsubscribed": res.Unsubscribed,
		"invalid":      invalid,
	})
}

// APIDeleteRecipient handles DELETE /api/v1/lists/{id}/recipients/{email}
func (h *Handlers) APIDeleteRecipient(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	email := r.PathValue("email")
	if u, err := url.PathUnescape(email); err == nil {
		email = u
	}

	err := h.recipients.DeleteRecipientByEmail(listID, email)
	switch {
	case err == nil:
		h.auditAPI(r, "api_recipient_delete", listID, "email="+repository.NormalizeEmail(email))
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, repository.ErrRecipientNotInList):
		h.apiError(w, http.StatusNotFound, "Recipient not found in this list", "NOT_FOUND")
	case errors.Is(err, repository.ErrRecipientUnsubscribed):
		h.apiError(w, http.StatusConflict, "Recipient is unsubscribed; it is kept so it is not added back as active", "RECIPIENT_UNSUBSCRIBED")
	case errors.Is(err, repository.ErrInUse):
		h.apiError(w, http.StatusConflict, "Cannot delete the recipient: campaigns were sent to this address", "RECIPIENT_IN_USE")
	default:
		h.logger.Error("api: failed to delete recipient", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
	}
}

// auditAPI writes an audit entry for an API-key action.
func (h *Handlers) auditAPI(r *http.Request, action, listID, details string) {
	keyID := ""
	if k := middleware.GetAPIKeyFromContext(r); k != nil {
		keyID = k.ID
		details = "api_key=" + k.Name + " " + details
	}
	h.settings.LogAction(r, keyID, "api-key", action, "recipient_list", listID, details)
}

func itoa(n int) string { return strconv.Itoa(n) }
