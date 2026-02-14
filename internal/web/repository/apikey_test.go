package repository

import (
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestAPIKeyRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	opts := APIKeyCreateOptions{
		Name:            "Test Key",
		CreatedBy:       "test@example.com",
		Permissions:     []string{"send"},
		AllowedDomains:  []string{"example.com", "test.com"},
		RateLimitMinute: 10,
		RateLimitHour:   100,
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	if result.ID == "" {
		t.Error("expected ID to be set")
	}
	if result.Key == "" {
		t.Error("expected Key to be returned")
	}
	if !hasPrefix(result.Key, "sk_") {
		t.Errorf("expected key to start with 'sk_', got %s", result.Key[:10])
	}
	if result.Name != "Test Key" {
		t.Errorf("expected name 'Test Key', got '%s'", result.Name)
	}
	if len(result.AllowedDomains) != 2 {
		t.Errorf("expected 2 allowed domains, got %d", len(result.AllowedDomains))
	}
	if result.RateLimitMinute != 10 {
		t.Errorf("expected rate limit minute 10, got %d", result.RateLimitMinute)
	}
}

func TestAPIKeyRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	// Create a key first
	opts := APIKeyCreateOptions{
		Name:           "Get Test Key",
		CreatedBy:      "test@example.com",
		AllowedDomains: []string{"example.com"},
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Get by ID
	key, err := repo.GetByID(result.ID)
	if err != nil {
		t.Fatalf("failed to get API key: %v", err)
	}

	if key == nil {
		t.Fatal("expected key to be found")
	}
	if key.Name != "Get Test Key" {
		t.Errorf("expected name 'Get Test Key', got '%s'", key.Name)
	}
	if len(key.AllowedDomains) != 1 || key.AllowedDomains[0] != "example.com" {
		t.Errorf("expected allowed domains ['example.com'], got %v", key.AllowedDomains)
	}
}

func TestAPIKeyRepository_GetByHash(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	opts := APIKeyCreateOptions{
		Name:      "Hash Test Key",
		CreatedBy: "test@example.com",
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Get by hash
	keyHash := HashKey(result.Key)
	key, err := repo.GetByHash(keyHash)
	if err != nil {
		t.Fatalf("failed to get API key by hash: %v", err)
	}

	if key == nil {
		t.Fatal("expected key to be found")
	}
	if key.ID != result.ID {
		t.Errorf("expected ID '%s', got '%s'", result.ID, key.ID)
	}
}

func TestAPIKeyRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	// Create multiple keys
	for i := 0; i < 3; i++ {
		opts := APIKeyCreateOptions{
			Name:      "List Key " + string(rune('A'+i)),
			CreatedBy: "test@example.com",
		}
		if _, err := repo.Create(opts); err != nil {
			t.Fatalf("failed to create API key: %v", err)
		}
	}

	// List all
	keys, total, err := repo.List(models.APIKeyFilter{})
	if err != nil {
		t.Fatalf("failed to list API keys: %v", err)
	}

	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// List with search
	keys, total, err = repo.List(models.APIKeyFilter{Search: "Key A"})
	if err != nil {
		t.Fatalf("failed to list API keys with search: %v", err)
	}

	if total != 1 {
		t.Errorf("expected total 1 with search, got %d", total)
	}
}

func TestAPIKeyRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	opts := APIKeyCreateOptions{
		Name:           "Update Test Key",
		CreatedBy:      "test@example.com",
		AllowedDomains: []string{"old.com"},
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Update
	err = repo.Update(result.ID, "Updated Name", []string{"new.com", "other.com"}, 20, 200)
	if err != nil {
		t.Fatalf("failed to update API key: %v", err)
	}

	// Verify update
	key, err := repo.GetByID(result.ID)
	if err != nil {
		t.Fatalf("failed to get API key: %v", err)
	}

	if key.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got '%s'", key.Name)
	}
	if len(key.AllowedDomains) != 2 {
		t.Errorf("expected 2 allowed domains, got %d", len(key.AllowedDomains))
	}
	if key.RateLimitMinute != 20 {
		t.Errorf("expected rate limit minute 20, got %d", key.RateLimitMinute)
	}
	if key.RateLimitHour != 200 {
		t.Errorf("expected rate limit hour 200, got %d", key.RateLimitHour)
	}
}

