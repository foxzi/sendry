package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/foxzi/sendry/internal/web/models"
)

// DKIMList shows all DKIM keys for a server
func (h *Handlers) DKIMList(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")

	// Verify server exists
	if _, err := h.sendry.GetClient(serverName); err != nil {
		h.error(w, r, http.StatusNotFound, "Server not found")
		return
	}

	keys, err := h.dkim.ListNewestFirst()
	if err != nil {
		h.logger.Error("failed to list DKIM keys", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to load DKIM keys")
		return
	}

	data := map[string]any{
		"Title":      "DKIM Keys",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"Keys":       keys,
		"ServerName": serverName,
	}

	h.render(w, "dkim_list", data)
}

// DKIMNew shows the new DKIM key form
func (h *Handlers) DKIMNew(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")

	// Verify server exists
	if _, err := h.sendry.GetClient(serverName); err != nil {
		h.error(w, r, http.StatusNotFound, "Server not found")
		return
	}

	data := map[string]any{
		"Title":      "New DKIM Key",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": serverName,
	}

	h.render(w, "dkim_new", data)
}

// DKIMCreate creates a new DKIM key
func (h *Handlers) DKIMCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	selector := strings.TrimSpace(r.FormValue("selector"))

	if domain == "" || selector == "" {
		h.error(w, r, http.StatusBadRequest, "Domain and selector are required")
		return
	}

	// Check if key already exists
	existing, _ := h.dkim.GetByDomainSelector(domain, selector)
	if existing != nil {
		h.error(w, r, http.StatusConflict, "DKIM key for this domain and selector already exists")
		return
	}

	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		h.logger.Error("failed to generate RSA key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to generate key")
		return
	}

	// Encode private key to PEM
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Generate public key for DNS record
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		h.logger.Error("failed to marshal public key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to generate public key")
		return
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	// Create DNS record value
	pubKeyBase64 := strings.ReplaceAll(string(publicKeyPEM), "-----BEGIN PUBLIC KEY-----", "")
	pubKeyBase64 = strings.ReplaceAll(pubKeyBase64, "-----END PUBLIC KEY-----", "")
	pubKeyBase64 = strings.ReplaceAll(pubKeyBase64, "\n", "")
	dnsRecord := fmt.Sprintf("v=DKIM1; k=rsa; p=%s", pubKeyBase64)

	// Save to database
	key := &models.DKIMKey{
		Domain:     domain,
		Selector:   selector,
		PrivateKey: string(privateKeyPEM),
		DNSRecord:  dnsRecord,
	}

	if err := h.dkim.Create(key); err != nil {
		h.logger.Error("failed to create DKIM key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to save DKIM key")
		return
	}

	// The key reaches a server only together with a domain that uses it
	// (see deployDomainToServer): uploading it here would switch the domain
	// to the new selector before its DNS record exists.
	http.Redirect(w, r, fmt.Sprintf("/dkim/%s", key.ID), http.StatusSeeOther)
}

// DKIMView redirects to the panel's DKIM key page: a key is one record for
// all servers, and that page lists every server with its deployment state.
func (h *Handlers) DKIMView(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dkim/"+url.PathEscape(r.PathValue("id")), http.StatusSeeOther)
}

// ============================================================================
// Central DKIM Management Handlers
// ============================================================================

// CentralDKIMList shows all DKIM keys (central management)
func (h *Handlers) CentralDKIMList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.dkim.ListNewestFirst()
	if err != nil {
		h.logger.Error("failed to list DKIM keys", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to load DKIM keys")
		return
	}

	data := map[string]any{
		"Title":   "DKIM Keys",
		"Active":  "dkim",
		"User":    h.getUserFromContext(r),
		"Keys":    keys,
		"Servers": h.getServersStatus(),
	}

	h.render(w, "central_dkim_list", data)
}

// CentralDKIMNew shows the new DKIM key form (central management)
func (h *Handlers) CentralDKIMNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":  "New DKIM Key",
		"Active": "dkim",
		"User":   h.getUserFromContext(r),
	}

	h.render(w, "central_dkim_new", data)
}

