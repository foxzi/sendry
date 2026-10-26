package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func postJobSchedule(h *Handlers, id string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+id+"/schedule", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.JobSchedule(rec, req)
	return rec
}

// scheduleTestJob schedules a launch of list l1 and returns its id
func scheduleTestJob(t *testing.T, h *Handlers, d *sql.DB) string {
	t.Helper()
	rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"},
		"scheduled_at": {"2099-03-10T09:00"}, "tz": {"UTC"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	var id string
	if err := d.QueryRow(`SELECT id FROM send_jobs`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func countRows(t *testing.T, d *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := d.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestJobSchedule_RebuildsItems(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	id := scheduleTestJob(t, h, d)
	if n := countRows(t, d, `SELECT COUNT(*) FROM send_job_items WHERE job_id = ?`, id); n != 4 {
		t.Fatalf("items before: %d", n)
	}

	// the list changes after scheduling, the edit picks it up
	mustExec(t, d, `INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('re', 'l1', 'e@example.org', '', '', '', 'active', '2026-01-05')`)
	mustExec(t, d, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l2', 'other', 'manual')`)
	mustExec(t, d, `INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('rx', 'l2', 'x@example.org', '', '', '', 'active', '2026-01-05')`)

	rec := postJobSchedule(h, id, url.Values{"recipient_list_id": {"l2"}, "servers": {"mx1", "mx2"},
		"strategy": {"random"}, "scheduled_at": {"2099-04-01T10:30"}, "tz": {"Europe/Moscow"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/c1" {
		t.Fatalf("schedule: %d %q %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}

	var list, servers, strategy, status string
	var at time.Time
	if err := d.QueryRow(`SELECT recipient_list_id, servers, strategy, status, scheduled_at FROM send_jobs WHERE id = ?`, id).
		Scan(&list, &servers, &strategy, &status, &at); err != nil {
		t.Fatal(err)
	}
	if list != "l2" || servers != `["mx1","mx2"]` || strategy != "random" || status != "scheduled" ||
		!at.Equal(time.Date(2099, 4, 1, 7, 30, 0, 0, time.UTC)) {
		t.Errorf("job %s %s %s %s %s", list, servers, strategy, status, at)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM send_job_items WHERE job_id = ?`, id); n != 1 {
		t.Errorf("items after: %d, want 1", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM send_job_items WHERE job_id = ? AND recipient_id = 'rx'`, id); n != 1 {
		t.Errorf("item of the new list missing")
	}
	if n := countRows(t, d, `SELECT skipped_unsubscribed FROM send_jobs WHERE id = ?`, id); n != 0 {
		t.Errorf("skipped count %d, want 0", n)
	}

	// back to l1: five recipients, one unsubscribed, now with the new one
	rec = postJobSchedule(h, id, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"},
		"scheduled_at": {"2099-04-01T10:30"}, "tz": {"UTC"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("schedule back: %d", rec.Code)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM send_job_items WHERE job_id = ?`, id); n != 5 {
		t.Errorf("items: %d, want 5", n)
	}
	if n := countRows(t, d, `SELECT skipped_unsubscribed FROM send_jobs WHERE id = ?`, id); n != 1 {
		t.Errorf("skipped count %d, want 1", n)
	}
}

func TestJobSchedule_RejectsNotScheduled(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	id := scheduleTestJob(t, h, d)
	mustExec(t, d, `UPDATE send_jobs SET status = 'running'`)

	rec := postJobSchedule(h, id, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}, "scheduled_at": {"2099-04-01T10:30"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("running job: %d", rec.Code)
	}
	if rec := postJobSchedule(h, "nope", url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown job: %d", rec.Code)
	}
}

func TestJobSchedule_RejectsBadInput(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	id := scheduleTestJob(t, h, d)
	mustExec(t, d, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('empty', 'empty', 'manual')`)

	for name, form := range map[string]url.Values{
		"empty list":   {"recipient_list_id": {"empty"}, "servers": {"mx1"}, "scheduled_at": {"2099-04-01T10:30"}},
		"no list":      {"servers": {"mx1"}},
		"no servers":   {"recipient_list_id": {"l1"}},
		"bad time":     {"recipient_list_id": {"l1"}, "servers": {"mx1"}, "scheduled_at": {"tomorrow"}},
		"bad strategy": {"recipient_list_id": {"l1"}, "servers": {"mx1"}, "strategy": {"x"}},
	} {
		if rec := postJobSchedule(h, id, form); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	// nothing changed
	if n := countRows(t, d, `SELECT COUNT(*) FROM send_job_items WHERE job_id = ?`, id); n != 4 {
		t.Errorf("items: %d, want 4", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM send_jobs WHERE recipient_list_id = 'l1' AND status = 'scheduled'`); n != 1 {
		t.Errorf("job changed")
	}
}

func TestJobSchedule_EmptyTimeStartsJob(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	id := scheduleTestJob(t, h, d)

	rec := postJobSchedule(h, id, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("schedule: %d %s", rec.Code, rec.Body.String())
	}
	var status string
	var startedAt, scheduledAt sql.NullTime
	d.QueryRow(`SELECT status, started_at, scheduled_at FROM send_jobs WHERE id = ?`, id).Scan(&status, &startedAt, &scheduledAt)
	if status != "running" || !startedAt.Valid || scheduledAt.Valid {
		t.Errorf("status %q started %v scheduled %v", status, startedAt, scheduledAt)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM send_job_items WHERE job_id = ?`, id); n != 4 {
		t.Errorf("items: %d, want 4", n)
	}
}

func TestJobCancel_ReturnPath(t *testing.T) {
	cases := map[string]string{
		"/campaigns/c1":            "/campaigns/c1",
		"/campaigns/c1?x=1":        "/campaigns/c1?x=1",
		"":                         "/jobs/J",
		"/jobs/other":              "/jobs/J",
		"https://evil.example.com": "/jobs/J",
		"//evil.example.com":       "/jobs/J",
		`/campaigns/\evil.com`:     "/jobs/J",
	}
	for ret, want := range cases {
		h, d := newCampaignTestHandlers(t, "https://panel.example.com")
		id := scheduleTestJob(t, h, d)
		req := httptest.NewRequest(http.MethodPost, "/jobs/"+id+"/cancel", strings.NewReader(url.Values{"return": {ret}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h.JobCancel(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%q: %d", ret, rec.Code)
		}
		if got, want := rec.Header().Get("Location"), strings.ReplaceAll(want, "J", id); got != want {
			t.Errorf("return %q: location %q, want %q", ret, got, want)
		}
		var status string
		d.QueryRow(`SELECT status FROM send_jobs`).Scan(&status)
		if status != "cancelled" {
			t.Errorf("status %q", status)
		}
	}
}

func TestCampaignView_ScheduledCard(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	id := scheduleTestJob(t, h, d)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1", nil)
	req.SetPathValue("id", "c1")
	h.CampaignView(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		`datetime="2099-03-10T09:00:00Z"`,
		"/campaigns/c1/launch?job=" + id,
		`action="/jobs/` + id + `/cancel"`,
		`name="return" value="/campaigns/c1"`,
		"&middot; 3 <span", // active recipients
	} {
		if !strings.Contains(body, want) {
			t.Errorf("campaign page without %q (status %d)", want, rec.Code)
		}
	}
}
