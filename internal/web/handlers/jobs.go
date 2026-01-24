package handlers

import (
	"net/http"
)

func (h *Handlers) JobList(w http.ResponseWriter, r *http.Request) {
	h.render(w, "jobs/list", nil)
}

func (h *Handlers) JobView(w http.ResponseWriter, r *http.Request) {
	h.render(w, "jobs/view", nil)
}

func (h *Handlers) JobItems(w http.ResponseWriter, r *http.Request) {
	h.render(w, "jobs/items", nil)
}

func (h *Handlers) JobPause(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (h *Handlers) JobResume(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (h *Handlers) JobCancel(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (h *Handlers) JobRetry(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}
