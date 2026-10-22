package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/stats"
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
	if job.Status == "" {
		job.Status = "draft"
	}
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
			COALESCE(j.dry_run, 0), COALESCE(j.dry_run_limit, 0), j.created_at, j.updated_at,
			COALESCE(j.skipped_unsubscribed, 0)
		FROM send_jobs j
		LEFT JOIN campaigns c ON j.campaign_id = c.id
		LEFT JOIN recipient_lists rl ON j.recipient_list_id = rl.id
		WHERE j.id = ?`, id,
	).Scan(&job.ID, &job.CampaignID, &campaignName, &job.RecipientListID, &listName, &job.Status,
		&scheduledAt, &startedAt, &completedAt, &job.Servers, &job.Strategy, &job.Stats,
		&job.DryRun, &job.DryRunLimit, &job.CreatedAt, &job.UpdatedAt, &job.SkippedUnsubscribed)

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
			COALESCE(j.dry_run, 0), COALESCE(j.dry_run_limit, 0), j.created_at, j.updated_at,
			COALESCE(j.skipped_unsubscribed, 0)
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
			&job.DryRun, &job.DryRunLimit, &job.CreatedAt, &job.UpdatedAt, &job.SkippedUnsubscribed)
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

// RetryFailedItems puts the failed items of a job back to pending, clearing
// their delivery state, and returns how many were reset.
func (r *JobRepository) RetryFailedItems(jobID string) (int64, error) {
	res, err := r.db.Exec(`
		UPDATE send_job_items SET status = ?, error = NULL, sendry_msg_id = NULL, queued_at = NULL, sent_at = NULL
		WHERE job_id = ? AND status = ?`,
		models.JobItemStatusPending, jobID, models.JobItemStatusFailed,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CreateItems creates multiple job items in a batch. Items are pending unless
// Status is set (skipped items carry their reason in Error).
func (r *JobRepository) CreateItems(items []models.SendJobItem) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for i := range items {
		items[i].ID = uuid.New().String()
		if items[i].Status == "" {
			items[i].Status = models.JobItemStatusPending
		}
		items[i].CreatedAt = now

		_, err := stmt.Exec(items[i].ID, items[i].JobID, items[i].RecipientID, items[i].VariantID, items[i].ServerName,
			items[i].Status, nullString(items[i].Error), items[i].CreatedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListItems returns job items with filtering.
// Error type is not stored, so with ErrorType set the items with an error are classified and paged in Go.
func (r *JobRepository) ListItems(filter models.JobItemFilter) ([]models.SendJobItem, int, error) {
	where := " WHERE i.job_id = ?"
	args := []any{filter.JobID}

	if filter.Status != "" {
		where += " AND i.status = ?"
		args = append(args, filter.Status)
	}
	if filter.ErrorType != "" {
		// the error of a skipped item is the skip reason, not a delivery error
		where += " AND COALESCE(i.error, '') != '' AND i.status != 'skipped'"
	}
	// skipped items were never sent, so they are neither opened nor not opened
	switch filter.Opened {
	case "yes":
		where += " AND i.opened_at IS NOT NULL"
	case "no":
		where += " AND i.opened_at IS NULL AND i.status != 'skipped'"
	}
	switch filter.Clicked {
	case "yes":
		where += " AND i.clicked_at IS NOT NULL"
	case "no":
		where += " AND i.clicked_at IS NULL AND i.status != 'skipped'"
	}

	// Count total
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM send_job_items i"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get items
	query := `
		SELECT i.id, i.job_id, i.recipient_id, r.email, i.variant_id, COALESCE(v.name, ''), i.server_name, i.status,
			COALESCE(i.sendry_msg_id, ''), COALESCE(i.error, ''), i.queued_at, i.sent_at, i.created_at,
			i.opened_at, COALESCE(i.open_count, 0), i.clicked_at, COALESCE(i.click_count, 0), i.unsubscribed_at
		FROM send_job_items i
		LEFT JOIN recipients r ON i.recipient_id = r.id
		LEFT JOIN campaign_variants v ON i.variant_id = v.id` + where + " ORDER BY i.created_at"

	if filter.ErrorType == "" {
		if filter.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
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
		var queuedAt, sentAt, openedAt, clickedAt, unsubAt sql.NullTime

		err := rows.Scan(&item.ID, &item.JobID, &item.RecipientID, &email, &item.VariantID, &variantName,
			&item.ServerName, &item.Status, &item.SendryMsgID, &item.Error, &queuedAt, &sentAt, &item.CreatedAt,
			&openedAt, &item.OpenCount, &clickedAt, &item.ClickCount, &unsubAt)
		if err != nil {
			return nil, 0, err
		}

		if filter.ErrorType != "" && stats.ClassifyError(item.Error) != filter.ErrorType {
			continue
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
		setItemEngagement(&item, openedAt, clickedAt, unsubAt)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if filter.ErrorType != "" {
		total = len(items)
		items = items[min(filter.Offset, total):]
		if filter.Limit > 0 && len(items) > filter.Limit {
			items = items[:filter.Limit]
		}
	}

	return items, total, nil
}

// ErrorCounts returns the number of job items per distinct error text and status
func (r *JobRepository) ErrorCounts(jobID string) ([]models.ErrorCount, error) {
	rows, err := r.db.Query(`
		SELECT error, status, COUNT(*)
		FROM send_job_items
		WHERE job_id = ? AND COALESCE(error, '') != '' AND status != 'skipped'
		GROUP BY error, status`, jobID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := []models.ErrorCount{}
	for rows.Next() {
		var c models.ErrorCount
		if err := rows.Scan(&c.Error, &c.Status, &c.Count); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}

	return counts, rows.Err()
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
			queued_at = COALESCE(queued_at, ?), sent_at = ?
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
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN status = 'skipped' THEN 1 ELSE 0 END) as skipped
		FROM send_job_items WHERE job_id = ?`, jobID,
	).Scan(&stats.Total, &stats.Pending, &stats.Queued, &stats.Sent, &stats.Failed, &stats.Skipped)

	return stats, err
}

