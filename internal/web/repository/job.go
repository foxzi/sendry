package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type JobRepository struct {
	db *sql.DB
}

func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

// Create creates a new send job
func (r *JobRepository) Create(job *models.SendJob) error {
	job.ID = uuid.New().String()
	job.Status = "draft"
	job.CreatedAt = time.Now()
	job.UpdatedAt = job.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status, scheduled_at, servers, strategy, stats, dry_run, dry_run_limit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.CampaignID, job.RecipientListID, job.Status, job.ScheduledAt, job.Servers, job.Strategy, job.Stats, job.DryRun, job.DryRunLimit, job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	return nil
}

// GetByID returns a job by ID
func (r *JobRepository) GetByID(id string) (*models.SendJob, error) {
	job := &models.SendJob{}
	var scheduledAt, startedAt, completedAt sql.NullTime
	var campaignName, listName sql.NullString

	err := r.db.QueryRow(`
		SELECT j.id, j.campaign_id, c.name, j.recipient_list_id, rl.name, j.status,
			j.scheduled_at, j.started_at, j.completed_at, j.servers, j.strategy, j.stats,
			COALESCE(j.dry_run, 0), COALESCE(j.dry_run_limit, 0), j.created_at, j.updated_at
		FROM send_jobs j
		LEFT JOIN campaigns c ON j.campaign_id = c.id
		LEFT JOIN recipient_lists rl ON j.recipient_list_id = rl.id
		WHERE j.id = ?`, id,
	).Scan(&job.ID, &job.CampaignID, &campaignName, &job.RecipientListID, &listName, &job.Status,
		&scheduledAt, &startedAt, &completedAt, &job.Servers, &job.Strategy, &job.Stats,
		&job.DryRun, &job.DryRunLimit, &job.CreatedAt, &job.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if campaignName.Valid {
		job.CampaignName = campaignName.String
	}
	if listName.Valid {
		job.ListName = listName.String
	}
	if scheduledAt.Valid {
		job.ScheduledAt = &scheduledAt.Time
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}

	return job, nil
}

// List returns jobs with optional filtering
func (r *JobRepository) List(filter models.JobListFilter) ([]models.SendJob, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM send_jobs WHERE 1=1"
	args := []any{}

	if filter.CampaignID != "" {
		countQuery += " AND campaign_id = ?"
		args = append(args, filter.CampaignID)
	}
	if filter.Status != "" {
		countQuery += " AND status = ?"
		args = append(args, filter.Status)
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get jobs
	query := `
		SELECT j.id, j.campaign_id, c.name, j.recipient_list_id, rl.name, j.status,
			j.scheduled_at, j.started_at, j.completed_at, j.servers, j.strategy, j.stats,
			COALESCE(j.dry_run, 0), COALESCE(j.dry_run_limit, 0), j.created_at, j.updated_at
		FROM send_jobs j
		LEFT JOIN campaigns c ON j.campaign_id = c.id
		LEFT JOIN recipient_lists rl ON j.recipient_list_id = rl.id
		WHERE 1=1`

	args = []any{}
	if filter.CampaignID != "" {
		query += " AND j.campaign_id = ?"
		args = append(args, filter.CampaignID)
	}
	if filter.Status != "" {
		query += " AND j.status = ?"
		args = append(args, filter.Status)
	}

	query += " ORDER BY j.created_at DESC"

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

	jobs := []models.SendJob{}
	for rows.Next() {
		var job models.SendJob
		var scheduledAt, startedAt, completedAt sql.NullTime
		var campaignName, listName sql.NullString

		err := rows.Scan(&job.ID, &job.CampaignID, &campaignName, &job.RecipientListID, &listName, &job.Status,
			&scheduledAt, &startedAt, &completedAt, &job.Servers, &job.Strategy, &job.Stats,
			&job.DryRun, &job.DryRunLimit, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}

		if campaignName.Valid {
			job.CampaignName = campaignName.String
		}
		if listName.Valid {
			job.ListName = listName.String
		}
		if scheduledAt.Valid {
			job.ScheduledAt = &scheduledAt.Time
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, job)
	}

	return jobs, total, nil
}

// UpdateStatus updates job status
func (r *JobRepository) UpdateStatus(id, status string) error {
	now := time.Now()
	var startedAt, completedAt *time.Time

	switch status {
	case "running":
		startedAt = &now
	case "completed", "failed", "cancelled":
		completedAt = &now
	}

	_, err := r.db.Exec(`
		UPDATE send_jobs SET status = ?, started_at = COALESCE(?, started_at), completed_at = ?, updated_at = ?
		WHERE id = ?`,
		status, startedAt, completedAt, now, id,
	)
	return err
}

// UpdateStats updates job statistics
func (r *JobRepository) UpdateStats(id string, stats models.JobStats) error {
	statsJSON, _ := json.Marshal(stats)
	_, err := r.db.Exec("UPDATE send_jobs SET stats = ?, updated_at = ? WHERE id = ?",
		string(statsJSON), time.Now(), id)
	return err
}

// Delete deletes a job
func (r *JobRepository) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM send_jobs WHERE id = ?", id)
	return err
}

// CreateItem creates a job item
func (r *JobRepository) CreateItem(item *models.SendJobItem) error {
	item.ID = uuid.New().String()
	item.Status = "pending"
	item.CreatedAt = time.Now()

	_, err := r.db.Exec(`
		INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.JobID, item.RecipientID, item.VariantID, item.ServerName, item.Status, item.CreatedAt,
	)
	return err
}

// CreateItems creates multiple job items in a batch
func (r *JobRepository) CreateItems(items []models.SendJobItem) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for i := range items {
		items[i].ID = uuid.New().String()
		items[i].Status = "pending"
		items[i].CreatedAt = now

		_, err := stmt.Exec(items[i].ID, items[i].JobID, items[i].RecipientID, items[i].VariantID, items[i].ServerName, items[i].Status, items[i].CreatedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListItems returns job items with filtering
func (r *JobRepository) ListItems(filter models.JobItemFilter) ([]models.SendJobItem, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM send_job_items WHERE job_id = ?"
	args := []any{filter.JobID}

	if filter.Status != "" {
		countQuery += " AND status = ?"
		args = append(args, filter.Status)
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get items
	query := `
		SELECT i.id, i.job_id, i.recipient_id, r.email, i.variant_id, COALESCE(v.name, ''), i.server_name, i.status,
			i.sendry_msg_id, i.error, i.queued_at, i.sent_at, i.created_at
		FROM send_job_items i
		LEFT JOIN recipients r ON i.recipient_id = r.id
		LEFT JOIN campaign_variants v ON i.variant_id = v.id
		WHERE i.job_id = ?`

	args = []any{filter.JobID}
	if filter.Status != "" {
		query += " AND i.status = ?"
		args = append(args, filter.Status)
	}

	query += " ORDER BY i.created_at"

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

	items := []models.SendJobItem{}
	for rows.Next() {
		var item models.SendJobItem
		var email, variantName sql.NullString
		var queuedAt, sentAt sql.NullTime

		err := rows.Scan(&item.ID, &item.JobID, &item.RecipientID, &email, &item.VariantID, &variantName,
			&item.ServerName, &item.Status, &item.SendryMsgID, &item.Error, &queuedAt, &sentAt, &item.CreatedAt)
		if err != nil {
			return nil, 0, err
		}

		if email.Valid {
			item.Email = email.String
		}
		if variantName.Valid {
			item.VariantName = variantName.String
		}
		if queuedAt.Valid {
			item.QueuedAt = &queuedAt.Time
		}
		if sentAt.Valid {
			item.SentAt = &sentAt.Time
		}

		items = append(items, item)
	}

	return items, total, nil
}

// UpdateItemStatus updates a job item status
func (r *JobRepository) UpdateItemStatus(id, status, sendryMsgID, errorMsg string) error {
	now := time.Now()
	var queuedAt, sentAt *time.Time

	switch status {
	case "queued":
		queuedAt = &now
	case "sent":
		sentAt = &now
	}

	_, err := r.db.Exec(`
		UPDATE send_job_items SET status = ?, sendry_msg_id = ?, error = ?,
			queued_at = COALESCE(?, queued_at), sent_at = ?
		WHERE id = ?`,
		status, sendryMsgID, errorMsg, queuedAt, sentAt, id,
	)
	return err
}

// GetStats returns aggregated stats for a job
func (r *JobRepository) GetStats(jobID string) (models.JobStats, error) {
	var stats models.JobStats

	err := r.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
			SUM(CASE WHEN status = 'queued' THEN 1 ELSE 0 END) as queued,
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
		FROM send_job_items WHERE job_id = ?`, jobID,
	).Scan(&stats.Total, &stats.Pending, &stats.Queued, &stats.Sent, &stats.Failed)

	return stats, err
}

// GetRunningJobs returns all jobs with status 'running'
func (r *JobRepository) GetRunningJobs() ([]models.SendJob, error) {
	rows, err := r.db.Query(`
		SELECT j.id, j.campaign_id, c.name, j.recipient_list_id, COALESCE(rl.name, ''), j.status,
			j.scheduled_at, j.started_at, j.completed_at, j.servers, j.strategy, COALESCE(j.stats, '{}'), j.created_at, j.updated_at
		FROM send_jobs j
		LEFT JOIN campaigns c ON j.campaign_id = c.id
		LEFT JOIN recipient_lists rl ON j.recipient_list_id = rl.id
		WHERE j.status = 'running'
		ORDER BY j.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []models.SendJob{}
	for rows.Next() {
		var job models.SendJob
		var scheduledAt, startedAt, completedAt sql.NullTime
		var campaignName, listName sql.NullString

		err := rows.Scan(&job.ID, &job.CampaignID, &campaignName, &job.RecipientListID, &listName, &job.Status,
			&scheduledAt, &startedAt, &completedAt, &job.Servers, &job.Strategy, &job.Stats, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return nil, err
		}

		if campaignName.Valid {
			job.CampaignName = campaignName.String
		}
		if listName.Valid {
			job.ListName = listName.String
		}
		if scheduledAt.Valid {
			job.ScheduledAt = &scheduledAt.Time
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

// GetScheduledJobsDue returns jobs with status 'scheduled' and scheduled_at <= now
func (r *JobRepository) GetScheduledJobsDue() ([]models.SendJob, error) {
	rows, err := r.db.Query(`
		SELECT j.id, j.campaign_id, c.name, j.recipient_list_id, COALESCE(rl.name, ''), j.status,
			j.scheduled_at, j.started_at, j.completed_at, j.servers, j.strategy, COALESCE(j.stats, '{}'), j.created_at, j.updated_at
		FROM send_jobs j
		LEFT JOIN campaigns c ON j.campaign_id = c.id
		LEFT JOIN recipient_lists rl ON j.recipient_list_id = rl.id
		WHERE j.status = 'scheduled' AND j.scheduled_at <= datetime('now')
		ORDER BY j.scheduled_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []models.SendJob{}
	for rows.Next() {
		var job models.SendJob
		var scheduledAt, startedAt, completedAt sql.NullTime
		var campaignName, listName sql.NullString

		err := rows.Scan(&job.ID, &job.CampaignID, &campaignName, &job.RecipientListID, &listName, &job.Status,
			&scheduledAt, &startedAt, &completedAt, &job.Servers, &job.Strategy, &job.Stats, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return nil, err
		}

		if campaignName.Valid {
			job.CampaignName = campaignName.String
		}
		if listName.Valid {
			job.ListName = listName.String
		}
		if scheduledAt.Valid {
			job.ScheduledAt = &scheduledAt.Time
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

// GetPendingItems returns pending items for processing
func (r *JobRepository) GetPendingItems(jobID string, limit int) ([]models.SendJobItem, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.job_id, i.recipient_id, r.email, COALESCE(r.name, ''), COALESCE(r.variables, ''),
			i.variant_id, i.server_name, i.status, i.created_at
		FROM send_job_items i
		LEFT JOIN recipients r ON i.recipient_id = r.id
		WHERE i.job_id = ? AND i.status = 'pending'
		ORDER BY i.created_at
		LIMIT ?`, jobID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.SendJobItem{}
	for rows.Next() {
		var item models.SendJobItem
		var email, name, variables sql.NullString
		err := rows.Scan(&item.ID, &item.JobID, &item.RecipientID, &email, &name, &variables,
			&item.VariantID, &item.ServerName, &item.Status, &item.CreatedAt)
		if err != nil {
			return nil, err
		}
		if email.Valid {
			item.Email = email.String
		}
		if name.Valid {
			item.RecipientName = name.String
		}
		if variables.Valid {
			item.RecipientVariables = variables.String
		}
		items = append(items, item)
	}

	return items, nil
}

// GetQueuedItems returns items with status 'queued' for status tracking
func (r *JobRepository) GetQueuedItems(limit int) ([]models.SendJobItem, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.job_id, i.recipient_id, r.email, i.variant_id, i.server_name,
			i.status, i.sendry_msg_id, i.created_at
		FROM send_job_items i
		LEFT JOIN recipients r ON i.recipient_id = r.id
		WHERE i.status = 'queued' AND i.sendry_msg_id != ''
		ORDER BY i.created_at
		LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.SendJobItem{}
	for rows.Next() {
		var item models.SendJobItem
		var email sql.NullString
		err := rows.Scan(&item.ID, &item.JobID, &item.RecipientID, &email, &item.VariantID,
			&item.ServerName, &item.Status, &item.SendryMsgID, &item.CreatedAt)
		if err != nil {
			return nil, err
		}
		if email.Valid {
			item.Email = email.String
		}
		items = append(items, item)
	}

	return items, nil
}
