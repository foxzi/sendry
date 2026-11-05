package models

import (
	"encoding/json"
	"slices"
	"time"
)

// APIKey represents an API key for authentication
type APIKey struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	KeyHash         string     `json:"-"`                 // SHA256 hash, never expose
	KeyPrefix       string     `json:"key_prefix"`        // First 8 chars for display
	Permissions     string     `json:"permissions"`       // JSON array of permissions
	AllowedDomains  []string   `json:"allowed_domains"`   // Domains allowed to send from (empty = all)
	RateLimitMinute int        `json:"rate_limit_minute"` // Max requests per minute (0 = unlimited)
	RateLimitHour   int        `json:"rate_limit_hour"`   // Max requests per hour (0 = unlimited)
	CreatedBy       string     `json:"created_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Active          bool       `json:"active"`
}

// CanSendFromDomain checks if the API key is allowed to send from the given domain
func (k *APIKey) CanSendFromDomain(domain string) bool {
	if len(k.AllowedDomains) == 0 {
		return true // No restrictions
	}
	for _, d := range k.AllowedDomains {
		if d == domain {
			return true
		}
	}
	return false
}

// API key permissions. A key holds at least one.
const (
	PermissionSend  = "send"  // send email (plain and template) and check its status
	PermissionLists = "lists" // read recipient lists, add and remove recipients
)

// APIKeyPermissions are all permissions, in display order.
var APIKeyPermissions = []string{PermissionSend, PermissionLists}

// ValidAPIKeyPermission reports whether p is a known permission.
func ValidAPIKeyPermission(p string) bool {
	return slices.Contains(APIKeyPermissions, p)
}

// legacyPermissions maps values stored before permissions were enforced
// (when every key could call every endpoint) to the current ones.
var legacyPermissions = map[string]string{
	"template": PermissionSend,
	"status":   PermissionSend,
}

// PermissionList returns the key's known permissions in display order.
// Keys stored without a readable list predate permissions and get send.
func (k *APIKey) PermissionList() []string {
	var stored []string
	_ = json.Unmarshal([]byte(k.Permissions), &stored)
	have := map[string]bool{}
	for _, p := range stored {
		if mapped, ok := legacyPermissions[p]; ok {
			p = mapped
		}
		have[p] = true
	}
	perms := make([]string, 0, len(APIKeyPermissions))
	for _, p := range APIKeyPermissions {
		if have[p] {
			perms = append(perms, p)
		}
	}
	if len(perms) == 0 {
		return []string{PermissionSend}
	}
	return perms
}

// HasPermission reports whether the key holds permission p.
func (k *APIKey) HasPermission(p string) bool {
	return slices.Contains(k.PermissionList(), p)
}

// APIKeyFilter for listing API keys
type APIKeyFilter struct {
	Active bool
	Search string
	Limit  int
	Offset int
}

// APIKeyWithStats includes usage statistics
type APIKeyWithStats struct {
	APIKey
	SendCount int `json:"send_count"`
}

// APIKeyCreateResult returned when creating a new key
// Contains the full key which is shown only once
type APIKeyCreateResult struct {
	APIKey
	Key string `json:"key"` // Full key, shown only on creation
}
