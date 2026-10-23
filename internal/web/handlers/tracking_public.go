package handlers

import (
	"errors"
	"mime"
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

// decodeToken verifies a token of the expected kind using the stored secret
// (or the previous one, kept after a rotation). err is tracking.ErrBadToken,
// tracking.ErrExpiredToken or a settings load error.
func (h *Handlers) decodeToken(token string, kind tracking.Kind) (tracking.Payload, error) {
	ts, err := h.settings.GetTrackingSettings()
	if err != nil || ts == nil || ts.Secret == "" {
		if err != nil {
			h.logger.Error("tracking: load settings", "error", err)
			return tracking.Payload{}, err
		}
		return tracking.Payload{}, tracking.ErrBadToken
	}
	p, err := tracking.NewSigner([]byte(ts.Secret), []byte(ts.PreviousSecret)).Decode(token)
	if err != nil {
		return tracking.Payload{}, err
	}
	if p.Kind != kind {
		return tracking.Payload{}, tracking.ErrBadToken
	}
	return p, nil
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
	p, err := h.decodeToken(token, tracking.KindOpen)
	if err != nil {
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
		IP:         h.proxies.ClientIP(r),
		UserAgent:  ua,
		IsMachine:  tracking.IsMachineOpen(ua),
	}
	if err := h.tracking.RecordEvent(ev); err != nil {
		h.logger.Error("tracking: record open", "error", err)
	}
}

// TrackClick handles GET /t/c/{token}. An expired token answers 410 and does
// not redirect: a click token is a signed redirect to any URL, so it must not
// work forever.
func (h *Handlers) TrackClick(w http.ResponseWriter, r *http.Request) {
	p, err := h.decodeToken(r.PathValue("token"), tracking.KindClick)
	if errors.Is(err, tracking.ErrExpiredToken) {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "This link has expired", http.StatusGone)
		return
	}
	if err != nil {
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
				IP:         h.proxies.ClientIP(r),
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
// items are unsubscribable at this stage. When the job item is gone (cleanup,
// campaign deletion) the recipient id from the token gives the address book
// and email; the sender is unknown then.
func (h *Handlers) unsubscribeSource(token string) (*models.TrackingSource, bool) {
	p, err := h.decodeToken(token, tracking.KindUnsubscribe)
	if err != nil || p.SourceType != models.SourceJobItem {
		return nil, false
	}
	src, err := h.tracking.ResolveSource(p.SourceType, p.SourceID)
	if err != nil {
		h.logger.Error("tracking: resolve source (unsubscribe)", "error", err)
		return nil, false
	}
	if (src == nil || src.ListID == "" || src.Email == "") && p.RecipientID != "" {
		rcpt, err := h.tracking.ResolveRecipient(p.RecipientID)
		if err != nil {
			h.logger.Error("tracking: resolve recipient (unsubscribe)", "error", err)
			return nil, false
		}
		if rcpt != nil {
			if src == nil {
				src = &models.TrackingSource{SourceType: p.SourceType, SourceID: p.SourceID}
			}
			src.ListID, src.Email, src.RecipientID = rcpt.ListID, rcpt.Email, rcpt.RecipientID
		}
	}
	if src == nil || src.ListID == "" || src.Email == "" {
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
	// RFC 8058 recommends multipart/form-data for one-click; ParseForm reads
	// only urlencoded bodies.
	var perr error
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "multipart/form-data" {
		perr = r.ParseMultipartForm(64 << 10)
	} else {
		perr = r.ParseForm()
	}
	if perr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if r.PostForm.Get("List-Unsubscribe") == "One-Click" {
		// Not in the book any more: nothing to send to, report success
		if _, err := h.unsubscribe(r, src, models.UnsubSourceOneClick); err != nil && !errors.Is(err, repository.ErrRecipientNotInList) {
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
		if _, err := h.unsubscribe(r, src, models.UnsubSourceLink); errors.Is(err, repository.ErrRecipientNotInList) {
			renderUnsubPage(w, http.StatusOK, newUnsubView(unsubNotInList, src))
			return
		} else if err != nil {
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
// actually changed. Returns repository.ErrRecipientNotInList when the address
// is no longer in the book.
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
		return false, err
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
			IP:         h.proxies.ClientIP(r),
			UserAgent:  r.UserAgent(),
		}
		if err := h.tracking.RecordEvent(ev); err != nil {
			h.logger.Error("tracking: record unsubscribe", "error", err)
		}
	}
	return changed, nil
}
