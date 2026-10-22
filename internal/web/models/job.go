package models

import (
	"time"

	"github.com/foxzi/sendry/internal/web/stats"
)

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
	Strategy        string     `json:"strategy"` // server distribution: round-robin, random, weighted
	Stats           string     `json:"stats"`    // JSON with stats
	DryRun          bool       `json:"dry_run"`
	DryRunLimit     int        `json:"dry_run_limit"`
	// SkippedUnsubscribed is how many recipients of the job were not sent to
	// because they unsubscribed in the list (at job start or while it ran)
	SkippedUnsubscribed int       `json:"skipped_unsubscribed"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// SendJobItem represents a single email in a send job
type SendJobItem struct {
	ID                 string     `json:"id"`
	JobID              string     `json:"job_id"`
	RecipientID        string     `json:"recipient_id"`
	Email              string     `json:"email,omitempty"`               // joined field
	RecipientName      string     `json:"recipient_name,omitempty"`      // joined field
	RecipientVariables string     `json:"recipient_variables,omitempty"` // joined field (JSON)
	VariantID          string     `json:"variant_id"`
	VariantName        string     `json:"variant_name,omitempty"` // joined field
	ServerName         string     `json:"server_name"`
	Status             string     `json:"status"` // pending, queued, sent, failed, skipped
	SendryMsgID        string     `json:"sendry_msg_id"`
	Error              string     `json:"error"`
	QueuedAt           *time.Time `json:"queued_at,omitempty"`
	SentAt             *time.Time `json:"sent_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	OpenedAt           *time.Time `json:"opened_at,omitempty"` // first non-machine open
	OpenCount          int        `json:"open_count"`
	ClickedAt          *time.Time `json:"clicked_at,omitempty"` // first non-machine click
	ClickCount         int        `json:"click_count"`
	UnsubscribedAt     *time.Time `json:"unsubscribed_at,omitempty"`
}

// Job item statuses. Skipped is terminal: the recipient is no longer active
// in the list (unsubscribed, bounced, deleted), the reason is in Error.
const (
	JobItemStatusPending = "pending"
	JobItemStatusQueued  = "queued"
	JobItemStatusSent    = "sent"
	JobItemStatusFailed  = "failed"
	JobItemStatusSkipped = "skipped"
)

// Reasons stored in the error of skipped job items
const (
	SkipReasonUnsubscribed = "recipient unsubscribed"
	SkipReasonDeleted      = "recipient deleted"
)

// SkipReason returns the reason for skipping a recipient with this status
// (recipients.status); "" means the recipient is active and is sent to.
func SkipReason(recipientStatus string) string {
	switch recipientStatus {
	case "", "active":
		return ""
	case RecipientStatusUnsubscribed:
		return SkipReasonUnsubscribed
	default:
		return "recipient " + recipientStatus
	}
}

// JobStats holds aggregated job statistics
type JobStats struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Queued  int `json:"queued"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// Progress returns the share of items in a final state (sent, failed, skipped), 0-100
func (s JobStats) Progress() int {
	if s.Total == 0 {
		return 0
	}
	return (s.Sent + s.Failed + s.Skipped) * 100 / s.Total
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
	JobID     string
	Status    string
	ErrorType stats.ErrorType
	// Engagement filters: "", "yes", "no"
	Opened  string
	Clicked string
	Limit   int
	Offset  int
}

// ErrorCount is the number of job items or sends with the same error text and status
type ErrorCount struct {
	Error  string
	Status string
	Count  int
}

// JobVariantTemplate is the template source of a variant frozen when the job
// sent its first message, so later edits of the template do not change the
// mail (and the data-sl link numbers) of a running job.
type JobVariantTemplate struct {
	VariantID       string
	TemplateID      string
	TemplateVersion int
	Subject         string
	HTML            string // numbered (data-sl), not rendered
	Text            string
}
