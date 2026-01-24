package domain

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/dkim"
)

// Manager manages domain-specific configurations including DKIM signers
type Manager struct {
	config  *config.Config
	signers map[string]*dkim.Signer // domain -> signer
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewManager creates a new domain manager
func NewManager(cfg *config.Config, logger *slog.Logger) (*Manager, error) {
	m := &Manager{
		config:  cfg,
		signers: make(map[string]*dkim.Signer),
		logger:  logger,
	}

	if err := m.loadSigners(); err != nil {
		return nil, err
	}

	return m, nil
}

// loadSigners loads DKIM signers for all configured domains
func (m *Manager) loadSigners() error {
	// Load legacy DKIM config
	if m.config.DKIM.Enabled {
		signer, err := dkim.NewSignerFromFile(
			m.config.DKIM.KeyFile,
			m.config.DKIM.Domain,
			m.config.DKIM.Selector,
		)
		if err != nil {
			return fmt.Errorf("failed to load legacy DKIM signer for %s: %w", m.config.DKIM.Domain, err)
		}
		m.signers[m.config.DKIM.Domain] = signer
		m.logger.Info("loaded DKIM signer",
			"domain", m.config.DKIM.Domain,
			"selector", m.config.DKIM.Selector,
		)
	}

	// Load multi-domain DKIM configs
	for domain, dc := range m.config.Domains {
		if dc.DKIM != nil && dc.DKIM.Enabled {
			// Skip if already loaded from legacy config
			if _, exists := m.signers[domain]; exists {
				continue
			}

			signer, err := dkim.NewSignerFromFile(
				dc.DKIM.KeyFile,
				domain,
				dc.DKIM.Selector,
			)
			if err != nil {
				return fmt.Errorf("failed to load DKIM signer for %s: %w", domain, err)
			}
			m.signers[domain] = signer
			m.logger.Info("loaded DKIM signer",
				"domain", domain,
				"selector", dc.DKIM.Selector,
			)
		}
	}

	return nil
}

// GetSigner returns the DKIM signer for a domain
// Returns nil if no signer is configured for this domain
func (m *Manager) GetSigner(domain string) *dkim.Signer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try exact match first
	if signer, ok := m.signers[domain]; ok {
		return signer
	}

	// Try to find a parent domain match (e.g., mail.example.com -> example.com)
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts); i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if signer, ok := m.signers[parentDomain]; ok {
			return signer
		}
	}

	return nil
}

// GetSignerForEmail returns the DKIM signer for an email address
func (m *Manager) GetSignerForEmail(email string) *dkim.Signer {
	domain := extractDomain(email)
	if domain == "" {
		return nil
	}
	return m.GetSigner(domain)
}

// GetDomainConfig returns the configuration for a specific domain
func (m *Manager) GetDomainConfig(domain string) *config.DomainConfig {
	return m.config.GetDomainConfig(domain)
}

// GetDomainMode returns the mode for a domain (production, sandbox, redirect, bcc)
// Defaults to "production" if not specified
func (m *Manager) GetDomainMode(domain string) string {
	dc := m.config.GetDomainConfig(domain)
	if dc != nil && dc.Mode != "" {
		return dc.Mode
	}
	return "production"
}

// GetRedirectAddresses returns redirect addresses for a domain in redirect mode
func (m *Manager) GetRedirectAddresses(domain string) []string {
	dc := m.config.GetDomainConfig(domain)
	if dc != nil {
		return dc.RedirectTo
	}
	return nil
}

// GetBCCAddresses returns BCC addresses for a domain in bcc mode
func (m *Manager) GetBCCAddresses(domain string) []string {
	dc := m.config.GetDomainConfig(domain)
	if dc != nil {
		return dc.BCCTo
	}
	return nil
}

// ListDomains returns all configured domains
func (m *Manager) ListDomains() []string {
	return m.config.GetAllDomains()
}

// HasDKIM returns true if DKIM is configured for any domain
func (m *Manager) HasDKIM() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.signers) > 0
}

// extractDomain extracts the domain part from an email address
func extractDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}
