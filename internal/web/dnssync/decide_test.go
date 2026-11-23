package dnssync

import (
	"context"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/dnsprovider"
	"github.com/foxzi/sendry/internal/web/models"
)

func TestMergeSPF(t *testing.T) {
	const rec = "v=spf1 a mx include:_spf.semena.run ~all"
	tests := []struct {
		name, current, want string
	}{
		{"keeps the terms and the all qualifier",
			"v=spf1 ip4:159.69.198.110 a mx include:mail.funtik.tech -all",
			"v=spf1 ip4:159.69.198.110 a mx include:mail.funtik.tech include:_spf.semena.run -all"},
		{"nothing to add",
			"v=spf1 +A MX include:_SPF.semena.run ip4:1.2.3.4 ?all",
			"v=spf1 +A MX include:_SPF.semena.run ip4:1.2.3.4 ?all"},
		{"no all term",
			"v=spf1 ip4:1.2.3.4",
			"v=spf1 ip4:1.2.3.4 a mx include:_spf.semena.run ~all"},
		{"redirect stays last and gets no all",
			"v=spf1 mx redirect=_spf.example.net",
			"v=spf1 mx a include:_spf.semena.run redirect=_spf.example.net"},
		{"not an SPF record", "hello", rec},
	}
	for _, tt := range tests {
		if got := MergeSPF(tt.current, rec); got != tt.want {
			t.Errorf("%s:\n got  %q\n want %q", tt.name, got, tt.want)
		}
	}
}

func TestEvaluate(t *testing.T) {
	const key = "v=DKIM1; k=rsa; p=AAAABBBB"
	tests := []struct {
		name     string
		kind     RecordType
		expected string
		current  string
		action   Action
		note     string
		optional bool
	}{
		{"spf missing", RecordSPF, "v=spf1 a mx ~all", "", ActionCreate, NoteSPFCreate, false},
		{"spf merged", RecordSPF, "v=spf1 a mx include:x ~all", "v=spf1 a mx -all", ActionUpdate, NoteSPFMerge, false},
		{"spf superset", RecordSPF, "v=spf1 a mx ~all", "v=spf1 ip4:1.1.1.1 a mx -all", ActionNoop, NoteSPFOK, false},
		{"dmarc own policy", RecordDMARC, "v=DMARC1; p=quarantine", "v=DMARC1; p=none", ActionUpdate, NoteDMARCDiffers, true},
		{"dmarc same", RecordDMARC, "v=DMARC1; p=none", "\"v=DMARC1;  p=none\"", ActionNoop, NoteDMARCOK, false},
		{"dkim missing", RecordDKIM, key, "", ActionCreate, NoteDKIMCreate, false},
		{"dkim same key written differently", RecordDKIM, key, "k=rsa; t=s; p=AAAA BBBB", ActionNoop, NoteDKIMOK, false},
		{"dkim same key in two strings", RecordDKIM, key, `v=DKIM1; p=AAAA" "BBBB`, ActionNoop, NoteDKIMOK, false},
		{"dkim other key", RecordDKIM, key, "v=DKIM1; k=rsa; p=CCCC", ActionUpdate, NoteDKIMDiffers, true},
		{"dkim not a key", RecordDKIM, key, "some text", ActionUpdate, NoteDKIMNotKey, true},
	}
	for _, tt := range tests {
		d := Evaluate(tt.kind, tt.expected, tt.current)
		if d.Action != tt.action || d.Note != tt.note || d.Optional != tt.optional {
			t.Errorf("%s: got %s/%s optional=%v, want %s/%s optional=%v",
				tt.name, d.Action, d.Note, d.Optional, tt.action, tt.note, tt.optional)
		}
	}
}

func TestDiffValues(t *testing.T) {
	show := func(parts []DiffPart) string {
		var b strings.Builder
		for _, p := range parts {
			switch p.Op {
			case DiffAdd:
				b.WriteString("+")
			case DiffDel:
				b.WriteString("-")
			}
			b.WriteString(p.Text + " ")
		}
		return strings.TrimSpace(b.String())
	}

	got := show(DiffValues(RecordSPF, "v=spf1 ip4:1.1.1.1 a mx ~all", "v=spf1 ip4:1.1.1.1 a mx include:x ~all"))
	if want := "v=spf1 ip4:1.1.1.1 a mx +include:x ~all"; got != want {
		t.Errorf("spf diff = %q, want %q", got, want)
	}
	got = show(DiffValues(RecordDMARC, "v=DMARC1; p=none; sp=none", "v=DMARC1; p=quarantine"))
	if want := "v=DMARC1; -p=none; -sp=none; +p=quarantine;"; got != want {
		t.Errorf("dmarc diff = %q, want %q", got, want)
	}
	got = show(DiffValues(RecordSPF, "", "v=spf1 a ~all"))
	if want := "+v=spf1 +a +~all"; got != want {
		t.Errorf("new record diff = %q, want %q", got, want)
	}

	long := "p=" + strings.Repeat("A", 200)
	parts := DiffValues(RecordDKIM, "", "v=DKIM1; "+long)
	if p := parts[1]; p.Full != long+";" || len(p.Text) >= len(long) || !strings.Contains(p.Text, "…") {
		t.Errorf("long term not shortened: %+v", p)
	}
}

