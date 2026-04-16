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

// CloudflareAuthMode selects the authentication scheme for the Cloudflare API.
type CloudflareAuthMode int

const (
	// CloudflareAuthToken uses a scoped API Token via the Authorization header.
	// This is the recommended mode.
	CloudflareAuthToken CloudflareAuthMode = iota
	// CloudflareAuthGlobalKey uses the legacy Global API Key (email + key) via
	// X-Auth-Email and X-Auth-Key headers.
	CloudflareAuthGlobalKey
)

// CloudflareProvider implements Provider using the Cloudflare v4 API.
// It supports both scoped API Tokens and legacy Global API Keys.
type CloudflareProvider struct {
	AuthMode CloudflareAuthMode
	Token    string // API Token (when AuthMode == CloudflareAuthToken)
	Email    string // Account email (when AuthMode == CloudflareAuthGlobalKey)
	APIKey   string // Global API Key (when AuthMode == CloudflareAuthGlobalKey)
	BaseURL  string
	Client   *http.Client
}

// NewCloudflare creates a provider using a scoped API Token.
// Token must have permissions: Zone:Read, DNS:Edit for the target zones.
func NewCloudflare(token string) *CloudflareProvider {
	return &CloudflareProvider{
		AuthMode: CloudflareAuthToken,
		Token:    token,
		BaseURL:  "https://api.cloudflare.com/client/v4",
		Client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// NewCloudflareGlobalKey creates a provider using the legacy Global API Key
// (account email + global key). This key has full access to the whole
// Cloudflare account, so API Tokens with scoped permissions are preferred.
func NewCloudflareGlobalKey(email, apiKey string) *CloudflareProvider {
	return &CloudflareProvider{
		AuthMode: CloudflareAuthGlobalKey,
		Email:    email,
		APIKey:   apiKey,
		BaseURL:  "https://api.cloudflare.com/client/v4",
		Client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *CloudflareProvider) Name() string { return "cloudflare" }

type cfResponse struct {
	Success  bool              `json:"success"`
	Errors   []json.RawMessage `json:"errors"`
	Messages []json.RawMessage `json:"messages"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

func (p *CloudflareProvider) setAuth(req *http.Request) {
	switch p.AuthMode {
	case CloudflareAuthGlobalKey:
		req.Header.Set("X-Auth-Email", p.Email)
		req.Header.Set("X-Auth-Key", p.APIKey)
	default:
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
}

func (p *CloudflareProvider) do(ctx context.Context, method, path string, body any, out any) error {
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
	p.setAuth(req)
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

	var envelope cfResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("parse response (%d): %w: %s", resp.StatusCode, err, truncate(string(raw), 400))
	}
	if !envelope.Success {
		return fmt.Errorf("cloudflare api error (%d): %s", resp.StatusCode, truncate(string(raw), 400))
	}

	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ResolveZone walks the FQDN parent labels to locate the hosted zone.
func (p *CloudflareProvider) ResolveZone(ctx context.Context, fqdn string) (*Zone, error) {
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

func (p *CloudflareProvider) lookupZone(ctx context.Context, name string) (*Zone, error) {
	var result struct {
		cfResponse
		Result []cfZone `json:"result"`
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("per_page", "1")
	if err := p.do(ctx, http.MethodGet, "/zones?"+q.Encode(), nil, &result); err != nil {
		return nil, err
	}
	if len(result.Result) == 0 {
		return nil, nil
	}
	return &Zone{ID: result.Result[0].ID, Name: result.Result[0].Name}, nil
}

// ListRecords returns records matching the exact name and type in the zone.
func (p *CloudflareProvider) ListRecords(ctx context.Context, zoneID, name, recordType string) ([]Record, error) {
	var result struct {
		cfResponse
		Result []cfRecord `json:"result"`
	}
	q := url.Values{}
	q.Set("type", recordType)
	q.Set("name", strings.TrimSuffix(strings.ToLower(name), "."))
	q.Set("per_page", "100")
	if err := p.do(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?"+q.Encode(), nil, &result); err != nil {
		return nil, err
	}

	records := make([]Record, 0, len(result.Result))
	for _, r := range result.Result {
		records = append(records, Record{
			ID:      r.ID,
			Type:    r.Type,
			Name:    r.Name,
			Content: r.Content,
			TTL:     r.TTL,
		})
	}
	return records, nil
}

// CreateRecord creates a DNS record.
func (p *CloudflareProvider) CreateRecord(ctx context.Context, zoneID string, r Record) error {
	payload := cfRecord{
		Type:    r.Type,
		Name:    r.Name,
		Content: r.Content,
		TTL:     ttlOrAuto(r.TTL),
	}
	return p.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", payload, nil)
}

// UpdateRecord updates an existing DNS record.
func (p *CloudflareProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r Record) error {
	payload := cfRecord{
		Type:    r.Type,
		Name:    r.Name,
		Content: r.Content,
		TTL:     ttlOrAuto(r.TTL),
	}
	return p.do(ctx, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+recordID, payload, nil)
}

func ttlOrAuto(t int) int {
	if t <= 0 {
		return 1 // Cloudflare's "auto"
	}
	return t
}
