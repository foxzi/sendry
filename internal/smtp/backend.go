package smtp

import (
	"log/slog"

	"github.com/emersion/go-smtp"
	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
)

// Backend implements smtp.Backend for go-smtp
type Backend struct {
	queue  queue.Queue
	auth   *config.AuthConfig
	logger *slog.Logger
}

// NewBackend creates a new SMTP backend
func NewBackend(q queue.Queue, auth *config.AuthConfig, logger *slog.Logger) *Backend {
	return &Backend{
		queue:  q,
		auth:   auth,
		logger: logger,
	}
}

// NewSession is called when a new SMTP connection is established
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return NewSession(b, c), nil
}
