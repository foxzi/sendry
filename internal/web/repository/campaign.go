package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

// campaignDB is what the queries run on: the database or a transaction
type campaignDB interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

type CampaignRepository struct {
	db   campaignDB
	conn *sql.DB // nil inside InTx
}

func NewCampaignRepository(db *sql.DB) *CampaignRepository {
	return &CampaignRepository{db: db, conn: db}
}

// InTx runs fn with a repository bound to one transaction: committed when fn
// returns nil, rolled back otherwise. Transactions take the write lock at
// BEGIN, so concurrent InTx calls run one after another.
func (r *CampaignRepository) InTx(fn func(tx *CampaignRepository) error) error {
	if r.conn == nil {
		return fn(r)
	}
	tx, err := r.conn.Begin()
	if err != nil {
		return err
	}
	if err := fn(&CampaignRepository{db: tx}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Create creates a new campaign
func (r *CampaignRepository) Create(c *models.Campaign) error {
	c.ID = uuid.New().String()
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO campaigns (id, name, description, from_email, from_name, reply_to, variables, tags, track_opens, track_clicks, add_utm, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Description, c.FromEmail, c.FromName, c.ReplyTo, c.Variables, c.Tags, c.TrackOpens, c.TrackClicks, c.AddUTM, c.CreatedAt, c.UpdatedAt,
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
		SELECT id, name, description, from_email, from_name, reply_to, variables, tags, track_opens, track_clicks, add_utm, created_at, updated_at
		FROM campaigns WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Description, &c.FromEmail, &c.FromName, &c.ReplyTo, &c.Variables, &c.Tags, &c.TrackOpens, &c.TrackClicks, &c.AddUTM, &c.CreatedAt, &c.UpdatedAt)

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
		countQuery += " AND (ulower(name) LIKE ? ESCAPE '\\' OR ulower(description) LIKE ? ESCAPE '\\')"
		args = append(args, searchPattern(filter.Search), searchPattern(filter.Search))
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get campaigns with stats
	query := `
		SELECT c.id, c.name, c.description, c.from_email, c.from_name, c.reply_to, c.variables, c.tags, c.track_opens, c.track_clicks, c.add_utm, c.created_at, c.updated_at,
			COALESCE((SELECT COUNT(*) FROM campaign_variants WHERE campaign_id = c.id AND weight > 0), 0) as variant_count, -- disabled versions (weight 0) are kept only for job history
			COALESCE((SELECT COUNT(*) FROM send_jobs WHERE campaign_id = c.id), 0) as job_count
		FROM campaigns c
		WHERE 1=1`

	args = []any{}
	if filter.Search != "" {
		query += " AND (ulower(c.name) LIKE ? ESCAPE '\\' OR ulower(c.description) LIKE ? ESCAPE '\\')"
		args = append(args, searchPattern(filter.Search), searchPattern(filter.Search))
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
			&c.Variables, &c.Tags, &c.TrackOpens, &c.TrackClicks, &c.AddUTM, &c.CreatedAt, &c.UpdatedAt,
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
		UPDATE campaigns SET name = ?, description = ?, from_email = ?, from_name = ?, reply_to = ?, variables = ?, tags = ?, track_opens = ?, track_clicks = ?, add_utm = ?, updated_at = ?
		WHERE id = ?`,
		c.Name, c.Description, c.FromEmail, c.FromName, c.ReplyTo, c.Variables, c.Tags, c.TrackOpens, c.TrackClicks, c.AddUTM, c.UpdatedAt, c.ID,
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

// UpdateVariant updates the name, template, subject override and weight of a variant
func (r *CampaignRepository) UpdateVariant(v *models.CampaignVariant) error {
	_, err := r.db.Exec(`
		UPDATE campaign_variants SET name = ?, template_id = ?, subject_override = ?, weight = ?
		WHERE id = ?`,
		v.Name, v.TemplateID, v.SubjectOverride, v.Weight, v.ID,
	)
	return err
}

// VariantInUse reports whether a send job used the variant
func (r *CampaignRepository) VariantInUse(id string) (bool, error) {
	var used bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM send_job_items WHERE variant_id = ?)", id).Scan(&used)
	return used, err
}

// DeleteVariant deletes a variant
func (r *CampaignRepository) DeleteVariant(id string) error {
	_, err := r.db.Exec("DELETE FROM campaign_variants WHERE id = ?", id)
	return inUse(err)
}

// UpdateVariables updates campaign variables
func (r *CampaignRepository) UpdateVariables(id, variables string) error {
	_, err := r.db.Exec("UPDATE campaigns SET variables = ?, updated_at = ? WHERE id = ?",
		variables, time.Now(), id)
	return err
}
