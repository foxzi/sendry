package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/sendry"
	emailtpl "github.com/foxzi/sendry/internal/web/template"
)

// WizardTemplate is a template as the new campaign wizard lists it
type WizardTemplate struct {
	ID             string
	Name           string
	Folder         string
	Subject        string
	HasUnsubscribe bool
}

// WizardDomain is what the wizard knows about a sender domain
type WizardDomain struct {
	Deployed    bool
	DKIMEnabled bool
}

// wizardLimit is the page size to load "all" recipient lists with
const wizardLimit = 100000

// VarPair is a campaign variable of the page
type VarPair struct {
	Key   string
	Value string
}

// CampaignContent is what the content row of the campaign page shows: the
// active variants (weight > 0), the first is A and the second is B.
type CampaignContent struct {
	ATemplateID string
	ASubject    string
	BTemplateID string
	BSubject    string
	HasB        bool
	SplitA      int // share of A in percent, 1-99; 50 without B, the start for a new B
	Extra       int // active variants beyond the first two
}

// campaignContent builds the content of a campaign from its variants;
// templateSubjects maps template IDs to their subjects.
func campaignContent(variants []models.CampaignVariant, templateSubjects map[string]string) CampaignContent {
	active := activeVariants(variants)
	subject := func(v models.CampaignVariant) string {
		if v.SubjectOverride != "" {
			return v.SubjectOverride
		}
		return templateSubjects[v.TemplateID]
	}

	content := CampaignContent{SplitA: 50}
	if len(active) > 0 {
		content.ATemplateID, content.ASubject = active[0].TemplateID, subject(active[0])
	}
	if len(active) > 1 {
		content.HasB = true
		content.BTemplateID, content.BSubject = active[1].TemplateID, subject(active[1])
		wA, wB := active[0].Weight, active[1].Weight
		content.SplitA = int(math.Round(100 * float64(wA) / float64(wA+wB)))
		content.SplitA = min(max(content.SplitA, 1), 99)
		content.Extra = len(active) - 2
	}
	return content
}

