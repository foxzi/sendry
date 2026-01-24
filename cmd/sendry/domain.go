package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/config"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Domain management commands",
}

var domainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured domains",
	RunE:  runDomainList,
}

var domainShowCmd = &cobra.Command{
	Use:   "show <domain>",
	Short: "Show domain configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runDomainShow,
}

var domainStatsCmd = &cobra.Command{
	Use:   "stats <domain>",
	Short: "Show domain statistics",
	Args:  cobra.ExactArgs(1),
	RunE:  runDomainStats,
}

func init() {
	domainCmd.AddCommand(domainListCmd, domainShowCmd, domainStatsCmd)
	rootCmd.AddCommand(domainCmd)
}

func runDomainList(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	domains := cfg.GetAllDomains()

	if len(domains) == 0 {
		fmt.Println("No domains configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DOMAIN\tMODE\tDKIM\tRATE LIMIT")
	fmt.Fprintln(w, "------\t----\t----\t----------")

	for _, domain := range domains {
		dc := cfg.GetDomainConfig(domain)

		mode := "production"
		dkimStatus := "disabled"
		rateLimit := "-"

		if dc != nil {
			if dc.Mode != "" {
				mode = dc.Mode
			}
			if dc.DKIM != nil && dc.DKIM.Enabled {
				dkimStatus = fmt.Sprintf("enabled (%s)", dc.DKIM.Selector)
			}
			if dc.RateLimit != nil {
				rateLimit = fmt.Sprintf("%d/h, %d/d", dc.RateLimit.MessagesPerHour, dc.RateLimit.MessagesPerDay)
			}
		}

		// Check legacy DKIM config
		if dkimStatus == "disabled" && cfg.DKIM.Enabled && cfg.DKIM.Domain == domain {
			dkimStatus = fmt.Sprintf("enabled (%s)", cfg.DKIM.Selector)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", domain, mode, dkimStatus, rateLimit)
	}

	w.Flush()
	return nil
}

func runDomainShow(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	domain := args[0]
	dc := cfg.GetDomainConfig(domain)

	fmt.Printf("Domain: %s\n\n", domain)

	// Mode
	mode := "production"
	if dc != nil && dc.Mode != "" {
		mode = dc.Mode
	}
	fmt.Printf("Mode: %s\n", mode)

	if dc != nil {
		// Redirect/BCC settings
		if mode == "redirect" && len(dc.RedirectTo) > 0 {
			fmt.Printf("Redirect To: %v\n", dc.RedirectTo)
		}
		if mode == "bcc" && len(dc.BCCTo) > 0 {
			fmt.Printf("BCC To: %v\n", dc.BCCTo)
		}
	}

	fmt.Println()

	// DKIM
	fmt.Println("DKIM:")
	dkimEnabled, selector, keyFile := cfg.GetDKIMConfig(domain)
	if dkimEnabled {
		fmt.Printf("  Enabled: true\n")
		fmt.Printf("  Selector: %s\n", selector)
		fmt.Printf("  Key File: %s\n", keyFile)
	} else {
		fmt.Printf("  Enabled: false\n")
	}

	fmt.Println()

	// Rate Limits
	fmt.Println("Rate Limits:")
	if dc != nil && dc.RateLimit != nil {
		fmt.Printf("  Messages per hour: %d\n", dc.RateLimit.MessagesPerHour)
		fmt.Printf("  Messages per day: %d\n", dc.RateLimit.MessagesPerDay)
		fmt.Printf("  Recipients per message: %d\n", dc.RateLimit.RecipientsPerMessage)
	} else {
		fmt.Printf("  Using default limits\n")
	}

	// TLS
	if dc != nil && dc.TLS != nil && dc.TLS.CertFile != "" {
		fmt.Println()
		fmt.Println("TLS:")
		fmt.Printf("  Cert File: %s\n", dc.TLS.CertFile)
		fmt.Printf("  Key File: %s\n", dc.TLS.KeyFile)
	}

	return nil
}

func runDomainStats(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	domain := args[0]

	// Check if domain exists
	dc := cfg.GetDomainConfig(domain)
	found := dc != nil || cfg.SMTP.Domain == domain || (cfg.DKIM.Enabled && cfg.DKIM.Domain == domain)

	if !found {
		return fmt.Errorf("domain not found in configuration: %s", domain)
	}

	fmt.Printf("Statistics for domain: %s\n\n", domain)
	fmt.Println("Note: Detailed statistics require connecting to a running server.")
	fmt.Printf("Use the API endpoint GET /api/v1/domains/%s/stats for live statistics.\n", domain)

	// Show basic info from config
	fmt.Println()
	fmt.Println("Configuration:")

	mode := "production"
	if dc != nil && dc.Mode != "" {
		mode = dc.Mode
	}
	fmt.Printf("  Mode: %s\n", mode)

	dkimEnabled, _, _ := cfg.GetDKIMConfig(domain)
	fmt.Printf("  DKIM Enabled: %v\n", dkimEnabled)

	if dc != nil && dc.RateLimit != nil {
		fmt.Printf("  Rate Limit: %d/hour, %d/day\n",
			dc.RateLimit.MessagesPerHour,
			dc.RateLimit.MessagesPerDay)
	}

	return nil
}

// Helper for JSON output
func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
