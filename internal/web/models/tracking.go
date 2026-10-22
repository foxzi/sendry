package models

import "time"

// Engagement event types stored in email_events.type
const (
	EventOpen        = "open"
	EventClick       = "click"
	EventUnsubscribe = "unsubscribe"
)

// Tracking source types: what a tracked message row is
const (
	SourceJobItem = "job_item" // send_job_items.id (campaign mail)
	SourceSend    = "send"     // sends.id (API mail)
)

// Unsubscribe source types
const (
	UnsubSourceLink     = "link"      // confirmed on the unsubscribe page
	UnsubSourceOneClick = "one_click" // RFC 8058 POST from the mail client
	UnsubSourceManual   = "manual"    // added by an operator in the panel
	UnsubSourceImport   = "import"    // bulk import
)

// RecipientStatusUnsubscribed is the recipients.status value of an address
// that unsubscribed from mail sent to its address book (recipient list)
const RecipientStatusUnsubscribed = "unsubscribed"

// EmailEvent is one row of the engagement log
type EmailEvent struct {
	ID         int64     `json:"id"`
	Type       string    `json:"type"`
	SourceType string    `json:"source_type"`
	SourceID   string    `json:"source_id"`
	CampaignID string    `json:"campaign_id,omitempty"`
	JobID      string    `json:"job_id,omitempty"`
	VariantID  string    `json:"variant_id,omitempty"`
	Email      string    `json:"email"`
	LinkID     *int      `json:"link_id,omitempty"`
	URL        string    `json:"url,omitempty"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	IsMachine  bool      `json:"is_machine"`
	CreatedAt  time.Time `json:"created_at"`
}

// TrackingSource is the context a tracking token resolves to
type TrackingSource struct {
	SourceType   string
	SourceID     string
	Email        string
	CampaignID   string
	CampaignName string
	FromName     string
	FromEmail    string
	JobID        string
	VariantID    string
	ListID       string     // address book the job was sent to (job items only)
	RecipientID  string     // recipients.id (job items only)
	SentAt       *time.Time // when the message was handed to the MTA, if known
}

// Unsubscribe is an unsubscribed address of one address book (recipient list).
// Entries listed in the panel come from recipients with status unsubscribed,
// joined with the unsubscribes journal; an address set to unsubscribed by hand
// in the recipient editor has no journal data (zero CreatedAt, empty source).
type Unsubscribe struct {
	ID          int64     `json:"id"`
	ListID      string    `json:"list_id"`
	RecipientID string    `json:"recipient_id,omitempty"`
	Email       string    `json:"email"`
	CampaignID  string    `json:"campaign_id,omitempty"`
	JobID       string    `json:"job_id,omitempty"`
	SourceType  string    `json:"source_type"`
	SourceID    string    `json:"source_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// TrackingSettings are the panel-wide tracking settings stored in the settings table
type TrackingSettings struct {
	// BaseURL is the proxying domain used in tracking links, stored as an
	// origin, e.g. https://lnk.example.com. Empty means server.public_url.
	BaseURL       string `json:"base_url"`
	OpensDefault  bool   `json:"opens_default"`
	ClicksDefault bool   `json:"clicks_default"`
	Secret        string `json:"-"` // HMAC key for tokens, generated on first use
}

// LinkInfo describes one numbered link of a template snapshot
type LinkInfo struct {
	ID    int    `json:"id"`    // data-sl number, 1-based, in source order
	URL   string `json:"url"`   // href as written in the template (may contain variables)
	Label string `json:"label"` // link text, image alt, or a generic name
	Kind  string `json:"kind"`  // text | image | button
}

// JobVariantSnapshot is the HTML of a variant frozen at job start
type JobVariantSnapshot struct {
	JobID           string     `json:"job_id"`
	VariantID       string     `json:"variant_id"`
	VariantName     string     `json:"variant_name"`
	TemplateID      string     `json:"template_id"`
	TemplateVersion int        `json:"template_version"`
	HTML            string     `json:"html"` // contains data-sl attributes
	Links           []LinkInfo `json:"links"`
	CreatedAt       time.Time  `json:"created_at"`
}

// TrackingStats aggregates engagement for a job or a campaign
type TrackingStats struct {
	Total        int `json:"total"`         // messages in scope
	Delivered    int `json:"delivered"`     // status sent (accepted by recipient MX)
	UniqueOpens  int `json:"unique_opens"`  // messages opened at least once
	TotalOpens   int `json:"total_opens"`   // all non-machine open events
	UniqueClicks int `json:"unique_clicks"` // messages clicked at least once
	TotalClicks  int `json:"total_clicks"`  // all non-machine click events
	Unsubscribed int `json:"unsubscribed"`  // messages that led to an unsubscribe
	SkippedUnsub int `json:"skipped_unsub"` // recipients skipped at job start (stop list)
}

func pct(a, b int) float64 {
	if b <= 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}

// OpenRate = unique opens / delivered, percent
func (s TrackingStats) OpenRate() float64 { return pct(s.UniqueOpens, s.Delivered) }

// CTR = unique clicks / delivered, percent
func (s TrackingStats) CTR() float64 { return pct(s.UniqueClicks, s.Delivered) }

// CTOR = unique clicks / unique opens, percent
func (s TrackingStats) CTOR() float64 { return pct(s.UniqueClicks, s.UniqueOpens) }

// UnsubRate = unsubscribed / delivered, percent
func (s TrackingStats) UnsubRate() float64 { return pct(s.Unsubscribed, s.Delivered) }

// LinkStat is the click count of one numbered link
type LinkStat struct {
	LinkID       int    `json:"link_id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	URL          string `json:"url"`
	UniqueClicks int    `json:"unique_clicks"` // distinct messages (recipients)
	TotalClicks  int    `json:"total_clicks"`
}
