package handlers

import (
	"net/http"
)

func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	h.render(w, "settings/index", nil)
}

func (h *Handlers) GlobalVariables(w http.ResponseWriter, r *http.Request) {
	h.render(w, "settings/variables", nil)
}

func (h *Handlers) GlobalVariablesUpdate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/settings/variables", http.StatusSeeOther)
}

func (h *Handlers) UserList(w http.ResponseWriter, r *http.Request) {
	h.render(w, "settings/users", nil)
}

func (h *Handlers) AuditLog(w http.ResponseWriter, r *http.Request) {
	h.render(w, "settings/audit", nil)
}

func (h *Handlers) Monitoring(w http.ResponseWriter, r *http.Request) {
	h.render(w, "monitoring/index", nil)
}
