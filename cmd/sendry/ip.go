package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	ipCheckAll     bool
	ipCheckTimeout int
)

// Popular DNSBL services
var defaultDNSBLs = []dnsblInfo{
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
	{Name: "Spamrats DYNA", Zone: "dyna.spamrats.com", Description: "SpamRATS Dynamic IP"},
	{Name: "Spamrats NOPTR", Zone: "noptr.spamrats.com", Description: "SpamRATS No PTR"},
	{Name: "Spamrats SPAM", Zone: "spam.spamrats.com", Description: "SpamRATS Spam"},
	{Name: "PSBL", Zone: "psbl.surriel.com", Description: "Passive Spam Block List"},
	{Name: "Mailspike BL", Zone: "bl.mailspike.net", Description: "Mailspike Blocklist"},
	{Name: "JustSpam", Zone: "dnsbl.justspam.org", Description: "JustSpam.org DNSBL"},
	{Name: "0Spam", Zone: "bl.0spam.org", Description: "0spam Project Blocklist"},
}

type dnsblInfo struct {
	Name        string
	Zone        string
	Description string
}

type dnsblResult struct {
	DNSBL       dnsblInfo
	Listed      bool
	ReturnCodes []string
	Error       error
}

var ipCmd = &cobra.Command{
	Use:   "ip",
	Short: "IP address management and diagnostics",
}

var ipCheckCmd = &cobra.Command{
	Use:   "check <ip>",
	Short: "Check IP address against DNS blacklists (DNSBL)",
	Long: `Check if an IP address is listed in popular DNS-based blackhole lists.

This helps diagnose email deliverability issues caused by IP reputation.

Examples:
  sendry ip check 1.2.3.4
  sendry ip check 192.168.1.1 --timeout 10`,
	Args: cobra.ExactArgs(1),
	RunE: runIPCheck,
}

var ipListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available DNSBL services",
	Long:  `Show all DNS blacklists that are checked by the 'ip check' command.`,
	RunE:  runIPList,
}

func init() {
	ipCheckCmd.Flags().IntVar(&ipCheckTimeout, "timeout", 30, "Timeout in seconds for all checks")

	ipCmd.AddCommand(ipCheckCmd)
	ipCmd.AddCommand(ipListCmd)
	rootCmd.AddCommand(ipCmd)
}

func runIPCheck(cmd *cobra.Command, args []string) error {
	ipStr := args[0]

	// Parse and validate IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// Only IPv4 is commonly supported by DNSBLs
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("only IPv4 addresses are supported for DNSBL checks")
	}

	// Reverse the IP for DNSBL query
	reversed := reverseIP(ip4)

	fmt.Printf("Checking IP %s against %d DNS blacklists...\n\n", ipStr, len(defaultDNSBLs))

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ipCheckTimeout)*time.Second)
	defer cancel()

	// Check all DNSBLs concurrently
	results := checkDNSBLs(ctx, reversed)

	// Count results
	listedCount := 0
	cleanCount := 0
	errorCount := 0

	// Print results
	fmt.Printf("%-20s %-35s %s\n", "STATUS", "BLACKLIST", "DETAILS")
	fmt.Println(strings.Repeat("-", 80))

	for _, r := range results {
		if r.Error != nil {
			errorCount++
			fmt.Printf("%-20s %-35s %s\n", "[ERROR]", r.DNSBL.Name, r.Error.Error())
		} else if r.Listed {
			listedCount++
			codes := ""
			if len(r.ReturnCodes) > 0 {
				codes = fmt.Sprintf("Return codes: %s", strings.Join(r.ReturnCodes, ", "))
			}
			fmt.Printf("%-20s %-35s %s\n", "[LISTED]", r.DNSBL.Name, codes)
		} else {
			cleanCount++
			fmt.Printf("%-20s %-35s\n", "[CLEAN]", r.DNSBL.Name)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Summary for %s:\n", ipStr)
	fmt.Printf("  Clean:  %d blacklists\n", cleanCount)
	fmt.Printf("  Listed: %d blacklists\n", listedCount)
	if errorCount > 0 {
		fmt.Printf("  Errors: %d blacklists\n", errorCount)
	}
	fmt.Println()

	if listedCount > 0 {
		fmt.Println("Warning: IP is listed in one or more blacklists.")
		fmt.Println("This may affect email deliverability. Consider:")
		fmt.Println("  1. Checking the blacklist websites for delisting procedures")
		fmt.Println("  2. Investigating the cause of the listing")
		fmt.Println("  3. Using a different IP address for sending")
	} else if cleanCount == len(defaultDNSBLs) {
		fmt.Println("IP address is clean - not listed in any checked blacklists.")
	}

	return nil
}

func runIPList(cmd *cobra.Command, args []string) error {
	fmt.Printf("Available DNS Blacklists (%d total):\n\n", len(defaultDNSBLs))
	fmt.Printf("%-20s %-30s %s\n", "NAME", "ZONE", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 90))

	for _, bl := range defaultDNSBLs {
		fmt.Printf("%-20s %-30s %s\n", bl.Name, bl.Zone, bl.Description)
	}

	return nil
}

func reverseIP(ip net.IP) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", ip4[3], ip4[2], ip4[1], ip4[0])
}

func checkDNSBLs(ctx context.Context, reversedIP string) []dnsblResult {
	results := make([]dnsblResult, len(defaultDNSBLs))
	var wg sync.WaitGroup

	for i, bl := range defaultDNSBLs {
		wg.Add(1)
		go func(idx int, dnsbl dnsblInfo) {
			defer wg.Done()
			results[idx] = checkSingleDNSBL(ctx, reversedIP, dnsbl)
		}(i, bl)
	}

	wg.Wait()
	return results
}

func checkSingleDNSBL(ctx context.Context, reversedIP string, dnsbl dnsblInfo) dnsblResult {
	result := dnsblResult{
		DNSBL:  dnsbl,
		Listed: false,
	}

	query := reversedIP + "." + dnsbl.Zone

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", query)
	if err != nil {
		// DNS NXDOMAIN means not listed (this is good)
		if dnsErr, ok := err.(*net.DNSError); ok {
			if dnsErr.IsNotFound || strings.Contains(dnsErr.Error(), "no such host") {
				// Not listed - this is the expected "clean" result
				return result
			}
			if dnsErr.IsTimeout {
				result.Error = fmt.Errorf("timeout")
				return result
			}
		}
		// Other errors
		result.Error = fmt.Errorf("lookup error: %v", err)
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
