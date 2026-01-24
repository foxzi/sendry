package dkim

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// KeyPair represents a DKIM key pair
type KeyPair struct {
	PrivateKey *rsa.PrivateKey
	Domain     string
	Selector   string
}

// GenerateKey generates a new RSA 2048-bit DKIM key pair
func GenerateKey(domain, selector string) (*KeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	return &KeyPair{
		PrivateKey: privateKey,
		Domain:     domain,
		Selector:   selector,
	}, nil
}

// SavePrivateKey saves the private key to a PEM file
func (kp *KeyPair) SavePrivateKey(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer file.Close()

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(kp.PrivateKey),
	}

	if err := pem.Encode(file, block); err != nil {
		return fmt.Errorf("failed to encode private key: %w", err)
	}

	return nil
}

// DNSRecord returns the DNS TXT record content for DKIM
func (kp *KeyPair) DNSRecord() string {
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&kp.PrivateKey.PublicKey)
	if err != nil {
		return ""
	}
	pubKeyBase64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	return fmt.Sprintf("v=DKIM1; k=rsa; p=%s", pubKeyBase64)
}

// DNSName returns the DNS record name for DKIM
func (kp *KeyPair) DNSName() string {
	return fmt.Sprintf("%s._domainkey.%s", kp.Selector, kp.Domain)
}

// LoadPrivateKey loads an RSA private key from a PEM file
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %s", block.Type)
	}
}
