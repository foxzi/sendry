package sandbox

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/mail"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/queue"
)

// DomainModeProvider provides domain mode information
type DomainModeProvider interface {
	GetDomainMode(domain string) string
	GetRedirectAddresses(domain string) []string
	GetBCCAddresses(domain string) []string
}

// RealSender is the interface for the actual SMTP sender
type RealSender interface {
	Send(ctx context.Context, msg *queue.Message) error
}

// Sender wraps a real sender and intercepts messages based on domain mode
type Sender struct {
	realSender       RealSender
	domainProvider   DomainModeProvider
	storage          *Storage
	logger           *slog.Logger
	simulateErrors   bool
	errorProbability float64 // 0.0 to 1.0
}

// NewSender creates a new sandbox sender
func NewSender(
	realSender RealSender,
	domainProvider DomainModeProvider,
	storage *Storage,
	logger *slog.Logger,
) *Sender {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Sender{
		realSender:       realSender,
		domainProvider:   domainProvider,
		storage:          storage,
		logger:           logger,
		simulateErrors:   false,
		errorProbability: 0.1, // 10% error rate when simulation is enabled
	}
}

// SetErrorSimulation enables/disables error simulation
func (s *Sender) SetErrorSimulation(enabled bool, probability float64) {
	s.simulateErrors = enabled
	if probability > 0 && probability <= 1 {
		s.errorProbability = probability
	}
}

// Send routes the message based on domain mode
func (s *Sender) Send(ctx context.Context, msg *queue.Message) error {
	// Extract sender domain
	domain := extractDomain(msg.From)
	if domain == "" {
		// Can't determine domain, use real sender
		return s.realSender.Send(ctx, msg)
	}

	mode := "production"
	if s.domainProvider != nil {
		mode = s.domainProvider.GetDomainMode(domain)
	}

	switch mode {
	case "sandbox":
		return s.handleSandbox(ctx, msg, domain)
	case "redirect":
		return s.handleRedirect(ctx, msg, domain)
	case "bcc":
		return s.handleBCC(ctx, msg, domain)
	default:
		// Production mode - send normally
		return s.realSender.Send(ctx, msg)
	}
}

// handleSandbox stores the message instead of sending
func (s *Sender) handleSandbox(ctx context.Context, msg *queue.Message, domain string) error {
	s.logger.Info("sandbox: capturing message",
		"id", msg.ID,
		"from", msg.From,
		"to", msg.To,
		"domain", domain,
	)

	// Simulate random errors if enabled
	if s.simulateErrors && rand.Float64() < s.errorProbability {
		errorTypes := []string{
			"550 User not found",
			"451 Temporary failure",
			"452 Insufficient storage",
			"421 Service not available",
		}
		errMsg := errorTypes[rand.Intn(len(errorTypes))]

		sandboxMsg := &Message{
			ID:           msg.ID,
			From:         msg.From,
			To:           msg.To,
			Subject:      extractSubject(msg.Data),
			Data:         msg.Data,
			Domain:       domain,
			Mode:         "sandbox",
			CapturedAt:   time.Now(),
			ClientIP:     msg.ClientIP,
			SimulatedErr: errMsg,
		}

		if err := s.storage.Save(ctx, sandboxMsg); err != nil {
			s.logger.Error("sandbox: failed to save message", "error", err)
		}

		// Return simulated error
		isTemp := strings.HasPrefix(errMsg, "4")
		return &SimulatedError{
			Message:   errMsg,
			Temporary: isTemp,
		}
	}

	// Store message in sandbox
	sandboxMsg := &Message{
		ID:         msg.ID,
		From:       msg.From,
		To:         msg.To,
		Subject:    extractSubject(msg.Data),
		Data:       msg.Data,
		Domain:     domain,
		Mode:       "sandbox",
		CapturedAt: time.Now(),
		ClientIP:   msg.ClientIP,
	}

	if err := s.storage.Save(ctx, sandboxMsg); err != nil {
		return fmt.Errorf("sandbox: failed to save message: %w", err)
	}

	s.logger.Info("sandbox: message captured",
		"id", msg.ID,
		"from", msg.From,
		"to", msg.To,
	)

	return nil
}

