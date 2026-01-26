package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
