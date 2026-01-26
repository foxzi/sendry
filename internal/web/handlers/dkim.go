package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"

	"github.com/foxzi/sendry/internal/web/models"
)

// DKIMList shows all DKIM keys
func (h *Handlers) DKIMList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.dkim.List()
	if err != nil {
		h.logger.Error("failed to list DKIM keys", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load DKIM keys")
		return
	}

	data := map[string]any{
		"Title":  "DKIM Keys",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
		"Keys":   keys,
	}

	h.render(w, "dkim_list", data)
}

// DKIMNew shows the new DKIM key form
func (h *Handlers) DKIMNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":  "New DKIM Key",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
	}

	h.render(w, "dkim_new", data)
}

// DKIMCreate creates a new DKIM key
func (h *Handlers) DKIMCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	selector := strings.TrimSpace(r.FormValue("selector"))

	if domain == "" || selector == "" {
		h.error(w, http.StatusBadRequest, "Domain and selector are required")
		return
	}

	// Check if key already exists
	existing, _ := h.dkim.GetByDomainSelector(domain, selector)
	if existing != nil {
		h.error(w, http.StatusConflict, "DKIM key for this domain and selector already exists")
		return
	}

	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		h.logger.Error("failed to generate RSA key", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to generate key")
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
		h.error(w, http.StatusInternalServerError, "Failed to generate public key")
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
		h.error(w, http.StatusInternalServerError, "Failed to save DKIM key")
		return
	}

	http.Redirect(w, r, "/settings/dkim/"+key.ID, http.StatusSeeOther)
}

// DKIMView shows a single DKIM key
func (h *Handlers) DKIMView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	key, err := h.dkim.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get DKIM key", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load DKIM key")
		return
	}
	if key == nil {
		h.error(w, http.StatusNotFound, "DKIM key not found")
		return
	}

	// Get server list for deployment
	servers := h.getServersStatus()

	data := map[string]any{
		"Title":   fmt.Sprintf("DKIM: %s._domainkey.%s", key.Selector, key.Domain),
		"Active":  "settings",
		"User":    h.getUserFromContext(r),
		"Key":     key,
		"DNSName": key.Selector + "._domainkey." + key.Domain,
		"Servers": servers,
	}

	h.render(w, "dkim_view", data)
}

// DKIMDelete deletes a DKIM key
func (h *Handlers) DKIMDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.dkim.Delete(id); err != nil {
		h.logger.Error("failed to delete DKIM key", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete DKIM key")
		return
	}

	http.Redirect(w, r, "/settings/dkim", http.StatusSeeOther)
}

// DKIMDeploy deploys a DKIM key to selected servers
func (h *Handlers) DKIMDeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	key, err := h.dkim.GetByID(id)
	if err != nil || key == nil {
		h.error(w, http.StatusNotFound, "DKIM key not found")
		return
	}

	servers := r.Form["servers"]
	if len(servers) == 0 {
		h.error(w, http.StatusBadRequest, "No servers selected")
		return
	}

	var deployErrors []string
	for _, serverName := range servers {
		client, err := h.sendry.GetClient(serverName)
		if err != nil {
			deployErrors = append(deployErrors, fmt.Sprintf("%s: %v", serverName, err))
			h.dkim.CreateDeployment(key.ID, serverName, "failed", err.Error())
			continue
		}

		_, err = client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
		if err != nil {
			deployErrors = append(deployErrors, fmt.Sprintf("%s: %v", serverName, err))
			h.dkim.CreateDeployment(key.ID, serverName, "failed", err.Error())
		} else {
			h.dkim.CreateDeployment(key.ID, serverName, "deployed", "")
		}
	}

	if len(deployErrors) > 0 {
		h.logger.Error("some deployments failed", "errors", deployErrors)
	}

	http.Redirect(w, r, "/settings/dkim/"+id, http.StatusSeeOther)
}

// DKIMDeploymentDelete removes a deployment record and optionally the key from server
func (h *Handlers) DKIMDeploymentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	serverName := r.PathValue("server")

	key, err := h.dkim.GetByID(id)
	if err != nil || key == nil {
		h.error(w, http.StatusNotFound, "DKIM key not found")
		return
	}

	// Try to delete from server
	client, err := h.sendry.GetClient(serverName)
	if err == nil {
		_ = client.DeleteDKIM(r.Context(), key.Domain, key.Selector)
	}

	// Remove deployment record
	if err := h.dkim.DeleteDeployment(key.ID, serverName); err != nil {
		h.logger.Error("failed to delete deployment", "error", err)
	}

	http.Redirect(w, r, "/settings/dkim/"+id, http.StatusSeeOther)
}
