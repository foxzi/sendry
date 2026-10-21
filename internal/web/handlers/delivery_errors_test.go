package handlers

import (
	"reflect"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/stats"
)

func TestSummarizeErrors(t *testing.T) {
	counts := []models.ErrorCount{
		{Error: "MAIL FROM failed: x509: certificate has expired", Status: "queued", Count: 1},
		{Error: "RCPT TO a failed: 550 5.1.1 User unknown", Status: "failed", Count: 2},
		{Error: "RCPT TO b failed: 550 5.1.1 No such user", Status: "failed", Count: 1},
		{Error: "RCPT TO c failed: 550 5.1.1 User unknown", Status: "queued", Count: 1},
		{Error: "something odd", Status: "sent", Count: 5},
	}

	want := []errorTypeCount{
		{Type: stats.ErrorAddressNotFound, Failed: 3, Retrying: 1},
		{Type: stats.ErrorTechnical, Retrying: 1},
	}
	if got := summarizeErrors(counts); !reflect.DeepEqual(got, want) {
		t.Errorf("summarizeErrors() = %+v, want %+v", got, want)
	}
}
