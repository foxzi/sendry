package models

import "time"

// Campaign represents an email campaign
type Campaign struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	FromEmail   string    `json:"from_email"`
	FromName    string    `json:"from_name"`
	ReplyTo     string    `json:"reply_to"`
	Variables   string    `json:"variables"` // JSON
	Tags        string    `json:"tags"`      // JSON array
	TrackOpens  bool      `json:"track_opens"`
	TrackClicks bool      `json:"track_clicks"`
	AddUTM      bool      `json:"add_utm"` // append utm_* and sc/sr/sl to tracked links
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CampaignVariant represents a template variant for A/B testing
type CampaignVariant struct {
	ID              string    `json:"id"`
	CampaignID      string    `json:"campaign_id"`
	Name            string    `json:"name"`
	TemplateID      string    `json:"template_id"`
	TemplateName    string    `json:"template_name,omitempty"` // joined field
	SubjectOverride string    `json:"subject_override"`
	Weight          int       `json:"weight"` // percentage weight for A/B testing
	CreatedAt       time.Time `json:"created_at"`
}

// CampaignWithStats includes campaign statistics
type CampaignWithStats struct {
	Campaign
	VariantCount int `json:"variant_count"`
	JobCount     int `json:"job_count"`
}

// CampaignListFilter for filtering campaigns
type CampaignListFilter struct {
	Search string
	Limit  int
	Offset int
}
