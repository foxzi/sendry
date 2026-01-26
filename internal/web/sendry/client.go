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

// SendWithTemplate sends an email using a template
func (c *Client) SendWithTemplate(ctx context.Context, req *SendTemplateRequest) (*SendResponse, error) {
	var resp SendResponse
	if err := c.request(ctx, http.MethodPost, "/api/v1/send/template", req, &resp); err != nil {
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

// ListTemplates lists templates
func (c *Client) ListTemplates(ctx context.Context, search string, limit, offset int) (*TemplateListResponse, error) {
	path := "/api/v1/templates"
	params := url.Values{}
	if search != "" {
		params.Set("search", search)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp TemplateListResponse
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTemplate gets a template by ID
func (c *Client) GetTemplate(ctx context.Context, id string) (*TemplateResponse, error) {
	var resp TemplateResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/templates/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
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

// DeleteTemplate deletes a template
func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/templates/"+id, nil, nil)
}

// PreviewTemplate previews a template with data
func (c *Client) PreviewTemplate(ctx context.Context, id string, data map[string]any) (*TemplatePreviewResponse, error) {
	var resp TemplatePreviewResponse
	req := TemplatePreviewRequest{Data: data}
	if err := c.request(ctx, http.MethodPost, "/api/v1/templates/"+id+"/preview", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListSandboxMessages lists sandbox messages
func (c *Client) ListSandboxMessages(ctx context.Context, domain, mode string, limit, offset int) (*SandboxListResponse, error) {
	path := "/api/v1/sandbox/messages"
	params := url.Values{}
	if domain != "" {
		params.Set("domain", domain)
	}
	if mode != "" {
		params.Set("mode", mode)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp SandboxListResponse
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSandboxMessage gets a sandbox message
func (c *Client) GetSandboxMessage(ctx context.Context, id string) (*SandboxMessageDetailResponse, error) {
	var resp SandboxMessageDetailResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/sandbox/messages/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSandboxStats gets sandbox statistics
func (c *Client) GetSandboxStats(ctx context.Context) (*SandboxStatsResponse, error) {
	var resp SandboxStatsResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/sandbox/stats", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GenerateDKIM generates a new DKIM key on the server
func (c *Client) GenerateDKIM(ctx context.Context, domain, selector string) (*DKIMResponse, error) {
	req := &DKIMGenerateRequest{Domain: domain, Selector: selector}
	var resp DKIMResponse
	if err := c.request(ctx, http.MethodPost, "/api/v1/dkim/generate", req, &resp); err != nil {
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

// GetDKIM gets DKIM information for a domain
func (c *Client) GetDKIM(ctx context.Context, domain string) (*DKIMInfoResponse, error) {
	var resp DKIMInfoResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/dkim/"+domain, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// VerifyDKIM verifies DKIM setup for a domain
func (c *Client) VerifyDKIM(ctx context.Context, domain string) (*DKIMVerifyResponse, error) {
	var resp DKIMVerifyResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/dkim/"+domain+"/verify", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteDKIM deletes a DKIM key from the server
func (c *Client) DeleteDKIM(ctx context.Context, domain, selector string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/dkim/"+domain+"/"+selector, nil, nil)
}
