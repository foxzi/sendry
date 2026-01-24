package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestStorage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Test Save and Get
	msg := &Message{
		ID:         "test-123",
		From:       "sender@example.com",
		To:         []string{"recipient@example.com"},
		Subject:    "Test Subject",
		Data:       []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nBody"),
		Domain:     "example.com",
		Mode:       "sandbox",
		CapturedAt: time.Now(),
		ClientIP:   "127.0.0.1",
	}

	if err := storage.Save(ctx, msg); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}

	retrieved, err := storage.Get(ctx, "test-123")
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected message, got nil")
	}

	if retrieved.ID != msg.ID {
		t.Errorf("expected ID %s, got %s", msg.ID, retrieved.ID)
	}
	if retrieved.From != msg.From {
		t.Errorf("expected From %s, got %s", msg.From, retrieved.From)
	}
	if retrieved.Subject != msg.Subject {
		t.Errorf("expected Subject %s, got %s", msg.Subject, retrieved.Subject)
	}
}

func TestStorageList(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Save multiple messages
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:         "msg-" + string(rune('a'+i)),
			From:       "sender@example.com",
			To:         []string{"recipient@example.com"},
			Subject:    "Test Subject",
			Data:       []byte("test"),
			Domain:     "example.com",
			Mode:       "sandbox",
			CapturedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := storage.Save(ctx, msg); err != nil {
			t.Fatalf("failed to save message: %v", err)
		}
	}

	// Test List
	messages, err := storage.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("expected 5 messages, got %d", len(messages))
	}

	// Test List with limit
	messages, err = storage.List(ctx, ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	// Test List with domain filter
	messages, err = storage.List(ctx, ListFilter{Domain: "example.com"})
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("expected 5 messages, got %d", len(messages))
	}

	messages, err = storage.List(ctx, ListFilter{Domain: "other.com"})
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
}

func TestStorageDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	msg := &Message{
		ID:         "delete-me",
		From:       "sender@example.com",
		To:         []string{"recipient@example.com"},
		Data:       []byte("test"),
		Domain:     "example.com",
		Mode:       "sandbox",
		CapturedAt: time.Now(),
	}

	if err := storage.Save(ctx, msg); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}

	// Verify it exists
	retrieved, err := storage.Get(ctx, "delete-me")
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected message before delete")
	}

	// Delete it
	if err := storage.Delete(ctx, "delete-me"); err != nil {
		t.Fatalf("failed to delete message: %v", err)
	}

	// Verify it's gone
	retrieved, err = storage.Get(ctx, "delete-me")
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}
	if retrieved != nil {
		t.Error("expected message to be deleted")
	}
}

func TestStorageClear(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Save messages for different domains
	domains := []string{"domain1.com", "domain2.com"}
	for _, domain := range domains {
		for i := 0; i < 3; i++ {
			msg := &Message{
				ID:         domain + "-" + string(rune('a'+i)),
				From:       "sender@" + domain,
				To:         []string{"recipient@example.com"},
				Data:       []byte("test"),
				Domain:     domain,
				Mode:       "sandbox",
				CapturedAt: time.Now(),
			}
			if err := storage.Save(ctx, msg); err != nil {
				t.Fatalf("failed to save message: %v", err)
			}
		}
	}

	// Clear only domain1.com
	count, err := storage.Clear(ctx, "domain1.com", 0)
	if err != nil {
		t.Fatalf("failed to clear messages: %v", err)
	}

	if count != 3 {
		t.Errorf("expected to clear 3 messages, cleared %d", count)
	}

	// Verify domain2.com still has messages
	messages, err := storage.List(ctx, ListFilter{Domain: "domain2.com"})
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 3 {
		t.Errorf("expected 3 messages remaining, got %d", len(messages))
	}
}

func TestStorageStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Save messages
	modes := []string{"sandbox", "redirect", "bcc"}
	for i, mode := range modes {
		msg := &Message{
			ID:         "stats-" + mode,
			From:       "sender@example.com",
			To:         []string{"recipient@example.com"},
			Data:       []byte("test data"),
			Domain:     "example.com",
			Mode:       mode,
			CapturedAt: time.Now().Add(time.Duration(i) * time.Hour),
		}
		if err := storage.Save(ctx, msg); err != nil {
			t.Fatalf("failed to save message: %v", err)
		}
	}

	stats, err := storage.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("expected total 3, got %d", stats.Total)
	}

	if stats.ByMode["sandbox"] != 1 {
		t.Errorf("expected 1 sandbox message, got %d", stats.ByMode["sandbox"])
	}
	if stats.ByMode["redirect"] != 1 {
		t.Errorf("expected 1 redirect message, got %d", stats.ByMode["redirect"])
	}
	if stats.ByMode["bcc"] != 1 {
		t.Errorf("expected 1 bcc message, got %d", stats.ByMode["bcc"])
	}

	if stats.ByDomain["example.com"] != 3 {
		t.Errorf("expected 3 messages for example.com, got %d", stats.ByDomain["example.com"])
	}
}

func TestStorageNewStorageCreatesBucket(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Create storage should create the bucket
	_, err = NewStorage(db)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Verify bucket exists
	err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("sandbox"))
		if bucket == nil {
			t.Error("sandbox bucket was not created")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to verify bucket: %v", err)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
