package dnsprovider

import (
	"context"
	"fmt"
)

// Record represents a generic DNS record used for comparison and upsert.
// ID is set only for existing records returned by the provider.
type Record struct {
	ID      string
	Type    string // e.g. TXT
	Name    string // FQDN without trailing dot
	Content string
	TTL     int // 0 or 1 means provider default/auto
}

// Zone represents a DNS zone.
type Zone struct {
	ID   string
	Name string // zone apex, e.g. example.com
}

// Provider is a minimal DNS provider abstraction.
// Implementations must be safe to call from a single goroutine.
type Provider interface {
	Name() string
	// ResolveZone finds a hosted zone that contains the given FQDN.
	ResolveZone(ctx context.Context, fqdn string) (*Zone, error)
	// ListRecords returns records of a given type for the exact name in zone.
	ListRecords(ctx context.Context, zoneID, name, recordType string) ([]Record, error)
	// CreateRecord creates a new DNS record in the given zone.
	CreateRecord(ctx context.Context, zoneID string, r Record) error
	// UpdateRecord updates an existing DNS record.
	UpdateRecord(ctx context.Context, zoneID, recordID string, r Record) error
}

// ErrZoneNotFound is returned when no matching zone is found.
var ErrZoneNotFound = fmt.Errorf("zone not found")