// campaignVarPairs lists the campaign variables JSON by key; values that are
// not strings are formatted with fmt.Sprint.
func campaignVarPairs(raw string) ([]VarPair, error) {
	vars, err := campaignVarsMap(raw)
	if err != nil {
		return nil, err
	}
	pairs := make([]VarPair, 0, len(vars))
	for k, v := range vars {
		pairs = append(pairs, VarPair{Key: k, Value: fmt.Sprint(v)})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	return pairs, nil
}

// campaignFlowData returns the data of the campaign pages: mode is create,
// edit or send, c is the campaign (a blank one for create).
func (h *Handlers) campaignFlowData(r *http.Request, mode string, c *models.Campaign) (map[string]any, error) {
	if c == nil {
		c = &models.Campaign{}
	}

	templateList, err := h.templates.ListSummaries()
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	templates := make([]WizardTemplate, len(templateList))
	subjects := make(map[string]string, len(templateList))
	for i, t := range templateList {
		templates[i] = WizardTemplate{ID: t.ID, Name: t.Name, Folder: t.Folder, Subject: t.Subject, HasUnsubscribe: t.HasUnsubscribe}
		subjects[t.ID] = t.Subject
	}

	folders, err := h.templates.GetFolders()
	if err != nil {
		h.logger.Error("failed to list template folders", "error", err)
		folders = []string{}
	}

	recipientLists, _, err := h.recipients.ListLists(models.RecipientListFilter{Limit: wizardLimit})
	if err != nil {
		return nil, fmt.Errorf("list recipient lists: %w", err)
	}
	if recipientLists == nil {
		recipientLists = []models.RecipientList{}
	}

	senderDomains := map[string]WizardDomain{}
	states, err := h.domains.SenderStates()
	if err != nil {
		h.logger.Error("failed to load sender domains", "error", err)
	}
	for name, s := range states {
		senderDomains[name] = WizardDomain{Deployed: s.Deployed, DKIMEnabled: s.DKIMEnabled}
	}

	content := CampaignContent{SplitA: 50}
	title := "New Campaign"
	if mode != "create" {
		variants, err := h.campaigns.GetVariants(c.ID)
		if err != nil {
			return nil, fmt.Errorf("get variants: %w", err)
		}
		content = campaignContent(variants, subjects)
		title = c.Name
		if mode == "send" {
			title = "Send " + c.Name
		}
	}

	variables, err := campaignVarPairs(c.Variables)
	if err != nil {
		h.logger.Error("campaign variables are not a JSON object", "campaign_id", c.ID, "error", err)
	}

	return map[string]any{
		"Title":          title,
		"Active":         "campaigns",
		"User":           h.getUserFromContext(r),
		"Mode":           mode,
		"Campaign":       c,
		"Content":        content,
		"Variables":      variables,
		"Templates":      templates,
		"Folders":        folders,
		"RecipientLists": recipientLists,
		"Servers":        h.cfg.Sendry.Servers,
		"SenderDomains":  senderDomains,
		"UserEmail":      middleware.GetUserEmail(r),
		"NoPublicURL":    !h.hasPublicOrigin(),
	}, nil
}

func (h *Handlers) CampaignNew(w http.ResponseWriter, r *http.Request) {
	// Tracking checkboxes are on by default for new campaigns, and off and
	// disabled without the public origin tracking links are built with
	hasOrigin := h.hasPublicOrigin()
	data, err := h.campaignFlowData(r, "create", &models.Campaign{TrackOpens: hasOrigin, TrackClicks: hasOrigin})
	if err != nil {
		h.logger.Error("failed to build new campaign page", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load the page")
		return
	}
	h.render(w, "campaign_new", data)
}

// wizardVariant is a variant of the wizard form
type wizardVariant struct {
	name       string
	templateID string
	subject    string
	weight     int
}

func (h *Handlers) CampaignCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	c := &models.Campaign{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: r.FormValue("description"),
		FromEmail:   strings.TrimSpace(r.FormValue("from_email")),
		FromName:    r.FormValue("from_name"),
		ReplyTo:     strings.TrimSpace(r.FormValue("reply_to")),
		Variables:   strings.TrimSpace(r.FormValue("variables")),
		TrackOpens:  r.FormValue("track_opens") != "",
		TrackClicks: r.FormValue("track_clicks") != "",
		AddUTM:      r.FormValue("add_utm") != "",
	}
	if !h.hasPublicOrigin() {
		c.TrackOpens, c.TrackClicks = false, false
	}

	// Everything is validated before the first write
	if serr := validateSender(c.FromEmail, c.ReplyTo); serr != nil {
		h.error(w, serr.status, serr.message)
		return
	}
	if c.Variables != "" {
		var vars map[string]string
		if err := json.Unmarshal([]byte(c.Variables), &vars); err != nil {
			h.error(w, http.StatusBadRequest, "Campaign variables must be a JSON object of strings: "+err.Error())
			return
		}
	}

	action := r.FormValue("action")
	if action == "" {
		action = "save"
	}
	if action != "save" && action != "send" {
		h.error(w, http.StatusBadRequest, "Unknown action")
		return
	}

	variants, perr := h.wizardVariants(r)
	if perr != nil {
		h.error(w, perr.status, perr.message)
		return
	}

	// Without a name it is made of the subject of variant A and the date
	if c.Name == "" {
		subject := variants[0].subject
		if subject == "" {
			if t, err := h.templates.GetByID(variants[0].templateID); err == nil && t != nil {
				subject = t.Subject
			}
		}
		c.Name = autoCampaignName(subject, time.Now())
	}

	var params jobParams
	if action == "send" {
		var perr *statusError
		if params, perr = parseJobParams(r); perr != nil {
			h.error(w, perr.status, perr.message)
			return
		}
		list, err := h.recipients.GetListByID(params.RecipientListID)
		if err != nil {
			h.logger.Error("failed to get recipient list", "error", err)
			h.error(w, http.StatusInternalServerError, "Failed to load recipient list")
			return
		}
		if list == nil {
			h.error(w, http.StatusBadRequest, "Recipient list not found")
			return
		}
		known := map[string]bool{}
		for _, s := range h.cfg.Sendry.Servers {
			known[s.Name] = true
		}
		for _, name := range params.Servers {
			if !known[name] {
				h.error(w, http.StatusBadRequest, "Server not found: "+name)
				return
			}
		}
	}

	// The campaign and its variants are stored together or not at all
	stored := make([]models.CampaignVariant, 0, len(variants))
	err := h.campaigns.InTx(func(repo *repository.CampaignRepository) error {
		if err := repo.Create(c); err != nil {
			return err
		}
		for _, wv := range variants {
			v := &models.CampaignVariant{
				CampaignID:      c.ID,
				Name:            wv.name,
				TemplateID:      wv.templateID,
				SubjectOverride: wv.subject,
				Weight:          wv.weight,
			}
			if err := repo.AddVariant(v); err != nil {
				return err
			}
			stored = append(stored, *v)
		}
		return nil
	})
	if err != nil {
		h.logger.Error("failed to create campaign", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create campaign")
		return
	}

	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r),
		"create", "campaign", c.ID, auditJSON(map[string]any{"name": c.Name}))

	if action == "save" {
		http.Redirect(w, r, "/campaigns/"+c.ID, http.StatusSeeOther)
		return
	}

	// GetVariants orders them like the send page does, and fills the IDs
	if fresh, err := h.campaigns.GetVariants(c.ID); err == nil && len(fresh) > 0 {
		stored = fresh
	}
	job, perr := h.startJob(r, c, stored, params)
	if perr != nil {
		// the campaign is saved; the person can send it from its page
		h.error(w, perr.status, perr.message)
		return
	}
	http.Redirect(w, r, "/jobs/"+job.ID, http.StatusSeeOther)
}

