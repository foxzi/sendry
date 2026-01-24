package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/config"
)

var ratelimitCmd = &cobra.Command{
	Use:   "ratelimit",
	Short: "Rate limit management commands",
}

var ratelimitShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current rate limits",
	RunE:  runRatelimitShow,
}

func init() {
	ratelimitCmd.AddCommand(ratelimitShowCmd)
	rootCmd.AddCommand(ratelimitCmd)
}

func runRatelimitShow(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	rl := cfg.RateLimit

	fmt.Println("Rate Limiting Configuration")
	fmt.Println("===========================")
	fmt.Printf("Enabled: %v\n\n", rl.Enabled)

	if !rl.Enabled {
		fmt.Println("Rate limiting is disabled")
		return nil
	}

	// Global limits
	fmt.Println("Global Limits:")
	if rl.Global != nil {
		fmt.Printf("  Messages per hour: %d\n", rl.Global.MessagesPerHour)
		fmt.Printf("  Messages per day:  %d\n", rl.Global.MessagesPerDay)
	} else {
		fmt.Println("  Not configured")
	}
	fmt.Println()

	// Default limits table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "LEVEL\tMESSAGES/HOUR\tMESSAGES/DAY")
	fmt.Fprintln(w, "-----\t-------------\t------------")

	if rl.DefaultDomain != nil {
		fmt.Fprintf(w, "Per Domain\t%d\t%d\n", rl.DefaultDomain.MessagesPerHour, rl.DefaultDomain.MessagesPerDay)
	} else {
		fmt.Fprintln(w, "Per Domain\t-\t-")
	}

	if rl.DefaultSender != nil {
		fmt.Fprintf(w, "Per Sender\t%d\t%d\n", rl.DefaultSender.MessagesPerHour, rl.DefaultSender.MessagesPerDay)
	} else {
		fmt.Fprintln(w, "Per Sender\t-\t-")
	}

	if rl.DefaultIP != nil {
		fmt.Fprintf(w, "Per IP\t%d\t%d\n", rl.DefaultIP.MessagesPerHour, rl.DefaultIP.MessagesPerDay)
	} else {
		fmt.Fprintln(w, "Per IP\t-\t-")
	}

	if rl.DefaultAPIKey != nil {
		fmt.Fprintf(w, "Per API Key\t%d\t%d\n", rl.DefaultAPIKey.MessagesPerHour, rl.DefaultAPIKey.MessagesPerDay)
	} else {
		fmt.Fprintln(w, "Per API Key\t-\t-")
	}

	w.Flush()

	// Per-domain overrides
	fmt.Println("\nPer-Domain Overrides:")
	hasOverrides := false

	for domain, dc := range cfg.Domains {
		if dc.RateLimit != nil {
			if !hasOverrides {
				fmt.Println()
				w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "DOMAIN\tMESSAGES/HOUR\tMESSAGES/DAY\tRECIPIENTS/MSG")
				fmt.Fprintln(w, "------\t-------------\t------------\t--------------")
				hasOverrides = true
			}
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\n",
				domain,
				dc.RateLimit.MessagesPerHour,
				dc.RateLimit.MessagesPerDay,
				dc.RateLimit.RecipientsPerMessage,
			)
		}
	}

	if hasOverrides {
		w.Flush()
	} else {
		fmt.Println("  None configured")
	}

	fmt.Println()
	fmt.Println("Note: To view current usage, use the API endpoint GET /api/v1/ratelimits")

	return nil
}
