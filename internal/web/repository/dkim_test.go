package repository

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestDKIMRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "example.com",
		Selector:   "mail",
		PrivateKey: "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}

	err := repo.Create(key)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if key.ID == "" {
		t.Error("Create() did not set ID")
	}

	if key.CreatedAt.IsZero() {
		t.Error("Create() did not set CreatedAt")
	}
}

func TestDKIMRepository_Create_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "example.com",
		Selector:   "mail",
		PrivateKey: "test-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}

	if err := repo.Create(key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to create duplicate
	duplicate := &models.DKIMKey{
		Domain:     "example.com",
		Selector:   "mail",
		PrivateKey: "another-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=anotherkey",
	}

	err := repo.Create(duplicate)
	if err == nil {
		t.Error("Create() should fail for duplicate domain+selector")
	}
}

func TestDKIMRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "example.com",
		Selector:   "mail",
		PrivateKey: "test-private-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}
	if err := repo.Create(key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(key.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetByID() returned nil")
	}

	if got.Domain != key.Domain {
		t.Errorf("GetByID() Domain = %v, want %v", got.Domain, key.Domain)
	}

	if got.Selector != key.Selector {
		t.Errorf("GetByID() Selector = %v, want %v", got.Selector, key.Selector)
	}

	if got.PrivateKey != key.PrivateKey {
		t.Errorf("GetByID() PrivateKey = %v, want %v", got.PrivateKey, key.PrivateKey)
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

func TestDKIMRepository_GetByDomainSelector(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "example.com",
		Selector:   "mail",
		PrivateKey: "test-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}
	if err := repo.Create(key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByDomainSelector("example.com", "mail")
	if err != nil {
		t.Fatalf("GetByDomainSelector() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetByDomainSelector() returned nil")
	}

	if got.ID != key.ID {
		t.Errorf("GetByDomainSelector() ID = %v, want %v", got.ID, key.ID)
	}

	// Test not found
	got, err = repo.GetByDomainSelector("other.com", "mail")
	if err != nil {
		t.Fatalf("GetByDomainSelector() error = %v", err)
	}
	if got != nil {
		t.Error("GetByDomainSelector() should return nil for non-existent")
	}
}

func TestDKIMRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	// Create multiple keys
	domains := []string{"alpha.com", "beta.com", "gamma.com"}
	for _, d := range domains {
		key := &models.DKIMKey{
			Domain:     d,
			Selector:   "mail",
			PrivateKey: "key-" + d,
			DNSRecord:  "v=DKIM1; k=rsa; p=" + d,
		}
		if err := repo.Create(key); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("List() returned %d keys, want 3", len(list))
	}

	// Verify DNSName is computed
	for _, k := range list {
		expectedDNSName := k.Selector + "._domainkey." + k.Domain
		if k.DNSName != expectedDNSName {
			t.Errorf("List() DNSName = %v, want %v", k.DNSName, expectedDNSName)
		}
	}
}

func TestDKIMRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "to-delete.com",
		Selector:   "mail",
		PrivateKey: "test-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}
	if err := repo.Create(key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(key.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, err := repo.GetByID(key.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got != nil {
		t.Error("GetByID() should return nil after deletion")
	}
}

func TestDKIMRepository_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	err := repo.Delete("non-existent")
	if err == nil {
		t.Error("Delete() should return error for non-existent ID")
	}
}

func TestDKIMRepository_Deployments(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "example.com",
		Selector:   "mail",
		PrivateKey: "test-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}
	if err := repo.Create(key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create deployment
	err := repo.CreateDeployment(key.ID, "server1", "deployed", "")
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Get deployments
	deployments, err := repo.GetDeployments(key.ID)
	if err != nil {
		t.Fatalf("GetDeployments() error = %v", err)
	}

	if len(deployments) != 1 {
		t.Errorf("GetDeployments() returned %d deployments, want 1", len(deployments))
	}

	if deployments[0].ServerName != "server1" {
		t.Errorf("GetDeployments() ServerName = %v, want server1", deployments[0].ServerName)
	}

	if deployments[0].Status != "deployed" {
		t.Errorf("GetDeployments() Status = %v, want deployed", deployments[0].Status)
	}

	// Update deployment (upsert)
	err = repo.CreateDeployment(key.ID, "server1", "failed", "connection error")
	if err != nil {
		t.Fatalf("CreateDeployment() upsert error = %v", err)
	}

	deployments, err = repo.GetDeployments(key.ID)
	if err != nil {
		t.Fatalf("GetDeployments() error = %v", err)
	}

	if deployments[0].Status != "failed" {
		t.Errorf("GetDeployments() Status = %v, want failed", deployments[0].Status)
	}

	if deployments[0].Error != "connection error" {
		t.Errorf("GetDeployments() Error = %v, want 'connection error'", deployments[0].Error)
	}

	// Delete deployment
	if err := repo.DeleteDeployment(key.ID, "server1"); err != nil {
		t.Fatalf("DeleteDeployment() error = %v", err)
	}

	deployments, err = repo.GetDeployments(key.ID)
	if err != nil {
		t.Fatalf("GetDeployments() error = %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("GetDeployments() returned %d deployments after delete, want 0", len(deployments))
	}
}

func TestDKIMRepository_DeploymentsCascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "cascade-test.com",
		Selector:   "mail",
		PrivateKey: "test-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}
	if err := repo.Create(key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create deployments
	repo.CreateDeployment(key.ID, "server1", "deployed", "")
	repo.CreateDeployment(key.ID, "server2", "deployed", "")

	// Delete key
	if err := repo.Delete(key.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deployments are gone
	deployments, err := repo.GetDeployments(key.ID)
	if err != nil {
		t.Fatalf("GetDeployments() error = %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("GetDeployments() returned %d after cascade delete, want 0", len(deployments))
	}
}

func TestDKIMRepository_GetByID_WithDeployments(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key := &models.DKIMKey{
		Domain:     "example.com",
		Selector:   "mail",
		PrivateKey: "test-key",
		DNSRecord:  "v=DKIM1; k=rsa; p=testkey",
	}
	if err := repo.Create(key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create deployments
	repo.CreateDeployment(key.ID, "server1", "deployed", "")
	repo.CreateDeployment(key.ID, "server2", "failed", "error")

	// GetByID should load deployments
	got, err := repo.GetByID(key.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if len(got.Deployments) != 2 {
		t.Errorf("GetByID() Deployments length = %d, want 2", len(got.Deployments))
	}
}

func TestDKIMRepository_ListWithDeploymentCount(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDKIMRepository(db)

	key1 := &models.DKIMKey{
		Domain:     "domain1.com",
		Selector:   "mail",
		PrivateKey: "key1",
		DNSRecord:  "record1",
	}
	key2 := &models.DKIMKey{
		Domain:     "domain2.com",
		Selector:   "mail",
		PrivateKey: "key2",
		DNSRecord:  "record2",
	}

	repo.Create(key1)
	repo.Create(key2)

	// Add deployments to key1 only
	repo.CreateDeployment(key1.ID, "server1", "deployed", "")
	repo.CreateDeployment(key1.ID, "server2", "deployed", "")

	list, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Find key1 in list
	var foundKey1 *models.DKIMKeyListItem
	for i := range list {
		if list[i].Domain == "domain1.com" {
			foundKey1 = &list[i]
			break
		}
	}

	if foundKey1 == nil {
		t.Fatal("List() did not return domain1.com")
	}

	if foundKey1.DeploymentCount != 2 {
		t.Errorf("List() DeploymentCount = %d, want 2", foundKey1.DeploymentCount)
	}
}
