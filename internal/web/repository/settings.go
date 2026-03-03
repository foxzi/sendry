package repository

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

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

// GetGlobalVariablesMap returns all global variables as a map
func (r *SettingsRepository) GetGlobalVariablesMap() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT key, value FROM global_variables`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vars := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		vars[key] = value
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
		SELECT id, email, COALESCE(name, '') as name, COALESCE(role, 'user') as role, created_at, updated_at
		FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
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
		SELECT id, email, COALESCE(name, '') as name, COALESCE(role, 'user') as role, created_at, updated_at
		FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// CreateUser creates a new user
func (r *SettingsRepository) CreateUser(email, name, password string, role models.UserRole) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	now := time.Now()
	_, err = r.db.Exec(`
		INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, email, name, string(hash), string(role), now, now,
	)
	if err != nil {
		return nil, err
	}
	return &models.User{ID: id, Email: email, Name: name, Role: role, CreatedAt: now, UpdatedAt: now}, nil
}

// UpdateUser updates a user's name and role
func (r *SettingsRepository) UpdateUser(id, name string, role models.UserRole) error {
	_, err := r.db.Exec(`
		UPDATE users SET name = ?, role = ?, updated_at = ? WHERE id = ?`,
		name, string(role), time.Now(), id,
	)
	return err
}

// ChangePassword updates a user's password
func (r *SettingsRepository) ChangePassword(id, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		string(hash), time.Now(), id,
	)
	return err
}

// DeleteUser deletes a user by ID
func (r *SettingsRepository) DeleteUser(id string) error {
	_, err := r.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// CountAdmins returns the number of admin users
func (r *SettingsRepository) CountAdmins() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	return count, err
}

// LogAction writes an audit log entry. userID and userEmail may be empty for
// unauthenticated actions (e.g. failed login). details is optional free-form text.
// The request is used only to extract the client IP address.
func (r *SettingsRepository) LogAction(req *http.Request, userID, userEmail, action, entityType, entityID, details string) {
	ip := extractIP(req)
	_ = r.AddAuditLog(&models.AuditLogEntry{
		UserID:     userID,
		UserEmail:  userEmail,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    details,
		IPAddress:  ip,
	})
}

// extractIP returns the best-effort client IP from a request.
func extractIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For may be a comma-separated list; take the first.
		parts := strings.SplitN(fwd, ",", 2)
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return strings.TrimSpace(real)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// detailsJSON returns a JSON-encoded details string for structured audit entries.
// Example: detailsJSON("name", "foo", "role", "admin")
func detailsJSON(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, fmt.Sprintf("%q:%q", pairs[i], pairs[i+1]))
	}
	return "{" + strings.Join(parts, ",") + "}"
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
