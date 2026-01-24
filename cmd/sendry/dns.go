package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	dnsCheckMX     bool
	dnsCheckSPF    bool
	dnsCheckDKIM   bool
	dnsCheckDMARC  bool
	dnsCheckMTASTS bool
	dnsCheckPTR    bool
	dnsCheckAll    bool
	dnsSelector    string
	dnsFormat      string
)

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: "DNS management commands",
}

var dnsCheckCmd = &cobra.Command{
	Use:   "check <domain>",
	Short: "Check DNS records for a domain",
	Long:  `Check MX, SPF, DKIM, DMARC, and other DNS records for a domain.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDNSCheck,
}

func init() {
	dnsCheckCmd.Flags().BoolVar(&dnsCheckMX, "mx", false, "Check MX records")
	dnsCheckCmd.Flags().BoolVar(&dnsCheckSPF, "spf", false, "Check SPF record")
	dnsCheckCmd.Flags().BoolVar(&dnsCheckDKIM, "dkim", false, "Check DKIM records")
	dnsCheckCmd.Flags().BoolVar(&dnsCheckDMARC, "dmarc", false, "Check DMARC record")
	dnsCheckCmd.Flags().BoolVar(&dnsCheckMTASTS, "mta-sts", false, "Check MTA-STS record")
	dnsCheckCmd.Flags().BoolVar(&dnsCheckPTR, "ptr", false, "Check PTR record")
	dnsCheckCmd.Flags().BoolVar(&dnsCheckAll, "all", false, "Check all records")
	dnsCheckCmd.Flags().StringVar(&dnsSelector, "selector", "sendry", "DKIM selector to check")
	dnsCheckCmd.Flags().StringVar(&dnsFormat, "format", "table", "Output format (table, json, yaml)")

	dnsCmd.AddCommand(dnsCheckCmd)
	rootCmd.AddCommand(dnsCmd)
}

type dnsCheckResult struct {
	Type    string `json:"type"`
	Status  string `json:"status"` // ok, warning, error, not_found
	Value   string `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
}

func runDNSCheck(cmd *cobra.Command, args []string) error {
	domain := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// If no specific checks requested, check all
	checkAll := dnsCheckAll || (!dnsCheckMX && !dnsCheckSPF && !dnsCheckDKIM && !dnsCheckDMARC && !dnsCheckMTASTS && !dnsCheckPTR)

	fmt.Printf("Checking DNS records for: %s\n\n", domain)

	results := make([]dnsCheckResult, 0)

	if checkAll || dnsCheckMX {
		result := checkMXRecords(ctx, domain)
		results = append(results, result)
		printResult(result)
	}

	if checkAll || dnsCheckSPF {
		result := checkSPFRecord(ctx, domain)
		results = append(results, result)
		printResult(result)
	}

	if checkAll || dnsCheckDKIM {
		result := checkDKIMRecord(ctx, domain, dnsSelector)
		results = append(results, result)
		printResult(result)
	}

	if checkAll || dnsCheckDMARC {
		result := checkDMARCRecord(ctx, domain)
		results = append(results, result)
		printResult(result)
	}

	if checkAll || dnsCheckMTASTS {
		result := checkMTASTSRecord(ctx, domain)
		results = append(results, result)
		printResult(result)
	}

	if checkAll || dnsCheckPTR {
		result := checkPTRRecord(ctx, domain)
		results = append(results, result)
		printResult(result)
	}

	// Summary
	fmt.Println()
	okCount := 0
	warnCount := 0
	errCount := 0
	for _, r := range results {
		switch r.Status {
		case "ok":
			okCount++
		case "warning":
			warnCount++
		case "error", "not_found":
			errCount++
		}
	}
	fmt.Printf("Summary: %d OK, %d warnings, %d errors\n", okCount, warnCount, errCount)

	return nil
}

func printResult(r dnsCheckResult) {
	statusIcon := "?"
	switch r.Status {
	case "ok":
		statusIcon = "[OK]"
	case "warning":
		statusIcon = "[WARN]"
	case "error":
		statusIcon = "[ERR]"
	case "not_found":
		statusIcon = "[N/A]"
	}

	fmt.Printf("%s %s\n", statusIcon, r.Type)
	if r.Value != "" {
		fmt.Printf("    Value: %s\n", r.Value)
	}
	if r.Message != "" {
		fmt.Printf("    %s\n", r.Message)
	}
	fmt.Println()
}

func checkMXRecords(ctx context.Context, domain string) dnsCheckResult {
	result := dnsCheckResult{Type: "MX Records"}

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

func checkSPFRecord(ctx context.Context, domain string) dnsCheckResult {
	result := dnsCheckResult{Type: "SPF Record"}

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

func checkDKIMRecord(ctx context.Context, domain, selector string) dnsCheckResult {
	result := dnsCheckResult{Type: fmt.Sprintf("DKIM Record (%s._domainkey)", selector)}

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

func checkDMARCRecord(ctx context.Context, domain string) dnsCheckResult {
	result := dnsCheckResult{Type: "DMARC Record"}

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

func checkMTASTSRecord(ctx context.Context, domain string) dnsCheckResult {
	result := dnsCheckResult{Type: "MTA-STS Record"}

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

func checkPTRRecord(ctx context.Context, domain string) dnsCheckResult {
	result := dnsCheckResult{Type: "PTR Record (reverse DNS)"}

	// First resolve the domain to IP
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", domain)
	if err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("Could not resolve domain to IP: %v", err)
		return result
	}

	if len(ips) == 0 {
		result.Status = "error"
		result.Message = "No A record found for domain"
		return result
	}

	ip := ips[0]

	// Lookup PTR record
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			result.Status = "warning"
			result.Message = fmt.Sprintf("No PTR record for %s (recommended for mail servers)", ip.String())
			return result
		}
		result.Status = "error"
		result.Message = fmt.Sprintf("PTR lookup failed: %v", err)
		return result
	}

	if len(names) == 0 {
		result.Status = "warning"
		result.Message = fmt.Sprintf("No PTR record for %s", ip.String())
		return result
	}

	result.Status = "ok"
	result.Value = fmt.Sprintf("%s -> %s", ip.String(), strings.Join(names, ", "))

	// Check if PTR matches domain
	ptrMatches := false
	for _, name := range names {
		name = strings.TrimSuffix(name, ".")
		if strings.EqualFold(name, domain) || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(domain)) {
			ptrMatches = true
			break
		}
	}

	if ptrMatches {
		result.Message = "PTR record matches domain"
	} else {
		result.Status = "warning"
		result.Message = "PTR record does not match domain (may affect deliverability)"
	}

	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
