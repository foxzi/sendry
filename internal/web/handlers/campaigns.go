package handlers

import (
	"net/http"
)

func (h *Handlers) CampaignList(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/list", nil)
}

func (h *Handlers) CampaignNew(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/new", nil)
}

func (h *Handlers) CampaignCreate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/campaigns", http.StatusSeeOther)
}

func (h *Handlers) CampaignView(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/view", nil)
}

func (h *Handlers) CampaignEdit(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/edit", nil)
}

func (h *Handlers) CampaignUpdate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/campaigns", http.StatusSeeOther)
}

func (h *Handlers) CampaignDelete(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/campaigns", http.StatusSeeOther)
}

func (h *Handlers) CampaignVariables(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/variables", nil)
}

func (h *Handlers) CampaignVariablesUpdate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/campaigns", http.StatusSeeOther)
}

func (h *Handlers) CampaignVariants(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/variants", nil)
}

func (h *Handlers) CampaignVariantCreate(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/campaigns", http.StatusSeeOther)
}

func (h *Handlers) CampaignSendPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/send", nil)
}

func (h *Handlers) CampaignSend(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (h *Handlers) CampaignJobs(w http.ResponseWriter, r *http.Request) {
	h.render(w, "campaigns/jobs", nil)
}
