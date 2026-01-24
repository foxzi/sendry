package handlers

import (
	"net/http"
)

func (h *Handlers) ServerList(w http.ResponseWriter, r *http.Request) {
	h.render(w, "servers/list", nil)
}

func (h *Handlers) ServerView(w http.ResponseWriter, r *http.Request) {
	h.render(w, "servers/view", nil)
}

func (h *Handlers) ServerQueue(w http.ResponseWriter, r *http.Request) {
	h.render(w, "servers/queue", nil)
}

func (h *Handlers) ServerDLQ(w http.ResponseWriter, r *http.Request) {
	h.render(w, "servers/dlq", nil)
}

func (h *Handlers) ServerDomains(w http.ResponseWriter, r *http.Request) {
	h.render(w, "servers/domains", nil)
}

func (h *Handlers) ServerSandbox(w http.ResponseWriter, r *http.Request) {
	h.render(w, "servers/sandbox", nil)
}
