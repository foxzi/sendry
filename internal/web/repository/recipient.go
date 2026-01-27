package repository

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type RecipientRepository struct {
	db *sql.DB
}

func NewRecipientRepository(db *sql.DB) *RecipientRepository {
	return &RecipientRepository{db: db}
}

// CreateList creates a new recipient list
func (r *RecipientRepository) CreateList(list *models.RecipientList) error {
	list.ID = uuid.New().String()
	list.CreatedAt = time.Now()
	list.UpdatedAt = list.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO recipient_lists (id, name, description, source_type, total_count, active_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		list.ID, list.Name, list.Description, list.SourceType, list.TotalCount, list.ActiveCount, list.CreatedAt, list.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create recipient list: %w", err)
	}
	return nil
}

// GetListByID returns a recipient list by ID
func (r *RecipientRepository) GetListByID(id string) (*models.RecipientList, error) {
	list := &models.RecipientList{}
	err := r.db.QueryRow(`
		SELECT id, name, description, source_type, total_count, active_count, created_at, updated_at
		FROM recipient_lists WHERE id = ?`, id,
	).Scan(&list.ID, &list.Name, &list.Description, &list.SourceType, &list.TotalCount, &list.ActiveCount, &list.CreatedAt, &list.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ListLists returns all recipient lists with optional filtering
func (r *RecipientRepository) ListLists(filter models.RecipientListFilter) ([]models.RecipientList, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM recipient_lists WHERE 1=1"
	args := []any{}

	if filter.Search != "" {
		countQuery += " AND (name LIKE ? OR description LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get lists
	query := `
		SELECT id, name, description, source_type, total_count, active_count, created_at, updated_at
		FROM recipient_lists WHERE 1=1`

	args = []any{}
	if filter.Search != "" {
		query += " AND (name LIKE ? OR description LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	query += " ORDER BY updated_at DESC"

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

	lists := []models.RecipientList{}
	for rows.Next() {
		var list models.RecipientList
		err := rows.Scan(&list.ID, &list.Name, &list.Description, &list.SourceType, &list.TotalCount, &list.ActiveCount, &list.CreatedAt, &list.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}
		lists = append(lists, list)
	}

	return lists, total, nil
}

// UpdateList updates a recipient list
func (r *RecipientRepository) UpdateList(list *models.RecipientList) error {
	list.UpdatedAt = time.Now()
	_, err := r.db.Exec(`
		UPDATE recipient_lists SET name = ?, description = ?, updated_at = ?
		WHERE id = ?`,
		list.Name, list.Description, list.UpdatedAt, list.ID,
	)
	return err
}

// DeleteList deletes a recipient list and all its recipients
func (r *RecipientRepository) DeleteList(id string) error {
	_, err := r.db.Exec("DELETE FROM recipient_lists WHERE id = ?", id)
	return err
}

// UpdateListCounts updates the total and active counts for a list
func (r *RecipientRepository) UpdateListCounts(listID string) error {
	_, err := r.db.Exec(`
		UPDATE recipient_lists SET
			total_count = (SELECT COUNT(*) FROM recipients WHERE list_id = ?),
			active_count = (SELECT COUNT(*) FROM recipients WHERE list_id = ? AND status = 'active'),
			updated_at = ?
		WHERE id = ?`,
		listID, listID, time.Now(), listID,
	)
	return err
}

// AddRecipient adds a single recipient to a list
func (r *RecipientRepository) AddRecipient(recipient *models.Recipient) error {
	recipient.ID = uuid.New().String()
	recipient.CreatedAt = time.Now()
	if recipient.Status == "" {
		recipient.Status = "active"
	}

	_, err := r.db.Exec(`
		INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(list_id, email) DO UPDATE SET
			name = excluded.name,
			variables = excluded.variables,
			tags = excluded.tags`,
		recipient.ID, recipient.ListID, recipient.Email, recipient.Name, recipient.Variables, recipient.Tags, recipient.Status, recipient.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to add recipient: %w", err)
	}

	return r.UpdateListCounts(recipient.ListID)
}

// GetRecipient returns a recipient by ID
func (r *RecipientRepository) GetRecipient(id string) (*models.Recipient, error) {
	recipient := &models.Recipient{}
	err := r.db.QueryRow(`
		SELECT id, list_id, email, name, variables, tags, status, created_at
		FROM recipients WHERE id = ?`, id,
	).Scan(&recipient.ID, &recipient.ListID, &recipient.Email, &recipient.Name, &recipient.Variables, &recipient.Tags, &recipient.Status, &recipient.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return recipient, nil
}

// ListRecipients returns recipients with filtering
func (r *RecipientRepository) ListRecipients(filter models.RecipientFilter) ([]models.Recipient, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM recipients WHERE list_id = ?"
	args := []any{filter.ListID}

	if filter.Search != "" {
		countQuery += " AND (email LIKE ? OR name LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.Status != "" {
		countQuery += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Tag != "" {
		countQuery += " AND tags LIKE ?"
		args = append(args, "%\""+filter.Tag+"\"%")
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get recipients
	query := `
		SELECT id, list_id, email, name, variables, tags, status, created_at
		FROM recipients WHERE list_id = ?`

	args = []any{filter.ListID}
	if filter.Search != "" {
		query += " AND (email LIKE ? OR name LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Tag != "" {
		query += " AND tags LIKE ?"
		args = append(args, "%\""+filter.Tag+"\"%")
	}

	query += " ORDER BY created_at DESC"

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

	recipients := []models.Recipient{}
	for rows.Next() {
		var rec models.Recipient
		err := rows.Scan(&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.Variables, &rec.Tags, &rec.Status, &rec.CreatedAt)
		if err != nil {
			return nil, 0, err
		}
		recipients = append(recipients, rec)
	}

	return recipients, total, nil
}

// UpdateRecipient updates a recipient
func (r *RecipientRepository) UpdateRecipient(recipient *models.Recipient) error {
	_, err := r.db.Exec(`
		UPDATE recipients SET email = ?, name = ?, variables = ?, tags = ?, status = ?
		WHERE id = ?`,
		recipient.Email, recipient.Name, recipient.Variables, recipient.Tags, recipient.Status, recipient.ID,
	)
	if err != nil {
		return err
	}
	return r.UpdateListCounts(recipient.ListID)
}

// DeleteRecipient deletes a recipient
func (r *RecipientRepository) DeleteRecipient(id string, listID string) error {
	_, err := r.db.Exec("DELETE FROM recipients WHERE id = ?", id)
	if err != nil {
		return err
	}
	return r.UpdateListCounts(listID)
}

// ImportCSV imports recipients from CSV data
func (r *RecipientRepository) ImportCSV(listID string, reader io.Reader) (*models.RecipientImportResult, error) {
	result := &models.RecipientImportResult{}

	csvReader := csv.NewReader(reader)

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Find column indices
	emailIdx := -1
	nameIdx := -1
	for i, col := range header {
		col = strings.ToLower(strings.TrimSpace(col))
		switch col {
		case "email", "e-mail", "email_address":
			emailIdx = i
		case "name", "full_name", "fullname":
			nameIdx = i
		}
	}

	if emailIdx == -1 {
		return nil, fmt.Errorf("email column not found in CSV")
	}

	// Process rows
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", result.Total+1, err))
			result.Total++
			continue
		}

		result.Total++

		if emailIdx >= len(record) {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: missing email column", result.Total))
			result.Skipped++
			continue
		}

		email := strings.TrimSpace(record[emailIdx])
		if email == "" {
			result.Skipped++
			continue
		}

		name := ""
		if nameIdx >= 0 && nameIdx < len(record) {
			name = strings.TrimSpace(record[nameIdx])
		}

		recipient := &models.Recipient{
			ListID: listID,
			Email:  email,
			Name:   name,
			Status: "active",
		}

		if err := r.AddRecipient(recipient); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d (%s): %v", result.Total, email, err))
			result.Skipped++
			continue
		}

		result.Imported++
	}

	// Update counts after import
	r.UpdateListCounts(listID)

	return result, nil
}

// StreamRecipients returns an iterator for recipients to avoid loading all into memory.
// Caller must call Close() on returned rows when done.
func (r *RecipientRepository) StreamRecipients(listID string) (*sql.Rows, error) {
	return r.db.Query(`
		SELECT id, list_id, email, name, variables, tags, status, created_at
		FROM recipients WHERE list_id = ?
		ORDER BY created_at DESC`, listID)
}

// ScanRecipient scans a single recipient from rows
func (r *RecipientRepository) ScanRecipient(rows *sql.Rows) (*models.Recipient, error) {
	rec := &models.Recipient{}
	if err := rows.Scan(&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.Variables, &rec.Tags, &rec.Status, &rec.CreatedAt); err != nil {
		return nil, err
	}
	return rec, nil
}

// GetTags returns all unique tags from a list
func (r *RecipientRepository) GetTags(listID string) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT json_each.value
		FROM recipients, json_each(recipients.tags)
		WHERE recipients.list_id = ? AND recipients.tags IS NOT NULL AND recipients.tags != ''
		ORDER BY json_each.value`, listID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}
