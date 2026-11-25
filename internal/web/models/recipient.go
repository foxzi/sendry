package models

import "time"

// RecipientList represents a list of email recipients
type RecipientList struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SourceType  string    `json:"source_type"` // manual, csv, api
	TotalCount  int       `json:"total_count"`
	ActiveCount int       `json:"active_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// UsedBy is filled by the pages with a delete button: the campaigns sent
	// to the list, which keep it from being deleted.
	UsedBy []CampaignRef `json:"-"`
}

// CampaignRef names a campaign for a link to it.
type CampaignRef struct {
	ID   string
	Name string
}

// Recipient represents a single email recipient
type Recipient struct {
	ID        string    `json:"id"`
	ListID    string    `json:"list_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Variables string    `json:"variables"` // JSON
	Tags      string    `json:"tags"`      // JSON array
	Status    string    `json:"status"`    // active, unsubscribed, bounced
	CreatedAt time.Time `json:"created_at"`
}

// RecipientListFilter for filtering recipient lists
type RecipientListFilter struct {
	Search string
	Limit  int
	Offset int
}

// RecipientFilter for filtering recipients within a list
type RecipientFilter struct {
	ListID string
	Search string
	Status string
	Tag    string
	Limit  int
	Offset int
}

// RecipientImportResult holds the result of an import operation
type RecipientImportResult struct {
	Total    int      `json:"total"`
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// RecipientMembership is one address book (recipient list) an email address
// is in, with its status there.
type RecipientMembership struct {
	RecipientID string
	ListID      string
	ListName    string
	Email       string // filled by the search over all lists
	Name        string // same
	Status      string
	CreatedAt   time.Time
}

// RecipientMessage is one campaign message addressed to an email address
// (a send job item), with the campaign and the address book it went through.
type RecipientMessage struct {
	ItemID         string
	JobID          string
	CampaignID     string
	CampaignName   string
	ListID         string
	ListName       string
	VariantName    string
	Status         string // job item status: pending, queued, sent, failed, skipped
	Error          string
	At             time.Time // sent, else queued, else created
	OpenedAt       *time.Time
	OpenCount      int
	ClickedAt      *time.Time
	ClickCount     int
	UnsubscribedAt *time.Time
	EventTotal     int // all engagement events of the message, shown or not
}

// RecipientTotals sums the campaign messages addressed to an email address.
type RecipientTotals struct {
	Messages int        // job items in any state, skipped ones included
	Sent     int        // handed over to the recipient's server
	Opened   int        // messages opened at least once
	Clicked  int        // messages clicked at least once
	LastSent *time.Time // latest sent_at
}

// RecipientEvent is an engagement event of an email address with the name of
// the campaign it came from.
type RecipientEvent struct {
	EmailEvent
	CampaignName string
}
