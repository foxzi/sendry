package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
)

// Rows shown on the recipient card; the totals above them count everything
const (
	recipientCardMessages         = 100
	recipientCardEventsPerMessage = 50
)

// Limits of the fields edited on the card, in characters. The form fields
// carry the same maxlength, but a direct request does not pass through them.
const (
	recipientNameMax     = 200
	recipientTagMax      = 50
	recipientTagsMax     = 30
	recipientVarNameMax  = 64
	recipientVarValueMax = 2000
	recipientVarsMax     = 50
)

// A variable is used in templates as {{Name}}, so a new one must be a name
// the template engine can read. Names that came from an import are kept.
var recipientVarNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// cardMessage is a campaign message on the recipient card with its
// engagement events, newest first
type cardMessage struct {
	models.RecipientMessage
	Events []models.RecipientEvent
}

// EventsHidden tells how many older events of the message are not shown.
func (m cardMessage) EventsHidden() int {
	if n := m.EventTotal - len(m.Events); n > 0 {
		return n
	}
	return 0
}

// cardMessages attaches each event to the message it belongs to.
func cardMessages(messages []models.RecipientMessage, events []models.RecipientEvent) []cardMessage {
	byItem := map[string][]models.RecipientEvent{}
	for _, e := range events {
		byItem[e.SourceID] = append(byItem[e.SourceID], e)
	}
	out := make([]cardMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, cardMessage{RecipientMessage: m, Events: byItem[m.ItemID]})
	}
	return out
}

// recipientVariable is one personal variable of a recipient for display
type recipientVariable struct {
	Key   string
	Value string
}

// Multiline tells the form to edit the value in a text area: a one-line
// field would drop the line breaks on any save.
func (v recipientVariable) Multiline() bool {
	return strings.ContainsAny(v.Value, "\r\n")
}

// sameText compares a typed value with a stored one; a browser sends every
// line break as CRLF whatever was stored.
func sameText(typed, stored string) bool {
	norm := strings.NewReplacer("\r\n", "\n", "\r", "\n")
	return norm.Replace(typed) == norm.Replace(stored)
}

// variableText is how a variable value is shown and edited: a string as it
// is, anything else as JSON.
func variableText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := marshalPlain(v)
	return b
}

// storedVariables decodes the variables JSON object of a recipient; nil for
// anything that is not an object. Numbers keep their digits (a long id or
// "10.00" would not survive a float).
func storedVariables(raw string) map[string]any {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return nil
	}
	obj, _ := v.(map[string]any)
	return obj
}

