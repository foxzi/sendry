package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/foxzi/sendry/internal/config"
)

func TestDKIMGenerate(t *testing.T) {
	tmpDir := t.TempDir()
	dkimDir := filepath.Join(tmpDir, "dkim")

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, dkimDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	body := `{"domain": "test.com", "selector": "mail"}`
	req := httptest.NewRequest("POST", "/dkim/generate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp DKIMGenerateResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Domain != "test.com" {
		t.Errorf("expected domain test.com, got %s", resp.Domain)
	}
	if resp.Selector != "mail" {
		t.Errorf("expected selector mail, got %s", resp.Selector)
	}
	if resp.DNSName != "mail._domainkey.test.com" {
		t.Errorf("expected DNS name mail._domainkey.test.com, got %s", resp.DNSName)
	}
	if resp.DNSRecord == "" {
		t.Error("expected non-empty DNS record")
	}

	// Check key file was created
	keyFile := filepath.Join(dkimDir, "test.com", "mail.key")
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Errorf("key file was not created: %s", keyFile)
	}
}

func TestDKIMGet(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
		DKIM: config.DKIMConfig{
			Enabled:  true,
			Domain:   "example.com",
			Selector: "default",
			KeyFile:  "/nonexistent/key.pem",
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/dkim/example.com", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp DKIMInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Domain != "example.com" {
		t.Errorf("expected domain example.com, got %s", resp.Domain)
	}
	if !resp.Enabled {
		t.Error("expected DKIM to be enabled")
	}
	if resp.Selector != "default" {
		t.Errorf("expected selector default, got %s", resp.Selector)
	}
}

