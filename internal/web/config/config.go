package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Sendry   SendryConfig   `yaml:"sendry"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	ListenAddr string    `yaml:"listen_addr"`
	TLS        TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	LocalEnabled  bool          `yaml:"local_enabled"`
	SessionSecret string        `yaml:"session_secret"`
	SessionTTL    time.Duration `yaml:"session_ttl"`
	OIDC          OIDCConfig    `yaml:"oidc"`
}

type OIDCConfig struct {
	Enabled       bool     `yaml:"enabled"`
	Provider      string   `yaml:"provider"`
	ClientID      string   `yaml:"client_id"`
	ClientSecret  string   `yaml:"client_secret"`
	IssuerURL     string   `yaml:"issuer_url"`
	RedirectURL   string   `yaml:"redirect_url"`
	Scopes        []string `yaml:"scopes"`
	AllowedGroups []string `yaml:"allowed_groups"`
}

type SendryConfig struct {
	Servers   []SendryServer  `yaml:"servers"`
	MultiSend MultiSendConfig `yaml:"multi_send"`
}

type SendryServer struct {
	Name    string `yaml:"name"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Env     string `yaml:"env"`
}

type MultiSendConfig struct {
	Strategy       string            `yaml:"strategy"`
	Weights        map[string]int    `yaml:"weights"`
	DomainAffinity map[string]string `yaml:"domain_affinity"`
	Failover       FailoverConfig    `yaml:"failover"`
}

type FailoverConfig struct {
	Enabled    bool `yaml:"enabled"`
	MaxRetries int  `yaml:"max_retries"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	setDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = ":8088"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "/var/lib/sendry-web/app.db"
	}
	if cfg.Auth.SessionTTL == 0 {
		cfg.Auth.SessionTTL = 24 * time.Hour
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if len(cfg.Auth.OIDC.Scopes) == 0 {
		cfg.Auth.OIDC.Scopes = []string{"openid", "profile", "email"}
	}
	if cfg.Sendry.MultiSend.Strategy == "" {
		cfg.Sendry.MultiSend.Strategy = "round_robin"
	}
}

func validate(cfg *Config) error {
	if cfg.Auth.SessionSecret == "" {
		return fmt.Errorf("auth.session_secret is required")
	}
	if len(cfg.Auth.SessionSecret) < 32 {
		return fmt.Errorf("auth.session_secret must be at least 32 characters")
	}
	if !cfg.Auth.LocalEnabled && !cfg.Auth.OIDC.Enabled {
		return fmt.Errorf("at least one auth method must be enabled (local or OIDC)")
	}
	if cfg.Auth.OIDC.Enabled {
		if cfg.Auth.OIDC.ClientID == "" {
			return fmt.Errorf("auth.oidc.client_id is required when OIDC is enabled")
		}
		if cfg.Auth.OIDC.ClientSecret == "" {
			return fmt.Errorf("auth.oidc.client_secret is required when OIDC is enabled")
		}
		if cfg.Auth.OIDC.IssuerURL == "" {
			return fmt.Errorf("auth.oidc.issuer_url is required when OIDC is enabled")
		}
	}
	return nil
}
