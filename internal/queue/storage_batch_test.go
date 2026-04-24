package queue

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEnqueueBatch(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewBoltStorage(filepath.Join(tmpDir, "q.db"))
	if err != nil {
		t.Fatalf("NewBoltStorage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	msgs := []*Message{
		{ID: "m1", From: "a@example.com", To: []string{"x@example.com"}, Data: []byte("a"), Status: StatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "m2", From: "b@example.com", To: []string{"y@example.com"}, Data: []byte("b"), Status: StatusPending, CreatedAt: now.Add(time.Millisecond), UpdatedAt: now},
		{ID: "m3", From: "c@example.com", To: []string{"z@example.com"}, Data: []byte("c"), Status: StatusPending, CreatedAt: now.Add(2 * time.Millisecond), UpdatedAt: now},
	}

	if err := storage.EnqueueBatch(ctx, msgs); err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}

	stats, err := storage.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Pending != 3 || stats.Total != 3 {
		t.Errorf("stats = %+v, want Pending=3 Total=3", stats)
	}

	for _, id := range []string{"m1", "m2", "m3"} {
		got, err := storage.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}
		if got == nil {
			t.Errorf("Get(%s) = nil", id)
		}
	}
}

func TestEnqueueBatchEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewBoltStorage(filepath.Join(tmpDir, "q.db"))
	if err != nil {
		t.Fatalf("NewBoltStorage: %v", err)
	}
	defer storage.Close()

	if err := storage.EnqueueBatch(context.Background(), nil); err != nil {
		t.Errorf("EnqueueBatch(nil) = %v, want nil", err)
	}
	if err := storage.EnqueueBatch(context.Background(), []*Message{}); err != nil {
		t.Errorf("EnqueueBatch([]) = %v, want nil", err)
	}
}