// CentralDKIMCreate creates a new DKIM key (central management)
func (h *Handlers) CentralDKIMCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	selector := strings.TrimSpace(r.FormValue("selector"))

	if domain == "" || selector == "" {
		h.error(w, r, http.StatusBadRequest, "Domain and selector are required")
		return
	}

	// Check if key already exists
	existing, _ := h.dkim.GetByDomainSelector(domain, selector)
	if existing != nil {
		h.error(w, r, http.StatusConflict, "DKIM key for this domain and selector already exists")
		return
	}

	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		h.logger.Error("failed to generate RSA key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to generate key")
		return
	}

	// Encode private key to PEM
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Generate public key for DNS record
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		h.logger.Error("failed to marshal public key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to generate public key")
		return
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	// Create DNS record value
	pubKeyBase64 := strings.ReplaceAll(string(publicKeyPEM), "-----BEGIN PUBLIC KEY-----", "")
	pubKeyBase64 = strings.ReplaceAll(pubKeyBase64, "-----END PUBLIC KEY-----", "")
	pubKeyBase64 = strings.ReplaceAll(pubKeyBase64, "\n", "")
	dnsRecord := fmt.Sprintf("v=DKIM1; k=rsa; p=%s", pubKeyBase64)

	// Save to database
	key := &models.DKIMKey{
		Domain:     domain,
		Selector:   selector,
		PrivateKey: string(privateKeyPEM),
		DNSRecord:  dnsRecord,
	}

	if err := h.dkim.Create(key); err != nil {
		h.logger.Error("failed to create DKIM key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to save DKIM key")
		return
	}

	// Not uploaded to servers here: see DKIMCreate.
	http.Redirect(w, r, fmt.Sprintf("/dkim/%s", key.ID), http.StatusSeeOther)
}

// CentralDKIMView shows a single DKIM key (central management)
func (h *Handlers) CentralDKIMView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	key, err := h.dkim.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get DKIM key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to load DKIM key")
		return
	}
	if key == nil {
		h.error(w, r, http.StatusNotFound, "DKIM key not found")
		return
	}

	// The key is uploaded to servers by deploying a domain that uses it, so
	// point at those domains. A key may be linked to a domain with another
	// name; with no link yet, suggest the domain the key was made for.
	linkedDomains, err := h.domains.ListByDKIMKey(key.ID)
	if err != nil {
		h.logger.Error("failed to list domains of DKIM key", "error", err)
	}
	var panelDomain *models.Domain
	if len(linkedDomains) == 0 {
		panelDomain, _ = h.domains.GetByDomain(key.Domain)
	}

	data := map[string]any{
		"LinkedDomains": linkedDomains,
		"PanelDomain":   panelDomain,
		"Title":         fmt.Sprintf("DKIM: %s._domainkey.%s", key.Selector, key.Domain),
		"Active":        "dkim",
		"User":          h.getUserFromContext(r),
		"Key":           key,
		"DNSName":       key.Selector + "._domainkey." + key.Domain,
	}

	h.render(w, "central_dkim_view", data)
}

// CentralDKIMDelete deletes a DKIM key (central management)
func (h *Handlers) CentralDKIMDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	key, err := h.dkim.GetByID(id)
	if err != nil || key == nil {
		h.error(w, r, http.StatusNotFound, "DKIM key not found")
		return
	}

	// Try to delete from all servers where deployed
	for _, d := range key.Deployments {
		client, err := h.sendry.GetClient(d.ServerName)
		if err == nil {
			_ = client.DeleteDKIM(r.Context(), key.Domain, key.Selector)
		}
	}

	if err := h.dkim.Delete(id); err != nil {
		h.logger.Error("failed to delete DKIM key", "error", err)
		h.error(w, r, http.StatusInternalServerError, "Failed to delete DKIM key")
		return
	}

	http.Redirect(w, r, "/dkim", http.StatusSeeOther)
}
