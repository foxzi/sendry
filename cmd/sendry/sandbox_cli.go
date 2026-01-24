package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/sandbox"
)

var (
	sandboxListDomain string
	sandboxListLimit  int
	sandboxListFrom   string
	sandboxShowFormat string
	sandboxClearDays  int
)

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Sandbox mode management commands",
}

var sandboxStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show domain modes status",
	RunE:  runSandboxStatus,
}

var sandboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages in sandbox",
	RunE:  runSandboxList,
}

var sandboxShowCmd = &cobra.Command{
	Use:   "show <message_id>",
	Short: "Show sandbox message details",
	Args:  cobra.ExactArgs(1),
	RunE:  runSandboxShow,
}

var sandboxExportCmd = &cobra.Command{
	Use:   "export <message_id>",
	Short: "Export sandbox message to file",
	Args:  cobra.ExactArgs(1),
	RunE:  runSandboxExport,
}

var sandboxClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear sandbox messages",
	RunE:  runSandboxClear,
}

var sandboxStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show sandbox statistics",
	RunE:  runSandboxStats,
}

func init() {
	sandboxListCmd.Flags().StringVar(&sandboxListDomain, "domain", "", "Filter by domain")
	sandboxListCmd.Flags().IntVar(&sandboxListLimit, "limit", 50, "Maximum number of messages")
	sandboxListCmd.Flags().StringVar(&sandboxListFrom, "from", "", "Filter by sender")

	sandboxShowCmd.Flags().StringVar(&sandboxShowFormat, "format", "text", "Output format (text, raw, html)")

	sandboxClearCmd.Flags().StringVar(&sandboxListDomain, "domain", "", "Clear only for specific domain")
	sandboxClearCmd.Flags().IntVar(&sandboxClearDays, "older-than", 0, "Clear messages older than N days")

	sandboxCmd.AddCommand(sandboxStatusCmd, sandboxListCmd, sandboxShowCmd, sandboxExportCmd, sandboxClearCmd, sandboxStatsCmd)
	rootCmd.AddCommand(sandboxCmd)
}

