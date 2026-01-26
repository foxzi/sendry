package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/dkim"
	"github.com/foxzi/sendry/internal/domain"
	"github.com/foxzi/sendry/internal/ratelimit"
)

// ManagementServer handles domain, DKIM, TLS, and rate limit management APIs
type ManagementServer struct {
	domainManager *domain.Manager
	rateLimiter   *ratelimit.Limiter
	config        *config.Config
	dkimKeysDir   string
	tlsCertsDir   string
}

// NewManagementServer creates a new management server
func NewManagementServer(
	domainMgr *domain.Manager,
	rateLimiter *ratelimit.Limiter,
	cfg *config.Config,
	dkimKeysDir string,
	tlsCertsDir string,
) *ManagementServer {
	return &ManagementServer{
		domainManager: domainMgr,
		rateLimiter:   rateLimiter,
		config:        cfg,
		dkimKeysDir:   dkimKeysDir,
		tlsCertsDir:   tlsCertsDir,
	}
}

// RegisterRoutes registers management API routes
func (m *ManagementServer) RegisterRoutes(r chi.Router) {
	// DKIM management
	r.Route("/dkim", func(r chi.Router) {
		r.Post("/generate", m.handleDKIMGenerate)
		r.Post("/upload", m.handleDKIMUpload)
		r.Get("/{domain}", m.handleDKIMGet)
		r.Get("/{domain}/verify", m.handleDKIMVerify)
		r.Delete("/{domain}/{selector}", m.handleDKIMDelete)
	})

	// TLS management
	r.Route("/tls", func(r chi.Router) {
		r.Get("/certificates", m.handleTLSList)
		r.Post("/certificates", m.handleTLSUpload)
		r.Post("/letsencrypt/{domain}", m.handleTLSLetsEncrypt)
	})

	// Domains management
	r.Route("/domains", func(r chi.Router) {
		r.Get("/", m.handleDomainsList)
		r.Post("/", m.handleDomainsCreate)
		r.Get("/{domain}", m.handleDomainsGet)
		r.Put("/{domain}", m.handleDomainsUpdate)
		r.Delete("/{domain}", m.handleDomainsDelete)
	})

	// Rate limits management
	r.Route("/ratelimits", func(r chi.Router) {
		r.Get("/", m.handleRateLimitsGet)
		r.Get("/{level}/{key}", m.handleRateLimitStats)
		r.Put("/{domain}", m.handleRateLimitsUpdate)
	})
}

// DKIM Handlers

// DKIMGenerateRequest is the request for POST /api/v1/dkim/generate
type DKIMGenerateRequest struct {
	Domain   string `json:"domain"`
	Selector string `json:"selector"`
}

// DKIMGenerateResponse is the response for POST /api/v1/dkim/generate
type DKIMGenerateResponse struct {
	Domain    string `json:"domain"`
	Selector  string `json:"selector"`
	DNSName   string `json:"dns_name"`
	DNSRecord string `json:"dns_record"`
	KeyFile   string `json:"key_file"`
}

// handleDKIMGenerate handles POST /api/v1/dkim/generate
func (m *ManagementServer) handleDKIMGenerate(w http.ResponseWriter, r *http.Request) {
	var req DKIMGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if req.Selector == "" {
		req.Selector = "default"
	}

	// Generate DKIM key
	keyPair, err := dkim.GenerateKey(req.Domain, req.Selector)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to generate DKIM key")
		return
	}

	// Save private key
	keyFile := filepath.Join(m.dkimKeysDir, req.Domain, req.Selector+".key")
	if err := keyPair.SavePrivateKey(keyFile); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to save DKIM key")
		return
	}

	sendJSON(w, http.StatusCreated, DKIMGenerateResponse{
		Domain:    req.Domain,
		Selector:  req.Selector,
		DNSName:   keyPair.DNSName(),
		DNSRecord: keyPair.DNSRecord(),
		KeyFile:   keyFile,
	})
}

// DKIMUploadRequest is the request for POST /api/v1/dkim/upload
type DKIMUploadRequest struct {
	Domain     string `json:"domain"`
	Selector   string `json:"selector"`
	PrivateKey string `json:"private_key"`
}

