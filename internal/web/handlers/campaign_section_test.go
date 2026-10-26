package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

func postSection(h *Handlers, id, row string, form url.Values) (*httptest.ResponseRecorder, string) {
	req := httptest.NewRequest(http.MethodPost, "/campaigns/"+id+"/section/"+row, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	req.SetPathValue("row", row)
	rec := httptest.NewRecorder()
	h.CampaignSection(rec, req)
	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body.Error
}

func TestCampaignSection_Simple(t *testing.T) {
	tests := []struct {
		name    string
		id, row string
		form    url.Values
		status  int
		check   string // SQL returning "1" when the write is right
	}{
		{"from ok", "c1", "from", url.Values{"from_email": {"x@example.com"}, "from_name": {"X"}, "reply_to": {"r@example.com"}}, 200,
			`SELECT from_email = 'x@example.com' AND from_name = 'X' AND reply_to = 'r@example.com' FROM campaigns WHERE id = 'c1'`},
		{"from no email", "c1", "from", url.Values{"from_name": {"X"}}, 400, ""},
		{"from bad email", "c1", "from", url.Values{"from_email": {"x"}}, 400, ""},
		{"from bad reply-to", "c1", "from", url.Values{"from_email": {"x@example.com"}, "reply_to": {"r"}}, 400, ""},
		{"vars ok", "c1", "vars", url.Values{"variables": {`{"a":"b"}`}}, 200, `SELECT variables = '{"a":"b"}' FROM campaigns WHERE id = 'c1'`},
		{"vars empty", "c1", "vars", url.Values{"variables": {" "}}, 200, `SELECT variables = '' FROM campaigns WHERE id = 'c1'`},
		{"vars not json", "c1", "vars", url.Values{"variables": {"{x"}}, 400, ""},
		{"vars not strings", "c1", "vars", url.Values{"variables": {`{"a":1}`}}, 400, ""},
		{"track ok", "c1", "track", url.Values{"track_opens": {"on"}, "add_utm": {"on"}}, 200,
			`SELECT track_opens = 1 AND track_clicks = 0 AND add_utm = 1 FROM campaigns WHERE id = 'c1'`},
		{"about ok", "c1", "about", url.Values{"name": {" New name "}, "description": {"d"}}, 200,
			`SELECT name = 'New name' AND description = 'd' FROM campaigns WHERE id = 'c1'`},
		{"about no name", "c1", "about", url.Values{"name": {"  "}}, 400, ""},
		{"unknown row", "c1", "nope", url.Values{}, 404, ""},
		{"unknown campaign", "zz", "from", url.Values{"from_email": {"x@example.com"}}, 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, d := newCampaignTestHandlers(t, "https://panel.example.com")
			rec, msg := postSection(h, tt.id, tt.row, tt.form)
			if rec.Code != tt.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tt.status != 200 && msg == "" {
				t.Errorf("no error message: %s", rec.Body.String())
			}
			if tt.check != "" {
				var ok bool
				if err := d.QueryRow(tt.check).Scan(&ok); err != nil || !ok {
					t.Errorf("wrong data written (%v)", err)
				}
			}
		})
	}
}

func TestCampaignSection_TrackWithoutPublicURL(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	rec, _ := postSection(h, "c1", "track", url.Values{"track_opens": {"on"}, "track_clicks": {"on"}, "add_utm": {"on"}})
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var ok bool
	d.QueryRow(`SELECT track_opens = 0 AND track_clicks = 0 AND add_utm = 1 FROM campaigns WHERE id = 'c1'`).Scan(&ok)
	if !ok {
		t.Error("tracking saved without public URL")
	}
}

type variantRow struct {
	id, name, tpl, subject string
	weight                 int
}

func variantRows(t *testing.T, h *Handlers) []variantRow {
	t.Helper()
	vs, err := h.campaigns.GetVariants("c1")
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]variantRow, len(vs))
	for i, v := range vs {
		rows[i] = variantRow{v.ID, v.Name, v.TemplateID, v.SubjectOverride, v.Weight}
	}
	return rows
}

func activeNames(rows []variantRow) string {
	var b strings.Builder
	for _, r := range rows {
		if r.weight > 0 {
			b.WriteString(r.name)
			b.WriteString(":")
			b.WriteString(string(rune('0' + r.weight/10)))
			b.WriteString(" ")
		}
	}
	return strings.TrimSpace(b.String())
}

