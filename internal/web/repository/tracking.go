package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

// TrackingRepository stores engagement events, per-message counters and
// job variant snapshots.
type TrackingRepository struct {
	db *sql.DB
}

func NewTrackingRepository(db *sql.DB) *TrackingRepository {
	return &TrackingRepository{db: db}
}

// ResolveSource loads the context of a tracked message: recipient email and,
// for job items, campaign/job/variant ids, campaign name and from fields.
// For sends Email is the first "to" address. Returns (nil, nil) if not found.
func (r *TrackingRepository) ResolveSource(sourceType, sourceID string) (*models.TrackingSource, error) {
	switch sourceType {
	case models.SourceJobItem:
		src := &models.TrackingSource{SourceType: sourceType, SourceID: sourceID}
		var email, campaignName, fromName, fromEmail, campaignID, variantID, listID, recipientID sql.NullString
		var sentAt sql.NullTime
		err := r.db.QueryRow(`
			SELECT rc.email, j.campaign_id, c.name, c.from_name, c.from_email, i.job_id, i.variant_id, i.sent_at,
				j.recipient_list_id, i.recipient_id
			FROM send_job_items i
			JOIN send_jobs j ON j.id = i.job_id
			LEFT JOIN recipients rc ON rc.id = i.recipient_id
			LEFT JOIN campaigns c ON c.id = j.campaign_id
			WHERE i.id = ?`, sourceID,
		).Scan(&email, &campaignID, &campaignName, &fromName, &fromEmail, &src.JobID, &variantID, &sentAt, &listID, &recipientID)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		src.Email = email.String
		src.CampaignID = campaignID.String
		src.CampaignName = campaignName.String
		src.FromName = fromName.String
		src.FromEmail = fromEmail.String
		src.VariantID = variantID.String
		src.ListID = listID.String
		src.RecipientID = recipientID.String
		if sentAt.Valid {
			src.SentAt = &sentAt.Time
		}
		return src, nil

	case models.SourceSend:
		src := &models.TrackingSource{SourceType: sourceType, SourceID: sourceID}
		var to string
		var sentAt sql.NullTime
		err := r.db.QueryRow(
			`SELECT from_address, to_addresses, sent_at FROM sends WHERE id = ?`, sourceID,
		).Scan(&src.FromEmail, &to, &sentAt)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var list []string
		if json.Unmarshal([]byte(to), &list) == nil && len(list) > 0 {
			src.Email = list[0]
		} else {
			src.Email = to
		}
		if sentAt.Valid {
			src.SentAt = &sentAt.Time
		}
		return src, nil
	}
	return nil, nil
}

// RecordEvent inserts e into email_events (CreatedAt defaults to now) and, in
// the same transaction, updates the counters of the source row
// (send_job_items or sends) unless e.IsMachine:
//   - open: open_count++, opened_at set if NULL
//   - click: click_count++, clicked_at set if NULL; a click also implies an
//     open, so opened_at is set if NULL (open_count unchanged)
//   - unsubscribe: unsubscribed_at set if NULL (send_job_items only)
func (r *TrackingRepository) RecordEvent(e *models.EmailEvent) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO email_events (type, source_type, source_id, campaign_id, job_id, variant_id,
			email, link_id, url, ip, user_agent, is_machine, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Type, e.SourceType, e.SourceID, nullString(e.CampaignID), nullString(e.JobID), nullString(e.VariantID),
		e.Email, e.LinkID, nullString(e.URL), nullString(e.IP), nullString(e.UserAgent), e.IsMachine, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}
	if id, err := res.LastInsertId(); err == nil {
		e.ID = id
	}

	if !e.IsMachine {
		var table string
		switch e.SourceType {
		case models.SourceJobItem:
			table = "send_job_items"
		case models.SourceSend:
			table = "sends"
		}
		var query string
		switch {
		case table == "":
		case e.Type == models.EventOpen:
			query = "UPDATE " + table + " SET open_count = open_count + 1, opened_at = COALESCE(opened_at, ?) WHERE id = ?"
		case e.Type == models.EventClick:
			query = "UPDATE " + table + " SET click_count = click_count + 1, clicked_at = COALESCE(clicked_at, ?), opened_at = COALESCE(opened_at, ?) WHERE id = ?"
		case e.Type == models.EventUnsubscribe && table == "send_job_items":
			query = "UPDATE " + table + " SET unsubscribed_at = COALESCE(unsubscribed_at, ?) WHERE id = ?"
		}
		if query != "" {
			args := []any{e.CreatedAt}
			if e.Type == models.EventClick {
				args = append(args, e.CreatedAt)
			}
			args = append(args, e.SourceID)
			if _, err := tx.Exec(query, args...); err != nil {
				return fmt.Errorf("failed to update counters: %w", err)
			}
		}
	}

	return tx.Commit()
}

