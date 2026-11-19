package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
)

// Rows shown on the recipient card; the totals above them count everything
const (
	recipientCardMessages = 100
	recipientCardEvents   = 500
)

// cardMessage is a campaign message on the recipient card with its
// engagement events, newest first
type cardMessage struct {
	models.RecipientMessage
	Events []models.RecipientEvent
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

// recipientVariables decodes the variables JSON object of a recipient into
// rows sorted by key. Anything that is not an object gives no rows.
func recipientVariables(raw string) []recipientVariable {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}
	out := make([]recipientVariable, 0, len(obj))
	for k, v := range obj {
		val, ok := v.(string)
		if !ok {
			b, _ := json.Marshal(v)
			val = string(b)
		}
		out = append(out, recipientVariable{Key: k, Value: val})
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

// RecipientView is the card of an email address: its record in the list it
// was opened from, the other lists the address is in, and the campaign
// messages and engagement events of the address in all lists.
func (h *Handlers) RecipientView(w http.ResponseWriter, r *http.Request) {
	list, rec, ok := h.cardRecipient(w, r)
	if !ok {
		return
	}

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
	events, err := h.recipients.Events(rec.Email, recipientCardEvents)
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
		"Variables":     recipientVariables(rec.Variables),
		"Tags":          recipientTags(rec.Tags),
		"OtherLists":    otherLists(memberships, rec.ID),
		"Totals":        totals,
		"Messages":      cardMessages(messages, events),
		"MessagesLimit": recipientCardMessages,
		"HasMessages":   totals.Messages > 0 || len(messages) > 0,
		"Unsub":         unsub,
		"UnsubCampaign": unsubCampaign,
		"Saved":         r.URL.Query().Get("saved") == "1",
	}

	h.render(w, "recipient_view", data)
}

// RecipientUpdate saves the name of a recipient from the card.
func (h *Handlers) RecipientUpdate(w http.ResponseWriter, r *http.Request) {
	list, rec, ok := h.cardRecipient(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}
	if err := h.recipients.UpdateRecipientName(list.ID, rec.ID, r.FormValue("name")); err != nil {
		h.logger.Error("failed to update recipient", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to update recipient")
		return
	}
	http.Redirect(w, r, recipientCardURL(list.ID, rec.ID)+"?saved=1", http.StatusSeeOther)
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
	_, err := h.unsubs.Add(&models.Unsubscribe{
		ListID:     list.ID,
		Email:      rec.Email,
		SourceType: models.UnsubSourceManual,
	})
	if err != nil && !errors.Is(err, repository.ErrRecipientNotInList) {
		h.logger.Error("failed to unsubscribe recipient", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to unsubscribe recipient")
		return
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
	if err := h.unsubs.Remove(list.ID, rec.Email); err != nil {
		h.logger.Error("failed to resubscribe recipient", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to resubscribe recipient")
		return
	}
	http.Redirect(w, r, recipientCardURL(list.ID, rec.ID), http.StatusSeeOther)
}
