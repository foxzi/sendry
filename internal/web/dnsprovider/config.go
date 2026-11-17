package dnsprovider

import (
	"strings"

	"github.com/foxzi/sendry/internal/web/config"
)

// FromConfig returns the providers that have credentials in the config.
// A scoped Cloudflare API token wins over the email + Global API Key pair.
func FromConfig(c config.DNSProvidersConfig) []Provider {
	var out []Provider

	cf := c.Cloudflare
	token := strings.TrimSpace(cf.APIToken)
	email, key := strings.TrimSpace(cf.Email), strings.TrimSpace(cf.APIKey)
	switch {
	case token != "":
		out = append(out, NewCloudflare(token))
	case email != "" && key != "":
		out = append(out, NewCloudflareGlobalKey(email, key))
	}

	if u, t := strings.TrimSpace(c.Namedot.URL), strings.TrimSpace(c.Namedot.Token); u != "" && t != "" {
		out = append(out, NewNamedot(u, t))
	}

	return out
}