var (
	templateVarRe = regexp.MustCompile(`\{\{.*?\}\}`)
	spacesRe      = regexp.MustCompile(`\s+`)
)

// autoCampaignName makes the name of a campaign without one: the subject
// without {{variables}} and with a date, "Spring sale, 02.01.2006"; just
// "Campaign 02.01.2006" if nothing is left of the subject.
func autoCampaignName(subject string, now time.Time) string {
	subject = templateVarRe.ReplaceAllString(subject, "")
	subject = strings.Trim(spacesRe.ReplaceAllString(subject, " "), " ,.!:;-")
	date := now.Format("02.01.2006")
	if subject == "" {
		return "Campaign " + date
	}
	return subject + ", " + date
}

// wizardVariants reads and validates the variants of the wizard form: A, and B
// with ab_enabled. The subject override is set only when the subject differs
// from the one of the template.
func (h *Handlers) wizardVariants(r *http.Request) ([]wizardVariant, *statusError) {
	abEnabled := r.FormValue("ab_enabled") != ""
	weightA := 100
	if abEnabled {
		split, err := strconv.Atoi(r.FormValue("split_a"))
		if err != nil || split < 1 || split > 99 {
			return nil, &statusError{http.StatusBadRequest, "Split of variant A must be between 1 and 99"}
		}
		weightA = split
	}

	forms := []wizardVariant{{name: "A", templateID: r.FormValue("template_id"), subject: r.FormValue("subject"), weight: weightA}}
	if abEnabled {
		forms = append(forms, wizardVariant{name: "B", templateID: r.FormValue("template_id_b"), subject: r.FormValue("subject_b"), weight: 100 - weightA})
	}

	for i := range forms {
		v := &forms[i]
		if v.templateID == "" {
			return nil, &statusError{http.StatusBadRequest, "Template is required for variant " + v.name}
		}
		t, err := h.templates.GetByID(v.templateID)
		if err != nil {
			h.logger.Error("failed to get template", "error", err)
			return nil, &statusError{http.StatusInternalServerError, "Failed to load template"}
		}
		if t == nil {
			return nil, &statusError{http.StatusBadRequest, "Template not found for variant " + v.name}
		}
		v.subject = strings.TrimSpace(v.subject)
		if v.subject == t.Subject {
			v.subject = ""
		}
	}
	return forms, nil
}

// campaignVarsMap parses the campaign variables JSON ("" = none)
func campaignVarsMap(raw string) (map[string]any, error) {
	vars := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return vars, nil
	}
	if err := json.Unmarshal([]byte(raw), &vars); err != nil {
		return nil, err
	}
	return vars, nil
}

// wizardRenderData returns the data a template is rendered with in the
// wizard: sample data, global variables, then the campaign variables on top;
// the unsubscribe link is "#".
func (h *Handlers) wizardRenderData(src string, campaignVars map[string]any) map[string]any {
	data := buildSampleData(src)
	globalVars, err := h.settings.GetGlobalVariablesMap()
	if err != nil {
		h.logger.Error("failed to get global variables", "error", err)
	}
	for k, v := range globalVars {
		data[k] = v
	}
	for k, v := range campaignVars {
		data[k] = v
	}
	data["unsubscribe_url"] = "#"
	return data
}

