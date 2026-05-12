package smtp

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Encryption string

const (
	EncSSL      Encryption = "ssl"
	EncSTARTTLS Encryption = "starttls"
	EncNone     Encryption = "none"
)

type Server struct {
	Host       string
	Port       int
	Username   string
	Password   string
	Encryption Encryption
}

type Message struct {
	From     string
	FromName string
	To       []string
	Subject  string
	HTML     string
	Text     string
}

func Send(srv Server, msg Message) error {
	addr := fmt.Sprintf("%s:%d", srv.Host, srv.Port)
	if len(msg.To) == 0 {
		return fmt.Errorf("at least one recipient required")
	}

	var c *smtp.Client
	var err error
	dialer := &net.Dialer{Timeout: 30 * time.Second}

	switch srv.Encryption {
	case EncSSL, "":
		conn, derr := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: srv.Host})
		if derr != nil {
			return fmt.Errorf("tls dial: %w", derr)
		}
		c, err = smtp.NewClient(conn, srv.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("smtp client: %w", err)
		}
	case EncSTARTTLS:
		conn, derr := dialer.Dial("tcp", addr)
		if derr != nil {
			return fmt.Errorf("dial: %w", derr)
		}
		c, err = smtp.NewClient(conn, srv.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("smtp client: %w", err)
		}
		if err := c.StartTLS(&tls.Config{ServerName: srv.Host}); err != nil {
			c.Close()
			return fmt.Errorf("starttls: %w", err)
		}
	case EncNone:
		conn, derr := dialer.Dial("tcp", addr)
		if derr != nil {
			return fmt.Errorf("dial: %w", derr)
		}
		c, err = smtp.NewClient(conn, srv.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("smtp client: %w", err)
		}
	default:
		return fmt.Errorf("unsupported encryption %q", srv.Encryption)
	}
	defer c.Quit()

	if srv.Username != "" {
		auth := smtp.PlainAuth("", srv.Username, srv.Password, srv.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	if err := c.Mail(msg.From); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, to := range msg.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", to, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(buildRFC822(msg)); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

func buildRFC822(m Message) []byte {
	var b strings.Builder
	from := m.From
	if m.FromName != "" {
		from = fmt.Sprintf("=?UTF-8?B?%s?= <%s>", base64.StdEncoding.EncodeToString([]byte(m.FromName)), m.From)
	}
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(m.To, ", ") + "\r\n")
	b.WriteString("Subject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(m.Subject)) + "?=\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")

	if m.Text == "" {
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
		b.WriteString("\r\n")
		b.WriteString(m.HTML)
		return []byte(b.String())
	}

	boundary := "sendry-mp-" + randomBoundary()
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(m.Text)
	b.WriteString("\r\n--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(m.HTML)
	b.WriteString("\r\n--" + boundary + "--\r\n")
	return []byte(b.String())
}

func randomBoundary() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
