package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/tracking"
)

func TestGetUserEmail(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		wantEmail string
	}{
		{"with email in context", "user@example.com", "user@example.com"},
		{"with empty email", "", ""},
		{"no email in context", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.email != "" {
				ctx := context.WithValue(r.Context(), ctxKeyUserEmail, tt.email)
				r = r.WithContext(ctx)
			}
			got := GetUserEmail(r)
			if got != tt.wantEmail {
				t.Errorf("GetUserEmail() = %q, want %q", got, tt.wantEmail)
			}
		})
	}
}

func TestGetUserEmail_WrongType(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), ctxKeyUserEmail, 12345)
	r = r.WithContext(ctx)
	got := GetUserEmail(r)
	if got != "" {
		t.Errorf("GetUserEmail() with wrong type = %q, want empty", got)
	}
}

func TestGetAPIKeyFromContext_Nil(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	got := GetAPIKeyFromContext(r)
	if got != nil {
		t.Errorf("GetAPIKeyFromContext() without key = %v, want nil", got)
	}
}

func TestMethodOverride(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		formMethod string
		wantMethod string
	}{
		{"POST with DELETE override", "POST", "DELETE", "DELETE"},
		{"POST with PUT override", "POST", "PUT", "PUT"},
		{"POST without override", "POST", "", "POST"},
		{"GET ignores override", "GET", "DELETE", "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod string
			handler := MethodOverride(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
			}))

			r := httptest.NewRequest(tt.method, "/test", nil)
			if tt.formMethod != "" {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.Form = map[string][]string{"_method": {tt.formMethod}}
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if gotMethod != tt.wantMethod {
				t.Errorf("MethodOverride() method = %q, want %q", gotMethod, tt.wantMethod)
			}
		})
	}
}

func TestMethodOverride_SkipsPublicTrackingPaths(t *testing.T) {
	for _, path := range []string{"/u/token", "/t/c/token"} {
		var gotMethod string
		var formParsed bool
		handler := MethodOverride(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			formParsed = r.Form != nil
		}))
		r := httptest.NewRequest("POST", path, strings.NewReader("_method=DELETE"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler.ServeHTTP(httptest.NewRecorder(), r)
		// The body must be left for the handler, which limits its size
		if gotMethod != "POST" || formParsed {
			t.Errorf("%s: method = %q, form parsed = %v", path, gotMethod, formParsed)
		}
	}
	if !IsPublicTrackingPath("/t/o/x.gif") || !IsPublicTrackingPath("/u/x") || IsPublicTrackingPath("/templates/u/") {
		t.Error("IsPublicTrackingPath")
	}
}

func TestIPFilter_TrustedProxies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	proxies, err := tracking.ParseTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	handler := IPFilter([]string{"203.0.113.0/24"}, proxies, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xRealIP    string
		want       int
	}{
		{"allowed peer", "203.0.113.5:1234", "", "", http.StatusOK},
		{"denied peer", "198.51.100.7:1234", "", "", http.StatusForbidden},
		{"spoofed X-Forwarded-For from untrusted peer", "198.51.100.7:1234", "203.0.113.5", "", http.StatusForbidden},
		{"spoofed X-Real-IP from untrusted peer", "198.51.100.7:1234", "", "203.0.113.5", http.StatusForbidden},
		{"trusted proxy forwards allowed client", "127.0.0.1:1234", "203.0.113.5", "", http.StatusOK},
		{"trusted proxy forwards denied client", "127.0.0.1:1234", "198.51.100.7", "", http.StatusForbidden},
		{"trusted proxy X-Real-IP", "127.0.0.1:1234", "", "203.0.113.5", http.StatusOK},
		{"forged X-Real-IP passed through by proxy", "127.0.0.1:1234", "198.51.100.7", "203.0.113.5", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestSameOrigin(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := SameOrigin(ok)

	tests := []struct {
		name    string
		method  string
		headers map[string]string
		want    int
	}{
		{"GET from another site passes", "GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusNoContent},
		{"same-origin POST", "POST", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusNoContent},
		{"typed URL POST", "POST", map[string]string{"Sec-Fetch-Site": "none"}, http.StatusNoContent},
		{"cross-site POST", "POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"sibling subdomain POST", "POST", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "https://evil.example.com"}, http.StatusForbidden},
		{"Sec-Fetch-Site wins over a rewritten Host", "POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://panel.example.com"}, http.StatusNoContent},
		{"old browser, same Origin", "POST", map[string]string{"Origin": "http://panel.local"}, http.StatusNoContent},
		{"old browser, foreign Origin", "DELETE", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"no browser headers", "POST", nil, http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "http://panel.local/campaigns/x/section/from", nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/campaigns", nil))
	if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}

func TestUploads(t *testing.T) {
	dir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
	if err := os.WriteFile(filepath.Join(dir, "x.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	h := http.StripPrefix("/uploads/", Uploads(dir))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/uploads/x.svg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox") {
		t.Errorf("Content-Security-Policy = %q, want a sandbox policy", csp)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/uploads/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("directory listing status = %d, want 404", rec.Code)
	}
}
