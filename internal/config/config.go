package config

import (
	"fmt"
	"os"
	"time"

	"github.com/foxzi/sendry/internal/headers"
	"gopkg.in/yaml.v3"
)

// Config is the main configuration structure
type Config struct {
	Server      ServerConfig            `yaml:"server"`
	SMTP        SMTPConfig              `yaml:"smtp"`
	API         APIConfig               `yaml:"api"`
	Queue       QueueConfig             `yaml:"queue"`
	Storage     StorageConfig           `yaml:"storage"`
	Logging     LoggingConfig           `yaml:"logging"`
	DKIM        DKIMConfig              `yaml:"dkim"`         // Legacy single-domain DKIM config
	Domains     map[string]DomainConfig `yaml:"domains"`      // Multi-domain configuration
	RateLimit   RateLimitConfig         `yaml:"rate_limit"`   // Rate limiting configuration
	HeaderRules *headers.Config         `yaml:"header_rules"` // Header manipulation rules
	Metrics     MetricsConfig           `yaml:"metrics"`      // Prometheus metrics configuration
	DLQ         DLQConfig               `yaml:"dlq"`          // Dead Letter Queue configuration

	// Internal: path to dynamic domains config file (not in YAML)
	domainsFile string `yaml:"-"`
}

// MetricsConfig contains Prometheus metrics settings
type MetricsConfig struct {
	Enabled       bool          `yaml:"enabled"`
	ListenAddr    string        `yaml:"listen_addr"`    // Default: :9090
	Path          string        `yaml:"path"`           // Default: /metrics
	FlushInterval time.Duration `yaml:"flush_interval"` // Default: 10s
	AllowedIPs    []string      `yaml:"allowed_ips"`    // IP addresses/CIDRs allowed to access metrics
}

// DLQConfig contains Dead Letter Queue settings
type DLQConfig struct {
	Enabled         bool          `yaml:"enabled"`          // Enable DLQ (false = delete failed messages)
	MaxAge          time.Duration `yaml:"max_age"`          // Delete DLQ messages older than this (0 = keep forever)
	MaxCount        int           `yaml:"max_count"`        // Max messages in DLQ (0 = unlimited)
	CleanupInterval time.Duration `yaml:"cleanup_interval"` // How often to run DLQ cleanup
}

// RateLimitConfig contains global rate limiting settings
type RateLimitConfig struct {
	Enabled bool `yaml:"enabled"`

	// Global limits (for entire server)
	Global *LimitValues `yaml:"global,omitempty"`

	// Default limits for domains
	DefaultDomain *LimitValues `yaml:"default_domain,omitempty"`

	// Default limits for senders
	DefaultSender *LimitValues `yaml:"default_sender,omitempty"`

	// Default limits for IPs
	DefaultIP *LimitValues `yaml:"default_ip,omitempty"`

	// Default limits for API keys
	DefaultAPIKey *LimitValues `yaml:"default_api_key,omitempty"`

	// Default limits for recipient domains (e.g., gmail.com, mail.ru)
	DefaultRecipientDomain *LimitValues `yaml:"default_recipient_domain,omitempty"`

	// Per-recipient-domain limits (overrides DefaultRecipientDomain)
	RecipientDomains map[string]*LimitValues `yaml:"recipient_domains,omitempty"`
}

// LimitValues contains rate limit values
type LimitValues struct {
	MessagesPerHour int `yaml:"messages_per_hour"`
	MessagesPerDay  int `yaml:"messages_per_day"`
}

