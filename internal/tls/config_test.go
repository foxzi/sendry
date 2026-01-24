package tls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCertificate(t *testing.T) {
	// Create temporary test certificates
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	// Self-signed test certificate and key
	certPEM := `-----BEGIN CERTIFICATE-----
MIICpDCCAYwCCQDU+pQ4P0jWMDANBgkqhkiG9w0BAQsFADAUMRIwEAYDVQQDDAls
b2NhbGhvc3QwHhcNMjQwMTAxMDAwMDAwWhcNMjUwMTAxMDAwMDAwWjAUMRIwEAYD
VQQDDAlsb2NhbGhvc3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC7
tqJ1W+y3aD6B+x+dF+jHdEJ7LV0mD3m+l/m9Y8C6EqjV3z+H8aYM5XXTG6a9E5c3
6W6Z+F3kL7F+cqNrZp4a3LCH3R0Q4Z8I0xMDi3Z1e7+A4F3H5K5c3wL8U0z8Dwhq
Z5X7Q5k7JDRF8Dl8L5X5a3J3D5I6J8c7D5J7E6E8D9F+G+H+I+J+K+L+M+N+O+P+
Q+R+S+T+U+V+W+X+Y+Z+a+b+c+d+e+f+g+h+i+j+k+l+m+n+o+p+q+r+s+t+u+v+w+
x+y+z+0+1+2+3+4+5+6+7+8+9+A+B+C+D+E+F+G+H+I+JAgMBAAEwDQYJKoZIhvcN
AQELBQADggEBAEFGgM1qYlN3bPhcDX8Ac7F7bJqH3G1yF+y3f7yW3m6d
-----END CERTIFICATE-----`

	keyPEM := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAu7aidVvst2g+gfsfnRfox3RCey1dJg95vpf5vWPAuhKo1d8/
h/GmDOV10xumvROXN+lumfhd5C+xfnKja2aeGtywh90dEOGfCNMTA4t2dXu/gOBd
x+SuXN8C/FNM/A8IameV+0OZOyQ0RfA5fC+V+WtydwuSCifHOw+SexOhPA/RfhvI
finiCkisMoxFCymuzGf8O4eMeZ3F0T4yF3C2DfMsL7T8HuKsCj1qfQp9ej4O0Gez
xvzKVEBjADlmNz4F7xP3gJL8C9W3Q0c4xH5L2d8K3R4yP0E4S7J8c7D5J7E6E8D9
F+G+H+I+J+K+L+M+N+O+P+QIDAQABAoIBAFaBC8j3TqV5c2A0tT3L5l3x0UT7Td4z
-----END RSA PRIVATE KEY-----`

	if err := os.WriteFile(certFile, []byte(certPEM), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(keyPEM), 0600); err != nil {
		t.Fatal(err)
	}

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