// handleRedirect redirects the message to configured addresses
func (s *Sender) handleRedirect(ctx context.Context, msg *queue.Message, domain string) error {
	redirectTo := s.domainProvider.GetRedirectAddresses(domain)
	if len(redirectTo) == 0 {
		s.logger.Warn("redirect: no redirect addresses configured, using sandbox",
			"domain", domain,
		)
		return s.handleSandbox(ctx, msg, domain)
	}

	s.logger.Info("redirect: redirecting message",
		"id", msg.ID,
		"from", msg.From,
		"original_to", msg.To,
		"redirect_to", redirectTo,
		"domain", domain,
	)

	// Store in sandbox for audit
	sandboxMsg := &Message{
		ID:         msg.ID,
		From:       msg.From,
		To:         redirectTo,
		OriginalTo: msg.To,
		Subject:    extractSubject(msg.Data),
		Data:       msg.Data,
		Domain:     domain,
		Mode:       "redirect",
		CapturedAt: time.Now(),
		ClientIP:   msg.ClientIP,
	}

	if err := s.storage.Save(ctx, sandboxMsg); err != nil {
		s.logger.Warn("redirect: failed to save to sandbox", "error", err)
	}

	// Create modified message with redirect recipients
	redirectedMsg := &queue.Message{
		ID:        msg.ID,
		From:      msg.From,
		To:        redirectTo,
		Data:      msg.Data,
		ClientIP:  msg.ClientIP,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	return s.realSender.Send(ctx, redirectedMsg)
}

// handleBCC sends to original recipients and BCC addresses
func (s *Sender) handleBCC(ctx context.Context, msg *queue.Message, domain string) error {
	bccTo := s.domainProvider.GetBCCAddresses(domain)
	if len(bccTo) == 0 {
		s.logger.Debug("bcc: no BCC addresses configured, sending normally",
			"domain", domain,
		)
		return s.realSender.Send(ctx, msg)
	}

	s.logger.Info("bcc: sending with BCC",
		"id", msg.ID,
		"from", msg.From,
		"to", msg.To,
		"bcc", bccTo,
		"domain", domain,
	)

	// Store in sandbox for audit
	sandboxMsg := &Message{
		ID:         msg.ID,
		From:       msg.From,
		To:         append(msg.To, bccTo...),
		OriginalTo: msg.To,
		Subject:    extractSubject(msg.Data),
		Data:       msg.Data,
		Domain:     domain,
		Mode:       "bcc",
		CapturedAt: time.Now(),
		ClientIP:   msg.ClientIP,
	}

	if err := s.storage.Save(ctx, sandboxMsg); err != nil {
		s.logger.Warn("bcc: failed to save to sandbox", "error", err)
	}

	// Send to original recipients
	err := s.realSender.Send(ctx, msg)
	if err != nil {
		return err
	}

	// Send to BCC recipients
	bccMsg := &queue.Message{
		ID:        msg.ID + "-bcc",
		From:      msg.From,
		To:        bccTo,
		Data:      msg.Data,
		ClientIP:  msg.ClientIP,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	if err := s.realSender.Send(ctx, bccMsg); err != nil {
		s.logger.Warn("bcc: failed to send to BCC recipients", "error", err)
		// Don't return error - original delivery was successful
	}

	return nil
}

// SimulatedError represents a simulated delivery error
type SimulatedError struct {
	Message   string
	Temporary bool
}

func (e *SimulatedError) Error() string {
	return e.Message
}

// extractDomain extracts the domain from an email address
func extractDomain(email string) string {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		// Try simple extraction
		at := strings.LastIndex(email, "@")
		if at <= 0 || at == len(email)-1 {
			return ""
		}
		return strings.ToLower(email[at+1:])
	}
	at := strings.LastIndex(addr.Address, "@")
	if at <= 0 || at == len(addr.Address)-1 {
		return ""
	}
	return strings.ToLower(addr.Address[at+1:])
}

// extractSubject extracts the Subject header from email data
func extractSubject(data []byte) string {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			break // End of headers
		}
		if strings.HasPrefix(strings.ToLower(line), "subject:") {
			return strings.TrimSpace(line[8:])
		}
	}
	return ""
}
