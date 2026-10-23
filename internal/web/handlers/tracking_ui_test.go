package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/views"
)

func newTrackingTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	v, err := views.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Server.PublicURL = "https://panel.example.com"
	return New(cfg, d, slog.New(slog.NewTextHandler(io.Discard, nil)), v, nil)
}

func postForm(h http.HandlerFunc, target string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestValidateTrackingBaseURL(t *testing.T) {
	ok := map[string]string{
		"":                         "",
		"lnk.example.com":          "https://lnk.example.com",
		"LNK.Example.com/":         "https://lnk.example.com",
		"https://lnk.example.com/": "https://lnk.example.com",
		"http://localhost:8088":    "http://localhost:8088",
	}
	for in, want := range ok {
		if got, msg := validateTrackingBaseURL(in); msg != "" || got != want {
			t.Errorf("%q: got %q %q, want %q", in, got, msg, want)
		}
	}
	for _, bad := range []string{"ftp://x", "https://x.com/path", "x.com?a=1", "user@x.com", "bad domain"} {
		if _, msg := validateTrackingBaseURL(bad); msg == "" {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestTrackingSettingsSave(t *testing.T) {
	h := newTrackingTestHandlers(t)

	// Without a proxying domain the page warns that the panel address is used
	rec := httptest.NewRecorder()
	h.TrackingSettings(rec, httptest.NewRequest(http.MethodGet, "/settings/tracking", nil))
	if !strings.Contains(rec.Body.String(), "trk.no_base_url_warn") {
		t.Errorf("no warning without base_url")
	}

	rec = postForm(h.TrackingSettingsSave, "/settings/tracking", url.Values{
		"base_url": {"lnk.example.com"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body.String())
	}
	s, err := h.settings.GetTrackingSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.BaseURL != "https://lnk.example.com" {
		t.Errorf("unexpected settings %+v", s)
	}

	// The page shows the effective origin
	rec = httptest.NewRecorder()
	h.TrackingSettings(rec, httptest.NewRequest(http.MethodGet, "/settings/tracking", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "https://lnk.example.com/t/c/") ||
		strings.Contains(rec.Body.String(), "trk.no_base_url_warn") {
		t.Errorf("settings page: %d %s", rec.Code, rec.Body.String())
	}

	for name, form := range map[string]url.Values{
		"bad base url": {"base_url": {"ftp://x"}},
		"base path":    {"base_url": {"https://x.com/path"}},
	} {
		rec := postForm(h.TrackingSettingsSave, "/settings/tracking", form)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "alert-error") {
			t.Errorf("%s: code %d", name, rec.Code)
		}
	}
	// Failed validation must not overwrite saved settings
	if s, _ := h.settings.GetTrackingSettings(); s.BaseURL != "https://lnk.example.com" {
		t.Errorf("settings overwritten: %+v", s)
	}
}

func TestTrackingSecretRotate(t *testing.T) {
	h := newTrackingTestHandlers(t)
	before, err := h.settings.GetTrackingSettings()
	if err != nil {
		t.Fatal(err)
	}
	rec := postForm(h.TrackingSecretRotate, "/settings/tracking/rotate-secret", url.Values{})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings/tracking?rotated=1" {
		t.Fatalf("code = %d loc = %q", rec.Code, rec.Header().Get("Location"))
	}
	after, _ := h.settings.GetTrackingSettings()
	if after.Secret == before.Secret || after.PreviousSecret != before.Secret {
		t.Errorf("rotation: before %q, after %+v", before.Secret, after)
	}
}

func seedJob(t *testing.T, h *Handlers) *models.SendJob {
	t.Helper()
	return seedJobTracked(t, h, true)
}

func seedJobTracked(t *testing.T, h *Handlers, trackOpens bool) *models.SendJob {
	t.Helper()
	c := &models.Campaign{Name: "camp", FromEmail: "a@example.com", TrackOpens: trackOpens}
	if err := h.campaigns.Create(c); err != nil {
		t.Fatal(err)
	}
	l := &models.RecipientList{Name: "list", SourceType: "manual"}
	if err := h.recipients.CreateList(l); err != nil {
		t.Fatal(err)
	}
	j := &models.SendJob{CampaignID: c.ID, RecipientListID: l.ID, Servers: "[]", Strategy: "round_robin", Stats: "{}"}
	if err := h.jobs.Create(j); err != nil {
		t.Fatal(err)
	}
	return j
}

func withID(r *http.Request, id string) *http.Request {
	r.SetPathValue("id", id)
	return r
}

func TestClickMapSnapshot(t *testing.T) {
	h := newTrackingTestHandlers(t)
	j := seedJob(t, h)

	rec := httptest.NewRecorder()
	h.JobClickMapSnapshot(rec, withID(httptest.NewRequest(http.MethodGet, "/jobs/nope/clickmap/snapshot", nil), "nope"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown job: %d", rec.Code)
	}

	// Job without snapshots
	rec = httptest.NewRecorder()
	h.JobClickMapSnapshot(rec, withID(httptest.NewRequest(http.MethodGet, "/x", nil), j.ID))
	if rec.Code != http.StatusNotFound {
		t.Errorf("no snapshot: %d", rec.Code)
	}

	// Map page shows the empty state
	rec = httptest.NewRecorder()
	h.JobClickMap(rec, withID(httptest.NewRequest(http.MethodGet, "/x", nil), j.ID))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "<iframe") {
		t.Errorf("empty clickmap: %d", rec.Code)
	}

	snap := &models.JobVariantSnapshot{
		JobID: j.ID, VariantID: "", VariantName: "A", HTML: `<html><a data-sl="1" href="https://x.com">Buy</a></html>`,
		Links: []models.LinkInfo{{ID: 1, URL: "https://x.com", Label: "Buy", Kind: "text"}},
	}
	if err := h.tracking.SaveSnapshot(snap); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	h.JobClickMapSnapshot(rec, withID(httptest.NewRequest(http.MethodGet, "/x", nil), j.ID))
	if rec.Code != 200 {
		t.Fatalf("snapshot: %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Errorf("csp = %q", csp)
	}
	for _, d := range []string{"form-action 'none'", "base-uri 'none'", "frame-ancestors 'self'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("csp lacks %q: %q", d, csp)
		}
	}
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("headers: %v", rec.Header())
	}
	if !strings.Contains(rec.Body.String(), `data-sl="1"`) {
		t.Errorf("body: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.JobClickMap(rec, withID(httptest.NewRequest(http.MethodGet, "/x", nil), j.ID))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `sandbox="allow-same-origin"`) || strings.Contains(body, "allow-scripts") || !strings.Contains(body, "https://x.com") {
		t.Errorf("clickmap page: %d %s", rec.Code, body)
	}
}

func TestJobViewRendersTrackingStats(t *testing.T) {
	h := newTrackingTestHandlers(t)
	j := seedJob(t, h)

	rec := httptest.NewRecorder()
	h.JobView(rec, withID(httptest.NewRequest(http.MethodGet, "/jobs/"+j.ID, nil), j.ID))
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("code %d: %s", rec.Code, body)
	}
	for _, want := range []string{"Opened (unique)", "eng-funnel", "CTR", "CTOR", "/jobs/" + j.ID + "/clickmap", "dl-bar"} {
		if !strings.Contains(body, want) {
			t.Errorf("job view lacks %q", want)
		}
	}

	// A campaign without open/click tracking shows no engagement cards
	off := seedJobTracked(t, h, false)
	rec = httptest.NewRecorder()
	h.JobView(rec, withID(httptest.NewRequest(http.MethodGet, "/jobs/"+off.ID, nil), off.ID))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "Opened (unique)") {
		t.Errorf("untracked job view: %d, cards shown", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.JobItems(rec, withID(httptest.NewRequest(http.MethodGet, "/jobs/"+j.ID+"/items?opened=yes&clicked=no", nil), j.ID))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `value="yes" selected`) {
		t.Errorf("job items: %d", rec.Code)
	}
}
