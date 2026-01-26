// Package dnscheck provides DNS record validation functions.
package dnscheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
)

// Domain validation errors
var (
	ErrInvalidDomain    = errors.New("invalid domain name")
	ErrInvalidIP        = errors.New("invalid IP address")
	ErrIPv6NotSupported = errors.New("IPv6 addresses are not supported for DNSBL checks")
)

// domainRegex validates domain name format (RFC 1035)
var domainRegex = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// ValidateDomain checks if domain name is valid
func ValidateDomain(domain string) error {
	if domain == "" {
		return ErrInvalidDomain
	}
	if len(domain) > 253 {
		return ErrInvalidDomain
	}
	if !domainRegex.MatchString(domain) {
		return ErrInvalidDomain
	}
	return nil
}

// ValidateSelector checks if DKIM selector is valid
func ValidateSelector(selector string) error {
	if selector == "" {
		return nil // Empty selector will use default
	}
	if len(selector) > 63 {
		return errors.New("selector too long")
	}
	// Selector follows same rules as domain label
	if !regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`).MatchString(selector) {
		return errors.New("invalid selector format")
	}
	return nil
}

// CheckResult represents a single DNS check result
type CheckResult struct {
	Type    string `json:"type"`
	Status  string `json:"status"` // ok, warning, error, not_found
	Value   string `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
}

// DomainCheckResult contains all DNS check results for a domain
type DomainCheckResult struct {
	Domain  string        `json:"domain"`
	Results []CheckResult `json:"results"`
	Summary Summary       `json:"summary"`
}

