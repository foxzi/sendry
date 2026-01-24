package dkim

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSigner(t *testing.T) {
	kp, err := GenerateKey("example.com", "sendry")
	if err != nil {
		t.Fatal(err)
	}

	signer := NewSigner(kp.PrivateKey, "example.com", "sendry")

	if signer.Domain() != "example.com" {
		t.Errorf("Domain() = %q, want %q", signer.Domain(), "example.com")
	}

	if signer.Selector() != "sendry" {
		t.Errorf("Selector() = %q, want %q", signer.Selector(), "sendry")
	}
}

func TestNewSignerFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate and save a key
	kp, err := GenerateKey("example.com", "sendry")
	if err != nil {
		t.Fatal(err)
	}

	keyPath := filepath.Join(tmpDir, "test.key")
	if err := kp.SavePrivateKey(keyPath); err != nil {
		t.Fatal(err)
	}

	t.Run("valid key file", func(t *testing.T) {
		signer, err := NewSignerFromFile(keyPath, "example.com", "sendry")
		if err != nil {
			t.Fatalf("NewSignerFromFile failed: %v", err)
		}

		if signer.Domain() != "example.com" {
			t.Errorf("Domain() = %q, want %q", signer.Domain(), "example.com")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := NewSignerFromFile("/nonexistent/key.pem", "example.com", "sendry")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})
}

func TestSign(t *testing.T) {
	kp, err := GenerateKey("example.com", "sendry")
	if err != nil {
		t.Fatal(err)
	}

	signer := NewSigner(kp.PrivateKey, "example.com", "sendry")

	message := []byte(`From: sender@example.com
To: recipient@example.org
Subject: Test Message
Date: Mon, 1 Jan 2024 12:00:00 +0000
MIME-Version: 1.0
Content-Type: text/plain; charset=utf-8

This is a test message.
`)

	signed, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Check that DKIM-Signature header was added
	if !bytes.Contains(signed, []byte("DKIM-Signature:")) {
		t.Error("signed message should contain DKIM-Signature header")
	}

	// Check that original content is preserved
	if !bytes.Contains(signed, []byte("This is a test message.")) {
		t.Error("signed message should contain original body")
	}

	// Check signature contains required fields
	signedStr := string(signed)
	if !strings.Contains(signedStr, "d=example.com") {
		t.Error("DKIM signature should contain domain")
	}
	if !strings.Contains(signedStr, "s=sendry") {
		t.Error("DKIM signature should contain selector")
	}
}

func TestSignEmptyMessage(t *testing.T) {
	kp, err := GenerateKey("example.com", "sendry")
	if err != nil {
		t.Fatal(err)
	}

	signer := NewSigner(kp.PrivateKey, "example.com", "sendry")

	// Minimal valid message
	message := []byte(`From: sender@example.com
To: recipient@example.org
Subject: Empty

`)

	signed, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if !bytes.Contains(signed, []byte("DKIM-Signature:")) {
		t.Error("signed message should contain DKIM-Signature header")
	}
}

func TestSignRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate key, save, and load it
	kp, err := GenerateKey("test.example.com", "mail")
	if err != nil {
		t.Fatal(err)
	}

	keyPath := filepath.Join(tmpDir, "roundtrip.key")
	if err := kp.SavePrivateKey(keyPath); err != nil {
		t.Fatal(err)
	}

	// Create signer from loaded key
	signer, err := NewSignerFromFile(keyPath, "test.example.com", "mail")
	if err != nil {
		t.Fatal(err)
	}

	message := []byte(`From: test@test.example.com
To: user@other.com
Subject: Round Trip Test

Testing round trip signing.
`)

	signed, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	signedStr := string(signed)
	if !strings.Contains(signedStr, "d=test.example.com") {
		t.Error("domain not found in signature")
	}
	if !strings.Contains(signedStr, "s=mail") {
		t.Error("selector not found in signature")
	}
}

func BenchmarkSign(b *testing.B) {
	tmpDir := b.TempDir()

	kp, _ := GenerateKey("example.com", "sendry")
	keyPath := filepath.Join(tmpDir, "bench.key")
	kp.SavePrivateKey(keyPath)

	signer, _ := NewSignerFromFile(keyPath, "example.com", "sendry")

	message := []byte(`From: sender@example.com
To: recipient@example.org
Subject: Benchmark Test
Date: Mon, 1 Jan 2024 12:00:00 +0000
MIME-Version: 1.0
Content-Type: text/plain; charset=utf-8

This is a test message for benchmarking DKIM signing performance.
`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		signer.Sign(message)
	}
}

// helper to create temp key file for testing
func createTempKeyFile(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	kp, _ := GenerateKey("example.com", "sendry")
	keyPath := filepath.Join(tmpDir, "test.key")
	kp.SavePrivateKey(keyPath)
	return keyPath, func() { os.RemoveAll(tmpDir) }
}