func TestSyncer_Apply_Confirm(t *testing.T) {
	fp := &fakeProvider{
		zoneID:   "zone1",
		zoneName: "example.com",
		records: map[string][]dnsprovider.Record{
			"example.com|TXT": {
				{ID: "r-spf", Type: "TXT", Name: "example.com", Content: "v=spf1 ip4:1.1.1.1 include:other.example -all"},
			},
			"_dmarc.example.com|TXT": {
				{ID: "r-dmarc", Type: "TXT", Name: "_dmarc.example.com", Content: "v=DMARC1; p=none"},
			},
		},
	}
	s := &Syncer{Provider: fp, Confirm: func(r SyncResult) (bool, string) {
		return r.Kind == RecordSPF, ""
	}}
	results, _ := s.Apply(context.Background(), BuildRecommended(&models.Domain{Domain: "example.com"}, "_spf.example.net"))

	if len(fp.updateCalls) != 1 || fp.updateCalls[0].ID != "r-spf" {
		t.Fatalf("updates = %+v, want only SPF", fp.updateCalls)
	}
	if got, want := fp.updateCalls[0].Record.Content, "v=spf1 ip4:1.1.1.1 include:other.example a mx include:_spf.example.net -all"; got != want {
		t.Errorf("SPF written = %q, want %q", got, want)
	}
	for _, r := range results {
		if r.Kind == RecordDMARC && (r.Applied || r.Error != "" || !r.Optional) {
			t.Errorf("DMARC = %+v, want left alone, optional, no error", r)
		}
	}
}

func TestSyncer_Plan_SeveralDKIMRecords(t *testing.T) {
	fp := &fakeProvider{
		zoneID:   "zone1",
		zoneName: "example.com",
		records: map[string][]dnsprovider.Record{
			"s1._domainkey.example.com|TXT": {
				{ID: "a", Content: "v=DKIM1; p=ONE"},
				{ID: "b", Content: "v=DKIM1; p=TWO"},
			},
		},
	}
	d := &models.Domain{Domain: "example.com", DKIMEnabled: true, DKIMSelector: "s1",
		DKIMKey: &models.DKIMKey{DNSRecord: "v=DKIM1; p=ONE"}}
	s := &Syncer{Provider: fp}
	results, _ := s.Apply(context.Background(), BuildRecommended(d, ""))
	for _, r := range results {
		if r.Kind == RecordDKIM && (r.Action != ActionSkip || r.Note != NoteDKIMMultiple || r.Applied) {
			t.Errorf("DKIM = %+v, want skipped as several records", r)
		}
	}
	for _, u := range fp.updateCalls {
		if u.ID == "a" || u.ID == "b" {
			t.Errorf("a DKIM record was replaced: %+v", u)
		}
	}
}

func TestSyncer_Plan_SeveralSPFRecords(t *testing.T) {
	fp := &fakeProvider{
		zoneID:   "zone1",
		zoneName: "example.com",
		records: map[string][]dnsprovider.Record{
			"example.com|TXT": {
				{ID: "a", Content: "v=spf1 ip4:192.0.2.1 -all"},
				{ID: "v", Content: "google-site-verification=abc"},
				{ID: "b", Content: "v=spf1 include:second.example ~all"},
			},
		},
	}
	s := &Syncer{Provider: fp}
	results, _ := s.Apply(context.Background(), BuildRecommended(&models.Domain{Domain: "example.com"}, ""))
	for _, r := range results {
		if r.Kind != RecordSPF {
			continue
		}
		if r.Action != ActionSkip || r.Note != NoteSPFMultiple || r.Applied || !r.NeedsManualFix() {
			t.Errorf("SPF = %+v, want skipped as several records", r)
		}
		if strings.Contains(r.Current, "google-site-verification") {
			t.Errorf("Current lists a record that is not SPF: %q", r.Current)
		}
	}
	for _, u := range fp.updateCalls {
		if u.ID == "a" || u.ID == "b" {
			t.Errorf("an SPF record was replaced: %+v", u)
		}
	}
}
