package dkim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	kp, err := GenerateKey("example.com", "sendry")
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if kp.PrivateKey == nil {
		t.Error("PrivateKey should not be nil")
	}

	if kp.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", kp.Domain, "example.com")
	}

	if kp.Selector != "sendry" {
		t.Errorf("Selector = %q, want %q", kp.Selector, "sendry")
	}

	if kp.PrivateKey.N.BitLen() < 2048 {
		t.Errorf("key size = %d bits, want >= 2048", kp.PrivateKey.N.BitLen())
	}
}

func TestDNSName(t *testing.T) {
	kp, err := GenerateKey("example.com", "mail")
	if err != nil {
		t.Fatal(err)
	}

	want := "mail._domainkey.example.com"
	got := kp.DNSName()
	if got != want {
		t.Errorf("DNSName() = %q, want %q", got, want)
	}
}

func TestDNSRecord(t *testing.T) {
	kp, err := GenerateKey("example.com", "sendry")
	if err != nil {
		t.Fatal(err)
	}

	record := kp.DNSRecord()

	if !strings.HasPrefix(record, "v=DKIM1; k=rsa; p=") {
		t.Errorf("DNSRecord() should start with 'v=DKIM1; k=rsa; p=', got %q", record)
	}

	if len(record) < 50 {
		t.Errorf("DNSRecord() too short: %d chars", len(record))
	}
}

func TestSavePrivateKey(t *testing.T) {
	tmpDir := t.TempDir()

	kp, err := GenerateKey("example.com", "sendry")
	if err != nil {
		t.Fatal(err)
	}

	keyPath := filepath.Join(tmpDir, "subdir", "test.key")

	if err := kp.SavePrivateKey(keyPath); err != nil {
		t.Fatalf("SavePrivateKey failed: %v", err)
	}

	// Check file exists
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file not created: %v", err)
	}

	// Check file permissions (should be 0600)
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Verify we can load the saved key
	loadedKey, err := LoadPrivateKey(keyPath)
	if err != nil {
		t.Fatalf("LoadPrivateKey failed: %v", err)
	}

	if loadedKey.N.Cmp(kp.PrivateKey.N) != 0 {
		t.Error("loaded key doesn't match original")
	}
}

func TestLoadPrivateKey(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("non-existent file", func(t *testing.T) {
		_, err := LoadPrivateKey("/nonexistent/key.pem")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("invalid PEM", func(t *testing.T) {
		badFile := filepath.Join(tmpDir, "bad.pem")
		if err := os.WriteFile(badFile, []byte("not a pem"), 0600); err != nil {
			t.Fatal(err)
		}

		_, err := LoadPrivateKey(badFile)
		if err == nil {
			t.Error("expected error for invalid PEM")
		}
	})

	t.Run("valid PKCS1 key", func(t *testing.T) {
		kp, err := GenerateKey("example.com", "sendry")
		if err != nil {
			t.Fatal(err)
		}

		keyPath := filepath.Join(tmpDir, "pkcs1.key")
		if err := kp.SavePrivateKey(keyPath); err != nil {
			t.Fatal(err)
		}

		loaded, err := LoadPrivateKey(keyPath)
		if err != nil {
			t.Errorf("LoadPrivateKey failed: %v", err)
		}

		if loaded.N.Cmp(kp.PrivateKey.N) != 0 {
			t.Error("loaded key doesn't match")
		}
	})
}
