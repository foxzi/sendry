package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type TemplateRepository struct {
	db *sql.DB
}

func NewTemplateRepository(db *sql.DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

// Create creates a new template and its first version
func (r *TemplateRepository) Create(t *models.Template, createdBy string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	t.ID = uuid.New().String()
	t.CurrentVersion = 1
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt

	// Insert template
	_, err = tx.Exec(`
		INSERT INTO templates (id, name, description, subject, html, text, variables, folder, current_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Description, t.Subject, t.HTML, t.Text, t.Variables, t.Folder, t.CurrentVersion, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	// Insert first version
	_, err = tx.Exec(`
		INSERT INTO template_versions (template_id, version, subject, html, text, variables, change_note, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, 1, t.Subject, t.HTML, t.Text, t.Variables, "Initial version", createdBy, t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create template version: %w", err)
	}

	return tx.Commit()
}

// GetByID returns a template by ID
func (r *TemplateRepository) GetByID(id string) (*models.Template, error) {
	t := &models.Template{}
	err := r.db.QueryRow(`
		SELECT id, name, description, subject, html, text, variables, folder, current_version, created_at, updated_at
		FROM templates WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Subject, &t.HTML, &t.Text, &t.Variables, &t.Folder, &t.CurrentVersion, &t.CreatedAt, &t.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// List returns templates with optional filtering
func (r *TemplateRepository) List(filter models.TemplateListFilter) ([]models.TemplateWithStatus, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM templates WHERE 1=1"
	args := []any{}

	if filter.Search != "" {
		countQuery += " AND (name LIKE ? OR description LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.Folder != "" {
		countQuery += " AND folder = ?"
		args = append(args, filter.Folder)
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get templates
	query := `
		SELECT t.id, t.name, t.description, t.subject, t.html, t.text, t.variables, t.folder, t.current_version, t.created_at, t.updated_at,
			COALESCE(d.deployed_count, 0) as deployed_count,
			COALESCE(d.out_of_sync_count, 0) as out_of_sync_count
		FROM templates t
		LEFT JOIN (
			SELECT template_id,
				COUNT(*) as deployed_count,
				SUM(CASE WHEN deployed_version < (SELECT current_version FROM templates WHERE id = template_id) THEN 1 ELSE 0 END) as out_of_sync_count
			FROM template_deployments
			GROUP BY template_id
		) d ON t.id = d.template_id
		WHERE 1=1`

	args = []any{}
	if filter.Search != "" {
		query += " AND (t.name LIKE ? OR t.description LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.Folder != "" {
		query += " AND t.folder = ?"
		args = append(args, filter.Folder)
	}

	query += " ORDER BY t.updated_at DESC"

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

	templates := []models.TemplateWithStatus{}
	for rows.Next() {
		var t models.TemplateWithStatus
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Subject, &t.HTML, &t.Text,
			&t.Variables, &t.Folder, &t.CurrentVersion, &t.CreatedAt, &t.UpdatedAt,
			&t.DeployedCount, &t.OutOfSyncCount,
		)
		if err != nil {
			return nil, 0, err
		}

		// Determine status
		if t.DeployedCount == 0 {
			t.Status = "draft"
		} else if t.OutOfSyncCount > 0 {
			t.Status = "out-of-sync"
		} else {
			t.Status = "deployed"
		}

		templates = append(templates, t)
	}

	return templates, total, nil
}

// Update updates a template and creates a new version
func (r *TemplateRepository) Update(t *models.Template, changeNote, updatedBy string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get current version
	var currentVersion int
	err = tx.QueryRow("SELECT current_version FROM templates WHERE id = ?", t.ID).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("template not found: %w", err)
	}

	newVersion := currentVersion + 1
	t.CurrentVersion = newVersion
	t.UpdatedAt = time.Now()

	// Update template
	_, err = tx.Exec(`
		UPDATE templates SET name = ?, description = ?, subject = ?, html = ?, text = ?, variables = ?, folder = ?, current_version = ?, updated_at = ?
		WHERE id = ?`,
		t.Name, t.Description, t.Subject, t.HTML, t.Text, t.Variables, t.Folder, t.CurrentVersion, t.UpdatedAt, t.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	// Insert new version
	_, err = tx.Exec(`
		INSERT INTO template_versions (template_id, version, subject, html, text, variables, change_note, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, newVersion, t.Subject, t.HTML, t.Text, t.Variables, changeNote, updatedBy, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create template version: %w", err)
	}

	return tx.Commit()
}

// Delete deletes a template
func (r *TemplateRepository) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM templates WHERE id = ?", id)
	return err
}

// GetVersions returns all versions of a template
func (r *TemplateRepository) GetVersions(templateID string) ([]models.TemplateVersion, error) {
	rows, err := r.db.Query(`
		SELECT id, template_id, version, subject, html, text, variables, change_note, created_by, created_at
		FROM template_versions WHERE template_id = ? ORDER BY version DESC`, templateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := []models.TemplateVersion{}
	for rows.Next() {
		var v models.TemplateVersion
		err := rows.Scan(&v.ID, &v.TemplateID, &v.Version, &v.Subject, &v.HTML, &v.Text, &v.Variables, &v.ChangeNote, &v.CreatedBy, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// GetVersion returns a specific version
func (r *TemplateRepository) GetVersion(templateID string, version int) (*models.TemplateVersion, error) {
	v := &models.TemplateVersion{}
	err := r.db.QueryRow(`
		SELECT id, template_id, version, subject, html, text, variables, change_note, created_by, created_at
		FROM template_versions WHERE template_id = ? AND version = ?`, templateID, version,
	).Scan(&v.ID, &v.TemplateID, &v.Version, &v.Subject, &v.HTML, &v.Text, &v.Variables, &v.ChangeNote, &v.CreatedBy, &v.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// GetDeployments returns all deployments for a template
func (r *TemplateRepository) GetDeployments(templateID string) ([]models.TemplateDeployment, error) {
	rows, err := r.db.Query(`
		SELECT id, template_id, server_name, remote_id, deployed_version, deployed_at
		FROM template_deployments WHERE template_id = ? ORDER BY server_name`, templateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deployments := []models.TemplateDeployment{}
	for rows.Next() {
		var d models.TemplateDeployment
		err := rows.Scan(&d.ID, &d.TemplateID, &d.ServerName, &d.RemoteID, &d.DeployedVersion, &d.DeployedAt)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, nil
}

// SaveDeployment saves or updates a deployment record
func (r *TemplateRepository) SaveDeployment(d *models.TemplateDeployment) error {
	_, err := r.db.Exec(`
		INSERT INTO template_deployments (template_id, server_name, remote_id, deployed_version, deployed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(template_id, server_name) DO UPDATE SET
			remote_id = excluded.remote_id,
			deployed_version = excluded.deployed_version,
			deployed_at = excluded.deployed_at`,
		d.TemplateID, d.ServerName, d.RemoteID, d.DeployedVersion, time.Now(),
	)
	return err
}

// GetFolders returns distinct folder names
func (r *TemplateRepository) GetFolders() ([]string, error) {
	rows, err := r.db.Query("SELECT DISTINCT folder FROM templates WHERE folder != '' ORDER BY folder")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := []string{}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, nil
}
