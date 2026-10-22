package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/tracking"
)

// Public tracking endpoints: open pixel, click redirect, unsubscribe page.
// They have no authentication: the signed token in the URL is the capability.

// decodeToken verifies a token of the expected kind using the stored secret.
func (h *Handlers) decodeToken(token string, kind tracking.Kind) (tracking.Payload, *models.TrackingSettings, bool) {
	ts, err := h.settings.GetTrackingSettings()
	if err != nil || ts == nil || ts.Secret == "" {
		if err != nil {
			h.logger.Error("tracking: load settings", "error", err)
		}
		return tracking.Payload{}, nil, false
	}
	p, err := tracking.NewSigner([]byte(ts.Secret)).Decode(token)
	if err != nil || p.Kind != kind {
		return tracking.Payload{}, ts, false
	}
	return p, ts, true
}

// TrackOpen handles GET /t/o/{token} (token may end with ".gif").
// It always answers with the transparent GIF so a bad token reveals nothing.
func (h *Handlers) TrackOpen(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("token"), ".gif")

	hd := w.Header()
	hd.Set("Content-Type", "image/gif")
	hd.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	hd.Set("Pragma", "no-cache")
	hd.Set("Expires", "0")
	defer func() {
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(tracking.TransparentGIF)
		}
	}()

	if r.Method == http.MethodHead {
		return
	}
	p, _, ok := h.decodeToken(token, tracking.KindOpen)
	if !ok {
		return
	}
	src, err := h.tracking.ResolveSource(p.SourceType, p.SourceID)
	if err != nil {
		h.logger.Error("tracking: resolve source (open)", "error", err)
		return
	}
	if src == nil {
		return
	}
	ua := r.UserAgent()
	ev := &models.EmailEvent{
		Type:       models.EventOpen,
		SourceType: src.SourceType,
		SourceID:   src.SourceID,
		CampaignID: src.CampaignID,
		JobID:      src.JobID,
		VariantID:  src.VariantID,
		Email:      src.Email,
		IP:         tracking.ClientIP(r),
		UserAgent:  ua,
		IsMachine:  tracking.IsMachineOpen(ua),
	}
	if err := h.tracking.RecordEvent(ev); err != nil {
		h.logger.Error("tracking: record open", "error", err)
	}
}

// TrackClick handles GET /t/c/{token}.
func (h *Handlers) TrackClick(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.decodeToken(r.PathValue("token"), tracking.KindClick)
	if !ok {
		http.NotFound(w, r)
		return
	}
	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		http.NotFound(w, r)
		return
	}

	// Scanners often probe with HEAD: redirect without recording.
	if r.Method != http.MethodHead {
		src, err := h.tracking.ResolveSource(p.SourceType, p.SourceID)
		if err != nil {
			h.logger.Error("tracking: resolve source (click)", "error", err)
		} else if src != nil {
			ua := r.UserAgent()
			linkID := p.LinkID
			ev := &models.EmailEvent{
				Type:       models.EventClick,
				SourceType: src.SourceType,
				SourceID:   src.SourceID,
				CampaignID: src.CampaignID,
				JobID:      src.JobID,
				VariantID:  src.VariantID,
				Email:      src.Email,
				LinkID:     &linkID,
				URL:        p.URL,
				IP:         tracking.ClientIP(r),
				UserAgent:  ua,
				IsMachine:  tracking.IsMachineClick(ua, src.SentAt, time.Now()),
			}
			if err := h.tracking.RecordEvent(ev); err != nil {
				h.logger.Error("tracking: record click", "error", err)
			}
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, p.URL, http.StatusFound)
}

// unsubscribeSource decodes an unsubscribe token and resolves it. Only job
// items are unsubscribable at this stage.
func (h *Handlers) unsubscribeSource(token string) (*models.TrackingSource, bool) {
	p, _, ok := h.decodeToken(token, tracking.KindUnsubscribe)
	if !ok || p.SourceType != models.SourceJobItem {
		return nil, false
	}
	src, err := h.tracking.ResolveSource(p.SourceType, p.SourceID)
	if err != nil {
		h.logger.Error("tracking: resolve source (unsubscribe)", "error", err)
		return nil, false
	}
	if src == nil {
		return nil, false
	}
	return src, true
}

// UnsubscribePage handles GET /u/{token}. It never changes state: mail
// scanners open every link.
func (h *Handlers) UnsubscribePage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	src, ok := h.unsubscribeSource(token)
	if !ok {
		renderUnsubPage(w, http.StatusNotFound, unsubView{State: unsubInvalid})
		return
	}
	state := unsubAsk
	if already, err := h.unsubs.IsUnsubscribed(src.ListID, src.Email); err != nil {
		h.logger.Error("tracking: unsubscribe lookup", "error", err)
	} else if already {
		state = unsubAlready
	}
	renderUnsubPage(w, http.StatusOK, newUnsubView(state, src))
}

// UnsubscribeConfirm handles POST /u/{token}: the page button and RFC 8058 one-click.
func (h *Handlers) UnsubscribeConfirm(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	src, ok := h.unsubscribeSource(token)
	if !ok {
		renderUnsubPage(w, http.StatusNotFound, unsubView{State: unsubInvalid})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if r.PostForm.Get("List-Unsubscribe") == "One-Click" {
		if _, err := h.unsubscribe(r, src, models.UnsubSourceOneClick); err != nil {
			http.Error(w, "error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Unsubscribed\n"))
		return
	}

	switch r.PostForm.Get("action") {
	case "resubscribe":
		if err := h.unsubs.Remove(src.ListID, src.Email); err != nil {
			h.logger.Error("tracking: resubscribe", "error", err)
			renderUnsubPage(w, http.StatusInternalServerError, unsubView{State: unsubError})
			return
		}
		renderUnsubPage(w, http.StatusOK, newUnsubView(unsubResubscribed, src))
	case "unsubscribe":
		if _, err := h.unsubscribe(r, src, models.UnsubSourceLink); err != nil {
			renderUnsubPage(w, http.StatusInternalServerError, unsubView{State: unsubError})
			return
		}
		renderUnsubPage(w, http.StatusOK, newUnsubView(unsubDone, src))
	default:
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
	}
}

// unsubscribe marks the address unsubscribed in the address book the job was
// sent to (other books are untouched) and records the event when the status
// actually changed.
func (h *Handlers) unsubscribe(r *http.Request, src *models.TrackingSource, sourceType string) (bool, error) {
	changed, err := h.unsubs.Add(&models.Unsubscribe{
		ListID:      src.ListID,
		RecipientID: src.RecipientID,
		Email:       src.Email,
		CampaignID:  src.CampaignID,
		JobID:       src.JobID,
		SourceType:  sourceType,
		SourceID:    src.SourceID,
	})
	if errors.Is(err, repository.ErrRecipientNotInList) {
		// The recipient was deleted from the book: nothing to send to anyway.
		return false, nil
	}
	if err != nil {
		h.logger.Error("tracking: unsubscribe", "error", err)
		return false, err
	}
	if changed {
		ev := &models.EmailEvent{
			Type:       models.EventUnsubscribe,
			SourceType: src.SourceType,
			SourceID:   src.SourceID,
			CampaignID: src.CampaignID,
			JobID:      src.JobID,
			VariantID:  src.VariantID,
			Email:      src.Email,
			IP:         tracking.ClientIP(r),
			UserAgent:  r.UserAgent(),
		}
		if err := h.tracking.RecordEvent(ev); err != nil {
			h.logger.Error("tracking: record unsubscribe", "error", err)
		}
	}
	return changed, nil
}
