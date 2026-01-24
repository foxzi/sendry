package models

import "time"

// SendJob represents a campaign send job
type SendJob struct {
	ID              string     `json:"id"`
	CampaignID      string     `json:"campaign_id"`
	CampaignName    string     `json:"campaign_name,omitempty"` // joined field
	RecipientListID string     `json:"recipient_list_id"`
	ListName        string     `json:"list_name,omitempty"` // joined field
	Status          string     `json:"status"`              // draft, scheduled, running, paused, completed, failed, cancelled
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Servers         string     `json:"servers"`  // JSON array of server names
	Strategy        string     `json:"strategy"` // round-robin, random, weighted
	Stats           string     `json:"stats"`    // JSON with stats
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// SendJobItem represents a single email in a send job
type SendJobItem struct {
	ID          string     `json:"id"`
	JobID       string     `json:"job_id"`
	RecipientID string     `json:"recipient_id"`
	Email       string     `json:"email,omitempty"` // joined field
	VariantID   string     `json:"variant_id"`
	VariantName string     `json:"variant_name,omitempty"` // joined field
	ServerName  string     `json:"server_name"`
	Status      string     `json:"status"` // pending, queued, sent, failed
	SendryMsgID string     `json:"sendry_msg_id"`
	Error       string     `json:"error"`
	QueuedAt    *time.Time `json:"queued_at,omitempty"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// JobStats holds aggregated job statistics
type JobStats struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Queued  int `json:"queued"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
}

// JobListFilter for filtering jobs
type JobListFilter struct {
	CampaignID string
	Status     string
	Limit      int
	Offset     int
}

// JobItemFilter for filtering job items
type JobItemFilter struct {
	JobID  string
	Status string
	Limit  int
	Offset int
}
