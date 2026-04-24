package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendBatchEndpoint(t *testing.T) {
	server, q := setupTestServer("test-api-key")

	body := `{
		"messages": [
			{"from": "a@example.com", "to": ["x@example.com"], "subject": "s1", "body": "b1"},
			{"from": "b@example.com", "to": ["y@example.com"], "subject": "s2", "body": "b2"},
			{"from": "c@example.com", "to": ["z@example.com"], "subject": "s3", "body": "b3"}
		]
	}`

	req := httptest.NewRequest("POST", "/api/v1/send/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d. Body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp BatchSendResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Accepted != 3 || resp.Rejected != 0 {
		t.Fatalf("Accepted=%d Rejected=%d, want 3/0", resp.Accepted, resp.Rejected)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("Results len=%d, want 3", len(resp.Results))
	}
	for i, r := range resp.Results {
		if r.Index != i {
			t.Errorf("Results[%d].Index=%d", i, r.Index)
		}
		if r.ID == "" {
			t.Errorf("Results[%d].ID is empty", i)
		}
		if r.Status != "pending" {
			t.Errorf("Results[%d].Status=%q, want pending", i, r.Status)
		}
		if r.Error != "" {
			t.Errorf("Results[%d].Error=%q, want empty", i, r.Error)
		}
	}

	if len(q.messages) != 3 {
		t.Errorf("Queue has %d messages, want 3", len(q.messages))
	}
}

func TestSendBatchMixedValidation(t *testing.T) {
	server, q := setupTestServer("test-api-key")

	body := `{
		"messages": [
			{"from": "a@example.com", "to": ["x@example.com"], "subject": "ok", "body": "b"},
			{"to": ["y@example.com"], "subject": "missing from"},
			{"from": "c@example.com", "subject": "missing to"},
			{"from": "d@example.com", "to": ["z@example.com"], "subject": "ok2", "body": "b2"}
		]
	}`

	req := httptest.NewRequest("POST", "/api/v1/send/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d. Body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp BatchSendResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Accepted != 2 {
		t.Errorf("Accepted=%d, want 2", resp.Accepted)
	}
	if resp.Rejected != 2 {
		t.Errorf("Rejected=%d, want 2", resp.Rejected)
	}
	if resp.Results[0].Error != "" || resp.Results[0].ID == "" {
		t.Errorf("Results[0] should be accepted, got %+v", resp.Results[0])
	}
	if resp.Results[1].Error == "" || resp.Results[1].ID != "" {
		t.Errorf("Results[1] should be rejected, got %+v", resp.Results[1])
	}
	if resp.Results[2].Error == "" || resp.Results[2].ID != "" {
		t.Errorf("Results[2] should be rejected, got %+v", resp.Results[2])
	}
	if resp.Results[3].Error != "" || resp.Results[3].ID == "" {
		t.Errorf("Results[3] should be accepted, got %+v", resp.Results[3])
	}

	if len(q.messages) != 2 {
		t.Errorf("Queue has %d messages, want 2", len(q.messages))
	}
}

func TestSendBatchEmpty(t *testing.T) {
	server, _ := setupTestServer("test-api-key")

	body := `{"messages": []}`
	req := httptest.NewRequest("POST", "/api/v1/send/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSendBatchTooLarge(t *testing.T) {
	server, _ := setupTestServer("test-api-key")

	// Build a payload with maxBatchSize + 1 messages
	parts := make([]string, 0, maxBatchSize+1)
	msg := `{"from":"a@b.com","to":["c@d.com"],"subject":"s","body":"b"}`
	for i := 0; i <= maxBatchSize; i++ {
		parts = append(parts, msg)
	}
	body := `{"messages":[` + strings.Join(parts, ",") + `]}`

	req := httptest.NewRequest("POST", "/api/v1/send/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("Status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}
