package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
)

func postCampaignCreate(h *Handlers, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/campaigns", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.CampaignCreate(rec, req)
	return rec
}

func postJSON(fn http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

func TestCampaignCreate_Save(t *testing.T) {
	tests := []struct {
		name        string
		form        url.Values
		wantWeights map[string]int // variant name -> weight
		wantSubject map[string]string
	}{
		{
			name:        "single variant",
			form:        url.Values{"name": {"n"}, "from_email": {"a@example.com"}, "template_id": {"t1"}, "subject": {"s"}, "action": {"save"}},
			wantWeights: map[string]int{"A": 100},
			wantSubject: map[string]string{"A": ""}, // equals the template subject
		},
		{
			name:        "subject differs from the template",
			form:        url.Values{"name": {"n"}, "from_email": {"a@example.com"}, "template_id": {"t1"}, "subject": {"Other"}},
			wantWeights: map[string]int{"A": 100},
			wantSubject: map[string]string{"A": "Other"},
		},
		{
			name: "ab split 70",
			form: url.Values{"name": {"n"}, "from_email": {"a@example.com"}, "template_id": {"t1"}, "ab_enabled": {"on"},
				"template_id_b": {"t1"}, "subject_b": {"B subject"}, "split_a": {"70"}, "action": {"save"}},
			wantWeights: map[string]int{"A": 70, "B": 30},
			wantSubject: map[string]string{"A": "", "B": "B subject"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, d := newCampaignTestHandlers(t, "https://panel.example.com")
			rec := postCampaignCreate(h, tt.form)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var id string
			if err := d.QueryRow(`SELECT id FROM campaigns WHERE name = 'n'`).Scan(&id); err != nil {
				t.Fatal(err)
			}
			if loc := rec.Header().Get("Location"); loc != "/campaigns/"+id {
				t.Errorf("redirect to %q", loc)
			}
			rows, err := d.Query(`SELECT name, weight, subject_override FROM campaign_variants WHERE campaign_id = ?`, id)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			got := map[string]int{}
			for rows.Next() {
				var name, subject string
				var weight int
				if err := rows.Scan(&name, &weight, &subject); err != nil {
					t.Fatal(err)
				}
				got[name] = weight
				if subject != tt.wantSubject[name] {
					t.Errorf("variant %s subject_override %q, want %q", name, subject, tt.wantSubject[name])
				}
			}
			if len(got) != len(tt.wantWeights) {
				t.Fatalf("variants %v, want %v", got, tt.wantWeights)
			}
			for name, w := range tt.wantWeights {
				if got[name] != w {
					t.Errorf("variant %s weight %d, want %d", name, got[name], w)
				}
			}
			var jobs int
			if err := d.QueryRow(`SELECT COUNT(*) FROM send_jobs`).Scan(&jobs); err != nil {
				t.Fatal(err)
			}
			if jobs != 0 {
				t.Errorf("save created %d jobs", jobs)
			}
		})
	}
}

func TestCampaignCreate_Send(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	h.cfg.Sendry.Servers = []config.SendryServer{{Name: "mx1"}}
	d.Exec(`UPDATE recipient_lists SET description = '' WHERE description IS NULL`) // the fixture leaves it NULL

	rec := postCampaignCreate(h, url.Values{"name": {"n"}, "from_email": {"a@example.com"}, "template_id": {"t1"},
		"action": {"send"}, "recipient_list_id": {"l1"}, "servers": {"mx1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var jobID, status string
	if err := d.QueryRow(`SELECT j.id, j.status FROM send_jobs j JOIN campaigns c ON c.id = j.campaign_id WHERE c.name = 'n'`).Scan(&jobID, &status); err != nil {
		t.Fatal(err)
	}
	if loc := rec.Header().Get("Location"); loc != "/jobs/"+jobID {
		t.Errorf("redirect to %q", loc)
	}
	if status != "running" {
		t.Errorf("job status %q", status)
	}
	var items, pending int
	if err := d.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) FROM send_job_items WHERE job_id = ?`, jobID).Scan(&items, &pending); err != nil {
		t.Fatal(err)
	}
	// l1 holds three active recipients and one unsubscribed (a skipped item)
	if items != 4 || pending != 3 {
		t.Errorf("items %d pending %d, want 4 and 3", items, pending)
	}
}

func TestCampaignCreate_ValidationCreatesNothing(t *testing.T) {
	base := func() url.Values {
		return url.Values{"name": {"n2"}, "from_email": {"a@example.com"}, "template_id": {"t1"}}
	}
	with := func(kv ...string) url.Values {
		v := base()
		for i := 0; i < len(kv); i += 2 {
			if kv[i+1] == "" {
				v.Del(kv[i])
			} else {
				v.Set(kv[i], kv[i+1])
			}
		}
		return v
	}
	tests := []struct {
		name string
		form url.Values
	}{
		{"no from", with("from_email", "")},
		{"no template", with("template_id", "")},
		{"unknown template", with("template_id", "nope")},
		{"bad variables", with("variables", "{not json")},
		{"unknown action", with("action", "boom")},
		{"ab without split", with("ab_enabled", "on", "template_id_b", "t1")},
		{"ab split out of range", with("ab_enabled", "on", "template_id_b", "t1", "split_a", "100")},
		{"ab unknown template b", with("ab_enabled", "on", "template_id_b", "nope", "split_a", "50")},
		{"send without list", with("action", "send", "servers", "mx1")},
		{"send unknown list", with("action", "send", "recipient_list_id", "nope", "servers", "mx1")},
		{"send without servers", with("action", "send", "recipient_list_id", "l1")},
		{"send unknown server", with("action", "send", "recipient_list_id", "l1", "servers", "zzz")},
		{"send bad strategy", with("action", "send", "recipient_list_id", "l1", "servers", "mx1", "strategy", "x")},
		{"send bad schedule", with("action", "send", "recipient_list_id", "l1", "servers", "mx1", "scheduled_at", "tomorrow")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, d := newCampaignTestHandlers(t, "https://panel.example.com")
			h.cfg.Sendry.Servers = []config.SendryServer{{Name: "mx1"}}
			d.Exec(`UPDATE recipient_lists SET description = '' WHERE description IS NULL`)
			rec := postCampaignCreate(h, tt.form)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var campaigns, variants, jobs int
			d.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE id != 'c1'`).Scan(&campaigns)
			d.QueryRow(`SELECT COUNT(*) FROM campaign_variants WHERE campaign_id != 'c1'`).Scan(&variants)
			d.QueryRow(`SELECT COUNT(*) FROM send_jobs`).Scan(&jobs)
			if campaigns+variants+jobs != 0 {
				t.Errorf("created campaigns=%d variants=%d jobs=%d", campaigns, variants, jobs)
			}
		})
	}
}