func openSandboxStorage() (*sandbox.Storage, *bolt.DB, error) {
	if cfgFile == "" {
		return nil, nil, fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	db, err := bolt.Open(cfg.Storage.Path, 0600, &bolt.Options{
		Timeout:  5 * time.Second,
		ReadOnly: false,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	storage, err := sandbox.NewStorage(db)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("failed to create sandbox storage: %w", err)
	}

	return storage, db, nil
}

func runSandboxStatus(cmd *cobra.Command, args []string) error {
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
	fmt.Fprintln(w, "DOMAIN\tMODE\tREDIRECT/BCC")
	fmt.Fprintln(w, "------\t----\t------------")

	for _, domain := range domains {
		dc := cfg.GetDomainConfig(domain)

		mode := "production"
		extra := "-"

		if dc != nil {
			if dc.Mode != "" {
				mode = dc.Mode
			}
			if dc.Mode == "redirect" && len(dc.RedirectTo) > 0 {
				extra = strings.Join(dc.RedirectTo, ", ")
			}
			if dc.Mode == "bcc" && len(dc.BCCTo) > 0 {
				extra = strings.Join(dc.BCCTo, ", ")
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", domain, mode, extra)
	}

	w.Flush()
	return nil
}

func runSandboxList(cmd *cobra.Command, args []string) error {
	storage, db, err := openSandboxStorage()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	filter := sandbox.ListFilter{
		Limit: sandboxListLimit,
	}

	if sandboxListDomain != "" {
		filter.Domain = sandboxListDomain
	}

	messages, err := storage.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	// Filter by sender if specified
	if sandboxListFrom != "" {
		filtered := make([]*sandbox.Message, 0)
		for _, msg := range messages {
			if strings.Contains(strings.ToLower(msg.From), strings.ToLower(sandboxListFrom)) {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	if len(messages) == 0 {
		fmt.Println("No messages in sandbox")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tMODE\tFROM\tTO\tSUBJECT\tCAPTURED")
	fmt.Fprintln(w, "--\t----\t----\t--\t-------\t--------")

	for _, msg := range messages {
		to := strings.Join(msg.To, ", ")
		if len(to) > 30 {
			to = to[:27] + "..."
		}

		subject := msg.Subject
		if len(subject) > 30 {
			subject = subject[:27] + "..."
		}

		captured := msg.CapturedAt.Format("2006-01-02 15:04")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			truncateID(msg.ID),
			msg.Mode,
			msg.From,
			to,
			subject,
			captured,
		)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d messages\n", len(messages))

	return nil
}

func runSandboxShow(cmd *cobra.Command, args []string) error {
	storage, db, err := openSandboxStorage()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	id := args[0]

	msg, err := storage.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}

	if msg == nil {
		return fmt.Errorf("message not found: %s", id)
	}

	switch sandboxShowFormat {
	case "raw":
		// Output raw email data
		fmt.Println(string(msg.Data))
		return nil

	case "html":
		// TODO: Extract and display HTML part
		fmt.Println("HTML view not yet implemented, showing raw data:")
		fmt.Println(string(msg.Data))
		return nil

	default:
		// Text format
		fmt.Printf("Message: %s\n\n", msg.ID)
		fmt.Printf("Mode:       %s\n", msg.Mode)
		fmt.Printf("Domain:     %s\n", msg.Domain)
		fmt.Printf("From:       %s\n", msg.From)
		fmt.Printf("To:         %s\n", strings.Join(msg.To, ", "))

		if len(msg.OriginalTo) > 0 {
			fmt.Printf("Original To: %s\n", strings.Join(msg.OriginalTo, ", "))
		}

		fmt.Printf("Subject:    %s\n", msg.Subject)
		fmt.Printf("Captured:   %s\n", msg.CapturedAt.Format(time.RFC3339))

		if msg.ClientIP != "" {
			fmt.Printf("Client IP:  %s\n", msg.ClientIP)
		}

		if msg.SimulatedErr != "" {
			fmt.Printf("\nSimulated Error: %s\n", msg.SimulatedErr)
		}

		// Show message data preview
		if len(msg.Data) > 0 {
			fmt.Println("\nMessage Data:")
			fmt.Println("---")
			preview := string(msg.Data)
			if len(preview) > 1000 {
				preview = preview[:1000] + "\n... (truncated, use --format raw for full message)"
			}
			fmt.Println(preview)
			fmt.Println("---")
		}

		return nil
	}
}

func runSandboxExport(cmd *cobra.Command, args []string) error {
	storage, db, err := openSandboxStorage()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	id := args[0]

	msg, err := storage.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}

	if msg == nil {
		return fmt.Errorf("message not found: %s", id)
	}

	// Create output filename
	filename := fmt.Sprintf("%s.eml", id)

	if err := os.WriteFile(filename, msg.Data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Message exported to: %s\n", filename)
	return nil
}

func runSandboxClear(cmd *cobra.Command, args []string) error {
	storage, db, err := openSandboxStorage()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	var olderThan time.Duration
	if sandboxClearDays > 0 {
		olderThan = time.Duration(sandboxClearDays) * 24 * time.Hour
	}

	count, err := storage.Clear(ctx, sandboxListDomain, olderThan)
	if err != nil {
		return fmt.Errorf("failed to clear sandbox: %w", err)
	}

	if sandboxListDomain != "" {
		fmt.Printf("Cleared %d messages from sandbox for domain %s\n", count, sandboxListDomain)
	} else {
		fmt.Printf("Cleared %d messages from sandbox\n", count)
	}

	return nil
}

func runSandboxStats(cmd *cobra.Command, args []string) error {
	storage, db, err := openSandboxStorage()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	stats, err := storage.Stats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get sandbox stats: %w", err)
	}

	fmt.Println("Sandbox Statistics")
	fmt.Println("==================")
	fmt.Printf("Total Messages: %d\n", stats.Total)
	fmt.Printf("Total Size:     %d bytes\n", stats.TotalSize)

	if len(stats.ByMode) > 0 {
		fmt.Println("\nBy Mode:")
		for mode, count := range stats.ByMode {
			fmt.Printf("  %s: %d\n", mode, count)
		}
	}

	if len(stats.ByDomain) > 0 {
		fmt.Println("\nBy Domain:")
		for domain, count := range stats.ByDomain {
			fmt.Printf("  %s: %d\n", domain, count)
		}
	}

	if !stats.OldestAt.IsZero() {
		fmt.Printf("\nOldest Message: %s\n", stats.OldestAt.Format(time.RFC3339))
	}
	if !stats.NewestAt.IsZero() {
		fmt.Printf("Newest Message: %s\n", stats.NewestAt.Format(time.RFC3339))
	}

	return nil
}
