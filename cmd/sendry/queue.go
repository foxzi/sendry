package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
)

var (
	queueListStatus string
	queueListLimit  int
	queueListDomain string
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Queue management commands",
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages in the queue",
	RunE:  runQueueList,
}

var queueShowCmd = &cobra.Command{
	Use:   "show <message_id>",
	Short: "Show message details",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueShow,
}

var queueStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show queue statistics",
	RunE:  runQueueStats,
}

var queueRetryCmd = &cobra.Command{
	Use:   "retry <message_id>",
	Short: "Retry a failed message",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueRetry,
}

var queueDeleteCmd = &cobra.Command{
	Use:   "delete <message_id>",
	Short: "Delete a message from queue",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueDelete,
}

func init() {
	queueListCmd.Flags().StringVar(&queueListStatus, "status", "", "Filter by status (pending, sending, delivered, failed, deferred)")
	queueListCmd.Flags().IntVar(&queueListLimit, "limit", 50, "Maximum number of messages to show")
	queueListCmd.Flags().StringVar(&queueListDomain, "domain", "", "Filter by domain")

	queueCmd.AddCommand(queueListCmd, queueShowCmd, queueStatsCmd, queueRetryCmd, queueDeleteCmd)
	rootCmd.AddCommand(queueCmd)
}

func openQueueStorage() (*queue.BoltStorage, error) {
	if cfgFile == "" {
		return nil, fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	storage, err := queue.NewBoltStorage(cfg.Storage.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open queue storage: %w", err)
	}

	return storage, nil
}

func runQueueList(cmd *cobra.Command, args []string) error {
	storage, err := openQueueStorage()
	if err != nil {
		return err
	}
	defer storage.Close()

	ctx := context.Background()

	filter := queue.ListFilter{
		Limit: queueListLimit,
	}

	if queueListStatus != "" {
		filter.Status = queue.MessageStatus(queueListStatus)
	}

	messages, err := storage.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	if len(messages) == 0 {
		fmt.Println("Queue is empty")
		return nil
	}

	// Filter by domain if specified
	if queueListDomain != "" {
		filtered := make([]*queue.Message, 0)
		for _, msg := range messages {
			if strings.Contains(msg.From, "@"+queueListDomain) {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tFROM\tTO\tCREATED\tRETRIES")
	fmt.Fprintln(w, "--\t------\t----\t--\t-------\t-------")

	for _, msg := range messages {
		to := strings.Join(msg.To, ", ")
		if len(to) > 40 {
			to = to[:37] + "..."
		}

		created := msg.CreatedAt.Format("2006-01-02 15:04")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n",
			truncateID(msg.ID),
			msg.Status,
			msg.From,
			to,
			created,
			msg.RetryCount,
		)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d messages\n", len(messages))

	return nil
}

func runQueueShow(cmd *cobra.Command, args []string) error {
	storage, err := openQueueStorage()
	if err != nil {
		return err
	}
	defer storage.Close()

	ctx := context.Background()
	id := args[0]

	msg, err := storage.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}

	if msg == nil {
		return fmt.Errorf("message not found: %s", id)
	}

	fmt.Printf("Message: %s\n\n", msg.ID)
	fmt.Printf("Status:      %s\n", msg.Status)
	fmt.Printf("From:        %s\n", msg.From)
	fmt.Printf("To:          %s\n", strings.Join(msg.To, ", "))
	fmt.Printf("Created:     %s\n", msg.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:     %s\n", msg.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("Retry Count: %d\n", msg.RetryCount)

	if msg.NextRetryAt.After(time.Time{}) {
		fmt.Printf("Next Retry:  %s\n", msg.NextRetryAt.Format(time.RFC3339))
	}

	if msg.LastError != "" {
		fmt.Printf("\nLast Error:\n  %s\n", msg.LastError)
	}

	if msg.ClientIP != "" {
		fmt.Printf("\nClient IP: %s\n", msg.ClientIP)
	}

	// Show message data preview
	if len(msg.Data) > 0 {
		fmt.Println("\nMessage Preview (first 500 bytes):")
		fmt.Println("---")
		preview := string(msg.Data)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		fmt.Println(preview)
		fmt.Println("---")
	}

	return nil
}

func runQueueStats(cmd *cobra.Command, args []string) error {
	storage, err := openQueueStorage()
	if err != nil {
		return err
	}
	defer storage.Close()

	ctx := context.Background()

	stats, err := storage.Stats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queue stats: %w", err)
	}

	fmt.Println("Queue Statistics")
	fmt.Println("================")
	fmt.Printf("Total:     %d\n", stats.Total)
	fmt.Printf("Pending:   %d\n", stats.Pending)
	fmt.Printf("Sending:   %d\n", stats.Sending)
	fmt.Printf("Deferred:  %d\n", stats.Deferred)
	fmt.Printf("Delivered: %d\n", stats.Delivered)
	fmt.Printf("Failed:    %d\n", stats.Failed)

	// DLQ stats
	dlqStats, err := storage.DLQStats(ctx)
	if err == nil && dlqStats.Total > 0 {
		fmt.Println("\nDead Letter Queue")
		fmt.Println("-----------------")
		fmt.Printf("Total:     %d\n", dlqStats.Total)
		fmt.Printf("Size:      %d bytes\n", dlqStats.TotalSize)
		if !dlqStats.OldestAt.IsZero() {
			fmt.Printf("Oldest:    %s\n", dlqStats.OldestAt.Format(time.RFC3339))
		}
	}

	return nil
}

func runQueueRetry(cmd *cobra.Command, args []string) error {
	storage, err := openQueueStorage()
	if err != nil {
		return err
	}
	defer storage.Close()

	ctx := context.Background()
	id := args[0]

	// Try to get message
	msg, err := storage.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}

	if msg == nil {
		// Try DLQ
		msg, err = storage.GetFromDLQ(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get message from DLQ: %w", err)
		}

		if msg != nil {
			// Retry from DLQ
			if err := storage.RetryFromDLQ(ctx, id); err != nil {
				return fmt.Errorf("failed to retry message from DLQ: %w", err)
			}
			fmt.Printf("Message %s moved from DLQ to pending queue\n", id)
			return nil
		}

		return fmt.Errorf("message not found: %s", id)
	}

	// Reset message for retry
	msg.Status = queue.StatusPending
	msg.RetryCount = 0
	msg.LastError = ""
	msg.UpdatedAt = time.Now()

	if err := storage.Update(ctx, msg); err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	fmt.Printf("Message %s queued for retry\n", id)
	return nil
}

func runQueueDelete(cmd *cobra.Command, args []string) error {
	storage, err := openQueueStorage()
	if err != nil {
		return err
	}
	defer storage.Close()

	ctx := context.Background()
	id := args[0]

	// Try regular queue first
	msg, _ := storage.Get(ctx, id)
	if msg != nil {
		if err := storage.Delete(ctx, id); err != nil {
			return fmt.Errorf("failed to delete message: %w", err)
		}
		fmt.Printf("Message %s deleted from queue\n", id)
		return nil
	}

	// Try DLQ
	msg, _ = storage.GetFromDLQ(ctx, id)
	if msg != nil {
		if err := storage.DeleteFromDLQ(ctx, id); err != nil {
			return fmt.Errorf("failed to delete message from DLQ: %w", err)
		}
		fmt.Printf("Message %s deleted from DLQ\n", id)
		return nil
	}

	return fmt.Errorf("message not found: %s", id)
}

func truncateID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "..."
}
