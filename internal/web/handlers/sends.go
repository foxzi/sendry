package handlers

import (
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/stats"
)

// SendsList shows the send history list
func (h *Handlers) SendsList(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	domain := r.URL.Query().Get("domain")
	server := r.URL.Query().Get("server")
	errorType := stats.ParseErrorType(r.URL.Query().Get("error_type"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 50
	offset := (page - 1) * limit

	filter := models.SendFilter{
		Search:       search,
		Status:       status,
		SenderDomain: domain,
		ServerName:   server,
		ErrorType:    errorType,
		Limit:        limit,
		Offset:       offset,
	}

	sends, total, err := h.sends.List(filter)
	if err != nil {
		h.logger.Error("failed to list sends", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load sends")
		return
	}

	// Get stats
	sendStats, _ := h.sends.GetStats(models.SendFilter{})
	errorCounts, err := h.sends.ErrorCounts()
	if err != nil {
		h.logger.Error("failed to count send errors", "error", err)
	}

	// Get unique domains and servers for filters
	domains, _ := h.sends.GetDomains()
	servers, _ := h.sends.GetServers()

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "API sends",
		"Active":     "sends",
		"User":       h.getUserFromContext(r),
		"Sends":      sends,
		"Stats":      sendStats,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
		"Status":     status,
		"Domain":     domain,
		"Server":     server,
		"Domains":    domains,
		"Servers":    servers,
		"Errors":     summarizeErrors(errorCounts),
		"ErrorType":  errorType,
		"ErrorTypes": stats.ErrorTypes,
	}

	h.render(w, "sends_list", data)
}

// SendView shows details of a single send
func (h *Handlers) SendView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.error(w, http.StatusBadRequest, "Send ID is required")
		return
	}

	send, err := h.sends.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get send", "id", id, "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load send")
		return
	}

	if send == nil {
		h.error(w, http.StatusNotFound, "Send not found")
		return
	}

	// Parse addresses
	toAddresses := repository.FromJSON(send.ToAddresses)
	ccAddresses := repository.FromJSON(send.CCAddresses)
	bccAddresses := repository.FromJSON(send.BCCAddresses)

	// Get template name if exists
	var templateName string
	if send.TemplateID != "" {
		tmpl, _ := h.templates.GetByID(send.TemplateID)
		if tmpl != nil {
			templateName = tmpl.Name
		}
	}

	// Get API key name if exists
	var apiKeyName string
	if send.APIKeyID != "" {
		key, _ := h.apiKeys.GetByID(send.APIKeyID)
		if key != nil {
			apiKeyName = key.Name
		}
	}

	events, err := h.tracking.SourceEvents(models.SourceSend, id, 200)
	if err != nil {
		h.logger.Error("failed to load send events", "id", id, "error", err)
	}

	data := map[string]any{
		"Title":        "Send details",
		"Events":       events,
		"Active":       "sends",
		"User":         h.getUserFromContext(r),
		"Send":         send,
		"ToAddresses":  toAddresses,
		"CCAddresses":  ccAddresses,
		"BCCAddresses": bccAddresses,
		"TemplateName": templateName,
		"APIKeyName":   apiKeyName,
	}

	h.render(w, "send_view", data)
}
