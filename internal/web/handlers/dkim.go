package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/sendry"
)

// DKIMList shows all DKIM keys for a server
func (h *Handlers) DKIMList(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")

	// Verify server exists
	if _, err := h.sendry.GetClient(serverName); err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	keys, err := h.dkim.List()
	if err != nil {
		h.logger.Error("failed to list DKIM keys", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load DKIM keys")
		return
	}

	// Compute IsDeployed for each key
	for i := range keys {
		deployments, _ := h.dkim.GetDeployments(keys[i].ID)
		for _, d := range deployments {
			if d.ServerName == serverName && d.Status == "deployed" {
				keys[i].IsDeployed = true
				break
			}
		}
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
		h.error(w, http.StatusNotFound, "Server not found")
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
	serverName := r.PathValue("server")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	selector := strings.TrimSpace(r.FormValue("selector"))
	autoDeploy := r.FormValue("auto_deploy") == "on"

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

	// Auto-deploy to current server if requested
	if autoDeploy {
		client, err := h.sendry.GetClient(serverName)
		if err == nil {
			resp, err := client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
			if err != nil {
				h.dkim.CreateDeployment(key.ID, serverName, "failed", err.Error())
			} else {
				// Update domain config with DKIM settings
				h.updateDomainDKIM(r.Context(), client, key.Domain, key.Selector, resp.KeyFile)
				h.dkim.CreateDeployment(key.ID, serverName, "deployed", "")
			}
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/servers/%s/dkim/%s", serverName, key.ID), http.StatusSeeOther)
}

// DKIMView shows a single DKIM key
func (h *Handlers) DKIMView(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
	id := r.PathValue("id")

	// Verify server exists
	if _, err := h.sendry.GetClient(serverName); err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

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

	// Check if deployed to current server
	isDeployed := false
	for _, d := range key.Deployments {
		if d.ServerName == serverName && d.Status == "deployed" {
			isDeployed = true
			break
		}
	}

	// Get all servers for deployment
	servers := h.getServersStatus()

	// Filter out current server for "other servers" list
	var otherServers []map[string]any
	for _, s := range servers {
		if s["Name"] != serverName {
			otherServers = append(otherServers, s)
		}
	}

	data := map[string]any{
		"Title":        fmt.Sprintf("DKIM: %s._domainkey.%s", key.Selector, key.Domain),
		"Active":       "servers",
		"User":         h.getUserFromContext(r),
		"Key":          key,
		"DNSName":      key.Selector + "._domainkey." + key.Domain,
		"Servers":      servers,
		"OtherServers": otherServers,
		"ServerName":   serverName,
		"IsDeployed":   isDeployed,
	}

	h.render(w, "dkim_view", data)
}

// DKIMDelete deletes a DKIM key
func (h *Handlers) DKIMDelete(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
	id := r.PathValue("id")

	if err := h.dkim.Delete(id); err != nil {
		h.logger.Error("failed to delete DKIM key", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete DKIM key")
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/servers/%s/dkim", serverName), http.StatusSeeOther)
}

// DKIMDeploy deploys a DKIM key to selected servers
func (h *Handlers) DKIMDeploy(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
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
	for _, srvName := range servers {
		client, err := h.sendry.GetClient(srvName)
		if err != nil {
			deployErrors = append(deployErrors, fmt.Sprintf("%s: %v", srvName, err))
			h.dkim.CreateDeployment(key.ID, srvName, "failed", err.Error())
			continue
		}

		resp, err := client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
		if err != nil {
			deployErrors = append(deployErrors, fmt.Sprintf("%s: %v", srvName, err))
			h.dkim.CreateDeployment(key.ID, srvName, "failed", err.Error())
		} else {
			// Update domain config with DKIM settings
			h.updateDomainDKIM(r.Context(), client, key.Domain, key.Selector, resp.KeyFile)
			h.dkim.CreateDeployment(key.ID, srvName, "deployed", "")
		}
	}

	if len(deployErrors) > 0 {
		h.logger.Error("some deployments failed", "errors", deployErrors)
	}

	http.Redirect(w, r, fmt.Sprintf("/servers/%s/dkim/%s", serverName, id), http.StatusSeeOther)
}

// DKIMDeploymentDelete removes a deployment record and optionally the key from server
func (h *Handlers) DKIMDeploymentDelete(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
	id := r.PathValue("id")

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

	http.Redirect(w, r, fmt.Sprintf("/servers/%s/dkim/%s", serverName, id), http.StatusSeeOther)
}

// ============================================================================
// Central DKIM Management Handlers
// ============================================================================

// CentralDKIMList shows all DKIM keys (central management)
func (h *Handlers) CentralDKIMList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.dkim.List()
	if err != nil {
		h.logger.Error("failed to list DKIM keys", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load DKIM keys")
		return
	}

	// Load deployments for each key
	for i := range keys {
		deployments, _ := h.dkim.GetDeployments(keys[i].ID)
		keys[i].Deployments = deployments
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
		"Title":   "New DKIM Key",
		"Active":  "dkim",
		"User":    h.getUserFromContext(r),
		"Servers": h.getServersStatus(),
	}

	h.render(w, "central_dkim_new", data)
}

// CentralDKIMCreate creates a new DKIM key (central management)
func (h *Handlers) CentralDKIMCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	selector := strings.TrimSpace(r.FormValue("selector"))
	deployServers := r.Form["servers"]

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

	// Deploy to selected servers
	for _, srvName := range deployServers {
		client, err := h.sendry.GetClient(srvName)
		if err != nil {
			h.dkim.CreateDeployment(key.ID, srvName, "failed", err.Error())
			continue
		}

		resp, err := client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
		if err != nil {
			h.dkim.CreateDeployment(key.ID, srvName, "failed", err.Error())
		} else {
			// Update domain config with DKIM settings
			h.updateDomainDKIM(r.Context(), client, key.Domain, key.Selector, resp.KeyFile)
			h.dkim.CreateDeployment(key.ID, srvName, "deployed", "")
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/dkim/%s", key.ID), http.StatusSeeOther)
}

// CentralDKIMView shows a single DKIM key (central management)
func (h *Handlers) CentralDKIMView(w http.ResponseWriter, r *http.Request) {
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

	// Get all servers
	servers := h.getServersStatus()

	// Mark which servers have this key deployed
	deployedMap := make(map[string]models.DKIMDeployment)
	for _, d := range key.Deployments {
		deployedMap[d.ServerName] = d
	}

	data := map[string]any{
		"Title":       fmt.Sprintf("DKIM: %s._domainkey.%s", key.Selector, key.Domain),
		"Active":      "dkim",
		"User":        h.getUserFromContext(r),
		"Key":         key,
		"DNSName":     key.Selector + "._domainkey." + key.Domain,
		"Servers":     servers,
		"DeployedMap": deployedMap,
	}

	h.render(w, "central_dkim_view", data)
}

// CentralDKIMDelete deletes a DKIM key (central management)
func (h *Handlers) CentralDKIMDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	key, err := h.dkim.GetByID(id)
	if err != nil || key == nil {
		h.error(w, http.StatusNotFound, "DKIM key not found")
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
		h.error(w, http.StatusInternalServerError, "Failed to delete DKIM key")
		return
	}

	http.Redirect(w, r, "/dkim", http.StatusSeeOther)
}

// CentralDKIMDeploy deploys a DKIM key to selected servers (central management)
func (h *Handlers) CentralDKIMDeploy(w http.ResponseWriter, r *http.Request) {
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
	for _, srvName := range servers {
		client, err := h.sendry.GetClient(srvName)
		if err != nil {
			deployErrors = append(deployErrors, fmt.Sprintf("%s: %v", srvName, err))
			h.dkim.CreateDeployment(key.ID, srvName, "failed", err.Error())
			continue
		}

		resp, err := client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
		if err != nil {
			deployErrors = append(deployErrors, fmt.Sprintf("%s: %v", srvName, err))
			h.dkim.CreateDeployment(key.ID, srvName, "failed", err.Error())
		} else {
			// Update domain config with DKIM settings
			h.updateDomainDKIM(r.Context(), client, key.Domain, key.Selector, resp.KeyFile)
			h.dkim.CreateDeployment(key.ID, srvName, "deployed", "")
		}
	}

	if len(deployErrors) > 0 {
		h.logger.Error("some deployments failed", "errors", deployErrors)
	}

	http.Redirect(w, r, fmt.Sprintf("/dkim/%s", id), http.StatusSeeOther)
}

// CentralDKIMDeploymentDelete removes a deployment from a specific server (central management)
func (h *Handlers) CentralDKIMDeploymentDelete(w http.ResponseWriter, r *http.Request) {
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

	http.Redirect(w, r, fmt.Sprintf("/dkim/%s", id), http.StatusSeeOther)
}

// updateDomainDKIM updates domain configuration with DKIM settings after key upload
func (h *Handlers) updateDomainDKIM(ctx context.Context, client *sendry.Client, domain, selector, keyFile string) {
	h.logger.Info("updateDomainDKIM called", "domain", domain, "selector", selector, "keyFile", keyFile)

	// Get current domain config or create new
	existingDomain, err := client.GetDomain(ctx, domain)

	var mode string
	if err == nil && existingDomain != nil {
		mode = existingDomain.Mode
	}
	if mode == "" {
		mode = "production"
	}

	// Update domain with DKIM config
	resp, err := client.UpdateDomain(ctx, domain, &sendry.DomainUpdateRequest{
		Mode: mode,
		DKIM: &sendry.DKIMConfig{
			Enabled:  true,
			Selector: selector,
			KeyFile:  keyFile,
		},
	})
	if err != nil {
		h.logger.Error("failed to update domain DKIM config", "domain", domain, "error", err)
	} else {
		h.logger.Info("domain DKIM config updated", "domain", domain, "response_dkim", resp.DKIM)
	}
}
