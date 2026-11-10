package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

// ErrRecipientUnsubscribed is returned when an operation would touch an
// unsubscribed address in a way that could lose the suppression.
var ErrRecipientUnsubscribed = errors.New("recipient is unsubscribed")

// ListCounts are live per-status recipient counts of a list.
type ListCounts struct {
	Total        int
	Active       int
	Unsubscribed int
}

// ListCountsByList returns live counts for every list that has recipients,
// keyed by list ID, in one query.
func (r *RecipientRepository) ListCountsByList() (map[string]ListCounts, error) {
	rows, err := r.db.Query(`
		SELECT list_id, COUNT(*),
			COALESCE(SUM(status = 'active'), 0),
			COALESCE(SUM(status = 'unsubscribed'), 0)
		FROM recipients GROUP BY list_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ListCounts{}
	for rows.Next() {
		var id string
		var c ListCounts
		if err := rows.Scan(&id, &c.Total, &c.Active, &c.Unsubscribed); err != nil {
			return nil, err
		}
		out[id] = c
	}
	return out, rows.Err()
}

// RecipientUpsert is one address to put into a list by UpsertRecipients.
// Email must already be normalised (NormalizeEmail) and validated.
type RecipientUpsert struct {
	Email     string
	Name      string
	Variables map[string]any
}

// UpsertResult counts the outcome of UpsertRecipients.
type UpsertResult struct {
	Added        int
	Updated      int
	Existing     int
	Unsubscribed int
}

// UpsertRecipients adds addresses to a list in one transaction.
//   - new address: inserted as active;
//   - existing active address: with updateExisting its name (when non-empty)
//     is replaced and its variables are merged key by key (given keys win);
//     without it the address is only counted as existing;
//   - existing unsubscribed (or any non-active) address is never touched and
//     never reactivated: it is counted as Unsubscribed.
//
// Callers dedupe the input; the list counters are refreshed after commit.
func (r *RecipientRepository) UpsertRecipients(listID string, items []RecipientUpsert, updateExisting bool) (*UpsertResult, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res := &UpsertResult{}
	now := time.Now()
	for _, it := range items {
		var id, status string
		var name, vars sql.NullString
		err := tx.QueryRow(
			`SELECT id, status, name, variables FROM recipients WHERE list_id = ? AND lower(email) = ? LIMIT 1`,
			listID, it.Email,
		).Scan(&id, &status, &name, &vars)

		switch {
		case err == sql.ErrNoRows:
			varsJSON := ""
			if len(it.Variables) > 0 {
				b, merr := json.Marshal(it.Variables)
				if merr != nil {
					return nil, merr
				}
				varsJSON = string(b)
			}
			if _, err := tx.Exec(`
				INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at)
				VALUES (?, ?, ?, ?, ?, '', 'active', ?)`,
				uuid.New().String(), listID, it.Email, it.Name, varsJSON, now,
			); err != nil {
				return nil, fmt.Errorf("failed to add recipient %s: %w", it.Email, err)
			}
			res.Added++
		case err != nil:
			return nil, err
		case status != "active":
			res.Unsubscribed++
		case !updateExisting:
			res.Existing++
		default:
			newName := name.String
			if it.Name != "" {
				newName = it.Name
			}
			merged := map[string]any{}
			if vars.String != "" {
				// an unparsable stored value is left to be overwritten
				_ = json.Unmarshal([]byte(vars.String), &merged)
				if merged == nil {
					merged = map[string]any{}
				}
			}
			for k, v := range it.Variables {
				merged[k] = v
			}
			varsJSON := vars.String
			if len(merged) > 0 {
				b, merr := json.Marshal(merged)
				if merr != nil {
					return nil, merr
				}
				varsJSON = string(b)
			}
			if _, err := tx.Exec(`UPDATE recipients SET name = ?, variables = ? WHERE id = ?`, newName, varsJSON, id); err != nil {
				return nil, fmt.Errorf("failed to update recipient %s: %w", it.Email, err)
			}
			res.Updated++
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, r.UpdateListCounts(listID)
}

// DeleteRecipientByEmail removes an address from a list.
// ErrRecipientNotInList if it is not there; ErrRecipientUnsubscribed if it is
// unsubscribed (deleting it would let it be added back as active, losing the
// suppression); ErrInUse if campaigns were sent to it (same as the UI).
func (r *RecipientRepository) DeleteRecipientByEmail(listID, email string) error {
	var id, status string
	err := r.db.QueryRow(
		`SELECT id, status FROM recipients WHERE list_id = ? AND lower(email) = ? LIMIT 1`,
		listID, NormalizeEmail(email),
	).Scan(&id, &status)
	if err == sql.ErrNoRows {
		return ErrRecipientNotInList
	}
	if err != nil {
		return err
	}
	if status == models.RecipientStatusUnsubscribed {
		return ErrRecipientUnsubscribed
	}
	return r.DeleteRecipient(id, listID)
}
