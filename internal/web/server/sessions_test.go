package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/db"
)

func TestPurgeExpiredSessions(t *testing.T) {
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if _, err := d.Exec(`INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
		VALUES ('u1', 'u@example.com', 'U', 'x', 'admin', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	sessions := map[string]time.Time{
		"expired-long-ago": now.Add(-48 * time.Hour),
		"expired-just-now": now.Add(-time.Second),
		"active":           now.Add(time.Hour),
	}
	for id, exp := range sessions {
		if _, err := d.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, 'u1', ?)", id, exp); err != nil {
			t.Fatal(err)
		}
	}

	n, err := purgeExpiredSessions(d.DB, now)
	if err != nil {
		t.Fatalf("purgeExpiredSessions() error = %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d sessions, want 2", n)
	}

	var left []string
	rows, err := d.Query("SELECT id FROM sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		left = append(left, id)
	}
	if len(left) != 1 || left[0] != "active" {
		t.Errorf("remaining sessions = %v, want [active]", left)
	}
}
