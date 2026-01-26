package dnscheck

import (
	"testing"
)

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
	}{
		{"valid simple", "example.com", false},
		{"valid subdomain", "sub.example.com", false},
		{"valid with dash", "my-domain.com", false},
		{"valid with numbers", "123.example.com", false},
		{"empty", "", true},
		{"too long", string(make([]byte, 254)), true},
		{"invalid chars", "example!.com", true},
		{"starts with dash", "-example.com", true},
		{"ends with dash", "example-.com", true},
		{"double dot", "example..com", true},
		{"path injection", "../etc/passwd", true},
		{"null byte", "example\x00.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomain(tt.domain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDomain(%q) error = %v, wantErr %v", tt.domain, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		wantErr  bool
	}{
		{"valid simple", "default", false},
		{"valid with numbers", "key2024", false},
		{"valid with dash", "dkim-key", false},
		{"empty (uses default)", "", false},
		{"too long", string(make([]byte, 64)), true},
		{"invalid chars", "selector!", true},
		{"starts with dash", "-selector", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSelector(tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSelector(%q) error = %v, wantErr %v", tt.selector, err, tt.wantErr)
			}
		})
	}
}
