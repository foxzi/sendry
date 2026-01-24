package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateTestCertificate creates a self-signed certificate and key for testing
func generateTestCertificate() (certPEM, keyPEM []byte, err error) {
	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode certificate to PEM
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return certPEM, keyPEM, nil
}

func TestLoadCertificate(t *testing.T) {
	// Create temporary test certificates
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	// Generate test certificate and key dynamically
	certPEM, keyPEM, err := generateTestCertificate()
	if err != nil {
		t.Fatalf("failed to generate test certificate: %v", err)
	}

	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}

	t.Run("valid certificate", func(t *testing.T) {
		cert, err := LoadCertificate(certFile, keyFile)
		if err != nil {
			t.Errorf("unexpected error loading valid certificate: %v", err)
		}
		if cert == nil {
			t.Error("expected certificate, got nil")
		}
	})

	t.Run("non-existent cert file", func(t *testing.T) {
		_, err := LoadCertificate("/nonexistent/cert.pem", "/nonexistent/key.pem")
		if err == nil {
			t.Error("expected error for non-existent files")
		}
	})

	t.Run("invalid cert", func(t *testing.T) {
		invalidCert := filepath.Join(tmpDir, "invalid.pem")
		if err := os.WriteFile(invalidCert, []byte("invalid"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadCertificate(invalidCert, keyFile)
		if err == nil {
			t.Error("expected error for invalid certificate")
		}
	})
}
