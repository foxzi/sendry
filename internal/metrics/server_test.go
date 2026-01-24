package metrics

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewServerWithAllowedIPs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New()

	tests := []struct {
		name       string
		allowedIPs []string
		wantCount  int
	}{
		{
			name:       "empty list",
			allowedIPs: nil,
			wantCount:  0,
		},
		{
			name:       "single IP",
			allowedIPs: []string{"192.168.1.1"},
			wantCount:  1,
		},
		{
			name:       "multiple IPs",
			allowedIPs: []string{"192.168.1.1", "10.0.0.1"},
			wantCount:  2,
		},
		{
			name:       "CIDR notation",
			allowedIPs: []string{"192.168.0.0/16", "10.0.0.0/8"},
			wantCount:  2,
		},
		{
			name:       "mixed",
			allowedIPs: []string{"192.168.1.1", "10.0.0.0/8", "172.16.0.1"},
			wantCount:  3,
		},
		{
			name:       "with invalid",
			allowedIPs: []string{"192.168.1.1", "invalid", "10.0.0.1"},
			wantCount:  2,
		},
		{
			name:       "IPv6",
			allowedIPs: []string{"::1", "fe80::/10"},
			wantCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServerWithAllowedIPs(m, ":9090", "/metrics", tt.allowedIPs, logger)
			if len(s.allowedIPs) != tt.wantCount {
				t.Errorf("expected %d allowed IPs, got %d", tt.wantCount, len(s.allowedIPs))
			}
		})
	}
}

func TestIsIPAllowed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New()

	s := NewServerWithAllowedIPs(m, ":9090", "/metrics", []string{
		"192.168.1.100",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"::1",
		"fe80::/10",
	}, logger)

	tests := []struct {
		ip      string
		allowed bool
	}{
		{"192.168.1.100", true},
		{"192.168.1.101", false},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"11.0.0.1", false},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.32.0.1", false},
		{"::1", true},
		{"fe80::1", true},
		{"2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("invalid test IP: %s", tt.ip)
			}
			if s.isIPAllowed(ip) != tt.allowed {
				t.Errorf("isIPAllowed(%s) = %v, want %v", tt.ip, !tt.allowed, tt.allowed)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New()
	s := NewServerWithAllowedIPs(m, ":9090", "/metrics", nil, logger)

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expectedIP string
	}{
		{
			name:       "from RemoteAddr with port",
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "from X-Forwarded-For single",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1"},
			expectedIP: "10.0.0.1",
		},
		{
			name:       "from X-Forwarded-For multiple",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1, 192.168.1.1, 127.0.0.1"},
			expectedIP: "10.0.0.1",
		},
		{
			name:       "from X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "172.16.0.1"},
			expectedIP: "172.16.0.1",
		},
		{
			name:       "X-Forwarded-For takes precedence",
			remoteAddr: "127.0.0.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1",
				"X-Real-IP":       "172.16.0.1",
			},
			expectedIP: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/metrics", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := s.getClientIP(req)
			if ip == nil {
				t.Fatal("getClientIP returned nil")
			}
			if ip.String() != tt.expectedIP {
				t.Errorf("getClientIP() = %s, want %s", ip.String(), tt.expectedIP)
			}
		})
	}
}

func TestIPFilterMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New()

	t.Run("no filtering when empty", func(t *testing.T) {
		s := NewServerWithAllowedIPs(m, ":9090", "/metrics", nil, logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/metrics", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()

		s.ipFilterMiddleware(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("allowed IP", func(t *testing.T) {
		s := NewServerWithAllowedIPs(m, ":9090", "/metrics", []string{"192.168.1.0/24"}, logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/metrics", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()

		s.ipFilterMiddleware(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("denied IP", func(t *testing.T) {
		s := NewServerWithAllowedIPs(m, ":9090", "/metrics", []string{"192.168.1.0/24"}, logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/metrics", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()

		s.ipFilterMiddleware(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
		}
	})
}
