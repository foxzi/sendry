package repository

import (
	"database/sql"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

// The recipient card shows one address across all address books: recipients
// are matched by lower(email), which idx_recipients_email covers, and their
// job items by recipient_id (idx_send_job_items_recipient).

// GetRecipient returns a recipient of a list, (nil, nil) if there is none.
func (r *RecipientRepository) GetRecipient(listID, id string) (*models.Recipient, error) {
	rec := &models.Recipient{}
	var name, variables, tags, status sql.NullString
	err := r.db.QueryRow(`
		SELECT id, list_id, email, name, variables, tags, status, created_at
		FROM recipients WHERE id = ? AND list_id = ?`, id, listID,
	).Scan(&rec.ID, &rec.ListID, &rec.Email, &name, &variables, &tags, &status, &rec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.Name, rec.Variables, rec.Tags, rec.Status = name.String, variables.String, tags.String, status.String
	return rec, nil
}

// UpdateRecipientName renames a recipient of a list.
func (r *RecipientRepository) UpdateRecipientName(listID, id, name string) error {
	_, err := r.db.Exec(`UPDATE recipients SET name = ? WHERE id = ? AND list_id = ?`, name, id, listID)
	return err
}

// Memberships returns every list the address is in, newest record first.
func (r *RecipientRepository) Memberships(email string) ([]models.RecipientMembership, error) {
	rows, err := r.db.Query(`
		SELECT r.id, r.list_id, COALESCE(l.name, ''), COALESCE(r.status, ''), r.created_at
		FROM recipients r
		LEFT JOIN recipient_lists l ON l.id = r.list_id
		WHERE lower(r.email) = ?
		ORDER BY r.created_at DESC, r.id`, NormalizeEmail(email))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.RecipientMembership{}
	for rows.Next() {
		var m models.RecipientMembership
		if err := rows.Scan(&m.RecipientID, &m.ListID, &m.ListName, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MessageTotals sums the campaign messages addressed to the address in all lists.
func (r *RecipientRepository) MessageTotals(email string) (models.RecipientTotals, error) {
	email = NormalizeEmail(email)
	var t models.RecipientTotals
	err := r.db.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(i.status = 'sent'), 0),
			COALESCE(SUM(i.opened_at IS NOT NULL), 0),
			COALESCE(SUM(i.clicked_at IS NOT NULL), 0)
		FROM send_job_items i
		JOIN recipients r ON r.id = i.recipient_id
		WHERE lower(r.email) = ?`, email,
	).Scan(&t.Messages, &t.Sent, &t.Opened, &t.Clicked)
	if err != nil {
		return t, err
	}

	// a plain column keeps its timestamp type, MAX() would return text
	var last sql.NullTime
	err = r.db.QueryRow(`
		SELECT i.sent_at FROM send_job_items i
		JOIN recipients r ON r.id = i.recipient_id
		WHERE lower(r.email) = ? AND i.sent_at IS NOT NULL
		ORDER BY i.sent_at DESC LIMIT 1`, email,
	).Scan(&last)
	if err != nil && err != sql.ErrNoRows {
		return t, err
	}
	if last.Valid {
		t.LastSent = &last.Time
	}
	return t, nil
}

// Messages returns the campaign messages addressed to the address in all
// lists, newest first.
func (r *RecipientRepository) Messages(email string, limit int) ([]models.RecipientMessage, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.job_id, j.campaign_id, COALESCE(c.name, ''), r.list_id, COALESCE(l.name, ''),
			COALESCE(v.name, ''), COALESCE(i.status, ''), COALESCE(i.error, ''),
			i.sent_at, i.queued_at, i.created_at,
			i.opened_at, COALESCE(i.open_count, 0), i.clicked_at, COALESCE(i.click_count, 0), i.unsubscribed_at
		FROM send_job_items i
		JOIN recipients r ON r.id = i.recipient_id
		JOIN send_jobs j ON j.id = i.job_id
		LEFT JOIN campaigns c ON c.id = j.campaign_id
		LEFT JOIN recipient_lists l ON l.id = r.list_id
		LEFT JOIN campaign_variants v ON v.id = i.variant_id
		WHERE lower(r.email) = ?
		ORDER BY COALESCE(i.sent_at, i.queued_at, i.created_at) DESC, i.id
		LIMIT ?`, NormalizeEmail(email), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.RecipientMessage{}
	for rows.Next() {
		var m models.RecipientMessage
		var sentAt, queuedAt, openedAt, clickedAt, unsubAt sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(&m.ItemID, &m.JobID, &m.CampaignID, &m.CampaignName, &m.ListID, &m.ListName,
			&m.VariantName, &m.Status, &m.Error, &sentAt, &queuedAt, &createdAt,
			&openedAt, &m.OpenCount, &clickedAt, &m.ClickCount, &unsubAt); err != nil {
			return nil, err
		}
		switch {
		case sentAt.Valid:
			m.At = sentAt.Time
		case queuedAt.Valid:
			m.At = queuedAt.Time
		default:
			m.At = createdAt
		}
		if openedAt.Valid {
			m.OpenedAt = &openedAt.Time
		}
		if clickedAt.Valid {
			m.ClickedAt = &clickedAt.Time
		}
		if unsubAt.Valid {
			m.UnsubscribedAt = &unsubAt.Time
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Events returns the opens, clicks and unsubscribes of the campaign messages
// addressed to the address in all lists, newest first.
func (r *RecipientRepository) Events(email string, limit int) ([]models.RecipientEvent, error) {
	rows, err := r.db.Query(`
		SELECT e.id, e.type, e.source_type, e.source_id, COALESCE(e.campaign_id, ''), COALESCE(c.name, ''),
			COALESCE(e.job_id, ''), e.email, e.link_id, COALESCE(e.url, ''), COALESCE(e.ip, ''),
			COALESCE(e.user_agent, ''), e.is_machine, e.created_at
		FROM email_events e
		LEFT JOIN campaigns c ON c.id = e.campaign_id
		WHERE e.source_type = ? AND e.source_id IN (
			SELECT i.id FROM send_job_items i JOIN recipients r ON r.id = i.recipient_id WHERE lower(r.email) = ?)
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ?`, models.SourceJobItem, NormalizeEmail(email), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.RecipientEvent{}
	for rows.Next() {
		var e models.RecipientEvent
		var linkID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Type, &e.SourceType, &e.SourceID, &e.CampaignID, &e.CampaignName,
			&e.JobID, &e.Email, &linkID, &e.URL, &e.IP, &e.UserAgent, &e.IsMachine, &e.CreatedAt); err != nil {
			return nil, err
		}
		if linkID.Valid {
			v := int(linkID.Int64)
			e.LinkID = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Get returns the journal row of an unsubscribe of email in listID,
// (nil, nil) if the address did not unsubscribe there.
func (r *UnsubscribeRepository) Get(listID, email string) (*models.Unsubscribe, error) {
	u := &models.Unsubscribe{ListID: listID}
	var recipientID, campaignID, jobID, sourceID sql.NullString
	err := r.db.QueryRow(`
		SELECT id, recipient_id, email, campaign_id, job_id, source_type, source_id, created_at
		FROM unsubscribes WHERE list_id = ? AND email = ?`, listID, NormalizeEmail(email),
	).Scan(&u.ID, &recipientID, &u.Email, &campaignID, &jobID, &u.SourceType, &sourceID, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.RecipientID, u.CampaignID, u.JobID, u.SourceID = recipientID.String, campaignID.String, jobID.String, sourceID.String
	return u, nil
}