// handleDKIMUpload handles POST /api/v1/dkim/upload
func (m *ManagementServer) handleDKIMUpload(w http.ResponseWriter, r *http.Request) {
	var req DKIMUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if req.PrivateKey == "" {
		sendError(w, http.StatusBadRequest, "private_key is required")
		return
	}
	if req.Selector == "" {
		req.Selector = "default"
	}

	// Validate the private key by trying to parse it
	privateKey, err := dkim.ParsePrivateKey([]byte(req.PrivateKey))
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid private key: "+err.Error())
		return
	}

	// Create domain directory
	domainDir := filepath.Join(m.dkimKeysDir, req.Domain)
	if err := os.MkdirAll(domainDir, 0755); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to create DKIM directory")
		return
	}

	// Save private key
	keyFile := filepath.Join(domainDir, req.Selector+".key")
	if err := os.WriteFile(keyFile, []byte(req.PrivateKey), 0600); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to save DKIM key")
		return
	}

	// Create KeyPair to get DNS record
	keyPair := &dkim.KeyPair{
		PrivateKey: privateKey,
		Domain:     req.Domain,
		Selector:   req.Selector,
	}

	sendJSON(w, http.StatusCreated, DKIMGenerateResponse{
		Domain:    req.Domain,
		Selector:  req.Selector,
		DNSName:   keyPair.DNSName(),
		DNSRecord: keyPair.DNSRecord(),
		KeyFile:   keyFile,
	})
}

// DKIMInfoResponse is the response for GET /api/v1/dkim/{domain}
type DKIMInfoResponse struct {
	Domain    string   `json:"domain"`
	Enabled   bool     `json:"enabled"`
	Selector  string   `json:"selector,omitempty"`
	KeyFile   string   `json:"key_file,omitempty"`
	DNSName   string   `json:"dns_name,omitempty"`
	DNSRecord string   `json:"dns_record,omitempty"`
	Selectors []string `json:"selectors,omitempty"`
}

// handleDKIMGet handles GET /api/v1/dkim/{domain}
func (m *ManagementServer) handleDKIMGet(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	if domainName == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	response := DKIMInfoResponse{
		Domain:  domainName,
		Enabled: false,
	}

	// Check config for DKIM settings
	enabled, selector, keyFile := m.config.GetDKIMConfig(domainName)
	if enabled {
		response.Enabled = true
		response.Selector = selector
		response.KeyFile = keyFile
		response.DNSName = selector + "._domainkey." + domainName

		// Try to load key to get DNS record
		if privateKey, err := dkim.LoadPrivateKey(keyFile); err == nil {
			keyPair := &dkim.KeyPair{
				PrivateKey: privateKey,
				Domain:     domainName,
				Selector:   selector,
			}
			response.DNSRecord = keyPair.DNSRecord()
		}
	}

	// List available selectors from keys directory
	domainDir := filepath.Join(m.dkimKeysDir, domainName)
	if entries, err := os.ReadDir(domainDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".key" {
				selectorName := entry.Name()[:len(entry.Name())-4]
				response.Selectors = append(response.Selectors, selectorName)
			}
		}
	}

	sendJSON(w, http.StatusOK, response)
}

// DKIMVerifyResponse is the response for GET /api/v1/dkim/{domain}/verify
type DKIMVerifyResponse struct {
	Domain   string `json:"domain"`
	Selector string `json:"selector"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
	DNSName  string `json:"dns_name"`
}

// handleDKIMVerify handles GET /api/v1/dkim/{domain}/verify
func (m *ManagementServer) handleDKIMVerify(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	if domainName == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	selector := r.URL.Query().Get("selector")
	if selector == "" {
		// Try to get from config
		enabled, configSelector, _ := m.config.GetDKIMConfig(domainName)
		if enabled {
			selector = configSelector
		} else {
			selector = "default"
		}
	}

	response := DKIMVerifyResponse{
		Domain:   domainName,
		Selector: selector,
		DNSName:  selector + "._domainkey." + domainName,
		Valid:    false,
	}

	// Check if key file exists
	enabled, _, keyFile := m.config.GetDKIMConfig(domainName)
	if !enabled {
		keyFile = filepath.Join(m.dkimKeysDir, domainName, selector+".key")
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		response.Error = "DKIM key not found"
		sendJSON(w, http.StatusOK, response)
		return
	}

	// Load and validate the key
	if _, err := dkim.LoadPrivateKey(keyFile); err != nil {
		response.Error = "Invalid DKIM key: " + err.Error()
		sendJSON(w, http.StatusOK, response)
		return
	}

	response.Valid = true
	sendJSON(w, http.StatusOK, response)
}

// handleDKIMDelete handles DELETE /api/v1/dkim/{domain}/{selector}
func (m *ManagementServer) handleDKIMDelete(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	selector := chi.URLParam(r, "selector")

	if domainName == "" || selector == "" {
		sendError(w, http.StatusBadRequest, "domain and selector are required")
		return
	}

	keyFile := filepath.Join(m.dkimKeysDir, domainName, selector+".key")
	if err := os.Remove(keyFile); err != nil {
		if os.IsNotExist(err) {
			sendError(w, http.StatusNotFound, "DKIM key not found")
			return
		}
		sendError(w, http.StatusInternalServerError, "Failed to delete DKIM key")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TLS Handlers

// TLSCertificateInfo represents TLS certificate information
type TLSCertificateInfo struct {
	Domain   string `json:"domain"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	ACME     bool   `json:"acme"`
}

