package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func jobRequest(h *Handlers, handler http.HandlerFunc, method, target, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestJobRetry_ResetsFailedItems(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	if rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	var jobID string
	if err := d.QueryRow(`SELECT id FROM send_jobs`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}

	// a: failed, c: sent, d: failed, b: skipped (unsubscribed)
	mustExec(t, d, `UPDATE send_job_items SET status = 'failed', error = 'boom', sendry_msg_id = 'm1', queued_at = CURRENT_TIMESTAMP
		WHERE recipient_id IN ('ra', 'rd')`)
	mustExec(t, d, `UPDATE send_job_items SET status = 'sent', sendry_msg_id = 'm2' WHERE recipient_id = 'rc'`)

	// a running job cannot be retried
	if rec := jobRequest(h, h.JobRetry, http.MethodPost, "/jobs/"+jobID+"/retry", jobID); rec.Code != http.StatusBadRequest {
		t.Errorf("retry of a running job: %d", rec.Code)
	}

	mustExec(t, d, `UPDATE send_jobs SET status = 'completed'`)
	view := jobRequest(h, h.JobView, http.MethodGet, "/jobs/"+jobID, jobID)
	if body := view.Body.String(); !strings.Contains(body, "Retry failed") || !strings.Contains(body, "Skipped") {
		t.Errorf("job page without retry button or skipped card: %d", view.Code)
	}

	rec := jobRequest(h, h.JobRetry, http.MethodPost, "/jobs/"+jobID+"/retry", jobID)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/jobs/"+jobID+"?retried=2" {
		t.Fatalf("retry: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	var status string
	d.QueryRow(`SELECT status FROM send_jobs`).Scan(&status)
	if status != "running" {
		t.Errorf("job status %q, want running", status)
	}
	rows, err := d.Query(`SELECT recipient_id, status, COALESCE(error, ''), COALESCE(sendry_msg_id, ''), queued_at IS NULL
		FROM send_job_items ORDER BY recipient_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var rid, st, e, msg string
		var noQueued bool
		rows.Scan(&rid, &st, &e, &msg, &noQueued)
		got = append(got, rid+":"+st+":"+e+":"+msg)
		if st == "pending" && !noQueued {
			t.Errorf("%s: queued_at not cleared", rid)
		}
	}
	want := "ra:pending:: rb:skipped:recipient unsubscribed: rc:sent::m2 rd:pending::"
	if strings.Join(got, " ") != want {
		t.Errorf("items %v, want %s", got, want)
	}

	view = jobRequest(h, h.JobView, http.MethodGet, "/jobs/"+jobID+"?retried=2", jobID)
	if body := view.Body.String(); !strings.Contains(body, "2 failed items queued for retry") || strings.Contains(body, "Retry failed") {
		t.Errorf("job page after retry: %d", view.Code)
	}
}

func mustExec(t *testing.T, d *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := d.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func TestJobView_NoPublicURLWarning(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	if rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	var jobID string
	if err := d.QueryRow(`SELECT id FROM send_jobs`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	const warn = "Sending is on hold"
	if body := jobRequest(h, h.JobView, http.MethodGet, "/jobs/"+jobID, jobID).Body.String(); strings.Contains(body, warn) {
		t.Error("warning shown with a public URL")
	}
	// The public URL was removed after the job started (e.g. an upgrade)
	h.cfg.Server.PublicURL = ""
	if body := jobRequest(h, h.JobView, http.MethodGet, "/jobs/"+jobID, jobID).Body.String(); !strings.Contains(body, warn) {
		t.Error("no warning for a job that cannot send")
	}
	mustExec(t, d, `UPDATE send_jobs SET status = 'cancelled'`)
	if body := jobRequest(h, h.JobView, http.MethodGet, "/jobs/"+jobID, jobID).Body.String(); strings.Contains(body, warn) {
		t.Error("warning shown for a cancelled job")
	}
}

// A list, recipient, variant or template a job used cannot be deleted (foreign
// keys): the user gets a 409 with an explanation, not a 500.
func TestDeleteInUse_Conflict(t *testing.T) {
	h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
	if rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	del := func(handler http.HandlerFunc, target string, vals map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, target, nil)
		for k, v := range vals {
			req.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"list":      del(h.RecipientListDelete, "/recipients/l1", map[string]string{"id": "l1"}),
		"recipient": del(h.RecipientDelete, "/recipients/l1/recipients/ra", map[string]string{"id": "l1", "recipientId": "ra"}),
		"variant":   del(h.CampaignVariantDelete, "/campaigns/c1/variants/v1", map[string]string{"id": "c1", "variantId": "v1"}),
		"template":  del(h.TemplateDelete, "/templates/t1", map[string]string{"id": "t1"}),
	} {
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Cannot delete") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
}
