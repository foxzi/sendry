package dnsprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNamedot_ResolveZone_WalksParents(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("bad auth header: %q", r.Header.Get("Authorization"))
		}
		name := r.URL.Query().Get("name")
		queries = append(queries, name)

		if name == "example.com" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   42,
				"name": "example.com.",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	z, err := p.ResolveZone(context.Background(), "mail.sub.example.com")
	if err != nil {
		t.Fatalf("ResolveZone error = %v", err)
	}
	if z.ID != "42" || z.Name != "example.com" {
		t.Fatalf("zone = %+v", z)
	}
	if len(queries) < 2 {
		t.Fatalf("expected multiple lookup attempts, got %v", queries)
	}
}

func TestNamedot_ResolveZone_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	_, err := p.ResolveZone(context.Background(), "mail.example.com")
	if err != ErrZoneNotFound {
		t.Fatalf("err = %v, want ErrZoneNotFound", err)
	}
}

func TestNamedot_ListRecords_FiltersByNameAndType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/rrsets") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":   7,
				"name": "example.com.",
				"type": "TXT",
				"ttl":  300,
				"records": []map[string]string{
					{"data": "\"v=spf1 a mx ~all\""},
				},
			},
			{
				"id":   8,
				"name": "_dmarc.example.com.",
				"type": "TXT",
				"ttl":  300,
				"records": []map[string]string{
					{"data": "\"v=DMARC1; p=none\""},
				},
			},
			{
				"id":   9,
				"name": "example.com.",
				"type": "MX",
				"records": []map[string]string{
					{"data": "10 mail.example.com."},
				},
			},
		})
	}))
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	records, err := p.ListRecords(context.Background(), "1", "example.com", "TXT")
	if err != nil {
		t.Fatalf("ListRecords error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1 (TXT at apex only)", len(records))
	}
	if records[0].Content != "v=spf1 a mx ~all" {
		t.Errorf("Content = %q, want unquoted SPF value", records[0].Content)
	}
	if records[0].ID != "7/0" {
		t.Errorf("ID = %q, want %q", records[0].ID, "7/0")
	}
}

// fakeNamedotRRSets serves GET /zones/1/rrsets with the given RRSets and
// records the body and path of the write that follows.
func fakeNamedotRRSets(t *testing.T, rrsets []map[string]any, method, path *string, body *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(rrsets)
			return
		}
		*method, *path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, body)
		_, _ = w.Write([]byte("{}"))
	}))
}

func recordData(t *testing.T, body map[string]any) []string {
	t.Helper()
	recs, ok := body["records"].([]any)
	if !ok {
		t.Fatalf("records = %v", body["records"])
	}
	var out []string
	for _, r := range recs {
		out = append(out, r.(map[string]any)["data"].(string))
	}
	return out
}

func TestNamedot_CreateRecord_NewRRSet(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := fakeNamedotRRSets(t, nil, &method, &path, &body)
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	err := p.CreateRecord(context.Background(), "1", Record{Type: "TXT", Name: "example.com", Content: "v=spf1 a mx ~all"})
	if err != nil {
		t.Fatalf("CreateRecord error = %v", err)
	}
	if method != http.MethodPost || path != "/zones/1/rrsets" {
		t.Errorf("%s %s, want POST /zones/1/rrsets", method, path)
	}
	if got := recordData(t, body); len(got) != 1 || got[0] != "\"v=spf1 a mx ~all\"" {
		t.Errorf("data = %q, want one quoted value", got)
	}
}

func TestNamedot_CreateRecord_AddsToExistingRRSet(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := fakeNamedotRRSets(t, []map[string]any{{
		"id": 7, "name": "example.com.", "type": "TXT", "ttl": 300,
		"records": []map[string]string{{"data": "\"google-site-verification=x\""}},
	}}, &method, &path, &body)
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	err := p.CreateRecord(context.Background(), "1", Record{Type: "TXT", Name: "example.com", Content: "v=spf1 a mx ~all"})
	if err != nil {
		t.Fatalf("CreateRecord error = %v", err)
	}
	if method != http.MethodPut || path != "/zones/1/rrsets/7" {
		t.Errorf("%s %s, want PUT /zones/1/rrsets/7", method, path)
	}
	want := []string{"\"google-site-verification=x\"", "\"v=spf1 a mx ~all\""}
	if got := recordData(t, body); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("data = %q, want %q", got, want)
	}
	if _, ok := body["id"]; ok {
		t.Errorf("rrset id sent in the body: %v", body)
	}
}

func TestNamedot_UpdateRecord_KeepsOtherValues(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := fakeNamedotRRSets(t, []map[string]any{{
		"id": 55, "name": "example.com.", "type": "TXT", "ttl": 300,
		"records": []map[string]string{{"data": "\"google-site-verification=x\""}, {"data": "\"v=spf1 -all\""}},
	}}, &method, &path, &body)
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	err := p.UpdateRecord(context.Background(), "1", "55/1", Record{Type: "TXT", Name: "example.com", Content: "v=spf1 a mx ~all"})
	if err != nil {
		t.Fatalf("UpdateRecord error = %v", err)
	}
	if method != http.MethodPut || path != "/zones/1/rrsets/55" {
		t.Errorf("%s %s, want PUT /zones/1/rrsets/55", method, path)
	}
	want := []string{"\"google-site-verification=x\"", "\"v=spf1 a mx ~all\""}
	if got := recordData(t, body); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("data = %q, want %q", got, want)
	}

	if err := p.UpdateRecord(context.Background(), "1", "55/5", Record{Type: "TXT"}); err == nil {
		t.Error("out of range value index accepted")
	}
	if err := p.UpdateRecord(context.Background(), "1", "55", Record{Type: "TXT"}); err == nil {
		t.Error("record id without index accepted")
	}
}

func TestNamedot_Do_URLEncodesName(t *testing.T) {
	var q url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	_, _ = p.ResolveZone(context.Background(), "example.com")
	if q.Get("name") == "" {
		t.Fatalf("name query param missing")
	}
}

func TestUnquoteTXT(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"v=spf1 a mx ~all"`, "v=spf1 a mx ~all"},
		// A long value stored as several strings is one value.
		{`"v=spf1 include:a.example ip4:198.51.100.0/24" " include:b.example -all"`, "v=spf1 include:a.example ip4:198.51.100.0/24 include:b.example -all"},
		{`"v=DKIM1; k=rsa; p=AAAA" "BBBB"`, "v=DKIM1; k=rsa; p=AAAABBBB"},
		{`"say \"hi\" \\ ok"`, `say "hi" \ ok`},
		{`plain value`, "plain value"},
		{`"open`, `"open`},
		{`"a" junk "b"`, `"a" junk "b"`},
	}
	for _, c := range cases {
		if got := unquoteTXT(c.in, "TXT"); got != c.want {
			t.Errorf("unquoteTXT(%s) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := unquoteTXT(`"x"`, "A"); got != `"x"` {
		t.Errorf("non-TXT data must stay as is, got %q", got)
	}
}
