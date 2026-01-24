package repository

import (
	"database/sql"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

type SettingsRepository struct {
	db *sql.DB
}

func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

// GetAllVariables returns all global variables
func (r *SettingsRepository) GetAllVariables() ([]models.GlobalVariable, error) {
	rows, err := r.db.Query(`
		SELECT key, value, COALESCE(description, '') as description, updated_at
		FROM global_variables ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vars := []models.GlobalVariable{}
	for rows.Next() {
		var v models.GlobalVariable
		if err := rows.Scan(&v.Key, &v.Value, &v.Description, &v.UpdatedAt); err != nil {
			return nil, err
		}
		vars = append(vars, v)
	}
	return vars, nil
}

// GetVariable returns a single variable
func (r *SettingsRepository) GetVariable(key string) (*models.GlobalVariable, error) {
	v := &models.GlobalVariable{}
	err := r.db.QueryRow(`
		SELECT key, value, COALESCE(description, '') as description, updated_at
		FROM global_variables WHERE key = ?`, key,
	).Scan(&v.Key, &v.Value, &v.Description, &v.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// SetVariable sets a global variable
func (r *SettingsRepository) SetVariable(key, value, description string) error {
	_, err := r.db.Exec(`
		INSERT INTO global_variables (key, value, description, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, description = excluded.description, updated_at = excluded.updated_at`,
		key, value, description, time.Now(),
	)
	return err
}

// DeleteVariable deletes a global variable
func (r *SettingsRepository) DeleteVariable(key string) error {
	_, err := r.db.Exec("DELETE FROM global_variables WHERE key = ?", key)
	return err
}

// ListUsers returns all users
func (r *SettingsRepository) ListUsers() ([]models.User, error) {
	rows, err := r.db.Query(`
		SELECT id, email, COALESCE(name, '') as name, created_at, updated_at
		FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// GetUserByID returns a user by ID
func (r *SettingsRepository) GetUserByID(id string) (*models.User, error) {
	u := &models.User{}
	err := r.db.QueryRow(`
		SELECT id, email, COALESCE(name, '') as name, created_at, updated_at
		FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// AddAuditLog adds an audit log entry
func (r *SettingsRepository) AddAuditLog(entry *models.AuditLogEntry) error {
	entry.CreatedAt = time.Now()
	_, err := r.db.Exec(`
		INSERT INTO audit_log (user_id, user_email, action, entity_type, entity_id, details, ip_address, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.UserID, entry.UserEmail, entry.Action, entry.EntityType, entry.EntityID, entry.Details, entry.IPAddress, entry.CreatedAt,
	)
	return err
}

// ListAuditLog returns audit log entries
func (r *SettingsRepository) ListAuditLog(filter models.AuditLogFilter) ([]models.AuditLogEntry, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM audit_log WHERE 1=1"
	args := []any{}

	if filter.UserID != "" {
		countQuery += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	if filter.Action != "" {
		countQuery += " AND action = ?"
		args = append(args, filter.Action)
	}
	if filter.EntityType != "" {
		countQuery += " AND entity_type = ?"
		args = append(args, filter.EntityType)
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get entries
	query := `
		SELECT id, COALESCE(user_id, '') as user_id, COALESCE(user_email, '') as user_email,
			action, COALESCE(entity_type, '') as entity_type, COALESCE(entity_id, '') as entity_id,
			COALESCE(details, '') as details, COALESCE(ip_address, '') as ip_address, created_at
		FROM audit_log WHERE 1=1`

	args = []any{}
	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	if filter.Action != "" {
		query += " AND action = ?"
		args = append(args, filter.Action)
	}
	if filter.EntityType != "" {
		query += " AND entity_type = ?"
		args = append(args, filter.EntityType)
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := []models.AuditLogEntry{}
	for rows.Next() {
		var e models.AuditLogEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.UserEmail, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.IPAddress, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}

	return entries, total, nil
}

// GetSetting returns a setting value
func (r *SettingsRepository) GetSetting(key string) (string, error) {
	var value string
	err := r.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting sets a setting value
func (r *SettingsRepository) SetSetting(key, value string) error {
	_, err := r.db.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now(),
	)
	return err
}