func TestAPIKeyRepository_ToggleActive(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	opts := APIKeyCreateOptions{
		Name:      "Toggle Test Key",
		CreatedBy: "test@example.com",
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Should start active
	key, _ := repo.GetByID(result.ID)
	if !key.Active {
		t.Error("expected key to be active initially")
	}

	// Toggle to inactive
	newActive, err := repo.ToggleActive(result.ID)
	if err != nil {
		t.Fatalf("failed to toggle API key: %v", err)
	}
	if newActive {
		t.Error("expected key to be inactive after toggle")
	}

	// Verify
	key, _ = repo.GetByID(result.ID)
	if key.Active {
		t.Error("expected key to be inactive")
	}

	// Toggle back to active
	newActive, err = repo.ToggleActive(result.ID)
	if err != nil {
		t.Fatalf("failed to toggle API key: %v", err)
	}
	if !newActive {
		t.Error("expected key to be active after second toggle")
	}
}

func TestAPIKeyRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	opts := APIKeyCreateOptions{
		Name:      "Delete Test Key",
		CreatedBy: "test@example.com",
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Delete
	err = repo.Delete(result.ID)
	if err != nil {
		t.Fatalf("failed to delete API key: %v", err)
	}

	// Verify deleted
	key, err := repo.GetByID(result.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != nil {
		t.Error("expected key to be deleted")
	}
}

func TestAPIKeyRepository_UpdateLastUsed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	opts := APIKeyCreateOptions{
		Name:      "Last Used Test Key",
		CreatedBy: "test@example.com",
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Initially should be nil
	key, _ := repo.GetByID(result.ID)
	if key.LastUsedAt != nil {
		t.Error("expected LastUsedAt to be nil initially")
	}

	// Update last used
	err = repo.UpdateLastUsed(result.ID)
	if err != nil {
		t.Fatalf("failed to update last used: %v", err)
	}

	// Verify updated
	key, _ = repo.GetByID(result.ID)
	if key.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set")
	}
	if time.Since(*key.LastUsedAt) > time.Minute {
		t.Error("expected LastUsedAt to be recent")
	}
}

func TestAPIKeyRepository_Expiration(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAPIKeyRepository(db)

	expTime := time.Now().Add(24 * time.Hour)
	opts := APIKeyCreateOptions{
		Name:      "Expiring Key",
		CreatedBy: "test@example.com",
		ExpiresAt: &expTime,
	}

	result, err := repo.Create(opts)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	key, err := repo.GetByID(result.ID)
	if err != nil {
		t.Fatalf("failed to get API key: %v", err)
	}

	if key.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	// Allow some time drift
	if key.ExpiresAt.Sub(expTime).Abs() > time.Second {
		t.Errorf("expected expiration time %v, got %v", expTime, key.ExpiresAt)
	}
}

func TestAPIKey_CanSendFromDomain(t *testing.T) {
	tests := []struct {
		name           string
		allowedDomains []string
		testDomain     string
		expected       bool
	}{
		{
			name:           "no restrictions allows all",
			allowedDomains: nil,
			testDomain:     "any.com",
			expected:       true,
		},
		{
			name:           "empty slice allows all",
			allowedDomains: []string{},
			testDomain:     "any.com",
			expected:       true,
		},
		{
			name:           "allowed domain passes",
			allowedDomains: []string{"example.com", "test.com"},
			testDomain:     "example.com",
			expected:       true,
		},
		{
			name:           "disallowed domain fails",
			allowedDomains: []string{"example.com", "test.com"},
			testDomain:     "other.com",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &models.APIKey{
				AllowedDomains: tt.allowedDomains,
			}
			result := key.CanSendFromDomain(tt.testDomain)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
