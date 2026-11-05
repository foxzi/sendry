package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/repository"
)

// newAPIListsServer wires the list endpoints behind the real APIAuth
// middleware with a real API key. The fixture has list l1 with a@ (active),
// b@ (unsubscribed), c@ and d@ (active).
func newAPIListsServer(t *testing.T) (http.Handler, *sql.DB, string) {
	t.Helper()
	h, d := newCampaignTestHandlers(t, "")
	keys := repository.NewAPIKeyRepository(d)
	res, err := keys.Create(repository.APIKeyCreateOptions{Name: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/lists", h.APIListLists)
	mux.HandleFunc("POST /api/v1/lists/{id}/recipients", h.APIAddRecipients)
	mux.HandleFunc("DELETE /api/v1/lists/{id}/recipients/{email}", h.APIDeleteRecipient)
	return middleware.APIAuth(keys, h.logger)(mux), d, res.Key
}

func apiDo(srv http.Handler, key, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

type addResp struct {
	Added, Updated, Existing, Unsubscribed int
	Invalid                                []struct{ Email, Error string }
}

func decodeAdd(t *testing.T, rec *httptest.ResponseRecorder) addResp {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var r addResp
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func recipientRow(t *testing.T, d *sql.DB, email string) (name, vars, status string, found bool) {
	t.Helper()
	var n, v sql.NullString
	err := d.QueryRow(`SELECT name, variables, status FROM recipients WHERE list_id='l1' AND email=?`, email).Scan(&n, &v, &status)
	if err == sql.ErrNoRows {
		return "", "", "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return n.String, v.String, status, true
}

func TestAPILists_RequiresKey(t *testing.T) {
	srv, _, _ := newAPIListsServer(t)
	for _, c := range [][2]string{{"GET", "/api/v1/lists"}, {"POST", "/api/v1/lists/l1/recipients"}, {"DELETE", "/api/v1/lists/l1/recipients/a@example.org"}} {
		if rec := apiDo(srv, "", c[0], c[1], "{}"); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", c[0], c[1], rec.Code)
		}
	}
}

func TestAPIListLists(t *testing.T) {
	srv, d, key := newAPIListsServer(t)
	d.Exec(`INSERT INTO recipient_lists (id, name, description, source_type) VALUES ('l2', 'empty', 'Shop', 'api')`)
	rec := apiDo(srv, key, "GET", "/api/v1/lists", "")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Lists []map[string]any `json:"lists"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	got := map[string]map[string]any{}
	for _, l := range out.Lists {
		got[l["id"].(string)] = l
	}
	if len(got) != 2 {
		t.Fatalf("lists: %v", out.Lists)
	}
	if l := got["l1"]; l["total"] != 4.0 || l["active"] != 3.0 || l["unsubscribed"] != 1.0 || l["name"] != "list" {
		t.Errorf("l1 = %v", l)
	} else if _, has := l["description"]; has {
		t.Errorf("empty description must be omitted: %v", l)
	}
	if l := got["l2"]; l["total"] != 0.0 || l["active"] != 0.0 || l["description"] != "Shop" {
		t.Errorf("l2 = %v", l)
	}
}

func TestAPIAddRecipients_NewInvalidDuplicates(t *testing.T) {
	srv, d, key := newAPIListsServer(t)
	body := `{"recipients":[
		{"email":"  New@Example.COM ","name":"First","variables":{"city":"Kazan"}},
		{"email":"new@example.com","name":"Second"},
		{"email":"not-an-email"},
		{"email":""},
		{"email":"Ann <ann@example.com>"},
		{"email":"x@nodot"},
		{"email":"other@example.com"}]}`
	r := decodeAdd(t, apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", body))
	if r.Added != 2 || r.Updated != 0 || r.Existing != 0 || r.Unsubscribed != 0 || len(r.Invalid) != 4 {
		t.Errorf("resp = %+v", r)
	}
	if r.Invalid[0].Email != "not-an-email" || r.Invalid[0].Error == "" {
		t.Errorf("invalid = %+v", r.Invalid)
	}
	// duplicate in the request: last wins (name Second); the first variables are lost with it
	name, vars, status, found := recipientRow(t, d, "new@example.com")
	if !found || name != "Second" || status != "active" || vars != "" {
		t.Errorf("row = %q %q %q %v", name, vars, status, found)
	}
	var total, active int
	d.QueryRow(`SELECT total_count, active_count FROM recipient_lists WHERE id='l1'`).Scan(&total, &active)
	if total != 6 || active != 5 {
		t.Errorf("counters = %d/%d", total, active)
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='api_recipients_add' AND entity_id='l1'`).Scan(&n)
	if n != 1 {
		t.Errorf("audit rows = %d", n)
	}
}

func TestAPIAddRecipients_ExistingAndUpdate(t *testing.T) {
	srv, d, key := newAPIListsServer(t)
	d.Exec(`UPDATE recipients SET name='Old', variables='{"city":"Moscow","plan":"pro"}' WHERE email='a@example.org'`)
	body := `{"recipients":[{"email":"A@example.org","name":"Ann","variables":{"city":"Kazan"}}]}`

	// update_existing absent/false: reported as existing, untouched
	r := decodeAdd(t, apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", body))
	if r.Existing != 1 || r.Updated != 0 || r.Added != 0 {
		t.Errorf("resp = %+v", r)
	}
	if name, vars, _, _ := recipientRow(t, d, "a@example.org"); name != "Old" || !strings.Contains(vars, "Moscow") {
		t.Errorf("row changed: %q %q", name, vars)
	}

	// update_existing true: name replaced, variables merged
	body = `{"update_existing":true,"recipients":[{"email":"a@example.org","name":"Ann","variables":{"city":"Kazan"}}]}`
	r = decodeAdd(t, apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", body))
	if r.Updated != 1 || r.Existing != 0 {
		t.Errorf("resp = %+v", r)
	}
	name, vars, status, _ := recipientRow(t, d, "a@example.org")
	var m map[string]any
	json.Unmarshal([]byte(vars), &m)
	if name != "Ann" || status != "active" || m["city"] != "Kazan" || m["plan"] != "pro" {
		t.Errorf("row = %q %q %q", name, vars, status)
	}

	// empty name in the request does not blank the stored name
	apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", `{"update_existing":true,"recipients":[{"email":"a@example.org"}]}`)
	if name, _, _, _ := recipientRow(t, d, "a@example.org"); name != "Ann" {
		t.Errorf("name blanked: %q", name)
	}
}

func TestAPIAddRecipients_UnsubscribedNeverReactivated(t *testing.T) {
	srv, d, key := newAPIListsServer(t)
	for _, upd := range []string{"false", "true"} {
		body := fmt.Sprintf(`{"update_existing":%s,"recipients":[{"email":"B@example.org","name":"Bob","variables":{"x":"1"}}]}`, upd)
		r := decodeAdd(t, apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", body))
		if r.Unsubscribed != 1 || r.Added != 0 || r.Updated != 0 || r.Existing != 0 {
			t.Errorf("update_existing=%s resp = %+v", upd, r)
		}
		name, vars, status, _ := recipientRow(t, d, "b@example.org")
		if status != "unsubscribed" || name != "" || vars != "" {
			t.Errorf("update_existing=%s row = %q %q %q", upd, name, vars, status)
		}
	}
}

func TestAPIAddRecipients_Errors(t *testing.T) {
	srv, d, key := newAPIListsServer(t)

	rec := apiDo(srv, key, "POST", "/api/v1/lists/nope/recipients", `{"recipients":[{"email":"a@b.com"}]}`)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "LIST_NOT_FOUND") {
		t.Errorf("unknown list: %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", `{bad`); rec.Code != 400 {
		t.Errorf("bad json: %d", rec.Code)
	}
	if rec := apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", `{"recipients":[]}`); rec.Code != 400 {
		t.Errorf("empty: %d", rec.Code)
	}

	// exactly 1000 is fine, 1001 is rejected before anything is written
	mk := func(n int) string {
		var sb strings.Builder
		sb.WriteString(`{"recipients":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `{"email":"bulk%d@example.com"}`, i)
		}
		sb.WriteString("]}")
		return sb.String()
	}
	rec = apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", mk(1001))
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "TOO_MANY_RECIPIENTS") {
		t.Errorf(">1000: %d %s", rec.Code, rec.Body.String())
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM recipients WHERE email LIKE 'bulk%'`).Scan(&n)
	if n != 0 {
		t.Errorf("rejected request wrote %d rows", n)
	}
	if r := decodeAdd(t, apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", mk(1000))); r.Added != 1000 {
		t.Errorf("1000: %+v", r)
	}

	// body over the size limit
	huge := `{"recipients":[{"email":"a@b.com","name":"` + strings.Repeat("x", 6<<20) + `"}]}`
	if rec := apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", huge); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("huge body: %d", rec.Code)
	}
}

func TestAPIDeleteRecipient(t *testing.T) {
	srv, d, key := newAPIListsServer(t)

	if rec := apiDo(srv, key, "DELETE", "/api/v1/lists/l1/recipients/C%40Example.org", ""); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if _, _, _, found := recipientRow(t, d, "c@example.org"); found {
		t.Error("recipient still there")
	}
	var total, active int
	d.QueryRow(`SELECT total_count, active_count FROM recipient_lists WHERE id='l1'`).Scan(&total, &active)
	if total != 3 || active != 2 {
		t.Errorf("counters = %d/%d", total, active)
	}
	if rec := apiDo(srv, key, "DELETE", "/api/v1/lists/l1/recipients/c@example.org", ""); rec.Code != 404 {
		t.Errorf("again: %d", rec.Code)
	}
	if rec := apiDo(srv, key, "DELETE", "/api/v1/lists/nope/recipients/a@example.org", ""); rec.Code != 404 {
		t.Errorf("unknown list: %d", rec.Code)
	}
	// unsubscribed address is kept (suppression)
	if rec := apiDo(srv, key, "DELETE", "/api/v1/lists/l1/recipients/b@example.org", ""); rec.Code != 409 {
		t.Errorf("unsubscribed: %d", rec.Code)
	}
	if _, _, st, found := recipientRow(t, d, "b@example.org"); !found || st != "unsubscribed" {
		t.Error("unsubscribed recipient was removed")
	}
	// address with campaign history: 409 like the UI
	d.Exec(`INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status) VALUES ('j1','c1','l1','completed')`)
	if _, err := d.Exec(`INSERT INTO send_job_items (id, job_id, recipient_id, status) VALUES ('i1','j1','ra','sent')`); err != nil {
		t.Skipf("fixture for in-use case: %v", err)
	}
	if rec := apiDo(srv, key, "DELETE", "/api/v1/lists/l1/recipients/a@example.org", ""); rec.Code != 409 {
		t.Errorf("in use: %d %s", rec.Code, rec.Body.String())
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='api_recipient_delete'`).Scan(&n)
	if n != 1 {
		t.Errorf("audit rows = %d", n)
	}
}

// The mux already unescapes {email}; a second unescape turned a literal
// "%41" in the local part into "A" and missed the address.
func TestAPIDeleteRecipient_PercentInLocalPart(t *testing.T) {
	srv, d, key := newAPIListsServer(t)
	if r := decodeAdd(t, apiDo(srv, key, "POST", "/api/v1/lists/l1/recipients", `{"recipients":[{"email":"x%41y@example.org"}]}`)); r.Added != 1 {
		t.Fatalf("add: %+v", r)
	}
	if rec := apiDo(srv, key, "DELETE", "/api/v1/lists/l1/recipients/x%2541y@example.org", ""); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if _, _, _, found := recipientRow(t, d, "x%41y@example.org"); found {
		t.Error("recipient still there")
	}
}
