package dnssync

import (
	"context"
	"testing"

	"github.com/foxzi/sendry/internal/web/dnsprovider"
	"github.com/foxzi/sendry/internal/web/models"
)

type fakeProvider struct {
	zoneID      string
	zoneName    string
	records     map[string][]dnsprovider.Record // key: name+type
	createCalls []dnsprovider.Record
	updateCalls []struct {
		ID     string
		Record dnsprovider.Record
	}
	failOnCreate bool
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) ResolveZone(_ context.Context, fqdn string) (*dnsprovider.Zone, error) {
	return &dnsprovider.Zone{ID: f.zoneID, Name: f.zoneName}, nil
}

func (f *fakeProvider) ListRecords(_ context.Context, _ string, name, t string) ([]dnsprovider.Record, error) {
	return f.records[name+"|"+t], nil
}

func (f *fakeProvider) CreateRecord(_ context.Context, _ string, r dnsprovider.Record) error {
	if f.failOnCreate {
		return errFake
	}
	f.createCalls = append(f.createCalls, r)
	key := r.Name + "|" + r.Type
	r.ID = "new-id"
	f.records[key] = append(f.records[key], r)
	return nil
}

func (f *fakeProvider) UpdateRecord(_ context.Context, _ string, id string, r dnsprovider.Record) error {
	f.updateCalls = append(f.updateCalls, struct {
		ID     string
		Record dnsprovider.Record
	}{ID: id, Record: r})
	key := r.Name + "|" + r.Type
	for i, rec := range f.records[key] {
		if rec.ID == id {
			f.records[key][i].Content = r.Content
			return nil
		}
	}
	return nil
}

var errFake = fakeErr("create failed")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestSyncer_Plan_CreateUpdateNoop(t *testing.T) {
	fp := &fakeProvider{
		zoneID:   "zone1",
		zoneName: "example.com",
		records: map[string][]dnsprovider.Record{
			// SPF already matches
			"example.com|TXT": {
				{ID: "r-spf", Type: "TXT", Name: "example.com", Content: "v=spf1 a mx include:_spf.mailgun.org ~all"},
			},
			// DMARC exists but differs
			"_dmarc.example.com|TXT": {
				{ID: "r-dmarc", Type: "TXT", Name: "_dmarc.example.com", Content: "v=DMARC1; p=none"},
			},
			// DKIM missing
		},
	}

	d := &models.Domain{
		Domain:       "example.com",
		DKIMEnabled:  true,
		DKIMSelector: "s1",
		DKIMKey:      &models.DKIMKey{DNSRecord: "v=DKIM1; k=rsa; p=ZZZ"},
	}
	entries := BuildRecommended(d, "_spf.mailgun.org")

	s := &Syncer{Provider: fp}
	results, err := s.Plan(context.Background(), entries)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	got := map[RecordType]SyncResult{}
	for _, r := range results {
		got[r.Kind] = r
	}

	if got[RecordSPF].Action != ActionNoop {
		t.Errorf("SPF action = %q, want noop", got[RecordSPF].Action)
	}
	if got[RecordDMARC].Action != ActionUpdate {
		t.Errorf("DMARC action = %q, want update", got[RecordDMARC].Action)
	}
	if got[RecordDKIM].Action != ActionCreate {
		t.Errorf("DKIM action = %q, want create", got[RecordDKIM].Action)
	}
}

func TestSyncer_Apply_CreatesAndUpdates(t *testing.T) {
	fp := &fakeProvider{
		zoneID:   "zone1",
		zoneName: "example.com",
		records: map[string][]dnsprovider.Record{
			"_dmarc.example.com|TXT": {
				{ID: "r-dmarc", Type: "TXT", Name: "_dmarc.example.com", Content: "v=DMARC1; p=none"},
			},
		},
	}

	d := &models.Domain{Domain: "example.com"}
	entries := BuildRecommended(d, "")

	s := &Syncer{Provider: fp}
	results, err := s.Apply(context.Background(), entries)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if len(fp.createCalls) != 1 {
		t.Errorf("expected 1 create call, got %d", len(fp.createCalls))
	}
	if len(fp.updateCalls) != 1 {
		t.Errorf("expected 1 update call, got %d", len(fp.updateCalls))
	}

	appliedCount := 0
	for _, r := range results {
		if r.Applied {
			appliedCount++
		}
	}
	if appliedCount != 2 {
		t.Errorf("applied = %d, want 2 (SPF create + DMARC update)", appliedCount)
	}
}