func TestCampaignSection_Content(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		for name, form := range map[string]url.Values{
			"no template":      {},
			"unknown template": {"template_id": {"nope"}},
			"ab without split": {"template_id": {"t1"}, "ab_enabled": {"on"}, "template_id_b": {"t1"}},
			"ab no template b": {"template_id": {"t1"}, "ab_enabled": {"on"}, "split_a": {"50"}},
		} {
			h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
			before := variantRows(t, h)
			rec, msg := postSection(h, "c1", "content", form)
			if rec.Code != 400 || msg == "" {
				t.Errorf("%s: status %d %s", name, rec.Code, rec.Body.String())
			}
			if len(variantRows(t, h)) != len(before) {
				t.Errorf("%s: variants changed", name)
			}
		}
	})

	t.Run("unused: weights and in-place update", func(t *testing.T) {
		h, d := newCampaignTestHandlers(t, "https://panel.example.com")
		d.Exec(`INSERT INTO templates (id, name, description, subject, html, text, variables, folder) VALUES ('t2', 'tpl2', '', 's2', 'x', '', '[]', '')`)
		// only the split changes
		rec, _ := postSection(h, "c1", "content", url.Values{"template_id": {"t1"}, "ab_enabled": {"on"}, "template_id_b": {"t1"}, "split_a": {"60"}})
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		rows := variantRows(t, h)
		if len(rows) != 2 || rows[0].id != "v1" || rows[0].weight != 60 || rows[1].id != "v2" || rows[1].weight != 40 {
			t.Errorf("rows %+v", rows)
		}
		// a new template and subject: updated in place
		rec, _ = postSection(h, "c1", "content", url.Values{"template_id": {"t2"}, "subject": {"Hi"}, "ab_enabled": {"on"}, "template_id_b": {"t1"}, "split_a": {"50"}})
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		rows = variantRows(t, h)
		if len(rows) != 2 || rows[0].id != "v1" || rows[0].tpl != "t2" || rows[0].subject != "Hi" || rows[0].weight != 50 || rows[1].id != "v2" {
			t.Errorf("rows %+v", rows)
		}
	})

	t.Run("used by a job: replaced with weight 0", func(t *testing.T) {
		h, d := newCampaignTestHandlers(t, "https://panel.example.com")
		d.Exec(`INSERT INTO templates (id, name, description, subject, html, text, variables, folder) VALUES ('t2', 'tpl2', '', 's2', 'x', '', '[]', '')`)
		if rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}}); rec.Code != http.StatusSeeOther {
			t.Fatalf("send: %d", rec.Code)
		}
		// make sure v2 was used too
		rec, _ := postSection(h, "c1", "content", url.Values{"template_id": {"t2"}, "ab_enabled": {"on"}, "template_id_b": {"t1"}, "split_a": {"50"}})
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		rows := variantRows(t, h)
		// v1 off, v2 off (B is re-created after the new A), new A and B active
		if len(rows) != 4 || rows[0].id != "v1" || rows[0].weight != 0 || rows[1].id != "v2" || rows[1].weight != 0 {
			t.Fatalf("rows %+v", rows)
		}
		if rows[2].name != "A" || rows[2].tpl != "t2" || rows[2].weight != 50 || rows[3].name != "B" || rows[3].tpl != "t1" || rows[3].weight != 50 {
			t.Errorf("rows %+v", rows)
		}
		// the old variants keep their template and items
		if rows[0].tpl != "t1" {
			t.Errorf("old variant changed: %+v", rows[0])
		}
		var items int
		d.QueryRow(`SELECT COUNT(*) FROM send_job_items WHERE variant_id IN ('v1', 'v2')`).Scan(&items)
		if items == 0 {
			t.Error("job items lost")
		}
	})

	t.Run("remove B", func(t *testing.T) {
		h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
		rec, _ := postSection(h, "c1", "content", url.Values{"template_id": {"t1"}})
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		rows := variantRows(t, h)
		if len(rows) != 1 || rows[0].id != "v1" || rows[0].weight != 100 {
			t.Errorf("rows %+v", rows)
		}
	})

	t.Run("remove B used by a job", func(t *testing.T) {
		h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
		postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}})
		postSection(h, "c1", "content", url.Values{"template_id": {"t1"}})
		rows := variantRows(t, h)
		if len(rows) != 2 || rows[0].weight != 100 || rows[1].id != "v2" || rows[1].weight != 0 {
			t.Errorf("rows %+v", rows)
		}
	})

	t.Run("add B", func(t *testing.T) {
		h, d := newCampaignTestHandlers(t, "https://panel.example.com")
		d.Exec(`DELETE FROM campaign_variants WHERE id = 'v2'`)
		rec, _ := postSection(h, "c1", "content", url.Values{"template_id": {"t1"}, "ab_enabled": {"on"}, "template_id_b": {"t1"}, "subject_b": {"Other"}, "split_a": {"70"}})
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		rows := variantRows(t, h)
		if len(rows) != 2 || rows[0].id != "v1" || rows[0].weight != 70 || rows[1].name != "B" || rows[1].weight != 30 || rows[1].subject != "Other" {
			t.Errorf("rows %+v", rows)
		}
	})

	t.Run("extra active variants are switched off", func(t *testing.T) {
		h, d := newCampaignTestHandlers(t, "https://panel.example.com")
		d.Exec(`INSERT INTO campaign_variants (id, campaign_id, name, template_id, subject_override, weight, created_at) VALUES ('v3', 'c1', 'C', 't1', '', 5, '2026-01-03')`)
		rec, _ := postSection(h, "c1", "content", url.Values{"template_id": {"t1"}, "ab_enabled": {"on"}, "template_id_b": {"t1"}, "split_a": {"50"}})
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		rows := variantRows(t, h)
		if len(rows) != 3 || rows[2].id != "v3" || rows[2].weight != 0 || activeNames(rows) != "A:5 B:5" {
			t.Errorf("rows %+v", rows)
		}
	})
}

