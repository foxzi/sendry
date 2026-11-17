package handlers

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/foxzi/sendry/internal/dnscheck"
	"github.com/foxzi/sendry/internal/web/dnssync"
)

// txtLookupFunc matches net.Resolver.LookupTXT; tests replace it.
type txtLookupFunc func(ctx context.Context, name string) ([]string, error)

// checkDomainRecords checks the domain's mail records from the panel itself
// and marks SPF, DKIM and DMARC records that exist but differ from the
// values the panel recommends (the ones dns-sync would publish).
func checkDomainRecords(ctx context.Context, domain, selector string, recommended []dnssync.PlanEntry, lookupTXT txtLookupFunc) (*dnscheck.DomainCheckResult, error) {
	res, err := dnscheck.CheckDomain(ctx, domain, dnscheck.CheckOptions{Selector: selector})
	if err != nil {
		return nil, err
	}
	compareWithRecommended(ctx, res, recommended, lookupTXT)
	return res, nil
}

func compareWithRecommended(ctx context.Context, res *dnscheck.DomainCheckResult, recommended []dnssync.PlanEntry, lookupTXT txtLookupFunc) {
	for _, e := range recommended {
		if e.Action == dnssync.ActionSkip || e.Name == "" {
			continue
		}
		i := findResult(res.Results, string(e.Kind))
		if i < 0 || res.Results[i].Status != "ok" {
			// Missing or malformed records already carry a clearer message.
			continue
		}
		records, err := lookupTXT(ctx, e.Name)
		if err != nil {
			var dnsErr *net.DNSError
			if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
				continue // transient failure: keep the plain check result
			}
		}
		r := &res.Results[i]
		if action, _ := dnssync.DecideAction(e.Expected, pickTXT(records, e.Expected)); action == dnssync.ActionNoop {
			r.Message = "Matches the recommended value"
		} else {
			r.Status = "warning"
			r.Message = "Differs from the recommended value (see Recommended DNS Records below)"
		}
	}

	res.Summary = dnscheck.Summary{}
	for _, r := range res.Results {
		switch r.Status {
		case "ok":
			res.Summary.OK++
		case "warning":
			res.Summary.Warnings++
		case "error":
			res.Summary.Errors++
		case "not_found":
			res.Summary.NotFound++
		}
	}
}

// findResult returns the index of the check result of the given kind
// (SPF, DKIM, DMARC); dnscheck names them "SPF Record", "DKIM Record (...)".
func findResult(results []dnscheck.CheckResult, kind string) int {
	for i, r := range results {
		if strings.HasPrefix(r.Type, kind+" ") {
			return i
		}
	}
	return -1
}

// pickTXT returns the record matching expected, else the first one of the
// same family (v=spf1, v=DMARC1, v=DKIM1), else "".
func pickTXT(records []string, expected string) string {
	want := dnssync.NormalizeTXT(expected)
	for _, r := range records {
		if dnssync.NormalizeTXT(r) == want {
			return r
		}
	}
	family := txtFamily(want)
	for _, r := range records {
		if family != "" && txtFamily(dnssync.NormalizeTXT(r)) == family {
			return r
		}
	}
	return ""
}

func txtFamily(s string) string {
	tag, _, _ := strings.Cut(s, ";")
	tag, _, _ = strings.Cut(tag, " ")
	if !strings.HasPrefix(tag, "v=") {
		return ""
	}
	return strings.ToLower(tag)
}
