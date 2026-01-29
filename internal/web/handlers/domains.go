package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/sendry"
)

// DomainsList shows domains for a server
func (h *Handlers) DomainsList(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	domains, err := client.ListDomains(r.Context())
	if err != nil {
		h.logger.Error("failed to list domains", "error", err, "server", serverName)
		h.error(w, http.StatusInternalServerError, "Failed to load domains")
		return
	}

	// Get DKIM keys for linking
	dkimKeys, _ := h.dkim.List()

	data := map[string]any{
		"Title":      fmt.Sprintf("%s - Domains", serverName),
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": serverName,
		"Domains":    domains.Domains,
		"DKIMKeys":   dkimKeys,
	}

	h.render(w, "domains_list", data)
}

// DomainsNew shows new domain form
func (h *Handlers) DomainsNew(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")

	// Get DKIM keys for selection
	dkimKeys, _ := h.dkim.List()

	data := map[string]any{
		"Title":      "New Domain",
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": serverName,
		"DKIMKeys":   dkimKeys,
		"Modes":      []string{"production", "sandbox", "redirect", "bcc"},
	}

	h.render(w, "domain_new", data)
}

// DomainsCreate creates a new domain
func (h *Handlers) DomainsCreate(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	if domain == "" {
		h.error(w, http.StatusBadRequest, "Domain name is required")
		return
	}

	req := &sendry.DomainCreateRequest{
		Domain:      domain,
		Mode:        r.FormValue("mode"),
		DefaultFrom: r.FormValue("default_from"),
	}

	// Parse DKIM settings
	if r.FormValue("dkim_enabled") == "on" {
		selector := r.FormValue("dkim_selector")
		if selector == "" {
			selector = "mail"
		}
		req.DKIM = &sendry.DKIMConfig{
			Enabled:  true,
			Selector: selector,
		}

		// Deploy DKIM key if selected
		dkimKeyID := r.FormValue("dkim_key_id")
		if dkimKeyID != "" {
			key, err := h.dkim.GetByID(dkimKeyID)
			if err == nil && key != nil && key.Domain == domain {
				_, err := client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
				if err != nil {
					h.logger.Error("failed to deploy DKIM key", "error", err)
				} else {
					h.dkim.CreateDeployment(key.ID, serverName, "deployed", "")
				}
			}
		}
	}

	// Parse rate limits
	msgsPerHour, _ := strconv.Atoi(r.FormValue("rate_limit_hour"))
	msgsPerDay, _ := strconv.Atoi(r.FormValue("rate_limit_day"))
	recipientsPerMsg, _ := strconv.Atoi(r.FormValue("rate_limit_recipients"))
	if msgsPerHour > 0 || msgsPerDay > 0 || recipientsPerMsg > 0 {
		req.RateLimit = &sendry.RateLimitCfg{
			MessagesPerHour:      msgsPerHour,
			MessagesPerDay:       msgsPerDay,
			RecipientsPerMessage: recipientsPerMsg,
		}
	}

	// Parse redirect/bcc addresses
	if req.Mode == "redirect" {
		redirectTo := strings.TrimSpace(r.FormValue("redirect_to"))
		if redirectTo != "" {
			req.RedirectTo = strings.Split(redirectTo, "\n")
			for i := range req.RedirectTo {
				req.RedirectTo[i] = strings.TrimSpace(req.RedirectTo[i])
			}
		}
	}
	if req.Mode == "bcc" {
		bccTo := strings.TrimSpace(r.FormValue("bcc_to"))
		if bccTo != "" {
			req.BCCTo = strings.Split(bccTo, "\n")
			for i := range req.BCCTo {
				req.BCCTo[i] = strings.TrimSpace(req.BCCTo[i])
			}
		}
	}

	_, err = client.CreateDomain(r.Context(), req)
	if err != nil {
		h.logger.Error("failed to create domain", "error", err)
		h.error(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create domain: %v", err))
		return
	}

	http.Redirect(w, r, "/servers/"+serverName+"/domains", http.StatusSeeOther)
}

// DomainsView shows domain details
func (h *Handlers) DomainsView(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
	domainName := r.PathValue("domain")

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	domain, err := client.GetDomain(r.Context(), domainName)
	if err != nil {
		h.logger.Error("failed to get domain", "error", err)
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Get DKIM keys
	dkimKeys, _ := h.dkim.List()

	data := map[string]any{
		"Title":      fmt.Sprintf("Domain: %s", domainName),
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": serverName,
		"Domain":     domain,
		"DKIMKeys":   dkimKeys,
		"Modes":      []string{"production", "sandbox", "redirect", "bcc"},
	}

	h.render(w, "domain_view", data)
}

// DomainsEdit shows domain edit form
func (h *Handlers) DomainsEdit(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
	domainName := r.PathValue("domain")

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	domain, err := client.GetDomain(r.Context(), domainName)
	if err != nil {
		h.logger.Error("failed to get domain", "error", err)
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Get DKIM keys
	dkimKeys, _ := h.dkim.List()

	data := map[string]any{
		"Title":      fmt.Sprintf("Edit Domain: %s", domainName),
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": serverName,
		"Domain":     domain,
		"DKIMKeys":   dkimKeys,
		"Modes":      []string{"production", "sandbox", "redirect", "bcc"},
	}

	h.render(w, "domain_edit", data)
}

// DomainsUpdate updates a domain
func (h *Handlers) DomainsUpdate(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
	domainName := r.PathValue("domain")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	req := &sendry.DomainUpdateRequest{
		Mode:        r.FormValue("mode"),
		DefaultFrom: r.FormValue("default_from"),
	}

	// Parse DKIM settings
	if r.FormValue("dkim_enabled") == "on" {
		selector := r.FormValue("dkim_selector")
		if selector == "" {
			selector = "mail"
		}
		req.DKIM = &sendry.DKIMConfig{
			Enabled:  true,
			Selector: selector,
		}

		// Deploy DKIM key if selected
		dkimKeyID := r.FormValue("dkim_key_id")
		if dkimKeyID != "" {
			key, err := h.dkim.GetByID(dkimKeyID)
			if err == nil && key != nil && key.Domain == domainName {
				_, err := client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
				if err != nil {
					h.logger.Error("failed to deploy DKIM key", "error", err)
				} else {
					h.dkim.CreateDeployment(key.ID, serverName, "deployed", "")
				}
			}
		}
	} else {
		req.DKIM = &sendry.DKIMConfig{Enabled: false}
	}

	// Parse rate limits
	msgsPerHour, _ := strconv.Atoi(r.FormValue("rate_limit_hour"))
	msgsPerDay, _ := strconv.Atoi(r.FormValue("rate_limit_day"))
	recipientsPerMsg, _ := strconv.Atoi(r.FormValue("rate_limit_recipients"))
	if msgsPerHour > 0 || msgsPerDay > 0 || recipientsPerMsg > 0 {
		req.RateLimit = &sendry.RateLimitCfg{
			MessagesPerHour:      msgsPerHour,
			MessagesPerDay:       msgsPerDay,
			RecipientsPerMessage: recipientsPerMsg,
		}
	}

	// Parse redirect/bcc addresses
	if req.Mode == "redirect" {
		redirectTo := strings.TrimSpace(r.FormValue("redirect_to"))
		if redirectTo != "" {
			req.RedirectTo = strings.Split(redirectTo, "\n")
			for i := range req.RedirectTo {
				req.RedirectTo[i] = strings.TrimSpace(req.RedirectTo[i])
			}
		}
	}
	if req.Mode == "bcc" {
		bccTo := strings.TrimSpace(r.FormValue("bcc_to"))
		if bccTo != "" {
			req.BCCTo = strings.Split(bccTo, "\n")
			for i := range req.BCCTo {
				req.BCCTo[i] = strings.TrimSpace(req.BCCTo[i])
			}
		}
	}

	_, err = client.UpdateDomain(r.Context(), domainName, req)
	if err != nil {
		h.logger.Error("failed to update domain", "error", err)
		h.error(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update domain: %v", err))
		return
	}

	http.Redirect(w, r, "/servers/"+serverName+"/domains/"+domainName, http.StatusSeeOther)
}

// DomainsDelete deletes a domain
func (h *Handlers) DomainsDelete(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")
	domainName := r.PathValue("domain")

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	err = client.DeleteDomain(r.Context(), domainName)
	if err != nil {
		h.logger.Error("failed to delete domain", "error", err)
		h.error(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete domain: %v", err))
		return
	}

	http.Redirect(w, r, "/servers/"+serverName+"/domains", http.StatusSeeOther)
}

// ============================================================================
// Central Domain Management Handlers
// ============================================================================

// CentralDomainsList shows all locally stored domains
func (h *Handlers) CentralDomainsList(w http.ResponseWriter, r *http.Request) {
	domains, err := h.domains.List(models.DomainFilter{})
	if err != nil {
		h.logger.Error("failed to list domains", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load domains")
		return
	}

	// Load deployments for each domain
	for i := range domains {
		deployments, _ := h.domains.GetDeployments(domains[i].ID)
		domains[i].Deployments = deployments
	}

	data := map[string]any{
		"Title":   "Domains",
		"Active":  "domains",
		"User":    h.getUserFromContext(r),
		"Domains": domains,
		"Servers": h.getServersStatus(),
	}

	h.render(w, "central_domains_list", data)
}

// CentralDomainsNew shows the new domain form
func (h *Handlers) CentralDomainsNew(w http.ResponseWriter, r *http.Request) {
	dkimKeys, _ := h.dkim.List()

	data := map[string]any{
		"Title":    "New Domain",
		"Active":   "domains",
		"User":     h.getUserFromContext(r),
		"Servers":  h.getServersStatus(),
		"DKIMKeys": dkimKeys,
		"Modes":    []string{"production", "sandbox", "redirect", "bcc"},
	}

	h.render(w, "central_domain_new", data)
}

// CentralDomainsCreate creates a new domain locally
func (h *Handlers) CentralDomainsCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	domainName := strings.TrimSpace(r.FormValue("domain"))
	if domainName == "" {
		h.error(w, http.StatusBadRequest, "Domain name is required")
		return
	}

	// Check if domain already exists
	existing, _ := h.domains.GetByDomain(domainName)
	if existing != nil {
		h.error(w, http.StatusConflict, "Domain already exists")
		return
	}

	domain := &models.Domain{
		Domain:      domainName,
		Mode:        r.FormValue("mode"),
		DefaultFrom: r.FormValue("default_from"),
	}

	// Parse DKIM settings
	if r.FormValue("dkim_enabled") == "on" {
		domain.DKIMEnabled = true
		domain.DKIMSelector = r.FormValue("dkim_selector")
		if domain.DKIMSelector == "" {
			domain.DKIMSelector = "mail"
		}
		domain.DKIMKeyID = r.FormValue("dkim_key_id")
	}

	// Parse rate limits
	domain.RateLimitHour, _ = strconv.Atoi(r.FormValue("rate_limit_hour"))
	domain.RateLimitDay, _ = strconv.Atoi(r.FormValue("rate_limit_day"))
	domain.RateLimitRecipients, _ = strconv.Atoi(r.FormValue("rate_limit_recipients"))

	// Parse redirect/bcc addresses
	if domain.Mode == "redirect" {
		redirectTo := strings.TrimSpace(r.FormValue("redirect_to"))
		if redirectTo != "" {
			domain.RedirectTo = parseAddresses(redirectTo)
		}
	}
	if domain.Mode == "bcc" {
		bccTo := strings.TrimSpace(r.FormValue("bcc_to"))
		if bccTo != "" {
			domain.BCCTo = parseAddresses(bccTo)
		}
	}

	if err := h.domains.Create(domain); err != nil {
		h.logger.Error("failed to create domain", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to create domain")
		return
	}

	// Deploy to selected servers
	deployServers := r.Form["servers"]
	for _, srvName := range deployServers {
		h.deployDomainToServer(r, domain, srvName)
	}

	http.Redirect(w, r, fmt.Sprintf("/domains/%s", domain.ID), http.StatusSeeOther)
}

// CentralDomainsView shows domain details
func (h *Handlers) CentralDomainsView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	domain, err := h.domains.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get domain", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load domain")
		return
	}
	if domain == nil {
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Load DKIM key if linked
	if domain.DKIMKeyID != "" {
		dkimKey, _ := h.dkim.GetByID(domain.DKIMKeyID)
		domain.DKIMKey = dkimKey
	}

	servers := h.getServersStatus()
	deployedMap := make(map[string]models.DomainDeployment)
	for _, d := range domain.Deployments {
		deployedMap[d.ServerName] = d
	}

	// Check for outdated deployments
	currentHash := domain.ConfigHash()
	outdatedCount := 0
	for _, d := range domain.Deployments {
		if d.ConfigHash != currentHash && d.Status == "deployed" {
			outdatedCount++
		}
	}

	data := map[string]any{
		"Title":         fmt.Sprintf("Domain: %s", domain.Domain),
		"Active":        "domains",
		"User":          h.getUserFromContext(r),
		"Domain":        domain,
		"Servers":       servers,
		"DeployedMap":   deployedMap,
		"OutdatedCount": outdatedCount,
		"ConfigHash":    currentHash,
	}

	h.render(w, "central_domain_view", data)
}

// CentralDomainsEdit shows domain edit form
func (h *Handlers) CentralDomainsEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	domain, err := h.domains.GetByID(id)
	if err != nil || domain == nil {
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	dkimKeys, _ := h.dkim.List()

	data := map[string]any{
		"Title":    fmt.Sprintf("Edit Domain: %s", domain.Domain),
		"Active":   "domains",
		"User":     h.getUserFromContext(r),
		"Domain":   domain,
		"DKIMKeys": dkimKeys,
		"Modes":    []string{"production", "sandbox", "redirect", "bcc"},
	}

	h.render(w, "central_domain_edit", data)
}

// CentralDomainsUpdate updates a domain
func (h *Handlers) CentralDomainsUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	domain, err := h.domains.GetByID(id)
	if err != nil || domain == nil {
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	domain.Mode = r.FormValue("mode")
	domain.DefaultFrom = r.FormValue("default_from")

	// Parse DKIM settings
	if r.FormValue("dkim_enabled") == "on" {
		domain.DKIMEnabled = true
		domain.DKIMSelector = r.FormValue("dkim_selector")
		if domain.DKIMSelector == "" {
			domain.DKIMSelector = "mail"
		}
		domain.DKIMKeyID = r.FormValue("dkim_key_id")
	} else {
		domain.DKIMEnabled = false
		domain.DKIMSelector = ""
		domain.DKIMKeyID = ""
	}

	// Parse rate limits
	domain.RateLimitHour, _ = strconv.Atoi(r.FormValue("rate_limit_hour"))
	domain.RateLimitDay, _ = strconv.Atoi(r.FormValue("rate_limit_day"))
	domain.RateLimitRecipients, _ = strconv.Atoi(r.FormValue("rate_limit_recipients"))

	// Parse redirect/bcc addresses
	domain.RedirectTo = nil
	domain.BCCTo = nil
	if domain.Mode == "redirect" {
		redirectTo := strings.TrimSpace(r.FormValue("redirect_to"))
		if redirectTo != "" {
			domain.RedirectTo = parseAddresses(redirectTo)
		}
	}
	if domain.Mode == "bcc" {
		bccTo := strings.TrimSpace(r.FormValue("bcc_to"))
		if bccTo != "" {
			domain.BCCTo = parseAddresses(bccTo)
		}
	}

	if err := h.domains.Update(id, domain); err != nil {
		h.logger.Error("failed to update domain", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to update domain")
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/domains/%s", id), http.StatusSeeOther)
}

// CentralDomainsDelete deletes a domain
func (h *Handlers) CentralDomainsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	domain, err := h.domains.GetByID(id)
	if err != nil || domain == nil {
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Optionally delete from servers
	for _, d := range domain.Deployments {
		client, err := h.sendry.GetClient(d.ServerName)
		if err == nil {
			_ = client.DeleteDomain(r.Context(), domain.Domain)
		}
	}

	if err := h.domains.Delete(id); err != nil {
		h.logger.Error("failed to delete domain", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete domain")
		return
	}

	http.Redirect(w, r, "/domains", http.StatusSeeOther)
}

// CentralDomainsDeploy deploys a domain to selected servers
func (h *Handlers) CentralDomainsDeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	domain, err := h.domains.GetByID(id)
	if err != nil || domain == nil {
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	servers := r.Form["servers"]
	if len(servers) == 0 {
		h.error(w, http.StatusBadRequest, "No servers selected")
		return
	}

	for _, srvName := range servers {
		h.deployDomainToServer(r, domain, srvName)
	}

	http.Redirect(w, r, fmt.Sprintf("/domains/%s", id), http.StatusSeeOther)
}

// CentralDomainsSync syncs all outdated deployments
func (h *Handlers) CentralDomainsSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	domain, err := h.domains.GetByID(id)
	if err != nil || domain == nil {
		h.error(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Find all outdated deployments
	currentHash := domain.ConfigHash()
	for _, d := range domain.Deployments {
		if d.ConfigHash != currentHash && d.Status != "failed" {
			h.deployDomainToServer(r, domain, d.ServerName)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/domains/%s", id), http.StatusSeeOther)
}

// CentralDomainsImport imports domain configuration from a server
func (h *Handlers) CentralDomainsImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	serverName := r.FormValue("server")
	domainName := r.FormValue("domain")

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Server not found")
		return
	}

	serverDomain, err := client.GetDomain(r.Context(), domainName)
	if err != nil {
		h.error(w, http.StatusNotFound, "Domain not found on server")
		return
	}

	// Check if already exists locally
	existing, _ := h.domains.GetByDomain(domainName)
	if existing != nil {
		h.error(w, http.StatusConflict, "Domain already exists locally")
		return
	}

	// Create local domain from server config
	domain := &models.Domain{
		Domain:      serverDomain.Domain,
		Mode:        serverDomain.Mode,
		DefaultFrom: serverDomain.DefaultFrom,
	}

	if serverDomain.DKIM != nil && serverDomain.DKIM.Enabled {
		domain.DKIMEnabled = true
		domain.DKIMSelector = serverDomain.DKIM.Selector
		// Try to find matching DKIM key
		dkimKey, _ := h.dkim.GetByDomainSelector(domainName, domain.DKIMSelector)
		if dkimKey != nil {
			domain.DKIMKeyID = dkimKey.ID
		}
	}

	if serverDomain.RateLimit != nil {
		domain.RateLimitHour = serverDomain.RateLimit.MessagesPerHour
		domain.RateLimitDay = serverDomain.RateLimit.MessagesPerDay
		domain.RateLimitRecipients = serverDomain.RateLimit.RecipientsPerMessage
	}

	domain.RedirectTo = serverDomain.RedirectTo
	domain.BCCTo = serverDomain.BCCTo

	if err := h.domains.Create(domain); err != nil {
		h.logger.Error("failed to create domain", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to import domain")
		return
	}

	// Record deployment
	h.domains.CreateDeployment(domain.ID, serverName, "deployed", domain.ConfigHash(), "")

	http.Redirect(w, r, fmt.Sprintf("/domains/%s", domain.ID), http.StatusSeeOther)
}

// Helper to deploy domain to a server
func (h *Handlers) deployDomainToServer(r *http.Request, domain *models.Domain, serverName string) {
	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.domains.CreateDeployment(domain.ID, serverName, "failed", domain.ConfigHash(), err.Error())
		return
	}

	// Build request
	req := &sendry.DomainCreateRequest{
		Domain:      domain.Domain,
		Mode:        domain.Mode,
		DefaultFrom: domain.DefaultFrom,
	}

	if domain.DKIMEnabled {
		req.DKIM = &sendry.DKIMConfig{
			Enabled:  true,
			Selector: domain.DKIMSelector,
		}

		// Deploy DKIM key if linked
		if domain.DKIMKeyID != "" {
			key, _ := h.dkim.GetByID(domain.DKIMKeyID)
			if key != nil && key.Domain == domain.Domain {
				_, err := client.UploadDKIM(r.Context(), key.Domain, key.Selector, key.PrivateKey)
				if err != nil {
					h.logger.Error("failed to deploy DKIM key", "error", err)
				} else {
					h.dkim.CreateDeployment(key.ID, serverName, "deployed", "")
				}
			}
		}
	}

	if domain.RateLimitHour > 0 || domain.RateLimitDay > 0 || domain.RateLimitRecipients > 0 {
		req.RateLimit = &sendry.RateLimitCfg{
			MessagesPerHour:      domain.RateLimitHour,
			MessagesPerDay:       domain.RateLimitDay,
			RecipientsPerMessage: domain.RateLimitRecipients,
		}
	}

	req.RedirectTo = domain.RedirectTo
	req.BCCTo = domain.BCCTo

	// Try to update first, then create
	updateReq := &sendry.DomainUpdateRequest{
		Mode:        req.Mode,
		DefaultFrom: req.DefaultFrom,
		DKIM:        req.DKIM,
		RateLimit:   req.RateLimit,
		RedirectTo:  req.RedirectTo,
		BCCTo:       req.BCCTo,
	}

	_, err = client.UpdateDomain(r.Context(), domain.Domain, updateReq)
	if err != nil {
		// Domain doesn't exist, create it
		_, err = client.CreateDomain(r.Context(), req)
	}

	if err != nil {
		h.domains.CreateDeployment(domain.ID, serverName, "failed", domain.ConfigHash(), err.Error())
	} else {
		h.domains.CreateDeployment(domain.ID, serverName, "deployed", domain.ConfigHash(), "")
	}
}

// Helper to parse newline-separated addresses
func parseAddresses(s string) []string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
