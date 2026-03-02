package sendry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(server.URL, "test-key")
}

func TestClient_GetQueue(t *testing.T) {
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/queue" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(QueueResponse{
			Stats: &QueueStats{Pending: 3},
			Messages: []*MessageSummary{
				{ID: "msg-1", From: "a@test.com", Subject: "Test 1"},
				{ID: "msg-2", From: "b@test.com", Subject: "Test 2"},
				{ID: "msg-3", From: "c@test.com", Subject: "Test 3"},
			},
		})
	})

	resp, err := client.GetQueue(context.Background())
	if err != nil {
		t.Fatalf("GetQueue() error = %v", err)
	}
	if resp.Stats.Pending != 3 {
		t.Errorf("GetQueue() pending = %d, want 3", resp.Stats.Pending)
	}
	if len(resp.Messages) != 3 {
		t.Errorf("GetQueue() messages = %d, want 3", len(resp.Messages))
	}
}

func TestClient_DeleteFromQueue(t *testing.T) {
	var gotPath string
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != "DELETE" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteFromQueue(context.Background(), "msg-123")
	if err != nil {
		t.Fatalf("DeleteFromQueue() error = %v", err)
	}
	if gotPath != "/api/v1/queue/msg-123" {
		t.Errorf("DeleteFromQueue() path = %q, want /api/v1/queue/msg-123", gotPath)
	}
}

func TestClient_PurgeQueue(t *testing.T) {
	var deletedIDs []string
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/queue" && r.Method == "GET" {
			json.NewEncoder(w).Encode(QueueResponse{
				Stats: &QueueStats{Pending: 2},
				Messages: []*MessageSummary{
					{ID: "msg-1"},
					{ID: "msg-2"},
				},
			})
			return
		}
		if r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/v1/queue/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/queue/")
			deletedIDs = append(deletedIDs, id)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	deleted, err := client.PurgeQueue(context.Background())
	if err != nil {
		t.Fatalf("PurgeQueue() error = %v", err)
	}
	if deleted != 2 {
		t.Errorf("PurgeQueue() deleted = %d, want 2", deleted)
	}
	if len(deletedIDs) != 2 {
		t.Errorf("PurgeQueue() delete requests = %d, want 2", len(deletedIDs))
	}
}

func TestClient_GetDLQ(t *testing.T) {
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/dlq" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(DLQResponse{
			Stats:    &DLQStats{Total: 1},
			Messages: []*MessageSummary{{ID: "dlq-1"}},
		})
	})

	resp, err := client.GetDLQ(context.Background())
	if err != nil {
		t.Fatalf("GetDLQ() error = %v", err)
	}
	if resp.Stats.Total != 1 {
		t.Errorf("GetDLQ() total = %d, want 1", resp.Stats.Total)
	}
}

func TestClient_RetryDLQ(t *testing.T) {
	var gotPath, gotMethod string
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.RetryDLQ(context.Background(), "dlq-1")
	if err != nil {
		t.Fatalf("RetryDLQ() error = %v", err)
	}
	if gotPath != "/api/v1/dlq/dlq-1/retry" {
		t.Errorf("RetryDLQ() path = %q, want /api/v1/dlq/dlq-1/retry", gotPath)
	}
	if gotMethod != "POST" {
		t.Errorf("RetryDLQ() method = %q, want POST", gotMethod)
	}
}

func TestClient_DeleteFromDLQ(t *testing.T) {
	var gotPath string
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteFromDLQ(context.Background(), "dlq-1")
	if err != nil {
		t.Fatalf("DeleteFromDLQ() error = %v", err)
	}
	if gotPath != "/api/v1/dlq/dlq-1" {
		t.Errorf("DeleteFromDLQ() path = %q, want /api/v1/dlq/dlq-1", gotPath)
	}
}

func TestClient_PurgeDLQ(t *testing.T) {
	var deletedIDs []string
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/dlq" && r.Method == "GET" {
			json.NewEncoder(w).Encode(DLQResponse{
				Stats: &DLQStats{Total: 3},
				Messages: []*MessageSummary{
					{ID: "dlq-1"},
					{ID: "dlq-2"},
					{ID: "dlq-3"},
				},
			})
			return
		}
		if r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/v1/dlq/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/dlq/")
			deletedIDs = append(deletedIDs, id)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	deleted, err := client.PurgeDLQ(context.Background())
	if err != nil {
		t.Fatalf("PurgeDLQ() error = %v", err)
	}
	if deleted != 3 {
		t.Errorf("PurgeDLQ() deleted = %d, want 3", deleted)
	}
}

func TestClient_PurgeQueue_EmptyQueue(t *testing.T) {
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(QueueResponse{
			Stats:    &QueueStats{Pending: 0},
			Messages: []*MessageSummary{},
		})
	})

	deleted, err := client.PurgeQueue(context.Background())
	if err != nil {
		t.Fatalf("PurgeQueue() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("PurgeQueue() deleted = %d, want 0", deleted)
	}
}

func TestClient_AuthHeader(t *testing.T) {
	var gotAuth string
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
	})

	_, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
}

func TestClient_APIError(t *testing.T) {
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "not found"})
	})

	_, err := client.GetQueue(context.Background())
	if err == nil {
		t.Fatal("GetQueue() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}