// SourceEvents returns events of one message, newest first.
func (r *TrackingRepository) SourceEvents(sourceType, sourceID string, limit int) ([]models.EmailEvent, error) {
	query := `
		SELECT id, type, source_type, source_id, COALESCE(campaign_id, ''), COALESCE(job_id, ''), COALESCE(variant_id, ''),
			email, link_id, COALESCE(url, ''), COALESCE(ip, ''), COALESCE(user_agent, ''), is_machine, created_at
		FROM email_events WHERE source_type = ? AND source_id = ?
		ORDER BY created_at DESC, id DESC`
	args := []any{sourceType, sourceID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []models.EmailEvent{}
	for rows.Next() {
		var e models.EmailEvent
		var linkID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Type, &e.SourceType, &e.SourceID, &e.CampaignID, &e.JobID, &e.VariantID,
			&e.Email, &linkID, &e.URL, &e.IP, &e.UserAgent, &e.IsMachine, &e.CreatedAt); err != nil {
			return nil, err
		}
		if linkID.Valid {
			v := int(linkID.Int64)
			e.LinkID = &v
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// stats aggregates engagement for the jobs matched by jobWhere (alias j)
// and the events matched by eventWhere.
func (r *TrackingRepository) stats(jobWhere, eventWhere string, arg string) (models.TrackingStats, error) {
	var st models.TrackingStats
	err := r.db.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN i.status = 'sent' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN i.opened_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN i.clicked_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN i.unsubscribed_at IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM send_job_items i JOIN send_jobs j ON j.id = i.job_id
		WHERE `+jobWhere, arg,
	).Scan(&st.Total, &st.Delivered, &st.UniqueOpens, &st.UniqueClicks, &st.Unsubscribed)
	if err != nil {
		return st, err
	}

	rows, err := r.db.Query(`
		SELECT type, COUNT(*) FROM email_events
		WHERE `+eventWhere+` AND is_machine = 0 AND source_type = ? GROUP BY type`, arg, models.SourceJobItem)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			return st, err
		}
		switch typ {
		case models.EventOpen:
			st.TotalOpens = n
		case models.EventClick:
			st.TotalClicks = n
		}
	}
	if err := rows.Err(); err != nil {
		return st, err
	}

	err = r.db.QueryRow(
		`SELECT COALESCE(SUM(skipped_unsubscribed), 0) FROM send_jobs j WHERE `+jobWhere, arg,
	).Scan(&st.SkippedUnsub)
	return st, err
}

// JobStats aggregates engagement of one job from send_job_items counters
// (unique = rows with opened_at / clicked_at / unsubscribed_at set) and
// email_events (totals, is_machine = 0). Delivered = items with status 'sent'.
// SkippedUnsub comes from send_jobs.skipped_unsubscribed.
func (r *TrackingRepository) JobStats(jobID string) (models.TrackingStats, error) {
	return r.stats("j.id = ?", "job_id = ?", jobID)
}

// CampaignStats is JobStats summed over all jobs of a campaign.
func (r *TrackingRepository) CampaignStats(campaignID string) (models.TrackingStats, error) {
	return r.stats("j.campaign_id = ?", "campaign_id = ?", campaignID)
}

// JobLinkStats returns click counts per link_id for a job, optionally for one
// variant (variantID == "" = all). Labels/kinds/urls are taken from the job's
// snapshot links; links without clicks are included with zeros; clicks on
// link ids missing from the snapshot are included with the event URL.
// Ordered by link_id. Machine clicks are excluded.
func (r *TrackingRepository) JobLinkStats(jobID, variantID string) ([]models.LinkStat, error) {
	snaps, err := r.GetSnapshots(jobID)
	if err != nil {
		return nil, err
	}
	byID := map[int]*models.LinkStat{}
	for _, s := range snaps {
		if variantID != "" && s.VariantID != variantID {
			continue
		}
		for _, l := range s.Links {
			if _, ok := byID[l.ID]; ok {
				continue // first label wins
			}
			byID[l.ID] = &models.LinkStat{LinkID: l.ID, Label: l.Label, Kind: l.Kind, URL: l.URL}
		}
	}

	query := `
		SELECT link_id, COUNT(DISTINCT source_id), COUNT(*), COALESCE(MAX(url), '')
		FROM email_events
		WHERE job_id = ? AND type = ? AND is_machine = 0 AND link_id IS NOT NULL`
	args := []any{jobID, models.EventClick}
	if variantID != "" {
		query += " AND variant_id = ?"
		args = append(args, variantID)
	}
	query += " GROUP BY link_id"
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, unique, total int
		var url string
		if err := rows.Scan(&id, &unique, &total, &url); err != nil {
			return nil, err
		}
		ls := byID[id]
		if ls == nil {
			ls = &models.LinkStat{LinkID: id, URL: url}
			byID[id] = ls
		}
		ls.UniqueClicks = unique
		ls.TotalClicks = total
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.LinkStat, 0, len(byID))
	for _, ls := range byID {
		out = append(out, *ls)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LinkID < out[j].LinkID })
	return out, nil
}

// SaveSnapshot upserts a job variant snapshot (links stored as JSON).
func (r *TrackingRepository) SaveSnapshot(s *models.JobVariantSnapshot) error {
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
		INSERT INTO job_variant_snapshots (job_id, variant_id, variant_name, template_id, template_version, html, links, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id, variant_id) DO UPDATE SET
			variant_name = excluded.variant_name, template_id = excluded.template_id,
			template_version = excluded.template_version, html = excluded.html,
			links = excluded.links, created_at = excluded.created_at`,
		s.JobID, s.VariantID, s.VariantName, s.TemplateID, s.TemplateVersion, s.HTML, string(linksJSON), s.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}
	return nil
}

// GetSnapshots returns all variant snapshots of a job ordered by variant_name.
func (r *TrackingRepository) GetSnapshots(jobID string) ([]models.JobVariantSnapshot, error) {
	rows, err := r.db.Query(`
		SELECT job_id, variant_id, COALESCE(variant_name, ''), COALESCE(template_id, ''), COALESCE(template_version, 0),
			html, COALESCE(links, '[]'), created_at
		FROM job_variant_snapshots WHERE job_id = ? ORDER BY variant_name, variant_id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.JobVariantSnapshot{}
	for rows.Next() {
		var s models.JobVariantSnapshot
		var links string
		if err := rows.Scan(&s.JobID, &s.VariantID, &s.VariantName, &s.TemplateID, &s.TemplateVersion,
			&s.HTML, &links, &s.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(links), &s.Links); err != nil {
			return nil, fmt.Errorf("bad snapshot links: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
