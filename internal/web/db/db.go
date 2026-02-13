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
		migrationDKIMKeys,
		migrationDKIMDeployments,
		migrationDomains,
		migrationDomainDeployments,
		migrationAPIKeys,
		migrationSends,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Run ALTER TABLE migrations (ignore errors for existing columns)
	alterMigrations := []string{
		"ALTER TABLE send_jobs ADD COLUMN dry_run INTEGER DEFAULT 0",
		"ALTER TABLE send_jobs ADD COLUMN dry_run_limit INTEGER DEFAULT 0",
		"ALTER TABLE api_keys ADD COLUMN rate_limit_minute INTEGER DEFAULT 0",
		"ALTER TABLE api_keys ADD COLUMN rate_limit_hour INTEGER DEFAULT 0",
	}
	for _, m := range alterMigrations {
		db.Exec(m) // Ignore errors (column may already exist)
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

const migrationDKIMKeys = `
CREATE TABLE IF NOT EXISTS dkim_keys (
    id TEXT PRIMARY KEY,
    domain TEXT NOT NULL,
    selector TEXT NOT NULL,
    private_key TEXT NOT NULL,
    dns_record TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(domain, selector)
);
CREATE INDEX IF NOT EXISTS idx_dkim_keys_domain ON dkim_keys(domain);
`

const migrationDKIMDeployments = `
CREATE TABLE IF NOT EXISTS dkim_deployments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dkim_key_id TEXT NOT NULL REFERENCES dkim_keys(id) ON DELETE CASCADE,
    server_name TEXT NOT NULL,
    deployed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'deployed',
    error TEXT,
    UNIQUE(dkim_key_id, server_name)
);
CREATE INDEX IF NOT EXISTS idx_dkim_deployments_key ON dkim_deployments(dkim_key_id);
`

const migrationDomains = `
CREATE TABLE IF NOT EXISTS domains (
    id TEXT PRIMARY KEY,
    domain TEXT UNIQUE NOT NULL,
    mode TEXT DEFAULT 'production',
    default_from TEXT,
    dkim_enabled INTEGER DEFAULT 0,
    dkim_selector TEXT,
    dkim_key_id TEXT REFERENCES dkim_keys(id) ON DELETE SET NULL,
    rate_limit_hour INTEGER DEFAULT 0,
    rate_limit_day INTEGER DEFAULT 0,
    rate_limit_recipients INTEGER DEFAULT 0,
    redirect_to TEXT,
    bcc_to TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_domains_domain ON domains(domain);
`

const migrationDomainDeployments = `
CREATE TABLE IF NOT EXISTS domain_deployments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    server_name TEXT NOT NULL,
    deployed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'deployed',
    error TEXT,
    config_hash TEXT,
    UNIQUE(domain_id, server_name)
);
CREATE INDEX IF NOT EXISTS idx_domain_deployments_domain ON domain_deployments(domain_id);
`

const migrationAPIKeys = `
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    key_hash TEXT UNIQUE NOT NULL,
    key_prefix TEXT NOT NULL,
    permissions TEXT DEFAULT '["send"]',
    created_by TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    active INTEGER DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_active ON api_keys(active);
`

const migrationSends = `
CREATE TABLE IF NOT EXISTS sends (
    id TEXT PRIMARY KEY,
    api_key_id TEXT REFERENCES api_keys(id) ON DELETE SET NULL,
    from_address TEXT NOT NULL,
    to_addresses TEXT NOT NULL,
    cc_addresses TEXT,
    bcc_addresses TEXT,
    subject TEXT,
    template_id TEXT REFERENCES templates(id) ON DELETE SET NULL,
    sender_domain TEXT NOT NULL,
    server_name TEXT NOT NULL,
    server_msg_id TEXT,
    status TEXT DEFAULT 'pending',
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    sent_at TIMESTAMP,
    client_ip TEXT
);
CREATE INDEX IF NOT EXISTS idx_sends_api_key ON sends(api_key_id);
CREATE INDEX IF NOT EXISTS idx_sends_status ON sends(status);
CREATE INDEX IF NOT EXISTS idx_sends_domain ON sends(sender_domain);
CREATE INDEX IF NOT EXISTS idx_sends_server ON sends(server_name);
CREATE INDEX IF NOT EXISTS idx_sends_created ON sends(created_at);
`