// renderWizardHTML renders the HTML of a template for the wizard preview;
// the source HTML is returned if it does not render.
func (h *Handlers) renderWizardHTML(t *models.Template, campaignVars map[string]any) string {
	data := h.wizardRenderData(t.HTML+" "+t.Subject, campaignVars)
	html, err := emailtpl.RenderHTML("wizard-preview", t.HTML, data)
	if err != nil {
		h.logger.Warn("wizard preview render failed", "template_id", t.ID, "error", err)
		html = t.HTML
	}
	return stripDarkModeCSS(html)
}

// writeHTML serves template HTML. It is user content: sandboxed even when
// opened directly, no scripts, forms or <base>, framed only by the panel.
func (h *Handlers) writeHTML(w http.ResponseWriter, html string) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src * data:; style-src 'unsafe-inline' *; font-src *; "+
		"form-action 'none'; base-uri 'none'; frame-ancestors 'self'")
	hd.Set("X-Frame-Options", "SAMEORIGIN")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "no-store")
	w.Write([]byte(html))
}

// TemplateFrame serves the template HTML rendered with sample data, for the
// wizard preview iframe.
func (h *Handlers) TemplateFrame(w http.ResponseWriter, r *http.Request) {
	t, err := h.templates.GetByID(r.PathValue("id"))
	if err != nil {
		h.logger.Error("failed to get template", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load template")
		return
	}
	if t == nil {
		h.error(w, http.StatusNotFound, "Template not found")
		return
	}
	h.writeHTML(w, h.renderWizardHTML(t, nil))
}

// CampaignWizardPreview renders a template with the campaign variables on top
// of the sample data.
func (h *Handlers) CampaignWizardPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TemplateID string `json:"template_id"`
		Variables  string `json:"variables"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := json.Unmarshal(raw, &body); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	vars, err := campaignVarsMap(body.Variables)
	if err != nil {
		http.Error(w, "Invalid variables JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	t, err := h.templates.GetByID(body.TemplateID)
	if err != nil {
		h.logger.Error("failed to get template", "error", err)
		http.Error(w, "Failed to load template", http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}
	h.writeHTML(w, h.renderWizardHTML(t, vars))
}

// CampaignWizardTest sends one test email of a template through the server
// picked for the run, or the first configured one.
func (h *Handlers) CampaignWizardTest(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, msg string) {
		h.logger.Error("wizard test failed", "status", status, "message", msg)
		h.json(w, status, map[string]string{"error": msg})
	}

	var body struct {
		TemplateID string `json:"template_id"`
		Subject    string `json:"subject"`
		FromEmail  string `json:"from_email"`
		FromName   string `json:"from_name"`
		ReplyTo    string `json:"reply_to"`
		Variables  string `json:"variables"`
		Server     string `json:"server"`
		To         string `json:"to"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := json.Unmarshal(raw, &body); err != nil {
		fail(http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	to := strings.TrimSpace(body.To)
	from := strings.TrimSpace(body.FromEmail)
	if to == "" || from == "" {
		fail(http.StatusBadRequest, "Recipient and From Email are required")
		return
	}
	vars, err := campaignVarsMap(body.Variables)
	if err != nil {
		fail(http.StatusBadRequest, "Invalid variables JSON: "+err.Error())
		return
	}
	t, err := h.templates.GetByID(body.TemplateID)
	if err != nil {
		h.logger.Error("failed to get template", "error", err)
		fail(http.StatusInternalServerError, "Failed to load template")
		return
	}
	if t == nil {
		fail(http.StatusNotFound, "Template not found")
		return
	}
	if len(h.cfg.Sendry.Servers) == 0 {
		fail(http.StatusInternalServerError, "No Sendry server is configured")
		return
	}
	serverName := h.cfg.Sendry.Servers[0].Name
	if body.Server != "" {
		serverName = body.Server
	}
	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		fail(http.StatusInternalServerError, "Server not found: "+serverName)
		return
	}

	subjectSrc := strings.TrimSpace(body.Subject)
	if subjectSrc == "" {
		subjectSrc = t.Subject
	}
	data := h.wizardRenderData(t.HTML+" "+t.Text+" "+subjectSrc, vars)
	data["email"] = to
	data["recipient_email"] = to

	subject, err := emailtpl.RenderText("subject", subjectSrc, data)
	if err != nil {
		fail(http.StatusBadRequest, "Render subject: "+err.Error())
		return
	}
	html, err := emailtpl.RenderHTML("html", t.HTML, data)
	if err != nil {
		fail(http.StatusBadRequest, "Render HTML: "+err.Error())
		return
	}
	text, err := emailtpl.RenderText("text", t.Text, data)
	if err != nil {
		fail(http.StatusBadRequest, "Render text: "+err.Error())
		return
	}
	html = makeAbsoluteURLs(html, h.cfg.Server.PublicURL, h.cfg.Server.PublicUploadURL)

	req := &sendry.SendRequest{
		From:    from,
		To:      []string{to},
		Subject: "[TEST] " + subject,
		HTML:    html,
		Body:    text,
	}
	if body.FromName != "" {
		req.From = body.FromName + " <" + from + ">"
	}
	if replyTo := strings.TrimSpace(body.ReplyTo); replyTo != "" {
		req.Headers = map[string]string{"Reply-To": replyTo}
	}

	resp, err := client.Send(r.Context(), req)
	if err != nil {
		h.logger.Error("failed to send wizard test email", "error", err, "template_id", t.ID, "server", serverName)
		fail(http.StatusInternalServerError, "Failed to send test email: "+err.Error())
		return
	}
	h.logger.Info("wizard test email sent", "template_id", t.ID, "server", serverName, "to", to, "message_id", resp.ID, "user", middleware.GetUserEmail(r))
	h.json(w, http.StatusOK, map[string]bool{"ok": true})
}

// sectionError answers a section save with a JSON error
func (h *Handlers) sectionError(w http.ResponseWriter, status int, message string) {
	h.logger.Warn("campaign section not saved", "status", status, "message", message)
	h.json(w, status, map[string]string{"error": message})
}

// CampaignSection saves one row of the campaign page: from, content, vars,
// track or about. The answer is JSON: {"ok":true} or {"error":"..."}.
func (h *Handlers) CampaignSection(w http.ResponseWriter, r *http.Request) {
	id, row := r.PathValue("id"), r.PathValue("row")

	if err := r.ParseForm(); err != nil {
		h.sectionError(w, http.StatusBadRequest, "Invalid form data")
		return
	}
	c, err := h.campaigns.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get campaign", "error", err)
		h.sectionError(w, http.StatusInternalServerError, "Failed to load campaign")
		return
	}
	if c == nil {
		h.sectionError(w, http.StatusNotFound, "Campaign not found")
		return
	}

	var serr *statusError
	switch row {
	case "from":
		serr = h.saveFromSection(r, c)
	case "content":
		serr = h.saveContentSection(r, c)
	case "vars":
		serr = h.saveVarsSection(r, c)
	case "track":
		c.TrackOpens = r.FormValue("track_opens") != ""
		c.TrackClicks = r.FormValue("track_clicks") != ""
		c.AddUTM = r.FormValue("add_utm") != ""
		if !h.hasPublicOrigin() {
			c.TrackOpens, c.TrackClicks = false, false
		}
		serr = h.updateCampaign(c)
	case "about":
		c.Name = strings.TrimSpace(r.FormValue("name"))
		c.Description = r.FormValue("description")
		if c.Name == "" {
			serr = &statusError{http.StatusBadRequest, "Name is required"}
		} else {
			serr = h.updateCampaign(c)
		}
	default:
		serr = &statusError{http.StatusNotFound, "Unknown section"}
	}
	if serr != nil {
		h.sectionError(w, serr.status, serr.message)
		return
	}

	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r),
		"update", "campaign", id, auditJSON(map[string]any{"section": row}))
	h.json(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) updateCampaign(c *models.Campaign) *statusError {
	if err := h.campaigns.Update(c); err != nil {
		h.logger.Error("failed to update campaign", "error", err)
		return &statusError{http.StatusInternalServerError, "Failed to update campaign"}
	}
	return nil
}

