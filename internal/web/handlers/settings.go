package handlers

import (
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/models"
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

	data := map[string]any{
		"Title":      "Monitoring",
		"Active":     "monitoring",
		"User":       h.getUserFromContext(r),
		"Servers":    servers,
		"ActiveJobs": activeJobs,
	}

	h.render(w, "monitoring", data)
}
