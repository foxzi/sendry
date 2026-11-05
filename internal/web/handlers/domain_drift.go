package handlers

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/sendry"
)

// Domain drift: compare what the panel deployed with what each MTA reports.
//
// The panel records a deployment (status and config hash) when it pushes a
// domain, but used to never look at the server again, so a domain deleted or
// edited directly on an MTA still showed "Deployed". Each server's domain list
// is fetched with one API call, whatever the number of domains, and cached for
// serverDomainsTTL. Changes the panel itself makes on a server drop that
// server's entry, so the next page shows the result right away.

const (
	serverDomainsTTL     = time.Minute
	serverDomainsTimeout = 3 * time.Second
)

// Deployment states shown for a domain on a server.
const (
	stateSynced      = "synced"       // the server has what the panel deployed
	stateOutdated    = "outdated"     // the panel's settings changed after the deployment
	stateFailed      = "failed"       // the last deployment failed
	stateMissing     = "missing"      // the panel deployed it, the server does not have it
	stateDiffers     = "differs"      // the server has it with other settings
	stateUnknown     = "unknown"      // the server could not be asked
	stateNotDeployed = "not_deployed" // the panel never deployed it there
)

type serverDomainsEntry struct {
	domains   map[string]*sendry.Domain
	err       error
	fetchedAt time.Time
}

type serverDomainsCache struct {
	mu      sync.Mutex
	entries map[string]serverDomainsEntry
}

func newServerDomainsCache() *serverDomainsCache {
	return &serverDomainsCache{entries: make(map[string]serverDomainsEntry)}
}

func (c *serverDomainsCache) invalidate(server string) {
	c.mu.Lock()
	delete(c.entries, server)
	c.mu.Unlock()
}

func (c *serverDomainsCache) store(server string, list []*sendry.Domain) {
	c.mu.Lock()
	c.entries[server] = newServerDomainsEntry(list, nil)
	c.mu.Unlock()
}

func newServerDomainsEntry(list []*sendry.Domain, err error) serverDomainsEntry {
	e := serverDomainsEntry{err: err, fetchedAt: time.Now()}
	if err == nil {
		e.domains = make(map[string]*sendry.Domain, len(list))
		for _, d := range list {
			e.domains[d.Domain] = d
		}
	}
	return e
}

// serverDomains returns each server's domain list, from the cache when it is
// fresh. Stale servers are asked in parallel; a failure is cached too, so an
// unreachable server does not slow down every page for the whole TTL.
func (h *Handlers) serverDomains(ctx context.Context, servers []string, refresh bool) map[string]serverDomainsEntry {
	out := make(map[string]serverDomainsEntry, len(servers))
	var stale []string
	c := h.serverDomainsCache
	c.mu.Lock()
	for _, s := range servers {
		if _, seen := out[s]; seen || slices.Contains(stale, s) {
			continue
		}
		if e, ok := c.entries[s]; ok && !refresh && time.Since(e.fetchedAt) < serverDomainsTTL {
			out[s] = e
		} else {
			stale = append(stale, s)
		}
	}
	c.mu.Unlock()
	if len(stale) == 0 {
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, serverDomainsTimeout)
	defer cancel()
	results := make([]serverDomainsEntry, len(stale))
	var wg sync.WaitGroup
	for i, s := range stale {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = h.fetchServerDomains(ctx, s)
		}()
	}
	wg.Wait()

	c.mu.Lock()
	for i, s := range stale {
		c.entries[s] = results[i]
		out[s] = results[i]
	}
	c.mu.Unlock()
	return out
}

func (h *Handlers) fetchServerDomains(ctx context.Context, server string) serverDomainsEntry {
	client, err := h.sendry.GetClient(server)
	if err != nil {
		return newServerDomainsEntry(nil, err)
	}
	resp, err := client.ListDomains(ctx)
	if err != nil {
		h.logger.Warn("failed to list server domains", "server", server, "error", err)
		return newServerDomainsEntry(nil, err)
	}
	return newServerDomainsEntry(resp.Domains, nil)
}

// domainSettings is the part of a domain's config that the panel deploys,
// in one shape for both sides of the comparison.
type domainSettings struct {
	Mode           string
	DefaultFrom    string
	RedirectTo     []string
	BCCTo          []string
	DKIM           bool
	DKIMSelector   string
	RateHour       int
	RateDay        int
	RateRecipients int
}

// panelDomainSettings mirrors deployDomainToServer: DKIM is sent only when a
// linked key exists, with that key's selector.
func (h *Handlers) panelDomainSettings(d *models.Domain) domainSettings {
	s := domainSettings{
		Mode:           modeOrDefault(d.Mode),
		DefaultFrom:    d.DefaultFrom,
		RedirectTo:     d.RedirectTo,
		BCCTo:          d.BCCTo,
		RateHour:       d.RateLimitHour,
		RateDay:        d.RateLimitDay,
		RateRecipients: d.RateLimitRecipients,
	}
	if d.DKIMEnabled && d.DKIMKeyID != "" {
		key := d.DKIMKey
		if key == nil {
			key, _ = h.dkim.GetByID(d.DKIMKeyID)
		}
		if key != nil {
			s.DKIM = true
			s.DKIMSelector = key.Selector
		}
	}
	return s
}

