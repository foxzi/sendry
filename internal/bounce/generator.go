package bounce

import (
	"bytes"
	"fmt"
	"net/mail"
	"text/template"
	"time"

	"github.com/foxzi/sendry/internal/queue"
)

// Generator generates bounce (DSN) messages
type Generator struct {
	hostname     string
	postmaster   string
	reportingMTA string
}

// NewGenerator creates a new bounce generator
func NewGenerator(hostname string) *Generator {
	return &Generator{
		hostname:     hostname,
		postmaster:   "postmaster@" + hostname,
		reportingMTA: hostname,
	}
}

// GenerateDSN generates a Delivery Status Notification (bounce) message
// per RFC 3464 (DSN format) and RFC 6522 (multipart/report)
func (g *Generator) GenerateDSN(msg *queue.Message, errorMsg string, permanent bool) ([]byte, error) {
	// Parse original message to get subject
	originalSubject := extractSubject(msg.Data)

	data := dsnData{
		Hostname:        g.hostname,
		Postmaster:      g.postmaster,
		ReportingMTA:    g.reportingMTA,
		Date:            time.Now().Format(time.RFC1123Z),
		MessageID:       fmt.Sprintf("<%s.dsn@%s>", msg.ID, g.hostname),
		OriginalFrom:    msg.From,
		Recipients:      msg.To,
		OriginalSubject: originalSubject,
		ErrorMessage:    errorMsg,
		OriginalID:      msg.ID,
		Action:          "failed",
		Status:          "5.0.0",
		DiagnosticCode:  errorMsg,
		Boundary:        fmt.Sprintf("==Boundary_%s==", msg.ID),
	}

	if permanent {
		data.Action = "failed"
		data.Status = "5.0.0" // Permanent failure
	} else {
		data.Action = "delayed"
		data.Status = "4.0.0" // Temporary failure
	}

	var buf bytes.Buffer
	if err := dsnTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to generate DSN: %w", err)
	}

	return buf.Bytes(), nil
}

type dsnData struct {
	Hostname        string
	Postmaster      string
	ReportingMTA    string
	Date            string
	MessageID       string
	OriginalFrom    string
	Recipients      []string
	OriginalSubject string
	ErrorMessage    string
	OriginalID      string
	Action          string
	Status          string
	DiagnosticCode  string
	Boundary        string
}

var dsnTemplate = template.Must(template.New("dsn").Parse(`From: Mail Delivery System <{{.Postmaster}}>
To: <{{.OriginalFrom}}>
Subject: Delivery Status Notification (Failure)
Date: {{.Date}}
Message-ID: {{.MessageID}}
MIME-Version: 1.0
Content-Type: multipart/report; report-type=delivery-status; boundary="{{.Boundary}}"
Auto-Submitted: auto-replied

This is a MIME-encapsulated message.

--{{.Boundary}}
Content-Type: text/plain; charset=utf-8

This is the mail delivery system at {{.Hostname}}.

I'm sorry to inform you that your message could not be delivered to one or more recipients.

For further assistance, please contact <{{.Postmaster}}>.

--- Original message information ---

Subject: {{.OriginalSubject}}
Recipients:{{range .Recipients}}
  - {{.}}{{end}}

--- Error details ---

{{.ErrorMessage}}

--{{.Boundary}}
Content-Type: message/delivery-status

Reporting-MTA: dns; {{.ReportingMTA}}
Arrival-Date: {{.Date}}
{{range .Recipients}}
Final-Recipient: rfc822; {{.}}
Action: {{$.Action}}
Status: {{$.Status}}
Diagnostic-Code: smtp; {{$.DiagnosticCode}}
{{end}}
--{{.Boundary}}--
`))

// extractSubject extracts Subject header from raw email data
func extractSubject(data []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return "(unknown)"
	}
	subject := msg.Header.Get("Subject")
	if subject == "" {
		return "(no subject)"
	}
	return subject
}
