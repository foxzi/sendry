package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
)

// mockQueue implements queue.Queue for testing
type mockQueue struct {
	messages map[string]*queue.Message
}

func newMockQueue() *mockQueue {
	return &mockQueue{messages: make(map[string]*queue.Message)}
}

func (m *mockQueue) Enqueue(ctx context.Context, msg *queue.Message) error {
	m.messages[msg.ID] = msg
	return nil
}

func (m *mockQueue) Dequeue(ctx context.Context) (*queue.Message, error) {
	return nil, nil
}

func (m *mockQueue) Update(ctx context.Context, msg *queue.Message) error {
	m.messages[msg.ID] = msg
	return nil
}

func (m *mockQueue) Get(ctx context.Context, id string) (*queue.Message, error) {
	return m.messages[id], nil
}

func (m *mockQueue) List(ctx context.Context, filter queue.ListFilter) ([]*queue.Message, error) {
	var result []*queue.Message
	for _, msg := range m.messages {
		if filter.Status != "" && msg.Status != filter.Status {
			continue
		}
		result = append(result, msg)
		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}
	return result, nil
}

func (m *mockQueue) Delete(ctx context.Context, id string) error {
	delete(m.messages, id)
	return nil
}

func (m *mockQueue) Stats(ctx context.Context) (*queue.QueueStats, error) {
	stats := &queue.QueueStats{Total: int64(len(m.messages))}
	for _, msg := range m.messages {
		switch msg.Status {
		case queue.StatusPending:
			stats.Pending++
		case queue.StatusSending:
			stats.Sending++
		case queue.StatusDelivered:
			stats.Delivered++
		case queue.StatusFailed:
			stats.Failed++
		case queue.StatusDeferred:
			stats.Deferred++
		}
	}
	return stats, nil
}

func (m *mockQueue) Close() error {
	return nil
}

func setupTestServer(apiKey string) (*Server, *mockQueue) {
	q := newMockQueue()
	cfg := &config.APIConfig{
		ListenAddr: ":8080",
		APIKey:     apiKey,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(q, cfg, logger)
	return server, q
}

func TestHealthEndpoint(t *testing.T) {
	server, _ := setupTestServer("")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Status = %q, want %q", resp.Status, "ok")
	}
}

func TestSendEndpoint(t *testing.T) {
	server, q := setupTestServer("test-api-key")

	body := `{
		"from": "sender@example.com",
		"to": ["recipient@example.com"],
		"subject": "Test",
		"body": "Hello"
	}`

	req := httptest.NewRequest("POST", "/api/v1/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Status = %d, want %d. Body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.ID == "" {
		t.Error("Response ID should not be empty")
	}
	if resp.Status != "pending" {
		t.Errorf("Status = %q, want %q", resp.Status, "pending")
	}

	// Verify message was queued
	if len(q.messages) != 1 {
		t.Errorf("Queue has %d messages, want 1", len(q.messages))
	}
}

func TestSendEndpointValidation(t *testing.T) {
	server, _ := setupTestServer("test-api-key")

	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing from", `{"to":["a@b.com"],"subject":"Test"}`, http.StatusBadRequest},
		{"missing to", `{"from":"a@b.com","subject":"Test"}`, http.StatusBadRequest},
		{"empty to", `{"from":"a@b.com","to":[],"subject":"Test"}`, http.StatusBadRequest},
		{"missing content", `{"from":"a@b.com","to":["b@c.com"]}`, http.StatusBadRequest},
		{"invalid json", `{invalid}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/send", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-api-key")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.want {
				t.Errorf("Status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	server, _ := setupTestServer("secret-key")

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"no auth", "", http.StatusUnauthorized},
		{"wrong key", "Bearer wrong-key", http.StatusUnauthorized},
		{"correct key", "Bearer secret-key", http.StatusAccepted},
		{"x-api-key header", "secret-key", http.StatusAccepted},
	}

	body := `{"from":"a@b.com","to":["b@c.com"],"subject":"Test","body":"Hi"}`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/send", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.header != "" {
				if tt.name == "x-api-key header" {
					req.Header.Set("X-API-Key", tt.header)
				} else {
					req.Header.Set("Authorization", tt.header)
				}
			}
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.want {
				t.Errorf("Status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestAuthMiddlewareNoKeyConfigured(t *testing.T) {
	server, _ := setupTestServer("") // No API key configured

	body := `{"from":"a@b.com","to":["b@c.com"],"subject":"Test","body":"Hi"}`
	req := httptest.NewRequest("POST", "/api/v1/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// No auth header
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Should allow without auth when no key configured
	if w.Code != http.StatusAccepted {
		t.Errorf("Status = %d, want %d (no auth required)", w.Code, http.StatusAccepted)
	}
}

func TestStatusEndpoint(t *testing.T) {
	server, q := setupTestServer("test-key")

	// Add a message
	q.messages["test-id"] = &queue.Message{
		ID:     "test-id",
		From:   "a@b.com",
		To:     []string{"c@d.com"},
		Status: queue.StatusDelivered,
	}

	req := httptest.NewRequest("GET", "/api/v1/status/test-id", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp StatusResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.ID != "test-id" {
		t.Errorf("ID = %q, want %q", resp.ID, "test-id")
	}
	if resp.Status != "delivered" {
		t.Errorf("Status = %q, want %q", resp.Status, "delivered")
	}
}

func TestStatusEndpointNotFound(t *testing.T) {
	server, _ := setupTestServer("test-key")

	req := httptest.NewRequest("GET", "/api/v1/status/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestQueueEndpoint(t *testing.T) {
	server, q := setupTestServer("test-key")

	// Add some messages
	q.messages["1"] = &queue.Message{ID: "1", Status: queue.StatusPending}
	q.messages["2"] = &queue.Message{ID: "2", Status: queue.StatusDelivered}

	req := httptest.NewRequest("GET", "/api/v1/queue", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp QueueResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Stats.Total != 2 {
		t.Errorf("Stats.Total = %d, want 2", resp.Stats.Total)
	}
}

func TestDeleteEndpoint(t *testing.T) {
	server, q := setupTestServer("test-key")

	q.messages["to-delete"] = &queue.Message{ID: "to-delete"}

	req := httptest.NewRequest("DELETE", "/api/v1/queue/to-delete", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNoContent)
	}

	if _, exists := q.messages["to-delete"]; exists {
		t.Error("Message should be deleted")
	}
}