// plainText drops what a one-line field must not hold: control characters
// and bytes that are not text.
func plainText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// recipientVariables decodes the variables JSON object of a recipient into
// rows sorted by key. Anything that is not an object gives no rows.
func recipientVariables(raw string) []recipientVariable {
	obj := storedVariables(raw)
	if obj == nil {
		return nil
	}
	out := make([]recipientVariable, 0, len(obj))
	for k, v := range obj {
		out = append(out, recipientVariable{Key: k, Value: variableText(v)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// recipientTags decodes the tags JSON array of a recipient.
func recipientTags(raw string) []string {
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil
	}
	return tags
}

// cardTags is the tags of a recipient as the card shows them: an array with
// other values than strings (from an API call) shows them as JSON, so that the
// field is not empty and the first typed tag does not drop them silently.
func cardTags(raw string) []string {
	if tags := recipientTags(raw); tags != nil {
		return tags
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var items []any
	if err := dec.Decode(&items); err != nil {
		return nil
	}
	tags := make([]string, 0, len(items))
	for _, v := range items {
		tags = append(tags, variableText(v))
	}
	return tags
}

// cardFormError is a refused field of the card form: the translation key of
// the message and its English text.
type cardFormError struct {
	Key  string
	Text string
}

// parseCardTags reads the comma-separated tags field into a JSON array.
// The limits apply to what the user changes: tags the record already has
// (from an import or the API) are kept as they are.
func parseCardTags(raw string, stored []string) (string, []string, *cardFormError) {
	had := map[string]bool{}
	for _, t := range stored {
		had[t] = true
	}
	tags := []string{}
	seen := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if !had[t] {
			t = strings.TrimSpace(plainText(t))
		}
		if t == "" || seen[t] {
			continue
		}
		if !had[t] && utf8.RuneCountInString(t) > recipientTagMax {
			return "", nil, &cardFormError{"cmp.rc_err_tag_long", fmt.Sprintf("A tag is too long (max %d characters)", recipientTagMax)}
		}
		seen[t] = true
		tags = append(tags, t)
	}
	if len(tags) > recipientTagsMax && len(tags) > len(stored) {
		return "", nil, &cardFormError{"cmp.rc_err_tags_many", fmt.Sprintf("Too many tags (max %d)", recipientTagsMax)}
	}
	b, _ := marshalPlain(tags)
	return b, tags, nil
}

// marshalPlain encodes JSON without the HTML escaping of json.Marshal, so
// that "R&D" is stored as typed.
func marshalPlain(v any) (string, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// parseCardVariables reads the name/value rows of the card form into a JSON
// object. A row without a name and a value is skipped. A value that was not
// a string (a number from an API call) and is left as shown keeps its type.
// The limits apply to what the user changes: a stored variable left as it is
// passes whatever its name and length.
func parseCardVariables(keys, values []string, stored string) (string, []recipientVariable, *cardFormError) {
	orig := storedVariables(stored)

	rows := []recipientVariable{}
	obj := map[string]any{}
	var bad *cardFormError
	for i, typed := range keys {
		// a stored name is kept to the letter, spaces around it included
		k := strings.TrimSpace(typed)
		if _, ok := orig[typed]; ok {
			k = typed
		}
		v := ""
		if i < len(values) {
			v = values[i]
		}
		o, known := orig[k]
		untouched := known && sameText(v, variableText(o))
		if k == "" && strings.TrimSpace(v) == "" && !untouched {
			continue
		}
		rows = append(rows, recipientVariable{Key: k, Value: v})
		_, dup := obj[k]
		switch {
		case bad != nil:
		case dup:
			bad = &cardFormError{"cmp.rc_err_var_dup", "Two variables have the same name"}
		case untouched:
		case k == "":
			bad = &cardFormError{"cmp.rc_err_var_noname", "A variable has a value but no name"}
		case !known && utf8.RuneCountInString(k) > recipientVarNameMax:
			bad = &cardFormError{"cmp.rc_err_var_name", "A variable name is too long"}
		case !known && !recipientVarNameRe.MatchString(k):
			bad = &cardFormError{"cmp.rc_err_var_name", "A variable name may contain Latin letters, digits and underscores, and cannot start with a digit"}
		case utf8.RuneCountInString(v) > recipientVarValueMax:
			bad = &cardFormError{"cmp.rc_err_var_value", fmt.Sprintf("A variable value is too long (max %d characters)", recipientVarValueMax)}
		}
		if untouched {
			obj[k] = o
		} else {
			obj[k] = v
		}
	}
	if bad == nil && len(obj) > recipientVarsMax && len(obj) > len(orig) {
		bad = &cardFormError{"cmp.rc_err_vars_many", fmt.Sprintf("Too many variables (max %d)", recipientVarsMax)}
	}
	if bad != nil {
		return "", rows, bad
	}
	b, _ := marshalPlain(obj)
	return b, rows, nil
}

// cardRecipient loads the list and the recipient named in the path and
// answers 404 itself when either is missing.
func (h *Handlers) cardRecipient(w http.ResponseWriter, r *http.Request) (*models.RecipientList, *models.Recipient, bool) {
	list, err := h.recipients.GetListByID(r.PathValue("id"))
	if err != nil {
		h.logger.Error("failed to get recipient list", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to load recipient list")
		return nil, nil, false
	}
	if list == nil {
		h.error(w, r, http.StatusNotFound, "Recipient list not found")
		return nil, nil, false
	}
	rec, err := h.recipients.GetRecipient(list.ID, r.PathValue("recipientId"))
	if err != nil {
		h.logger.Error("failed to get recipient", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to load recipient")
		return nil, nil, false
	}
	if rec == nil {
		h.error(w, r, http.StatusNotFound, "Recipient not found")
		return nil, nil, false
	}
	return list, rec, true
}

// otherLists drops the record the card was opened from.
func otherLists(all []models.RecipientMembership, recipientID string) []models.RecipientMembership {
	out := make([]models.RecipientMembership, 0, len(all))
	for _, m := range all {
		if m.RecipientID != recipientID {
			out = append(out, m)
		}
	}
	return out
}

func recipientCardURL(listID, recipientID string) string {
	return fmt.Sprintf("/recipients/%s/recipients/%s", listID, recipientID)
}

// auditRecipient writes an audit entry for an action of a panel user on a
// recipient, in the same shape as the API ones (the list and the address).
func (h *Handlers) auditRecipient(r *http.Request, action, listID, details string) {
	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r), action, "recipient_list", listID, details)
}

// cardForm is what the edit form of the card shows: the stored fields or,
// after a refused save, what the user typed.
type cardForm struct {
	Name      string
	Tags      string // comma-separated
	Variables []recipientVariable
	Error     *cardFormError
}

func storedCardForm(rec *models.Recipient) cardForm {
	return cardForm{
		Name:      rec.Name,
		Tags:      strings.Join(cardTags(rec.Tags), ", "),
		Variables: recipientVariables(rec.Variables),
	}
}

// RecipientView is the card of an email address: its record in the list it
// was opened from, the other lists the address is in, and the campaign
// messages and engagement events of the address in all lists.
func (h *Handlers) RecipientView(w http.ResponseWriter, r *http.Request) {
	list, rec, ok := h.cardRecipient(w, r)
	if !ok {
		return
	}
	h.renderRecipientCard(w, r, http.StatusOK, list, rec, storedCardForm(rec))
}

func (h *Handlers) renderRecipientCard(w http.ResponseWriter, r *http.Request, status int, list *models.RecipientList, rec *models.Recipient, form cardForm) {
	// The sections below are independent: a failed one is logged and shown empty
	memberships, err := h.recipients.Memberships(rec.Email)
	if err != nil {
		h.logger.Error("failed to load recipient lists", "error", err)
	}
	totals, err := h.recipients.MessageTotals(rec.Email)
	if err != nil {
		h.logger.Error("failed to load recipient totals", "error", err)
	}
	messages, err := h.recipients.Messages(rec.Email, recipientCardMessages)
	if err != nil {
		h.logger.Error("failed to load recipient messages", "error", err)
	}
	itemIDs := make([]string, 0, len(messages))
	for _, m := range messages {
		itemIDs = append(itemIDs, m.ItemID)
	}
	events, err := h.recipients.Events(itemIDs, recipientCardEventsPerMessage)
	if err != nil {
		h.logger.Error("failed to load recipient events", "error", err)
	}

	var unsub *models.Unsubscribe
	var unsubCampaign string
	if rec.Status == models.RecipientStatusUnsubscribed {
		if unsub, err = h.unsubs.Get(list.ID, rec.Email); err != nil {
			h.logger.Error("failed to load unsubscribe", "error", err)
		}
		if unsub != nil && unsub.CampaignID != "" {
			if c, cerr := h.campaigns.GetByID(unsub.CampaignID); cerr == nil && c != nil {
				unsubCampaign = c.Name
			}
		}
	}

	data := map[string]any{
		"Title":         rec.Email,
		"Active":        "recipients",
		"User":          h.getUserFromContext(r),
		"List":          list,
		"Recipient":     rec,
		"Form":          form,
		"NameMax":       recipientNameMax,
		"VarNameMax":    recipientVarNameMax,
		"VarValueMax":   recipientVarValueMax,
		"OtherLists":    otherLists(memberships, rec.ID),
		"Totals":        totals,
		"Messages":      cardMessages(messages, events),
		"MessagesLimit": recipientCardMessages,
		"HasMessages":   totals.Messages > 0 || len(messages) > 0,
		"Unsub":         unsub,
		"UnsubByReader": rec.Status == models.RecipientStatusUnsubscribed &&
			(unsub == nil || (unsub.SourceType != models.UnsubSourceManual && unsub.SourceType != models.UnsubSourceAPI)),
		"UnsubCampaign": unsubCampaign,
		"Saved":         form.Error == nil && r.URL.Query().Get("saved") == "1",
	}

	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	h.render(w, "recipient_view", data)
}

// RecipientUpdate saves the fields edited on the card: the name and, when
// the form sends them, the tags and the variables. A refused field shows the
// card again with what was typed.
func (h *Handlers) RecipientUpdate(w http.ResponseWriter, r *http.Request) {
	list, rec, ok := h.cardRecipient(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	form := storedCardForm(rec)
	var fields repository.RecipientFields
	var changed []string

	if r.Form.Has("name") {
		name := r.FormValue("name")
		if name != rec.Name {
			name = strings.TrimSpace(plainText(name))
		}
		form.Name = name
		if name == rec.Name || name == strings.TrimSpace(rec.Name) {
			// left as it is, whatever its length
		} else if utf8.RuneCountInString(name) > recipientNameMax {
			form.Error = &cardFormError{"cmp.rc_err_name_long", fmt.Sprintf("Name is too long (max %d characters)", recipientNameMax)}
		} else {
			fields.Name = &name
			changed = append(changed, "name")
		}
	}
	// tags left as shown are not parsed again: a stored tag with a comma
	// would be split in two
	if r.Form.Has("tags") && !sameText(r.FormValue("tags"), form.Tags) {
		form.Tags = r.FormValue("tags")
		stored, tags, ferr := parseCardTags(form.Tags, cardTags(rec.Tags))
		switch {
		case ferr != nil:
			if form.Error == nil {
				form.Error = ferr
			}
		case strings.Join(tags, "\x00") != strings.Join(recipientTags(rec.Tags), "\x00"):
			fields.Tags = &stored
			changed = append(changed, "tags")
		}
	}
	// the marker tells an emptied variables block from a form without one
	if r.Form.Has("vars") {
		stored, rows, ferr := parseCardVariables(r.Form["var_key"], r.Form["var_value"], rec.Variables)
		form.Variables = rows
		switch {
		case ferr != nil:
			if form.Error == nil {
				form.Error = ferr
			}
		case !sameVariables(stored, rec.Variables):
			fields.Variables = &stored
			changed = append(changed, "variables")
		}
	}

	if form.Error != nil {
		h.renderRecipientCard(w, r, http.StatusBadRequest, list, rec, form)
		return
	}
	if len(changed) > 0 {
		if err := h.recipients.UpdateRecipientFields(list.ID, rec.ID, fields); err != nil {
			h.logger.Error("failed to update recipient", "error", err)
			h.error(w, r, http.StatusInternalServerError, "Failed to update recipient")
			return
		}
		h.auditRecipient(r, "recipient_update", list.ID, "email="+repository.NormalizeEmail(rec.Email)+" fields="+strings.Join(changed, ","))
	}
	http.Redirect(w, r, recipientCardURL(list.ID, rec.ID)+"?saved=1", http.StatusSeeOther)
}

// sameVariables tells whether two variables texts hold the same object. A
// stored text that is not a JSON object (empty, broken, an array) has no
// variables for the form, so it equals an empty object: saving the card
// without adding a variable leaves such a text as it is.
func sameVariables(a, b string) bool {
	norm := func(s string) string {
		obj := storedVariables(s)
		if obj == nil {
			return "{}"
		}
		out, _ := marshalPlain(obj)
		return out
	}
	return norm(a) == norm(b)
}

// RecipientUnsubscribe unsubscribes the address in this list by hand: no
// campaign is sent to it any more, the same address in other lists is untouched.
func (h *Handlers) RecipientUnsubscribe(w http.ResponseWriter, r *http.Request) {
	list, rec, ok := h.cardRecipient(w, r)
	if !ok {
		return
	}
	// Only an active address is unsubscribed: a repeat (stale tab) would
	// overwrite the journal row of the reader's own unsubscribe, and another
	// status (bounced) must not become one that can be resubscribed
	if rec.Status != "" && rec.Status != "active" {
		http.Redirect(w, r, recipientCardURL(list.ID, rec.ID), http.StatusSeeOther)
		return
	}
	changed, err := h.unsubs.AddIfActive(&models.Unsubscribe{
		ListID:     list.ID,
		Email:      rec.Email,
		SourceType: models.UnsubSourceManual,
		SourceID:   middleware.GetUserID(r),
	})
	if err != nil && !errors.Is(err, repository.ErrRecipientNotInList) {
		h.logger.Error("failed to unsubscribe recipient", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to unsubscribe recipient")
		return
	}
	if changed {
		h.auditRecipient(r, "recipient_unsubscribe", list.ID, "email="+repository.NormalizeEmail(rec.Email))
	}
	http.Redirect(w, r, recipientCardURL(list.ID, rec.ID), http.StatusSeeOther)
}

// RecipientResubscribe makes an unsubscribed address of this list active again.
func (h *Handlers) RecipientResubscribe(w http.ResponseWriter, r *http.Request) {
	list, rec, ok := h.cardRecipient(w, r)
	if !ok {
		return
	}
	if rec.Status != models.RecipientStatusUnsubscribed {
		http.Redirect(w, r, recipientCardURL(list.ID, rec.ID), http.StatusSeeOther)
		return
	}
	// what is being undone goes to the audit log: the journal row is deleted
	undone := "unknown"
	if u, err := h.unsubs.Get(list.ID, rec.Email); err == nil && u != nil {
		undone = u.SourceType
	}
	if err := h.unsubs.Remove(list.ID, rec.Email); err != nil {
		h.logger.Error("failed to resubscribe recipient", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to resubscribe recipient")
		return
	}
	h.auditRecipient(r, "recipient_resubscribe", list.ID, "email="+repository.NormalizeEmail(rec.Email)+" unsubscribed_by="+undone)
	http.Redirect(w, r, recipientCardURL(list.ID, rec.ID), http.StatusSeeOther)
}
