package models

import "time"

// DKIMKey represents a DKIM key stored in the database
type DKIMKey struct {
	ID         string    `json:"id"`
	Domain     string    `json:"domain"`
	Selector   string    `json:"selector"`
	PrivateKey string    `json:"private_key,omitempty"` // Hidden in list views
	DNSRecord  string    `json:"dns_record"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Computed fields
	Deployments []DKIMDeployment `json:"deployments,omitempty"`
}

// DKIMDeployment represents a DKIM key deployment to a server
type DKIMDeployment struct {
	ID         int64     `json:"id"`
	DKIMKeyID  string    `json:"dkim_key_id"`
	ServerName string    `json:"server_name"`
	DeployedAt time.Time `json:"deployed_at"`
	Status     string    `json:"status"` // deployed, failed
	Error      string    `json:"error,omitempty"`
}

// DKIMKeyListItem represents a DKIM key in list view (without private key)
type DKIMKeyListItem struct {
	ID              string           `json:"id"`
	Domain          string           `json:"domain"`
	Selector        string           `json:"selector"`
	DNSName         string           `json:"dns_name"`
	DNSRecord       string           `json:"dns_record"`
	CreatedAt       time.Time        `json:"created_at"`
	DeploymentCount int              `json:"deployment_count"`
	Deployments     []DKIMDeployment `json:"deployments,omitempty"`
	IsDeployed      bool             `json:"is_deployed"` // For server-specific views
}
