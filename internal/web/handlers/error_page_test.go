package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/middleware"
)

func TestErrorPage(t *testing.T) {
	h, _, cleanup := newDomainViewTestHandlers(t)
	defer cleanup()

	call := func(mode, accept string, fn http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if mode != "" {
			req.Header.Set("Sec-Fetch-Mode", mode)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		rec := httptest.NewRecorder()
		fn(rec, req)
		return rec
	}
	notFound := func(w http.ResponseWriter, r *http.Request) {
		h.error(w, r, http.StatusNotFound, "DKIM key not found")
	}

	// A page load gets the panel page with the status kept.
	rec := call("navigate", "", notFound)
	if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("navigate: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if body := rec.Body.String(); !strings.Contains(body, "err.not_found_title") || !strings.Contains(body, "DKIM key not found") {
		t.Errorf("navigate body: %s", body)
	}

	// Without Sec-Fetch-Mode, Accept decides.
	if rec := call("", "text/html,application/xhtml+xml", notFound); !strings.Contains(rec.Body.String(), "err.not_found_title") {
		t.Errorf("old browser should get the page: %s", rec.Body.String())
	}

	// fetch() keeps the plain text that scripts show to the user.
	for _, c := range []struct{ mode, accept string }{{"cors", ""}, {"same-origin", "*/*"}, {"", ""}} {
		rec := call(c.mode, c.accept, notFound)
		if rec.Code != http.StatusNotFound || strings.TrimSpace(rec.Body.String()) != "DKIM key not found" {
			t.Errorf("fetch %+v: %d %q", c, rec.Code, rec.Body.String())
		}
	}

	// Admin-only pages answer a non-admin with the access denied page.
	admin := middleware.AdminOnly(h.Forbidden)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler reached by a non-admin")
	}))
	rec = call("navigate", "", admin.ServeHTTP)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "err.forbidden_title") {
		t.Errorf("forbidden: %d %s", rec.Code, rec.Body.String())
	}
}

func TestNotFoundRoute(t *testing.T) {
	h, _, cleanup := newDomainViewTestHandlers(t)
	defer cleanup()

	get := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		rec := httptest.NewRecorder()
		h.NotFound(rec, req)
		return rec
	}

	// A copied link with a trailing slash opens the page it points to.
	rec := get("/recipients/07c24b78/?tab=history")
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/recipients/07c24b78?tab=history" {
		t.Fatalf("trailing slash: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// A double slash must not become a redirect to another host.
	if rec := get("//evil.example/"); rec.Code != http.StatusNotFound {
		t.Errorf("//host/: got %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// An unknown page is 404, not the dashboard.
	if rec := get("/no-such-page"); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "err.not_found_title") {
		t.Errorf("unknown page: %d", rec.Code)
	}
}
