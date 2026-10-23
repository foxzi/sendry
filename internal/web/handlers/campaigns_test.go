package handlers

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/views"
)

// newCampaignTestHandlers returns handlers on a fresh database with list l1
// (recipients in list order: a active, b unsubscribed, c active, d active),
// template t1 and campaign c1 with variants v1 (weight 3) and v2 (weight 1).
func newCampaignTestHandlers(t *testing.T, publicURL string) (*Handlers, *sql.DB) {
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
	cfg.Server.PublicURL = publicURL

	for _, q := range []string{
		`INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`,
		// list order is created_at DESC
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('ra', 'l1', 'a@example.org', '', '', '', 'active', '2026-01-04')`,
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('rb', 'l1', 'b@example.org', '', '', '', 'unsubscribed', '2026-01-03')`,
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('rc', 'l1', 'c@example.org', '', '', '', 'active', '2026-01-02')`,
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('rd', 'l1', 'd@example.org', '', '', '', 'active', '2026-01-01')`,
		`INSERT INTO templates (id, name, description, subject, html, text, variables, folder) VALUES ('t1', 'tpl', '', 's', '<a href="{{ unsubscribe_url }}">u</a>', '', '[]', '')`,
		`INSERT INTO campaigns (id, name, description, from_email, from_name, reply_to, variables, tags) VALUES ('c1', 'camp', '', 'news@example.com', '', '', '', '')`,
		`INSERT INTO campaign_variants (id, campaign_id, name, template_id, subject_override, weight, created_at) VALUES ('v1', 'c1', 'A', 't1', '', 3, '2026-01-01')`,
		`INSERT INTO campaign_variants (id, campaign_id, name, template_id, subject_override, weight, created_at) VALUES ('v2', 'c1', 'B', 't1', '', 1, '2026-01-02')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return New(cfg, d, slog.New(slog.NewTextHandler(io.Discard, nil)), v, nil), d.DB
}

func postCampaignSend(h *Handlers, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/campaigns/c1/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.CampaignSend(rec, req)
	return rec
}

func TestCampaignSend_WithoutPublicURL(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	if _, err := d.Exec(`UPDATE campaigns SET track_opens = 1`); err != nil {
		t.Fatal(err)
	}

	// the page warns, but still offers to send
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/send", nil)
	req.SetPathValue("id", "c1")
	page := httptest.NewRecorder()
	h.CampaignSendPage(page, req)
	body := page.Body.String()
	if !strings.Contains(body, "The proxying domain is not set.") {
		t.Errorf("send page without warning: %s", body)
	}
	if strings.Contains(body, " disabled ") {
		t.Errorf("send button disabled without public URL")
	}

	// the job is created, and opens are not counted: there is no pixel
	rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var trackOpens bool
	if err := d.QueryRow(`SELECT track_opens FROM send_jobs`).Scan(&trackOpens); err != nil {
		t.Fatal(err)
	}
	if trackOpens {
		t.Error("job tracks opens without public URL")
	}
}

func TestCampaignSendPage_UnsubscribeLinkDetected(t *testing.T) {
	h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/send", nil)
	req.SetPathValue("id", "c1")
	page := httptest.NewRecorder()
	h.CampaignSendPage(page, req)
	body := page.Body.String()
	if strings.Contains(body, "unsubscribe_url}}. ") {
		t.Errorf("unsubscribe link with spaces not detected")
	}
	if strings.Contains(body, "The proxying domain is not set.") {
		t.Errorf("warning shown with a public URL")
	}
}

func TestCampaignForm_TrackingDisabledWithoutPublicURL(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	req := httptest.NewRequest(http.MethodGet, "/campaigns/new", nil)
	rec := httptest.NewRecorder()
	h.CampaignNew(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `name="track_opens" value="on" disabled>`) || !strings.Contains(body, `name="track_clicks" value="on" disabled>`) {
		t.Errorf("tracking checkboxes not disabled: %s", body)
	}

	// a crafted form cannot turn tracking on
	form := url.Values{"name": {"c2"}, "from_email": {"a@example.com"}, "track_opens": {"on"}, "track_clicks": {"on"}}
	post := httptest.NewRequest(http.MethodPost, "/campaigns", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.CampaignCreate(httptest.NewRecorder(), post)
	var opens, clicks bool
	if err := d.QueryRow(`SELECT track_opens, track_clicks FROM campaigns WHERE name = 'c2'`).Scan(&opens, &clicks); err != nil {
		t.Fatal(err)
	}
	if opens || clicks {
		t.Errorf("tracking saved without public URL: opens=%v clicks=%v", opens, clicks)
	}
}

func TestCampaignSend_ItemsWeightsAndSkipped(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")

	rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	rows, err := d.Query(`SELECT i.recipient_id, i.variant_id FROM send_job_items i JOIN recipients r ON r.id = i.recipient_id
		WHERE i.status = 'pending' ORDER BY r.created_at DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var rid, vid string
		rows.Scan(&rid, &vid)
		got = append(got, rid+":"+vid)
	}
	// weights 3:1 give v1, v1, v2; the unsubscribed recipient is not among them
	if want := "ra:v1 rc:v1 rd:v2"; strings.Join(got, " ") != want {
		t.Errorf("items %v, want %s", got, want)
	}
	var skippedItems int
	d.QueryRow(`SELECT COUNT(*) FROM send_job_items WHERE status = 'skipped' AND error = ?`, models.SkipReasonUnsubscribed).Scan(&skippedItems)
	if skippedItems != 1 {
		t.Errorf("skipped items = %d, want 1", skippedItems)
	}
	var skipped int
	d.QueryRow(`SELECT skipped_unsubscribed FROM send_jobs`).Scan(&skipped)
	if skipped != 1 {
		t.Errorf("skipped_unsubscribed = %d, want 1", skipped)
	}
}

