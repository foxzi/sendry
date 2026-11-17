package dnsprovider

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
)

func TestFromConfig(t *testing.T) {
	if got := FromConfig(config.DNSProvidersConfig{}); len(got) != 0 {
		t.Fatalf("empty config gave %d providers", len(got))
	}

	// Half-filled credentials are ignored.
	got := FromConfig(config.DNSProvidersConfig{
		Cloudflare: config.CloudflareDNSConfig{Email: "a@example.com"},
		Namedot:    config.NamedotDNSConfig{URL: "https://dns.example.com"},
	})
	if len(got) != 0 {
		t.Fatalf("incomplete credentials gave %d providers", len(got))
	}

	got = FromConfig(config.DNSProvidersConfig{
		Cloudflare: config.CloudflareDNSConfig{APIToken: "t", Email: "a@example.com", APIKey: "k"},
		Namedot:    config.NamedotDNSConfig{URL: "https://dns.example.com/", Token: "n"},
	})
	if len(got) != 2 || got[0].Name() != "cloudflare" || got[1].Name() != "namedot" {
		t.Fatalf("providers = %v", got)
	}
	if cf := got[0].(*CloudflareProvider); cf.AuthMode != CloudflareAuthToken {
		t.Errorf("token should win over global key, mode = %v", cf.AuthMode)
	}

	got = FromConfig(config.DNSProvidersConfig{Cloudflare: config.CloudflareDNSConfig{Email: "a@example.com", APIKey: "k"}})
	if len(got) != 1 || got[0].(*CloudflareProvider).AuthMode != CloudflareAuthGlobalKey {
		t.Errorf("global key provider = %v", got)
	}
}
