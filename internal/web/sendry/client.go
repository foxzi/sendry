package sendry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a Sendry API client
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Sendry API client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// request performs an HTTP request to the Sendry API
func (c *Client) request(ctx context.Context, method, path string, body any, result any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("API error: %s", errResp.Error)
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// Health checks server health
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.request(ctx, http.MethodGet, "/health", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Send sends an email
func (c *Client) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	var resp SendResponse
	if err := c.request(ctx, http.MethodPost, "/api/v1/send", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetStatus gets message status
func (c *Client) GetStatus(ctx context.Context, id string) (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/status/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetQueue gets queue status
func (c *Client) GetQueue(ctx context.Context) (*QueueResponse, error) {
	var resp QueueResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/queue", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteFromQueue deletes a message from queue
func (c *Client) DeleteFromQueue(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/queue/"+id, nil, nil)
}

// PurgeQueue deletes all messages from queue
func (c *Client) PurgeQueue(ctx context.Context) (int, error) {
	queue, err := c.GetQueue(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, m := range queue.Messages {
		if err := c.DeleteFromQueue(ctx, m.ID); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// GetDLQ gets dead letter queue
func (c *Client) GetDLQ(ctx context.Context) (*DLQResponse, error) {
	var resp DLQResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/dlq", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RetryDLQ retries a message from DLQ
func (c *Client) RetryDLQ(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodPost, "/api/v1/dlq/"+id+"/retry", nil, nil)
}

// DeleteFromDLQ deletes a message from DLQ
func (c *Client) DeleteFromDLQ(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/dlq/"+id, nil, nil)
}

// PurgeDLQ deletes all messages from DLQ
func (c *Client) PurgeDLQ(ctx context.Context) (int, error) {
	dlq, err := c.GetDLQ(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, m := range dlq.Messages {
		if err := c.DeleteFromDLQ(ctx, m.ID); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// ListDomains lists all domains
func (c *Client) ListDomains(ctx context.Context) (*DomainsListResponse, error) {
	var resp DomainsListResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/domains", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetDomain gets a domain by name
func (c *Client) GetDomain(ctx context.Context, domain string) (*Domain, error) {
	var resp Domain
	if err := c.request(ctx, http.MethodGet, "/api/v1/domains/"+domain, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateDomain creates a new domain
func (c *Client) CreateDomain(ctx context.Context, req *DomainCreateRequest) (*Domain, error) {
	var resp Domain
	if err := c.request(ctx, http.MethodPost, "/api/v1/domains", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateDomain updates an existing domain
func (c *Client) UpdateDomain(ctx context.Context, domain string, req *DomainUpdateRequest) (*Domain, error) {
	var resp Domain
	if err := c.request(ctx, http.MethodPut, "/api/v1/domains/"+domain, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteDomain deletes a domain
func (c *Client) DeleteDomain(ctx context.Context, domain string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/domains/"+domain, nil, nil)
}

// CreateTemplate creates a new template
func (c *Client) CreateTemplate(ctx context.Context, req *TemplateCreateRequest) (*TemplateResponse, error) {
	var resp TemplateResponse
	if err := c.request(ctx, http.MethodPost, "/api/v1/templates", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateTemplate updates a template
func (c *Client) UpdateTemplate(ctx context.Context, id string, req *TemplateUpdateRequest) (*TemplateResponse, error) {
	var resp TemplateResponse
	if err := c.request(ctx, http.MethodPut, "/api/v1/templates/"+id, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UploadDKIM uploads an existing DKIM key to the server
func (c *Client) UploadDKIM(ctx context.Context, domain, selector, privateKey string) (*DKIMResponse, error) {
	req := &DKIMUploadRequest{Domain: domain, Selector: selector, PrivateKey: privateKey}
	var resp DKIMResponse
	if err := c.request(ctx, http.MethodPost, "/api/v1/dkim/upload", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteDKIM deletes a DKIM key from the server
func (c *Client) DeleteDKIM(ctx context.Context, domain, selector string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/dkim/"+domain+"/"+selector, nil, nil)
}

// CheckDNS checks DNS records for a domain
func (c *Client) CheckDNS(ctx context.Context, domain, selector string) (*DNSCheckResult, error) {
	path := "/api/v1/dns/check/" + url.PathEscape(domain)
	if selector != "" {
		path += "?selector=" + url.QueryEscape(selector)
	}
	var resp DNSCheckResult
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CheckIP checks an IP address against DNSBL services
func (c *Client) CheckIP(ctx context.Context, ip string) (*IPCheckResult, error) {
	var resp IPCheckResult
	if err := c.request(ctx, http.MethodGet, "/api/v1/ip/check/"+url.PathEscape(ip), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
