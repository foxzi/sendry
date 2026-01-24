package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"
)

// LoadCertificate loads TLS certificate from PEM files
func LoadCertificate(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ManualCertificateInfo contains information about a manually configured certificate
type ManualCertificateInfo struct {
	Subject   string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
	DaysLeft  int
	DNSNames  []string
}

// GetCertificateInfo reads certificate info from a PEM file
func GetCertificateInfo(certFile string) (*ManualCertificateInfo, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

	return &ManualCertificateInfo{
		Subject:   cert.Subject.CommonName,
		Issuer:    cert.Issuer.CommonName,
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
		DaysLeft:  daysLeft,
		DNSNames:  cert.DNSNames,
	}, nil
}