// DomainConfig contains per-domain settings
type DomainConfig struct {
	// DKIM settings for this domain
	DKIM *DomainDKIMConfig `yaml:"dkim,omitempty"`

	// TLS settings for this domain (for SNI)
	TLS *DomainTLSConfig `yaml:"tls,omitempty"`

	// Rate limiting for this domain
	RateLimit *DomainRateLimitConfig `yaml:"rate_limit,omitempty"`

	// Default From address for this domain
	DefaultFrom string `yaml:"default_from,omitempty"`

	// Domain mode: production, sandbox, redirect, bcc
	Mode string `yaml:"mode,omitempty"`

	// Redirect settings (when mode=redirect)
	RedirectTo []string `yaml:"redirect_to,omitempty"`

	// BCC settings (when mode=bcc)
	BCCTo []string `yaml:"bcc_to,omitempty"`
}

// DomainDKIMConfig contains DKIM settings for a domain
type DomainDKIMConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Selector string `yaml:"selector"`
	KeyFile  string `yaml:"key_file"`
}

// DomainTLSConfig contains TLS settings for a domain
type DomainTLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// DomainRateLimitConfig contains rate limit settings for a domain
type DomainRateLimitConfig struct {
	MessagesPerHour      int `yaml:"messages_per_hour"`
	MessagesPerDay       int `yaml:"messages_per_day"`
	RecipientsPerMessage int `yaml:"recipients_per_message"`
}

// ServerConfig contains server-wide settings
type ServerConfig struct {
	Hostname string `yaml:"hostname"` // FQDN of the server
}

