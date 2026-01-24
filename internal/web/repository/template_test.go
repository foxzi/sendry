package repository

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestTemplateRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	tmpl := &models.Template{
		Name:        "Test Template",
		Description: "Test description",
		Subject:     "Test Subject",
		HTML:        "<h1>Hello {{name}}</h1>",
		Text:        "Hello {{name}}",
	}

	err := repo.Create(tmpl, "test@example.com")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if tmpl.ID == "" {
		t.Error("Create() did not set ID")
	}

	if tmpl.CurrentVersion != 1 {
		t.Errorf("Create() CurrentVersion = %d, want 1", tmpl.CurrentVersion)
	}
}

func TestTemplateRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	// Create template
	tmpl := &models.Template{
		Name:        "Test Template",
		Description: "Test description",
		Subject:     "Test Subject",
		HTML:        "<h1>Hello</h1>",
	}
	if err := repo.Create(tmpl, "test@example.com"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Get by ID
	got, err := repo.GetByID(tmpl.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetByID() returned nil")
	}

	if got.Name != tmpl.Name {
		t.Errorf("GetByID() Name = %v, want %v", got.Name, tmpl.Name)
	}

	if got.Subject != tmpl.Subject {
		t.Errorf("GetByID() Subject = %v, want %v", got.Subject, tmpl.Subject)
	}

	// Test not found
	got, err = repo.GetByID("non-existent")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got != nil {
		t.Error("GetByID() should return nil for non-existent ID")
	}
}

func TestTemplateRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	// Create multiple templates
	for i := 0; i < 5; i++ {
		tmpl := &models.Template{
			Name:    "Template " + string(rune('A'+i)),
			Subject: "Subject",
		}
		if err := repo.Create(tmpl, "test@example.com"); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List all
	templates, total, err := repo.List(models.TemplateListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(templates) != 5 {
		t.Errorf("List() returned %d templates, want 5", len(templates))
	}

	if total != 5 {
		t.Errorf("List() total = %d, want 5", total)
	}

	// Test pagination
	templates, _, err = repo.List(models.TemplateListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(templates) != 2 {
		t.Errorf("List() with limit=2 returned %d templates, want 2", len(templates))
	}

	// Test search
	templates, _, err = repo.List(models.TemplateListFilter{Search: "Template A", Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(templates) != 1 {
		t.Errorf("List() with search returned %d templates, want 1", len(templates))
	}
}

func TestTemplateRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	tmpl := &models.Template{
		Name:    "Original Name",
		Subject: "Original Subject",
	}
	if err := repo.Create(tmpl, "test@example.com"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update
	tmpl.Name = "Updated Name"
	tmpl.Subject = "Updated Subject"

	if err := repo.Update(tmpl, "Updated content", "test@example.com"); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	got, err := repo.GetByID(tmpl.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.Name != "Updated Name" {
		t.Errorf("Update() Name = %v, want %v", got.Name, "Updated Name")
	}

	if got.Subject != "Updated Subject" {
		t.Errorf("Update() Subject = %v, want %v", got.Subject, "Updated Subject")
	}

	// Check version incremented
	if got.CurrentVersion != 2 {
		t.Errorf("Update() CurrentVersion = %d, want 2", got.CurrentVersion)
	}
}

func TestTemplateRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	tmpl := &models.Template{
		Name:    "To Delete",
		Subject: "Subject",
	}
	if err := repo.Create(tmpl, "test@example.com"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete
	if err := repo.Delete(tmpl.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deletion
	got, err := repo.GetByID(tmpl.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got != nil {
		t.Error("GetByID() should return nil after deletion")
	}
}

func TestTemplateRepository_Versions(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	tmpl := &models.Template{
		Name:    "Versioned Template",
		Subject: "Subject v1",
		HTML:    "<h1>v1</h1>",
	}
	if err := repo.Create(tmpl, "test@example.com"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update to create v2
	tmpl.Subject = "Subject v2"
	tmpl.HTML = "<h1>v2</h1>"
	if err := repo.Update(tmpl, "Updated to v2", "test@example.com"); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// List versions
	versions, err := repo.GetVersions(tmpl.ID)
	if err != nil {
		t.Fatalf("GetVersions() error = %v", err)
	}

	if len(versions) != 2 {
		t.Errorf("GetVersions() returned %d versions, want 2", len(versions))
	}

	// Get specific version
	v1, err := repo.GetVersion(tmpl.ID, 1)
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if v1 == nil {
		t.Fatal("GetVersion() returned nil for v1")
	}
	if v1.Subject != "Subject v1" {
		t.Errorf("GetVersion() v1.Subject = %v, want %v", v1.Subject, "Subject v1")
	}
}

func TestTemplateRepository_Deployments(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	tmpl := &models.Template{
		Name:    "Deploy Test",
		Subject: "Subject",
	}
	if err := repo.Create(tmpl, "test@example.com"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Save deployment
	deployment := &models.TemplateDeployment{
		TemplateID:      tmpl.ID,
		ServerName:      "test-server",
		RemoteID:        "remote-123",
		DeployedVersion: 1,
	}

	if err := repo.SaveDeployment(deployment); err != nil {
		t.Fatalf("SaveDeployment() error = %v", err)
	}

	// Get deployment
	got, err := repo.GetDeployment(tmpl.ID, "test-server")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetDeployment() returned nil")
	}

	if got.RemoteID != "remote-123" {
		t.Errorf("GetDeployment() RemoteID = %v, want %v", got.RemoteID, "remote-123")
	}

	// List deployments
	deployments, err := repo.GetDeployments(tmpl.ID)
	if err != nil {
		t.Fatalf("GetDeployments() error = %v", err)
	}

	if len(deployments) != 1 {
		t.Errorf("GetDeployments() returned %d deployments, want 1", len(deployments))
	}

	// Update deployment (upsert)
	deployment.DeployedVersion = 2
	if err := repo.SaveDeployment(deployment); err != nil {
		t.Fatalf("SaveDeployment() update error = %v", err)
	}

	got, err = repo.GetDeployment(tmpl.ID, "test-server")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if got.DeployedVersion != 2 {
		t.Errorf("GetDeployment() DeployedVersion = %d, want 2", got.DeployedVersion)
	}
}

func TestTemplateRepository_Folders(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTemplateRepository(db)

	// Create templates with folders
	folders := []string{"newsletters", "transactional", "marketing"}
	for i, folder := range folders {
		tmpl := &models.Template{
			Name:    "Template " + string(rune('A'+i)),
			Subject: "Subject",
			Folder:  folder,
		}
		if err := repo.Create(tmpl, "test@example.com"); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Get folders
	got, err := repo.GetFolders()
	if err != nil {
		t.Fatalf("GetFolders() error = %v", err)
	}

	if len(got) != 3 {
		t.Errorf("GetFolders() returned %d folders, want 3", len(got))
	}
}
