package db

import (
	"context"
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

	// Foreign keys are enabled in the DSN: PRAGMA foreign_keys is per
	// connection, and the DSN applies it to every connection of the pool.
	// _txlock=immediate: a transaction takes the write lock at BEGIN and waits
	// for it (busy_timeout). A deferred transaction that reads and then writes
	// fails at once with "database is locked" when another connection
	// committed in between, and busy_timeout does not help there.
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
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
		migrationEmailEvents,
		migrationUnsubscribes,
		migrationJobVariantSnapshots,
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
		// Open/click tracking and unsubscribes
		// Existing campaigns keep sending untracked; new ones take the defaults from settings
		"ALTER TABLE campaigns ADD COLUMN track_opens INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE campaigns ADD COLUMN track_clicks INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE campaigns ADD COLUMN add_utm INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE api_keys ADD COLUMN track_opens INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE api_keys ADD COLUMN track_clicks INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE send_jobs ADD COLUMN skipped_unsubscribed INTEGER NOT NULL DEFAULT 0",
		// Whether the campaign tracked opens when the job was created; NULL
		// for older jobs (statistics then use the campaign setting)
		"ALTER TABLE send_jobs ADD COLUMN track_opens INTEGER",
		"ALTER TABLE send_job_items ADD COLUMN opened_at TIMESTAMP",
		"ALTER TABLE send_job_items ADD COLUMN open_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE send_job_items ADD COLUMN clicked_at TIMESTAMP",
		"ALTER TABLE send_job_items ADD COLUMN click_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE send_job_items ADD COLUMN unsubscribed_at TIMESTAMP",
		"ALTER TABLE sends ADD COLUMN opened_at TIMESTAMP",
		"ALTER TABLE sends ADD COLUMN open_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE sends ADD COLUMN clicked_at TIMESTAMP",
		"ALTER TABLE sends ADD COLUMN click_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE job_variant_snapshots ADD COLUMN template_subject TEXT",
		"ALTER TABLE job_variant_snapshots ADD COLUMN template_html TEXT",
		"ALTER TABLE job_variant_snapshots ADD COLUMN template_text TEXT",
	}
	for _, m := range alterMigrations {
		db.Exec(m) // Ignore errors (column may already exist)
	}

	// Indexes added after the tables were created, and ones no query uses
	indexMigrations := []string{
		"DROP INDEX IF EXISTS idx_email_events_email",
		"DROP INDEX IF EXISTS idx_email_events_created",
		"DROP INDEX IF EXISTS idx_unsubscribes_campaign",
		"DROP INDEX IF EXISTS idx_unsubscribes_created",
		"CREATE INDEX IF NOT EXISTS idx_email_events_job_stats ON email_events(job_id, type, is_machine)",
		"CREATE INDEX IF NOT EXISTS idx_email_events_dedup ON email_events(source_type, source_id, type, created_at)",
	}
	for _, m := range indexMigrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("index migration failed: %w", err)
		}
	}

	if err := db.normalizeRecipientEmails(); err != nil {
		return fmt.Errorf("normalize recipient emails: %w", err)
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

// normalizeRecipientEmails lower-cases recipient emails once and adds the
// unique index on (list_id, lower(email)), so the same address in different
// case is one recipient. Addresses that collide after lower-casing are merged
// into one row: a non-active one wins (an unsubscribe must not be lost), then
// the oldest. Job items and unsubscribe journal rows are moved to that row.
func (db *DB) normalizeRecipientEmails() error {
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_recipients_list_email'`,
	).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	steps := []string{
		`CREATE TEMP TABLE recipient_dups (id TEXT PRIMARY KEY, keep_id TEXT NOT NULL)`,
		`INSERT INTO recipient_dups (id, keep_id)
		SELECT id, keep_id FROM (
			SELECT id, FIRST_VALUE(id) OVER (
				PARTITION BY list_id, lower(trim(email))
				ORDER BY CASE WHEN COALESCE(status, 'active') = 'active' THEN 1 ELSE 0 END, created_at, id
			) AS keep_id
			FROM recipients
		) WHERE id != keep_id`,
		`UPDATE send_job_items SET recipient_id = (SELECT keep_id FROM recipient_dups d WHERE d.id = send_job_items.recipient_id)
		WHERE recipient_id IN (SELECT id FROM recipient_dups)`,
		`UPDATE unsubscribes SET recipient_id = (SELECT keep_id FROM recipient_dups d WHERE d.id = unsubscribes.recipient_id)
		WHERE recipient_id IN (SELECT id FROM recipient_dups)`,
		`DELETE FROM recipients WHERE id IN (SELECT id FROM recipient_dups)`,
		`UPDATE recipients SET email = lower(trim(email)) WHERE email != lower(trim(email))`,
		`UPDATE recipient_lists SET
			total_count = (SELECT COUNT(*) FROM recipients WHERE list_id = recipient_lists.id),
			active_count = (SELECT COUNT(*) FROM recipients WHERE list_id = recipient_lists.id AND status = 'active')
		WHERE id IN (SELECT DISTINCT r.list_id FROM recipients r JOIN recipient_dups d ON d.keep_id = r.id)`,
		`DROP TABLE recipient_dups`,
		`CREATE UNIQUE INDEX idx_recipients_list_email ON recipients(list_id, lower(email))`,
	}
	for _, s := range steps {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) dropColumnIfExists(table, column string) error {
	if !db.columnExists(table, column) {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)); err == nil {
		return nil
	}

	// The pragma is per connection (and ignored inside a transaction), so the
	// whole rebuild runs on one connection with foreign keys off
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable fk: %w", err)
	}
	defer conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")

	tx, err := conn.BeginTx(ctx, nil)
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

