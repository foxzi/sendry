package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NamedotProvider implements Provider using the namedot REST API
// (github.com/foxzi/namedot).
//
// Notes on the namedot API model:
//   - Records are grouped into RRSets (name + type). An RRSet has an ID.
//   - An RRSet can contain multiple Record entries (e.g. multiple TXT values).
//   - Updating a record means PUTting a new RRSet representation for a given ID.
//
// The provider abstraction in this package treats each individual record as
// independent (ID == rrset ID). For TXT-based records this is acceptable
// because SPF/DMARC/DKIM names normally have exactly one value; when multiple
// values exist, the provider replaces the entire RRSet with the new value
// during UpdateRecord. This matches how the Cloudflare provider behaves for
// the same use case.
type NamedotProvider struct {
	Token   string
	BaseURL string
	Client  *http.Client
}

// NewNamedot creates a new namedot provider using the given bearer token and
// base URL (for example, "https://dns.example.com").
func NewNamedot(baseURL, token string) *NamedotProvider {
	return &NamedotProvider{
		Token:   token,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *NamedotProvider) Name() string { return "namedot" }

type namedotZone struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

type namedotRecord struct {
	Data string `json:"data"`
}

type namedotRRSet struct {
	ID      uint64          `json:"id,omitempty"`
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	TTL     int             `json:"ttl,omitempty"`
	Records []namedotRecord `json:"records"`
}

func (p *NamedotProvider) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		// Let caller handle (e.g. treat as "no zone"/"no rrset").
		return errNotFound{body: truncate(string(raw), 400)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("namedot api error (%d): %s", resp.StatusCode, truncate(string(raw), 400))
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("parse response: %w: %s", err, truncate(string(raw), 400))
		}
	}
	return nil
}

type errNotFound struct{ body string }

func (e errNotFound) Error() string { return "namedot: not found: " + e.body }

// ResolveZone walks FQDN parent labels to find the hosted zone in namedot.
func (p *NamedotProvider) ResolveZone(ctx context.Context, fqdn string) (*Zone, error) {
	fqdn = strings.TrimSuffix(strings.ToLower(fqdn), ".")
	labels := strings.Split(fqdn, ".")

	for i := 0; i < len(labels)-1; i++ {
		candidate := strings.Join(labels[i:], ".")
		z, err := p.lookupZone(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if z != nil {
			return z, nil
		}
	}
	return nil, ErrZoneNotFound
}

func (p *NamedotProvider) lookupZone(ctx context.Context, name string) (*Zone, error) {
	q := url.Values{}
	q.Set("name", name)

	// namedot may return either a single object (exact match by name) or an
	// array (list with filter). Accept both.
	var one namedotZone
	err := p.do(ctx, http.MethodGet, "/zones?"+q.Encode(), nil, &one)
	if err == nil && one.ID != 0 {
		return &Zone{ID: fmt.Sprintf("%d", one.ID), Name: strings.TrimSuffix(one.Name, ".")}, nil
	}
	if _, ok := err.(errNotFound); ok {
		return nil, nil
	}
	// Fall back to list shape.
	var list []namedotZone
	err2 := p.do(ctx, http.MethodGet, "/zones?"+q.Encode(), nil, &list)
	if err2 != nil {
		if _, ok := err2.(errNotFound); ok {
			return nil, nil
		}
		// Prefer the first parse error if both failed.
		if err != nil {
			return nil, err
		}
		return nil, err2
	}
	for _, z := range list {
		if strings.EqualFold(strings.TrimSuffix(z.Name, "."), name) {
			return &Zone{ID: fmt.Sprintf("%d", z.ID), Name: strings.TrimSuffix(z.Name, ".")}, nil
		}
	}
	return nil, nil
}

// ListRecords fetches all RRSets in a zone and returns records matching the
// requested name and type. Each returned Record ID is the namedot RRSet ID.
func (p *NamedotProvider) ListRecords(ctx context.Context, zoneID, name, recordType string) ([]Record, error) {
	var rrsets []namedotRRSet
	if err := p.do(ctx, http.MethodGet, "/zones/"+zoneID+"/rrsets", nil, &rrsets); err != nil {
		if _, ok := err.(errNotFound); ok {
			return nil, nil
		}
		return nil, err
	}

	targetName := strings.TrimSuffix(strings.ToLower(name), ".")
	out := make([]Record, 0)
	for _, rs := range rrsets {
		if !strings.EqualFold(rs.Type, recordType) {
			continue
		}
		rsName := strings.TrimSuffix(strings.ToLower(rs.Name), ".")
		if rsName != targetName {
			continue
		}
		for _, rec := range rs.Records {
			out = append(out, Record{
				ID:      fmt.Sprintf("%d", rs.ID),
				Type:    rs.Type,
				Name:    rsName,
				Content: unquoteTXT(rec.Data, rs.Type),
				TTL:     rs.TTL,
			})
		}
	}
	return out, nil
}

// CreateRecord creates a new RRSet with a single record.
func (p *NamedotProvider) CreateRecord(ctx context.Context, zoneID string, r Record) error {
	rs := namedotRRSet{
		Name:    strings.TrimSuffix(strings.ToLower(r.Name), "."),
		Type:    r.Type,
		TTL:     r.TTL,
		Records: []namedotRecord{{Data: quoteTXT(r.Content, r.Type)}},
	}
	return p.do(ctx, http.MethodPost, "/zones/"+zoneID+"/rrsets", rs, nil)
}

// UpdateRecord replaces the RRSet identified by recordID with a single record
// containing the new value.
func (p *NamedotProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r Record) error {
	rs := namedotRRSet{
		Name:    strings.TrimSuffix(strings.ToLower(r.Name), "."),
		Type:    r.Type,
		TTL:     r.TTL,
		Records: []namedotRecord{{Data: quoteTXT(r.Content, r.Type)}},
	}
	return p.do(ctx, http.MethodPut, "/zones/"+zoneID+"/rrsets/"+recordID, rs, nil)
}

// quoteTXT wraps TXT content in quotes as required by namedot's BIND-style
// storage. Values that already start with a quote are returned as-is.
func quoteTXT(data, recordType string) string {
	if !strings.EqualFold(recordType, "TXT") {
		return data
	}
	if strings.HasPrefix(data, "\"") {
		return data
	}
	// Escape any embedded quotes/backslashes so the string round-trips.
	escaped := strings.ReplaceAll(data, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}

// unquoteTXT strips surrounding quotes on TXT values so that comparisons in
// the sync layer (which also normalizes) work naturally.
func unquoteTXT(data, recordType string) string {
	if !strings.EqualFold(recordType, "TXT") {
		return data
	}
	s := strings.TrimSpace(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		s = strings.ReplaceAll(s, "\\\"", "\"")
		s = strings.ReplaceAll(s, "\\\\", "\\")
	}
	return s
}
