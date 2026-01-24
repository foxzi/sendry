package smtp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"

	"github.com/foxzi/sendry/internal/metrics"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/ratelimit"
)

// Session implements smtp.Session and smtp.AuthSession for go-smtp
type Session struct {
	backend    *Backend
	conn       *smtp.Conn
	from       string
	to         []string
	authUser   string
	logger     *slog.Logger
	serverType string
}

// NewSession creates a new SMTP session
func NewSession(b *Backend, c *smtp.Conn) *Session {
	s := &Session{
		backend:    b,
		conn:       c,
		logger:     b.logger.With("remote_addr", c.Conn().RemoteAddr().String()),
		serverType: b.serverType,
	}

	// Track connection metrics
	metrics.IncSMTPConnections(s.serverType)

	return s
}

// AuthMechanisms returns supported authentication mechanisms
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

// Auth handles authentication
func (s *Session) Auth(mech string) (sasl.Server, error) {
	if mech != sasl.Plain {
		return nil, errors.New("unsupported authentication mechanism")
	}

	return sasl.NewPlainServer(func(identity, username, password string) error {
		if identity != "" && identity != username {
			return errors.New("identity must be empty or match username")
		}

		// Check credentials
		if s.backend.auth == nil || s.backend.auth.Users == nil {
			return errors.New("authentication not configured")
		}

		expectedPassword, ok := s.backend.auth.Users[username]
		if !ok || expectedPassword != password {
			s.logger.Warn("authentication failed", "username", username)
			metrics.IncSMTPAuthFailed()
			return smtp.ErrAuthFailed
		}

		s.authUser = username
		s.logger.Info("authentication successful", "username", username)
		metrics.IncSMTPAuthSuccess()
		return nil
	}), nil
}

// Mail handles MAIL FROM command
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	// Check if authentication is required
	if s.backend.auth != nil && s.backend.auth.Required && s.authUser == "" {
		return &smtp.SMTPError{
			Code:    530,
			Message: "Authentication required",
		}
	}

	s.from = from
	s.logger.Debug("MAIL FROM", "from", from)
	return nil
}

// Rcpt handles RCPT TO command
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	s.logger.Debug("RCPT TO", "to", to)
	return nil
}

// Data handles DATA command
func (s *Session) Data(r io.Reader) error {
	// Check rate limits before processing
	ctx := context.Background()
	if err := s.checkRateLimits(ctx); err != nil {
		return err
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return &smtp.SMTPError{
			Code:    442,
			Message: "Failed to read message data",
		}
	}

	// Create message
	msg := &queue.Message{
		ID:        uuid.New().String(),
		From:      s.from,
		To:        s.to,
		Data:      data,
		Status:    queue.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		AuthUser:  s.authUser,
		ClientIP:  s.conn.Conn().RemoteAddr().String(),
	}

	// Enqueue message
	if err := s.backend.queue.Enqueue(ctx, msg); err != nil {
		s.logger.Error("failed to enqueue message", "error", err)
		return &smtp.SMTPError{
			Code:    451,
			Message: "Failed to queue message",
		}
	}

	s.logger.Info("message queued",
		"id", msg.ID,
		"from", s.from,
		"to", s.to,
		"size", len(data),
	)

	return nil
}

// checkRateLimits checks if the message is within rate limits
func (s *Session) checkRateLimits(ctx context.Context) error {
	req := &ratelimit.Request{
		Domain: extractDomainFromEmail(s.from),
		Sender: s.from,
		IP:     extractIP(s.conn.Conn().RemoteAddr().String()),
	}

	return s.backend.CheckRateLimit(ctx, req)
}

// extractDomainFromEmail extracts domain from email address
func extractDomainFromEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

// extractIP extracts IP from address string (removes port)
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// Reset resets the session state
func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

// Logout handles session logout
func (s *Session) Logout() error {
	s.logger.Debug("session logout")
	metrics.DecSMTPConnectionsActive()
	return nil
}
