package repository

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates an in-memory SQLite database with all migrations applied
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	// Apply migrations
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS templates (
			id TEXT PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			subject TEXT NOT NULL,
			html TEXT,
			text TEXT,
			variables JSON,
			folder TEXT,
			current_version INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS template_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			template_id TEXT NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			subject TEXT NOT NULL,
			html TEXT,
			text TEXT,
			variables JSON,
			change_note TEXT,
			created_by TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(template_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS template_deployments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			template_id TEXT NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
			server_name TEXT NOT NULL,
			remote_id TEXT,
			deployed_version INTEGER NOT NULL,
			deployed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(template_id, server_name)
		)`,
		`CREATE TABLE IF NOT EXISTS recipient_lists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			source_type TEXT NOT NULL,
			total_count INTEGER DEFAULT 0,
			active_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS recipients (
			id TEXT PRIMARY KEY,
			list_id TEXT NOT NULL REFERENCES recipient_lists(id) ON DELETE CASCADE,
			email TEXT NOT NULL,
			name TEXT,
			variables JSON,
			tags JSON,
			status TEXT DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(list_id, email)
		)`,
		`CREATE TABLE IF NOT EXISTS campaigns (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			from_email TEXT NOT NULL,
			from_name TEXT,
			reply_to TEXT,
			variables JSON,
			tags JSON,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS campaign_variants (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			template_id TEXT NOT NULL REFERENCES templates(id),
			subject_override TEXT,
			weight INTEGER DEFAULT 100,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS send_jobs (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			recipient_list_id TEXT NOT NULL REFERENCES recipient_lists(id),
			status TEXT DEFAULT 'draft',
			scheduled_at TIMESTAMP,
			started_at TIMESTAMP,
			completed_at TIMESTAMP,
			servers JSON,
			strategy TEXT,
			stats JSON,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS send_job_items (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL REFERENCES send_jobs(id) ON DELETE CASCADE,
			recipient_id TEXT NOT NULL REFERENCES recipients(id),
			variant_id TEXT REFERENCES campaign_variants(id),
			server_name TEXT,
			status TEXT DEFAULT 'pending',
			sendry_msg_id TEXT,
			error TEXT,
			queued_at TIMESTAMP,
			sent_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS global_variables (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			description TEXT,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT,
			user_email TEXT,
			action TEXT NOT NULL,
			entity_type TEXT,
			entity_id TEXT,
			details JSON,
			ip_address TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			t.Fatalf("migration failed: %v", err)
		}
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}
