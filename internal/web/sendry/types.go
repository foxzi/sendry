package sendry

import "time"

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status  string      `json:"status"`
	Version string      `json:"version"`
	Uptime  string      `json:"uptime"`
	Queue   *QueueStats `json:"queue,omitempty"`
}

// QueueStats represents queue statistics
type QueueStats struct {
	Pending   int `json:"pending"`
	Sent      int `json:"sent"`
	Failed    int `json:"failed"`
	Retrying  int `json:"retrying"`
	RateLimit int `json:"rate_limit"`
}

// SendRequest represents email send request
type SendRequest struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	CC      []string          `json:"cc,omitempty"`
	BCC     []string          `json:"bcc,omitempty"`
	Subject string            `json:"subject"`
	Body    string            `json:"body,omitempty"`
	HTML    string            `json:"html,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SendResponse represents send response
type SendResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// StatusResponse represents message status
type StatusResponse struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	From       string    `json:"from"`
	To         []string  `json:"to"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	RetryCount int       `json:"retry_count"`
	LastError  string    `json:"last_error,omitempty"`
}

// QueueResponse represents queue response
type QueueResponse struct {
	Stats    *QueueStats       `json:"stats"`
	Messages []*MessageSummary `json:"messages"`
}

// MessageSummary represents a message summary
type MessageSummary struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// DLQResponse represents DLQ response
type DLQResponse struct {
	Stats    *DLQStats         `json:"stats"`
	Messages []*MessageSummary `json:"messages"`
}

// DLQStats represents DLQ statistics
type DLQStats struct {
	Total int `json:"total"`
}

// DomainsListResponse represents domains list
type DomainsListResponse struct {
	Domains []*Domain `json:"domains"`
}

// Domain represents a domain configuration
type Domain struct {
	Domain      string        `json:"domain"`
	DKIM        *DKIMConfig   `json:"dkim,omitempty"`
	TLS         *TLSConfig    `json:"tls,omitempty"`
	RateLimit   *RateLimitCfg `json:"rate_limit,omitempty"`
	Mode        string        `json:"mode"`
	DefaultFrom string        `json:"default_from,omitempty"`
	RedirectTo  []string      `json:"redirect_to,omitempty"`
	BCCTo       []string      `json:"bcc_to,omitempty"`
}

// DKIMConfig represents DKIM configuration
type DKIMConfig struct {
	Enabled  bool   `json:"Enabled"`
	Selector string `json:"Selector"`
	KeyFile  string `json:"KeyFile,omitempty"`
}

// TLSConfig represents TLS configuration
type TLSConfig struct {
	Required bool `json:"required"`
}

// RateLimitCfg represents rate limit configuration
type RateLimitCfg struct {
	MessagesPerHour      int `json:"messages_per_hour"`
	MessagesPerDay       int `json:"messages_per_day"`
	RecipientsPerMessage int `json:"recipients_per_message"`
}

// Template represents a template
type Template struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Subject     string     `json:"subject"`
	HTML        string     `json:"html,omitempty"`
	Text        string     `json:"text,omitempty"`
	Variables   []Variable `json:"variables,omitempty"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Variable represents a template variable
type Variable struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
	Required    bool   `json:"required"`
}

// TemplateResponse is alias for Template
type TemplateResponse = Template

// TemplateCreateRequest represents template create request
type TemplateCreateRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Subject     string     `json:"subject"`
	HTML        string     `json:"html,omitempty"`
	Text        string     `json:"text,omitempty"`
	Variables   []Variable `json:"variables,omitempty"`
}

// TemplateUpdateRequest represents template update request
type TemplateUpdateRequest = TemplateCreateRequest

// DomainCreateRequest represents domain create/update request
type DomainCreateRequest struct {
	Domain      string        `json:"domain,omitempty"`
	DKIM        *DKIMConfig   `json:"dkim,omitempty"`
	TLS         *TLSConfig    `json:"tls,omitempty"`
	RateLimit   *RateLimitCfg `json:"rate_limit,omitempty"`
	Mode        string        `json:"mode,omitempty"`
	DefaultFrom string        `json:"default_from,omitempty"`
	RedirectTo  []string      `json:"redirect_to,omitempty"`
	BCCTo       []string      `json:"bcc_to,omitempty"`
}

// DomainUpdateRequest is alias for DomainCreateRequest
type DomainUpdateRequest = DomainCreateRequest

// DKIMUploadRequest represents DKIM upload request
type DKIMUploadRequest struct {
	Domain     string `json:"domain"`
	Selector   string `json:"selector"`
	PrivateKey string `json:"private_key"`
}

// DKIMResponse represents DKIM generation/upload response
type DKIMResponse struct {
	Domain    string `json:"domain"`
	Selector  string `json:"selector"`
	DNSName   string `json:"dns_name"`
	DNSRecord string `json:"dns_record"`
	KeyFile   string `json:"key_file"`
}

// DNSCheckResult represents DNS check result for a domain
type DNSCheckResult struct {
	Domain  string          `json:"domain"`
	Results []DNSCheckItem  `json:"results"`
	Summary DNSCheckSummary `json:"summary"`
}

// DNSCheckItem represents a single DNS check result
type DNSCheckItem struct {
	Type    string `json:"type"`
	Status  string `json:"status"` // ok, warning, error, not_found
	Value   string `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
}

// DNSCheckSummary represents DNS check summary
type DNSCheckSummary struct {
	OK       int `json:"ok"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
	NotFound int `json:"not_found"`
}

// IPCheckResult represents IP DNSBL check result
type IPCheckResult struct {
	IP      string         `json:"ip"`
	Results []DNSBLResult  `json:"results"`
	Summary IPCheckSummary `json:"summary"`
}

// DNSBLResult represents a single DNSBL check result
type DNSBLResult struct {
	DNSBL       DNSBLInfo `json:"dnsbl"`
	Listed      bool      `json:"listed"`
	ReturnCodes []string  `json:"return_codes,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// DNSBLInfo represents DNSBL service info
type DNSBLInfo struct {
	Name        string `json:"name"`
	Zone        string `json:"zone"`
	Description string `json:"description"`
}

// IPCheckSummary represents IP check summary
type IPCheckSummary struct {
	Clean  int `json:"clean"`
	Listed int `json:"listed"`
	Errors int `json:"errors"`
}
