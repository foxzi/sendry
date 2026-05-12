package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
		migrationEmailBlocks,
		migrationMediaFiles,
		migrationTemplateBlockRefs,
		migrationUserSMTPServers,
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
		"ALTER TABLE api_keys ADD COLUMN allowed_domains TEXT DEFAULT '[]'",
		"ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'",
		"ALTER TABLE users ADD COLUMN password_hash TEXT",
		"UPDATE users SET role = 'admin' WHERE id = (SELECT id FROM users ORDER BY created_at ASC LIMIT 1)",
		"ALTER TABLE templates ADD COLUMN use_blocks INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE template_block_refs ADD COLUMN gap_height INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE template_block_refs ADD COLUMN gap_color TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE templates ADD COLUMN container_radius INTEGER NOT NULL DEFAULT 8",
		"ALTER TABLE templates ADD COLUMN container_transparent INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE templates ADD COLUMN container_width INTEGER NOT NULL DEFAULT 600",
		"ALTER TABLE templates ADD COLUMN container_padding_v INTEGER NOT NULL DEFAULT 20",
		"ALTER TABLE templates ADD COLUMN container_padding_h INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE email_blocks ADD COLUMN border_radius INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE email_blocks ADD COLUMN padding_v INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE email_blocks ADD COLUMN padding_h INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE email_blocks ADD COLUMN background TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE templates ADD COLUMN page_background TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE templates ADD COLUMN container_radius_top INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE templates ADD COLUMN container_radius_bottom INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE template_block_refs ADD COLUMN condition TEXT NOT NULL DEFAULT ''",
	}
	for _, m := range alterMigrations {
		db.Exec(m) // Ignore errors (column may already exist)
	}

	if err := db.dropColumnIfExists("templates", "container_background"); err != nil {
		return fmt.Errorf("drop container_background: %w", err)
	}
	if err := db.dropColumnIfExists("templates", "block_divider_width"); err != nil {
		return fmt.Errorf("drop block_divider_width: %w", err)
	}
	if err := db.dropColumnIfExists("templates", "block_divider_color"); err != nil {
		return fmt.Errorf("drop block_divider_color: %w", err)
	}

	return nil
}

func (db *DB) dropColumnIfExists(table, column string) error {
	if !db.columnExists(table, column) {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)); err == nil {
		return nil
	}
	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable fk: %w", err)
	}
	defer db.Exec("PRAGMA foreign_keys = ON")

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	cols, err := tableColumnsExcept(tx, table, column)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return fmt.Errorf("table %s has no columns left after removing %s", table, column)
	}

	createSQL, err := tableCreateSQLWithoutColumn(tx, table, column)
	if err != nil {
		return err
	}

	tmp := table + "__migrated"
	createSQL = replaceTableName(createSQL, table, tmp)
	if _, err := tx.Exec(createSQL); err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	colList := strings.Join(cols, ", ")
	if _, err := tx.Exec(fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", tmp, colList, colList, table)); err != nil {
		return fmt.Errorf("copy rows: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf("DROP TABLE %s", table)); err != nil {
		return fmt.Errorf("drop original: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", tmp, table)); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return tx.Commit()
}

func (db *DB) columnExists(table, column string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}

func tableColumnsExcept(tx *sql.Tx, table, except string) ([]string, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		if name == except {
			continue
		}
		cols = append(cols, name)
	}
	return cols, nil
}

func tableCreateSQLWithoutColumn(tx *sql.Tx, table, column string) (string, error) {
	var sqlText string
	err := tx.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&sqlText)
	if err != nil {
		return "", err
	}
	var out []string
	colRE := regexp.MustCompile(`(?i)^\s*` + regexp.QuoteMeta(column) + `\s+`)
	for _, line := range strings.Split(sqlText, "\n") {
		if colRE.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	rebuilt := strings.Join(out, "\n")
	rebuilt = regexp.MustCompile(`,\s*\)`).ReplaceAllString(rebuilt, "\n)")
	return rebuilt, nil
}

func replaceTableName(createSQL, oldName, newName string) string {
	re := regexp.MustCompile(`(?i)(CREATE\s+TABLE\s+(IF\s+NOT\s+EXISTS\s+)?["` + "`" + `]?)` + regexp.QuoteMeta(oldName) + `(["` + "`" + `]?)`)
	return re.ReplaceAllString(createSQL, "${1}"+newName+"${3}")
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
    use_blocks INTEGER NOT NULL DEFAULT 0,
    container_radius INTEGER NOT NULL DEFAULT 8,
    container_transparent INTEGER NOT NULL DEFAULT 0,
    container_width INTEGER NOT NULL DEFAULT 600,
    container_padding_v INTEGER NOT NULL DEFAULT 20,
    container_padding_h INTEGER NOT NULL DEFAULT 0,
    page_background TEXT NOT NULL DEFAULT '',
    container_radius_top INTEGER NOT NULL DEFAULT 0,
    container_radius_bottom INTEGER NOT NULL DEFAULT 0,
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

const migrationEmailBlocks = `
CREATE TABLE IF NOT EXISTS email_blocks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general',
    html TEXT NOT NULL,
    preview_text TEXT,
    border_radius INTEGER NOT NULL DEFAULT 0,
    padding_v INTEGER NOT NULL DEFAULT 0,
    padding_h INTEGER NOT NULL DEFAULT 0,
    background TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_email_blocks_category ON email_blocks(category);
`

const migrationMediaFiles = `
CREATE TABLE IF NOT EXISTS media_files (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    orig_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    url TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const migrationTemplateBlockRefs = `
CREATE TABLE IF NOT EXISTS template_block_refs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id TEXT NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    block_id TEXT NOT NULL REFERENCES email_blocks(id),
    position INTEGER NOT NULL,
    gap_height INTEGER NOT NULL DEFAULT 0,
    gap_color TEXT NOT NULL DEFAULT '',
    condition TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_template_block_refs_template ON template_block_refs(template_id, position);
CREATE INDEX IF NOT EXISTS idx_template_block_refs_block ON template_block_refs(block_id);
`

const migrationUserSMTPServers = `
CREATE TABLE IF NOT EXISTS user_smtp_servers (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    username TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    encryption TEXT NOT NULL DEFAULT 'ssl',
    from_address TEXT NOT NULL,
    from_name TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_smtp_servers_user ON user_smtp_servers(user_id);
`
