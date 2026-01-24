package queue

import (
	"time"
)

// MessageStatus represents the status of a message in the queue
type MessageStatus string

const (
	StatusPending   MessageStatus = "pending"
	StatusSending   MessageStatus = "sending"
	StatusDelivered MessageStatus = "delivered"
	StatusFailed    MessageStatus = "failed"
	StatusDeferred  MessageStatus = "deferred"
)

// Message represents an email message in the queue
type Message struct {
	ID          string        `json:"id"`
	From        string        `json:"from"`
	To          []string      `json:"to"`
	Data        []byte        `json:"data"` // Raw email data (RFC 5322)
	Status      MessageStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	NextRetryAt time.Time     `json:"next_retry_at"`
	RetryCount  int           `json:"retry_count"`
	LastError   string        `json:"last_error,omitempty"`
	ClientIP    string        `json:"client_ip,omitempty"`
	AuthUser    string        `json:"auth_user,omitempty"`
}

// DeliveryAttempt represents a delivery attempt record
type DeliveryAttempt struct {
	Timestamp time.Time `json:"timestamp"`
	MXHost    string    `json:"mx_host"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Response  string    `json:"response,omitempty"`
}

// QueueStats represents queue statistics
type QueueStats struct {
	Pending   int64 `json:"pending"`
	Sending   int64 `json:"sending"`
	Delivered int64 `json:"delivered"`
	Failed    int64 `json:"failed"`
	Deferred  int64 `json:"deferred"`
	Total     int64 `json:"total"`
}

// ListFilter represents filter options for listing messages
type ListFilter struct {
	Status MessageStatus
	Limit  int
	Offset int
}
