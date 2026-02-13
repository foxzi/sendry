package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

type SendRepository struct {
	db *sql.DB
}

func NewSendRepository(db *sql.DB) *SendRepository {
	return &SendRepository{db: db}
}

// Create creates a new send record
func (r *SendRepository) Create(s *models.Send) error {
	s.ID = uuid.New().String()
	s.CreatedAt = time.Now()

	_, err := r.db.Exec(`
		INSERT INTO sends (id, api_key_id, from_address, to_addresses, cc_addresses, bcc_addresses,
			subject, template_id, sender_domain, server_name, server_msg_id, status, error_message,
			created_at, sent_at, client_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, nullString(s.APIKeyID), s.FromAddress, s.ToAddresses, nullString(s.CCAddresses), nullString(s.BCCAddresses),
		s.Subject, nullString(s.TemplateID), s.SenderDomain, s.ServerName, nullString(s.ServerMsgID),
		s.Status, nullString(s.ErrorMessage), s.CreatedAt, s.SentAt, nullString(s.ClientIP),
	)
	if err != nil {
		return fmt.Errorf("failed to create send: %w", err)
	}
	return nil
}

// GetByID returns a send by ID
func (r *SendRepository) GetByID(id string) (*models.Send, error) {
	s := &models.Send{}
	var apiKeyID, ccAddr, bccAddr, templateID, serverMsgID, errorMsg, clientIP sql.NullString
	var sentAt sql.NullTime

	err := r.db.QueryRow(`
		SELECT id, api_key_id, from_address, to_addresses, cc_addresses, bcc_addresses,
			subject, template_id, sender_domain, server_name, server_msg_id, status, error_message,
			created_at, sent_at, client_ip
		FROM sends WHERE id = ?`, id,
	).Scan(&s.ID, &apiKeyID, &s.FromAddress, &s.ToAddresses, &ccAddr, &bccAddr,
		&s.Subject, &templateID, &s.SenderDomain, &s.ServerName, &serverMsgID,
		&s.Status, &errorMsg, &s.CreatedAt, &sentAt, &clientIP)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.APIKeyID = apiKeyID.String
	s.CCAddresses = ccAddr.String
	s.BCCAddresses = bccAddr.String
	s.TemplateID = templateID.String
	s.ServerMsgID = serverMsgID.String
	s.ErrorMessage = errorMsg.String
	s.ClientIP = clientIP.String
	if sentAt.Valid {
		s.SentAt = &sentAt.Time
	}

	return s, nil
}

// List returns sends with optional filtering
func (r *SendRepository) List(filter models.SendFilter) ([]models.SendWithDetails, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM sends WHERE 1=1"
	args := []any{}

	if filter.APIKeyID != "" {
		countQuery += " AND api_key_id = ?"
		args = append(args, filter.APIKeyID)
	}
	if filter.Status != "" {
		countQuery += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.SenderDomain != "" {
		countQuery += " AND sender_domain = ?"
		args = append(args, filter.SenderDomain)
	}
	if filter.ServerName != "" {
		countQuery += " AND server_name = ?"
		args = append(args, filter.ServerName)
	}
	if filter.TemplateID != "" {
		countQuery += " AND template_id = ?"
		args = append(args, filter.TemplateID)
	}
	if filter.FromDate != nil {
		countQuery += " AND created_at >= ?"
		args = append(args, *filter.FromDate)
	}
	if filter.ToDate != nil {
		countQuery += " AND created_at <= ?"
		args = append(args, *filter.ToDate)
	}
	if filter.Search != "" {
		countQuery += " AND (from_address LIKE ? OR to_addresses LIKE ? OR subject LIKE ?)"
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get sends with details
	query := `
		SELECT s.id, s.api_key_id, s.from_address, s.to_addresses, s.cc_addresses, s.bcc_addresses,
			s.subject, s.template_id, s.sender_domain, s.server_name, s.server_msg_id, s.status,
			s.error_message, s.created_at, s.sent_at, s.client_ip,
			COALESCE(k.name, '') as api_key_name,
			COALESCE(t.name, '') as template_name
		FROM sends s
		LEFT JOIN api_keys k ON s.api_key_id = k.id
		LEFT JOIN templates t ON s.template_id = t.id
		WHERE 1=1`

	queryArgs := []any{}
	if filter.APIKeyID != "" {
		query += " AND s.api_key_id = ?"
		queryArgs = append(queryArgs, filter.APIKeyID)
	}
	if filter.Status != "" {
		query += " AND s.status = ?"
		queryArgs = append(queryArgs, filter.Status)
	}
	if filter.SenderDomain != "" {
		query += " AND s.sender_domain = ?"
		queryArgs = append(queryArgs, filter.SenderDomain)
	}
	if filter.ServerName != "" {
		query += " AND s.server_name = ?"
		queryArgs = append(queryArgs, filter.ServerName)
	}
	if filter.TemplateID != "" {
		query += " AND s.template_id = ?"
		queryArgs = append(queryArgs, filter.TemplateID)
	}
	if filter.FromDate != nil {
		query += " AND s.created_at >= ?"
		queryArgs = append(queryArgs, *filter.FromDate)
	}
	if filter.ToDate != nil {
		query += " AND s.created_at <= ?"
		queryArgs = append(queryArgs, *filter.ToDate)
	}
	if filter.Search != "" {
		query += " AND (s.from_address LIKE ? OR s.to_addresses LIKE ? OR s.subject LIKE ?)"
		queryArgs = append(queryArgs, "%"+filter.Search+"%", "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	query += " ORDER BY s.created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sends []models.SendWithDetails
	for rows.Next() {
		var s models.SendWithDetails
		var apiKeyID, ccAddr, bccAddr, templateID, serverMsgID, errorMsg, clientIP sql.NullString
		var sentAt sql.NullTime

		if err := rows.Scan(&s.ID, &apiKeyID, &s.FromAddress, &s.ToAddresses, &ccAddr, &bccAddr,
			&s.Subject, &templateID, &s.SenderDomain, &s.ServerName, &serverMsgID,
			&s.Status, &errorMsg, &s.CreatedAt, &sentAt, &clientIP,
			&s.APIKeyName, &s.TemplateName); err != nil {
			return nil, 0, err
		}

		s.APIKeyID = apiKeyID.String
		s.CCAddresses = ccAddr.String
		s.BCCAddresses = bccAddr.String
		s.TemplateID = templateID.String
		s.ServerMsgID = serverMsgID.String
		s.ErrorMessage = errorMsg.String
		s.ClientIP = clientIP.String
		if sentAt.Valid {
			s.SentAt = &sentAt.Time
		}

		sends = append(sends, s)
	}

	return sends, total, rows.Err()
}

// UpdateStatus updates the status of a send
func (r *SendRepository) UpdateStatus(id, status, errorMsg, serverMsgID string, sentAt *time.Time) error {
	_, err := r.db.Exec(`
		UPDATE sends SET status = ?, error_message = ?, server_msg_id = ?, sent_at = ?
		WHERE id = ?`,
		status, nullString(errorMsg), nullString(serverMsgID), sentAt, id,
	)
	return err
}

// GetStats returns aggregated statistics
func (r *SendRepository) GetStats(filter models.SendFilter) (*models.SendStats, error) {
	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
		FROM sends WHERE 1=1`

	args := []any{}
	if filter.SenderDomain != "" {
		query += " AND sender_domain = ?"
		args = append(args, filter.SenderDomain)
	}
	if filter.ServerName != "" {
		query += " AND server_name = ?"
		args = append(args, filter.ServerName)
	}
	if filter.FromDate != nil {
		query += " AND created_at >= ?"
		args = append(args, *filter.FromDate)
	}
	if filter.ToDate != nil {
		query += " AND created_at <= ?"
		args = append(args, *filter.ToDate)
	}

	stats := &models.SendStats{}
	err := r.db.QueryRow(query, args...).Scan(&stats.Total, &stats.Pending, &stats.Sent, &stats.Failed)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// GetDomains returns list of unique sender domains
func (r *SendRepository) GetDomains() ([]string, error) {
	rows, err := r.db.Query("SELECT DISTINCT sender_domain FROM sends ORDER BY sender_domain")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// GetServers returns list of unique server names
func (r *SendRepository) GetServers() ([]string, error) {
	rows, err := r.db.Query("SELECT DISTINCT server_name FROM sends ORDER BY server_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

// helper to convert []string to JSON string
func ToJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// helper to parse JSON array to []string
func FromJSON(s string) []string {
	var arr []string
	json.Unmarshal([]byte(s), &arr)
	return arr
}
