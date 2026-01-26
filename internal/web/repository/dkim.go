package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type DKIMRepository struct {
	db *sql.DB
}

func NewDKIMRepository(db *sql.DB) *DKIMRepository {
	return &DKIMRepository{db: db}
}

// Create creates a new DKIM key
func (r *DKIMRepository) Create(key *models.DKIMKey) error {
	key.ID = uuid.New().String()
	key.CreatedAt = time.Now()
	key.UpdatedAt = key.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO dkim_keys (id, domain, selector, private_key, dns_record, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		key.ID, key.Domain, key.Selector, key.PrivateKey, key.DNSRecord, key.CreatedAt, key.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create DKIM key: %w", err)
	}
	return nil
}

// GetByID returns a DKIM key by ID
func (r *DKIMRepository) GetByID(id string) (*models.DKIMKey, error) {
	key := &models.DKIMKey{}
	err := r.db.QueryRow(`
		SELECT id, domain, selector, private_key, dns_record, created_at, updated_at
		FROM dkim_keys WHERE id = ?`, id,
	).Scan(&key.ID, &key.Domain, &key.Selector, &key.PrivateKey, &key.DNSRecord, &key.CreatedAt, &key.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Load deployments
	deployments, err := r.GetDeployments(id)
	if err != nil {
		return nil, err
	}
	key.Deployments = deployments

	return key, nil
}

// GetByDomainSelector returns a DKIM key by domain and selector
func (r *DKIMRepository) GetByDomainSelector(domain, selector string) (*models.DKIMKey, error) {
	key := &models.DKIMKey{}
	err := r.db.QueryRow(`
		SELECT id, domain, selector, private_key, dns_record, created_at, updated_at
		FROM dkim_keys WHERE domain = ? AND selector = ?`, domain, selector,
	).Scan(&key.ID, &key.Domain, &key.Selector, &key.PrivateKey, &key.DNSRecord, &key.CreatedAt, &key.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return key, nil
}

// List returns all DKIM keys with deployment counts
func (r *DKIMRepository) List() ([]models.DKIMKeyListItem, error) {
	rows, err := r.db.Query(`
		SELECT k.id, k.domain, k.selector, k.dns_record, k.created_at,
			COUNT(d.id) as deployment_count
		FROM dkim_keys k
		LEFT JOIN dkim_deployments d ON k.id = d.dkim_key_id
		GROUP BY k.id
		ORDER BY k.domain, k.selector`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.DKIMKeyListItem
	for rows.Next() {
		var k models.DKIMKeyListItem
		if err := rows.Scan(&k.ID, &k.Domain, &k.Selector, &k.DNSRecord, &k.CreatedAt, &k.DeploymentCount); err != nil {
			return nil, err
		}
		k.DNSName = k.Selector + "._domainkey." + k.Domain
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Delete deletes a DKIM key and its deployments
func (r *DKIMRepository) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM dkim_keys WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("DKIM key not found")
	}
	return nil
}

// CreateDeployment records a deployment of a DKIM key to a server
func (r *DKIMRepository) CreateDeployment(keyID, serverName, status, errMsg string) error {
	_, err := r.db.Exec(`
		INSERT INTO dkim_deployments (dkim_key_id, server_name, deployed_at, status, error)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(dkim_key_id, server_name) DO UPDATE SET
			deployed_at = excluded.deployed_at,
			status = excluded.status,
			error = excluded.error`,
		keyID, serverName, time.Now(), status, errMsg,
	)
	return err
}

// GetDeployments returns all deployments for a DKIM key
func (r *DKIMRepository) GetDeployments(keyID string) ([]models.DKIMDeployment, error) {
	rows, err := r.db.Query(`
		SELECT id, dkim_key_id, server_name, deployed_at, status, COALESCE(error, '')
		FROM dkim_deployments WHERE dkim_key_id = ?
		ORDER BY server_name`, keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []models.DKIMDeployment
	for rows.Next() {
		var d models.DKIMDeployment
		if err := rows.Scan(&d.ID, &d.DKIMKeyID, &d.ServerName, &d.DeployedAt, &d.Status, &d.Error); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

// DeleteDeployment removes a deployment record
func (r *DKIMRepository) DeleteDeployment(keyID, serverName string) error {
	_, err := r.db.Exec("DELETE FROM dkim_deployments WHERE dkim_key_id = ? AND server_name = ?", keyID, serverName)
	return err
}
