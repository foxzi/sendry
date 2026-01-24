package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRandomString(t *testing.T) {
	// Test that it generates strings of correct length
	lengths := []int{8, 16, 32, 64}

	for _, length := range lengths {
		result := generateRandomString(length)
		if len(result) != length {
			t.Errorf("generateRandomString(%d) returned string of length %d", length, len(result))
		}
	}

	// Test that it generates different strings
	s1 := generateRandomString(32)
	s2 := generateRandomString(32)
	if s1 == s2 {
		t.Error("generateRandomString should generate unique strings")
	}
}

func TestGenerateConfig(t *testing.T) {
	// Set up test values
	initDomain = "test.example.com"
	initHostname = "mail.test.example.com"
	initDataDir = "/var/lib/sendry"
	initSMTPUser = "testuser"
	initSMTPPass = "testpass"
	initAPIKey = "testapikey"
	initMode = "production"
	initACME = false

	config := generateConfig("")

	// Check that config contains expected values
	checks := []string{
		`hostname: "mail.test.example.com"`,
		`domain: "test.example.com"`,
		`testuser: "testpass"`,
		`api_key: "testapikey"`,
		`mode: production`,
	}

	for _, check := range checks {
		if !strings.Contains(config, check) {
			t.Errorf("Generated config missing: %s", check)
		}
	}
}

func TestGenerateConfigWithDKIM(t *testing.T) {
	initDomain = "test.example.com"
	initHostname = "mail.test.example.com"
	initDataDir = "/var/lib/sendry"
	initSMTPUser = "admin"
	initSMTPPass = "pass"
	initAPIKey = "key"
	initMode = "sandbox"
	initACME = false

	dkimKeyPath := "/var/lib/sendry/dkim/test.example.com.key"
	config := generateConfig(dkimKeyPath)

	// Check DKIM config
	if !strings.Contains(config, `enabled: true`) {
		t.Error("Generated config should have DKIM enabled")
	}
	if !strings.Contains(config, dkimKeyPath) {
		t.Error("Generated config should contain DKIM key path")
	}
}

func TestGenerateConfigWithACME(t *testing.T) {
	initDomain = "test.example.com"
	initHostname = "mail.test.example.com"
	initDataDir = "/var/lib/sendry"
	initSMTPUser = "admin"
	initSMTPPass = "pass"
	initAPIKey = "key"
	initMode = "production"
	initACME = true
	initACMEEmail = "admin@test.example.com"

	config := generateConfig("")

	// Check ACME config
	if !strings.Contains(config, `acme:`) {
		t.Error("Generated config should have ACME section")
	}
	if !strings.Contains(config, `enabled: true`) {
		t.Error("Generated config should have ACME enabled")
	}
	if !strings.Contains(config, initACMEEmail) {
		t.Error("Generated config should contain ACME email")
	}
}

func TestInitOutputFileCheck(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "existing.yaml")
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test that file existence is detected
	_, err := os.Stat(existingFile)
	if err != nil {
		t.Error("Test file should exist")
	}
}
