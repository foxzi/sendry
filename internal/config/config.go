package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the main configuration structure
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	SMTP    SMTPConfig    `yaml:"smtp"`
	API     APIConfig     `yaml:"api"`
	Queue   QueueConfig   `yaml:"queue"`
	Storage StorageConfig `yaml:"storage"`
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig contains server-wide settings
type ServerConfig struct {
	Hostname string `yaml:"hostname"` // FQDN of the server
}

// SMTPConfig contains SMTP server settings
type SMTPConfig struct {
	ListenAddr      string        `yaml:"listen_addr"`
	SubmissionAddr  string        `yaml:"submission_addr"`
	Domain          string        `yaml:"domain"`
	MaxMessageBytes int           `yaml:"max_message_bytes"`
	MaxRecipients   int           `yaml:"max_recipients"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	Auth            AuthConfig    `yaml:"auth"`
}

// AuthConfig contains SMTP authentication settings
type AuthConfig struct {
	Required bool              `yaml:"required"`
	Users    map[string]string `yaml:"users"` // username -> password
}

// APIConfig contains HTTP API settings
type APIConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	APIKey     string `yaml:"api_key"`
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
	Path string `yaml:"path"`
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

	if c.API.ListenAddr == "" {
		c.API.ListenAddr = ":8080"
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

	return nil
}
