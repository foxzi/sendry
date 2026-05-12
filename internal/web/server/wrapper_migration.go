package server

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/foxzi/sendry/internal/web/auth"
	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/handlers"
	"github.com/foxzi/sendry/internal/web/views"
)

const wrapperRebuildVersion = "1"

const wrapperRebuildSettingKey = "wrapper_rebuild_version"

func runWrapperRebuildMigration(
	database *db.DB,
	viewEngine *views.Engine,
	cfg *config.Config,
	oidcProvider *auth.OIDCProvider,
	logger *slog.Logger,
) error {
	current, err := getSetting(database.DB, wrapperRebuildSettingKey)
	if err != nil {
		return fmt.Errorf("read setting: %w", err)
	}
	if current == wrapperRebuildVersion {
		return nil // already applied
	}

	h := handlers.New(cfg, database, logger, viewEngine, oidcProvider)

	logger.Info("wrapper rebuild: starting",
		"from_version", current,
		"to_version", wrapperRebuildVersion,
	)

	rebuilt, skipped, failed, err := h.RebuildAllBlockTemplates("system@migration")
	if err != nil {
		return fmt.Errorf("rebuild: %w", err)
	}

	logger.Info("wrapper rebuild: done",
		"rebuilt", rebuilt,
		"skipped", skipped,
		"failed", failed,
	)

	if err := setSetting(database.DB, wrapperRebuildSettingKey, wrapperRebuildVersion); err != nil {
		return fmt.Errorf("save setting: %w", err)
	}
	return nil
}

func getSetting(d *sql.DB, key string) (string, error) {
	var v string
	err := d.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func setSetting(d *sql.DB, key, value string) error {
	_, err := d.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}