// TLSListResponse is the response for GET /api/v1/tls/certificates
type TLSListResponse struct {
	Certificates []TLSCertificateInfo `json:"certificates"`
	ACMEEnabled  bool                 `json:"acme_enabled"`
	ACMEDomains  []string             `json:"acme_domains,omitempty"`
}

// handleTLSList handles GET /api/v1/tls/certificates
func (m *ManagementServer) handleTLSList(w http.ResponseWriter, r *http.Request) {
	response := TLSListResponse{
		Certificates: []TLSCertificateInfo{},
		ACMEEnabled:  m.config.SMTP.TLS.ACME.Enabled,
		ACMEDomains:  m.config.SMTP.TLS.ACME.Domains,
	}

	// Add main TLS certificate
	if m.config.SMTP.TLS.CertFile != "" {
		response.Certificates = append(response.Certificates, TLSCertificateInfo{
			Domain:   m.config.SMTP.Domain,
			CertFile: m.config.SMTP.TLS.CertFile,
			KeyFile:  m.config.SMTP.TLS.KeyFile,
			ACME:     false,
		})
	}

	// Add domain-specific certificates
	for domain, dc := range m.config.Domains {
		if dc.TLS != nil && dc.TLS.CertFile != "" {
			response.Certificates = append(response.Certificates, TLSCertificateInfo{
				Domain:   domain,
				CertFile: dc.TLS.CertFile,
				KeyFile:  dc.TLS.KeyFile,
				ACME:     false,
			})
		}
	}

	// List certificates from directory
	if entries, err := os.ReadDir(m.tlsCertsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				domain := entry.Name()
				certFile := filepath.Join(m.tlsCertsDir, domain, "cert.pem")
				keyFile := filepath.Join(m.tlsCertsDir, domain, "key.pem")
				if _, err := os.Stat(certFile); err == nil {
					response.Certificates = append(response.Certificates, TLSCertificateInfo{
						Domain:   domain,
						CertFile: certFile,
						KeyFile:  keyFile,
						ACME:     false,
					})
				}
			}
		}
	}

	sendJSON(w, http.StatusOK, response)
}

