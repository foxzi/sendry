package models

import (
	"testing"
)

func TestDomain_ConfigHash(t *testing.T) {
	domain := &Domain{
		Domain:        "example.com",
		Mode:          "production",
		DefaultFrom:   "noreply@example.com",
		DKIMEnabled:   true,
		DKIMSelector:  "mail",
		RateLimitHour: 1000,
	}

	hash1 := domain.ConfigHash()
	if hash1 == "" {
		t.Error("ConfigHash() returned empty string")
	}

	// Same config should produce same hash
	hash2 := domain.ConfigHash()
	if hash1 != hash2 {
		t.Errorf("ConfigHash() not deterministic: %v != %v", hash1, hash2)
	}

	// Change config, hash should change
	domain.Mode = "sandbox"
	hash3 := domain.ConfigHash()
	if hash1 == hash3 {
		t.Error("ConfigHash() should change when config changes")
	}
}

func TestDomain_ConfigHash_DifferentDomains(t *testing.T) {
	domain1 := &Domain{
		Domain: "example1.com",
		Mode:   "production",
	}

	domain2 := &Domain{
		Domain: "example2.com",
		Mode:   "production",
	}

	if domain1.ConfigHash() == domain2.ConfigHash() {
		t.Error("Different domains should have different hashes")
	}
}

func TestDomain_ConfigHash_WithArrays(t *testing.T) {
	domain := &Domain{
		Domain:     "example.com",
		Mode:       "redirect",
		RedirectTo: []string{"a@test.com", "b@test.com"},
	}

	hash1 := domain.ConfigHash()

	// Same array should produce same hash
	domain2 := &Domain{
		Domain:     "example.com",
		Mode:       "redirect",
		RedirectTo: []string{"a@test.com", "b@test.com"},
	}

	hash2 := domain2.ConfigHash()
	if hash1 != hash2 {
		t.Errorf("ConfigHash() not deterministic for arrays: %v != %v", hash1, hash2)
	}

	// Different array should produce different hash
	domain3 := &Domain{
		Domain:     "example.com",
		Mode:       "redirect",
		RedirectTo: []string{"a@test.com", "c@test.com"},
	}

	hash3 := domain3.ConfigHash()
	if hash1 == hash3 {
		t.Error("ConfigHash() should differ for different arrays")
	}
}

func TestDomain_ConfigHash_NilVsEmpty(t *testing.T) {
	domain1 := &Domain{
		Domain:     "example.com",
		Mode:       "production",
		RedirectTo: nil,
	}

	domain2 := &Domain{
		Domain:     "example.com",
		Mode:       "production",
		RedirectTo: []string{},
	}

	// nil and empty array might produce different hashes, that's OK
	// but they should be consistent
	hash1 := domain1.ConfigHash()
	hash2 := domain2.ConfigHash()

	// Just verify they don't panic
	if hash1 == "" || hash2 == "" {
		t.Error("ConfigHash() should not return empty string")
	}
}
