package auth

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
)

func TestGenerateState(t *testing.T) {
	state1, err := generateState()
	if err != nil {
		t.Fatalf("generateState() error = %v", err)
	}

	if len(state1) == 0 {
		t.Error("generateState() returned empty string")
	}

	// Check that states are unique
	state2, err := generateState()
	if err != nil {
		t.Fatalf("generateState() error = %v", err)
	}

	if state1 == state2 {
		t.Error("generateState() returned duplicate states")
	}

	// Check state length (32 bytes base64 encoded = 44 chars)
	if len(state1) < 40 {
		t.Errorf("generateState() state too short: %d chars", len(state1))
	}
}

func TestOIDCProvider_StateManagement(t *testing.T) {
	// Create a minimal provider for testing state management
	// We can't test full OIDC flow without a real provider
	cfg := &config.OIDCConfig{
		Enabled:      true,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		IssuerURL:    "https://example.com",
		RedirectURL:  "https://app.example.com/callback",
		Scopes:       []string{"openid", "profile", "email"},
	}

	// Create provider with minimal state tracking only
	p := &OIDCProvider{
		config: cfg,
		states: make(map[string]struct{}),
	}

	// Test state storage
	state := "test-state-12345"
	p.mu.Lock()
	p.states[state] = struct{}{}
	p.mu.Unlock()

	// Verify state exists
	p.mu.RLock()
	_, exists := p.states[state]
	p.mu.RUnlock()

	if !exists {
		t.Error("State was not stored")
	}

	// Test state consumption (should be deleted after use)
	p.mu.Lock()
	_, valid := p.states[state]
	if valid {
		delete(p.states, state)
	}
	p.mu.Unlock()

	if !valid {
		t.Error("State should have been valid")
	}

	// Verify state is removed
	p.mu.RLock()
	_, exists = p.states[state]
	p.mu.RUnlock()

	if exists {
		t.Error("State should have been removed after consumption")
	}

	// Test invalid state
	p.mu.RLock()
	_, exists = p.states["invalid-state"]
	p.mu.RUnlock()

	if exists {
		t.Error("Invalid state should not exist")
	}
}

func TestOIDCProvider_DisabledConfig(t *testing.T) {
	cfg := &config.OIDCConfig{
		Enabled: false,
	}

	provider, err := NewOIDCProvider(nil, cfg)
	if err != nil {
		t.Fatalf("NewOIDCProvider() error = %v", err)
	}

	if provider != nil {
		t.Error("NewOIDCProvider() should return nil for disabled config")
	}
}

func TestUserInfo(t *testing.T) {
	info := &UserInfo{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{"admin", "users"},
	}

	if info.Email != "test@example.com" {
		t.Errorf("UserInfo.Email = %v, want %v", info.Email, "test@example.com")
	}

	if info.Name != "Test User" {
		t.Errorf("UserInfo.Name = %v, want %v", info.Name, "Test User")
	}

	if len(info.Groups) != 2 {
		t.Errorf("UserInfo.Groups length = %v, want %v", len(info.Groups), 2)
	}
}