// Summary contains check statistics
type Summary struct {
	OK       int `json:"ok"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
	NotFound int `json:"not_found"`
}

// CheckOptions specifies which checks to perform
type CheckOptions struct {
	MX       bool   `json:"mx"`
	SPF      bool   `json:"spf"`
	DKIM     bool   `json:"dkim"`
	DMARC    bool   `json:"dmarc"`
	MTASTS   bool   `json:"mta_sts"`
	Selector string `json:"selector"` // DKIM selector
}

// CheckDomain performs DNS checks for a domain
func CheckDomain(ctx context.Context, domain string, opts CheckOptions) (*DomainCheckResult, error) {
	// Validate domain
	if err := ValidateDomain(domain); err != nil {
		return nil, err
	}

	// Validate selector
	if err := ValidateSelector(opts.Selector); err != nil {
		return nil, err
	}

	result := &DomainCheckResult{
		Domain:  domain,
		Results: make([]CheckResult, 0),
	}

	// Default selector
	if opts.Selector == "" {
		opts.Selector = "sendry"
	}

	// If no specific checks requested, check all
	checkAll := !opts.MX && !opts.SPF && !opts.DKIM && !opts.DMARC && !opts.MTASTS

	if checkAll || opts.MX {
		result.Results = append(result.Results, CheckMX(ctx, domain))
	}

	if checkAll || opts.SPF {
		result.Results = append(result.Results, CheckSPF(ctx, domain))
	}

	if checkAll || opts.DKIM {
		result.Results = append(result.Results, CheckDKIM(ctx, domain, opts.Selector))
	}

	if checkAll || opts.DMARC {
		result.Results = append(result.Results, CheckDMARC(ctx, domain))
	}

	if checkAll || opts.MTASTS {
		result.Results = append(result.Results, CheckMTASTS(ctx, domain))
	}

	// Calculate summary
	for _, r := range result.Results {
		switch r.Status {
		case "ok":
			result.Summary.OK++
		case "warning":
			result.Summary.Warnings++
		case "error":
			result.Summary.Errors++
		case "not_found":
			result.Summary.NotFound++
		}
	}

	return result, nil
}

// CheckMX checks MX records for a domain
func CheckMX(ctx context.Context, domain string) CheckResult {
	result := CheckResult{Type: "MX Records"}

	mxRecords, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			result.Status = "not_found"
			result.Message = "No MX records found"
			return result
		}
		result.Status = "error"
		result.Message = fmt.Sprintf("Lookup failed: %v", err)
		return result
	}

	if len(mxRecords) == 0 {
		result.Status = "not_found"
		result.Message = "No MX records found"
		return result
	}

	var values []string
	for _, mx := range mxRecords {
		values = append(values, fmt.Sprintf("%s (priority %d)", mx.Host, mx.Pref))
	}
	result.Status = "ok"
	result.Value = strings.Join(values, ", ")
	result.Message = fmt.Sprintf("%d MX record(s) found", len(mxRecords))

	return result
}

// CheckSPF checks SPF record for a domain
func CheckSPF(ctx context.Context, domain string) CheckResult {
	result := CheckResult{Type: "SPF Record"}

	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, domain)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			result.Status = "not_found"
			result.Message = "No SPF record found (recommended to add)"
			return result
		}
		result.Status = "error"
		result.Message = fmt.Sprintf("Lookup failed: %v", err)
		return result
	}

	for _, txt := range txtRecords {
		if strings.HasPrefix(txt, "v=spf1") {
			result.Status = "ok"
			result.Value = txt

			// Basic validation
			if strings.Contains(txt, "+all") {
				result.Status = "warning"
				result.Message = "SPF uses +all (allows any sender) - consider using ~all or -all"
			} else if strings.Contains(txt, "-all") {
				result.Message = "SPF configured with strict policy (-all)"
			} else if strings.Contains(txt, "~all") {
				result.Message = "SPF configured with soft fail (~all)"
			}

			return result
		}
	}

	result.Status = "not_found"
	result.Message = "No SPF record found (recommended to add)"
	return result
}

// CheckDKIM checks DKIM record for a domain
func CheckDKIM(ctx context.Context, domain, selector string) CheckResult {
	result := CheckResult{Type: fmt.Sprintf("DKIM Record (%s._domainkey)", selector)}

	dkimDomain := fmt.Sprintf("%s._domainkey.%s", selector, domain)

	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, dkimDomain)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			result.Status = "not_found"
			result.Message = fmt.Sprintf("No DKIM record found for selector '%s'", selector)
			return result
		}
		result.Status = "error"
		result.Message = fmt.Sprintf("Lookup failed: %v", err)
		return result
	}

	// Join potentially split TXT record
	fullRecord := strings.Join(txtRecords, "")

	if strings.Contains(fullRecord, "v=DKIM1") {
		result.Status = "ok"
		result.Value = truncateString(fullRecord, 100)

		// Check key type
		if strings.Contains(fullRecord, "k=rsa") {
			result.Message = "DKIM configured with RSA key"
		} else if strings.Contains(fullRecord, "k=ed25519") {
			result.Message = "DKIM configured with Ed25519 key"
		}

		// Check for public key
		if !strings.Contains(fullRecord, "p=") {
			result.Status = "warning"
			result.Message = "DKIM record missing public key (p=)"
		}

		return result
	}

	result.Status = "warning"
	result.Value = truncateString(fullRecord, 100)
	result.Message = "TXT record found but doesn't appear to be a valid DKIM record"
	return result
}

// CheckDMARC checks DMARC record for a domain
func CheckDMARC(ctx context.Context, domain string) CheckResult {
	result := CheckResult{Type: "DMARC Record"}

	dmarcDomain := "_dmarc." + domain

	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, dmarcDomain)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			result.Status = "not_found"
			result.Message = "No DMARC record found (recommended to add)"
			return result
		}
		result.Status = "error"
		result.Message = fmt.Sprintf("Lookup failed: %v", err)
		return result
	}

	fullRecord := strings.Join(txtRecords, "")

	if strings.HasPrefix(fullRecord, "v=DMARC1") {
		result.Status = "ok"
		result.Value = fullRecord

		// Parse policy
		if strings.Contains(fullRecord, "p=reject") {
			result.Message = "DMARC configured with reject policy (strict)"
		} else if strings.Contains(fullRecord, "p=quarantine") {
			result.Message = "DMARC configured with quarantine policy"
		} else if strings.Contains(fullRecord, "p=none") {
			result.Status = "warning"
			result.Message = "DMARC configured with none policy (monitoring only)"
		}

		return result
	}

	result.Status = "warning"
	result.Value = fullRecord
	result.Message = "TXT record found but doesn't appear to be a valid DMARC record"
	return result
}

// CheckMTASTS checks MTA-STS record for a domain
func CheckMTASTS(ctx context.Context, domain string) CheckResult {
	result := CheckResult{Type: "MTA-STS Record"}

	mtastsDomain := "_mta-sts." + domain

	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, mtastsDomain)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			result.Status = "not_found"
			result.Message = "No MTA-STS record found (optional)"
			return result
		}
		result.Status = "error"
		result.Message = fmt.Sprintf("Lookup failed: %v", err)
		return result
	}

	fullRecord := strings.Join(txtRecords, "")

	if strings.HasPrefix(fullRecord, "v=STSv1") {
		result.Status = "ok"
		result.Value = fullRecord
		result.Message = "MTA-STS configured"
		return result
	}

	result.Status = "warning"
	result.Value = fullRecord
	result.Message = "TXT record found but doesn't appear to be a valid MTA-STS record"
	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// DNSBL checking

// DNSBLInfo represents a DNS blacklist service
type DNSBLInfo struct {
	Name        string `json:"name"`
	Zone        string `json:"zone"`
	Description string `json:"description"`
}

// DNSBLResult represents a single DNSBL check result
type DNSBLResult struct {
	DNSBL       DNSBLInfo `json:"dnsbl"`
	Listed      bool      `json:"listed"`
	ReturnCodes []string  `json:"return_codes,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// IPCheckResult contains all DNSBL check results for an IP
type IPCheckResult struct {
	IP      string        `json:"ip"`
	Results []DNSBLResult `json:"results"`
	Summary IPSummary     `json:"summary"`
}

// IPSummary contains DNSBL check statistics
type IPSummary struct {
	Clean  int `json:"clean"`
	Listed int `json:"listed"`
	Errors int `json:"errors"`
}

// DefaultDNSBLs is the list of popular DNSBL services
var DefaultDNSBLs = []DNSBLInfo{
	{Name: "Spamhaus ZEN", Zone: "zen.spamhaus.org", Description: "Combined Spamhaus blocklist (SBL, XBL, PBL)"},
	{Name: "Spamhaus SBL", Zone: "sbl.spamhaus.org", Description: "Spamhaus Block List"},
	{Name: "Spamhaus XBL", Zone: "xbl.spamhaus.org", Description: "Exploits Block List"},
	{Name: "Spamhaus PBL", Zone: "pbl.spamhaus.org", Description: "Policy Block List"},
	{Name: "Barracuda", Zone: "b.barracudacentral.org", Description: "Barracuda Reputation Block List"},
	{Name: "SpamCop", Zone: "bl.spamcop.net", Description: "SpamCop Blocking List"},
	{Name: "SORBS DNSBL", Zone: "dnsbl.sorbs.net", Description: "SORBS aggregate zone"},
	{Name: "SORBS Spam", Zone: "spam.dnsbl.sorbs.net", Description: "SORBS spam sources"},
	{Name: "UCEPROTECT L1", Zone: "dnsbl-1.uceprotect.net", Description: "UCEPROTECT Level 1"},
	{Name: "UCEPROTECT L2", Zone: "dnsbl-2.uceprotect.net", Description: "UCEPROTECT Level 2"},
	{Name: "UCEPROTECT L3", Zone: "dnsbl-3.uceprotect.net", Description: "UCEPROTECT Level 3"},
	{Name: "PSBL", Zone: "psbl.surriel.com", Description: "Passive Spam Block List"},
	{Name: "Mailspike BL", Zone: "bl.mailspike.net", Description: "Mailspike Blocklist"},
	{Name: "JustSpam", Zone: "dnsbl.justspam.org", Description: "JustSpam.org DNSBL"},
	{Name: "0Spam", Zone: "bl.0spam.org", Description: "0spam Project Blocklist"},
}

// CheckIP checks an IP address against DNSBL services
func CheckIP(ctx context.Context, ipStr string) (*IPCheckResult, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, ErrInvalidIP
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return nil, ErrIPv6NotSupported
	}

	reversed := reverseIP(ip4)

	result := &IPCheckResult{
		IP:      ipStr,
		Results: make([]DNSBLResult, len(DefaultDNSBLs)),
	}

	var wg sync.WaitGroup
	for i, bl := range DefaultDNSBLs {
		wg.Add(1)
		go func(idx int, dnsbl DNSBLInfo) {
			defer wg.Done()
			result.Results[idx] = checkSingleDNSBL(ctx, reversed, dnsbl)
		}(i, bl)
	}
	wg.Wait()

	// Calculate summary
	for _, r := range result.Results {
		if r.Error != "" {
			result.Summary.Errors++
		} else if r.Listed {
			result.Summary.Listed++
		} else {
			result.Summary.Clean++
		}
	}

	return result, nil
}

func reverseIP(ip net.IP) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", ip4[3], ip4[2], ip4[1], ip4[0])
}

func checkSingleDNSBL(ctx context.Context, reversedIP string, dnsbl DNSBLInfo) DNSBLResult {
	result := DNSBLResult{
		DNSBL:  dnsbl,
		Listed: false,
	}

	query := reversedIP + "." + dnsbl.Zone

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", query)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok {
			if dnsErr.IsNotFound || strings.Contains(dnsErr.Error(), "no such host") {
				// Not listed - this is the expected "clean" result
				return result
			}
			if dnsErr.IsTimeout {
				result.Error = "timeout"
				return result
			}
		}
		result.Error = fmt.Sprintf("lookup error: %v", err)
		return result
	}

	// If we got IP addresses back, the IP is listed
	if len(ips) > 0 {
		result.Listed = true
		for _, ip := range ips {
			result.ReturnCodes = append(result.ReturnCodes, ip.String())
		}
	}

	return result
}

// ListDNSBLs returns the list of available DNSBL services
func ListDNSBLs() []DNSBLInfo {
	return DefaultDNSBLs
}
