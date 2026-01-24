package tls

import (
	"crypto/tls"
	"net/http"

	"golang.org/x/crypto/acme/autocert"
)

// ACMEManager manages automatic TLS certificates from Let's Encrypt
type ACMEManager struct {
	manager *autocert.Manager
}

// NewACMEManager creates a new ACME manager
func NewACMEManager(email string, domains []string, cacheDir string) *ACMEManager {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      email,
		HostPolicy: autocert.HostWhitelist(domains...),
		Cache:      autocert.DirCache(cacheDir),
	}

	return &ACMEManager{manager: m}
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
