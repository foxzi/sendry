package handlers

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/foxzi/sendry/internal/web/dnsprovider"
)

// dnsHosting tells who serves a domain's DNS: the public NS records and
// whether the zone exists in the DNS provider accounts the panel knows.
type dnsHosting struct {
	NS      []string // name servers, sorted, without the trailing dot
	NSZone  string   // name the NS records were found at (the domain or a parent)
	NSError string

	Providers []providerZone // one per configured provider, in config order
}

type providerZone struct {
	Provider string // display name, e.g. Cloudflare
	Zone     string // zone name when found
	Found    bool
	Error    string
}

// nsLookupFunc matches net.Resolver.LookupNS; tests replace it.
type nsLookupFunc func(ctx context.Context, name string) ([]*net.NS, error)

var providerDisplayNames = map[string]string{
	"cloudflare": "Cloudflare",
	"namedot":    "namedot",
}

// checkDNSHosting looks up NS records and asks every provider for the zone,
// all in parallel. Failures are reported in the result, never returned.
func checkDNSHosting(ctx context.Context, domain string, providers []dnsprovider.Provider, lookupNS nsLookupFunc) *dnsHosting {
	res := &dnsHosting{Providers: make([]providerZone, len(providers))}
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		res.NSZone, res.NS, res.NSError = findNS(ctx, domain, lookupNS)
	}()

	for i, p := range providers {
		wg.Add(1)
		go func(i int, p dnsprovider.Provider) {
			defer wg.Done()
			pz := providerZone{Provider: providerDisplayNames[p.Name()]}
			if pz.Provider == "" {
				pz.Provider = p.Name()
			}
			zone, err := p.ResolveZone(ctx, domain)
			switch {
			case errors.Is(err, dnsprovider.ErrZoneNotFound):
			case err != nil:
				pz.Error = err.Error()
			case zone != nil:
				pz.Found, pz.Zone = true, zone.Name
			}
			res.Providers[i] = pz
		}(i, p)
	}

	wg.Wait()
	return res
}

// findNS returns the NS records of the domain or, for a subdomain without
// its own delegation, of the nearest parent that has them.
func findNS(ctx context.Context, domain string, lookupNS nsLookupFunc) (zone string, hosts []string, errMsg string) {
	labels := strings.Split(domain, ".")
	for i := 0; i < len(labels)-1; i++ {
		name := strings.Join(labels[i:], ".")
		records, err := lookupNS(ctx, name)
		if err != nil {
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
				continue
			}
			return "", nil, err.Error()
		}
		if len(records) == 0 {
			continue
		}
		for _, ns := range records {
			hosts = append(hosts, strings.TrimSuffix(strings.ToLower(ns.Host), "."))
		}
		sort.Strings(hosts)
		return name, hosts, ""
	}
	return "", nil, "no NS records found"
}