// GetRunningJobs returns all jobs with status 'running'
func (r *JobRepository) GetRunningJobs() ([]models.SendJob, error) {
	rows, err := r.db.Query(`
		SELECT j.id, j.campaign_id, c.name, j.recipient_list_id, COALESCE(rl.name, ''), j.status,
			j.scheduled_at, j.started_at, j.completed_at, j.servers, j.strategy, COALESCE(j.stats, '{}'), j.created_at, j.updated_at,
			COALESCE(j.skipped_unsubscribed, 0)
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
			&scheduledAt, &startedAt, &completedAt, &job.Servers, &job.Strategy, &job.Stats, &job.CreatedAt, &job.UpdatedAt, &job.SkippedUnsubscribed)
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
			j.scheduled_at, j.started_at, j.completed_at, j.servers, j.strategy, COALESCE(j.stats, '{}'), j.created_at, j.updated_at,
			COALESCE(j.skipped_unsubscribed, 0)
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
			&scheduledAt, &startedAt, &completedAt, &job.Servers, &job.Strategy, &job.Stats, &job.CreatedAt, &job.UpdatedAt, &job.SkippedUnsubscribed)
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

// GetPendingItems returns up to limit pending items whose recipient is still
// active in the list. Pending items of recipients that unsubscribed, bounced or
// were deleted after the job was created are marked skipped on the way, so
// they are never sent.
func (r *JobRepository) GetPendingItems(jobID string, limit int) ([]models.SendJobItem, error) {
	for {
		items, skipped, err := r.pendingBatch(jobID, limit)
		if err != nil {
			return nil, err
		}
		if len(skipped) == 0 {
			return items, nil
		}
		if err := r.skipItems(jobID, skipped); err != nil {
			return nil, err
		}
		if len(items) > 0 {
			return items, nil
		}
		// the whole batch was skipped: look at the next one
	}
}

// pendingBatch reads the next limit pending items of a job and splits them into
// items to send and skip reasons by item id (recipient no longer active).
func (r *JobRepository) pendingBatch(jobID string, limit int) ([]models.SendJobItem, map[string]string, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.job_id, i.recipient_id, r.id, r.email, COALESCE(r.name, ''), COALESCE(r.variables, ''),
			COALESCE(r.status, 'active'), i.variant_id, i.server_name, i.status, i.created_at
		FROM send_job_items i
		LEFT JOIN recipients r ON i.recipient_id = r.id
		WHERE i.job_id = ? AND i.status = 'pending'
		ORDER BY i.created_at
		LIMIT ?`, jobID, limit,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := []models.SendJobItem{}
	skipped := map[string]string{}
	for rows.Next() {
		var item models.SendJobItem
		var recipientID, email, name, variables sql.NullString
		var recipientStatus string
		err := rows.Scan(&item.ID, &item.JobID, &item.RecipientID, &recipientID, &email, &name, &variables,
			&recipientStatus, &item.VariantID, &item.ServerName, &item.Status, &item.CreatedAt)
		if err != nil {
			return nil, nil, err
		}
		if !recipientID.Valid {
			skipped[item.ID] = models.SkipReasonDeleted
			continue
		}
		if reason := models.SkipReason(recipientStatus); reason != "" {
			skipped[item.ID] = reason
			continue
		}
		item.Email = email.String
		item.RecipientName = name.String
		item.RecipientVariables = variables.String
		items = append(items, item)
	}

	return items, skipped, rows.Err()
}

// skipItems marks pending items of a job skipped with their reason and adds
// the unsubscribed ones to send_jobs.skipped_unsubscribed.
func (r *JobRepository) skipItems(jobID string, reasons map[string]string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	unsubscribed := int64(0)
	for id, reason := range reasons {
		res, err := tx.Exec(`UPDATE send_job_items SET status = ?, error = ? WHERE id = ? AND status = 'pending'`,
			models.JobItemStatusSkipped, reason, id)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && reason == models.SkipReasonUnsubscribed {
			unsubscribed += n
		}
	}
	if unsubscribed > 0 {
		if _, err := tx.Exec(`UPDATE send_jobs SET skipped_unsubscribed = skipped_unsubscribed + ?, updated_at = ? WHERE id = ?`,
			unsubscribed, time.Now(), jobID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetQueuedItems returns items with status 'queued' for status tracking, starting after the given rowid.
// Walking by rowid lets the caller cycle through all queued items instead of re-checking the oldest ones.
// It also returns the rowid of the last item, to be passed as afterRowID on the next call.
func (r *JobRepository) GetQueuedItems(afterRowID int64, limit int) ([]models.SendJobItem, int64, error) {
	rows, err := r.db.Query(`
		SELECT i.rowid, i.id, i.job_id, i.recipient_id, r.email, i.variant_id, i.server_name,
			i.status, i.sendry_msg_id, COALESCE(i.error, ''), i.created_at
		FROM send_job_items i
		LEFT JOIN recipients r ON i.recipient_id = r.id
		WHERE i.status = 'queued' AND i.sendry_msg_id != '' AND i.rowid > ?
		ORDER BY i.rowid
		LIMIT ?`, afterRowID, limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []models.SendJobItem{}
	lastRowID := afterRowID
	for rows.Next() {
		var item models.SendJobItem
		var email sql.NullString
		err := rows.Scan(&lastRowID, &item.ID, &item.JobID, &item.RecipientID, &email, &item.VariantID,
			&item.ServerName, &item.Status, &item.SendryMsgID, &item.Error, &item.CreatedAt)
		if err != nil {
			return nil, 0, err
		}
		if email.Valid {
			item.Email = email.String
		}
		items = append(items, item)
	}

	return items, lastRowID, rows.Err()

}

// setItemEngagement copies nullable engagement timestamps into the item
func setItemEngagement(item *models.SendJobItem, openedAt, clickedAt, unsubAt sql.NullTime) {
	if openedAt.Valid {
		item.OpenedAt = &openedAt.Time
	}
	if clickedAt.Valid {
		item.ClickedAt = &clickedAt.Time
	}
	if unsubAt.Valid {
		item.UnsubscribedAt = &unsubAt.Time
	}
}