// SMTPConfig contains SMTP server settings
type SMTPConfig struct {
	ListenAddr      string        `yaml:"listen_addr"`
	SubmissionAddr  string        `yaml:"submission_addr"`
	SMTPSAddr       string        `yaml:"smtps_addr"`
	Domain          string        `yaml:"domain"`
	MaxMessageBytes int           `yaml:"max_message_bytes"`
	MaxRecipients   int           `yaml:"max_recipients"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	Auth            AuthConfig    `yaml:"auth"`
	TLS             TLSConfig     `yaml:"tls"`
	AllowedIPs      []string      `yaml:"allowed_ips"` // IP addresses/CIDRs allowed to connect (empty = allow all)
}

// TLSConfig contains TLS certificate settings
type TLSConfig struct {
	CertFile string     `yaml:"cert_file"`
	KeyFile  string     `yaml:"key_file"`
	ACME     ACMEConfig `yaml:"acme"`
}

// ACMEConfig contains Let's Encrypt ACME settings
type ACMEConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Email    string   `yaml:"email"`
	Domains  []string `yaml:"domains"`
	CacheDir string   `yaml:"cache_dir"`
	OnDemand bool     `yaml:"on_demand"` // If true, port 80 is not opened; use 'sendry tls renew' instead
}

// DKIMConfig contains DKIM signing settings
type DKIMConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Selector string `yaml:"selector"`
	KeyFile  string `yaml:"key_file"`
	Domain   string `yaml:"domain"`
}

// AuthConfig contains SMTP authentication settings
type AuthConfig struct {
	Required bool              `yaml:"required"`
	Users    map[string]string `yaml:"users"` // username -> password

	// Brute force protection settings
	MaxFailures   int           `yaml:"max_failures"`   // Max auth failures before blocking (default: 5)
	BlockDuration time.Duration `yaml:"block_duration"` // How long to block after max failures (default: 15m)
	FailureWindow time.Duration `yaml:"failure_window"` // Window for counting failures (default: 5m)
}

// APIConfig contains HTTP API settings
type APIConfig struct {
	ListenAddr     string        `yaml:"listen_addr"`
	APIKey         string        `yaml:"api_key"`
	MaxHeaderBytes int           `yaml:"max_header_bytes"` // Max HTTP header size (default: 1MB)
	ReadTimeout    time.Duration `yaml:"read_timeout"`     // HTTP read timeout (default: 30s)
	WriteTimeout   time.Duration `yaml:"write_timeout"`    // HTTP write timeout (default: 30s)
	IdleTimeout    time.Duration `yaml:"idle_timeout"`     // HTTP idle timeout (default: 60s)
	AllowedIPs     []string      `yaml:"allowed_ips"`      // IP addresses/CIDRs allowed to access API (empty = allow all)
}

// QueueConfig contains queue processor settings
type QueueConfig struct {
	Workers         int           `yaml:"workers"`
	RetryInterval   time.Duration `yaml:"retry_interval"`
	MaxRetries      int           `yaml:"max_retries"`
	ProcessInterval time.Duration `yaml:"process_interval"`
}

// StorageConfig contains storage settings
type StorageConfig struct {
	Path      string           `yaml:"path"`
	Retention *RetentionConfig `yaml:"retention"` // Message retention settings
}

// RetentionConfig contains message retention settings
type RetentionConfig struct {
	DeliveredMaxAge time.Duration `yaml:"delivered_max_age"` // Delete delivered messages older than this (0 = keep forever)
	CleanupInterval time.Duration `yaml:"cleanup_interval"`  // How often to run cleanup
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, text
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.setDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// setDefaults sets default values for configuration
func (c *Config) setDefaults() {
	if c.Server.Hostname == "" {
		hostname, _ := os.Hostname()
		c.Server.Hostname = hostname
	}

	if c.SMTP.ListenAddr == "" {
		c.SMTP.ListenAddr = ":25"
	}
	if c.SMTP.SubmissionAddr == "" {
		c.SMTP.SubmissionAddr = ":587"
	}
	if c.SMTP.SMTPSAddr == "" {
		c.SMTP.SMTPSAddr = ":465"
	}
	if c.SMTP.TLS.ACME.CacheDir == "" {
		c.SMTP.TLS.ACME.CacheDir = "/var/lib/sendry/certs"
	}
	if c.SMTP.MaxMessageBytes == 0 {
		c.SMTP.MaxMessageBytes = 10 * 1024 * 1024 // 10MB
	}
	if c.SMTP.MaxRecipients == 0 {
		c.SMTP.MaxRecipients = 100
	}
	if c.SMTP.ReadTimeout == 0 {
		c.SMTP.ReadTimeout = 60 * time.Second
	}
	if c.SMTP.WriteTimeout == 0 {
		c.SMTP.WriteTimeout = 60 * time.Second
	}

	// Auth brute force protection defaults
	if c.SMTP.Auth.MaxFailures == 0 {
		c.SMTP.Auth.MaxFailures = 5
	}
	if c.SMTP.Auth.BlockDuration == 0 {
		c.SMTP.Auth.BlockDuration = 15 * time.Minute
	}
	if c.SMTP.Auth.FailureWindow == 0 {
		c.SMTP.Auth.FailureWindow = 5 * time.Minute
	}

	if c.API.ListenAddr == "" {
		c.API.ListenAddr = ":8080"
	}
	if c.API.MaxHeaderBytes == 0 {
		c.API.MaxHeaderBytes = 1 << 20 // 1 MB
	}
	if c.API.ReadTimeout == 0 {
		c.API.ReadTimeout = 30 * time.Second
	}
	if c.API.WriteTimeout == 0 {
		c.API.WriteTimeout = 30 * time.Second
	}
	if c.API.IdleTimeout == 0 {
		c.API.IdleTimeout = 60 * time.Second
	}

	if c.Queue.Workers == 0 {
		c.Queue.Workers = 4
	}
	if c.Queue.RetryInterval == 0 {
		c.Queue.RetryInterval = 5 * time.Minute
	}
	if c.Queue.MaxRetries == 0 {
		c.Queue.MaxRetries = 5
	}
	if c.Queue.ProcessInterval == 0 {
		c.Queue.ProcessInterval = 10 * time.Second
	}

	if c.Storage.Path == "" {
		c.Storage.Path = "/var/lib/sendry/queue.db"
	}

	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}

	// Metrics defaults
	if c.Metrics.ListenAddr == "" {
		c.Metrics.ListenAddr = ":9090"
	}
	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}
	if c.Metrics.FlushInterval == 0 {
		c.Metrics.FlushInterval = 10 * time.Second
	}

	// DLQ defaults
	if !c.DLQ.Enabled && c.DLQ.MaxAge == 0 && c.DLQ.MaxCount == 0 && c.DLQ.CleanupInterval == 0 {
		// If nothing is set, enable DLQ by default
		c.DLQ.Enabled = true
	}
	if c.DLQ.CleanupInterval == 0 {
		c.DLQ.CleanupInterval = time.Hour
	}

	// Retention defaults
	if c.Storage.Retention == nil {
		c.Storage.Retention = &RetentionConfig{}
	}
	if c.Storage.Retention.CleanupInterval == 0 {
		c.Storage.Retention.CleanupInterval = time.Hour
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.SMTP.Domain == "" {
		return fmt.Errorf("smtp.domain is required")
	}

	if c.SMTP.Auth.Required && len(c.SMTP.Auth.Users) == 0 {
		return fmt.Errorf("smtp.auth.users must not be empty when auth is required")
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid logging.level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	validLogFormats := map[string]bool{"json": true, "text": true}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid logging.format: %s (must be json or text)", c.Logging.Format)
	}

	// Validate TLS configuration
	if err := c.validateTLS(); err != nil {
		return err
	}

	// Validate DKIM configuration
	if err := c.validateDKIM(); err != nil {
		return err
	}

	// Validate multi-domain configuration
	if err := c.validateDomains(); err != nil {
		return err
	}

	return nil
}

// validateTLS validates TLS configuration
func (c *Config) validateTLS() error {
	tls := c.SMTP.TLS
	hasCerts := tls.CertFile != "" && tls.KeyFile != ""
	hasACME := tls.ACME.Enabled

	if hasCerts && hasACME {
		return fmt.Errorf("cannot use both manual certificates and ACME")
	}

	if hasCerts {
		if tls.CertFile == "" {
			return fmt.Errorf("smtp.tls.cert_file is required when using manual certificates")
		}
		if tls.KeyFile == "" {
			return fmt.Errorf("smtp.tls.key_file is required when using manual certificates")
		}
	}

	if hasACME {
		if tls.ACME.Email == "" {
			return fmt.Errorf("smtp.tls.acme.email is required when ACME is enabled")
		}
		if len(tls.ACME.Domains) == 0 {
			return fmt.Errorf("smtp.tls.acme.domains must not be empty when ACME is enabled")
		}
	}

	return nil
}

// validateDKIM validates DKIM configuration
func (c *Config) validateDKIM() error {
	if !c.DKIM.Enabled {
		return nil
	}

	if c.DKIM.Selector == "" {
		return fmt.Errorf("dkim.selector is required when DKIM is enabled")
	}
	if c.DKIM.KeyFile == "" {
		return fmt.Errorf("dkim.key_file is required when DKIM is enabled")
	}
	if c.DKIM.Domain == "" {
		return fmt.Errorf("dkim.domain is required when DKIM is enabled")
	}

	return nil
}

// HasTLS returns true if TLS is configured
func (c *Config) HasTLS() bool {
	return (c.SMTP.TLS.CertFile != "" && c.SMTP.TLS.KeyFile != "") || c.SMTP.TLS.ACME.Enabled
}

// GetDomainConfig returns the configuration for a specific domain
// Falls back to legacy config if domain not found in multi-domain config
func (c *Config) GetDomainConfig(domain string) *DomainConfig {
	if c.Domains != nil {
		if dc, ok := c.Domains[domain]; ok {
			return &dc
		}
	}
	return nil
}

// GetDKIMConfig returns DKIM config for a domain
// First checks multi-domain config, then falls back to legacy config
func (c *Config) GetDKIMConfig(domain string) (enabled bool, selector, keyFile string) {
	// Check multi-domain config first
	if dc := c.GetDomainConfig(domain); dc != nil && dc.DKIM != nil && dc.DKIM.Enabled {
		return true, dc.DKIM.Selector, dc.DKIM.KeyFile
	}

	// Fall back to legacy config if domain matches
	if c.DKIM.Enabled && c.DKIM.Domain == domain {
		return true, c.DKIM.Selector, c.DKIM.KeyFile
	}

	return false, "", ""
}

// GetAllDomains returns all configured domains
func (c *Config) GetAllDomains() []string {
	domains := make(map[string]bool)

	// Add SMTP domain
	if c.SMTP.Domain != "" {
		domains[c.SMTP.Domain] = true
	}

	// Add legacy DKIM domain
	if c.DKIM.Enabled && c.DKIM.Domain != "" {
		domains[c.DKIM.Domain] = true
	}

	// Add multi-domain configs
	for domain := range c.Domains {
		domains[domain] = true
	}

	result := make([]string, 0, len(domains))
	for domain := range domains {
		result = append(result, domain)
	}
	return result
}

// validateDomains validates multi-domain configuration
func (c *Config) validateDomains() error {
	for domain, dc := range c.Domains {
		if domain == "" {
			return fmt.Errorf("empty domain name in domains configuration")
		}

		// Validate DKIM config
		if dc.DKIM != nil && dc.DKIM.Enabled {
			if dc.DKIM.Selector == "" {
				return fmt.Errorf("domains.%s.dkim.selector is required when DKIM is enabled", domain)
			}
			if dc.DKIM.KeyFile == "" {
				return fmt.Errorf("domains.%s.dkim.key_file is required when DKIM is enabled", domain)
			}
		}

		// Validate TLS config
		if dc.TLS != nil {
			if (dc.TLS.CertFile == "") != (dc.TLS.KeyFile == "") {
				return fmt.Errorf("domains.%s.tls requires both cert_file and key_file", domain)
			}
		}

		// Validate mode
		if dc.Mode != "" {
			validModes := map[string]bool{"production": true, "sandbox": true, "redirect": true, "bcc": true}
			if !validModes[dc.Mode] {
				return fmt.Errorf("domains.%s.mode must be one of: production, sandbox, redirect, bcc", domain)
			}

			if dc.Mode == "redirect" && len(dc.RedirectTo) == 0 {
				return fmt.Errorf("domains.%s.redirect_to is required when mode is redirect", domain)
			}
			if dc.Mode == "bcc" && len(dc.BCCTo) == 0 {
				return fmt.Errorf("domains.%s.bcc_to is required when mode is bcc", domain)
			}
		}
	}

	return nil
}

// LoadDynamicDomains loads domain configs from the dynamic domains file
// This merges API-created domains with static config file domains
func (c *Config) LoadDynamicDomains() error {
	if c.domainsFile == "" {
		return nil
	}

	data, err := os.ReadFile(c.domainsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's OK
		}
		return fmt.Errorf("failed to read domains file: %w", err)
	}

	var dynamicDomains map[string]DomainConfig
	if err := yaml.Unmarshal(data, &dynamicDomains); err != nil {
		return fmt.Errorf("failed to parse domains file: %w", err)
	}

	// Merge dynamic domains into config (dynamic domains override static)
	if c.Domains == nil {
		c.Domains = make(map[string]DomainConfig)
	}
	for domain, dc := range dynamicDomains {
		c.Domains[domain] = dc
	}

	return nil
}

// SaveDomains saves the current domain configuration to the dynamic domains file
func (c *Config) SaveDomains() error {
	if c.domainsFile == "" {
		return nil
	}

	data, err := yaml.Marshal(c.Domains)
	if err != nil {
		return fmt.Errorf("failed to marshal domains: %w", err)
	}

	if err := os.WriteFile(c.domainsFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write domains file: %w", err)
	}

	return nil
}

// SetDomainsFile sets the path for dynamic domains persistence
func (c *Config) SetDomainsFile(path string) {
	c.domainsFile = path
}
