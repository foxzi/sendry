package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/sendry"
)

var (
	ErrDomainNotFound     = errors.New("domain not configured")
	ErrNoServersAvailable = errors.New("no healthy servers available for domain")
	ErrAllServersFailed   = errors.New("all servers failed to send")
	ErrTemplateNotFound   = errors.New("template not found")
	ErrInvalidRequest     = errors.New("invalid request")
)

// APISendRequest represents the API request for sending email
type APISendRequest struct {
	From         string            `json:"from"`
	To           []string          `json:"to"`
	CC           []string          `json:"cc,omitempty"`
	BCC          []string          `json:"bcc,omitempty"`
	Subject      string            `json:"subject,omitempty"`
	Body         string            `json:"body,omitempty"`
	HTML         string            `json:"html,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	TemplateID   string            `json:"template_id,omitempty"`
	TemplateName string            `json:"template_name,omitempty"`
	Data         map[string]any    `json:"data,omitempty"`
	PreferServer string            `json:"prefer_server,omitempty"`
}

// APISendResponse represents the API response
type APISendResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	ServerName string `json:"server_name"`
	ServerMsgID string `json:"server_msg_id,omitempty"`
}

// ServerInfo holds information about a server for routing
type ServerInfo struct {
	Name    string
	Client  *sendry.Client
	Weight  int
	Healthy bool
}

// EmailRouter routes emails to appropriate MTA servers
type EmailRouter struct {
	domains    *repository.DomainRepository
	templates  *repository.TemplateRepository
	sends      *repository.SendRepository
	settings   *repository.SettingsRepository
	sendry     *sendry.Manager
	cfg        *config.MultiSendConfig
	logger     *slog.Logger

	mu         sync.Mutex
	rrCounters map[string]int // Round-robin counters per domain
}

// RouterConfig contains configuration for the router
type RouterConfig struct {
	Domains    *repository.DomainRepository
	Templates  *repository.TemplateRepository
	Sends      *repository.SendRepository
	Settings   *repository.SettingsRepository
	Sendry     *sendry.Manager
	MultiSend  *config.MultiSendConfig
	Logger     *slog.Logger
}

// NewEmailRouter creates a new email router
func NewEmailRouter(cfg RouterConfig) *EmailRouter {
	return &EmailRouter{
		domains:    cfg.Domains,
		templates:  cfg.Templates,
		sends:      cfg.Sends,
		settings:   cfg.Settings,
		sendry:     cfg.Sendry,
		cfg:        cfg.MultiSend,
		logger:     cfg.Logger,
		rrCounters: make(map[string]int),
	}
}

// Send sends an email through the appropriate MTA server
func (r *EmailRouter) Send(ctx context.Context, req *APISendRequest, apiKeyID, clientIP string) (*APISendResponse, error) {
	// Validate request
	if req.From == "" {
		return nil, fmt.Errorf("%w: from is required", ErrInvalidRequest)
	}
	if len(req.To) == 0 {
		return nil, fmt.Errorf("%w: to is required", ErrInvalidRequest)
	}

	// Extract sender domain
	senderDomain := extractDomain(req.From)
	if senderDomain == "" {
		return nil, fmt.Errorf("%w: invalid from address", ErrInvalidRequest)
	}

	// Find domain in database
	domain, err := r.domains.GetByDomain(senderDomain)
	if err != nil {
		r.logger.Error("failed to lookup domain", "domain", senderDomain, "error", err)
		return nil, fmt.Errorf("domain lookup failed: %w", err)
	}
	if domain == nil {
		return nil, ErrDomainNotFound
	}

	// Get servers where domain is deployed
	servers, err := r.getDeployedServers(ctx, domain)
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, ErrNoServersAvailable
	}

	// Build send request (resolve template if needed)
	sendReq, templateID, err := r.buildSendRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// Select server
	var selectedServer *ServerInfo
	if req.PreferServer != "" {
		// Try to use preferred server
		for i := range servers {
			if servers[i].Name == req.PreferServer {
				selectedServer = &servers[i]
				break
			}
		}
	}
	if selectedServer == nil {
		selectedServer = r.selectServer(senderDomain, servers)
	}

	// Create send record
	send := &models.Send{
		APIKeyID:     apiKeyID,
		FromAddress:  req.From,
		ToAddresses:  repository.ToJSON(req.To),
		CCAddresses:  repository.ToJSON(req.CC),
		BCCAddresses: repository.ToJSON(req.BCC),
		Subject:      sendReq.Subject,
		TemplateID:   templateID,
		SenderDomain: senderDomain,
		ServerName:   selectedServer.Name,
		Status:       models.SendStatusPending,
		ClientIP:     clientIP,
	}
	if err := r.sends.Create(send); err != nil {
		r.logger.Error("failed to create send record", "error", err)
		// Continue anyway - don't fail the send because of logging
	}

	// Send with failover
	resp, actualServer, err := r.sendWithFailover(ctx, sendReq, servers, selectedServer)

	// Update send record with result
	now := time.Now()
	if err != nil {
		r.sends.UpdateStatus(send.ID, models.SendStatusFailed, err.Error(), "", nil)
		return nil, err
	}

	r.sends.UpdateStatus(send.ID, models.SendStatusSent, "", resp.ID, &now)

	r.logger.Info("email sent via API",
		"send_id", send.ID,
		"from", req.From,
		"to", req.To,
		"server", actualServer,
		"server_msg_id", resp.ID,
	)

	return &APISendResponse{
		ID:         send.ID,
		Status:     models.SendStatusSent,
		ServerName: actualServer,
		ServerMsgID: resp.ID,
	}, nil
}

// getDeployedServers returns servers where domain is deployed
func (r *EmailRouter) getDeployedServers(ctx context.Context, domain *models.Domain) ([]ServerInfo, error) {
	var servers []ServerInfo

	for _, dep := range domain.Deployments {
		// Only use "deployed" status servers
		if dep.Status != "deployed" {
			continue
		}

		// Get server client
		client, err := r.sendry.GetClient(dep.ServerName)
		if err != nil {
			r.logger.Warn("failed to get client for server", "server", dep.ServerName, "error", err)
			continue
		}

		// Get weight from config
		weight := 1
		if r.cfg != nil && r.cfg.Weights != nil {
			if w, ok := r.cfg.Weights[dep.ServerName]; ok {
				weight = w
			}
		}

		servers = append(servers, ServerInfo{
			Name:    dep.ServerName,
			Client:  client,
			Weight:  weight,
			Healthy: true, // Assume healthy, will check on send
		})
	}

	return servers, nil
}

// buildSendRequest creates a sendry.SendRequest from APISendRequest
func (r *EmailRouter) buildSendRequest(ctx context.Context, req *APISendRequest) (*sendry.SendRequest, string, error) {
	// If template is specified, resolve it
	if req.TemplateID != "" || req.TemplateName != "" {
		return r.resolveTemplate(ctx, req)
	}

	// Validate raw content
	if req.Subject == "" && req.Body == "" && req.HTML == "" {
		return nil, "", fmt.Errorf("%w: subject, body or html is required", ErrInvalidRequest)
	}

	return &sendry.SendRequest{
		From:    req.From,
		To:      req.To,
		CC:      req.CC,
		BCC:     req.BCC,
		Subject: req.Subject,
		Body:    req.Body,
		HTML:    req.HTML,
		Headers: req.Headers,
	}, "", nil
}

// resolveTemplate loads and renders a template
func (r *EmailRouter) resolveTemplate(ctx context.Context, req *APISendRequest) (*sendry.SendRequest, string, error) {
	var tmpl *models.Template
	var err error

	if req.TemplateID != "" {
		tmpl, err = r.templates.GetByID(req.TemplateID)
	} else if req.TemplateName != "" {
		tmpl, err = r.templates.GetByName(req.TemplateName)
	}

	if err != nil {
		return nil, "", fmt.Errorf("template lookup failed: %w", err)
	}
	if tmpl == nil {
		return nil, "", ErrTemplateNotFound
	}

	// Get global variables
	globalVars, _ := r.settings.GetGlobalVariablesMap()

	// Merge variables (request data takes precedence)
	data := mergeVariables(globalVars, req.Data)

	// Add built-in variables
	if len(req.To) > 0 {
		data["email"] = req.To[0]
		data["recipient_email"] = req.To[0]
	}

	// Render template
	subject := renderVars(tmpl.Subject, data)
	html := renderVars(tmpl.HTML, data)
	text := renderVars(tmpl.Text, data)

	return &sendry.SendRequest{
		From:    req.From,
		To:      req.To,
		CC:      req.CC,
		BCC:     req.BCC,
		Subject: subject,
		HTML:    html,
		Body:    text,
		Headers: req.Headers,
	}, tmpl.ID, nil
}

// selectServer chooses a server based on strategy
func (r *EmailRouter) selectServer(domain string, servers []ServerInfo) *ServerInfo {
	if len(servers) == 1 {
		return &servers[0]
	}

	strategy := "round_robin"
	if r.cfg != nil && r.cfg.Strategy != "" {
		strategy = r.cfg.Strategy
	}

	switch strategy {
	case "weighted":
		return r.weightedSelect(servers)
	case "first_healthy":
		return r.firstHealthySelect(servers)
	default:
		return r.roundRobinSelect(domain, servers)
	}
}

// roundRobinSelect selects server using round-robin
func (r *EmailRouter) roundRobinSelect(domain string, servers []ServerInfo) *ServerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	counter := r.rrCounters[domain]
	r.rrCounters[domain] = (counter + 1) % len(servers)

	return &servers[counter]
}

// weightedSelect selects server based on weights
func (r *EmailRouter) weightedSelect(servers []ServerInfo) *ServerInfo {
	totalWeight := 0
	for _, s := range servers {
		totalWeight += s.Weight
	}

	// Simple weighted selection (deterministic for now)
	r.mu.Lock()
	defer r.mu.Unlock()

	counter := r.rrCounters["_weighted"]
	r.rrCounters["_weighted"] = (counter + 1) % totalWeight

	cumulative := 0
	for i := range servers {
		cumulative += servers[i].Weight
		if counter < cumulative {
			return &servers[i]
		}
	}

	return &servers[0]
}

// firstHealthySelect returns first server (assumes all are healthy)
func (r *EmailRouter) firstHealthySelect(servers []ServerInfo) *ServerInfo {
	return &servers[0]
}

// sendWithFailover attempts to send with failover to other servers
func (r *EmailRouter) sendWithFailover(ctx context.Context, req *sendry.SendRequest, servers []ServerInfo, primary *ServerInfo) (*sendry.SendResponse, string, error) {
	failoverEnabled := r.cfg != nil && r.cfg.Failover.Enabled
	maxRetries := 1
	if failoverEnabled && r.cfg.Failover.MaxRetries > 0 {
		maxRetries = r.cfg.Failover.MaxRetries
	}

	// Try primary first
	resp, err := primary.Client.Send(ctx, req)
	if err == nil {
		return resp, primary.Name, nil
	}

	r.logger.Warn("primary server failed", "server", primary.Name, "error", err)

	if !failoverEnabled || len(servers) <= 1 {
		return nil, primary.Name, fmt.Errorf("send failed on %s: %w", primary.Name, err)
	}

	// Failover to other servers
	tried := map[string]bool{primary.Name: true}
	for attempt := 0; attempt < maxRetries && attempt < len(servers)-1; attempt++ {
		for i := range servers {
			if tried[servers[i].Name] {
				continue
			}
			tried[servers[i].Name] = true

			r.logger.Info("failover attempt", "server", servers[i].Name, "attempt", attempt+1)

			resp, err = servers[i].Client.Send(ctx, req)
			if err == nil {
				return resp, servers[i].Name, nil
			}

			r.logger.Warn("failover server failed", "server", servers[i].Name, "error", err)
			break // Only try one server per attempt
		}
	}

	return nil, primary.Name, ErrAllServersFailed
}

// extractDomain extracts domain from email address
func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		// Handle "Name <email@domain>" format
		if idx := strings.Index(email, "<"); idx >= 0 {
			email = email[idx+1:]
			if idx := strings.Index(email, ">"); idx >= 0 {
				email = email[:idx]
			}
			parts = strings.Split(email, "@")
		}
	}
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

// mergeVariables merges global and request variables
func mergeVariables(global map[string]string, data map[string]any) map[string]any {
	result := make(map[string]any)

	// Add global variables
	for k, v := range global {
		result[k] = v
	}

	// Override with request data
	for k, v := range data {
		result[k] = v
	}

	return result
}

var varPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// renderVars replaces {{variable}} placeholders with values
func renderVars(template string, data map[string]any) string {
	return varPattern.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable name
		varName := strings.TrimSpace(match[2 : len(match)-2])

		if val, ok := data[varName]; ok {
			return fmt.Sprintf("%v", val)
		}

		// Keep original if variable not found
		return match
	})
}
