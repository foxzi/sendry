package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

// SetSkippedUnsubscribed stores how many list recipients were left out at job
// start because they are unsubscribed in the list.
func (r *JobRepository) SetSkippedUnsubscribed(jobID string, n int) error {
	_, err := r.db.Exec("UPDATE send_jobs SET skipped_unsubscribed = ?, updated_at = ? WHERE id = ?",
		n, time.Now().UTC(), jobID)
	return err
}

// GetVariantTemplates returns the template sources frozen for a job, by
// variant id. Variants without a snapshot (or with a snapshot from before the
// source was stored) are absent. The rendered snapshot HTML is not read.
func (r *JobRepository) GetVariantTemplates(jobID string) (map[string]*models.JobVariantTemplate, error) {
	rows, err := r.db.Query(`
		SELECT variant_id, COALESCE(template_id, ''), COALESCE(template_version, 0),
			COALESCE(template_subject, ''), template_html, COALESCE(template_text, '')
		FROM job_variant_snapshots
		WHERE job_id = ? AND template_html IS NOT NULL`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]*models.JobVariantTemplate{}
	for rows.Next() {
		t := &models.JobVariantTemplate{}
		if err := rows.Scan(&t.VariantID, &t.TemplateID, &t.TemplateVersion, &t.Subject, &t.HTML, &t.Text); err != nil {
			return nil, err
		}
		out[t.VariantID] = t
	}
	return out, rows.Err()
}

// FreezeVariantTemplate stores the snapshot of a job variant together with its
// frozen template source. An existing snapshot that already has a source is
// kept as is (the first freeze wins); one without a source is completed.
func (r *JobRepository) FreezeVariantTemplate(s *models.JobVariantSnapshot, t *models.JobVariantTemplate) error {
	links := s.Links
	if links == nil {
		links = []models.LinkInfo{}
	}
	linksJSON, err := json.Marshal(links)
	if err != nil {
		return err
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	_, err = r.db.Exec(`
		INSERT INTO job_variant_snapshots (job_id, variant_id, variant_name, template_id, template_version, html, links,
			template_subject, template_html, template_text, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id, variant_id) DO UPDATE SET
			variant_name = excluded.variant_name, template_id = excluded.template_id,
			template_version = excluded.template_version, html = excluded.html, links = excluded.links,
			template_subject = excluded.template_subject, template_html = excluded.template_html,
			template_text = excluded.template_text, created_at = excluded.created_at
		WHERE job_variant_snapshots.template_html IS NULL`,
		s.JobID, s.VariantID, s.VariantName, t.TemplateID, t.TemplateVersion, s.HTML, string(linksJSON),
		t.Subject, t.HTML, t.Text, s.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to freeze variant template: %w", err)
	}
	return nil
}
