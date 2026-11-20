package dnssync

import "strings"

// Diff operations.
const (
	DiffSame = "same"
	DiffAdd  = "add"
	DiffDel  = "del"
)

// DiffPart is one term of a record value in a preview of a change.
type DiffPart struct {
	Text string // term as shown; long ones (DKIM keys) are shortened
	Full string // whole term when Text is shortened
	Op   string
}

// diffTermMax is the longest term shown whole; it is also about where the
// fixed header of a base64 RSA public key ends.
const diffTermMax = 48

// Diff lists the terms of the value after publishing in order, each marked
// as kept or added, with the terms that would be removed in their old place.
func (r SyncResult) Diff() []DiffPart {
	return DiffValues(r.Kind, r.Current, r.Proposed)
}

// DiffValues compares two values of a record term by term: SPF terms are
// separated by spaces, DKIM and DMARC tags by semicolons.
func DiffValues(kind RecordType, from, to string) []DiffPart {
	a, b := splitTerms(kind, from), splitTerms(kind, to)

	// Longest common subsequence of terms.
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	parts := make([]DiffPart, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			parts = append(parts, diffPart(kind, a[i], DiffSame))
			i++
			j++
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			parts = append(parts, diffPart(kind, a[i], DiffDel))
			i++
		default:
			parts = append(parts, diffPart(kind, b[j], DiffAdd))
			j++
		}
	}
	return parts
}

func splitTerms(kind RecordType, value string) []string {
	value = NormalizeTXT(value)
	if kind == RecordSPF {
		return strings.Fields(value)
	}
	var terms []string
	for _, t := range strings.Split(value, ";") {
		if t = strings.TrimSpace(t); t != "" {
			terms = append(terms, t)
		}
	}
	return terms
}

func diffPart(kind RecordType, term, op string) DiffPart {
	if kind != RecordSPF {
		term += ";"
	}
	p := DiffPart{Text: term, Op: op}
	if len(term) > diffTermMax {
		// RSA keys of one size start and end with the same characters, so
		// the piece shown comes from past that common header.
		p.Full = term
		p.Text = term[:6] + "…" + term[diffTermMax:min(len(term), diffTermMax+20)] + "…"
	}
	return p
}