func TestJobRecipients_DryRunSkipped(t *testing.T) {
	h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
	cases := []struct{ limit, recipients, skipped int }{
		{1, 1, 0}, // only "a" is within the limit
		{2, 2, 1}, // "b" was passed over to reach "c"
		{0, 3, 1},
	}
	for _, c := range cases {
		recs, skipped, err := h.jobRecipients("l1", c.limit)
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != c.recipients || len(skipped) != c.skipped {
			t.Errorf("limit %d: %d recipients, %d skipped; want %d, %d", c.limit, len(recs), len(skipped), c.recipients, c.skipped)
		}
	}
}

func TestWeightedPicker(t *testing.T) {
	seq := func(weights ...int) string {
		vs := make([]models.CampaignVariant, len(weights))
		for i, w := range weights {
			vs[i].Weight = w
		}
		next := weightedPicker(vs)
		var b strings.Builder
		for i := 0; i < 8; i++ {
			b.WriteByte(byte('0' + next()))
		}
		return b.String()
	}
	cases := []struct {
		weights []int
		want    string
	}{
		{[]int{100}, "00000000"},
		{[]int{100, 100}, "01010101"},
		{[]int{50, 50, 50}, "01201201"},
		{[]int{3, 1}, "00100010"},
		{[]int{70, 30}, "01000100"},
		{[]int{0, 1}, "11111111"},
		{[]int{0, 0}, "01010101"},
	}
	for _, c := range cases {
		if got := seq(c.weights...); got != c.want {
			t.Errorf("weights %v: got %s, want %s", c.weights, got, c.want)
		}
	}

	// shares follow the weights over a long run
	next := weightedPicker([]models.CampaignVariant{{Weight: 70}, {Weight: 20}, {Weight: 10}})
	counts := make([]int, 3)
	for i := 0; i < 1000; i++ {
		counts[next()]++
	}
	if counts[0] != 700 || counts[1] != 200 || counts[2] != 100 {
		t.Errorf("counts %v, want 700/200/100", counts)
	}
}

func TestParseScheduledAt(t *testing.T) {
	cases := []struct{ tz, offset, want string }{
		{"Europe/Moscow", "", "2026-03-10T06:00:00Z"},
		{"America/New_York", "-180", "2026-03-10T13:00:00Z"}, // EDT since 2026-03-08; the IANA name wins
		{"Bad/Zone", "-180", "2026-03-10T06:00:00Z"},
		{"", "300", "2026-03-10T14:00:00Z"},
		{"", "", "2026-03-10T09:00:00Z"},
		{"Local", "x", "2026-03-10T09:00:00Z"},
	}
	for _, c := range cases {
		got, err := parseScheduledAt("2026-03-10T09:00", c.tz, c.offset)
		if err != nil {
			t.Fatal(err)
		}
		if got.Format(time.RFC3339) != c.want {
			t.Errorf("tz %q offset %q: got %s, want %s", c.tz, c.offset, got.Format(time.RFC3339), c.want)
		}
	}
	if _, err := parseScheduledAt("tomorrow", "", ""); err == nil {
		t.Error("bad value accepted")
	}
}

func TestCampaignSend_Scheduled(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"},
		"scheduled_at": {"2026-03-10T09:00"}, "tz": {"Europe/Moscow"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	var status string
	var at time.Time
	if err := d.QueryRow(`SELECT status, scheduled_at FROM send_jobs`).Scan(&status, &at); err != nil {
		t.Fatal(err)
	}
	if status != "scheduled" || !at.Equal(time.Date(2026, 3, 10, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("job %s at %s", status, at)
	}
}

func TestServerPicker(t *testing.T) {
	cfg := &config.Config{}
	cfg.Sendry.MultiSend.Weights = map[string]int{"a": 3, "b": 1}
	h := &Handlers{cfg: cfg}
	servers := []string{"a", "b"}
	seq := func(strategy string) string {
		next := h.serverPicker(strategy, servers)
		var b strings.Builder
		for i := 0; i < 8; i++ {
			b.WriteString(servers[next()])
		}
		return b.String()
	}
	if got := seq(jobStrategyRoundRobin); got != "abababab" {
		t.Errorf("round-robin: %s", got)
	}
	if got := seq(jobStrategyWeighted); got != "aabaaaba" {
		t.Errorf("weighted: %s", got)
	}
	for _, c := range seq(jobStrategyRandom) {
		if c != 'a' && c != 'b' {
			t.Fatalf("random picked %q", c)
		}
	}
}
