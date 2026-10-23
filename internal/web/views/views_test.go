package views

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/stats"
)

func TestRenderJobErrors(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	job := &models.SendJob{ID: "0123456789abcdef", CampaignName: "camp", Status: "running", CreatedAt: time.Now()}
	items := []models.SendJobItem{{Email: "a@example.org", Status: "failed", Error: "RCPT TO a failed: 550 5.1.1 User unknown"}}

	var buf bytes.Buffer
	err = e.Render(&buf, "job_view", map[string]any{
		"Job":   job,
		"Stats": models.JobStats{Total: 1, Failed: 1},
		"Items": items,
		"Errors": []struct {
			Type     stats.ErrorType
			Failed   int
			Retrying int
		}{{Type: stats.ErrorAddressNotFound, Failed: 1}},
	})
	if err != nil {
		t.Fatalf("render job_view: %v", err)
	}
	if !strings.Contains(buf.String(), "Address does not exist") ||
		!strings.Contains(buf.String(), "/jobs/0123456789abcdef/items?status=failed&error_type=address_not_found") {
		t.Error("job_view has no link to the error type")
	}

	buf.Reset()
	err = e.Render(&buf, "job_items", map[string]any{
		"Job":        job,
		"Items":      items,
		"Total":      1,
		"Page":       1,
		"TotalPages": 2,
		"Status":     "failed",
		"ErrorType":  stats.ErrorAddressNotFound,
		"ErrorTypes": stats.ErrorTypes,
	})
	if err != nil {
		t.Fatalf("render job_items: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`<option value="address_not_found" data-i18n="error_type.address_not_found" selected>`,
		`>Address does not exist</span>`,
		`&error_type=address_not_found`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("job_items has no %q", want)
		}
	}
}

func TestRenderSendErrors(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	sends := []models.SendWithDetails{{Send: models.Send{
		ID: "0123456789abcdef", FromAddress: "shop@example.com", ToAddresses: `["a@example.org"]`,
		Status: "queued", ErrorMessage: "MAIL FROM failed: x509: certificate has expired", CreatedAt: time.Now(),
	}}}

	var buf bytes.Buffer
	err = e.Render(&buf, "sends_list", map[string]any{
		"Sends":      sends,
		"Stats":      &models.SendStats{Total: 1, Queued: 1},
		"Total":      1,
		"Page":       1,
		"TotalPages": 2,
		"ErrorType":  stats.ErrorTechnical,
		"ErrorTypes": stats.ErrorTypes,
		"Errors": []struct {
			Type     stats.ErrorType
			Failed   int
			Retrying int
		}{{Type: stats.ErrorTechnical, Retrying: 1}},
	})
	if err != nil {
		t.Fatalf("render sends_list: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`<a href="/sends?status=queued&error_type=technical#results">1</a>`,
		`<option value="technical" data-i18n="error_type.technical" selected>`,
		`>Temporary technical error</span>`,
		`&error_type=technical" class="btn btn-sm">Next`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sends_list has no %q", want)
		}
	}
}

func TestLayoutFooterYear(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var buf bytes.Buffer
	if err := e.Render(&buf, "sends_list", map[string]any{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := fmt.Sprintf("Sendry Web &copy; %d", time.Now().Year()); !strings.Contains(buf.String(), want) {
		t.Errorf("footer has no %q", want)
	}
}
