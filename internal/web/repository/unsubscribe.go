package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

// ErrRecipientNotInList is returned when an address is not in the address book
var ErrRecipientNotInList = errors.New("recipient is not in this list")

// UnsubscribeRepository manages unsubscribes per address book (recipient
// list). An unsubscribe sets recipients.status = 'unsubscribed' for that list
// only (campaign jobs take active recipients only) and writes a journal row
// in the unsubscribes table. Emails are matched case-insensitively.
type UnsubscribeRepository struct {
	db *sql.DB
}

func NewUnsubscribeRepository(db *sql.DB) *UnsubscribeRepository {
	return &UnsubscribeRepository{db: db}
}

// refreshListCounts recomputes the list counters inside tx (same as
// RecipientRepository.UpdateListCounts).
func refreshListCounts(tx *sql.Tx, listID string) error {
	_, err := tx.Exec(`
		UPDATE recipient_lists SET
			total_count = (SELECT COUNT(*) FROM recipients WHERE list_id = ?),
			active_count = (SELECT COUNT(*) FROM recipients WHERE list_id = ? AND status = 'active'),
			updated_at = ?
		WHERE id = ?`,
		listID, listID, time.Now(), listID,
	)
	return err
}

func nullIfEmpty(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// Add unsubscribes u.Email in list u.ListID in one transaction: sets the
// recipient status to unsubscribed, upserts the journal row (latest campaign,
// job, source and time win), and refreshes the list counters. changed reports
// whether the recipient status actually changed (it was not unsubscribed
// before). Returns ErrRecipientNotInList if the list has no such address.
// CreatedAt defaults to now; RecipientID is filled from the recipients row.
func (r *UnsubscribeRepository) Add(u *models.Unsubscribe) (changed bool, err error) {
	email := strings.ToLower(strings.TrimSpace(u.Email))
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}

	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var recipientID string
	err = tx.QueryRow(
		`SELECT id FROM recipients WHERE list_id = ? AND lower(email) = lower(?) ORDER BY created_at LIMIT 1`,
		u.ListID, email,
	).Scan(&recipientID)
	if err == sql.ErrNoRows {
		return false, ErrRecipientNotInList
	}
	if err != nil {
		return false, err
	}
	u.RecipientID = recipientID
	u.Email = email

	res, err := tx.Exec(
		`UPDATE recipients SET status = ? WHERE list_id = ? AND lower(email) = lower(?) AND status != ?`,
		models.RecipientStatusUnsubscribed, u.ListID, email, models.RecipientStatusUnsubscribed,
	)
	if err != nil {
		return false, fmt.Errorf("failed to update recipient status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	changed = n > 0

	_, err = tx.Exec(`
		INSERT INTO unsubscribes (list_id, recipient_id, email, campaign_id, job_id, source_type, source_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(list_id, email) DO UPDATE SET
			recipient_id = excluded.recipient_id,
			campaign_id = excluded.campaign_id,
			job_id = excluded.job_id,
			source_type = excluded.source_type,
			source_id = excluded.source_id,
			created_at = excluded.created_at`,
		u.ListID, recipientID, email, nullIfEmpty(u.CampaignID), nullIfEmpty(u.JobID),
		u.SourceType, nullIfEmpty(u.SourceID), u.CreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("failed to write unsubscribe journal: %w", err)
	}

	if err := refreshListCounts(tx, u.ListID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

// Remove resubscribes email in listID: status back to active (only if it is
// unsubscribed), journal row deleted, list counters refreshed. No error if
// the address is not unsubscribed.
func (r *UnsubscribeRepository) Remove(listID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE recipients SET status = 'active' WHERE list_id = ? AND lower(email) = lower(?) AND status = ?`,
		listID, email, models.RecipientStatusUnsubscribed,
	); err != nil {
		return fmt.Errorf("failed to restore recipient status: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM unsubscribes WHERE list_id = ? AND email = ?`, listID, email); err != nil {
		return fmt.Errorf("failed to delete unsubscribe journal row: %w", err)
	}
	if err := refreshListCounts(tx, listID); err != nil {
		return err
	}
	return tx.Commit()
}

// IsUnsubscribed reports whether email has status unsubscribed in listID.
func (r *UnsubscribeRepository) IsUnsubscribed(listID, email string) (bool, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM recipients WHERE list_id = ? AND lower(email) = lower(?) AND status = ?`,
		listID, strings.TrimSpace(email), models.RecipientStatusUnsubscribed,
	).Scan(&n)
	return n > 0, err
}

// CountInList returns how many recipients of listID are unsubscribed.
func (r *UnsubscribeRepository) CountInList(listID string) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM recipients WHERE list_id = ? AND status = ?`,
		listID, models.RecipientStatusUnsubscribed,
	).Scan(&n)
	return n, err
}
