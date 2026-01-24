package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type CampaignRepository struct {
	db *sql.DB
}

func NewCampaignRepository(db *sql.DB) *CampaignRepository {
	return &CampaignRepository{db: db}
}

// Create creates a new campaign
func (r *CampaignRepository) Create(c *models.Campaign) error {
	c.ID = uuid.New().String()
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO campaigns (id, name, description, from_email, from_name, reply_to, variables, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Description, c.FromEmail, c.FromName, c.ReplyTo, c.Variables, c.Tags, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create campaign: %w", err)
	}
	return nil
}

// GetByID returns a campaign by ID
func (r *CampaignRepository) GetByID(id string) (*models.Campaign, error) {
	c := &models.Campaign{}
	err := r.db.QueryRow(`
		SELECT id, name, description, from_email, from_name, reply_to, variables, tags, created_at, updated_at
		FROM campaigns WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Description, &c.FromEmail, &c.FromName, &c.ReplyTo, &c.Variables, &c.Tags, &c.CreatedAt, &c.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// List returns campaigns with optional filtering
func (r *CampaignRepository) List(filter models.CampaignListFilter) ([]models.CampaignWithStats, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM campaigns WHERE 1=1"
	args := []any{}

	if filter.Search != "" {
		countQuery += " AND (name LIKE ? OR description LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get campaigns with stats
	query := `
		SELECT c.id, c.name, c.description, c.from_email, c.from_name, c.reply_to, c.variables, c.tags, c.created_at, c.updated_at,
			COALESCE((SELECT COUNT(*) FROM campaign_variants WHERE campaign_id = c.id), 0) as variant_count,
			COALESCE((SELECT COUNT(*) FROM send_jobs WHERE campaign_id = c.id), 0) as job_count
		FROM campaigns c
		WHERE 1=1`

	args = []any{}
	if filter.Search != "" {
		query += " AND (c.name LIKE ? OR c.description LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	query += " ORDER BY c.updated_at DESC"

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

	campaigns := []models.CampaignWithStats{}
	for rows.Next() {
		var c models.CampaignWithStats
		err := rows.Scan(
			&c.ID, &c.Name, &c.Description, &c.FromEmail, &c.FromName, &c.ReplyTo,
			&c.Variables, &c.Tags, &c.CreatedAt, &c.UpdatedAt,
			&c.VariantCount, &c.JobCount,
		)
		if err != nil {
			return nil, 0, err
		}
		campaigns = append(campaigns, c)
	}

	return campaigns, total, nil
}

// Update updates a campaign
func (r *CampaignRepository) Update(c *models.Campaign) error {
	c.UpdatedAt = time.Now()
	_, err := r.db.Exec(`
		UPDATE campaigns SET name = ?, description = ?, from_email = ?, from_name = ?, reply_to = ?, variables = ?, tags = ?, updated_at = ?
		WHERE id = ?`,
		c.Name, c.Description, c.FromEmail, c.FromName, c.ReplyTo, c.Variables, c.Tags, c.UpdatedAt, c.ID,
	)
	return err
}

// Delete deletes a campaign
func (r *CampaignRepository) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM campaigns WHERE id = ?", id)
	return err
}

// AddVariant adds a template variant to a campaign
func (r *CampaignRepository) AddVariant(v *models.CampaignVariant) error {
	v.ID = uuid.New().String()
	v.CreatedAt = time.Now()

	_, err := r.db.Exec(`
		INSERT INTO campaign_variants (id, campaign_id, name, template_id, subject_override, weight, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.CampaignID, v.Name, v.TemplateID, v.SubjectOverride, v.Weight, v.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to add variant: %w", err)
	}
	return nil
}

// GetVariants returns all variants for a campaign
func (r *CampaignRepository) GetVariants(campaignID string) ([]models.CampaignVariant, error) {
	rows, err := r.db.Query(`
		SELECT v.id, v.campaign_id, v.name, v.template_id, t.name, v.subject_override, v.weight, v.created_at
		FROM campaign_variants v
		LEFT JOIN templates t ON v.template_id = t.id
		WHERE v.campaign_id = ?
		ORDER BY v.created_at`, campaignID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	variants := []models.CampaignVariant{}
	for rows.Next() {
		var v models.CampaignVariant
		var templateName sql.NullString
		err := rows.Scan(&v.ID, &v.CampaignID, &v.Name, &v.TemplateID, &templateName, &v.SubjectOverride, &v.Weight, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		if templateName.Valid {
			v.TemplateName = templateName.String
		}
		variants = append(variants, v)
	}

	return variants, nil
}

// GetVariant returns a variant by ID
func (r *CampaignRepository) GetVariant(id string) (*models.CampaignVariant, error) {
	v := &models.CampaignVariant{}
	var templateName sql.NullString
	err := r.db.QueryRow(`
		SELECT v.id, v.campaign_id, v.name, v.template_id, t.name, v.subject_override, v.weight, v.created_at
		FROM campaign_variants v
		LEFT JOIN templates t ON v.template_id = t.id
		WHERE v.id = ?`, id,
	).Scan(&v.ID, &v.CampaignID, &v.Name, &v.TemplateID, &templateName, &v.SubjectOverride, &v.Weight, &v.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if templateName.Valid {
		v.TemplateName = templateName.String
	}
	return v, nil
}

// UpdateVariant updates a variant
func (r *CampaignRepository) UpdateVariant(v *models.CampaignVariant) error {
	_, err := r.db.Exec(`
		UPDATE campaign_variants SET name = ?, template_id = ?, subject_override = ?, weight = ?
		WHERE id = ?`,
		v.Name, v.TemplateID, v.SubjectOverride, v.Weight, v.ID,
	)
	return err
}

// DeleteVariant deletes a variant
func (r *CampaignRepository) DeleteVariant(id string) error {
	_, err := r.db.Exec("DELETE FROM campaign_variants WHERE id = ?", id)
	return err
}

// UpdateVariables updates campaign variables
func (r *CampaignRepository) UpdateVariables(id, variables string) error {
	_, err := r.db.Exec("UPDATE campaigns SET variables = ?, updated_at = ? WHERE id = ?",
		variables, time.Now(), id)
	return err
}
