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
	if records[0].ID != "7" {
		t.Errorf("ID = %q, want %q", records[0].ID, "7")
	}
}

func TestNamedot_CreateRecord_QuotesTXT(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	err := p.CreateRecord(context.Background(), "1", Record{
		Type:    "TXT",
		Name:    "example.com",
		Content: "v=spf1 a mx ~all",
	})
	if err != nil {
		t.Fatalf("CreateRecord error = %v", err)
	}

	recs, ok := body["records"].([]any)
	if !ok || len(recs) != 1 {
		t.Fatalf("records = %v", body["records"])
	}
	first := recs[0].(map[string]any)
	if first["data"] != "\"v=spf1 a mx ~all\"" {
		t.Errorf("data = %q, want quoted", first["data"])
	}
}

func TestNamedot_UpdateRecord_UsesPUT(t *testing.T) {
	var seenPath, seenMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenMethod = r.Method
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	p := NewNamedot(srv.URL, "tok")
	err := p.UpdateRecord(context.Background(), "10", "55", Record{
		Type:    "TXT",
		Name:    "_dmarc.example.com",
		Content: "v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com",
	})
	if err != nil {
		t.Fatalf("UpdateRecord error = %v", err)
	}
	if seenMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", seenMethod)
	}
	wantPath := "/zones/10/rrsets/55"
	if seenPath != wantPath {
		t.Errorf("path = %s, want %s", seenPath, wantPath)
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
