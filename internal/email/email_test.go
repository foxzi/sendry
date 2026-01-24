package email

import "testing"

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{"simple", "user@example.com", "example.com"},
		{"with name", "User Name <user@example.com>", "example.com"},
		{"uppercase", "user@EXAMPLE.COM", "example.com"},
		{"mixed case", "user@Sub.Example.Com", "sub.example.com"},
		{"invalid no at", "invalid", ""},
		{"invalid empty before at", "@example.com", ""},
		{"invalid empty after at", "user@", ""},
		{"empty", "", ""},
		{"single char domain", "user@a", "a"},
		{"subdomain", "user@mail.example.com", "mail.example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractDomain(tc.email)
			if result != tc.expected {
				t.Errorf("ExtractDomain(%q) = %q, want %q", tc.email, result, tc.expected)
			}
		})
	}
}

func TestExtractDomainOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		def      string
		expected string
	}{
		{"valid email", "user@example.com", "localhost", "example.com"},
		{"invalid returns default", "invalid", "localhost", "localhost"},
		{"empty returns default", "", "localhost", "localhost"},
		{"custom default", "invalid", "custom.local", "custom.local"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractDomainOrDefault(tc.email, tc.def)
			if result != tc.expected {
				t.Errorf("ExtractDomainOrDefault(%q, %q) = %q, want %q", tc.email, tc.def, result, tc.expected)
			}
		})
	}
}
