package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/foxzi/sendry/internal/web/db"
)

// setupTestDB creates a temporary SQLite database with the real migrations applied
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d.DB
}
