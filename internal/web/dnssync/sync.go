package dnssync

import (
	"context"
	"fmt"
	"strings"

	"github.com/foxzi/sendry/internal/web/dnsprovider"
)

// Syncer reconciles recommended DNS records with a DNS provider.
type Syncer struct {
	Provider dnsprovider.Provider
	// Zone, when set, is used for every entry instead of resolving the
	// zone of each record name through the provider.
	Zone *dnsprovider.Zone
	// Confirm, when set, is asked before every create or update with the
	// plan for that record. On false the record is left alone; a non-empty
	// reason is reported as the record's error.
	Confirm func(SyncResult) (ok bool, reason string)
}

func (s *Syncer) zone(ctx context.Context, name string) (*dnsprovider.Zone, error) {
	if s.Zone != nil {
		return s.Zone, nil
	}
	return s.Provider.ResolveZone(ctx, name)
}

// SyncResult describes what was planned or done for one record.
type SyncResult struct {
	Kind     RecordType
	Type     string
	Name     string
	Expected string // recommended value
	Current  string // value at the provider now; several values are joined by newlines
	Proposed string // value publishing writes: Expected, or Current merged with it for SPF
	Action   Action
	Note     string // why this action, see the Note constants
	Optional bool   // a replacement that needs an explicit choice, see Decision
	Reason   string
	Applied  bool   // true if Apply was called and succeeded
	Error    string // non-empty on failure
}

// NeedsManualFix tells that the record is wrong in a way publishing does
// not repair (several SPF records, several keys under a DKIM selector).
func (r SyncResult) NeedsManualFix() bool {
	return r.Action == ActionSkip && r.Note != ""
}

// Changes tells whether publishing would create or replace the record.
func (r SyncResult) Changes() bool {
	return r.Action == ActionCreate || r.Action == ActionUpdate
}

// planOne compares one entry with the provider's records. It returns the
// zone and the ID of the record an update replaces. On a lookup failure the
// result is a skip and err is set.
func (s *Syncer) planOne(ctx context.Context, e PlanEntry) (res SyncResult, zone *dnsprovider.Zone, existingID string, err error) {
	res = SyncResult{Kind: e.Kind, Type: e.Type, Name: e.Name, Expected: e.Expected}
	if e.Action == ActionSkip {
		res.Action, res.Reason = ActionSkip, e.Reason
		return res, nil, "", nil
	}

	zone, err = s.zone(ctx, e.Name)
	if err != nil {
		res.Action, res.Reason = ActionSkip, fmt.Sprintf("zone lookup failed: %v", err)
		return res, nil, "", err
	}
	records, err := s.Provider.ListRecords(ctx, zone.ID, e.Name, e.Type)
	if err != nil {
		res.Action, res.Reason = ActionSkip, fmt.Sprintf("list records failed: %v", err)
		return res, nil, "", err
	}

	// A selector name holds exactly one key: with several TXT records there
	// verifiers fail, and replacing one of them would not fix that.
	if e.Kind == RecordDKIM && len(records) > 1 {
		values := make([]string, len(records))
		for i, r := range records {
			values[i] = r.Content
		}
		res.Current = strings.Join(values, "\n")
		res.Proposed = e.Expected
		res.Action, res.Note = ActionSkip, NoteDKIMMultiple
		res.Reason = "several TXT records at the selector name, leave one by hand"
		return res, zone, "", nil
	}

	// A name has at most one SPF record: with two, receivers treat SPF as
	// broken, and adding terms to one of them would not fix that.
	if e.Kind == RecordSPF {
		var spf []string
		for _, r := range records {
			if recordPrefix(NormalizeTXT(r.Content)) == "v=spf1" {
				spf = append(spf, r.Content)
			}
		}
		if len(spf) > 1 {
			res.Current = strings.Join(spf, "\n")
			res.Proposed = e.Expected
			res.Action, res.Note = ActionSkip, NoteSPFMultiple
			res.Reason = "several SPF records at the name, leave one by hand"
			return res, zone, "", nil
		}
	}

	existingID, res.Current = pickMatchingRecord(records, e)
	d := Evaluate(e.Kind, e.Expected, res.Current)
	res.Action, res.Proposed, res.Note, res.Reason, res.Optional = d.Action, d.Proposed, d.Note, d.Reason, d.Optional
	return res, zone, existingID, nil
}

// Plan fetches current records from the provider and returns a per-record
// plan without making any changes.
func (s *Syncer) Plan(ctx context.Context, entries []PlanEntry) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(entries))
	for _, e := range entries {
		res, _, _, _ := s.planOne(ctx, e)
		results = append(results, res)
	}
	return results, nil
}

// Apply fetches the plan and performs create/update calls on the provider.
// Entries with Action == ActionNoop/ActionSkip are left untouched, and so
// are the ones Confirm turns down.
func (s *Syncer) Apply(ctx context.Context, entries []PlanEntry) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(entries))
	for _, e := range entries {
		res, zone, existingID, err := s.planOne(ctx, e)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		if res.Changes() && s.Confirm != nil {
			if ok, why := s.Confirm(res); !ok {
				res.Error = why
				results = append(results, res)
				continue
			}
		}

		rec := dnsprovider.Record{Type: e.Type, Name: e.Name, Content: res.Proposed}
		switch res.Action {
		case ActionCreate:
			err = s.Provider.CreateRecord(ctx, zone.ID, rec)
		case ActionUpdate:
			err = s.Provider.UpdateRecord(ctx, zone.ID, existingID, rec)
		default:
			results = append(results, res)
			continue
		}
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Applied = true
		}
		results = append(results, res)
	}
	return results, nil
}

// pickMatchingRecord returns the existing record the entry should replace:
// one equal to the expected value, else one of the same family (v=spf1,
// v=DMARC1, v=DKIM1). DKIM and DMARC names hold nothing else, so any TXT
// there counts (a DKIM record may omit v=DKIM1). The apex is shared with
// unrelated TXT records (site verifications), so for SPF nothing else
// matches and the record is created next to them. Empty when nothing fits.
func pickMatchingRecord(records []dnsprovider.Record, e PlanEntry) (string, string) {
	if len(records) == 0 {
		return "", ""
	}
	normExpected := NormalizeTXT(e.Expected)
	prefix := recordPrefix(normExpected)

	// exact match
	for _, r := range records {
		if NormalizeTXT(r.Content) == normExpected {
			return r.ID, r.Content
		}
	}
	// same family (e.g. v=spf1 ...)
	if prefix != "" {
		for _, r := range records {
			if recordPrefix(NormalizeTXT(r.Content)) == prefix {
				return r.ID, r.Content
			}
		}
	}
	if e.Kind == RecordSPF {
		return "", ""
	}
	return records[0].ID, records[0].Content
}

// recordPrefix returns the "v=..." tag of a TXT record, lower-cased.
// For "v=spf1 a mx ~all" it returns "v=spf1". If no such tag is present,
// returns the empty string.
func recordPrefix(s string) string {
	if len(s) < 2 || s[0] != 'v' || s[1] != '=' {
		return ""
	}
	for i := 2; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == ';' {
			return toLower(s[:i])
		}
	}
	return toLower(s)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