func TestDomainsList(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
		Domains: map[string]config.DomainConfig{
			"example.com": {
				Mode: "production",
			},
			"test.com": {
				Mode: "sandbox",
			},
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/domains/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp DomainsListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have at least the domains from config
	if len(resp.Domains) == 0 {
		t.Error("expected at least one domain")
	}
}

func TestDomainsCreate(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	body := `{"domain": "new.com", "mode": "sandbox"}`
	req := httptest.NewRequest("POST", "/domains/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp DomainResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Domain != "new.com" {
		t.Errorf("expected domain new.com, got %s", resp.Domain)
	}
	if resp.Mode != "sandbox" {
		t.Errorf("expected mode sandbox, got %s", resp.Mode)
	}

	// Check domain was added to config
	if _, exists := cfg.Domains["new.com"]; !exists {
		t.Error("domain was not added to config")
	}
}

func TestDomainsUpdate(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
		Domains: map[string]config.DomainConfig{
			"test.com": {
				Mode: "sandbox",
			},
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	body := `{"mode": "production"}`
	req := httptest.NewRequest("PUT", "/domains/test.com", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Check domain was updated
	if cfg.Domains["test.com"].Mode != "production" {
		t.Errorf("expected mode production, got %s", cfg.Domains["test.com"].Mode)
	}
}

func TestDomainsDelete(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
		Domains: map[string]config.DomainConfig{
			"test.com": {
				Mode: "sandbox",
			},
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("DELETE", "/domains/test.com", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d: %s", http.StatusNoContent, w.Code, w.Body.String())
	}

	// Check domain was deleted
	if _, exists := cfg.Domains["test.com"]; exists {
		t.Error("domain was not deleted from config")
	}
}

func TestRateLimitsGet(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
		RateLimit: config.RateLimitConfig{
			Enabled: true,
			Global: &config.LimitValues{
				MessagesPerHour: 1000,
				MessagesPerDay:  10000,
			},
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/ratelimits/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp RateLimitsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Enabled {
		t.Error("expected rate limiting to be enabled")
	}
	if resp.Global == nil {
		t.Fatal("expected global rate limits")
	}
	if resp.Global.MessagesPerHour != 1000 {
		t.Errorf("expected messages per hour 1000, got %d", resp.Global.MessagesPerHour)
	}
}

func TestTLSList(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
			TLS: config.TLSConfig{
				CertFile: "/path/to/cert.pem",
				KeyFile:  "/path/to/key.pem",
			},
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tmpDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/tls/certificates", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp TLSListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(resp.Certificates))
	}
	if resp.Certificates[0].Domain != "example.com" {
		t.Errorf("expected domain example.com, got %s", resp.Certificates[0].Domain)
	}
}

func TestTLSUpload(t *testing.T) {
	tmpDir := t.TempDir()
	tlsDir := filepath.Join(tmpDir, "tls")

	cfg := &config.Config{
		SMTP: config.SMTPConfig{
			Domain: "example.com",
		},
	}

	mgmt := NewManagementServer(nil, nil, cfg, tmpDir, tlsDir)

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	body := `{
		"domain": "test.com",
		"certificate": "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
		"private_key": "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"
	}`
	req := httptest.NewRequest("POST", "/tls/certificates", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	// Check files were created
	certFile := filepath.Join(tlsDir, "test.com", "cert.pem")
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Errorf("cert file was not created: %s", certFile)
	}

	keyFile := filepath.Join(tlsDir, "test.com", "key.pem")
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Errorf("key file was not created: %s", keyFile)
	}
}

func TestDNSCheckValidDomain(t *testing.T) {
	cfg := &config.Config{}
	mgmt := NewManagementServer(nil, nil, cfg, t.TempDir(), t.TempDir())

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/dns/check/example.com", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["domain"] != "example.com" {
		t.Errorf("expected domain example.com, got %v", resp["domain"])
	}
	if resp["results"] == nil {
		t.Error("expected results array")
	}
	if resp["summary"] == nil {
		t.Error("expected summary object")
	}
}

func TestDNSCheckInvalidDomain(t *testing.T) {
	cfg := &config.Config{}
	mgmt := NewManagementServer(nil, nil, cfg, t.TempDir(), t.TempDir())

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	tests := []struct {
		name   string
		domain string
	}{
		{"path injection", "../etc/passwd"},
		{"invalid chars", "exam!ple.com"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/dns/check/" + tt.domain
			if tt.domain == "" {
				path = "/dns/check/"
			}
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Either 400 Bad Request or 404 Not Found for empty
			if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
				t.Errorf("expected status 400 or 404, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestDNSCheckWithSelector(t *testing.T) {
	cfg := &config.Config{}
	mgmt := NewManagementServer(nil, nil, cfg, t.TempDir(), t.TempDir())

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/dns/check/example.com?selector=dkim2024", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestIPCheckValidIP(t *testing.T) {
	cfg := &config.Config{}
	mgmt := NewManagementServer(nil, nil, cfg, t.TempDir(), t.TempDir())

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	// Use localhost - will be clean in all DNSBLs
	req := httptest.NewRequest("GET", "/ip/check/127.0.0.1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["ip"] != "127.0.0.1" {
		t.Errorf("expected ip 127.0.0.1, got %v", resp["ip"])
	}
	if resp["results"] == nil {
		t.Error("expected results array")
	}
}

func TestIPCheckInvalidIP(t *testing.T) {
	cfg := &config.Config{}
	mgmt := NewManagementServer(nil, nil, cfg, t.TempDir(), t.TempDir())

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	tests := []struct {
		name string
		ip   string
	}{
		{"not an IP", "notanip"},
		{"invalid format", "999.999.999.999"},
		{"partial", "192.168"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ip/check/"+tt.ip, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestIPCheckIPv6NotSupported(t *testing.T) {
	cfg := &config.Config{}
	mgmt := NewManagementServer(nil, nil, cfg, t.TempDir(), t.TempDir())

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/ip/check/::1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	errMsg, ok := resp["error"].(string)
	if !ok || errMsg == "" {
		t.Error("expected error message")
	}
}

func TestDNSBLList(t *testing.T) {
	cfg := &config.Config{}
	mgmt := NewManagementServer(nil, nil, cfg, t.TempDir(), t.TempDir())

	router := chi.NewRouter()
	mgmt.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/ip/dnsbls", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	count, ok := resp["count"].(float64)
	if !ok || count < 10 {
		t.Errorf("expected count >= 10, got %v", resp["count"])
	}

	dnsbls, ok := resp["dnsbls"].([]interface{})
	if !ok || len(dnsbls) < 10 {
		t.Errorf("expected at least 10 DNSBLs, got %d", len(dnsbls))
	}
}