func (h *Handlers) saveFromSection(r *http.Request, c *models.Campaign) *statusError {
	c.FromEmail = strings.TrimSpace(r.FormValue("from_email"))
	c.FromName = r.FormValue("from_name")
	c.ReplyTo = strings.TrimSpace(r.FormValue("reply_to"))
	if serr := validateSender(c.FromEmail, c.ReplyTo); serr != nil {
		return serr
	}
	return h.updateCampaign(c)
}

// validateSender checks the From and Reply-To (optional) addresses of a campaign
func validateSender(from, replyTo string) *statusError {
	if from == "" {
		return &statusError{http.StatusBadRequest, "From Email is required"}
	}
	if !strings.Contains(from, "@") {
		return &statusError{http.StatusBadRequest, "From Email is not an email address"}
	}
	if replyTo != "" && !strings.Contains(replyTo, "@") {
		return &statusError{http.StatusBadRequest, "Reply-To is not an email address"}
	}
	return nil
}

func (h *Handlers) saveVarsSection(r *http.Request, c *models.Campaign) *statusError {
	raw := strings.TrimSpace(r.FormValue("variables"))
	if raw != "" {
		var vars map[string]string
		if err := json.Unmarshal([]byte(raw), &vars); err != nil {
			return &statusError{http.StatusBadRequest, "Campaign variables must be a JSON object of strings: " + err.Error()}
		}
	}
	c.Variables = raw
	return h.updateCampaign(c)
}

