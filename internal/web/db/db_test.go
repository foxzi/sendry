package db

import (
	"path/filepath"
	"testing"
)

func TestMigrate_NormalizesRecipientEmails(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.SetMaxOpenConns(1)
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}

	// a database from before the normalization: mixed-case duplicates
	for _, q := range []string{
		`DROP INDEX idx_recipients_list_email`,
		`INSERT INTO recipient_lists (id, name, source_type, total_count, active_count) VALUES ('l1', 'one', 'manual', 4, 3)`,
		`INSERT INTO recipient_lists (id, name, source_type) VALUES ('l2', 'two', 'manual')`,
		`INSERT INTO recipients (id, list_id, email, status, created_at) VALUES ('a1', 'l1', 'User@Example.org', 'active', '2026-01-01')`,
		`INSERT INTO recipients (id, list_id, email, status, created_at) VALUES ('a2', 'l1', 'user@example.org', 'unsubscribed', '2026-01-02')`,
		`INSERT INTO recipients (id, list_id, email, status, created_at) VALUES ('a3', 'l1', ' USER@example.org', 'active', '2026-01-03')`,
		`INSERT INTO recipients (id, list_id, email, status, created_at) VALUES ('b1', 'l1', 'Other@Example.org', 'active', '2026-01-01')`,
		`INSERT INTO recipients (id, list_id, email, status, created_at) VALUES ('c1', 'l2', 'USER@example.org', 'active', '2026-01-01')`,
		`INSERT INTO campaigns (id, name, from_email) VALUES ('c', 'c', 'a@example.com')`,
		`INSERT INTO send_jobs (id, campaign_id, recipient_list_id) VALUES ('j', 'c', 'l1')`,
		`INSERT INTO send_job_items (id, job_id, recipient_id) VALUES ('i1', 'j', 'a1')`,
		`INSERT INTO send_job_items (id, job_id, recipient_id) VALUES ('i3', 'j', 'a3')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}

	rows, err := d.Query(`SELECT id, list_id, email, status FROM recipients ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var id, list, email, status string
		rows.Scan(&id, &list, &email, &status)
		got = append(got, id+" "+list+" "+email+" "+status)
	}
	rows.Close()
	want := []string{
		"a2 l1 user@example.org unsubscribed", // the unsubscribed duplicate wins
		"b1 l1 other@example.org active",
		"c1 l2 user@example.org active", // another list is not touched
	}
	if len(got) != len(want) {
		t.Fatalf("recipients = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("recipient %d = %q, want %q", i, got[i], want[i])
		}
	}

	var moved int
	d.QueryRow(`SELECT COUNT(*) FROM send_job_items WHERE recipient_id = 'a2'`).Scan(&moved)
	if moved != 2 {
		t.Errorf("job items moved to the kept recipient: %d, want 2", moved)
	}
	var total, active int
	d.QueryRow(`SELECT total_count, active_count FROM recipient_lists WHERE id = 'l1'`).Scan(&total, &active)
	if total != 2 || active != 1 {
		t.Errorf("list counts = %d/%d, want 2/1", total, active)
	}

	if _, err := d.Exec(`INSERT INTO recipients (id, list_id, email) VALUES ('x', 'l1', 'OTHER@example.org')`); err == nil {
		t.Error("case-insensitive duplicate inserted")
	}
}

func TestMigrate_Indexes(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	// an old database still has the dropped indexes
	if _, err := d.Exec(`CREATE INDEX idx_email_events_email ON email_events(email)`); err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}

	exists := func(name string) bool {
		var n int
		d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&n)
		return n > 0
	}
	for _, name := range []string{"idx_email_events_email", "idx_email_events_created", "idx_unsubscribes_campaign", "idx_unsubscribes_created"} {
		if exists(name) {
			t.Errorf("index %s not dropped", name)
		}
	}
	for _, name := range []string{"idx_email_events_job_stats", "idx_email_events_dedup", "idx_recipients_list_email"} {
		if !exists(name) {
			t.Errorf("index %s missing", name)
		}
	}

	// tracking is off for campaigns that existed before the columns were added
	var dflt string
	d.QueryRow(`SELECT dflt_value FROM pragma_table_info('campaigns') WHERE name = 'track_opens'`).Scan(&dflt)
	if dflt != "0" {
		t.Errorf("track_opens default = %q, want 0", dflt)
	}
}

func TestMigrate_DropsTrackingDefaults(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	// a database from v0.6.0 has per-key and global tracking defaults
	for _, q := range []string{
		`ALTER TABLE api_keys ADD COLUMN track_opens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE api_keys ADD COLUMN track_clicks INTEGER NOT NULL DEFAULT 0`,
		`INSERT INTO settings (key, value) VALUES ('tracking.opens_default', '1'), ('tracking.clicks_default', '0'), ('tracking.base_url', 'https://lnk.example.com')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}

	for _, col := range []string{"track_opens", "track_clicks"} {
		if d.columnExists("api_keys", col) {
			t.Errorf("api_keys.%s not dropped", col)
		}
	}
	var keys []string
	rows, err := d.Query(`SELECT key FROM settings WHERE key LIKE 'tracking.%' ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		rows.Scan(&k)
		keys = append(keys, k)
	}
	rows.Close()
	if len(keys) != 1 || keys[0] != "tracking.base_url" {
		t.Errorf("tracking settings = %v, want only tracking.base_url", keys)
	}
}
