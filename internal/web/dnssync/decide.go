package dnssync

import "strings"

// Notes tell why a record got its action; the panel turns them into hints.
const (
	NoteSPFCreate    = "spf_create"
	NoteSPFMerge     = "spf_merge" // the current record is kept, missing terms are added
	NoteSPFOK        = "spf_ok"
	NoteSPFMultiple  = "spf_multiple" // several SPF records at the name
	NoteDMARCCreate  = "dmarc_create"
	NoteDMARCDiffers = "dmarc_differs"
	NoteDMARCOK      = "dmarc_ok"
	NoteDKIMCreate   = "dkim_create"
	NoteDKIMOK       = "dkim_ok"       // the published key is the panel's key
	NoteDKIMDiffers  = "dkim_differs"  // another key under the same selector
	NoteDKIMNotKey   = "dkim_not_key"  // the TXT at the selector name is not a DKIM key
	NoteDKIMMultiple = "dkim_multiple" // several TXT at the selector name
)

// Decision is what publishing a recommended record would do given the
// value that is there now.
type Decision struct {
	Action   Action
	Proposed string // value that would be written
	Note     string
	Reason   string
	// Optional marks a replacement that may break what the current value
	// serves (someone else's DKIM key, the domain's own DMARC policy), so
	// it needs an explicit choice rather than being part of the default.
	Optional bool
}

// Evaluate compares the recommended value with the current one ("" when
// there is no record) by the rules of the record kind: SPF is merged into
// the current record, DKIM is compared by the public key only, the rest by
// the whole value.
func Evaluate(kind RecordType, expected, current string) Decision {
	d := Decision{Proposed: expected}
	if strings.TrimSpace(current) == "" {
		d.Action, d.Reason = ActionCreate, "no current record found"
		switch kind {
		case RecordSPF:
			d.Note = NoteSPFCreate
		case RecordDMARC:
			d.Note = NoteDMARCCreate
		case RecordDKIM:
			d.Note = NoteDKIMCreate
		}
		return d
	}

	switch kind {
	case RecordSPF:
		d.Proposed = MergeSPF(current, expected)
		if NormalizeTXT(d.Proposed) == NormalizeTXT(current) {
			d.Action, d.Note, d.Reason = ActionNoop, NoteSPFOK, "has every recommended term"
		} else {
			d.Action, d.Note, d.Reason = ActionUpdate, NoteSPFMerge, "recommended terms are missing"
		}
	case RecordDKIM:
		key := DKIMPublicKey(current)
		switch {
		case key == "":
			d.Action, d.Note, d.Reason, d.Optional = ActionUpdate, NoteDKIMNotKey, "current record is not a DKIM key", true
		case key == DKIMPublicKey(expected):
			d.Action, d.Note, d.Reason = ActionNoop, NoteDKIMOK, "same public key"
			d.Proposed = current
		default:
			d.Action, d.Note, d.Reason, d.Optional = ActionUpdate, NoteDKIMDiffers, "another public key is published", true
		}
	default:
		d.Action, d.Reason = DecideAction(expected, current)
		if kind == RecordDMARC {
			d.Note = NoteDMARCOK
			if d.Action == ActionUpdate {
				d.Note, d.Optional = NoteDMARCDiffers, true
			}
		}
	}
	return d
}

// MergeSPF returns the current SPF record with the recommended mechanisms
// it lacks added before its "all" term (or redirect modifier). Everything
// already there stays, including the "all" qualifier; the recommended one
// is used only when the current record has neither "all" nor a redirect.
func MergeSPF(current, expected string) string {
	cur := strings.Fields(NormalizeTXT(current))
	exp := strings.Fields(NormalizeTXT(expected))
	if len(cur) == 0 || !strings.EqualFold(cur[0], "v=spf1") || len(exp) == 0 {
		return NormalizeTXT(expected)
	}

	have := make(map[string]bool, len(cur))
	end := len(cur)
	for i, t := range cur {
		have[spfTermKey(t)] = true
		if end == len(cur) && (isSPFAll(t) || strings.HasPrefix(strings.ToLower(t), "redirect=")) {
			end = i
		}
	}

	var add []string
	expAll := ""
	for _, t := range exp[1:] {
		if isSPFAll(t) {
			expAll = t
			continue
		}
		if !have[spfTermKey(t)] {
			have[spfTermKey(t)] = true
			add = append(add, t)
		}
	}

	out := make([]string, 0, len(cur)+len(add)+1)
	out = append(out, cur[:end]...)
	out = append(out, add...)
	out = append(out, cur[end:]...)
	if end == len(cur) && expAll != "" {
		out = append(out, expAll)
	}
	return strings.Join(out, " ")
}

// spfTermKey makes "+include:X" and "include:x" the same term.
func spfTermKey(t string) string {
	return strings.TrimPrefix(strings.ToLower(t), "+")
}

func isSPFAll(t string) bool {
	return strings.TrimLeft(strings.ToLower(t), "+-~?") == "all"
}

// DKIMPublicKey returns the p= tag of a DKIM record without whitespace and
// quotes (long keys are stored as several strings), or "" when there is none.
func DKIMPublicKey(record string) string {
	for _, tag := range strings.Split(record, ";") {
		k, v, ok := strings.Cut(tag, "=")
		if !ok || strings.ToLower(strings.Trim(k, " \t\r\n\"")) != "p" {
			continue
		}
		return strings.Map(func(r rune) rune {
			if r == '"' || r == ' ' || r == '\t' || r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, v)
	}
	return ""
}

var noteHints = map[string]string{
	NoteSPFCreate:    "There is no SPF record yet, it will be created.",
	NoteSPFMerge:     "The current record stays, only the missing terms are added.",
	NoteSPFOK:        "The record already has everything it needs.",
	NoteSPFMultiple:  "There are several SPF records on this name and there must be one, otherwise receivers ignore SPF. Leave one by hand, then run the check again.",
	NoteDMARCCreate:  "There is no DMARC record yet, it will be created.",
	NoteDMARCDiffers: "The domain already has its own DMARC policy. Tick it only to replace the policy with the recommended one.",
	NoteDMARCOK:      "Matches the recommended policy.",
	NoteDKIMCreate:   "No key is published under this selector yet, it will be created.",
	NoteDKIMOK:       "The published key is the one the panel signs with.",
	NoteDKIMDiffers:  "Another key is published under this selector. Tick it to replace the key if it is your old one. If another service signs with it, give the panel key a different selector instead.",
	NoteDKIMNotKey:   "The record under this name is not a DKIM key. Tick it to replace the record with the panel key.",
	NoteDKIMMultiple: "There are several records under this selector and there must be one. Leave one by hand, then run the check again.",
}

// Hint explains the planned action in a sentence (English; the panel
// translates by Note).
func (r SyncResult) Hint() string {
	if h, ok := noteHints[r.Note]; ok {
		return h
	}
	return r.Reason
}
