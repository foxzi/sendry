package dnssync

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestBuildRecommended_SPFWithoutInclude(t *testing.T) {
	d := &models.Domain{Domain: "example.com"}
	entries := BuildRecommended(d, "")

	spf := findByKind(t, entries, RecordSPF)
	if spf.Expected != "v=spf1 a mx ~all" {
		t.Errorf("SPF = %q, want default without include", spf.Expected)
	}
	if spf.Name != "example.com" {
		t.Errorf("SPF Name = %q, want %q", spf.Name, "example.com")
	}
}

func TestBuildRecommended_SPFWithInclude(t *testing.T) {
	d := &models.Domain{Domain: "Example.COM"}
	entries := BuildRecommended(d, "_spf.mailgun.org")

	spf := findByKind(t, entries, RecordSPF)
	if spf.Expected != "v=spf1 a mx include:_spf.mailgun.org ~all" {
		t.Errorf("SPF = %q, want include", spf.Expected)
	}
	if spf.Name != "example.com" {
		t.Errorf("SPF Name = %q, want lowercased", spf.Name)
	}
}

func TestBuildRecommended_DMARC(t *testing.T) {
	d := &models.Domain{Domain: "example.com"}
	entries := BuildRecommended(d, "")

	dmarc := findByKind(t, entries, RecordDMARC)
	if dmarc.Name != "_dmarc.example.com" {
		t.Errorf("DMARC Name = %q", dmarc.Name)
	}
	if dmarc.Expected != "v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com" {
		t.Errorf("DMARC Expected = %q", dmarc.Expected)
	}
}

func TestBuildRecommended_DKIM_Skipped(t *testing.T) {
	d := &models.Domain{Domain: "example.com"}
	entries := BuildRecommended(d, "")

	dkim := findByKind(t, entries, RecordDKIM)
	if dkim.Action != ActionSkip {
		t.Errorf("DKIM Action = %q, want Skip", dkim.Action)
	}
}

func TestBuildRecommended_DKIM_Enabled(t *testing.T) {
	d := &models.Domain{
		Domain:       "example.com",
		DKIMEnabled:  true,
		DKIMSelector: "s1",
		DKIMKey: &models.DKIMKey{
			DNSRecord: "v=DKIM1; k=rsa; p=XYZ",
		},
	}
	entries := BuildRecommended(d, "")
	dkim := findByKind(t, entries, RecordDKIM)

	if dkim.Name != "s1._domainkey.example.com" {
		t.Errorf("DKIM Name = %q", dkim.Name)
	}
	if dkim.Expected != "v=DKIM1; k=rsa; p=XYZ" {
		t.Errorf("DKIM Expected = %q", dkim.Expected)
	}
	if dkim.Action == ActionSkip {
		t.Error("DKIM should not be skipped when enabled and linked")
	}
}

func TestDecideAction(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		current  string
		want     Action
	}{
		{"no current -> create", "v=spf1 a mx ~all", "", ActionCreate},
		{"match -> noop", "v=spf1 a mx ~all", "v=spf1 a mx ~all", ActionNoop},
		{"whitespace diff -> noop", "v=spf1 a mx ~all", "v=spf1  a  mx  ~all", ActionNoop},
		{"quoted match -> noop", "v=spf1 a mx ~all", "\"v=spf1 a mx ~all\"", ActionNoop},
		{"different -> update", "v=spf1 a mx ~all", "v=spf1 -all", ActionUpdate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := DecideAction(tt.expected, tt.current)
			if got != tt.want {
				t.Errorf("DecideAction(%q,%q) = %q, want %q", tt.expected, tt.current, got, tt.want)
			}
		})
	}
}

func findByKind(t *testing.T, entries []PlanEntry, kind RecordType) PlanEntry {
	t.Helper()
	for _, e := range entries {
		if e.Kind == kind {
			return e
		}
	}
	t.Fatalf("no entry of kind %q found", kind)
	return PlanEntry{}
}
