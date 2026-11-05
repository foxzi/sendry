package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/sendry"
	"github.com/foxzi/sendry/internal/web/views"
)

// fakeMTA serves GET /api/v1/domains from a list the test can change.
type fakeMTA struct {
	mu      sync.Mutex
	domains []*sendry.Domain
	fail    atomic.Bool
	calls   atomic.Int32
}

func (f *fakeMTA) set(domains ...*sendry.Domain) {
	f.mu.Lock()
	f.domains = domains
	f.mu.Unlock()
}

func (f *fakeMTA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail.Load() {
		http.Error(w, `{"error":"down"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(sendry.DomainsListResponse{Domains: f.domains})
}

func newDriftTestHandlers(t *testing.T) (*Handlers, *db.DB, *fakeMTA) {
	t.Helper()
	mta := &fakeMTA{}
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
	return New(cfg, database, testLogger(), v, nil), database, mta
}

// deployedDomain stores a domain with a successful deployment to mta-1, as
// deployDomainToServer would record it.
func deployedDomain(t *testing.T, database *db.DB, d *models.Domain) *models.Domain {
	t.Helper()
	repo := repository.NewDomainRepository(database.DB)
	if err := repo.Create(d); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateDeployment(d.ID, "mta-1", "deployed", d.ConfigHash(), ""); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestDeploymentStates(t *testing.T) {
	h, database, mta := newDriftTestHandlers(t)
	d := deployedDomain(t, database, &models.Domain{
		Domain: "shop.example", Mode: "sandbox", RateLimitHour: 100,
	})
	onServer := func() *sendry.Domain {
		return &sendry.Domain{Domain: "shop.example", Mode: "sandbox",
			RateLimit: &sendry.RateLimitCfg{MessagesPerHour: 100}}
	}
	state := func() deploymentState {
		t.Helper()
		snap := h.serverDomains(context.Background(), deployedServers(d), true)
		return h.deploymentStates(d, snap)["mta-1"]
	}

	mta.set(onServer())
	if st := state(); st.Status != stateSynced {
		t.Errorf("same settings: %+v", st)
	}

	changed := onServer()
	changed.Mode = "production"
	changed.RateLimit = nil
	mta.set(changed)
	st := state()
	if st.Status != stateDiffers || strings.Join(st.Diff, ",") != "mode,rate_limit" {
		t.Errorf("changed on server: %+v", st)
	}
	if !st.NeedsSync() {
		t.Error("differs should need sync")
	}

	mta.set()
	if st := state(); st.Status != stateMissing || !st.NeedsSync() {
		t.Errorf("deleted on server: %+v", st)
	}

	mta.fail.Store(true)
	if st := state(); st.Status != stateUnknown || st.NeedsSync() {
		t.Errorf("server down: %+v", st)
	}
	mta.fail.Store(false)

	// A panel-side change wins over whatever the server has.
	d.Mode = "production"
	mta.set(onServer())
	if st := state(); st.Status != stateOutdated {
		t.Errorf("changed in panel: %+v", st)
	}
}

func TestServerDomainsCache(t *testing.T) {
	h, _, mta := newDriftTestHandlers(t)
	ctx := context.Background()
	mta.set(&sendry.Domain{Domain: "a.example"})

	h.serverDomains(ctx, []string{"mta-1", "mta-1"}, false)
	h.serverDomains(ctx, []string{"mta-1"}, false)
	if n := mta.calls.Load(); n != 1 {
		t.Fatalf("fresh cache: %d calls, want 1", n)
	}

	h.serverDomains(ctx, []string{"mta-1"}, true)
	if n := mta.calls.Load(); n != 2 {
		t.Fatalf("refresh: %d calls, want 2", n)
	}

	h.serverDomainsCache.invalidate("mta-1")
	snap := h.serverDomains(ctx, []string{"mta-1"}, false)
	if n := mta.calls.Load(); n != 3 {
		t.Fatalf("after invalidate: %d calls, want 3", n)
	}
	if snap["mta-1"].domains["a.example"] == nil {
		t.Error("domain missing from snapshot")
	}

	// An unknown server is reported, not fatal.
	if e := h.serverDomains(ctx, []string{"nope"}, false)["nope"]; e.err == nil {
		t.Error("unknown server should carry an error")
	}
}

func TestCentralDomainViewShowsDrift(t *testing.T) {
	h, database, mta := newDriftTestHandlers(t)
	d := deployedDomain(t, database, &models.Domain{Domain: "gone.example", Mode: "production"})
	mta.set() // deleted directly on the server

	body := renderCentralDomainViewHTML(t, h, d.ID)
	if !strings.Contains(body, `data-i18n="infra.missing_on_server"`) {
		t.Error("card does not show the domain as missing on the server")
	}
	if !strings.Contains(body, `/domains/`+d.ID+`/sync`) {
		t.Error("card does not offer a sync")
	}
}

func TestSummarizeStates(t *testing.T) {
	st := func(statuses ...string) map[string]deploymentState {
		m := map[string]deploymentState{}
		for i, s := range statuses {
			m[string(rune('a'+i))] = deploymentState{Status: s}
		}
		return m
	}
	for _, tc := range []struct {
		states        map[string]deploymentState
		synced, total int
		badge         string
	}{
		{st(stateSynced, stateSynced, stateSynced), 3, 3, "success"},
		{st(stateSynced, stateUnknown), 1, 2, "draft"},
		{st(stateSynced, stateUnknown, stateOutdated), 1, 3, "warning"},
		{st(stateSynced, stateOutdated, stateMissing), 1, 3, "failed"},
		{st(stateDiffers), 0, 1, "failed"},
		{st(stateFailed, stateSynced), 1, 2, "failed"},
	} {
		got := summarizeStates(tc.states)
		if got.Synced != tc.synced || got.Total != tc.total || got.Badge != tc.badge {
			t.Errorf("%v: got %+v", tc.states, got)
		}
	}
	if got := summarizeStates(st(stateMissing, stateFailed, stateOutdated)); got.Label != "infra.sum.failed" {
		t.Errorf("failed should win the label: %+v", got)
	}
	got := summarizeStates(map[string]deploymentState{"mta-1": {Status: stateSynced}}, "mta-3", "mta-1", "mta-2")
	if got.Total != 1 || got.Badge != "success" || strings.Join(got.NotDeployed, ",") != "mta-2,mta-3" {
		t.Errorf("not deployed servers: %+v", got)
	}
	if got := summarizeStates(st(stateSynced, stateOutdated)); got.Label != "infra.sum.outdated" {
		t.Errorf("outdated label: %+v", got)
	}
}

func TestCentralDomainsListShowsServerStates(t *testing.T) {
	h, database, mta := newDriftTestHandlers(t)
	deployedDomain(t, database, &models.Domain{Domain: "one.example", Mode: "production"})
	deployedDomain(t, database, &models.Domain{Domain: "two.example", Mode: "production"})
	mta.set(&sendry.Domain{Domain: "one.example", Mode: "production"}) // two.example is gone

	req := httptest.NewRequest(http.MethodGet, "/domains", nil)
	w := httptest.NewRecorder()
	h.CentralDomainsList(w, req)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "</html>") {
		t.Fatalf("status %d, page not rendered to the end: %s", w.Code, body)
	}
	for _, want := range []string{"srv-icon--success", "srv-icon--failed", `data-i18n="infra.missing_on_server"`, "mta-1"} {
		if !strings.Contains(body, want) {
			t.Errorf("list does not contain %q", want)
		}
	}
}