// TLSUploadRequest is the request for POST /api/v1/tls/certificates
type TLSUploadRequest struct {
	Domain      string `json:"domain"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
}

// handleTLSUpload handles POST /api/v1/tls/certificates
func (m *ManagementServer) handleTLSUpload(w http.ResponseWriter, r *http.Request) {
	var req TLSUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain == "" || req.Certificate == "" || req.PrivateKey == "" {
		sendError(w, http.StatusBadRequest, "domain, certificate, and private_key are required")
		return
	}

	// Create domain directory
	domainDir := filepath.Join(m.tlsCertsDir, req.Domain)
	if err := os.MkdirAll(domainDir, 0755); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to create certificate directory")
		return
	}

	// Save certificate
	certFile := filepath.Join(domainDir, "cert.pem")
	if err := os.WriteFile(certFile, []byte(req.Certificate), 0644); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to save certificate")
		return
	}

	// Save private key
	keyFile := filepath.Join(domainDir, "key.pem")
	if err := os.WriteFile(keyFile, []byte(req.PrivateKey), 0600); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to save private key")
		return
	}

	sendJSON(w, http.StatusCreated, TLSCertificateInfo{
		Domain:   req.Domain,
		CertFile: certFile,
		KeyFile:  keyFile,
		ACME:     false,
	})
}

// handleTLSLetsEncrypt handles POST /api/v1/tls/letsencrypt/{domain}
func (m *ManagementServer) handleTLSLetsEncrypt(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	if domainName == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	if !m.config.SMTP.TLS.ACME.Enabled {
		sendError(w, http.StatusBadRequest, "ACME (Let's Encrypt) is not enabled in configuration")
		return
	}

	// Check if domain is in the allowed list
	found := false
	for _, d := range m.config.SMTP.TLS.ACME.Domains {
		if d == domainName {
			found = true
			break
		}
	}

	if !found {
		sendError(w, http.StatusBadRequest, "Domain not in ACME allowed domains list")
		return
	}

	sendJSON(w, http.StatusAccepted, map[string]string{
		"status":  "pending",
		"message": "Certificate will be obtained automatically on first TLS connection",
		"domain":  domainName,
	})
}

// Domains Handlers

// DomainResponse represents a domain configuration
type DomainResponse struct {
	Domain      string                        `json:"domain"`
	DKIM        *config.DomainDKIMConfig      `json:"dkim,omitempty"`
	TLS         *config.DomainTLSConfig       `json:"tls,omitempty"`
	RateLimit   *config.DomainRateLimitConfig `json:"rate_limit,omitempty"`
	Mode        string                        `json:"mode,omitempty"`
	DefaultFrom string                        `json:"default_from,omitempty"`
	RedirectTo  []string                      `json:"redirect_to,omitempty"`
	BCCTo       []string                      `json:"bcc_to,omitempty"`
}

// DomainsListResponse is the response for GET /api/v1/domains
type DomainsListResponse struct {
	Domains []DomainResponse `json:"domains"`
}

// handleDomainsList handles GET /api/v1/domains
func (m *ManagementServer) handleDomainsList(w http.ResponseWriter, r *http.Request) {
	var domains []string
	if m.domainManager != nil {
		domains = m.domainManager.ListDomains()
	} else {
		// Fallback to config if domain manager is not available
		domains = m.config.GetAllDomains()
	}

	response := DomainsListResponse{
		Domains: make([]DomainResponse, 0, len(domains)),
	}

	for _, d := range domains {
		dr := DomainResponse{Domain: d}
		if dc := m.config.GetDomainConfig(d); dc != nil {
			dr.DKIM = dc.DKIM
			dr.TLS = dc.TLS
			dr.RateLimit = dc.RateLimit
			dr.Mode = dc.Mode
			dr.DefaultFrom = dc.DefaultFrom
			dr.RedirectTo = dc.RedirectTo
			dr.BCCTo = dc.BCCTo
		}
		response.Domains = append(response.Domains, dr)
	}

	sendJSON(w, http.StatusOK, response)
}

// DomainCreateRequest is the request for POST /api/v1/domains
type DomainCreateRequest struct {
	Domain      string                        `json:"domain"`
	DKIM        *config.DomainDKIMConfig      `json:"dkim,omitempty"`
	TLS         *config.DomainTLSConfig       `json:"tls,omitempty"`
	RateLimit   *config.DomainRateLimitConfig `json:"rate_limit,omitempty"`
	Mode        string                        `json:"mode,omitempty"`
	DefaultFrom string                        `json:"default_from,omitempty"`
	RedirectTo  []string                      `json:"redirect_to,omitempty"`
	BCCTo       []string                      `json:"bcc_to,omitempty"`
}

// handleDomainsCreate handles POST /api/v1/domains
func (m *ManagementServer) handleDomainsCreate(w http.ResponseWriter, r *http.Request) {
	var req DomainCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	// Check if domain already exists
	if m.config.GetDomainConfig(req.Domain) != nil {
		sendError(w, http.StatusConflict, "Domain already exists")
		return
	}

	// Add domain to config (in memory only - need config persistence for production)
	if m.config.Domains == nil {
		m.config.Domains = make(map[string]config.DomainConfig)
	}

	m.config.Domains[req.Domain] = config.DomainConfig{
		DKIM:        req.DKIM,
		TLS:         req.TLS,
		RateLimit:   req.RateLimit,
		Mode:        req.Mode,
		DefaultFrom: req.DefaultFrom,
		RedirectTo:  req.RedirectTo,
		BCCTo:       req.BCCTo,
	}

	sendJSON(w, http.StatusCreated, DomainResponse{
		Domain:      req.Domain,
		DKIM:        req.DKIM,
		TLS:         req.TLS,
		RateLimit:   req.RateLimit,
		Mode:        req.Mode,
		DefaultFrom: req.DefaultFrom,
		RedirectTo:  req.RedirectTo,
		BCCTo:       req.BCCTo,
	})
}

// handleDomainsGet handles GET /api/v1/domains/{domain}
func (m *ManagementServer) handleDomainsGet(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	if domainName == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	dc := m.config.GetDomainConfig(domainName)
	if dc == nil {
		sendError(w, http.StatusNotFound, "Domain not found")
		return
	}

	sendJSON(w, http.StatusOK, DomainResponse{
		Domain:      domainName,
		DKIM:        dc.DKIM,
		TLS:         dc.TLS,
		RateLimit:   dc.RateLimit,
		Mode:        dc.Mode,
		DefaultFrom: dc.DefaultFrom,
		RedirectTo:  dc.RedirectTo,
		BCCTo:       dc.BCCTo,
	})
}

// handleDomainsUpdate handles PUT /api/v1/domains/{domain}
func (m *ManagementServer) handleDomainsUpdate(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	if domainName == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	var req DomainCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Check if domain exists in explicit config
	if m.config.Domains == nil {
		m.config.Domains = make(map[string]config.DomainConfig)
	}

	// Update or create domain config
	m.config.Domains[domainName] = config.DomainConfig{
		DKIM:        req.DKIM,
		TLS:         req.TLS,
		RateLimit:   req.RateLimit,
		Mode:        req.Mode,
		DefaultFrom: req.DefaultFrom,
		RedirectTo:  req.RedirectTo,
		BCCTo:       req.BCCTo,
	}

	sendJSON(w, http.StatusOK, DomainResponse{
		Domain:      domainName,
		DKIM:        req.DKIM,
		TLS:         req.TLS,
		RateLimit:   req.RateLimit,
		Mode:        req.Mode,
		DefaultFrom: req.DefaultFrom,
		RedirectTo:  req.RedirectTo,
		BCCTo:       req.BCCTo,
	})
}

// handleDomainsDelete handles DELETE /api/v1/domains/{domain}
func (m *ManagementServer) handleDomainsDelete(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	if domainName == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	// Cannot delete the main SMTP domain
	if domainName == m.config.SMTP.Domain {
		sendError(w, http.StatusBadRequest, "Cannot delete main SMTP domain")
		return
	}

	if m.config.Domains == nil {
		sendError(w, http.StatusNotFound, "Domain not found")
		return
	}

	if _, exists := m.config.Domains[domainName]; !exists {
		sendError(w, http.StatusNotFound, "Domain not found")
		return
	}

	delete(m.config.Domains, domainName)
	w.WriteHeader(http.StatusNoContent)
}

// Rate Limits Handlers

// RateLimitsResponse is the response for GET /api/v1/ratelimits
type RateLimitsResponse struct {
	Enabled       bool                 `json:"enabled"`
	Global        *config.LimitValues  `json:"global,omitempty"`
	DefaultDomain *config.LimitValues  `json:"default_domain,omitempty"`
	DefaultSender *config.LimitValues  `json:"default_sender,omitempty"`
	DefaultIP     *config.LimitValues  `json:"default_ip,omitempty"`
	DefaultAPIKey *config.LimitValues  `json:"default_api_key,omitempty"`
	Domains       map[string]*DomainRL `json:"domains,omitempty"`
}

// DomainRL represents rate limits for a domain
type DomainRL struct {
	MessagesPerHour      int `json:"messages_per_hour"`
	MessagesPerDay       int `json:"messages_per_day"`
	RecipientsPerMessage int `json:"recipients_per_message"`
}

// handleRateLimitsGet handles GET /api/v1/ratelimits
func (m *ManagementServer) handleRateLimitsGet(w http.ResponseWriter, r *http.Request) {
	response := RateLimitsResponse{
		Enabled:       m.config.RateLimit.Enabled,
		Global:        m.config.RateLimit.Global,
		DefaultDomain: m.config.RateLimit.DefaultDomain,
		DefaultSender: m.config.RateLimit.DefaultSender,
		DefaultIP:     m.config.RateLimit.DefaultIP,
		DefaultAPIKey: m.config.RateLimit.DefaultAPIKey,
		Domains:       make(map[string]*DomainRL),
	}

	// Add domain-specific rate limits
	for domain, dc := range m.config.Domains {
		if dc.RateLimit != nil {
			response.Domains[domain] = &DomainRL{
				MessagesPerHour:      dc.RateLimit.MessagesPerHour,
				MessagesPerDay:       dc.RateLimit.MessagesPerDay,
				RecipientsPerMessage: dc.RateLimit.RecipientsPerMessage,
			}
		}
	}

	sendJSON(w, http.StatusOK, response)
}

// RateLimitStatsResponse is the response for GET /api/v1/ratelimits/{level}/{key}
type RateLimitStatsResponse struct {
	Level       string `json:"level"`
	Key         string `json:"key"`
	HourlyCount int    `json:"hourly_count"`
	DailyCount  int    `json:"daily_count"`
	HourlyLimit int    `json:"hourly_limit"`
	DailyLimit  int    `json:"daily_limit"`
}

// handleRateLimitStats handles GET /api/v1/ratelimits/{level}/{key}
func (m *ManagementServer) handleRateLimitStats(w http.ResponseWriter, r *http.Request) {
	level := chi.URLParam(r, "level")
	key := chi.URLParam(r, "key")

	if level == "" || key == "" {
		sendError(w, http.StatusBadRequest, "level and key are required")
		return
	}

	if m.rateLimiter == nil {
		sendError(w, http.StatusServiceUnavailable, "Rate limiting is not enabled")
		return
	}

	stats, err := m.rateLimiter.GetStats(r.Context(), ratelimit.Level(level), key)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to get rate limit stats")
		return
	}

	response := RateLimitStatsResponse{
		Level:       level,
		Key:         key,
		HourlyCount: stats.HourlyCount,
		DailyCount:  stats.DailyCount,
	}

	// Get configured limits
	switch ratelimit.Level(level) {
	case ratelimit.LevelGlobal:
		if m.config.RateLimit.Global != nil {
			response.HourlyLimit = m.config.RateLimit.Global.MessagesPerHour
			response.DailyLimit = m.config.RateLimit.Global.MessagesPerDay
		}
	case ratelimit.LevelDomain:
		if dc := m.config.GetDomainConfig(key); dc != nil && dc.RateLimit != nil {
			response.HourlyLimit = dc.RateLimit.MessagesPerHour
			response.DailyLimit = dc.RateLimit.MessagesPerDay
		} else if m.config.RateLimit.DefaultDomain != nil {
			response.HourlyLimit = m.config.RateLimit.DefaultDomain.MessagesPerHour
			response.DailyLimit = m.config.RateLimit.DefaultDomain.MessagesPerDay
		}
	case ratelimit.LevelSender:
		if m.config.RateLimit.DefaultSender != nil {
			response.HourlyLimit = m.config.RateLimit.DefaultSender.MessagesPerHour
			response.DailyLimit = m.config.RateLimit.DefaultSender.MessagesPerDay
		}
	case ratelimit.LevelIP:
		if m.config.RateLimit.DefaultIP != nil {
			response.HourlyLimit = m.config.RateLimit.DefaultIP.MessagesPerHour
			response.DailyLimit = m.config.RateLimit.DefaultIP.MessagesPerDay
		}
	case ratelimit.LevelAPIKey:
		if m.config.RateLimit.DefaultAPIKey != nil {
			response.HourlyLimit = m.config.RateLimit.DefaultAPIKey.MessagesPerHour
			response.DailyLimit = m.config.RateLimit.DefaultAPIKey.MessagesPerDay
		}
	}

	sendJSON(w, http.StatusOK, response)
}

// RateLimitUpdateRequest is the request for PUT /api/v1/ratelimits/{domain}
type RateLimitUpdateRequest struct {
	MessagesPerHour      int `json:"messages_per_hour"`
	MessagesPerDay       int `json:"messages_per_day"`
	RecipientsPerMessage int `json:"recipients_per_message"`
}

// handleRateLimitsUpdate handles PUT /api/v1/ratelimits/{domain}
func (m *ManagementServer) handleRateLimitsUpdate(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	if domainName == "" {
		sendError(w, http.StatusBadRequest, "domain is required")
		return
	}

	var req RateLimitUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Update config
	if m.config.Domains == nil {
		m.config.Domains = make(map[string]config.DomainConfig)
	}

	dc := m.config.Domains[domainName]
	dc.RateLimit = &config.DomainRateLimitConfig{
		MessagesPerHour:      req.MessagesPerHour,
		MessagesPerDay:       req.MessagesPerDay,
		RecipientsPerMessage: req.RecipientsPerMessage,
	}
	m.config.Domains[domainName] = dc

	sendJSON(w, http.StatusOK, DomainRL{
		MessagesPerHour:      req.MessagesPerHour,
		MessagesPerDay:       req.MessagesPerDay,
		RecipientsPerMessage: req.RecipientsPerMessage,
	})
}

// Helper functions

func sendJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{"error": message})
}
