package template

import (
	"time"
)

// Template represents an email template
type Template struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Subject     string         `json:"subject"`
	HTML        string         `json:"html,omitempty"`
	Text        string         `json:"text,omitempty"`
	Variables   []VariableInfo `json:"variables,omitempty"`
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// VariableInfo documents a template variable
type VariableInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Example     string `json:"example,omitempty"`
}

// RenderResult contains rendered template output
type RenderResult struct {
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

// ListFilter contains filters for listing templates
type ListFilter struct {
	Limit  int
	Offset int
	Search string
}

// Stats contains template statistics
type Stats struct {
	Total int64 `json:"total"`
}
