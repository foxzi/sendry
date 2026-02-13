package models

import "time"

// Send represents a single email send through the API
type Send struct {
	ID           string     `json:"id"`
	APIKeyID     string     `json:"api_key_id,omitempty"`
	FromAddress  string     `json:"from_address"`
	ToAddresses  string     `json:"to_addresses"`   // JSON array
	CCAddresses  string     `json:"cc_addresses"`   // JSON array
	BCCAddresses string     `json:"bcc_addresses"`  // JSON array
	Subject      string     `json:"subject"`
	TemplateID   string     `json:"template_id,omitempty"`
	SenderDomain string     `json:"sender_domain"`
	ServerName   string     `json:"server_name"`
	ServerMsgID  string     `json:"server_msg_id,omitempty"`
	Status       string     `json:"status"` // pending, sent, failed
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	ClientIP     string     `json:"client_ip,omitempty"`
}

// SendStatus constants
const (
	SendStatusPending = "pending"
	SendStatusSent    = "sent"
	SendStatusFailed  = "failed"
)

// SendFilter for listing sends
type SendFilter struct {
	APIKeyID     string
	Status       string
	SenderDomain string
	ServerName   string
	TemplateID   string
	FromDate     *time.Time
	ToDate       *time.Time
	Search       string // Search in from/to/subject
	Limit        int
	Offset       int
}

// SendStats aggregated statistics
type SendStats struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
}

// SendWithDetails includes additional info for display
type SendWithDetails struct {
	Send
	APIKeyName   string `json:"api_key_name,omitempty"`
	TemplateName string `json:"template_name,omitempty"`
}
