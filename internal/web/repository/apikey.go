package repository

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type APIKeyRepository struct {
	db *sql.DB
}

func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

// APIKeyCreateOptions contains options for creating an API key
type APIKeyCreateOptions struct {
	Name            string
	CreatedBy       string
	Permissions     []string
	ExpiresAt       *time.Time
	RateLimitMinute int
	RateLimitHour   int
}

// Create creates a new API key and returns the full key (only shown once)
func (r *APIKeyRepository) Create(opts APIKeyCreateOptions) (*models.APIKeyCreateResult, error) {
	// Generate random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	key := "sk_" + hex.EncodeToString(keyBytes)

	// Hash the key for storage
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	// Get prefix for display
	keyPrefix := key[:11] // "sk_" + first 8 chars

	// Serialize permissions
	permJSON := `["send"]`
	if len(opts.Permissions) > 0 {
		permJSON = `["` + opts.Permissions[0] + `"]`
		for _, p := range opts.Permissions[1:] {
			permJSON = permJSON[:len(permJSON)-1] + `","` + p + `"]`
		}
	}

	apiKey := &models.APIKey{
		ID:              uuid.New().String(),
		Name:            opts.Name,
		KeyHash:         keyHash,
		KeyPrefix:       keyPrefix,
		Permissions:     permJSON,
		RateLimitMinute: opts.RateLimitMinute,
		RateLimitHour:   opts.RateLimitHour,
		CreatedBy:       opts.CreatedBy,
		CreatedAt:       time.Now(),
		ExpiresAt:       opts.ExpiresAt,
		Active:          true,
	}

	_, err := r.db.Exec(`
		INSERT INTO api_keys (id, name, key_hash, key_prefix, permissions, rate_limit_minute, rate_limit_hour, created_by, created_at, expires_at, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		apiKey.ID, apiKey.Name, apiKey.KeyHash, apiKey.KeyPrefix, apiKey.Permissions,
		apiKey.RateLimitMinute, apiKey.RateLimitHour,
		apiKey.CreatedBy, apiKey.CreatedAt, apiKey.ExpiresAt, 1,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	return &models.APIKeyCreateResult{
		APIKey: *apiKey,
		Key:    key,
	}, nil
}

// GetByID returns an API key by ID
func (r *APIKeyRepository) GetByID(id string) (*models.APIKey, error) {
	k := &models.APIKey{}
	var expiresAt, lastUsedAt sql.NullTime
	var rateLimitMinute, rateLimitHour sql.NullInt64

	err := r.db.QueryRow(`
		SELECT id, name, key_hash, key_prefix, permissions,
		       COALESCE(rate_limit_minute, 0), COALESCE(rate_limit_hour, 0),
		       created_by, created_at, last_used_at, expires_at, active
		FROM api_keys WHERE id = ?`, id,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Permissions,
		&rateLimitMinute, &rateLimitHour,
		&k.CreatedBy, &k.CreatedAt, &lastUsedAt, &expiresAt, &k.Active)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	k.RateLimitMinute = int(rateLimitMinute.Int64)
	k.RateLimitHour = int(rateLimitHour.Int64)

	if expiresAt.Valid {
		k.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.Time
	}

	return k, nil
}

// GetByHash returns an API key by its hash (for authentication)
func (r *APIKeyRepository) GetByHash(keyHash string) (*models.APIKey, error) {
	k := &models.APIKey{}
	var expiresAt, lastUsedAt sql.NullTime
	var rateLimitMinute, rateLimitHour sql.NullInt64

	err := r.db.QueryRow(`
		SELECT id, name, key_hash, key_prefix, permissions,
		       COALESCE(rate_limit_minute, 0), COALESCE(rate_limit_hour, 0),
		       created_by, created_at, last_used_at, expires_at, active
		FROM api_keys WHERE key_hash = ?`, keyHash,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Permissions,
		&rateLimitMinute, &rateLimitHour,
		&k.CreatedBy, &k.CreatedAt, &lastUsedAt, &expiresAt, &k.Active)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	k.RateLimitMinute = int(rateLimitMinute.Int64)
	k.RateLimitHour = int(rateLimitHour.Int64)

	if expiresAt.Valid {
		k.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.Time
	}

	return k, nil
}

// List returns all API keys with optional filtering
func (r *APIKeyRepository) List(filter models.APIKeyFilter) ([]models.APIKeyWithStats, int, error) {
	countQuery := "SELECT COUNT(*) FROM api_keys WHERE 1=1"
	args := []any{}

	if filter.Search != "" {
		countQuery += " AND name LIKE ?"
		args = append(args, "%"+filter.Search+"%")
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT k.id, k.name, k.key_hash, k.key_prefix, k.permissions,
		       COALESCE(k.rate_limit_minute, 0), COALESCE(k.rate_limit_hour, 0),
		       k.created_by, k.created_at, k.last_used_at, k.expires_at, k.active,
		       COALESCE(s.send_count, 0) as send_count
		FROM api_keys k
		LEFT JOIN (
			SELECT api_key_id, COUNT(*) as send_count
			FROM sends
			GROUP BY api_key_id
		) s ON k.id = s.api_key_id
		WHERE 1=1`

	if filter.Search != "" {
		query += " AND k.name LIKE ?"
	}

	query += " ORDER BY k.created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var keys []models.APIKeyWithStats
	for rows.Next() {
		var k models.APIKeyWithStats
		var expiresAt, lastUsedAt sql.NullTime

		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Permissions,
			&k.RateLimitMinute, &k.RateLimitHour,
			&k.CreatedBy, &k.CreatedAt, &lastUsedAt, &expiresAt, &k.Active, &k.SendCount); err != nil {
			return nil, 0, err
		}

		if expiresAt.Valid {
			k.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			k.LastUsedAt = &lastUsedAt.Time
		}

		keys = append(keys, k)
	}

	return keys, total, rows.Err()
}

// UpdateLastUsed updates the last_used_at timestamp
func (r *APIKeyRepository) UpdateLastUsed(id string) error {
	_, err := r.db.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", time.Now(), id)
	return err
}

// Deactivate deactivates an API key
func (r *APIKeyRepository) Deactivate(id string) error {
	result, err := r.db.Exec("UPDATE api_keys SET active = 0 WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("API key not found")
	}
	return nil
}

// Activate activates an API key
func (r *APIKeyRepository) Activate(id string) error {
	result, err := r.db.Exec("UPDATE api_keys SET active = 1 WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("API key not found")
	}
	return nil
}

// ToggleActive toggles the active status of an API key
func (r *APIKeyRepository) ToggleActive(id string) (bool, error) {
	// Get current status
	var active bool
	err := r.db.QueryRow("SELECT active FROM api_keys WHERE id = ?", id).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("API key not found")
	}

	// Toggle
	newActive := !active
	_, err = r.db.Exec("UPDATE api_keys SET active = ? WHERE id = ?", newActive, id)
	if err != nil {
		return false, err
	}

	return newActive, nil
}

// Delete permanently deletes an API key
func (r *APIKeyRepository) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("API key not found")
	}
	return nil
}

// HashKey computes SHA256 hash of an API key
func HashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