// email_events is an append-only log of engagement events (opens, clicks,
// unsubscribes). Rows are never deleted for now; retention is decided later.
const migrationEmailEvents = `
CREATE TABLE IF NOT EXISTS email_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    campaign_id TEXT,
    job_id TEXT,
    variant_id TEXT,
    email TEXT NOT NULL,
    link_id INTEGER,
    url TEXT,
    ip TEXT,
    user_agent TEXT,
    is_machine INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_email_events_source ON email_events(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_email_events_job ON email_events(job_id, type);
CREATE INDEX IF NOT EXISTS idx_email_events_campaign ON email_events(campaign_id, type);
`

// unsubscribes is the journal of unsubscribes per address book (recipient
// list). The source of truth for sending is recipients.status = 'unsubscribed'
// in that list; the same address in another list keeps receiving mail. The
// journal records when and from which campaign the address unsubscribed.
const migrationUnsubscribes = `
CREATE TABLE IF NOT EXISTS unsubscribes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    list_id TEXT NOT NULL REFERENCES recipient_lists(id) ON DELETE CASCADE,
    recipient_id TEXT,
    email TEXT NOT NULL,
    campaign_id TEXT,
    job_id TEXT,
    source_type TEXT NOT NULL,
    source_id TEXT,
    created_at TIMESTAMP NOT NULL,
    UNIQUE(list_id, email)
);
`

// job_variant_snapshots keeps the HTML of each variant as it was when the job
// started, with data-sl link numbers, so the click map stays correct after
// the template is edited. html is rendered with placeholder variables for the
// click map; template_* is the frozen template source (html with data-sl) the
// worker renders for every recipient of the job.
const migrationJobVariantSnapshots = `
CREATE TABLE IF NOT EXISTS job_variant_snapshots (
    job_id TEXT NOT NULL REFERENCES send_jobs(id) ON DELETE CASCADE,
    variant_id TEXT NOT NULL,
    variant_name TEXT,
    template_id TEXT,
    template_version INTEGER,
    html TEXT NOT NULL,
    links JSON NOT NULL DEFAULT '[]',
    template_subject TEXT,
    template_html TEXT,
    template_text TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (job_id, variant_id)
);
`
