package handlers

import (
	"net/http"
	"strconv"

	"github.com/foxzi/sendry/internal/web/models"
)

func (h *Handlers) CampaignList(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	filter := models.CampaignListFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	}

	campaigns, total, err := h.campaigns.List(filter)
	if err != nil {
		h.logger.Error("failed to list campaigns", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load campaigns")
		return
	}

	totalPages := (total + limit - 1) / limit

	data := map[string]any{
		"Title":      "Campaigns",
		"Active":     "campaigns",
		"User":       h.getUserFromContext(r),
		"Campaigns":  campaigns,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Search":     search,
	}

	h.render(w, "campaigns", data)
}

func (h *Handlers) CampaignNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":  "New Campaign",
		"Active": "campaigns",
		"User":   h.getUserFromContext(r),
	}

	h.render(w, "campaign_new", data)
}

func (h *Handlers) CampaignCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	c := &models.Campaign{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		FromEmail:   r.FormValue("from_email"),
		FromName:    r.FormValue("from_name"),
		ReplyTo:     r.FormValue("reply_to"),
		Variables:   r.FormValue("variables"),
		Tags:        r.FormValue("tags"),
	}

	if c.Name == "" || c.FromEmail == "" {
		h.error(w, http.StatusBadRequest, "Name and From Email are required")
		return
	}

	if err := h.campaigns.Create(c); err != nil {
		h.logger.Error("failed to create campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create campaign")
		return
	}

	http.Redirect(w, r, "/campaigns/"+c.ID, http.StatusSeeOther)
}

func (h *Handlers) CampaignView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load campaign")
		return
	}
	if c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	variants, err := h.campaigns.GetVariants(id)
	if err != nil {
		h.logger.Error("failed to get variants", "error", err)
	}

	// Get recipient lists for send page
	recipientLists, _, _ := h.recipients.ListLists(models.RecipientListFilter{Limit: 100})

	data := map[string]any{
		"Title":          c.Name,
		"Active":         "campaigns",
		"User":           h.getUserFromContext(r),
		"Campaign":       c,
		"Variants":       variants,
		"RecipientLists": recipientLists,
		"Servers":        h.cfg.Sendry.Servers,
	}

	h.render(w, "campaign_view", data)
}

func (h *Handlers) CampaignEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load campaign")
		return
	}
	if c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	data := map[string]any{
		"Title":    "Edit " + c.Name,
		"Active":   "campaigns",
		"User":     h.getUserFromContext(r),
		"Campaign": c,
	}

	h.render(w, "campaign_edit", data)
}

func (h *Handlers) CampaignUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	c.Name = r.FormValue("name")
	c.Description = r.FormValue("description")
	c.FromEmail = r.FormValue("from_email")
	c.FromName = r.FormValue("from_name")
	c.ReplyTo = r.FormValue("reply_to")
	c.Tags = r.FormValue("tags")

	if err := h.campaigns.Update(c); err != nil {
		h.logger.Error("failed to update campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update campaign")
		return
	}

	http.Redirect(w, r, "/campaigns/"+id, http.StatusSeeOther)
}

func (h *Handlers) CampaignDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.campaigns.Delete(id); err != nil {
		h.logger.Error("failed to delete campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete campaign")
		return
	}

	http.Redirect(w, r, "/campaigns", http.StatusSeeOther)
}

func (h *Handlers) CampaignVariables(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	data := map[string]any{
		"Title":    c.Name + " - Variables",
		"Active":   "campaigns",
		"User":     h.getUserFromContext(r),
		"Campaign": c,
	}

	h.render(w, "campaign_variables", data)
}

func (h *Handlers) CampaignVariablesUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	variables := r.FormValue("variables")

	if err := h.campaigns.UpdateVariables(id, variables); err != nil {
		h.logger.Error("failed to update variables", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update variables")
		return
	}

	http.Redirect(w, r, "/campaigns/"+id, http.StatusSeeOther)
}

func (h *Handlers) CampaignVariants(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	variants, err := h.campaigns.GetVariants(id)
	if err != nil {
		h.logger.Error("failed to get variants", "error", err)
	}

	// Get templates for selection
	templateList, _, _ := h.templates.List(models.TemplateListFilter{Limit: 100})

	data := map[string]any{
		"Title":     c.Name + " - Variants",
		"Active":    "campaigns",
		"User":      h.getUserFromContext(r),
		"Campaign":  c,
		"Variants":  variants,
		"Templates": templateList,
	}

	h.render(w, "campaign_variants", data)
}

func (h *Handlers) CampaignVariantCreate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	weight, _ := strconv.Atoi(r.FormValue("weight"))
	if weight <= 0 {
		weight = 100
	}

	v := &models.CampaignVariant{
		CampaignID:      id,
		Name:            r.FormValue("name"),
		TemplateID:      r.FormValue("template_id"),
		SubjectOverride: r.FormValue("subject_override"),
		Weight:          weight,
	}

	if v.Name == "" || v.TemplateID == "" {
		h.error(w, http.StatusBadRequest, "Name and template are required")
		return
	}

	if err := h.campaigns.AddVariant(v); err != nil {
		h.logger.Error("failed to add variant", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to add variant")
		return
	}

	http.Redirect(w, r, "/campaigns/"+id+"/variants", http.StatusSeeOther)
}

func (h *Handlers) CampaignVariantDelete(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("id")
	variantID := r.PathValue("variantId")

	if err := h.campaigns.DeleteVariant(variantID); err != nil {
		h.logger.Error("failed to delete variant", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete variant")
		return
	}

	http.Redirect(w, r, "/campaigns/"+campaignID+"/variants", http.StatusSeeOther)
}

func (h *Handlers) CampaignSendPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	variants, _ := h.campaigns.GetVariants(id)
	recipientLists, _, _ := h.recipients.ListLists(models.RecipientListFilter{Limit: 100})

	data := map[string]any{
		"Title":          "Send " + c.Name,
		"Active":         "campaigns",
		"User":           h.getUserFromContext(r),
		"Campaign":       c,
		"Variants":       variants,
		"RecipientLists": recipientLists,
		"Servers":        h.cfg.Sendry.Servers,
	}

	h.render(w, "campaign_send", data)
}

func (h *Handlers) CampaignSend(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// TODO: Implement actual sending logic
	// This will create a send job and queue messages

	http.Redirect(w, r, "/campaigns/"+id, http.StatusSeeOther)
}

func (h *Handlers) CampaignJobs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := h.campaigns.GetByID(id)
	if err != nil || c == nil {
		h.error(w, http.StatusNotFound, "Campaign not found")
		return
	}

	// TODO: Get jobs for this campaign

	data := map[string]any{
		"Title":    c.Name + " - Jobs",
		"Active":   "campaigns",
		"User":     h.getUserFromContext(r),
		"Campaign": c,
		"Jobs":     []any{},
	}

	h.render(w, "campaign_jobs", data)
}
