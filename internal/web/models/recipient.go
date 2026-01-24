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
