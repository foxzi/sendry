package models

import "time"

type Template struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	Subject             string    `json:"subject"`
	HTML                string    `json:"html"`
	Text                string    `json:"text"`
	Variables           string    `json:"variables"` // JSON
	Folder              string    `json:"folder"`
	CurrentVersion      int       `json:"current_version"`
	UseBlocks           bool      `json:"use_blocks"`
	ContainerRadius      int       `json:"container_radius"`
	ContainerRadiusTop   int       `json:"container_radius_top"`    // overrides ContainerRadius for top corners when > 0
	ContainerRadiusBottom int      `json:"container_radius_bottom"` // overrides ContainerRadius for bottom corners when > 0
	ContainerTransparent bool      `json:"container_transparent"`
	ContainerWidth       int       `json:"container_width"`
	ContainerPaddingV    int       `json:"container_padding_v"`
	ContainerPaddingH    int       `json:"container_padding_h"`
	PageBackground       string    `json:"page_background"` // CSS colour around the email container; empty -> "#F5F5F5"
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	BlockRefs []TemplateBlockRef `json:"block_refs,omitempty"`
}

type TemplateBlockRef struct {
	ID         int64     `json:"id"`
	TemplateID string    `json:"template_id"`
	BlockID    string    `json:"block_id"`
	Position   int       `json:"position"`
	GapHeight  int       `json:"gap_height"`
	GapColor   string    `json:"gap_color"`
	Condition string    `json:"condition"`
	CreatedAt time.Time `json:"created_at"`
}

type TemplateVersion struct {
	ID         int64     `json:"id"`
	TemplateID string    `json:"template_id"`
	Version    int       `json:"version"`
	Subject    string    `json:"subject"`
	HTML       string    `json:"html"`
	Text       string    `json:"text"`
	Variables  string    `json:"variables"`
	ChangeNote string    `json:"change_note"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type TemplateDeployment struct {
	ID              int64     `json:"id"`
	TemplateID      string    `json:"template_id"`
	ServerName      string    `json:"server_name"`
	RemoteID        string    `json:"remote_id"`
	DeployedVersion int       `json:"deployed_version"`
	DeployedAt      time.Time `json:"deployed_at"`
}

// TemplateWithStatus includes deployment status info
type TemplateWithStatus struct {
	Template
	DeployedCount   int    `json:"deployed_count"`
	OutOfSyncCount  int    `json:"out_of_sync_count"`
	Status          string `json:"status"` // draft, deployed, out-of-sync
}

// TemplateListFilter for filtering template list
type TemplateListFilter struct {
	Search string
	Folder string
	Limit  int
	Offset int
}
