package handlers

import (
	"net/http"
)

func (h *Handlers) TemplateList(w http.ResponseWriter, r *http.Request) {
	h.render(w, "templates/list", nil)
}

func (h *Handlers) TemplateNew(w http.ResponseWriter, r *http.Request) {
	h.render(w, "templates/new", nil)
}

func (h *Handlers) TemplateCreate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/templates", http.StatusSeeOther)
}

func (h *Handlers) TemplateView(w http.ResponseWriter, r *http.Request) {
	h.render(w, "templates/view", nil)
}

func (h *Handlers) TemplateEdit(w http.ResponseWriter, r *http.Request) {
	h.render(w, "templates/edit", nil)
}

func (h *Handlers) TemplateUpdate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/templates", http.StatusSeeOther)
}

func (h *Handlers) TemplateDelete(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/templates", http.StatusSeeOther)
}

func (h *Handlers) TemplateVersions(w http.ResponseWriter, r *http.Request) {
	h.render(w, "templates/versions", nil)
}

func (h *Handlers) TemplateDeploy(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/templates", http.StatusSeeOther)
}

func (h *Handlers) TemplatePreview(w http.ResponseWriter, r *http.Request) {
	h.render(w, "templates/preview", nil)
}
