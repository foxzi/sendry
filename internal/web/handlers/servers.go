package handlers

import (
	"net/http"
)

// ServerList shows all configured Sendry servers
func (h *Handlers) ServerList(w http.ResponseWriter, r *http.Request) {
	servers := make([]map[string]any, 0, len(h.cfg.Sendry.Servers))
	for _, s := range h.cfg.Sendry.Servers {
		servers = append(servers, map[string]any{
			"Name":   s.Name,
			"URL":    s.BaseURL,
			"Env":    s.Env,
			"Online": true, // TODO: actual health check via Sendry API
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

	var server map[string]any
	for _, s := range h.cfg.Sendry.Servers {
		if s.Name == name {
			server = map[string]any{
				"Name": s.Name,
				"URL":  s.BaseURL,
				"Env":  s.Env,
			}
			break
		}
	}

	if server == nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	// TODO: fetch actual stats from Sendry API
	data := map[string]any{
		"Title":  server["Name"],
		"Active": "servers",
		"User":   h.getUserFromContext(r),
		"Server": server,
		"Stats": map[string]any{
			"QueueSize":   0,
			"DLQSize":     0,
			"Delivered":   0,
			"Failed":      0,
			"RateLimited": 0,
		},
	}

	h.render(w, "server_view", data)
}

// ServerQueue shows the queue for a server
func (h *Handlers) ServerQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// TODO: fetch queue from Sendry API
	data := map[string]any{
		"Title":      name + " - Queue",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Messages":   []any{},
		"Total":      0,
	}

	h.render(w, "server_queue", data)
}

// ServerDLQ shows the dead letter queue for a server
func (h *Handlers) ServerDLQ(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// TODO: fetch DLQ from Sendry API
	data := map[string]any{
		"Title":      name + " - Dead Letter Queue",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Messages":   []any{},
		"Total":      0,
	}

	h.render(w, "server_dlq", data)
}

// ServerDomains shows domains configured on a server
func (h *Handlers) ServerDomains(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// TODO: fetch domains from Sendry API
	data := map[string]any{
		"Title":      name + " - Domains",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
		"Domains":    []any{},
	}

	h.render(w, "server_domains", data)
}

// ServerSandbox provides sandbox testing interface
func (h *Handlers) ServerSandbox(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	data := map[string]any{
		"Title":      name + " - Sandbox",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": name,
	}

	if r.Method == http.MethodPost {
		// TODO: send test email via Sendry API
		data["Success"] = "Test email sent!"
	}

	h.render(w, "server_sandbox", data)
}
