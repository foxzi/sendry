package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/sendry"
	"github.com/foxzi/sendry/internal/web/views"
)

// dkimMTA is a fake MTA that records what the panel sends to it.
type dkimMTA struct {
	mu         sync.Mutex
	calls      []string // "METHOD path"
	uploads    []sendry.DKIMUploadRequest
	domainReqs []sendry.DomainCreateRequest
	failUpload bool
}

const dkimMTAKeyFile = "/var/lib/sendry/dkim/uploaded.key"

func (f *dkimMTA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/dkim/upload":
		var req sendry.DKIMUploadRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.uploads = append(f.uploads, req)
		if f.failUpload {
			http.Error(w, `{"error":"disk full"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(sendry.DKIMResponse{Domain: req.Domain, Selector: req.Selector, KeyFile: dkimMTAKeyFile})
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/domains/"):
		var req sendry.DomainCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.domainReqs = append(f.domainReqs, req)
		_ = json.NewEncoder(w).Encode(sendry.Domain{Domain: strings.TrimPrefix(r.URL.Path, "/api/v1/domains/")})
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		_ = json.NewEncoder(w).Encode(sendry.DomainsListResponse{})
	}
}

func (f *dkimMTA) snapshot() (calls []string, uploads []sendry.DKIMUploadRequest, domainReqs []sendry.DomainCreateRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...), append([]sendry.DKIMUploadRequest(nil), f.uploads...), append([]sendry.DomainCreateRequest(nil), f.domainReqs...)
}

func newDKIMTestHandlers(t *testing.T) (*Handlers, *dkimMTA) {
	t.Helper()
	mta := &dkimMTA{}
	srv := httptest.NewServer(mta)
	t.Cleanup(srv.Close)

	database, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	v, err := views.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Sendry.Servers = []config.SendryServer{{Name: "mta-1", BaseURL: srv.URL}}
	return New(cfg, database, testLogger(), v, nil), mta
}

func createTestDKIMKey(t *testing.T, h *Handlers, domain string) *models.DKIMKey {
	t.Helper()
	key := &models.DKIMKey{Domain: domain, Selector: "sendry", PrivateKey: "PRIVATE-PEM", DNSRecord: "v=DKIM1; k=rsa; p=abc"}
	if err := h.dkim.Create(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func createDKIMTestDomain(t *testing.T, h *Handlers, d *models.Domain) *models.Domain {
	t.Helper()
	if d.Mode == "" {
		d.Mode = "production"
	}
	if err := h.domains.Create(d); err != nil {
		t.Fatal(err)
	}
	return d
}

// lastDeployment returns the domain's deployment record for mta-1.
func lastDeployment(t *testing.T, h *Handlers, domainID string) models.DomainDeployment {
	t.Helper()
	d, err := h.domains.GetByID(domainID)
	if err != nil || d == nil {
		t.Fatalf("GetByID() = %v, %v", d, err)
	}
	if len(d.Deployments) != 1 || d.Deployments[0].ServerName != "mta-1" {
		t.Fatalf("deployments = %+v, want one for mta-1", d.Deployments)
	}
	return d.Deployments[0]
}

// Creating a key only stores it in the panel: nothing is sent to a server,
// whichever form it comes from and whatever the old form fields say.
func TestDKIMCreateDoesNotUpload(t *testing.T) {
	forms := map[string]func(h *Handlers, w http.ResponseWriter, r *http.Request){
		"central": func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.CentralDKIMCreate(w, r) },
		"server page": func(h *Handlers, w http.ResponseWriter, r *http.Request) {
			r.SetPathValue("server", "mta-1")
			h.DKIMCreate(w, r)
		},
	}
	for name, create := range forms {
		t.Run(name, func(t *testing.T) {
			h, mta := newDKIMTestHandlers(t)
			post := func(form url.Values) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodPost, "/dkim", strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				w := httptest.NewRecorder()
				create(h, w, r)
				return w
			}

			// servers/auto_deploy are what the removed form controls used to send.
			form := url.Values{"domain": {" example.com "}, "selector": {"sendry"}, "servers": {"mta-1"}, "auto_deploy": {"on"}}
			w := post(form)
			if w.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d. Body: %s", w.Code, http.StatusSeeOther, w.Body.String())
			}

			found, err := h.dkim.GetByDomainSelector("example.com", "sendry")
			if err != nil || found == nil {
				t.Fatalf("key not stored: %v, %v", found, err)
			}
			if loc := w.Header().Get("Location"); loc != "/dkim/"+found.ID {
				t.Errorf("Location = %q, want %q", loc, "/dkim/"+found.ID)
			}
			key, err := h.dkim.GetByID(found.ID)
			if err != nil || key == nil {
				t.Fatalf("GetByID() = %v, %v", key, err)
			}
			if !strings.Contains(key.PrivateKey, "RSA PRIVATE KEY") {
				t.Error("private key is not a PEM RSA key")
			}
			if !strings.HasPrefix(key.DNSRecord, "v=DKIM1; k=rsa; p=") {
				t.Errorf("DNS record = %q", key.DNSRecord)
			}
			if len(key.Deployments) != 0 {
				t.Errorf("key has deployments %+v, want none", key.Deployments)
			}
			if calls, _, _ := mta.snapshot(); len(calls) != 0 {
				t.Errorf("creating a key called the server: %v", calls)
			}

			if w := post(form); w.Code != http.StatusConflict {
				t.Errorf("duplicate key: status = %d, want %d", w.Code, http.StatusConflict)
			}
			if w := post(url.Values{"domain": {"example.com"}}); w.Code != http.StatusBadRequest {
				t.Errorf("missing selector: status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			if calls, _, _ := mta.snapshot(); len(calls) != 0 {
				t.Errorf("rejected requests called the server: %v", calls)
			}
		})
	}
}

func renderDKIMView(t *testing.T, h *Handlers, keyID string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/dkim/"+keyID, nil)
	r.SetPathValue("id", keyID)
	w := httptest.NewRecorder()
	h.CentralDKIMView(w, r)
	return w.Code, w.Body.String()
}

// The key page points at the domains that deploy the key.
func TestCentralDKIMViewDeploymentHint(t *testing.T) {
	const (
		hintLinked = `data-i18n="infra.dkim_deploy_domain"`
		hintLink   = `data-i18n="infra.dkim_link_in_domain"`
		hintNone   = `data-i18n="infra.dkim_no_panel_domain"`
	)
	only := func(t *testing.T, body, want string) {
		t.Helper()
		for _, hint := range []string{hintLinked, hintLink, hintNone} {
			if got := strings.Contains(body, hint); got != (hint == want) {
				t.Errorf("hint %s present = %v, want %v", hint, got, hint == want)
			}
		}
	}

	h, mta := newDKIMTestHandlers(t)
	key := createTestDKIMKey(t, h, "example.com")

	// No domain with the key's name in the panel.
	code, body := renderDKIMView(t, h, key.ID)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", code, body)
	}
	only(t, body, hintNone)
	if !strings.Contains(body, `href="/domains/new"`) {
		t.Error("page does not offer to add the domain")
	}
	if strings.Contains(body, "/dkim/"+key.ID+"/deploy") || strings.Contains(body, "/deployments/") {
		t.Error("page still has deploy/remove controls")
	}

	// A domain with the same name exists, but the key is not selected in it.
	sameName := createDKIMTestDomain(t, h, &models.Domain{Domain: "example.com"})
	_, body = renderDKIMView(t, h, key.ID)
	only(t, body, hintLink)
	if !strings.Contains(body, `href="/domains/`+sameName.ID+`"`) {
		t.Error("page does not link the domain with the key's name")
	}

	// Domains that use the key win over the same-name domain, sorted by name.
	second := createDKIMTestDomain(t, h, &models.Domain{Domain: "other.com", DKIMEnabled: true, DKIMKeyID: key.ID})
	first := createDKIMTestDomain(t, h, &models.Domain{Domain: "another.com", DKIMEnabled: true, DKIMKeyID: key.ID})
	_, body = renderDKIMView(t, h, key.ID)
	only(t, body, hintLinked)
	i, j := strings.Index(body, `href="/domains/`+first.ID+`"`), strings.Index(body, `href="/domains/`+second.ID+`"`)
	if i < 0 || j < 0 || i > j {
		t.Errorf("linked domains missing or unsorted: another.com at %d, other.com at %d", i, j)
	}
	if strings.Contains(body, `href="/domains/`+sameName.ID+`"`) {
		t.Error("page links the same-name domain although the key is used elsewhere")
	}

	if code, _ := renderDKIMView(t, h, "no-such-key"); code != http.StatusNotFound {
		t.Errorf("unknown key: status = %d, want %d", code, http.StatusNotFound)
	}
	if calls, _, _ := mta.snapshot(); len(calls) != 0 {
		t.Errorf("viewing a key called the server: %v", calls)
	}
}

// Deploying a domain is the only way a key reaches a server.
func TestDeployDomainToServerDKIM(t *testing.T) {
	req := func() *http.Request { return httptest.NewRequest(http.MethodPost, "/", nil) }

	t.Run("uploads the linked key with the domain", func(t *testing.T) {
		h, mta := newDKIMTestHandlers(t)
		// The key was made for another domain name: the upload goes by the key.
		key := createTestDKIMKey(t, h, "key-domain.com")
		d := createDKIMTestDomain(t, h, &models.Domain{Domain: "other.com", DKIMEnabled: true, DKIMKeyID: key.ID})

		h.deployDomainToServer(req(), d, "mta-1")

		calls, uploads, domainReqs := mta.snapshot()
		if len(uploads) != 1 || uploads[0].Domain != "key-domain.com" || uploads[0].Selector != "sendry" || uploads[0].PrivateKey != "PRIVATE-PEM" {
			t.Fatalf("uploads = %+v", uploads)
		}
		if len(calls) != 2 || calls[0] != "POST /api/v1/dkim/upload" || calls[1] != "PUT /api/v1/domains/other.com" {
			t.Errorf("calls = %v, want key upload then domain update", calls)
		}
		if len(domainReqs) != 1 || domainReqs[0].DKIM == nil {
			t.Fatalf("domain requests = %+v, want one with DKIM", domainReqs)
		}
		if got := *domainReqs[0].DKIM; !got.Enabled || got.Selector != "sendry" || got.KeyFile != dkimMTAKeyFile {
			t.Errorf("domain DKIM = %+v", got)
		}
		if dep := lastDeployment(t, h, d.ID); dep.Status != "deployed" || dep.Error != "" {
			t.Errorf("domain deployment = %+v", dep)
		}
		stored, err := h.dkim.GetByID(key.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stored.Deployments) != 1 || stored.Deployments[0].ServerName != "mta-1" || stored.Deployments[0].Status != "deployed" {
			t.Errorf("key deployments = %+v, want deployed on mta-1", stored.Deployments)
		}

		// Deleting the key removes it from the servers it was uploaded to.
		r := httptest.NewRequest(http.MethodDelete, "/dkim/"+key.ID, nil)
		r.SetPathValue("id", key.ID)
		w := httptest.NewRecorder()
		h.CentralDKIMDelete(w, r)
		if w.Code != http.StatusSeeOther {
			t.Fatalf("delete status = %d. Body: %s", w.Code, w.Body.String())
		}
		calls, _, _ = mta.snapshot()
		if last := calls[len(calls)-1]; last != "DELETE /api/v1/dkim/key-domain.com/sendry" {
			t.Errorf("last call = %q, want the key removed from mta-1", last)
		}
		if gone, _ := h.dkim.GetByID(key.ID); gone != nil {
			t.Error("key still in the panel after delete")
		}
	})

	// In the cases below DKIM is on but there is no key to send. The MTA
	// replaces the whole domain config, so the domain must not be sent at all.
	notDeployed := func(t *testing.T, h *Handlers, mta *dkimMTA, d *models.Domain, wantUploads int) {
		t.Helper()
		calls, uploads, domainReqs := mta.snapshot()
		if len(uploads) != wantUploads {
			t.Errorf("uploads = %d, want %d", len(uploads), wantUploads)
		}
		if len(domainReqs) != 0 || len(calls) != wantUploads {
			t.Errorf("domain was sent to the server: %v", calls)
		}
		dep := lastDeployment(t, h, d.ID)
		if dep.Status != "failed" || !strings.Contains(dep.Error, "no DKIM key could be deployed") {
			t.Errorf("domain deployment = %+v, want failed with a DKIM hint", dep)
		}
	}

	t.Run("dkim on without a linked key", func(t *testing.T) {
		h, mta := newDKIMTestHandlers(t)
		d := createDKIMTestDomain(t, h, &models.Domain{Domain: "example.com", DKIMEnabled: true})

		h.deployDomainToServer(req(), d, "mta-1")

		notDeployed(t, h, mta, d, 0)
	})

	t.Run("linked key is gone", func(t *testing.T) {
		h, mta := newDKIMTestHandlers(t)
		d := createDKIMTestDomain(t, h, &models.Domain{Domain: "example.com", DKIMEnabled: true})
		d.DKIMKeyID = "deleted-key"

		h.deployDomainToServer(req(), d, "mta-1")

		notDeployed(t, h, mta, d, 0)
	})

	t.Run("key upload fails", func(t *testing.T) {
		h, mta := newDKIMTestHandlers(t)
		mta.failUpload = true
		key := createTestDKIMKey(t, h, "example.com")
		d := createDKIMTestDomain(t, h, &models.Domain{Domain: "example.com", DKIMEnabled: true, DKIMKeyID: key.ID})

		h.deployDomainToServer(req(), d, "mta-1")

		notDeployed(t, h, mta, d, 1)
		stored, err := h.dkim.GetByID(key.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stored.Deployments) != 0 {
			t.Errorf("key deployments = %+v, want none after a failed upload", stored.Deployments)
		}
	})

	t.Run("dkim off does not upload a linked key", func(t *testing.T) {
		h, mta := newDKIMTestHandlers(t)
		key := createTestDKIMKey(t, h, "example.com")
		d := createDKIMTestDomain(t, h, &models.Domain{Domain: "example.com", DKIMEnabled: false, DKIMKeyID: key.ID})

		h.deployDomainToServer(req(), d, "mta-1")

		calls, uploads, domainReqs := mta.snapshot()
		if len(uploads) != 0 {
			t.Errorf("key uploaded although DKIM is off: %v", calls)
		}
		if len(domainReqs) != 1 || domainReqs[0].DKIM != nil {
			t.Errorf("domain requests = %+v, want one without DKIM", domainReqs)
		}
		if dep := lastDeployment(t, h, d.ID); dep.Status != "deployed" {
			t.Errorf("domain deployment = %+v", dep)
		}
	})

	t.Run("unknown server", func(t *testing.T) {
		h, mta := newDKIMTestHandlers(t)
		key := createTestDKIMKey(t, h, "example.com")
		d := createDKIMTestDomain(t, h, &models.Domain{Domain: "example.com", DKIMEnabled: true, DKIMKeyID: key.ID})

		h.deployDomainToServer(req(), d, "mta-9")

		if calls, _, _ := mta.snapshot(); len(calls) != 0 {
			t.Errorf("calls = %v, want none", calls)
		}
		stored, err := h.domains.GetByID(d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stored.Deployments) != 1 || stored.Deployments[0].ServerName != "mta-9" || stored.Deployments[0].Status != "failed" {
			t.Errorf("deployments = %+v, want failed for mta-9", stored.Deployments)
		}
	})
}
