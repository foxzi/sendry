package handlers

import (
	"net/http"
	"sync"

	"github.com/foxzi/sendry/internal/web/sendry"
)

// QueueOverview shows combined queue and DLQ from all servers
func (h *Handlers) QueueOverview(w http.ResponseWriter, r *http.Request) {
	servers := h.sendry.GetServers()

	type serverStats struct {
		Name      string
		QueueSize int
		DLQSize   int
		Error     string
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	stats := make([]serverStats, 0, len(servers))
	queueMessages := make([]map[string]any, 0)
	dlqMessages := make([]map[string]any, 0)
	totalQueue := 0
	totalDLQ := 0

	for _, srv := range servers {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			client, err := h.sendry.GetClient(name)
			if err != nil {
				mu.Lock()
				stats = append(stats, serverStats{Name: name, Error: err.Error()})
				mu.Unlock()
				return
			}

			st := serverStats{Name: name}

			// Fetch queue
			queue, err := client.GetQueue(r.Context())
			if err != nil {
				st.Error = err.Error()
			} else {
				if queue.Stats != nil {
					st.QueueSize = queue.Stats.Pending
				}
				var msgs []map[string]any
				for _, m := range queue.Messages {
					msgs = append(msgs, map[string]any{
						"ID":        m.ID,
						"From":      m.From,
						"To":        m.To,
						"Subject":   m.Subject,
						"Status":    m.Status,
						"CreatedAt": m.CreatedAt,
						"Server":    name,
					})
				}
				mu.Lock()
				queueMessages = append(queueMessages, msgs...)
				totalQueue += st.QueueSize
				mu.Unlock()
			}

			// Fetch DLQ
			dlq, err := client.GetDLQ(r.Context())
			if err == nil {
				if dlq.Stats != nil {
					st.DLQSize = dlq.Stats.Total
				}
				var msgs []map[string]any
				for _, m := range dlq.Messages {
					msgs = append(msgs, map[string]any{
						"ID":        m.ID,
						"From":      m.From,
						"To":        m.To,
						"Subject":   m.Subject,
						"Status":    m.Status,
						"CreatedAt": m.CreatedAt,
						"Server":    name,
					})
				}
				mu.Lock()
				dlqMessages = append(dlqMessages, msgs...)
				totalDLQ += st.DLQSize
				mu.Unlock()
			}

			mu.Lock()
			stats = append(stats, st)
			mu.Unlock()
		}(srv.Name)
	}
	wg.Wait()

	data := map[string]any{
		"Title":         "Queue",
		"Active":        "queue",
		"User":          h.getUserFromContext(r),
		"QueueMessages": queueMessages,
		"DLQMessages":   dlqMessages,
		"TotalQueue":    totalQueue,
		"TotalDLQ":      totalDLQ,
		"Servers":       stats,
	}

	h.render(w, "queue_overview", data)
}

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

// QueueMessageView shows details of a single queue message
func (h *Handlers) QueueMessageView(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	status, err := client.GetStatus(r.Context(), id)
	if err != nil {
		http.Error(w, "Message not found: "+err.Error(), http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Title":      "Message " + id[:8] + "...",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Message":    status,
	}

	h.render(w, "server_queue_view", data)
}

// QueueMessageDelete deletes a message from queue
func (h *Handlers) QueueMessageDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	if err := client.DeleteFromQueue(r.Context(), id); err != nil {
		h.logger.Error("failed to delete queue message", "server", name, "id", id, "error", err)
	}

	http.Redirect(w, r, "/servers/"+name+"/queue", http.StatusSeeOther)
}

// DLQMessageRetry retries a message from DLQ
func (h *Handlers) DLQMessageRetry(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	if err := client.RetryDLQ(r.Context(), id); err != nil {
		h.logger.Error("failed to retry DLQ message", "server", name, "id", id, "error", err)
	}

	http.Redirect(w, r, "/servers/"+name+"/dlq", http.StatusSeeOther)
}

// DLQMessageDelete deletes a message from DLQ
func (h *Handlers) DLQMessageDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	if err := client.DeleteFromDLQ(r.Context(), id); err != nil {
		h.logger.Error("failed to delete DLQ message", "server", name, "id", id, "error", err)
	}

	http.Redirect(w, r, "/servers/"+name+"/dlq", http.StatusSeeOther)
}

// QueuePurge deletes all messages from queue
func (h *Handlers) QueuePurge(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	deleted, err := client.PurgeQueue(r.Context())
	if err != nil {
		h.logger.Error("failed to purge queue", "server", name, "error", err)
	} else {
		h.logger.Info("queue purged", "server", name, "deleted", deleted)
	}

	http.Redirect(w, r, "/servers/"+name+"/queue", http.StatusSeeOther)
}

// DLQPurge deletes all messages from DLQ
func (h *Handlers) DLQPurge(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	client, err := h.sendry.GetClient(name)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	deleted, err := client.PurgeDLQ(r.Context())
	if err != nil {
		h.logger.Error("failed to purge DLQ", "server", name, "error", err)
	} else {
		h.logger.Info("DLQ purged", "server", name, "deleted", deleted)
	}

	http.Redirect(w, r, "/servers/"+name+"/dlq", http.StatusSeeOther)
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

	// Get all servers for selection
	servers := h.getServersStatus()

	data := map[string]any{
		"Title":      "Send Test Email",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Servers":    servers,
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data["Error"] = "Invalid form data"
			h.render(w, "server_sandbox", data)
			return
		}

		// Use selected server from form, fallback to URL param
		selectedServer := r.FormValue("server")
		if selectedServer == "" {
			selectedServer = name
		}
		data["ServerName"] = selectedServer

		client, err := h.sendry.GetClient(selectedServer)
		if err != nil {
			data["Error"] = "Server not found: " + selectedServer
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
			data["Success"] = "Email queued on " + selectedServer + " with ID: " + resp.ID
		}
	} else {
		// Verify server exists for GET
		if _, err := h.sendry.GetClient(name); err != nil {
			http.Error(w, "Server not found", http.StatusNotFound)
			return
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
