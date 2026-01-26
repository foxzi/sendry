package dnscheck

import (
	"context"
	"testing"
	"time"
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

func TestCheckDomainValidation(t *testing.T) {
	ctx := context.Background()

	// Invalid domain should return error
	_, err := CheckDomain(ctx, "invalid!domain", CheckOptions{})
	if err == nil {
		t.Error("expected error for invalid domain")
	}

	// Invalid selector should return error
	_, err = CheckDomain(ctx, "example.com", CheckOptions{Selector: "invalid!"})
	if err == nil {
		t.Error("expected error for invalid selector")
	}
}

func TestCheckDomainStructure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with a known domain
	result, err := CheckDomain(ctx, "example.com", CheckOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Domain != "example.com" {
		t.Errorf("expected domain example.com, got %s", result.Domain)
	}

	// Should have 5 results (MX, SPF, DKIM, DMARC, MTA-STS)
	if len(result.Results) != 5 {
		t.Errorf("expected 5 results, got %d", len(result.Results))
	}

	// Check that summary adds up
	total := result.Summary.OK + result.Summary.Warnings + result.Summary.Errors + result.Summary.NotFound
	if total != len(result.Results) {
		t.Errorf("summary total %d doesn't match results count %d", total, len(result.Results))
	}
}

func TestCheckDomainSelectiveChecks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Only check MX
	result, err := CheckDomain(ctx, "example.com", CheckOptions{MX: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 1 {
		t.Errorf("expected 1 result for MX only, got %d", len(result.Results))
	}

	if result.Results[0].Type != "MX Records" {
		t.Errorf("expected MX Records type, got %s", result.Results[0].Type)
	}
}

func TestCheckIPValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		ip      string
		wantErr error
	}{
		{"valid IPv4", "8.8.8.8", nil},
		{"localhost", "127.0.0.1", nil},
		{"invalid", "not-an-ip", ErrInvalidIP},
		{"IPv6", "::1", ErrIPv6NotSupported},
		{"IPv6 full", "2001:4860:4860::8888", ErrIPv6NotSupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CheckIP(ctx, tt.ip)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("CheckIP(%s) error = %v, want %v", tt.ip, err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("CheckIP(%s) unexpected error: %v", tt.ip, err)
			}
		})
	}
}

func TestCheckIPStructure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use localhost - should be clean everywhere
	result, err := CheckIP(ctx, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IP != "127.0.0.1" {
		t.Errorf("expected IP 127.0.0.1, got %s", result.IP)
	}

	// Should have results for all DNSBLs
	if len(result.Results) != len(DefaultDNSBLs) {
		t.Errorf("expected %d results, got %d", len(DefaultDNSBLs), len(result.Results))
	}

	// Check summary adds up
	total := result.Summary.Clean + result.Summary.Listed + result.Summary.Errors
	if total != len(result.Results) {
		t.Errorf("summary total %d doesn't match results count %d", total, len(result.Results))
	}
}

func TestReverseIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.2.3.4", "4.3.2.1"},
		{"192.168.1.1", "1.1.168.192"},
		{"127.0.0.1", "1.0.0.127"},
		{"8.8.8.8", "8.8.8.8"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ip := parseIPv4(tt.input)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.input)
			}
			result := reverseIP(ip)
			if result != tt.expected {
				t.Errorf("reverseIP(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func parseIPv4(s string) []byte {
	var ip [4]byte
	var parts [4]int
	n, _ := sscanf(s, "%d.%d.%d.%d", &parts[0], &parts[1], &parts[2], &parts[3])
	if n != 4 {
		return nil
	}
	for i := 0; i < 4; i++ {
		ip[i] = byte(parts[i])
	}
	return ip[:]
}

func sscanf(s, format string, args ...interface{}) (int, error) {
	var a, b, c, d int
	n := 0
	for i, j := 0, 0; i < len(s) && j < len(format); {
		if format[j] == '%' && j+1 < len(format) && format[j+1] == 'd' {
			// Parse number
			start := i
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if start == i {
				return n, nil
			}
			num := 0
			for k := start; k < i; k++ {
				num = num*10 + int(s[k]-'0')
			}
			switch n {
			case 0:
				a = num
			case 1:
				b = num
			case 2:
				c = num
			case 3:
				d = num
			}
			n++
			j += 2
		} else {
			if i < len(s) && s[i] == format[j] {
				i++
				j++
			} else {
				return n, nil
			}
		}
	}
	if len(args) > 0 {
		*args[0].(*int) = a
	}
	if len(args) > 1 {
		*args[1].(*int) = b
	}
	if len(args) > 2 {
		*args[2].(*int) = c
	}
	if len(args) > 3 {
		*args[3].(*int) = d
	}
	return n, nil
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
		{"", 10, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestListDNSBLs(t *testing.T) {
	dnsbls := ListDNSBLs()

	if len(dnsbls) < 10 {
		t.Errorf("expected at least 10 DNSBLs, got %d", len(dnsbls))
	}

	// Check that all DNSBLs have required fields
	for i, bl := range dnsbls {
		if bl.Name == "" {
			t.Errorf("DNSBL %d has empty name", i)
		}
		if bl.Zone == "" {
			t.Errorf("DNSBL %d has empty zone", i)
		}
	}
}

func TestDefaultDNSBLsContent(t *testing.T) {
	// Check that Spamhaus is in the list
	found := false
	for _, bl := range DefaultDNSBLs {
		if bl.Name == "Spamhaus ZEN" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Spamhaus ZEN in DefaultDNSBLs")
	}
}
