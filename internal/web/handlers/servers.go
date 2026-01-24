package handlers

import (
	"net/http"

	"github.com/foxzi/sendry/internal/web/sendry"
)

// ServerList shows all configured Sendry servers
func (h *Handlers) ServerList(w http.ResponseWriter, r *http.Request) {
	statuses := h.sendry.GetAllStatus(r.Context())
	servers := make([]map[string]any, 0, len(statuses))
	for _, s := range statuses {
		servers = append(servers, map[string]any{
			"Name":      s.Name,
			"BaseURL":   s.BaseURL,
			"Env":       s.Env,
			"Online":    s.Online,
			"Version":   s.Version,
			"QueueSize": s.QueueSize,
			"Error":     s.Error,
		})
	}

	data := map[string]any{
		"Title":   "Servers",
		"Active":  "servers",
		"User":    h.getUserFromContext(r),
		"Servers": servers,
	}

	h.render(w, "servers", data)
}

// ServerView shows details of a single server
func (h *Handlers) ServerView(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	status, err := h.sendry.GetServerStatus(r.Context(), name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	server := map[string]any{
		"Name":    status.Name,
		"BaseURL": status.BaseURL,
		"Env":     status.Env,
		"Online":  status.Online,
		"Version": status.Version,
		"Uptime":  status.Uptime,
	}

	// Get queue and DLQ stats
	var queueSize, dlqSize int
	client, err := h.sendry.GetClient(name)
	if err == nil {
		if queue, err := client.GetQueue(r.Context()); err == nil && queue.Stats != nil {
			queueSize = queue.Stats.Pending
		}
		if dlq, err := client.GetDLQ(r.Context()); err == nil && dlq.Stats != nil {
			dlqSize = dlq.Stats.Total
		}
	}

	data := map[string]any{
		"Title":  status.Name,
		"Active": "servers",
		"User":   h.getUserFromContext(r),
		"Server": server,
		"Stats": map[string]any{
			"QueueSize": queueSize,
			"DLQSize":   dlqSize,
		},
	}

	h.render(w, "server_view", data)
}

// ServerQueue shows the queue for a server
func (h *Handlers) ServerQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	var messages []map[string]any
	var total int

	queue, err := client.GetQueue(r.Context())
	if err == nil {
		for _, m := range queue.Messages {
			messages = append(messages, map[string]any{
				"ID":        m.ID,
				"From":      m.From,
				"To":        m.To,
				"Subject":   m.Subject,
				"Status":    m.Status,
				"CreatedAt": m.CreatedAt,
			})
		}
		if queue.Stats != nil {
			total = queue.Stats.Pending
		}
	}

	data := map[string]any{
		"Title":      name + " - Queue",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Messages":   messages,
		"Total":      total,
		"Error":      errMsg(err),
	}

	h.render(w, "server_queue", data)
}

// ServerDLQ shows the dead letter queue for a server
func (h *Handlers) ServerDLQ(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	var messages []map[string]any
	var total int

	dlq, err := client.GetDLQ(r.Context())
	if err == nil {
		for _, m := range dlq.Messages {
			messages = append(messages, map[string]any{
				"ID":        m.ID,
				"From":      m.From,
				"To":        m.To,
				"Subject":   m.Subject,
				"Status":    m.Status,
				"CreatedAt": m.CreatedAt,
			})
		}
		if dlq.Stats != nil {
			total = dlq.Stats.Total
		}
	}

	data := map[string]any{
		"Title":      name + " - Dead Letter Queue",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Messages":   messages,
		"Total":      total,
		"Error":      errMsg(err),
	}

	h.render(w, "server_dlq", data)
}

// ServerDomains shows domains configured on a server
func (h *Handlers) ServerDomains(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	var domains []map[string]any

	resp, err := client.ListDomains(r.Context())
	if err == nil {
		for _, d := range resp.Domains {
			dkim := false
			if d.DKIM != nil {
				dkim = d.DKIM.Enabled
			}
			rateLimit := 0
			if d.RateLimit != nil {
				rateLimit = d.RateLimit.MessagesPerHour
			}
			domains = append(domains, map[string]any{
				"Name":      d.Domain,
				"Mode":      d.Mode,
				"DKIM":      dkim,
				"RateLimit": rateLimit,
			})
		}
	}

	data := map[string]any{
		"Title":      name + " - Domains",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Domains":    domains,
		"Error":      errMsg(err),
	}

	h.render(w, "server_domains", data)
}

// ServerSandbox provides sandbox testing interface
func (h *Handlers) ServerSandbox(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Title":      name + " - Sandbox",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data["Error"] = "Invalid form data"
			h.render(w, "server_sandbox", data)
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
			data["Success"] = "Email queued with ID: " + resp.ID
		}
	}

	h.render(w, "server_sandbox", data)
}

func errMsg(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
