package repository

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestSettingsRepository_GlobalVariables(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepository(db)

	// Set variables
	vars := map[string]string{
		"company_name":  "Test Company",
		"support_email": "support@test.com",
		"website":       "https://test.com",
	}

	for key, value := range vars {
		if err := repo.SetVariable(key, value, "Test variable"); err != nil {
			t.Fatalf("SetVariable(%s) error = %v", key, err)
		}
	}

	// Get all variables
	got, err := repo.GetAllVariables()
	if err != nil {
		t.Fatalf("GetAllVariables() error = %v", err)
	}

	if len(got) != 3 {
		t.Errorf("GetAllVariables() returned %d variables, want 3", len(got))
	}

	// Get variables as map
	gotMap, err := repo.GetGlobalVariablesMap()
	if err != nil {
		t.Fatalf("GetGlobalVariablesMap() error = %v", err)
	}

	if gotMap["company_name"] != "Test Company" {
		t.Errorf("GetGlobalVariablesMap()[company_name] = %v, want %v", gotMap["company_name"], "Test Company")
	}

	// Get single variable
	v, err := repo.GetVariable("company_name")
	if err != nil {
		t.Fatalf("GetVariable() error = %v", err)
	}
	if v == nil {
		t.Fatal("GetVariable() returned nil")
	}
	if v.Value != "Test Company" {
		t.Errorf("GetVariable().Value = %v, want %v", v.Value, "Test Company")
	}

	// Update variable
	if err := repo.SetVariable("company_name", "Updated Company", "Updated"); err != nil {
		t.Fatalf("SetVariable() update error = %v", err)
	}

	gotMap, _ = repo.GetGlobalVariablesMap()
	if gotMap["company_name"] != "Updated Company" {
		t.Errorf("GetGlobalVariablesMap()[company_name] after update = %v, want %v", gotMap["company_name"], "Updated Company")
	}

	// Delete variable
	if err := repo.DeleteVariable("website"); err != nil {
		t.Fatalf("DeleteVariable() error = %v", err)
	}

	got, _ = repo.GetAllVariables()
	if len(got) != 2 {
		t.Errorf("GetAllVariables() after delete returned %d variables, want 2", len(got))
	}
}

func TestSettingsRepository_Settings(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepository(db)

	// Set setting
	if err := repo.SetSetting("test_key", "test_value"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	// Get setting
	got, err := repo.GetSetting("test_key")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}

	if got != "test_value" {
		t.Errorf("GetSetting() = %v, want %v", got, "test_value")
	}

	// Update setting
	if err := repo.SetSetting("test_key", "updated_value"); err != nil {
		t.Fatalf("SetSetting() update error = %v", err)
	}

	got, _ = repo.GetSetting("test_key")
	if got != "updated_value" {
		t.Errorf("GetSetting() after update = %v, want %v", got, "updated_value")
	}

	// Get non-existent setting
	got, err = repo.GetSetting("non_existent")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if got != "" {
		t.Errorf("GetSetting() for non-existent = %v, want empty string", got)
	}
}

func TestSettingsRepository_AuditLog(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepository(db)

	// Add audit log entry
	entry := &models.AuditLogEntry{
		UserID:     "user-123",
		UserEmail:  "test@example.com",
		Action:     "create",
		EntityType: "template",
		EntityID:   "tmpl-123",
		Details:    `{"name": "Test Template"}`,
		IPAddress:  "127.0.0.1",
	}

	if err := repo.AddAuditLog(entry); err != nil {
		t.Fatalf("AddAuditLog() error = %v", err)
	}

	// Add more entries
	entry2 := &models.AuditLogEntry{
		UserID:     "user-123",
		UserEmail:  "test@example.com",
		Action:     "update",
		EntityType: "template",
		EntityID:   "tmpl-123",
	}
	repo.AddAuditLog(entry2)

	entry3 := &models.AuditLogEntry{
		UserID:     "user-456",
		UserEmail:  "other@example.com",
		Action:     "delete",
		EntityType: "campaign",
		EntityID:   "camp-123",
	}
	repo.AddAuditLog(entry3)

	// List all
	entries, total, err := repo.ListAuditLog(models.AuditLogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLog() error = %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("ListAuditLog() returned %d entries, want 3", len(entries))
	}
	if total != 3 {
		t.Errorf("ListAuditLog() total = %d, want 3", total)
	}

	// Filter by user
	entries, _, err = repo.ListAuditLog(models.AuditLogFilter{UserID: "user-123", Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLog() error = %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("ListAuditLog() with user filter returned %d entries, want 2", len(entries))
	}

	// Filter by action
	entries, _, err = repo.ListAuditLog(models.AuditLogFilter{Action: "delete", Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLog() error = %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("ListAuditLog() with action filter returned %d entries, want 1", len(entries))
	}
}
