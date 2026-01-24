package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Create temp config file
	content := `
server:
  hostname: "mail.test.com"

smtp:
  listen_addr: ":2525"
  submission_addr: ":2587"
  domain: "test.com"
  max_message_bytes: 5242880
  max_recipients: 50
  read_timeout: 30s
  write_timeout: 30s
  auth:
    required: true
    users:
      testuser: "testpass"

api:
  listen_addr: ":9080"
  api_key: "test-api-key"

queue:
  workers: 2
  retry_interval: 1m
  max_retries: 3
  process_interval: 5s

storage:
  path: "/tmp/test.db"

logging:
  level: "debug"
  format: "text"
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check values
	if cfg.Server.Hostname != "mail.test.com" {
		t.Errorf("Hostname = %v, want mail.test.com", cfg.Server.Hostname)
	}
	if cfg.SMTP.ListenAddr != ":2525" {
		t.Errorf("SMTP.ListenAddr = %v, want :2525", cfg.SMTP.ListenAddr)
	}
	if cfg.SMTP.Domain != "test.com" {
		t.Errorf("SMTP.Domain = %v, want test.com", cfg.SMTP.Domain)
	}
	if !cfg.SMTP.Auth.Required {
		t.Error("SMTP.Auth.Required = false, want true")
	}
	if cfg.SMTP.Auth.Users["testuser"] != "testpass" {
		t.Errorf("SMTP.Auth.Users[testuser] = %v, want testpass", cfg.SMTP.Auth.Users["testuser"])
	}
	if cfg.API.APIKey != "test-api-key" {
		t.Errorf("API.APIKey = %v, want test-api-key", cfg.API.APIKey)
	}
	if cfg.Queue.Workers != 2 {
		t.Errorf("Queue.Workers = %v, want 2", cfg.Queue.Workers)
	}
	if cfg.Queue.RetryInterval != time.Minute {
		t.Errorf("Queue.RetryInterval = %v, want 1m", cfg.Queue.RetryInterval)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %v, want debug", cfg.Logging.Level)
	}
}

func TestLoadDefaults(t *testing.T) {
	content := `
smtp:
  domain: "test.com"
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check defaults
	if cfg.SMTP.ListenAddr != ":25" {
		t.Errorf("SMTP.ListenAddr = %v, want :25", cfg.SMTP.ListenAddr)
	}
	if cfg.SMTP.SubmissionAddr != ":587" {
		t.Errorf("SMTP.SubmissionAddr = %v, want :587", cfg.SMTP.SubmissionAddr)
	}
	if cfg.SMTP.MaxMessageBytes != 10*1024*1024 {
		t.Errorf("SMTP.MaxMessageBytes = %v, want 10MB", cfg.SMTP.MaxMessageBytes)
	}
	if cfg.API.ListenAddr != ":8080" {
		t.Errorf("API.ListenAddr = %v, want :8080", cfg.API.ListenAddr)
	}
	if cfg.Queue.Workers != 4 {
		t.Errorf("Queue.Workers = %v, want 4", cfg.Queue.Workers)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %v, want info", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Logging.Format = %v, want json", cfg.Logging.Format)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				SMTP:    SMTPConfig{Domain: "test.com"},
				Logging: LoggingConfig{Level: "info", Format: "json"},
			},
			wantErr: false,
		},
		{
			name: "missing domain",
			cfg: Config{
				SMTP:    SMTPConfig{Domain: ""},
				Logging: LoggingConfig{Level: "info", Format: "json"},
			},
			wantErr: true,
		},
		{
			name: "auth required but no users",
			cfg: Config{
				SMTP: SMTPConfig{
					Domain: "test.com",
					Auth:   AuthConfig{Required: true, Users: nil},
				},
				Logging: LoggingConfig{Level: "info", Format: "json"},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			cfg: Config{
				SMTP:    SMTPConfig{Domain: "test.com"},
				Logging: LoggingConfig{Level: "invalid", Format: "json"},
			},
			wantErr: true,
		},
		{
			name: "invalid log format",
			cfg: Config{
				SMTP:    SMTPConfig{Domain: "test.com"},
				Logging: LoggingConfig{Level: "info", Format: "invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() expected error for nonexistent file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	content := `invalid: yaml: content: [`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("Load() expected error for invalid YAML")
	}
}
