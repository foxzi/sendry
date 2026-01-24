package main

import (
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up old data (jobs, audit logs, template versions)",
	RunE:  runCleanup,
}

var (
	cleanupJobsDays       int
	cleanupAuditDays      int
	cleanupVersionsKeep   int
	cleanupDryRun         bool
)

func init() {
	cleanupCmd.Flags().IntVar(&cleanupJobsDays, "jobs-days", 90, "Delete completed job items older than N days")
	cleanupCmd.Flags().IntVar(&cleanupAuditDays, "audit-days", 180, "Delete audit log entries older than N days")
	cleanupCmd.Flags().IntVar(&cleanupVersionsKeep, "versions-keep", 50, "Keep only last N versions per template")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be deleted without actually deleting")
	cleanupCmd.Flags().StringVarP(&configFile, "config", "c", "/etc/sendry/web.yaml", "Path to configuration file")
}

func runCleanup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	if cleanupDryRun {
		fmt.Println("Dry run mode - no data will be deleted")
		fmt.Println()
	}

	// Cleanup completed job items
	jobsCutoff := time.Now().AddDate(0, 0, -cleanupJobsDays)
	if err := cleanupJobItems(database, jobsCutoff); err != nil {
		return fmt.Errorf("failed to cleanup job items: %w", err)
	}

	// Cleanup audit logs
	auditCutoff := time.Now().AddDate(0, 0, -cleanupAuditDays)
	if err := cleanupAuditLogs(database, auditCutoff); err != nil {
		return fmt.Errorf("failed to cleanup audit logs: %w", err)
	}

	// Cleanup old template versions
	if err := cleanupTemplateVersions(database, cleanupVersionsKeep); err != nil {
		return fmt.Errorf("failed to cleanup template versions: %w", err)
	}

	if !cleanupDryRun {
		fmt.Println("\nCleanup completed")
	}

	return nil
}

func cleanupJobItems(database *db.DB, cutoff time.Time) error {
	// Count items to delete
	var count int
	err := database.QueryRow(`
		SELECT COUNT(*) FROM send_job_items
		WHERE status IN ('sent', 'failed', 'skipped') AND created_at < ?`,
		cutoff,
	).Scan(&count)
	if err != nil {
		return err
	}

	fmt.Printf("Job items older than %d days: %d\n", cleanupJobsDays, count)

	if !cleanupDryRun && count > 0 {
		result, err := database.Exec(`
			DELETE FROM send_job_items
			WHERE status IN ('sent', 'failed', 'skipped') AND created_at < ?`,
			cutoff,
		)
		if err != nil {
			return err
		}
		deleted, _ := result.RowsAffected()
		fmt.Printf("  Deleted: %d\n", deleted)
	}

	// Also cleanup completed jobs with no remaining items
	var completedCount int
	err = database.QueryRow(`
		SELECT COUNT(*) FROM send_jobs j
		WHERE j.status IN ('completed', 'failed', 'cancelled')
		AND j.created_at < ?
		AND NOT EXISTS (SELECT 1 FROM send_job_items WHERE job_id = j.id)`,
		cutoff,
	).Scan(&completedCount)
	if err != nil {
		return err
	}

	fmt.Printf("Completed jobs with no items: %d\n", completedCount)

	if !cleanupDryRun && completedCount > 0 {
		result, err := database.Exec(`
			DELETE FROM send_jobs
			WHERE status IN ('completed', 'failed', 'cancelled')
			AND created_at < ?
			AND NOT EXISTS (SELECT 1 FROM send_job_items WHERE job_id = send_jobs.id)`,
			cutoff,
		)
		if err != nil {
			return err
		}
		deleted, _ := result.RowsAffected()
		fmt.Printf("  Deleted: %d\n", deleted)
	}

	return nil
}

func cleanupAuditLogs(database *db.DB, cutoff time.Time) error {
	var count int
	err := database.QueryRow(`
		SELECT COUNT(*) FROM audit_log WHERE created_at < ?`,
		cutoff,
	).Scan(&count)
	if err != nil {
		return err
	}

	fmt.Printf("Audit log entries older than %d days: %d\n", cleanupAuditDays, count)

	if !cleanupDryRun && count > 0 {
		result, err := database.Exec(`DELETE FROM audit_log WHERE created_at < ?`, cutoff)
		if err != nil {
			return err
		}
		deleted, _ := result.RowsAffected()
		fmt.Printf("  Deleted: %d\n", deleted)
	}

	return nil
}

func cleanupTemplateVersions(database *db.DB, keepCount int) error {
	// Get templates with more than keepCount versions
	rows, err := database.Query(`
		SELECT template_id, COUNT(*) as version_count
		FROM template_versions
		GROUP BY template_id
		HAVING version_count > ?`,
		keepCount,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	totalDeleted := 0
	for rows.Next() {
		var templateID string
		var versionCount int
		if err := rows.Scan(&templateID, &versionCount); err != nil {
			return err
		}

		toDelete := versionCount - keepCount
		fmt.Printf("Template %s: %d versions (keeping %d, removing %d)\n",
			templateID, versionCount, keepCount, toDelete)

		if !cleanupDryRun {
			// Delete oldest versions, keeping the most recent keepCount
			result, err := database.Exec(`
				DELETE FROM template_versions
				WHERE template_id = ?
				AND version NOT IN (
					SELECT version FROM template_versions
					WHERE template_id = ?
					ORDER BY version DESC LIMIT ?
				)`,
				templateID, templateID, keepCount,
			)
			if err != nil {
				return err
			}
			deleted, _ := result.RowsAffected()
			totalDeleted += int(deleted)
			fmt.Printf("  Deleted: %d versions\n", deleted)
		}
	}

	if totalDeleted > 0 || cleanupDryRun {
		fmt.Printf("Total template versions to cleanup: %d\n", totalDeleted)
	}

	return nil
}
