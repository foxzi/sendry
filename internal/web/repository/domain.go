package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type DomainRepository struct {
	db *sql.DB
}

func NewDomainRepository(db *sql.DB) *DomainRepository {
	return &DomainRepository{db: db}
}

// Create creates a new domain configuration
func (r *DomainRepository) Create(domain *models.Domain) error {
	domain.ID = uuid.New().String()
	domain.CreatedAt = time.Now()
	domain.UpdatedAt = domain.CreatedAt

	redirectJSON, _ := json.Marshal(domain.RedirectTo)
	bccJSON, _ := json.Marshal(domain.BCCTo)

	_, err := r.db.Exec(`
		INSERT INTO domains (id, domain, mode, default_from, dkim_enabled, dkim_selector, dkim_key_id,
			rate_limit_hour, rate_limit_day, rate_limit_recipients, redirect_to, bcc_to, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		domain.ID, domain.Domain, domain.Mode, domain.DefaultFrom,
		domain.DKIMEnabled, domain.DKIMSelector, nullString(domain.DKIMKeyID),
		domain.RateLimitHour, domain.RateLimitDay, domain.RateLimitRecipients,
		string(redirectJSON), string(bccJSON), domain.CreatedAt, domain.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create domain: %w", err)
	}
	return nil
}

// GetByID returns a domain by ID
func (r *DomainRepository) GetByID(id string) (*models.Domain, error) {
	domain := &models.Domain{}
	var redirectJSON, bccJSON sql.NullString
	var dkimKeyID sql.NullString

	err := r.db.QueryRow(`
		SELECT id, domain, mode, COALESCE(default_from, ''), dkim_enabled, COALESCE(dkim_selector, ''), dkim_key_id,
			rate_limit_hour, rate_limit_day, rate_limit_recipients, redirect_to, bcc_to, created_at, updated_at
		FROM domains WHERE id = ?`, id,
	).Scan(&domain.ID, &domain.Domain, &domain.Mode, &domain.DefaultFrom,
		&domain.DKIMEnabled, &domain.DKIMSelector, &dkimKeyID,
		&domain.RateLimitHour, &domain.RateLimitDay, &domain.RateLimitRecipients,
		&redirectJSON, &bccJSON, &domain.CreatedAt, &domain.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if dkimKeyID.Valid {
		domain.DKIMKeyID = dkimKeyID.String
	}

	if redirectJSON.Valid && redirectJSON.String != "" {
		json.Unmarshal([]byte(redirectJSON.String), &domain.RedirectTo)
	}
	if bccJSON.Valid && bccJSON.String != "" {
		json.Unmarshal([]byte(bccJSON.String), &domain.BCCTo)
	}

	// Load deployments
	deployments, err := r.GetDeployments(id)
	if err != nil {
		return nil, err
	}
	domain.Deployments = deployments

	return domain, nil
}

// GetByDomain returns a domain by domain name
func (r *DomainRepository) GetByDomain(domainName string) (*models.Domain, error) {
	domain := &models.Domain{}
	var redirectJSON, bccJSON sql.NullString
	var dkimKeyID sql.NullString

	err := r.db.QueryRow(`
		SELECT id, domain, mode, COALESCE(default_from, ''), dkim_enabled, COALESCE(dkim_selector, ''), dkim_key_id,
			rate_limit_hour, rate_limit_day, rate_limit_recipients, redirect_to, bcc_to, created_at, updated_at
		FROM domains WHERE domain = ?`, domainName,
	).Scan(&domain.ID, &domain.Domain, &domain.Mode, &domain.DefaultFrom,
		&domain.DKIMEnabled, &domain.DKIMSelector, &dkimKeyID,
		&domain.RateLimitHour, &domain.RateLimitDay, &domain.RateLimitRecipients,
		&redirectJSON, &bccJSON, &domain.CreatedAt, &domain.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if dkimKeyID.Valid {
		domain.DKIMKeyID = dkimKeyID.String
	}

	if redirectJSON.Valid && redirectJSON.String != "" {
		json.Unmarshal([]byte(redirectJSON.String), &domain.RedirectTo)
	}
	if bccJSON.Valid && bccJSON.String != "" {
		json.Unmarshal([]byte(bccJSON.String), &domain.BCCTo)
	}

	// Load deployments
	deployments, err := r.GetDeployments(domain.ID)
	if err != nil {
		return nil, err
	}
	domain.Deployments = deployments

	return domain, nil
}

// List returns all domains with deployment counts
func (r *DomainRepository) List(filter models.DomainFilter) ([]models.DomainListItem, error) {
	query := `
		SELECT d.id, d.domain, d.mode, d.dkim_enabled, COALESCE(d.dkim_selector, ''), d.created_at,
			COUNT(dd.id) as deployment_count,
			SUM(CASE WHEN dd.status = 'outdated' THEN 1 ELSE 0 END) as outdated_count
		FROM domains d
		LEFT JOIN domain_deployments dd ON d.id = dd.domain_id
		WHERE 1=1`

	args := []interface{}{}

	if filter.Search != "" {
		query += " AND d.domain LIKE ?"
		args = append(args, "%"+filter.Search+"%")
	}
	if filter.Mode != "" {
		query += " AND d.mode = ?"
		args = append(args, filter.Mode)
	}

	query += " GROUP BY d.id ORDER BY d.domain"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []models.DomainListItem
	for rows.Next() {
		var d models.DomainListItem
		if err := rows.Scan(&d.ID, &d.Domain, &d.Mode, &d.DKIMEnabled, &d.DKIMSelector,
			&d.CreatedAt, &d.DeploymentCount, &d.OutdatedCount); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// Update updates a domain configuration
func (r *DomainRepository) Update(id string, domain *models.Domain) error {
	domain.UpdatedAt = time.Now()

	redirectJSON, _ := json.Marshal(domain.RedirectTo)
	bccJSON, _ := json.Marshal(domain.BCCTo)

	result, err := r.db.Exec(`
		UPDATE domains SET
			mode = ?, default_from = ?, dkim_enabled = ?, dkim_selector = ?, dkim_key_id = ?,
			rate_limit_hour = ?, rate_limit_day = ?, rate_limit_recipients = ?,
			redirect_to = ?, bcc_to = ?, updated_at = ?
		WHERE id = ?`,
		domain.Mode, domain.DefaultFrom, domain.DKIMEnabled, domain.DKIMSelector, nullString(domain.DKIMKeyID),
		domain.RateLimitHour, domain.RateLimitDay, domain.RateLimitRecipients,
		string(redirectJSON), string(bccJSON), domain.UpdatedAt, id,
	)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("domain not found")
	}

	// Mark all deployments as outdated
	_, err = r.db.Exec(`
		UPDATE domain_deployments SET status = 'outdated'
		WHERE domain_id = ? AND status = 'deployed'`, id)

	return err
}

// Delete deletes a domain and its deployments
func (r *DomainRepository) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM domains WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("domain not found")
	}
	return nil
}

// CreateDeployment records a deployment of a domain to a server
func (r *DomainRepository) CreateDeployment(domainID, serverName, status, configHash, errMsg string) error {
	_, err := r.db.Exec(`
		INSERT INTO domain_deployments (domain_id, server_name, deployed_at, status, config_hash, error)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(domain_id, server_name) DO UPDATE SET
			deployed_at = excluded.deployed_at,
			status = excluded.status,
			config_hash = excluded.config_hash,
			error = excluded.error`,
		domainID, serverName, time.Now(), status, configHash, errMsg,
	)
	return err
}

// GetDeployments returns all deployments for a domain
func (r *DomainRepository) GetDeployments(domainID string) ([]models.DomainDeployment, error) {
	rows, err := r.db.Query(`
		SELECT id, domain_id, server_name, deployed_at, status, COALESCE(error, ''), COALESCE(config_hash, '')
		FROM domain_deployments WHERE domain_id = ?
		ORDER BY server_name`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []models.DomainDeployment
	for rows.Next() {
		var d models.DomainDeployment
		if err := rows.Scan(&d.ID, &d.DomainID, &d.ServerName, &d.DeployedAt, &d.Status, &d.Error, &d.ConfigHash); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

// GetDeployment returns a specific deployment
func (r *DomainRepository) GetDeployment(domainID, serverName string) (*models.DomainDeployment, error) {
	var d models.DomainDeployment
	err := r.db.QueryRow(`
		SELECT id, domain_id, server_name, deployed_at, status, COALESCE(error, ''), COALESCE(config_hash, '')
		FROM domain_deployments WHERE domain_id = ? AND server_name = ?`, domainID, serverName,
	).Scan(&d.ID, &d.DomainID, &d.ServerName, &d.DeployedAt, &d.Status, &d.Error, &d.ConfigHash)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// DeleteDeployment removes a deployment record
func (r *DomainRepository) DeleteDeployment(domainID, serverName string) error {
	_, err := r.db.Exec("DELETE FROM domain_deployments WHERE domain_id = ? AND server_name = ?", domainID, serverName)
	return err
}

// GetOutdatedDeployments returns all deployments that need sync
func (r *DomainRepository) GetOutdatedDeployments(domainID string) ([]models.DomainDeployment, error) {
	rows, err := r.db.Query(`
		SELECT id, domain_id, server_name, deployed_at, status, COALESCE(error, ''), COALESCE(config_hash, '')
		FROM domain_deployments WHERE domain_id = ? AND status = 'outdated'
		ORDER BY server_name`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []models.DomainDeployment
	for rows.Next() {
		var d models.DomainDeployment
		if err := rows.Scan(&d.ID, &d.DomainID, &d.ServerName, &d.DeployedAt, &d.Status, &d.Error, &d.ConfigHash); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

// helper function for nullable strings
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
