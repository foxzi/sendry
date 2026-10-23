package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/tracking"
)

// Panel pages for tracking: settings, click map.

// validateTrackingBaseURL normalises the proxying domain to an origin. The
// operator may type a bare domain ("lnk.example.com", https is assumed) or a
// full origin ("https://lnk.example.com"). Paths, queries and credentials are
// rejected. Empty means "use the panel address".
func validateTrackingBaseURL(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || strings.ContainsAny(u.Host, " _") {
		return "", "Укажите домен, например lnk.example.com"
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", "Укажите только домен, без пути и параметров"
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), ""
}

func (h *Handlers) renderTrackingSettings(w http.ResponseWriter, r *http.Request, s *models.TrackingSettings, errMsg, okMsg string) {
	data := map[string]any{
		"Title":    "Tracking Settings",
		"Active":   "tracking",
		"User":     h.getUserFromContext(r),
		"Settings": s,
		"Fallback": strings.TrimRight(h.cfg.Server.PublicURL, "/"),
		"Origin":   tracking.ResolveBaseURL(s.BaseURL, h.cfg.Server.PublicURL),
		"Rotated":  s.PreviousSecret != "",
		"Error":    errMsg,
		"Success":  okMsg,
	}
	h.render(w, "settings_tracking", data)
}

// TrackingSettings handles GET /settings/tracking.
func (h *Handlers) TrackingSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetTrackingSettings()
	if err != nil || s == nil {
		h.logger.Error("failed to load tracking settings", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load tracking settings")
		return
	}
	ok := ""
	switch {
	case r.URL.Query().Get("saved") == "1":
		ok = "Settings saved"
	case r.URL.Query().Get("rotated") == "1":
		ok = "Secret rotated"
	}
	h.renderTrackingSettings(w, r, s, "", ok)
}

// TrackingSettingsSave handles POST /settings/tracking.
func (h *Handlers) TrackingSettingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}
	s := &models.TrackingSettings{}
	rawBase := r.FormValue("base_url")
	base, berr := validateTrackingBaseURL(rawBase)
	if berr != "" {
		// Re-render with what the user typed so nothing is lost
		s.BaseURL = strings.TrimSpace(rawBase)
		w.WriteHeader(http.StatusBadRequest)
		h.renderTrackingSettings(w, r, s, berr, "")
		return
	}
	s.BaseURL = base

	if err := h.settings.SaveTrackingSettings(s); err != nil {
		h.logger.Error("failed to save tracking settings", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save tracking settings")
		return
	}
	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r), "update", "tracking_settings", "tracking",
		auditJSON(map[string]any{
			"base_url": s.BaseURL,
		}))
	http.Redirect(w, r, "/settings/tracking?saved=1", http.StatusSeeOther)
}

// TrackingSecretRotate handles POST /settings/tracking/rotate-secret. New mail
// is signed with a fresh secret; links signed with the current one keep
// working (it becomes the previous secret), links signed with the one before
// it stop working.
func (h *Handlers) TrackingSecretRotate(w http.ResponseWriter, r *http.Request) {
	if err := h.settings.RotateTrackingSecret(); err != nil {
		h.logger.Error("failed to rotate tracking secret", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to rotate tracking secret")
		return
	}
	h.settings.LogAction(r, middleware.GetUserID(r), middleware.GetUserEmail(r), "rotate_secret", "tracking_settings", "tracking", "{}")
	http.Redirect(w, r, "/settings/tracking?rotated=1", http.StatusSeeOther)
}

// clickMapRow is a link row of the click map table
type clickMapRow struct {
	models.LinkStat
	Share float64
}

// JobClickMap handles GET /jobs/{id}/clickmap.
func (h *Handlers) JobClickMap(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		h.error(w, http.StatusNotFound, "Job not found")
		return
	}
	snaps, err := h.tracking.GetSnapshots(id)
	if err != nil {
		h.logger.Error("failed to load snapshots", "error", err)
	}
	variant := r.URL.Query().Get("variant")
	var current *models.JobVariantSnapshot
	for i := range snaps {
		if snaps[i].VariantID == variant {
			current = &snaps[i]
		}
	}
	if current == nil && len(snaps) > 0 {
		current = &snaps[0]
		variant = current.VariantID
	}

	var rows []clickMapRow
	var links []models.LinkStat
	if current != nil {
		links, err = h.tracking.JobLinkStats(id, variant, snaps)
		if err != nil {
			h.logger.Error("failed to load link stats", "error", err)
		}
		sum := 0
		for _, l := range links {
			sum += l.TotalClicks
		}
		for _, l := range links {
			share := 0.0
			if sum > 0 {
				share = float64(l.TotalClicks) * 100 / float64(sum)
			}
			rows = append(rows, clickMapRow{LinkStat: l, Share: share})
		}
	}
	if links == nil {
		links = []models.LinkStat{}
	}

	data := map[string]any{
		"Title":     "Click Map",
		"Active":    "jobs",
		"User":      h.getUserFromContext(r),
		"Job":       job,
		"Snapshots": snaps,
		"Current":   current,
		"Variant":   variant,
		"Rows":      rows,
		"Links":     links,
	}
	h.render(w, "job_clickmap", data)
}

// JobClickMapSnapshot handles GET /jobs/{id}/clickmap/snapshot?variant=ID and
// returns the raw snapshot HTML for the click map iframe.
func (h *Handlers) JobClickMapSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := h.jobs.GetByID(id)
	if err != nil || job == nil {
		http.NotFound(w, r)
		return
	}
	snaps, err := h.tracking.GetSnapshots(id)
	if err != nil {
		h.logger.Error("failed to load snapshots", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	variant := r.URL.Query().Get("variant")
	var found *models.JobVariantSnapshot
	for i := range snaps {
		if variant == "" || snaps[i].VariantID == variant {
			found = &snaps[i]
			break
		}
	}
	if found == nil {
		http.NotFound(w, r)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	// Snapshot HTML is user content: no scripts, no forms, no <base>, and it
	// may only be framed by the panel itself.
	hd.Set("Content-Security-Policy", "default-src 'none'; img-src * data:; style-src 'unsafe-inline' *; font-src *; "+
		"form-action 'none'; base-uri 'none'; frame-ancestors 'self'")
	hd.Set("X-Frame-Options", "SAMEORIGIN")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "no-store")
	fmt.Fprint(w, found.HTML)
}
