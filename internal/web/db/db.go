package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

func New(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &DB{db}, nil
}

func (db *DB) Migrate() error {
	migrations := []string{
		migrationUsers,
		migrationSessions,
		migrationTemplates,
		migrationTemplateVersions,
		migrationTemplateDeployments,
		migrationRecipientLists,
		migrationRecipients,
		migrationCampaigns,
		migrationCampaignVariants,
		migrationSendJobs,
		migrationSendJobItems,
		migrationGlobalVariables,
		migrationAuditLog,
		migrationSettings,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

const migrationUsers = `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const migrationSessions = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`

const migrationTemplates = `
CREATE TABLE IF NOT EXISTS templates (
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
);
`

const migrationTemplateVersions = `
CREATE TABLE IF NOT EXISTS template_versions (
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
);
`

const migrationTemplateDeployments = `
CREATE TABLE IF NOT EXISTS template_deployments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id TEXT NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    server_name TEXT NOT NULL,
    remote_id TEXT,
    deployed_version INTEGER NOT NULL,
    deployed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(template_id, server_name)
);
`

const migrationRecipientLists = `
CREATE TABLE IF NOT EXISTS recipient_lists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    source_type TEXT NOT NULL,
    total_count INTEGER DEFAULT 0,
    active_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const migrationRecipients = `
CREATE TABLE IF NOT EXISTS recipients (
    id TEXT PRIMARY KEY,
    list_id TEXT NOT NULL REFERENCES recipient_lists(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    name TEXT,
    variables JSON,
    tags JSON,
    status TEXT DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(list_id, email)
);
CREATE INDEX IF NOT EXISTS idx_recipients_list_id ON recipients(list_id);
CREATE INDEX IF NOT EXISTS idx_recipients_status ON recipients(status);
`

const migrationCampaigns = `
CREATE TABLE IF NOT EXISTS campaigns (
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
);
`

const migrationCampaignVariants = `
CREATE TABLE IF NOT EXISTS campaign_variants (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    template_id TEXT NOT NULL REFERENCES templates(id),
    subject_override TEXT,
    weight INTEGER DEFAULT 100,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_campaign_variants_campaign ON campaign_variants(campaign_id);
`

const migrationSendJobs = `
CREATE TABLE IF NOT EXISTS send_jobs (
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
);
CREATE INDEX IF NOT EXISTS idx_send_jobs_campaign ON send_jobs(campaign_id);
CREATE INDEX IF NOT EXISTS idx_send_jobs_status ON send_jobs(status);
`

const migrationSendJobItems = `
CREATE TABLE IF NOT EXISTS send_job_items (
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
);
CREATE INDEX IF NOT EXISTS idx_send_job_items_job ON send_job_items(job_id);
CREATE INDEX IF NOT EXISTS idx_send_job_items_status ON send_job_items(status);
`

const migrationGlobalVariables = `
CREATE TABLE IF NOT EXISTS global_variables (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    description TEXT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const migrationAuditLog = `
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT,
    user_email TEXT,
    action TEXT NOT NULL,
    entity_type TEXT,
    entity_id TEXT,
    details JSON,
    ip_address TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_log_user ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);
`

const migrationSettings = `
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`
