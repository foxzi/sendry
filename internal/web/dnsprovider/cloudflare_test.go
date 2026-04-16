package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudflare_AuthHeaders_APIToken(t *testing.T) {
	var gotAuth, gotEmail, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotEmail = r.Header.Get("X-Auth-Email")
		gotKey = r.Header.Get("X-Auth-Key")

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  []map[string]string{{"id": "z1", "name": "example.com"}},
		})
	}))
	defer srv.Close()

	p := NewCloudflare("token-abc")
	p.BaseURL = srv.URL

	if _, err := p.ResolveZone(context.Background(), "mail.example.com"); err != nil {
		t.Fatalf("ResolveZone error = %v", err)
	}
	if gotAuth != "Bearer token-abc" {
		t.Errorf("Authorization = %q, want Bearer token-abc", gotAuth)
	}
	if gotEmail != "" || gotKey != "" {
		t.Errorf("unexpected X-Auth-* headers: email=%q key=%q", gotEmail, gotKey)
	}
}

func TestCloudflare_AuthHeaders_GlobalKey(t *testing.T) {
	var gotAuth, gotEmail, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotEmail = r.Header.Get("X-Auth-Email")
		gotKey = r.Header.Get("X-Auth-Key")

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  []map[string]string{{"id": "z1", "name": "example.com"}},
		})
	}))
	defer srv.Close()

	p := NewCloudflareGlobalKey("user@example.com", "GLOBALKEY")
	p.BaseURL = srv.URL

	if _, err := p.ResolveZone(context.Background(), "mail.example.com"); err != nil {
		t.Fatalf("ResolveZone error = %v", err)
	}
	if gotAuth != "" {
		t.Errorf("unexpected Authorization header: %q", gotAuth)
	}
	if gotEmail != "user@example.com" {
		t.Errorf("X-Auth-Email = %q", gotEmail)
	}
	if gotKey != "GLOBALKEY" {
		t.Errorf("X-Auth-Key = %q", gotKey)
	}
}
