package handlers

import (
	"context"
	"net"
	"testing"

	"github.com/foxzi/sendry/internal/dnscheck"
	"github.com/foxzi/sendry/internal/web/dnssync"
)

func TestCompareWithRecommended(t *testing.T) {
	res := &dnscheck.DomainCheckResult{Results: []dnscheck.CheckResult{
		{Type: "MX Records", Status: "ok"},
		{Type: "SPF Record", Status: "ok", Value: "v=spf1 a mx ~all"},
		{Type: "DKIM Record (mail._domainkey)", Status: "ok"},
		{Type: "DMARC Record", Status: "warning", Message: "none policy"},
		{Type: "MTA-STS Record", Status: "not_found"},
	}}
	recommended := []dnssync.PlanEntry{
		{Kind: dnssync.RecordSPF, Name: "example.com", Expected: "v=spf1 a mx ~all"},
		{Kind: dnssync.RecordDKIM, Name: "mail._domainkey.example.com", Expected: "v=DKIM1; k=rsa; p=NEW"},
		{Kind: dnssync.RecordDMARC, Name: "_dmarc.example.com", Expected: "v=DMARC1; p=quarantine"},
	}
	dns := map[string][]string{
		"example.com":                 {"google-site-verification=x", "v=spf1  a mx ~all"},
		"mail._domainkey.example.com": {"v=DKIM1; k=rsa; p=OLD"},
		"_dmarc.example.com":          {"v=DMARC1; p=none"},
	}
	lookup := func(_ context.Context, name string) ([]string, error) {
		if v, ok := dns[name]; ok {
			return v, nil
		}
		return nil, &net.DNSError{IsNotFound: true}
	}

	compareWithRecommended(context.Background(), res, recommended, lookup)

	if r := res.Results[1]; r.Status != "ok" || r.Message != "Matches the recommended value" {
		t.Errorf("SPF = %+v", r)
	}
	if r := res.Results[2]; r.Status != "warning" {
		t.Errorf("stale DKIM key not flagged: %+v", r)
	}
	if r := res.Results[3]; r.Message != "none policy" {
		t.Errorf("existing warning overwritten: %+v", r)
	}
	if s := res.Summary; s.OK != 2 || s.Warnings != 2 || s.NotFound != 1 {
		t.Errorf("summary = %+v", s)
	}
}

func TestPickTXT(t *testing.T) {
	records := []string{"foo", "v=DMARC1; p=none", "v=DMARC1; p=quarantine"}
	if got := pickTXT(records, "v=DMARC1; p=quarantine"); got != "v=DMARC1; p=quarantine" {
		t.Errorf("exact match = %q", got)
	}
	if got := pickTXT(records, "v=DMARC1; p=reject"); got != "v=DMARC1; p=none" {
		t.Errorf("same family = %q", got)
	}
	if got := pickTXT([]string{"foo"}, "v=spf1 -all"); got != "" {
		t.Errorf("unrelated = %q", got)
	}
}
