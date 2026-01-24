package handlers

import (
	"net/http"
)

func (h *Handlers) RecipientListList(w http.ResponseWriter, r *http.Request) {
	h.render(w, "recipients/list", nil)
}

func (h *Handlers) RecipientListNew(w http.ResponseWriter, r *http.Request) {
	h.render(w, "recipients/new", nil)
}

func (h *Handlers) RecipientListCreate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/recipients", http.StatusSeeOther)
}

func (h *Handlers) RecipientListView(w http.ResponseWriter, r *http.Request) {
	h.render(w, "recipients/view", nil)
}

func (h *Handlers) RecipientListEdit(w http.ResponseWriter, r *http.Request) {
	h.render(w, "recipients/edit", nil)
}

func (h *Handlers) RecipientListUpdate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/recipients", http.StatusSeeOther)
}

func (h *Handlers) RecipientListDelete(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/recipients", http.StatusSeeOther)
}

func (h *Handlers) RecipientImportPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "recipients/import", nil)
}

func (h *Handlers) RecipientImport(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/recipients", http.StatusSeeOther)
}

func (h *Handlers) RecipientsList(w http.ResponseWriter, r *http.Request) {
	h.render(w, "recipients/recipients", nil)
}
