package queue

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestBoltStorage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewBoltStorage(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Test Enqueue
	msg := &Message{
		ID:        "test-id-1",
		From:      "sender@test.com",
		To:        []string{"recipient@test.com"},
		Data:      []byte("test email data"),
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := storage.Enqueue(ctx, msg); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Test Get
	got, err := storage.Get(ctx, "test-id-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.ID != msg.ID {
		t.Errorf("Get().ID = %v, want %v", got.ID, msg.ID)
	}
	if got.From != msg.From {
		t.Errorf("Get().From = %v, want %v", got.From, msg.From)
	}
	if got.Status != StatusPending {
		t.Errorf("Get().Status = %v, want %v", got.Status, StatusPending)
	}

	// Test Get nonexistent
	notFound, err := storage.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if notFound != nil {
		t.Error("Get() expected nil for nonexistent message")
	}

	// Test Dequeue
	dequeued, err := storage.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}
	if dequeued.ID != msg.ID {
		t.Errorf("Dequeue().ID = %v, want %v", dequeued.ID, msg.ID)
	}
	if dequeued.Status != StatusSending {
		t.Errorf("Dequeue().Status = %v, want %v", dequeued.Status, StatusSending)
	}

	// Test Dequeue empty queue
	empty, err := storage.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if empty != nil {
		t.Error("Dequeue() expected nil for empty queue")
	}

	// Test Update
	dequeued.Status = StatusDelivered
	if err := storage.Update(ctx, dequeued); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	updated, _ := storage.Get(ctx, dequeued.ID)
	if updated.Status != StatusDelivered {
		t.Errorf("Updated status = %v, want %v", updated.Status, StatusDelivered)
	}

	// Test Stats
	stats, err := storage.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Total != 1 {
		t.Errorf("Stats().Total = %v, want 1", stats.Total)
	}
	if stats.Delivered != 1 {
		t.Errorf("Stats().Delivered = %v, want 1", stats.Delivered)
	}

	// Test Delete
	if err := storage.Delete(ctx, msg.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	deleted, _ := storage.Get(ctx, msg.ID)
	if deleted != nil {
		t.Error("Delete() message still exists")
	}
}

func TestBoltStorageDeferred(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewBoltStorage(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Enqueue a message
	msg := &Message{
		ID:        "deferred-test",
		From:      "sender@test.com",
		To:        []string{"recipient@test.com"},
		Data:      []byte("test"),
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	storage.Enqueue(ctx, msg)

	// Dequeue and defer
	dequeued, _ := storage.Dequeue(ctx)
	dequeued.Status = StatusDeferred
	dequeued.NextRetryAt = time.Now().Add(-1 * time.Second) // In the past, ready for retry
	storage.Update(ctx, dequeued)

	// Should be able to dequeue again
	retried, err := storage.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if retried == nil {
		t.Fatal("Dequeue() should return deferred message")
	}
	if retried.ID != msg.ID {
		t.Errorf("Dequeue().ID = %v, want %v", retried.ID, msg.ID)
	}
}

func TestBoltStorageList(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewBoltStorage(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Add multiple messages
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:        "msg-" + string(rune('a'+i)),
			From:      "sender@test.com",
			To:        []string{"recipient@test.com"},
			Data:      []byte("test"),
			Status:    StatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		storage.Enqueue(ctx, msg)
	}

	// List all
	all, err := storage.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 5 {
		t.Errorf("List() returned %d messages, want 5", len(all))
	}

	// List with limit
	limited, err := storage.List(ctx, ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("List(limit=2) returned %d messages, want 2", len(limited))
	}

	// List with status filter
	storage.Dequeue(ctx) // Changes one to StatusSending

	pending, err := storage.List(ctx, ListFilter{Status: StatusPending})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(pending) != 4 {
		t.Errorf("List(status=pending) returned %d messages, want 4", len(pending))
	}
}

func TestNewBoltStorageCreateDir(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "nested", "test.db")

	storage, err := NewBoltStorage(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStorage() should create directories, error = %v", err)
	}
	storage.Close()
}
