package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/foxzi/sendry/internal/web/dnsprovider"
)

// fakeNamedot is an in-memory namedot API with one zone.
type fakeNamedot struct {
	mu     sync.Mutex
	zone   string
	rrsets map[uint64]map[string]any
	nextID uint64
}

func (f *fakeNamedot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/zones":
		if r.URL.Query().Get("name") != f.zone {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": f.zone + "."})
	case r.URL.Path == "/zones/1/rrsets" && r.Method == http.MethodGet:
		list := []map[string]any{}
		for id, rs := range f.rrsets {
			c := map[string]any{"id": id}
			for k, v := range rs {
				c[k] = v
			}
			list = append(list, c)
		}
		_ = json.NewEncoder(w).Encode(list)
	case r.URL.Path == "/zones/1/rrsets" && r.Method == http.MethodPost:
		f.nextID++
		f.rrsets[f.nextID] = decodeBody(r)
		_, _ = w.Write([]byte("{}"))
	case strings.HasPrefix(r.URL.Path, "/zones/1/rrsets/") && r.Method == http.MethodPut:
		var id uint64
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/zones/1/rrsets/"), "%d", &id)
		f.rrsets[id] = decodeBody(r)
		_, _ = w.Write([]byte("{}"))
	default:
		http.NotFound(w, r)
	}
}

func decodeBody(r *http.Request) map[string]any {
	var m map[string]any
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &m)
	return m
}

func TestCentralDomainsPublishDNS(t *testing.T) {
	h, database, cleanup := newDomainViewTestHandlers(t)
	defer cleanup()
	domainID := createTestDomain(t, database, "pub.example")

	ns := &fakeNamedot{zone: "pub.example", nextID: 10, rrsets: map[uint64]map[string]any{
		10: {"name": "pub.example.", "type": "TXT", "records": []any{map[string]any{"data": `"google-site-verification=x"`}}},
	}}
	srv := httptest.NewServer(ns)
	defer srv.Close()
	h.dnsProviders = []dnsprovider.Provider{dnsprovider.NewNamedot(srv.URL, "tok")}

	req := httptest.NewRequest(http.MethodPost, "/domains/"+domainID+"/dns-publish", strings.NewReader("provider=namedot"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", domainID)
	w := httptest.NewRecorder()
	h.CentralDomainsPublishDNS(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	if q := loc.Query(); q.Get("count") != "2" || q.Get("publish_error") != "" || q.Get("published") != "namedot" {
		t.Fatalf("redirect = %s, want 2 records (SPF, DMARC; DKIM has no key) and no error", loc)
	}

	apex := ns.rrsets[10]["records"].([]any)
	if len(apex) != 2 || apex[0].(map[string]any)["data"] != `"google-site-verification=x"` ||
		apex[1].(map[string]any)["data"] != `"v=spf1 a mx ~all"` {
		t.Errorf("apex TXT = %v, want verification kept and SPF added", apex)
	}
	if len(ns.rrsets) != 2 {
		t.Errorf("rrsets = %v, want apex + _dmarc", ns.rrsets)
	}

	// The check now sees nothing left to publish.
	req = httptest.NewRequest(http.MethodGet, "/domains/"+domainID+"?check=true", nil)
	req.SetPathValue("id", domainID)
	w = httptest.NewRecorder()
	h.CentralDomainsView(w, req)
	body := w.Body.String()
	if !strings.Contains(body, `data-i18n="infra.dns_action_noop"`) || strings.Contains(body, "/dns-publish") {
		t.Errorf("after publishing the plan should be up to date with no publish button")
	}

	// Unknown provider.
	req = httptest.NewRequest(http.MethodPost, "/domains/"+domainID+"/dns-publish", strings.NewReader("provider=cloudflare"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", domainID)
	w = httptest.NewRecorder()
	h.CentralDomainsPublishDNS(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("unconfigured provider: status = %d", w.Code)
	}
}
