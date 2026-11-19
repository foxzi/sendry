package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func cardRequest(method, path string, form url.Values) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "l1")
	req.SetPathValue("recipientId", "ra")
	return req
}

func TestRecipientCard(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	for _, q := range []string{
		`UPDATE recipients SET variables = '{"city":"Moscow","orders":3}', tags = '["vip"]' WHERE id = 'ra'`,
		`INSERT INTO recipient_lists (id, name, description, source_type) VALUES ('l2', 'second list', '', 'manual')`,
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('rz', 'l2', 'a@example.org', '', '', '', 'active', '2026-01-05')`,
		`INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status) VALUES ('j1', 'c1', 'l1', 'completed')`,
		`INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, status, sent_at, opened_at, open_count) VALUES ('i1', 'j1', 'ra', 'v1', 'sent', '2026-02-01 10:00:00', '2026-02-01 11:00:00', 1)`,
		`INSERT INTO email_events (type, source_type, source_id, campaign_id, job_id, email, user_agent, created_at) VALUES ('open', 'job_item', 'i1', 'c1', 'j1', 'a@example.org', 'TestMailApp', '2026-02-01 11:00:00')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	view := func() string {
		t.Helper()
		rec := httptest.NewRecorder()
		h.RecipientView(rec, cardRequest(http.MethodGet, "/recipients/l1/recipients/ra", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("card: status %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	body := view()
	for _, want := range []string{
		"a@example.org", "Moscow", "<code>orders</code>", "vip", "second list",
		`href="/campaigns/c1"`, `href="/jobs/j1"`, "TestMailApp", "/recipients/l1/recipients/ra/unsubscribe", "rc-track",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("card does not contain %q", want)
		}
	}

	// rename
	rec := httptest.NewRecorder()
	h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", url.Values{"name": {"Anna"}}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update: status %d", rec.Code)
	}
	if body = view(); !strings.Contains(body, `value="Anna"`) {
		t.Error("name is not saved")
	}

	// unsubscribe by hand: this list only, with a journal row
	rec = httptest.NewRecorder()
	h.RecipientUnsubscribe(rec, cardRequest(http.MethodPost, "/recipients/l1/recipients/ra/unsubscribe", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unsubscribe: status %d", rec.Code)
	}
	var s1, s2, source string
	var active int
	if err := d.QueryRow(`SELECT status FROM recipients WHERE id = 'ra'`).Scan(&s1); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT status FROM recipients WHERE id = 'rz'`).Scan(&s2); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT source_type FROM unsubscribes WHERE list_id = 'l1' AND email = 'a@example.org'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT active_count FROM recipient_lists WHERE id = 'l1'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if s1 != "unsubscribed" || s2 != "active" || source != "manual" || active != 2 {
		t.Errorf("after unsubscribe: l1 %q, l2 %q, source %q, active %d", s1, s2, source, active)
	}
	body = view()
	if !strings.Contains(body, "cmp.rc_unsub_manual") || !strings.Contains(body, "/recipients/l1/recipients/ra/resubscribe") {
		t.Error("card does not show the manual unsubscribe")
	}

	// resubscribe
	rec = httptest.NewRecorder()
	h.RecipientResubscribe(rec, cardRequest(http.MethodPost, "/recipients/l1/recipients/ra/resubscribe", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("resubscribe: status %d", rec.Code)
	}
	if err := d.QueryRow(`SELECT status FROM recipients WHERE id = 'ra'`).Scan(&s1); err != nil {
		t.Fatal(err)
	}
	if s1 != "active" {
		t.Errorf("after resubscribe: %q", s1)
	}

	// a recipient of another list is not found
	req := cardRequest(http.MethodGet, "/recipients/l1/recipients/rz", nil)
	req.SetPathValue("recipientId", "rz")
	rec = httptest.NewRecorder()
	h.RecipientView(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("foreign recipient: status %d", rec.Code)
	}
}

// A stale card must not rewrite the reader's own unsubscribe, and a bounced
// address must not become active through unsubscribe + resubscribe.
func TestRecipientCard_StatusGuards(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	for _, q := range []string{
		`INSERT INTO unsubscribes (list_id, recipient_id, email, campaign_id, source_type, created_at) VALUES ('l1', 'rb', 'b@example.org', 'c1', 'link', '2026-02-01 10:00:00')`,
		`UPDATE recipients SET status = 'bounced' WHERE id = 'rc'`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	post := func(recipientID, action string, fn http.HandlerFunc) {
		t.Helper()
		req := cardRequest(http.MethodPost, "/recipients/l1/recipients/"+recipientID+"/"+action, nil)
		req.SetPathValue("recipientId", recipientID)
		rec := httptest.NewRecorder()
		fn(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s %s: status %d", action, recipientID, rec.Code)
		}
	}

	post("rb", "unsubscribe", h.RecipientUnsubscribe)
	var source string
	if err := d.QueryRow(`SELECT source_type FROM unsubscribes WHERE list_id = 'l1' AND email = 'b@example.org'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "link" {
		t.Errorf("journal source = %q, want link", source)
	}

	post("rc", "unsubscribe", h.RecipientUnsubscribe)
	post("rc", "resubscribe", h.RecipientResubscribe)
	var status string
	if err := d.QueryRow(`SELECT status FROM recipients WHERE id = 'rc'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "bounced" {
		t.Errorf("status = %q, want bounced", status)
	}
}
