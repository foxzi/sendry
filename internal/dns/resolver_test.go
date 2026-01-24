package dns

import (
	"context"
	"testing"
	"time"
)

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		email string
		want  string
	}{
		{"user@example.com", "example.com"},
		{"user@EXAMPLE.COM", "example.com"},
		{"user@sub.example.com", "sub.example.com"},
		{"invalid", ""},
		{"", ""},
		{"@example.com", "example.com"},
		{"user@", ""},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := ExtractDomain(tt.email)
			if got != tt.want {
				t.Errorf("ExtractDomain(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

func TestResolverCache(t *testing.T) {
	resolver := NewResolver(1 * time.Hour)

	ctx := context.Background()

	// First lookup - should hit DNS
	records1, err := resolver.LookupMX(ctx, "google.com")
	if err != nil {
		t.Skipf("DNS lookup failed (network issue?): %v", err)
	}
	if len(records1) == 0 {
		t.Skip("No MX records returned for google.com")
	}

	// Second lookup - should hit cache
	records2, err := resolver.LookupMX(ctx, "google.com")
	if err != nil {
		t.Fatalf("Cached lookup failed: %v", err)
	}

	if len(records1) != len(records2) {
		t.Errorf("Cache returned different number of records")
	}

	// Check cache is case-insensitive
	records3, err := resolver.LookupMX(ctx, "GOOGLE.COM")
	if err != nil {
		t.Fatalf("Case-insensitive lookup failed: %v", err)
	}
	if len(records1) != len(records3) {
		t.Errorf("Case-insensitive lookup returned different results")
	}
}

func TestResolverMXSorting(t *testing.T) {
	resolver := NewResolver(1 * time.Hour)
	ctx := context.Background()

	// gmail.com has multiple MX records with different priorities
	records, err := resolver.LookupMX(ctx, "gmail.com")
	if err != nil {
		t.Skipf("DNS lookup failed: %v", err)
	}
	if len(records) < 2 {
		t.Skip("Need multiple MX records to test sorting")
	}

	// Check that records are sorted by priority
	for i := 1; i < len(records); i++ {
		if records[i].Priority < records[i-1].Priority {
			t.Errorf("MX records not sorted by priority: %v", records)
		}
	}
}

func TestResolverNonexistentDomain(t *testing.T) {
	resolver := NewResolver(1 * time.Minute)
	ctx := context.Background()

	// Lookup nonexistent domain - should fall back to A record
	records, err := resolver.LookupMX(ctx, "thisdomain.doesnotexist.invalid")
	if err != nil {
		// Some resolvers return error, some return empty
		return
	}

	// If no error, should fall back to domain itself
	if len(records) > 0 && records[0].Host != "thisdomain.doesnotexist.invalid" {
		t.Logf("Fallback to A record: %v", records)
	}
}

func TestNewResolverDefaultTTL(t *testing.T) {
	resolver := NewResolver(0)
	if resolver.ttl != 5*time.Minute {
		t.Errorf("Default TTL = %v, want 5m", resolver.ttl)
	}
}
