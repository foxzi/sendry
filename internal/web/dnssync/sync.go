package dnssync

import (
	"context"
	"fmt"

	"github.com/foxzi/sendry/internal/web/dnsprovider"
)

// Syncer reconciles recommended DNS records with a DNS provider.
type Syncer struct {
	Provider dnsprovider.Provider
}

// SyncResult describes what was planned or done for one record.
type SyncResult struct {
	Kind     RecordType
	Type     string
	Name     string
	Expected string
	Current  string
	Action   Action
	Reason   string
	Applied  bool   // true if Apply was called and succeeded
	Error    string // non-empty on failure
}

// Plan fetches current records from the provider and returns a per-record
// plan without making any changes.
func (s *Syncer) Plan(ctx context.Context, entries []PlanEntry) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(entries))
	for _, e := range entries {
		res := SyncResult{
			Kind:     e.Kind,
			Type:     e.Type,
			Name:     e.Name,
			Expected: e.Expected,
		}
		if e.Action == ActionSkip {
			res.Action = ActionSkip
			res.Reason = e.Reason
			results = append(results, res)
			continue
		}

		zone, err := s.Provider.ResolveZone(ctx, e.Name)
		if err != nil {
			res.Action = ActionSkip
			res.Reason = fmt.Sprintf("zone lookup failed: %v", err)
			results = append(results, res)
			continue
		}

		records, err := s.Provider.ListRecords(ctx, zone.ID, e.Name, e.Type)
		if err != nil {
			res.Action = ActionSkip
			res.Reason = fmt.Sprintf("list records failed: %v", err)
			results = append(results, res)
			continue
		}

		current := pickMatchingTXT(records, e.Expected)
		res.Current = current
		res.Action, res.Reason = DecideAction(e.Expected, current)
		results = append(results, res)
	}
	return results, nil
}

// Apply fetches the plan and performs create/update calls on the provider.
// Entries with Action == ActionNoop/ActionSkip are left untouched.
func (s *Syncer) Apply(ctx context.Context, entries []PlanEntry) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(entries))
	for _, e := range entries {
		res := SyncResult{
			Kind:     e.Kind,
			Type:     e.Type,
			Name:     e.Name,
			Expected: e.Expected,
		}
		if e.Action == ActionSkip {
			res.Action = ActionSkip
			res.Reason = e.Reason
			results = append(results, res)
			continue
		}

		zone, err := s.Provider.ResolveZone(ctx, e.Name)
		if err != nil {
			res.Action = ActionSkip
			res.Reason = fmt.Sprintf("zone lookup failed: %v", err)
			res.Error = err.Error()
			results = append(results, res)
			continue
		}

		records, err := s.Provider.ListRecords(ctx, zone.ID, e.Name, e.Type)
		if err != nil {
			res.Action = ActionSkip
			res.Reason = fmt.Sprintf("list records failed: %v", err)
			res.Error = err.Error()
			results = append(results, res)
			continue
		}

		existingID, current := pickMatchingRecord(records, e.Expected)
		res.Current = current
		res.Action, res.Reason = DecideAction(e.Expected, current)

		switch res.Action {
		case ActionCreate:
			err := s.Provider.CreateRecord(ctx, zone.ID, dnsprovider.Record{
				Type:    e.Type,
				Name:    e.Name,
				Content: e.Expected,
			})
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Applied = true
			}
		case ActionUpdate:
			err := s.Provider.UpdateRecord(ctx, zone.ID, existingID, dnsprovider.Record{
				Type:    e.Type,
				Name:    e.Name,
				Content: e.Expected,
			})
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Applied = true
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// pickMatchingTXT returns the content of the most relevant existing TXT:
// if any record matches expected normalized form, return it; otherwise
// pick one starting with the same prefix as expected (e.g. v=spf1, v=DMARC1);
// if none match by prefix, return the first record's content (or empty).
func pickMatchingTXT(records []dnsprovider.Record, expected string) string {
	_, content := pickMatchingRecord(records, expected)
	return content
}

// pickMatchingRecord is like pickMatchingTXT but also returns the record ID
// so it can be used for updates.
func pickMatchingRecord(records []dnsprovider.Record, expected string) (string, string) {
	if len(records) == 0 {
		return "", ""
	}
	normExpected := NormalizeTXT(expected)
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
