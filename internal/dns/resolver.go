package dns

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// MXRecord represents an MX record
type MXRecord struct {
	Host     string
	Priority uint16
}

// Resolver performs DNS lookups for MX records with caching
type Resolver struct {
	cache map[string]cacheEntry
	ttl   time.Duration
	mu    sync.RWMutex
}

type cacheEntry struct {
	records   []MXRecord
	expiresAt time.Time
}

// NewResolver creates a new DNS resolver
func NewResolver(cacheTTL time.Duration) *Resolver {
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}
	return &Resolver{
		cache: make(map[string]cacheEntry),
		ttl:   cacheTTL,
	}
}

// LookupMX returns MX records sorted by priority
func (r *Resolver) LookupMX(ctx context.Context, domain string) ([]MXRecord, error) {
	domain = strings.ToLower(domain)

	// Check cache
	r.mu.RLock()
	entry, ok := r.cache[domain]
	r.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.records, nil
	}

	// Perform DNS lookup
	mxRecords, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil {
		// If no MX records, fall back to A record (domain itself)
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return []MXRecord{{Host: domain, Priority: 0}}, nil
		}
		return nil, err
	}

	// Convert to our format and sort by priority
	records := make([]MXRecord, len(mxRecords))
	for i, mx := range mxRecords {
		records[i] = MXRecord{
			Host:     strings.TrimSuffix(mx.Host, "."),
			Priority: mx.Pref,
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Priority < records[j].Priority
	})

	// Update cache
	r.mu.Lock()
	r.cache[domain] = cacheEntry{
		records:   records,
		expiresAt: time.Now().Add(r.ttl),
	}
	r.mu.Unlock()

	return records, nil
}

// ExtractDomain extracts the domain part from an email address
func ExtractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}
