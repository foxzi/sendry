package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/sendry"
)

// Settings shows settings overview
func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":  "Settings",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
	}

	h.render(w, "settings", data)
}

// GlobalVariables lists and manages global template variables
func (h *Handlers) GlobalVariables(w http.ResponseWriter, r *http.Request) {
	vars, err := h.settings.GetAllVariables()
	if err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to load variables")
		return
	}

	data := map[string]any{
		"Title":     "Global Variables",
		"Active":    "settings",
		"User":      h.getUserFromContext(r),
		"Variables": vars,
	}

	h.render(w, "settings_variables", data)
}

// GlobalVariablesUpdate updates global variables
func (h *Handlers) GlobalVariablesUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	action := r.FormValue("action")
	key := r.FormValue("key")
	value := r.FormValue("value")
	description := r.FormValue("description")

	switch action {
	case "delete":
		if err := h.settings.DeleteVariable(key); err != nil {
			h.error(w, http.StatusInternalServerError, "Failed to delete variable")
			return
		}
	default:
		if key == "" {
			h.error(w, http.StatusBadRequest, "Key is required")
			return
		}
		if err := h.settings.SetVariable(key, value, description); err != nil {
			h.error(w, http.StatusInternalServerError, "Failed to save variable")
			return
		}
	}

	http.Redirect(w, r, "/settings/variables", http.StatusSeeOther)
}

// UserList shows all users
func (h *Handlers) UserList(w http.ResponseWriter, r *http.Request) {
	users, err := h.settings.ListUsers()
	if err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to load users")
		return
	}

	data := map[string]any{
		"Title":  "Users",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
		"Users":  users,
	}

	h.render(w, "settings_users", data)
}

// AuditLog shows audit log entries
func (h *Handlers) AuditLog(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	filter := models.AuditLogFilter{
		UserID:     r.URL.Query().Get("user_id"),
		Action:     r.URL.Query().Get("action"),
		EntityType: r.URL.Query().Get("entity_type"),
		Limit:      limit,
		Offset:     offset,
	}

	entries, total, err := h.settings.ListAuditLog(filter)
	if err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to load audit log")
		return
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Audit Log",
		"Active":     "settings",
		"User":       h.getUserFromContext(r),
		"Entries":    entries,
		"Page":       page,
		"TotalPages": totalPages,
		"Total":      total,
		"Filter":     filter,
	}

	h.render(w, "settings_audit", data)
}

// SettingsTestEmail provides test email sending interface
func (h *Handlers) SettingsTestEmail(w http.ResponseWriter, r *http.Request) {
	servers := h.getServersStatus()

	data := map[string]any{
		"Title":   "Send Test Email",
		"Active":  "settings",
		"User":    h.getUserFromContext(r),
		"Servers": servers,
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data["Error"] = "Invalid form data"
			h.render(w, "settings_test_email", data)
			return
		}

		selectedServer := r.FormValue("server")
		if selectedServer == "" {
			data["Error"] = "Server is required"
			h.render(w, "settings_test_email", data)
			return
		}

		client, err := h.sendry.GetClient(selectedServer)
		if err != nil {
			data["Error"] = "Server not found: " + selectedServer
			h.render(w, "settings_test_email", data)
			return
		}

		req := &sendry.SendRequest{
			From:    r.FormValue("from"),
			To:      []string{r.FormValue("to")},
			Subject: r.FormValue("subject"),
		}

		if r.FormValue("html") == "1" {
			req.HTML = r.FormValue("body")
		} else {
			req.Body = r.FormValue("body")
		}

		resp, err := client.Send(r.Context(), req)
		if err != nil {
			data["Error"] = err.Error()
		} else {
			data["Success"] = "Email queued on " + selectedServer + " with ID: " + resp.ID
		}
	}

	h.render(w, "settings_test_email", data)
}

// Monitoring shows system monitoring overview
func (h *Handlers) Monitoring(w http.ResponseWriter, r *http.Request) {
	// Gather live stats from all servers
	statuses := h.sendry.GetAllStatus(r.Context())
	servers := make([]map[string]any, 0, len(statuses))

	for _, s := range statuses {
		serverData := map[string]any{
			"Name":      s.Name,
			"Env":       s.Env,
			"Online":    s.Online,
			"Version":   s.Version,
			"QueueSize": s.QueueSize,
			"DLQSize":   0,
			"Error":     s.Error,
		}

		// Get DLQ size if server is online
		if s.Online {
			if client, err := h.sendry.GetClient(s.Name); err == nil {
				if dlq, err := client.GetDLQ(r.Context()); err == nil && dlq.Stats != nil {
					serverData["DLQSize"] = dlq.Stats.Total
				}
			}
		}

		servers = append(servers, serverData)
	}

	// Get active jobs count
	var activeJobs int
	h.db.QueryRow("SELECT COUNT(*) FROM send_jobs WHERE status = 'running'").Scan(&activeJobs)

	// Get list of domains for filter dropdown
	domains, _ := h.sends.GetDomains()

	data := map[string]any{
		"Title":      "Monitoring",
		"Active":     "monitoring",
		"User":       h.getUserFromContext(r),
		"Servers":    servers,
		"ActiveJobs": activeJobs,
		"Domains":    domains,
	}

	h.render(w, "monitoring", data)
}

// MonitoringAPIStats returns JSON data for monitoring charts
func (h *Handlers) MonitoringAPIStats(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	domain := r.URL.Query().Get("domain")

	filter := models.MonitoringFilter{
		Period: period,
		Domain: domain,
	}

	// Get time series data
	timeSeries, err := h.sends.GetTimeSeries(filter)
	if err != nil {
		h.jsonError(w, "Failed to get time series", http.StatusInternalServerError)
		return
	}

	// Get domain stats
	domainStats, err := h.sends.GetDomainStats(filter)
	if err != nil {
		h.jsonError(w, "Failed to get domain stats", http.StatusInternalServerError)
		return
	}

	// Calculate totals
	var totalSent, totalFailed int
	for _, ds := range domainStats {
		totalSent += ds.Sent
		totalFailed += ds.Failed
	}

	data := models.MonitoringData{
		TimeSeries:  timeSeries,
		DomainStats: domainStats,
		Period:      period,
		TotalSent:   totalSent,
		TotalFailed: totalFailed,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// MonitoringAPIServers returns JSON data for server status
func (h *Handlers) MonitoringAPIServers(w http.ResponseWriter, r *http.Request) {
	statuses := h.sendry.GetAllStatus(r.Context())
	servers := make([]models.ServerStats, 0, len(statuses))

	for _, s := range statuses {
		serverData := models.ServerStats{
			Name:      s.Name,
			Env:       s.Env,
			Online:    s.Online,
			QueueSize: s.QueueSize,
			Error:     s.Error,
		}

		// Get DLQ size if server is online
		if s.Online {
			if client, err := h.sendry.GetClient(s.Name); err == nil {
				if dlq, err := client.GetDLQ(r.Context()); err == nil && dlq.Stats != nil {
					serverData.DLQSize = dlq.Stats.Total
				}
			}
		}

		servers = append(servers, serverData)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

// jsonError writes a JSON error response
func (h *Handlers) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