func TestTemplateFrame(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	d.Exec(`UPDATE templates SET html = '<p>Hi {{ name }}</p><a href="{{ unsubscribe_url }}">u</a>'`)

	req := httptest.NewRequest(http.MethodGet, "/templates/t1/frame", nil)
	req.SetPathValue("id", "t1")
	rec := httptest.NewRecorder()
	h.TemplateFrame(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type %q", ct)
	}
	body := rec.Body.String()
	if strings.Contains(body, "{{") || !strings.Contains(body, `href="#"`) {
		t.Errorf("not rendered: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/templates/nope/frame", nil)
	req.SetPathValue("id", "nope")
	rec = httptest.NewRecorder()
	h.TemplateFrame(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown template: status %d", rec.Code)
	}
}

func TestCampaignWizardPreview(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	d.Exec(`UPDATE templates SET html = '<p>Promo {{ promo_code }}</p>'`)

	tests := []struct {
		name     string
		body     string
		wantCode int
		want     string
	}{
		{"campaign variables win", `{"template_id":"t1","variables":"{\"promo_code\":\"SPRING25\"}"}`, 200, "Promo SPRING25"},
		{"no variables", `{"template_id":"t1","variables":""}`, 200, "<p>Promo "},
		{"bad variables JSON", `{"template_id":"t1","variables":"{oops"}`, 400, ""},
		{"bad body JSON", `not json`, 400, ""},
		{"unknown template", `{"template_id":"nope"}`, 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postJSON(h.CampaignWizardPreview, "/campaigns/wizard/preview", tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body %q lacks %q", rec.Body.String(), tt.want)
			}
		})
	}
}

func TestCampaignWizardTest_Validation(t *testing.T) {
	h, _ := newCampaignTestHandlers(t, "")
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"bad JSON", `x`, 400},
		{"no recipient", `{"template_id":"t1","from_email":"a@example.com"}`, 400},
		{"bad variables", `{"template_id":"t1","from_email":"a@example.com","to":"b@example.com","variables":"{"}`, 400},
		{"unknown template", `{"template_id":"nope","from_email":"a@example.com","to":"b@example.com"}`, 404},
		{"no servers", `{"template_id":"t1","from_email":"a@example.com","to":"b@example.com"}`, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postJSON(h.CampaignWizardTest, "/campaigns/wizard/test", tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("no error in %s", rec.Body.String())
			}
		})
	}
}
