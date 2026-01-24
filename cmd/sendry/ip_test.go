package main

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestReverseIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected string
	}{
		{"1.2.3.4", "4.3.2.1"},
		{"192.168.1.1", "1.1.168.192"},
		{"8.8.8.8", "8.8.8.8"},
		{"0.0.0.0", "0.0.0.0"},
		{"255.255.255.255", "255.255.255.255"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			result := reverseIP(ip)
			if result != tt.expected {
				t.Errorf("reverseIP(%s) = %s, want %s", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestReverseIPv6(t *testing.T) {
	// IPv6 should return empty string
	ip := net.ParseIP("2001:db8::1")
	result := reverseIP(ip)
	if result != "" {
		t.Errorf("reverseIP(IPv6) should return empty string, got %s", result)
	}
}

func TestCheckSingleDNSBL(t *testing.T) {
	// Test with a fake DNSBL that doesn't exist - should return not listed
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dnsbl := dnsblInfo{
		Name: "Test DNSBL",
		Zone: "nonexistent.test.invalid",
	}

	result := checkSingleDNSBL(ctx, "8.8.8.8", dnsbl)
	if result.Listed {
		t.Error("Expected IP not to be listed in fake DNSBL")
	}
	// Error is acceptable for non-existent domain
}

func TestDefaultDNSBLs(t *testing.T) {
	// Ensure we have DNSBLs configured
	if len(defaultDNSBLs) == 0 {
		t.Error("No default DNSBLs configured")
	}

	// Ensure all DNSBLs have required fields
	for i, bl := range defaultDNSBLs {
		if bl.Name == "" {
			t.Errorf("DNSBL %d has empty name", i)
		}
		if bl.Zone == "" {
			t.Errorf("DNSBL %d (%s) has empty zone", i, bl.Name)
		}
	}
}

func TestCheckDNSBLsContextCancellation(t *testing.T) {
	// Test that context cancellation stops the checks
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	results := checkDNSBLs(ctx, "8.8.8.8")

	// All results should have errors due to cancelled context
	for _, r := range results {
		if r.Listed {
			t.Error("Expected no listings when context is cancelled")
		}
	}
}
