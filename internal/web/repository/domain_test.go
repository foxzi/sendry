package repository

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestDomainRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain:      "example.com",
		Mode:        "production",
		DefaultFrom: "noreply@example.com",
		DKIMEnabled: true,
		DKIMSelector: "mail",
	}

	err := repo.Create(domain)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if domain.ID == "" {
		t.Error("Create() did not set ID")
	}

	if domain.CreatedAt.IsZero() {
		t.Error("Create() did not set CreatedAt")
	}
}

func TestDomainRepository_Create_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain: "example.com",
		Mode:   "production",
	}

	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to create duplicate
	duplicate := &models.Domain{
		Domain: "example.com",
		Mode:   "sandbox",
	}

	err := repo.Create(duplicate)
	if err == nil {
		t.Error("Create() should fail for duplicate domain")
	}
}

func TestDomainRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain:              "example.com",
		Mode:                "production",
		DefaultFrom:         "noreply@example.com",
		RateLimitHour:       1000,
		RateLimitDay:        10000,
		RateLimitRecipients: 100,
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(domain.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetByID() returned nil")
	}

	if got.Domain != domain.Domain {
		t.Errorf("GetByID() Domain = %v, want %v", got.Domain, domain.Domain)
	}

	if got.Mode != domain.Mode {
		t.Errorf("GetByID() Mode = %v, want %v", got.Mode, domain.Mode)
	}

	if got.RateLimitHour != domain.RateLimitHour {
		t.Errorf("GetByID() RateLimitHour = %v, want %v", got.RateLimitHour, domain.RateLimitHour)
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

func TestDomainRepository_GetByDomain(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain: "example.com",
		Mode:   "production",
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByDomain("example.com")
	if err != nil {
		t.Fatalf("GetByDomain() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetByDomain() returned nil")
	}

	if got.ID != domain.ID {
		t.Errorf("GetByDomain() ID = %v, want %v", got.ID, domain.ID)
	}

	// Test not found
	got, err = repo.GetByDomain("non-existent.com")
	if err != nil {
		t.Fatalf("GetByDomain() error = %v", err)
	}
	if got != nil {
		t.Error("GetByDomain() should return nil for non-existent domain")
	}
}

func TestDomainRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	// Create multiple domains
	domains := []string{"alpha.com", "beta.com", "gamma.com", "delta.com", "epsilon.com"}
	for _, d := range domains {
		domain := &models.Domain{
			Domain: d,
			Mode:   "production",
		}
		if err := repo.Create(domain); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List all
	list, err := repo.List(models.DomainFilter{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 5 {
		t.Errorf("List() returned %d domains, want 5", len(list))
	}

	// Test search
	list, err = repo.List(models.DomainFilter{Search: "alpha"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 1 {
		t.Errorf("List() with search returned %d domains, want 1", len(list))
	}

	// Test limit
	list, err = repo.List(models.DomainFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("List() with limit=2 returned %d domains, want 2", len(list))
	}
}

func TestDomainRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain:        "example.com",
		Mode:          "production",
		RateLimitHour: 1000,
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update
	domain.Mode = "sandbox"
	domain.RateLimitHour = 500

	if err := repo.Update(domain.ID, domain); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify
	got, err := repo.GetByID(domain.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.Mode != "sandbox" {
		t.Errorf("Update() Mode = %v, want sandbox", got.Mode)
	}

	if got.RateLimitHour != 500 {
		t.Errorf("Update() RateLimitHour = %v, want 500", got.RateLimitHour)
	}
}

func TestDomainRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain: "to-delete.com",
		Mode:   "production",
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete
	if err := repo.Delete(domain.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify
	got, err := repo.GetByID(domain.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got != nil {
		t.Error("GetByID() should return nil after deletion")
	}
}

func TestDomainRepository_Deployments(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain: "example.com",
		Mode:   "production",
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create deployment
	err := repo.CreateDeployment(domain.ID, "server1", "deployed", "abc123", "")
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Get deployments
	deployments, err := repo.GetDeployments(domain.ID)
	if err != nil {
		t.Fatalf("GetDeployments() error = %v", err)
	}

	if len(deployments) != 1 {
		t.Errorf("GetDeployments() returned %d deployments, want 1", len(deployments))
	}

	if deployments[0].ServerName != "server1" {
		t.Errorf("GetDeployments() ServerName = %v, want server1", deployments[0].ServerName)
	}

	if deployments[0].ConfigHash != "abc123" {
		t.Errorf("GetDeployments() ConfigHash = %v, want abc123", deployments[0].ConfigHash)
	}

	// Get specific deployment
	dep, err := repo.GetDeployment(domain.ID, "server1")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if dep == nil {
		t.Fatal("GetDeployment() returned nil")
	}

	// Update deployment (upsert)
	err = repo.CreateDeployment(domain.ID, "server1", "outdated", "def456", "")
	if err != nil {
		t.Fatalf("CreateDeployment() upsert error = %v", err)
	}

	dep, err = repo.GetDeployment(domain.ID, "server1")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if dep.Status != "outdated" {
		t.Errorf("GetDeployment() Status = %v, want outdated", dep.Status)
	}

	if dep.ConfigHash != "def456" {
		t.Errorf("GetDeployment() ConfigHash = %v, want def456", dep.ConfigHash)
	}

	// Delete deployment
	if err := repo.DeleteDeployment(domain.ID, "server1"); err != nil {
		t.Fatalf("DeleteDeployment() error = %v", err)
	}

	dep, err = repo.GetDeployment(domain.ID, "server1")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if dep != nil {
		t.Error("GetDeployment() should return nil after deletion")
	}
}

func TestDomainRepository_DeploymentsCascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain: "cascade-test.com",
		Mode:   "production",
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create deployments
	repo.CreateDeployment(domain.ID, "server1", "deployed", "hash1", "")
	repo.CreateDeployment(domain.ID, "server2", "deployed", "hash2", "")

	// Delete domain
	if err := repo.Delete(domain.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deployments are gone
	deployments, err := repo.GetDeployments(domain.ID)
	if err != nil {
		t.Fatalf("GetDeployments() error = %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("GetDeployments() returned %d deployments after cascade delete, want 0", len(deployments))
	}
}

func TestDomainRepository_GetOutdatedDeployments(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain: "example.com",
		Mode:   "production",
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create deployments with different statuses
	repo.CreateDeployment(domain.ID, "server1", "deployed", "hash1", "")
	repo.CreateDeployment(domain.ID, "server2", "outdated", "hash2", "")
	repo.CreateDeployment(domain.ID, "server3", "outdated", "hash3", "")
	repo.CreateDeployment(domain.ID, "server4", "failed", "hash4", "error")

	// Get outdated
	outdated, err := repo.GetOutdatedDeployments(domain.ID)
	if err != nil {
		t.Fatalf("GetOutdatedDeployments() error = %v", err)
	}

	if len(outdated) != 2 {
		t.Errorf("GetOutdatedDeployments() returned %d, want 2", len(outdated))
	}
}

func TestDomainRepository_RedirectBCC(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain:     "redirect-test.com",
		Mode:       "redirect",
		RedirectTo: []string{"qa@test.com", "dev@test.com"},
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(domain.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if len(got.RedirectTo) != 2 {
		t.Errorf("GetByID() RedirectTo length = %d, want 2", len(got.RedirectTo))
	}

	if got.RedirectTo[0] != "qa@test.com" {
		t.Errorf("GetByID() RedirectTo[0] = %v, want qa@test.com", got.RedirectTo[0])
	}

	// Test BCC
	domain2 := &models.Domain{
		Domain: "bcc-test.com",
		Mode:   "bcc",
		BCCTo:  []string{"archive@test.com"},
	}
	if err := repo.Create(domain2); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got2, err := repo.GetByID(domain2.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if len(got2.BCCTo) != 1 {
		t.Errorf("GetByID() BCCTo length = %d, want 1", len(got2.BCCTo))
	}
}

func TestDomainRepository_UpdateMarksDeploymentsOutdated(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDomainRepository(db)

	domain := &models.Domain{
		Domain: "example.com",
		Mode:   "production",
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create deployment
	repo.CreateDeployment(domain.ID, "server1", "deployed", "hash1", "")

	// Update domain
	domain.Mode = "sandbox"
	if err := repo.Update(domain.ID, domain); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Check deployment is marked outdated
	dep, err := repo.GetDeployment(domain.ID, "server1")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if dep.Status != "outdated" {
		t.Errorf("GetDeployment() Status = %v, want outdated after domain update", dep.Status)
	}
}