func serverDomainSettings(d *sendry.Domain) domainSettings {
	s := domainSettings{
		Mode:        modeOrDefault(d.Mode),
		DefaultFrom: d.DefaultFrom,
		RedirectTo:  d.RedirectTo,
		BCCTo:       d.BCCTo,
	}
	if d.DKIM != nil && d.DKIM.Enabled {
		s.DKIM = true
		s.DKIMSelector = d.DKIM.Selector
	}
	if d.RateLimit != nil {
		s.RateHour = d.RateLimit.MessagesPerHour
		s.RateDay = d.RateLimit.MessagesPerDay
		s.RateRecipients = d.RateLimit.RecipientsPerMessage
	}
	return s
}

func modeOrDefault(mode string) string {
	if mode == "" {
		return "production"
	}
	return mode
}

// diffSettings lists the settings that differ, as i18n key suffixes
// (infra.diff.<name>).
func diffSettings(want, got domainSettings) []string {
	var diff []string
	if want.Mode != got.Mode {
		diff = append(diff, "mode")
	}
	if want.DKIM != got.DKIM || want.DKIMSelector != got.DKIMSelector {
		diff = append(diff, "dkim")
	}
	if want.RateHour != got.RateHour || want.RateDay != got.RateDay || want.RateRecipients != got.RateRecipients {
		diff = append(diff, "rate_limit")
	}
	if want.DefaultFrom != got.DefaultFrom {
		diff = append(diff, "default_from")
	}
	if !slices.Equal(want.RedirectTo, got.RedirectTo) {
		diff = append(diff, "redirect_to")
	}
	if !slices.Equal(want.BCCTo, got.BCCTo) {
		diff = append(diff, "bcc_to")
	}
	return diff
}

// deploymentState is a domain's state on one server.
type deploymentState struct {
	Status    string
	Diff      []string
	CheckedAt time.Time
	Error     string
}

// NeedsSync reports whether a redeploy would fix the state.
func (s deploymentState) NeedsSync() bool {
	return s.Status == stateOutdated || s.Status == stateMissing || s.Status == stateDiffers
}

// deploymentStates computes the domain's state on every server it was
// deployed to, keyed by server name.
func (h *Handlers) deploymentStates(d *models.Domain, snap map[string]serverDomainsEntry) map[string]deploymentState {
	states := make(map[string]deploymentState, len(d.Deployments))
	hash := d.ConfigHash()
	var want *domainSettings
	for _, dep := range d.Deployments {
		e, checked := snap[dep.ServerName]
		st := deploymentState{CheckedAt: e.fetchedAt}
		switch {
		case dep.Status == "failed":
			st.Status = stateFailed
			st.Error = dep.Error
		case dep.Status == "outdated" || dep.ConfigHash != hash:
			st.Status = stateOutdated
		case !checked || e.err != nil:
			st.Status = stateUnknown
			if e.err != nil {
				st.Error = e.err.Error()
			}
		case e.domains[d.Domain] == nil:
			st.Status = stateMissing
		default:
			if want == nil {
				w := h.panelDomainSettings(d)
				want = &w
			}
			st.Diff = diffSettings(*want, serverDomainSettings(e.domains[d.Domain]))
			if len(st.Diff) > 0 {
				st.Status = stateDiffers
			} else {
				st.Status = stateSynced
			}
		}
		states[dep.ServerName] = st
	}
	return states
}

func deployedServers(d *models.Domain) []string {
	servers := make([]string, 0, len(d.Deployments))
	for _, dep := range d.Deployments {
		servers = append(servers, dep.ServerName)
	}
	return servers
}

// driftSummary is one badge per domain in the domain list, however many
// servers it is on: Synced of Total servers match the panel. Badge (colour)
// and Label (i18n key) name the worst server's state, so the badge says why
// the domain is not in sync, not only that it is not.
type driftSummary struct {
	Synced, Total int
	Badge         string
	Label         string
	Servers       []serverState // per server, by name, for the hover details
	NotDeployed   []string      // panel servers the domain is not deployed to
}

type serverState struct {
	Name  string
	State deploymentState
}

// driftRank orders states from fine to worst; ties keep the first seen.
var driftRank = map[string]struct {
	rank         int
	badge, label string
}{
	stateSynced:   {0, "success", "infra.sum.synced"},
	stateUnknown:  {1, "draft", "infra.sum.unknown"},
	stateOutdated: {2, "warning", "infra.sum.outdated"},
	stateDiffers:  {3, "failed", "infra.sum.drifted"},
	stateMissing:  {3, "failed", "infra.sum.drifted"},
	stateFailed:   {4, "failed", "infra.sum.failed"},
}

// summarizeStates builds the list badge. allServers are the servers the
// panel knows; those without a deployment are listed apart and do not count.
func summarizeStates(states map[string]deploymentState, allServers ...string) driftSummary {
	sum := driftSummary{Total: len(states)}
	worst := driftRank[stateSynced]
	for _, s := range states {
		if s.Status == stateSynced {
			sum.Synced++
		}
		if r, ok := driftRank[s.Status]; ok && r.rank > worst.rank {
			worst = r
		}
	}
	sum.Badge, sum.Label = worst.badge, worst.label
	for name, st := range states {
		sum.Servers = append(sum.Servers, serverState{Name: name, State: st})
	}
	slices.SortFunc(sum.Servers, func(a, b serverState) int { return strings.Compare(a.Name, b.Name) })
	for _, name := range allServers {
		if _, ok := states[name]; !ok {
			sum.NotDeployed = append(sum.NotDeployed, name)
		}
	}
	slices.Sort(sum.NotDeployed)
	return sum
}
