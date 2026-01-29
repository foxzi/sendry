package models

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// Domain represents a domain configuration stored locally
type Domain struct {
	ID                  string    `json:"id"`
	Domain              string    `json:"domain"`
	Mode                string    `json:"mode"` // production, sandbox, redirect, bcc
	DefaultFrom         string    `json:"default_from,omitempty"`
	DKIMEnabled         bool      `json:"dkim_enabled"`
	DKIMSelector        string    `json:"dkim_selector,omitempty"`
	DKIMKeyID           string    `json:"dkim_key_id,omitempty"`
	RateLimitHour       int       `json:"rate_limit_hour"`
	RateLimitDay        int       `json:"rate_limit_day"`
	RateLimitRecipients int       `json:"rate_limit_recipients"`
	RedirectTo          []string  `json:"redirect_to,omitempty"`
	BCCTo               []string  `json:"bcc_to,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// Computed fields
	Deployments []DomainDeployment `json:"deployments,omitempty"`
	DKIMKey     *DKIMKey           `json:"dkim_key,omitempty"`
}

// DomainDeployment represents a domain deployment to a server
type DomainDeployment struct {
	ID         int64     `json:"id"`
	DomainID   string    `json:"domain_id"`
	ServerName string    `json:"server_name"`
	DeployedAt time.Time `json:"deployed_at"`
	Status     string    `json:"status"` // deployed, failed, outdated
	Error      string    `json:"error,omitempty"`
	ConfigHash string    `json:"config_hash"`
}

// DomainListItem represents a domain in list view
type DomainListItem struct {
	ID              string             `json:"id"`
	Domain          string             `json:"domain"`
	Mode            string             `json:"mode"`
	DKIMEnabled     bool               `json:"dkim_enabled"`
	DKIMSelector    string             `json:"dkim_selector,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	DeploymentCount int                `json:"deployment_count"`
	OutdatedCount   int                `json:"outdated_count"`
	Deployments     []DomainDeployment `json:"deployments,omitempty"`
}

// DomainFilter for listing domains
type DomainFilter struct {
	Search string
	Mode   string
	Limit  int
	Offset int
}

// ConfigHash calculates a hash of the domain configuration for change detection
func (d *Domain) ConfigHash() string {
	data := struct {
		Domain              string   `json:"domain"`
		Mode                string   `json:"mode"`
		DefaultFrom         string   `json:"default_from"`
		DKIMEnabled         bool     `json:"dkim_enabled"`
		DKIMSelector        string   `json:"dkim_selector"`
		DKIMKeyID           string   `json:"dkim_key_id"`
		RateLimitHour       int      `json:"rate_limit_hour"`
		RateLimitDay        int      `json:"rate_limit_day"`
		RateLimitRecipients int      `json:"rate_limit_recipients"`
		RedirectTo          []string `json:"redirect_to"`
		BCCTo               []string `json:"bcc_to"`
	}{
		Domain:              d.Domain,
		Mode:                d.Mode,
		DefaultFrom:         d.DefaultFrom,
		DKIMEnabled:         d.DKIMEnabled,
		DKIMSelector:        d.DKIMSelector,
		DKIMKeyID:           d.DKIMKeyID,
		RateLimitHour:       d.RateLimitHour,
		RateLimitDay:        d.RateLimitDay,
		RateLimitRecipients: d.RateLimitRecipients,
		RedirectTo:          d.RedirectTo,
		BCCTo:               d.BCCTo,
	}

	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash[:8])
}

// IsOutdated checks if a deployment is outdated compared to current config
func (d *Domain) IsOutdated(deployment DomainDeployment) bool {
	return deployment.ConfigHash != d.ConfigHash()
}

// GetOutdatedDeployments returns list of deployments that need to be synced
func (d *Domain) GetOutdatedDeployments() []DomainDeployment {
	var outdated []DomainDeployment
	currentHash := d.ConfigHash()
	for _, dep := range d.Deployments {
		if dep.ConfigHash != currentHash && dep.Status != "failed" {
			outdated = append(outdated, dep)
		}
	}
	return outdated
}