func TestCampaignContent(t *testing.T) {
	subjects := map[string]string{"t1": "S1", "t2": "S2"}
	v := func(tpl, subj string, w int) models.CampaignVariant {
		return models.CampaignVariant{TemplateID: tpl, SubjectOverride: subj, Weight: w}
	}
	tests := []struct {
		name string
		in   []models.CampaignVariant
		want CampaignContent
	}{
		{"none", nil, CampaignContent{SplitA: 50}},
		{"one", []models.CampaignVariant{v("t1", "", 100)}, CampaignContent{ATemplateID: "t1", ASubject: "S1", SplitA: 50}},
		{"two", []models.CampaignVariant{v("t1", "Own", 70), v("t2", "", 30)},
			CampaignContent{ATemplateID: "t1", ASubject: "Own", BTemplateID: "t2", BSubject: "S2", HasB: true, SplitA: 70}},
		{"inactive skipped", []models.CampaignVariant{v("t1", "", 0), v("t2", "", 100)}, CampaignContent{ATemplateID: "t2", ASubject: "S2", SplitA: 50}},
		{"rounding", []models.CampaignVariant{v("t1", "", 1), v("t2", "", 2)},
			CampaignContent{ATemplateID: "t1", ASubject: "S1", BTemplateID: "t2", BSubject: "S2", HasB: true, SplitA: 33}},
		{"clamp", []models.CampaignVariant{v("t1", "", 1), v("t2", "", 1000)},
			CampaignContent{ATemplateID: "t1", ASubject: "S1", BTemplateID: "t2", BSubject: "S2", HasB: true, SplitA: 1}},
		{"extra", []models.CampaignVariant{v("t1", "", 1), v("t2", "", 1), v("t1", "", 1), v("t1", "", 1)},
			CampaignContent{ATemplateID: "t1", ASubject: "S1", BTemplateID: "t2", BSubject: "S2", HasB: true, SplitA: 50, Extra: 2}},
	}
	for _, tt := range tests {
		if got := campaignContent(tt.in, subjects); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestCampaignVarPairs(t *testing.T) {
	got, err := campaignVarPairs(`{"b":2,"a":"x","c":true}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []VarPair{{"a", "x"}, {"b", "2"}, {"c", "true"}}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("got %v", got)
	}
	if got, err := campaignVarPairs(""); err != nil || len(got) != 0 {
		t.Errorf("empty: %v %v", got, err)
	}
}

func TestCampaignFlowData(t *testing.T) {
	h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c, _ := h.campaigns.GetByID("c1")

	data, err := h.campaignFlowData(req, "edit", c)
	if err != nil {
		t.Fatal(err)
	}
	content := data["Content"].(CampaignContent)
	if !content.HasB || content.SplitA != 75 || content.ASubject != "s" {
		t.Errorf("content %+v", content)
	}
	if data["Mode"] != "edit" || data["NoPublicURL"] != false || data["Campaign"] != c {
		t.Errorf("data %v", data)
	}

	data, err = h.campaignFlowData(req, "create", nil)
	if err != nil {
		t.Fatal(err)
	}
	if data["Campaign"] == nil || data["Content"].(CampaignContent).SplitA != 50 {
		t.Errorf("create data %v", data)
	}
}

func TestAutoCampaignName(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	tests := []struct{ subject, want string }{
		{"Spring sale", "Spring sale, 07.03.2026"},
		{"  Spring   sale!  ", "Spring sale, 07.03.2026"},
		{"Hi {{name}}, big news", "Hi , big news, 07.03.2026"},
		{"{{name}}, welcome", "welcome, 07.03.2026"},
		{"Hello {{ first_name }}!", "Hello, 07.03.2026"},
		{"{{a}}", "Campaign 07.03.2026"},
		{"", "Campaign 07.03.2026"},
		{" - ,. ", "Campaign 07.03.2026"},
	}
	for _, tt := range tests {
		if got := autoCampaignName(tt.subject, now); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.subject, got, tt.want)
		}
	}
}

func TestCampaignCreate_AutoName(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	date := time.Now().Format("02.01.2006")
	tests := []struct {
		form url.Values
		want string
	}{
		{url.Values{"from_email": {"a@example.com"}, "template_id": {"t1"}, "subject": {"Big {{x}} sale"}}, "Big sale, " + date},
		// no subject: the template subject "s"
		{url.Values{"from_email": {"a@example.com"}, "template_id": {"t1"}}, "s, " + date},
		{url.Values{"from_email": {"a@example.com"}, "template_id": {"t1"}, "name": {" Mine "}, "tags": {"x"}}, "Mine"},
	}
	for _, tt := range tests {
		if rec := postCampaignCreate(h, tt.form); rec.Code != http.StatusSeeOther {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
	}
	rows, err := d.Query(`SELECT name, tags FROM campaigns WHERE id != 'c1' ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var name, tags string
		rows.Scan(&name, &tags)
		if name != tests[i].want {
			t.Errorf("name %q, want %q", name, tests[i].want)
		}
		if tags != "" {
			t.Errorf("tags %q saved", tags)
		}
		i++
	}
	if i != len(tests) {
		t.Errorf("%d campaigns created", i)
	}
}

func TestCampaignSend_OnlyActiveVariants(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "https://panel.example.com")
	d.Exec(`UPDATE campaign_variants SET weight = 0 WHERE id = 'v2'`)
	if rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var other int
	d.QueryRow(`SELECT COUNT(*) FROM send_job_items WHERE variant_id != 'v1'`).Scan(&other)
	if other != 0 {
		t.Errorf("%d items use a switched off variant", other)
	}

	d.Exec(`UPDATE campaign_variants SET weight = 0`)
	rec := postCampaignSend(h, url.Values{"recipient_list_id": {"l1"}, "servers": {"mx1"}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no variants configured") {
		t.Errorf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCampaignRedirect(t *testing.T) {
	h, _ := newCampaignTestHandlers(t, "")
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/edit", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.CampaignRedirect(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/campaigns/c1" {
		t.Errorf("%d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestTemplateFrame_SandboxHeaders(t *testing.T) {
	h, _ := newCampaignTestHandlers(t, "https://panel.example.com")
	req := httptest.NewRequest(http.MethodGet, "/templates/t1/frame", nil)
	req.SetPathValue("id", "t1")
	rec := httptest.NewRecorder()
	h.TemplateFrame(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	// template HTML is user content: it must not run scripts on the panel origin when opened directly
	if !strings.HasPrefix(csp, "sandbox;") || !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP = %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("no nosniff")
	}
}
