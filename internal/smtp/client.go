package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/dkim"
	"github.com/foxzi/sendry/internal/dns"
	"github.com/foxzi/sendry/internal/queue"
)

// DeliveryError represents a delivery error with type information
type DeliveryError struct {
	Temporary bool
	Message   string
}

func (e *DeliveryError) Error() string {
	return e.Message
}

// Client sends emails to external MX servers
type Client struct {
	resolver   *dns.Resolver
	timeout    time.Duration
	hostname   string
	logger     *slog.Logger
	dkimSigner *dkim.Signer
}

// NewClient creates a new SMTP client
func NewClient(resolver *dns.Resolver, hostname string, timeout time.Duration, logger *slog.Logger) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		resolver: resolver,
		timeout:  timeout,
		hostname: hostname,
		logger:   logger,
	}
}

// SetDKIMSigner sets the DKIM signer for outgoing messages
func (c *Client) SetDKIMSigner(signer *dkim.Signer) {
	c.dkimSigner = signer
}

// Send sends a message to all recipients
func (c *Client) Send(ctx context.Context, msg *queue.Message) error {
	// Group recipients by domain
	byDomain := make(map[string][]string)
	for _, to := range msg.To {
		domain := dns.ExtractDomain(to)
		if domain == "" {
			continue
		}
		byDomain[domain] = append(byDomain[domain], to)
	}

	var lastErr error
	var permanentErr bool

	for domain, recipients := range byDomain {
		err := c.sendToDomain(ctx, domain, msg.From, recipients, msg.Data)
		if err != nil {
			lastErr = err
			if de, ok := err.(*DeliveryError); ok && !de.Temporary {
				permanentErr = true
			}
		}
	}

	if lastErr != nil {
		if permanentErr {
			return &DeliveryError{
				Temporary: false,
				Message:   lastErr.Error(),
			}
		}
		return &DeliveryError{
			Temporary: true,
			Message:   lastErr.Error(),
		}
	}

	return nil
}

// sendToDomain sends to all recipients in a single domain
func (c *Client) sendToDomain(ctx context.Context, domain string, from string, to []string, data []byte) error {
	// Lookup MX records
	mxRecords, err := c.resolver.LookupMX(ctx, domain)
	if err != nil {
		return &DeliveryError{
			Temporary: true,
			Message:   fmt.Sprintf("MX lookup failed for %s: %v", domain, err),
		}
	}

	// Try each MX host in order of priority
	var lastErr error
	for _, mx := range mxRecords {
		err := c.sendToMX(ctx, mx.Host, from, to, data)
		if err == nil {
			return nil
		}

		c.logger.Warn("delivery to MX failed",
			"mx", mx.Host,
			"domain", domain,
			"error", err,
		)
		lastErr = err

		// If permanent error, don't try other MX
		if de, ok := err.(*DeliveryError); ok && !de.Temporary {
			return de
		}
	}

	if lastErr != nil {
		return lastErr
	}

	return &DeliveryError{
		Temporary: true,
		Message:   fmt.Sprintf("no MX hosts available for %s", domain),
	}
}

// sendToMX sends to a specific MX host
func (c *Client) sendToMX(ctx context.Context, mx string, from string, to []string, data []byte) error {
	addr := net.JoinHostPort(mx, "25")

	// Create connection with timeout
	dialer := &net.Dialer{
		Timeout: c.timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return &DeliveryError{
			Temporary: true,
			Message:   fmt.Sprintf("connection failed to %s: %v", addr, err),
		}
	}
	defer conn.Close()

	// Set deadline
	deadline, ok := ctx.Deadline()
	if ok {
		conn.SetDeadline(deadline)
	} else {
		conn.SetDeadline(time.Now().Add(c.timeout))
	}

	// Create SMTP client
	client, err := smtp.NewClient(conn, mx)
	if err != nil {
		return &DeliveryError{
			Temporary: true,
			Message:   fmt.Sprintf("SMTP client creation failed: %v", err),
		}
	}
	defer client.Close()

	// Send HELO
	if err := client.Hello(c.hostname); err != nil {
		return c.categorizeError(err, "HELO")
	}

	// Try STARTTLS (opportunistic)
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: mx,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			c.logger.Warn("STARTTLS failed, continuing without encryption",
				"mx", mx,
				"error", err,
			)
		} else {
			c.logger.Debug("STARTTLS successful", "mx", mx)
		}
	}

	// Sign message with DKIM if signer is configured
	messageData := data
	if c.dkimSigner != nil {
		signed, err := c.dkimSigner.Sign(data)
		if err != nil {
			c.logger.Warn("DKIM signing failed, sending unsigned",
				"error", err,
			)
		} else {
			messageData = signed
		}
	}

	// Send MAIL FROM
	if err := client.Mail(from); err != nil {
		return c.categorizeError(err, "MAIL FROM")
	}

	// Send RCPT TO for each recipient
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return c.categorizeError(err, fmt.Sprintf("RCPT TO %s", recipient))
		}
	}

	// Send DATA
	wc, err := client.Data()
	if err != nil {
		return c.categorizeError(err, "DATA")
	}

	_, err = bytes.NewReader(messageData).WriteTo(wc)
	if err != nil {
		wc.Close()
		return &DeliveryError{
			Temporary: true,
			Message:   fmt.Sprintf("failed to write message data: %v", err),
		}
	}

	if err := wc.Close(); err != nil {
		return c.categorizeError(err, "DATA close")
	}

	// Quit
	client.Quit()

	c.logger.Info("message delivered",
		"mx", mx,
		"from", from,
		"to", to,
	)

	return nil
}

// categorizeError determines if an SMTP error is temporary or permanent
func (c *Client) categorizeError(err error, stage string) *DeliveryError {
	msg := fmt.Sprintf("%s failed: %v", stage, err)

	// Check for common permanent error codes (5xx)
	errStr := err.Error()
	if strings.Contains(errStr, "550") ||
		strings.Contains(errStr, "551") ||
		strings.Contains(errStr, "552") ||
		strings.Contains(errStr, "553") ||
		strings.Contains(errStr, "554") {
		return &DeliveryError{
			Temporary: false,
			Message:   msg,
		}
	}

	// Assume temporary by default
	return &DeliveryError{
		Temporary: true,
		Message:   msg,
	}
}

// IsTemporaryError checks if the error is temporary
func IsTemporaryError(err error) bool {
	var de *DeliveryError
	if errors.As(err, &de) {
		return de.Temporary
	}
	return true // Assume temporary if unknown
}
