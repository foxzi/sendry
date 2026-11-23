package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

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
	email := r.PathValue("email") // already unescaped by the mux

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

// APIUnsubscribeRecipient handles POST /api/v1/lists/{id}/recipients/{email}/unsubscribe:
// the address stays in the list but no campaign is sent to it any more. The
// same address in other lists is untouched. Repeating the call is safe.
func (h *Handlers) APIUnsubscribeRecipient(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	email := repository.NormalizeEmail(r.PathValue("email")) // already unescaped by the mux

	status, err := h.recipients.RecipientStatusByEmail(listID, email)
	switch {
	case errors.Is(err, repository.ErrRecipientNotInList):
		h.apiError(w, http.StatusNotFound, "Recipient not found in this list", "NOT_FOUND")
		return
	case err != nil:
		h.logger.Error("api: failed to load recipient", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}

	resp := map[string]any{"email": email, "status": models.RecipientStatusUnsubscribed, "changed": false}
	switch status {
	case models.RecipientStatusUnsubscribed:
		// keep the journal row of the first unsubscribe (who, when, which campaign)
		h.apiJSON(w, http.StatusOK, resp)
		return
	case "", "active":
	default:
		// e.g. bounced: turning it into unsubscribed would let it be resubscribed to active
		h.apiError(w, http.StatusConflict, "Recipient is not active (status: "+status+")", "RECIPIENT_NOT_ACTIVE")
		return
	}

	u := &models.Unsubscribe{ListID: listID, Email: email, SourceType: models.UnsubSourceAPI}
	if k := middleware.GetAPIKeyFromContext(r); k != nil {
		u.SourceID = k.ID
	}
	// the status is checked again together with the write: a reader may have
	// unsubscribed in between, and their journal row must stay
	changed, err := h.unsubs.AddIfActive(u)
	switch {
	case errors.Is(err, repository.ErrRecipientNotInList): // deleted in between
		h.apiError(w, http.StatusNotFound, "Recipient not found in this list", "NOT_FOUND")
		return
	case err != nil:
		h.logger.Error("api: failed to unsubscribe recipient", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	if !changed {
		// lost a race: answer by what the address is now
		if now, _ := h.recipients.RecipientStatusByEmail(listID, email); now != models.RecipientStatusUnsubscribed {
			h.apiError(w, http.StatusConflict, "Recipient is not active (status: "+now+")", "RECIPIENT_NOT_ACTIVE")
			return
		}
		h.apiJSON(w, http.StatusOK, resp)
		return
	}
	h.auditAPI(r, "api_recipient_unsubscribe", listID, "email="+email)
	resp["changed"] = true
	h.apiJSON(w, http.StatusOK, resp)
}

// APIGetRecipient handles GET /api/v1/lists/{id}/recipients/{email}: the
// record of the address in the list with its status.
func (h *Handlers) APIGetRecipient(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	rec, err := h.recipients.GetRecipientByEmail(listID, r.PathValue("email"))
	if err != nil {
		h.logger.Error("api: failed to load recipient", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	if rec == nil {
		h.apiError(w, http.StatusNotFound, "Recipient not found in this list", "NOT_FOUND")
		return
	}

	status := rec.Status
	if status == "" {
		status = "active"
	}
	// decoded so that a long number keeps its digits
	variables := storedVariables(rec.Variables)
	if variables == nil {
		variables = map[string]any{}
	}
	tags := recipientTags(rec.Tags)
	if tags == nil {
		tags = []string{}
	}
	resp := map[string]any{
		"email":      repository.NormalizeEmail(rec.Email),
		"name":       rec.Name,
		"status":     status,
		"variables":  variables,
		"tags":       tags,
		"created_at": rec.CreatedAt.UTC().Format(time.RFC3339),
	}
	if status == models.RecipientStatusUnsubscribed {
		u, err := h.unsubs.Get(listID, rec.Email)
		if err != nil {
			h.logger.Error("api: failed to load unsubscribe", "list", listID, "error", err)
		}
		if u != nil {
			resp["unsubscribed_at"] = u.CreatedAt.UTC().Format(time.RFC3339)
			resp["unsubscribed_by"] = u.SourceType
		}
	}
	h.apiJSON(w, http.StatusOK, resp)
}

const apiNotUnsubscribedByAPI = "The address was not unsubscribed through the API; resubscribe it on the address card in the panel"

// APIResubscribeRecipient handles POST /api/v1/lists/{id}/recipients/{email}/resubscribe:
// an address unsubscribed through the API becomes active again. Any other
// unsubscribe is not undone through the API: the reader's own (the link in
// a message, the button of the mail client) and one made by hand in the
// panel, which is often an operator acting on the reader's request. Those
// take a person on the address card. Repeating the call is safe.
func (h *Handlers) APIResubscribeRecipient(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	email := repository.NormalizeEmail(r.PathValue("email"))

	status, err := h.recipients.RecipientStatusByEmail(listID, email)
	switch {
	case errors.Is(err, repository.ErrRecipientNotInList):
		h.apiError(w, http.StatusNotFound, "Recipient not found in this list", "NOT_FOUND")
		return
	case err != nil:
		h.logger.Error("api: failed to load recipient", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}

	resp := map[string]any{"email": email, "status": "active", "changed": false}
	switch status {
	case "", "active":
		h.apiJSON(w, http.StatusOK, resp)
		return
	case models.RecipientStatusUnsubscribed:
	default:
		h.apiError(w, http.StatusConflict, "Recipient is not unsubscribed (status: "+status+")", "RECIPIENT_NOT_UNSUBSCRIBED")
		return
	}

	u, err := h.unsubs.Get(listID, email)
	if err != nil {
		h.logger.Error("api: failed to load unsubscribe", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	// without a journal row nobody knows who unsubscribed: not the API's to undo
	if u == nil || u.SourceType != models.UnsubSourceAPI {
		h.apiError(w, http.StatusConflict, apiNotUnsubscribedByAPI, "NOT_UNSUBSCRIBED_BY_API")
		return
	}

	// the source is checked again together with the write: the reader may
	// have unsubscribed in between
	removed, err := h.unsubs.RemoveIfSource(listID, email, models.UnsubSourceAPI)
	if err != nil {
		h.logger.Error("api: failed to resubscribe recipient", "list", listID, "error", err)
		h.apiError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}
	if !removed {
		h.apiError(w, http.StatusConflict, apiNotUnsubscribedByAPI, "NOT_UNSUBSCRIBED_BY_API")
		return
	}
	h.auditAPI(r, "api_recipient_resubscribe", listID, "email="+email+" unsubscribed_by="+u.SourceType)
	resp["changed"] = true
	h.apiJSON(w, http.StatusOK, resp)
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
