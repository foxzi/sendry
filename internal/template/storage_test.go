package template

import (
	"context"
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func setupTestDB(t *testing.T) (*bolt.DB, func()) {
	tmpfile, err := os.CreateTemp("", "template_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpfile.Close()

	db, err := bolt.Open(tmpfile.Name(), 0600, nil)
	if err != nil {
		os.Remove(tmpfile.Name())
		t.Fatalf("failed to open db: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpfile.Name())
	}

	return db, cleanup
}

func TestStorage_Create(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	ctx := context.Background()
	tmpl := &Template{
		Name:    "welcome",
		Subject: "Hello {{.Name}}",
		Text:    "Welcome!",
	}

	err = storage.Create(ctx, tmpl)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if tmpl.ID == "" {
		t.Error("Create() did not set ID")
	}
	if tmpl.Version != 1 {
		t.Errorf("Create() version = %d, want 1", tmpl.Version)
	}
	if tmpl.CreatedAt.IsZero() {
		t.Error("Create() did not set CreatedAt")
	}
}

func TestStorage_CreateDuplicateName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	ctx := context.Background()

	tmpl1 := &Template{
		Name:    "welcome",
		Subject: "Hello",
		Text:    "Welcome!",
	}
	if err := storage.Create(ctx, tmpl1); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tmpl2 := &Template{
		Name:    "welcome",
		Subject: "Hi",
		Text:    "Hi!",
	}
	err = storage.Create(ctx, tmpl2)
	if err == nil {
		t.Error("Create() should fail for duplicate name")
	}
}

func TestStorage_Get(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	ctx := context.Background()
	tmpl := &Template{
		Name:    "welcome",
		Subject: "Hello {{.Name}}",
		Text:    "Welcome!",
	}
	if err := storage.Create(ctx, tmpl); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Get by ID
	got, err := storage.Get(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.Name != "welcome" {
		t.Errorf("Get() name = %v, want welcome", got.Name)
	}

	// Get non-existent
	got, err = storage.Get(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Error("Get() should return nil for non-existent")
	}
}

func TestStorage_GetByName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	ctx := context.Background()
	tmpl := &Template{
		Name:    "welcome",
		Subject: "Hello",
		Text:    "Welcome!",
	}
	if err := storage.Create(ctx, tmpl); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := storage.GetByName(ctx, "welcome")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByName() returned nil")
	}
	if got.ID != tmpl.ID {
		t.Errorf("GetByName() id = %v, want %v", got.ID, tmpl.ID)
	}
}

func TestStorage_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	ctx := context.Background()

	// Create templates
	for _, name := range []string{"welcome", "goodbye", "newsletter"} {
		tmpl := &Template{
			Name:    name,
			Subject: "Test",
			Text:    "Test",
		}
		if err := storage.Create(ctx, tmpl); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List all
	list, err := storage.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List() len = %d, want 3", len(list))
	}

	// List with limit
	list, err = storage.List(ctx, ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List() len = %d, want 2", len(list))
	}

	// List with search
	list, err = storage.List(ctx, ListFilter{Search: "news"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() len = %d, want 1", len(list))
	}
}

func TestStorage_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	ctx := context.Background()
	tmpl := &Template{
		Name:    "welcome",
		Subject: "Hello",
		Text:    "Welcome!",
	}
	if err := storage.Create(ctx, tmpl); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update
	tmpl.Subject = "Hi"
	if err := storage.Update(ctx, tmpl); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if tmpl.Version != 2 {
		t.Errorf("Update() version = %d, want 2", tmpl.Version)
	}

	// Verify
	got, err := storage.Get(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Subject != "Hi" {
		t.Errorf("Get() subject = %v, want Hi", got.Subject)
	}
}

func TestStorage_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	storage, err := NewStorage(db)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	ctx := context.Background()
	tmpl := &Template{
		Name:    "welcome",
		Subject: "Hello",
		Text:    "Welcome!",
	}
	if err := storage.Create(ctx, tmpl); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := storage.Delete(ctx, tmpl.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	got, err := storage.Get(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Error("Delete() did not remove template")
	}

	// Verify name index removed
	got, err = storage.GetByName(ctx, "welcome")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if got != nil {
		t.Error("Delete() did not remove name index")
	}
}
