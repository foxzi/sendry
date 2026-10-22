package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadYAML(t *testing.T, body string) (*Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "web.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(p)
}

const baseYAML = `
auth:
  local_enabled: true
  session_secret: "0123456789abcdef0123456789abcdef"
`

func TestTrustedProxies(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Server.TrustedProxies) != len(DefaultTrustedProxies) || cfg.Server.TrustedProxies[0] != "127.0.0.0/8" {
		t.Errorf("default = %v", cfg.Server.TrustedProxies)
	}
	for _, want := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"} {
		found := false
		for _, p := range cfg.Server.TrustedProxies {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Errorf("default trusted proxies miss %s: %v", want, cfg.Server.TrustedProxies)
		}
	}

	cfg, err = loadYAML(t, baseYAML+"server:\n  trusted_proxies: []\n")
	if err != nil || len(cfg.Server.TrustedProxies) != 0 {
		t.Errorf("explicit empty list = %v, %v", cfg.Server.TrustedProxies, err)
	}

	cfg, err = loadYAML(t, baseYAML+"server:\n  trusted_proxies: [\"10.0.0.0/8\", \"192.0.2.1\"]\n")
	if err != nil || len(cfg.Server.TrustedProxies) != 2 {
		t.Errorf("custom = %v, %v", cfg, err)
	}

	if _, err := loadYAML(t, baseYAML+"server:\n  trusted_proxies: [\"nope\"]\n"); err == nil {
		t.Error("invalid proxy accepted")
	}
}

func TestMultiSendStrategy(t *testing.T) {
	cfg, err := loadYAML(t, baseYAML)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sendry.MultiSend.Strategy != StrategyRoundRobin {
		t.Errorf("default strategy = %q", cfg.Sendry.MultiSend.Strategy)
	}

	for _, s := range []string{"round_robin", "weighted", "first_healthy", "weighted_round_robin", "domain_affinity"} {
		if _, err := loadYAML(t, baseYAML+"sendry:\n  multi_send:\n    strategy: "+s+"\n"); err != nil {
			t.Errorf("strategy %q rejected: %v", s, err)
		}
	}

	if _, err := loadYAML(t, baseYAML+"sendry:\n  multi_send:\n    strategy: random\n"); err == nil {
		t.Error("unknown strategy accepted")
	}
}
