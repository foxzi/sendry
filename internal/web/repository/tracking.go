package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

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
// for job items, campaign/job/variant ids and from fields. SentAt is when the
// message was handed to the MTA: queued_at of a job item, created_at of a
// send (API sends are handed over synchronously).
// For sends Email is the first "to" address. Returns (nil, nil) if not found.
func (r *TrackingRepository) ResolveSource(sourceType, sourceID string) (*models.TrackingSource, error) {
	switch sourceType {
	case models.SourceJobItem:
		src := &models.TrackingSource{SourceType: sourceType, SourceID: sourceID}
		var email, fromName, fromEmail, campaignID, variantID, listID, recipientID sql.NullString
		var sentAt sql.NullTime
		err := r.db.QueryRow(`
			SELECT rc.email, j.campaign_id, c.from_name, c.from_email, i.job_id, i.variant_id, i.queued_at,
				j.recipient_list_id, i.recipient_id
			FROM send_job_items i
			JOIN send_jobs j ON j.id = i.job_id
			LEFT JOIN recipients rc ON rc.id = i.recipient_id
			LEFT JOIN campaigns c ON c.id = j.campaign_id
			WHERE i.id = ?`, sourceID,
		).Scan(&email, &campaignID, &fromName, &fromEmail, &src.JobID, &variantID, &sentAt, &listID, &recipientID)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		src.Email = email.String
		src.CampaignID = campaignID.String
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
			`SELECT from_address, to_addresses, created_at FROM sends WHERE id = ?`, sourceID,
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

// Limits of stored request data: the public endpoints take whatever the
// client sends, so long values are cut before they reach the database.
const (
	maxEventUserAgent = 512
	maxEventURL       = 2048
)

// EventDedupWindow: a repeat of the same event (same message, type, link and
// IP) within this window is dropped, so reloads and hammering the public
// endpoints neither fill the disk nor inflate statistics.
const EventDedupWindow = 30 * time.Second

// truncateUTF8 cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// RecordEvent inserts e into email_events (CreatedAt defaults to now) and, in
// the same transaction, updates the counters of the source row
// (send_job_items or sends) unless e.IsMachine:
//   - open: open_count++, opened_at set if NULL
//   - click: click_count++, clicked_at set if NULL; a click also implies an
//     open, so opened_at is set if NULL (open_count unchanged)
//   - unsubscribe: unsubscribed_at set if NULL (send_job_items only)
//
// UserAgent and URL are truncated. If the same event (source, type, link_id,
// IP) was already recorded within EventDedupWindow, nothing is written:
// neither the event nor the counters; e.ID stays 0 in that case.
func (r *TrackingRepository) RecordEvent(e *models.EmailEvent) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	e.CreatedAt = e.CreatedAt.UTC()
	e.UserAgent = truncateUTF8(e.UserAgent, maxEventUserAgent)
	e.URL = truncateUTF8(e.URL, maxEventURL)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if e.IP != "" {
		var linkID any
		if e.LinkID != nil {
			linkID = *e.LinkID
		}
		var dup int
		err := tx.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT 1 FROM email_events
				WHERE source_type = ? AND source_id = ? AND type = ? AND created_at >= ?
					AND link_id IS ? AND ip = ?
				LIMIT 1)`,
			e.SourceType, e.SourceID, e.Type, e.CreatedAt.Add(-EventDedupWindow), linkID, e.IP,
		).Scan(&dup)
		if err != nil {
			return fmt.Errorf("failed to check duplicate event: %w", err)
		}
		if dup > 0 {
			return nil
		}
	}

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
		if err := updateEventCounters(tx, e); err != nil {
			return fmt.Errorf("failed to update counters: %w", err)
		}
	}

	return tx.Commit()
}

// updateEventCounters applies a human event to the counters of its source row.
func updateEventCounters(tx *sql.Tx, e *models.EmailEvent) error {
	var err error
	at := e.CreatedAt
	switch e.SourceType {
	case models.SourceJobItem:
		switch e.Type {
		case models.EventOpen:
			_, err = tx.Exec(`UPDATE send_job_items SET open_count = open_count + 1, opened_at = COALESCE(opened_at, ?) WHERE id = ?`,
				at, e.SourceID)
		case models.EventClick:
			_, err = tx.Exec(`UPDATE send_job_items SET click_count = click_count + 1, clicked_at = COALESCE(clicked_at, ?),
				opened_at = COALESCE(opened_at, ?) WHERE id = ?`, at, at, e.SourceID)
		case models.EventUnsubscribe:
			_, err = tx.Exec(`UPDATE send_job_items SET unsubscribed_at = COALESCE(unsubscribed_at, ?) WHERE id = ?`,
				at, e.SourceID)
		}
	case models.SourceSend:
		switch e.Type {
		case models.EventOpen:
			_, err = tx.Exec(`UPDATE sends SET open_count = open_count + 1, opened_at = COALESCE(opened_at, ?) WHERE id = ?`,
				at, e.SourceID)
		case models.EventClick:
			_, err = tx.Exec(`UPDATE sends SET click_count = click_count + 1, clicked_at = COALESCE(clicked_at, ?),
				opened_at = COALESCE(opened_at, ?) WHERE id = ?`, at, at, e.SourceID)
		}
	}
	return err
}

// DeleteCampaignEvents removes the engagement events of all jobs of a
// campaign. Call it when a campaign is deleted: email_events has no foreign
// keys.
func (r *TrackingRepository) DeleteCampaignEvents(campaignID string) error {
	_, err := r.db.Exec(`DELETE FROM email_events WHERE campaign_id = ?`, campaignID)
	return err
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

// stats aggregates engagement over the job items matched by jobWhere
// (aliases i = send_job_items, j = send_jobs, c = campaigns). Everything is
// counted over the same base, items accepted by the MTA (status queued or
// sent), so rates never exceed 100%. Opens are counted only when the
// campaign tracks opens: a click implies an open, but without the pixel an
// open rate would be made of clicks alone. Totals come from the per-item
// counters, which hold human events only. Skipped items are not messages:
// they are left out of Total, and the unsubscribed ones are counted as
// distinct recipients in SkippedUnsub.
func (r *TrackingRepository) stats(jobWhere string, arg string) (models.TrackingStats, error) {
	var st models.TrackingStats
	err := r.db.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN status != 'skipped' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(acc), 0),
			COALESCE(SUM(CASE WHEN acc = 1 AND opens = 1 AND opened_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN acc = 1 AND opens = 1 THEN open_count ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN acc = 1 AND clicked_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN acc = 1 THEN click_count ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN acc = 1 AND unsubscribed_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COUNT(DISTINCT CASE WHEN status = 'skipped' AND error = ? THEN recipient_id END)
		FROM (
			SELECT CASE WHEN i.status IN ('queued', 'sent') THEN 1 ELSE 0 END AS acc,
				COALESCE(c.track_opens, 1) AS opens, i.status, i.error, i.recipient_id,
				i.opened_at, i.open_count, i.clicked_at, i.click_count, i.unsubscribed_at
			FROM send_job_items i
			JOIN send_jobs j ON j.id = i.job_id
			LEFT JOIN campaigns c ON c.id = j.campaign_id
			WHERE `+jobWhere+`
		)`, models.SkipReasonUnsubscribed, arg,
	).Scan(&st.Total, &st.Delivered, &st.UniqueOpens, &st.TotalOpens, &st.UniqueClicks, &st.TotalClicks, &st.Unsubscribed, &st.SkippedUnsub)
	if err != nil {
		return st, err
	}

	err = r.db.QueryRow(`SELECT COUNT(*) FROM send_jobs j WHERE `+jobWhere, arg).Scan(&st.Jobs)
	return st, err
}

// JobStats aggregates engagement of one job (see stats).
func (r *TrackingRepository) JobStats(jobID string) (models.TrackingStats, error) {
	return r.stats("j.id = ?", jobID)
}

// CampaignStats aggregates engagement over the real (not dry-run) jobs of a
// campaign. SkippedUnsub counts distinct recipients: an address skipped by
// several jobs counts once.
func (r *TrackingRepository) CampaignStats(campaignID string) (models.TrackingStats, error) {
	return r.stats("j.campaign_id = ? AND COALESCE(j.dry_run, 0) = 0", campaignID)
}

// JobLinkStats returns click counts per link_id for a job, optionally for one
// variant (variantID == "" = all). Labels/kinds/urls are taken from the job's
// snapshot links; links without clicks are included with zeros; clicks on
// link ids missing from the snapshot are included with the event URL.
// Ordered by link_id. Machine clicks are excluded. snaps are the job's
// snapshots if the caller already loaded them; nil loads them.
func (r *TrackingRepository) JobLinkStats(jobID, variantID string, snaps []models.JobVariantSnapshot) ([]models.LinkStat, error) {
	if snaps == nil {
		var err error
		if snaps, err = r.GetSnapshots(jobID); err != nil {
			return nil, err
		}
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
