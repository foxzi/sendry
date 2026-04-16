package dnssync

import (
	"fmt"
	"strings"

	"github.com/foxzi/sendry/internal/web/models"
)

// RecordType represents which kind of record the plan entry is.
type RecordType string

const (
	RecordSPF   RecordType = "SPF"
	RecordDKIM  RecordType = "DKIM"
	RecordDMARC RecordType = "DMARC"
)

// Action describes what should happen with a record.
type Action string

const (
	ActionNoop   Action = "noop"
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionSkip   Action = "skip" // skipped (e.g. DKIM key not available)
)

// PlanEntry is a single recommended-vs-current comparison for one record.
type PlanEntry struct {
	Kind     RecordType
	Type     string // DNS record type, e.g. TXT
	Name     string // FQDN
	Expected string
	Current  string
	Action   Action
	Reason   string // optional human-readable reason, especially for Skip/Noop
}

// BuildRecommended returns recommended DNS records for a domain.
// spfInclude is the value of the global variable `spf_include` (may be empty).
// When DKIM is enabled on the domain and a key is linked, a DKIM entry is
// added with the expected DNS record value; otherwise DKIM is skipped.
func BuildRecommended(domain *models.Domain, spfInclude string) []PlanEntry {
	d := strings.ToLower(strings.TrimSpace(domain.Domain))
	entries := make([]PlanEntry, 0, 3)

	// SPF
	spfValue := "v=spf1 a mx ~all"
	if s := strings.TrimSpace(spfInclude); s != "" {
		spfValue = fmt.Sprintf("v=spf1 a mx include:%s ~all", s)
	}
	entries = append(entries, PlanEntry{
		Kind:     RecordSPF,
		Type:     "TXT",
		Name:     d,
		Expected: spfValue,
	})

	// DMARC
	entries = append(entries, PlanEntry{
		Kind:     RecordDMARC,
		Type:     "TXT",
		Name:     "_dmarc." + d,
		Expected: fmt.Sprintf("v=DMARC1; p=quarantine; rua=mailto:dmarc@%s", d),
	})

	// DKIM (only when enabled and key is linked)
	if domain.DKIMEnabled && domain.DKIMKey != nil && strings.TrimSpace(domain.DKIMKey.DNSRecord) != "" {
		selector := domain.DKIMSelector
		if selector == "" {
			selector = "mail"
		}
		entries = append(entries, PlanEntry{
			Kind:     RecordDKIM,
			Type:     "TXT",
			Name:     fmt.Sprintf("%s._domainkey.%s", selector, d),
			Expected: domain.DKIMKey.DNSRecord,
		})
	} else {
		entries = append(entries, PlanEntry{
			Kind:   RecordDKIM,
			Type:   "TXT",
			Name:   "",
			Action: ActionSkip,
			Reason: "DKIM is not enabled or DKIM key is not linked",
		})
	}

	return entries
}

// NormalizeTXT canonicalizes a TXT record value for comparison:
// trims surrounding whitespace and quotes and collapses inner whitespace.
func NormalizeTXT(s string) string {
	s = strings.TrimSpace(s)
	// Cloudflare returns TXT without surrounding quotes, but be defensive.
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")
	// Collapse any run of whitespace to a single space.
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// DecideAction compares expected and current and returns the action to take.
// `current` is empty when no record exists yet.
func DecideAction(expected, current string) (Action, string) {
	switch {
	case current == "":
		return ActionCreate, "no current record found"
	case NormalizeTXT(expected) == NormalizeTXT(current):
		return ActionNoop, "matches expected value"
	default:
		return ActionUpdate, "value differs from expected"
	}
}
