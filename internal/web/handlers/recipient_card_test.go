package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
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
		"a@example.org", `value="Moscow"`, `value="orders"`, `value="3"`, `value="vip"`, "second list",
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

	// spaces around the name are dropped, a name over the limit is refused
	rec = httptest.NewRecorder()
	h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", url.Values{"name": {"  Anna B  "}}))
	if body = view(); rec.Code != http.StatusSeeOther || !strings.Contains(body, `value="Anna B"`) {
		t.Errorf("trimmed name: status %d, not saved trimmed", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", url.Values{"name": {strings.Repeat("я", 201)}}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("long name: status %d, want 400", rec.Code)
	}
	if body = view(); !strings.Contains(body, `value="Anna B"`) {
		t.Error("long name replaced the saved one")
	}
	rec = httptest.NewRecorder()
	h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", url.Values{"name": {strings.Repeat("я", 200)}}))
	if rec.Code != http.StatusSeeOther {
		t.Errorf("name of 200 characters: status %d", rec.Code)
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

// The card form saves the tags and the variables together with the name, and
// shows the card again with what was typed when a field is refused.
func TestRecipientCard_EditFields(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	if _, err := d.Exec(`UPDATE recipients SET name = 'Ann', variables = '{"city":"Moscow","orders":3,"Старое имя":"x"}', tags = '["vip"]' WHERE id = 'ra'`); err != nil {
		t.Fatal(err)
	}
	save := func(form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", form))
		return rec
	}
	row := func() (name, vars, tags string) {
		t.Helper()
		if err := d.QueryRow(`SELECT name, variables, tags FROM recipients WHERE id = 'ra'`).Scan(&name, &vars, &tags); err != nil {
			t.Fatal(err)
		}
		return
	}
	audits := func() (n int, last string) {
		t.Helper()
		d.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'recipient_update'`).Scan(&n)
		d.QueryRow(`SELECT details FROM audit_log WHERE action = 'recipient_update' ORDER BY id DESC LIMIT 1`).Scan(&last)
		return
	}

	// a number left as shown stays a number, an imported name is kept, an
	// empty row is skipped, a removed row removes the variable
	rec := save(url.Values{
		"name": {"Ann"}, "vars": {"1"}, "tags": {" vip, new ,vip,, "},
		"var_key":   {"orders", "Старое имя", "plan", ""},
		"var_value": {"3", "y", "pro", " "},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save: status %d: %s", rec.Code, rec.Body.String())
	}
	name, vars, tags := row()
	if name != "Ann" || tags != `["vip","new"]` || vars != `{"orders":3,"plan":"pro","Старое имя":"y"}` {
		t.Errorf("saved: name %q, tags %s, variables %s", name, tags, vars)
	}
	if n, last := audits(); n != 1 || last != "email=a@example.org fields=tags,variables" {
		t.Errorf("audit: %d rows, last %q", n, last)
	}

	// nothing changed: no write, no audit row
	save(url.Values{"name": {"Ann"}, "vars": {"1"}, "tags": {"vip, new"},
		"var_key": {"orders", "Старое имя", "plan"}, "var_value": {"3", "y", "pro"}})
	if n, _ := audits(); n != 1 {
		t.Errorf("audit rows after a save without changes = %d", n)
	}

	// refused fields: nothing is saved, the card comes back with the input
	for _, c := range []struct {
		what string
		form url.Values
		key  string
	}{
		{"value without a name", url.Values{"vars": {"1"}, "var_key": {""}, "var_value": {"lost"}}, "cmp.rc_err_var_noname"},
		{"new name the template cannot read", url.Values{"vars": {"1"}, "var_key": {"my var"}, "var_value": {"x"}}, "cmp.rc_err_var_name"},
		{"same name twice", url.Values{"vars": {"1"}, "var_key": {"plan", "plan"}, "var_value": {"a", "b"}}, "cmp.rc_err_var_dup"},
		{"long value", url.Values{"vars": {"1"}, "var_key": {"plan"}, "var_value": {strings.Repeat("я", 2001)}}, "cmp.rc_err_var_value"},
		{"long tag", url.Values{"tags": {strings.Repeat("я", 51)}}, "cmp.rc_err_tag_long"},
	} {
		c.form.Set("name", "Typed name")
		rec := save(c.form)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.key) || !strings.Contains(rec.Body.String(), `value="Typed name"`) {
			t.Errorf("%s: status %d, error key or typed name missing", c.what, rec.Code)
		}
	}
	if n2, v2, t2 := row(); n2 != name || v2 != vars || t2 != tags {
		t.Errorf("a refused save changed the row: %q %s %s", n2, v2, t2)
	}

	// all variables and tags removed
	if rec := save(url.Values{"name": {"Ann"}, "vars": {"1"}, "tags": {""}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("clear: status %d", rec.Code)
	}
	if _, vars, tags := row(); vars != "{}" || tags != "[]" {
		t.Errorf("after clear: variables %s, tags %s", vars, tags)
	}
}

// Unsubscribing and resubscribing by hand leave audit rows naming the user.
func TestRecipientCard_Audit(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	h.RecipientUnsubscribe(httptest.NewRecorder(), cardRequest(http.MethodPost, "/recipients/l1/recipients/ra/unsubscribe", nil))
	h.RecipientUnsubscribe(httptest.NewRecorder(), cardRequest(http.MethodPost, "/recipients/l1/recipients/ra/unsubscribe", nil))
	h.RecipientResubscribe(httptest.NewRecorder(), cardRequest(http.MethodPost, "/recipients/l1/recipients/ra/resubscribe", nil))
	h.RecipientResubscribe(httptest.NewRecorder(), cardRequest(http.MethodPost, "/recipients/l1/recipients/ra/resubscribe", nil))

	rows, err := d.Query(`SELECT action, entity_type, entity_id, details FROM audit_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var a, et, id, det string
		if err := rows.Scan(&a, &et, &id, &det); err != nil {
			t.Fatal(err)
		}
		got = append(got, a+" "+et+" "+id+" "+det)
	}
	want := []string{
		"recipient_unsubscribe recipient_list l1 email=a@example.org",
		"recipient_resubscribe recipient_list l1 email=a@example.org unsubscribed_by=manual",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("audit = %q, want %q", got, want)
	}
}

// A message with more events than the card shows says so.
func TestRecipientCard_EventsCut(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	for _, q := range []string{
		`INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status) VALUES ('j1', 'c1', 'l1', 'completed')`,
		`INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, status, sent_at) VALUES ('i1', 'j1', 'ra', 'v1', 'sent', '2026-02-01 10:00:00')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < recipientCardEventsPerMessage+3; i++ {
		if _, err := d.Exec(`INSERT INTO email_events (type, source_type, source_id, campaign_id, job_id, email, created_at)
			VALUES ('open', 'job_item', 'i1', 'c1', 'j1', 'a@example.org', datetime('2026-02-01 11:00:00', '+' || ? || ' minutes'))`, i); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	h.RecipientView(rec, cardRequest(http.MethodGet, "/recipients/l1/recipients/ra", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "cmp.rc_events_latest") || !strings.Contains(body, `</span> 53`) {
		t.Error("the card does not say that older events are not shown")
	}
	if n := strings.Count(body, `data-i18n="trk.ev_open"`); n != recipientCardEventsPerMessage {
		t.Errorf("events shown = %d", n)
	}
}

// The audience search finds an address in any list.
func TestRecipientListList_FindsAddresses(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	for _, q := range []string{
		`INSERT INTO recipient_lists (id, name, description, source_type) VALUES ('l2', 'second list', '', 'manual')`,
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES ('rz', 'l2', 'A@Example.org', 'Zed', '', '', 'unsubscribed', '2026-01-05')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	page := func(query string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		h.RecipientListList(rec, httptest.NewRequest(http.MethodGet, "/recipients?search="+url.QueryEscape(query), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("search %q: status %d", query, rec.Code)
		}
		return rec.Body.String()
	}

	body := page("a@example")
	for _, want := range []string{`href="/recipients/l1/recipients/ra"`, `href="/recipients/l2/recipients/rz"`, "second list", "Zed", "status.unsubscribed"} {
		if !strings.Contains(body, want) {
			t.Errorf("search by address: no %q", want)
		}
	}
	if strings.Contains(body, "common.nothing_found") {
		t.Error("found addresses, yet the page says nothing is found")
	}
	// a list is still found by its name; two characters do not search addresses
	if body := page("second"); !strings.Contains(body, `href="/recipients/l2"`) || strings.Contains(body, "cmp.found_addresses") {
		t.Error("search by list name is broken")
	}
	if body := page("a@"); strings.Contains(body, "cmp.found_addresses") {
		t.Error("two characters searched the addresses")
	}
	if body := page("zzz-nothing"); !strings.Contains(body, "common.nothing_found") {
		t.Error("no empty state for a query without matches")
	}
}

// Data that came from an import or the API may break the limits of the card
// form. Editing another field must not be refused because of it, nor change it.
func TestRecipientCard_StoredDataOverLimits(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	longKey, longTag, longName := strings.Repeat("k", 70), strings.Repeat("t", 60), strings.Repeat("n", 250)
	vars := `{"` + longKey + `":"v","bad name":"` + strings.Repeat("x", 2100) + `","text":"l1\nl2"}`
	if _, err := d.Exec(`UPDATE recipients SET name = ?, variables = ?, tags = ? WHERE id = 'ra'`,
		longName, vars, `["`+longTag+`","50%","R&D"]`); err != nil {
		t.Fatal(err)
	}
	save := func(form url.Values) int {
		t.Helper()
		rec := httptest.NewRecorder()
		h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", form))
		return rec.Code
	}
	stored := func() (name, v, tags string) {
		t.Helper()
		if err := d.QueryRow(`SELECT name, variables, tags FROM recipients WHERE id = 'ra'`).Scan(&name, &v, &tags); err != nil {
			t.Fatal(err)
		}
		return
	}

	// the multi-line value is edited in a text area
	rec := httptest.NewRecorder()
	h.RecipientView(rec, cardRequest(http.MethodGet, "/recipients/l1/recipients/ra", nil))
	if !strings.Contains(rec.Body.String(), "<textarea name=\"var_value\"") {
		t.Error("no text area for the multi-line value")
	}

	// everything as the form shows it (a browser sends CRLF), one tag added
	form := url.Values{
		"name": {longName}, "vars": {"1"}, "tags": {longTag + ", 50%, R&D, new"},
		"var_key":   {longKey, "bad name", "text"},
		"var_value": {"v", strings.Repeat("x", 2100), "l1\r\nl2"},
	}
	if code := save(form); code != http.StatusSeeOther {
		t.Fatalf("save with stored data over the limits: status %d", code)
	}
	name, v, tags := stored()
	if name != longName || v != vars || tags != `["`+longTag+`","50%","R&D","new"]` {
		t.Errorf("stored data changed: name %d chars, variables changed %v, tags %s", len(name), v != vars, tags)
	}

	// the limits still apply to what is typed
	form.Set("name", longName+"x")
	if code := save(form); code != http.StatusBadRequest {
		t.Errorf("longer name: status %d", code)
	}
	form.Set("name", longName)
	form["var_key"][1] = "bad name 2"
	if code := save(form); code != http.StatusBadRequest {
		t.Errorf("new unreadable variable name: status %d", code)
	}
}

// A tag saved from the card is found by the tag filter of the list, whatever
// characters it has, and only as the whole tag.
func TestRecipientCard_TagFilter(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	rec := httptest.NewRecorder()
	h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra",
		url.Values{"name": {""}, "tags": {`R&D, <x>, a_b, 50%, it's "q", две части`}}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save: status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := d.Exec(`UPDATE recipients SET tags = '["axb","R&Ds"]' WHERE id = 'rc'`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE recipients SET tags = 'not json' WHERE id = 'rd'`); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"R&D", "<x>", "a_b", "50%", `it's "q"`, "две части"} {
		got, total, err := h.recipients.ListRecipients(models.RecipientFilter{ListID: "l1", Tag: tag})
		if err != nil {
			t.Fatalf("filter %q: %v", tag, err)
		}
		if total != 1 || len(got) != 1 || got[0].ID != "ra" {
			t.Errorf("filter %q: %d rows", tag, total)
		}
	}
	if _, total, _ := h.recipients.ListRecipients(models.RecipientFilter{ListID: "l1", Tag: "R&"}); total != 0 {
		t.Errorf("part of a tag matched %d rows", total)
	}
}

// What the form cannot show or would spoil must survive a save: variables
// that are not an object, long numbers, a stored name the form would refuse.
func TestRecipientCard_OddStoredVariables(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	set := func(vars string) {
		t.Helper()
		if _, err := d.Exec(`UPDATE recipients SET variables = ? WHERE id = 'ra'`, vars); err != nil {
			t.Fatal(err)
		}
	}
	save := func(form url.Values) int {
		t.Helper()
		form.Set("vars", "1")
		rec := httptest.NewRecorder()
		h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", form))
		return rec.Code
	}
	stored := func() string {
		t.Helper()
		var v string
		if err := d.QueryRow(`SELECT variables FROM recipients WHERE id = 'ra'`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	// not an object: the form shows no rows and saving the name leaves the text alone
	for _, vars := range []string{`[1,2]`, `"hello"`, `42`, `{broken`, ``} {
		set(vars)
		if code := save(url.Values{"name": {"N " + vars}}); code != http.StatusSeeOther || stored() != vars {
			t.Errorf("variables %q: status %d, stored %q", vars, code, stored())
		}
	}

	// a long number and a decimal keep their digits when another variable changes
	set(`{"a":"x","id":12345678901234567890,"price":10.00}`)
	rec := httptest.NewRecorder()
	h.RecipientView(rec, cardRequest(http.MethodGet, "/recipients/l1/recipients/ra", nil))
	if body := rec.Body.String(); !strings.Contains(body, `value="12345678901234567890"`) || !strings.Contains(body, `value="10.00"`) {
		t.Error("the card shows the numbers rounded")
	}
	if code := save(url.Values{"var_key": {"a", "id", "price"}, "var_value": {"y", "12345678901234567890", "10.00"}}); code != http.StatusSeeOther {
		t.Fatalf("save: status %d", code)
	}
	if got := stored(); got != `{"a":"y","id":12345678901234567890,"price":10.00}` {
		t.Errorf("numbers after a save = %s", got)
	}

	// a stored empty name and a name with spaces around it do not block the save and are kept
	set(`{"":"v"," sp ":"1","ok":"1"}`)
	if code := save(url.Values{"var_key": {"", " sp ", "ok"}, "var_value": {"v", "1", "2"}}); code != http.StatusSeeOther {
		t.Fatalf("save with a stored empty name: status %d", code)
	}
	if got := stored(); got != `{"":"v"," sp ":"1","ok":"2"}` {
		t.Errorf("stored names changed: %s", got)
	}

	// control characters do not get into the name and the tags
	rec = httptest.NewRecorder()
	h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra",
		url.Values{"name": {"A\x00n\nn\xffa"}, "tags": {"a\nb, \x01"}}))
	var name, tags string
	d.QueryRow(`SELECT name, tags FROM recipients WHERE id = 'ra'`).Scan(&name, &tags)
	if rec.Code != http.StatusSeeOther || name != "Anna" || tags != `["ab"]` {
		t.Errorf("control characters: status %d, name %q, tags %s", rec.Code, name, tags)
	}
}

// A recipient is deleted only through its own list.
func TestRecipientDelete_OwnListOnly(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	if _, err := d.Exec(`INSERT INTO recipient_lists (id, name, description, source_type) VALUES ('l2', 'second list', '', 'manual')`); err != nil {
		t.Fatal(err)
	}
	del := func(listID string) {
		t.Helper()
		req := cardRequest(http.MethodDelete, "/recipients/"+listID+"/recipients/ra", nil)
		req.SetPathValue("id", listID)
		h.RecipientDelete(httptest.NewRecorder(), req)
	}
	count := func() (n int) {
		t.Helper()
		if err := d.QueryRow(`SELECT COUNT(*) FROM recipients WHERE id = 'ra'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	if del("l2"); count() != 1 {
		t.Fatal("deleted through another list")
	}
	if del("l1"); count() != 0 {
		t.Error("not deleted through its own list")
	}
}

// An unsubscribed recipient is not deleted: an import would add it back as active
func TestRecipientDelete_KeepsUnsubscribed(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	if _, err := d.Exec(`UPDATE recipients SET status = 'unsubscribed' WHERE id = 'ra'`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.RecipientDelete(rec, cardRequest(http.MethodDelete, "/recipients/l1/recipients/ra", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", rec.Code)
	}
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM recipients WHERE id = 'ra'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("recipient deleted: %d, %v", n, err)
	}
}

// Tags left as the card shows them are not split again: a stored tag may hold
// a comma, and an array of other values than strings is shown, not dropped
func TestRecipientCard_TagsLeftAsShown(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	for _, stored := range []string{`["a,b","vip"]`, `[1,"x"]`} {
		if _, err := d.Exec(`UPDATE recipients SET tags = ? WHERE id = 'ra'`, stored); err != nil {
			t.Fatal(err)
		}
		view := httptest.NewRecorder()
		h.RecipientView(view, cardRequest(http.MethodGet, "/recipients/l1/recipients/ra", nil))
		shown := strings.Join(cardTags(stored), ", ")
		if shown == "" || !strings.Contains(view.Body.String(), `value="`+shown+`"`) {
			t.Errorf("%s: the tags field does not show %q", stored, shown)
		}
		rec := httptest.NewRecorder()
		h.RecipientUpdate(rec, cardRequest(http.MethodPut, "/recipients/l1/recipients/ra", url.Values{"name": {"Ann"}, "tags": {shown}}))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s: status %d", stored, rec.Code)
		}
		var tags string
		if err := d.QueryRow(`SELECT tags FROM recipients WHERE id = 'ra'`).Scan(&tags); err != nil {
			t.Fatal(err)
		}
		if tags != stored {
			t.Errorf("tags = %s, want %s", tags, stored)
		}
	}
}
