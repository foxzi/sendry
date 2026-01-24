package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// ACMEManager manages automatic TLS certificates from Let's Encrypt
type ACMEManager struct {
	manager *autocert.Manager
	domains []string
}

// NewACMEManager creates a new ACME manager
func NewACMEManager(email string, domains []string, cacheDir string) *ACMEManager {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      email,
		HostPolicy: autocert.HostWhitelist(domains...),
		Cache:      autocert.DirCache(cacheDir),
	}

	return &ACMEManager{
		manager: m,
		domains: domains,
	}
}

// CertificateInfo contains information about a certificate
type CertificateInfo struct {
	Domain    string
	NotBefore time.Time
	NotAfter  time.Time
	DaysLeft  int
	IsNew     bool
}

// EnsureCertificates obtains/validates certificates for all configured domains at startup
// The HTTP challenge server must be running before calling this method
// Returns info about each certificate
func (a *ACMEManager) EnsureCertificates(ctx context.Context) ([]CertificateInfo, error) {
	var results []CertificateInfo

	for _, domain := range a.domains {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		// Create a fake ClientHelloInfo to trigger certificate fetch
		hello := &tls.ClientHelloInfo{
			ServerName: domain,
		}

		// GetCertificate will fetch from cache or obtain new certificate from Let's Encrypt
		// If certificate is about to expire, autocert will automatically renew it
		cert, err := a.manager.GetCertificate(hello)
		if err != nil {
			return results, fmt.Errorf("failed to obtain certificate for %s: %w", domain, err)
		}

		// Parse certificate to get expiration info
		if cert != nil && len(cert.Certificate) > 0 {
			leaf := cert.Leaf
			if leaf == nil && cert.Certificate != nil {
				// Parse the leaf certificate if not already parsed
				var parseErr error
				leaf, parseErr = parseCertificate(cert.Certificate[0])
				if parseErr != nil {
					return results, fmt.Errorf("failed to parse certificate for %s: %w", domain, parseErr)
				}
			}

			if leaf != nil {
				daysLeft := int(time.Until(leaf.NotAfter).Hours() / 24)
				info := CertificateInfo{
					Domain:    domain,
					NotBefore: leaf.NotBefore,
					NotAfter:  leaf.NotAfter,
					DaysLeft:  daysLeft,
					IsNew:     daysLeft > 85, // Let's Encrypt certs are valid for 90 days
				}
				results = append(results, info)
			}
		}
	}

	return results, nil
}

// Domains returns the list of configured domains
func (a *ACMEManager) Domains() []string {
	return a.domains
}

// TLSConfig returns TLS configuration for use with servers
func (a *ACMEManager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: a.manager.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

// HTTPHandler returns HTTP handler for HTTP-01 ACME challenge
func (a *ACMEManager) HTTPHandler(fallback http.Handler) http.Handler {
	return a.manager.HTTPHandler(fallback)
}

// parseCertificate parses a DER-encoded certificate
func parseCertificate(der []byte) (*x509.Certificate, error) {
	return x509.ParseCertificate(der)
}