// saveContentSection writes the variants A and B of the content row in one
// transaction (see replaceVariants).
func (h *Handlers) saveContentSection(r *http.Request, c *models.Campaign) *statusError {
	forms, serr := h.wizardVariants(r)
	if serr != nil {
		return serr
	}
	err := h.campaigns.InTx(func(repo *repository.CampaignRepository) error {
		return replaceVariants(repo, c.ID, forms)
	})
	if err != nil {
		h.logger.Error("failed to save variants", "campaign_id", c.ID, "error", err)
		return &statusError{http.StatusInternalServerError, "Failed to save variants"}
	}
	return nil
}

// replaceVariants makes forms the active variants of a campaign. Variants
// jobs have used are never changed or deleted (items point to them): they are
// switched off with weight 0 and a new one takes their place.
func replaceVariants(repo *repository.CampaignRepository, campaignID string, forms []wizardVariant) error {
	existing, err := repo.GetVariants(campaignID)
	if err != nil {
		return fmt.Errorf("get variants: %w", err)
	}
	active := activeVariants(existing)

	// retire takes a variant out of the sending: deletes it, or switches it
	// off if jobs used it
	retire := func(v models.CampaignVariant) error {
		used, err := repo.VariantInUse(v.ID)
		if err != nil {
			return fmt.Errorf("check variant: %w", err)
		}
		if !used {
			err = repo.DeleteVariant(v.ID)
			if !errors.Is(err, repository.ErrInUse) {
				return err
			}
		}
		v.Weight = 0
		return repo.UpdateVariant(&v)
	}

	// A new variant is created after the existing ones, so once A is replaced
	// by a new one B has to be replaced too to stay the second
	forceNew := false
	for i, f := range forms {
		var old *models.CampaignVariant
		if i < len(active) {
			old = &active[i]
		}
		switch {
		case old != nil && !forceNew && old.TemplateID == f.templateID && old.SubjectOverride == f.subject:
			if old.Weight != f.weight {
				old.Weight = f.weight
				if err := repo.UpdateVariant(old); err != nil {
					return fmt.Errorf("update variant: %w", err)
				}
			}
			continue
		case old != nil && !forceNew:
			used, err := repo.VariantInUse(old.ID)
			if err != nil {
				return fmt.Errorf("check variant: %w", err)
			}
			if !used {
				old.Name, old.TemplateID, old.SubjectOverride, old.Weight = f.name, f.templateID, f.subject, f.weight
				if err := repo.UpdateVariant(old); err != nil {
					return fmt.Errorf("update variant: %w", err)
				}
				continue
			}
			forceNew = true
			fallthrough
		case old != nil:
			if err := retire(*old); err != nil {
				return fmt.Errorf("retire variant: %w", err)
			}
		}
		v := &models.CampaignVariant{
			CampaignID:      campaignID,
			Name:            f.name,
			TemplateID:      f.templateID,
			SubjectOverride: f.subject,
			Weight:          f.weight,
		}
		forceNew = true
		if err := repo.AddVariant(v); err != nil {
			return fmt.Errorf("add variant: %w", err)
		}
	}

	// B removed, and active variants beyond A and B are switched off
	for i := len(forms); i < len(active); i++ {
		v := active[i]
		if i == 1 {
			if err := retire(v); err != nil {
				return fmt.Errorf("retire variant: %w", err)
			}
			continue
		}
		v.Weight = 0
		if err := repo.UpdateVariant(&v); err != nil {
			return fmt.Errorf("switch off variant: %w", err)
		}
	}
	return nil
}
