package ipfilter

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs []string
		wantCount  int
	}{
		{
			name:       "empty list",
			allowedIPs: []string{},
			wantCount:  0,
		},
		{
			name:       "single IP",
			allowedIPs: []string{"192.168.1.1"},
			wantCount:  1,
		},
		{
			name:       "CIDR range",
			allowedIPs: []string{"10.0.0.0/8"},
			wantCount:  1,
		},
		{
			name:       "multiple entries",
			allowedIPs: []string{"192.168.1.1", "10.0.0.0/8", "172.16.0.0/12"},
			wantCount:  3,
		},
		{
			name:       "with whitespace",
			allowedIPs: []string{"  192.168.1.1  ", " 10.0.0.0/8 "},
			wantCount:  2,
		},
		{
			name:       "invalid entries ignored",
			allowedIPs: []string{"192.168.1.1", "invalid", "10.0.0.0/8"},
			wantCount:  2,
		},
		{
			name:       "IPv6",
			allowedIPs: []string{"::1", "2001:db8::/32"},
			wantCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(tt.allowedIPs, newTestLogger())
			if f.Count() != tt.wantCount {
				t.Errorf("Count() = %d, want %d", f.Count(), tt.wantCount)
			}
		})
	}
}

func TestFilter_Enabled(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs []string
		want       bool
	}{
		{
			name:       "empty list - disabled",
			allowedIPs: []string{},
			want:       false,
		},
		{
			name:       "with entries - enabled",
			allowedIPs: []string{"192.168.1.1"},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(tt.allowedIPs, newTestLogger())
			if f.Enabled() != tt.want {
				t.Errorf("Enabled() = %v, want %v", f.Enabled(), tt.want)
			}
		})
	}
}

func TestFilter_IsAllowed(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs []string
		testIP     string
		want       bool
	}{
		{
			name:       "empty filter allows all",
			allowedIPs: []string{},
			testIP:     "1.2.3.4",
			want:       true,
		},
		{
			name:       "exact IP match",
			allowedIPs: []string{"192.168.1.1"},
			testIP:     "192.168.1.1",
			want:       true,
		},
		{
			name:       "exact IP no match",
			allowedIPs: []string{"192.168.1.1"},
			testIP:     "192.168.1.2",
			want:       false,
		},
		{
			name:       "CIDR contains",
			allowedIPs: []string{"192.168.0.0/16"},
			testIP:     "192.168.1.100",
			want:       true,
		},
		{
			name:       "CIDR not contains",
			allowedIPs: []string{"192.168.0.0/16"},
			testIP:     "10.0.0.1",
			want:       false,
		},
		{
			name:       "multiple ranges one matches",
			allowedIPs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			testIP:     "172.20.1.1",
			want:       true,
		},
		{
			name:       "IPv6 exact",
			allowedIPs: []string{"::1"},
			testIP:     "::1",
			want:       true,
		},
		{
			name:       "IPv6 CIDR",
			allowedIPs: []string{"2001:db8::/32"},
			testIP:     "2001:db8::1",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(tt.allowedIPs, newTestLogger())
			ip := net.ParseIP(tt.testIP)
			if ip == nil {
				t.Fatalf("failed to parse test IP: %s", tt.testIP)
			}
			if got := f.IsAllowed(ip); got != tt.want {
				t.Errorf("IsAllowed(%s) = %v, want %v", tt.testIP, got, tt.want)
			}
		})
	}
}

func TestFilter_IsAllowedString(t *testing.T) {
	f := New([]string{"192.168.1.0/24"}, newTestLogger())

	if !f.IsAllowedString("192.168.1.50") {
		t.Error("IsAllowedString should allow IP in range")
	}

	if f.IsAllowedString("10.0.0.1") {
		t.Error("IsAllowedString should deny IP outside range")
	}

	if f.IsAllowedString("invalid") {
		t.Error("IsAllowedString should deny invalid IP")
	}
}

func TestFilter_IsAllowedAddr(t *testing.T) {
	f := New([]string{"192.168.1.0/24"}, newTestLogger())

	if !f.IsAllowedAddr("192.168.1.50:8080") {
		t.Error("IsAllowedAddr should allow addr in range")
	}

	if f.IsAllowedAddr("10.0.0.1:8080") {
		t.Error("IsAllowedAddr should deny addr outside range")
	}

	// IP without port
	if !f.IsAllowedAddr("192.168.1.50") {
		t.Error("IsAllowedAddr should handle IP without port")
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		wantIP     string
	}{
		{
			name:       "X-Forwarded-For single",
			xff:        "203.0.113.50",
			remoteAddr: "127.0.0.1:12345",
			wantIP:     "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For chain",
			xff:        "203.0.113.50, 70.41.3.18, 150.172.238.178",
			remoteAddr: "127.0.0.1:12345",
			wantIP:     "203.0.113.50",
		},
		{
			name:       "X-Real-IP",
			xri:        "198.51.100.25",
			remoteAddr: "127.0.0.1:12345",
			wantIP:     "198.51.100.25",
		},
		{
			name:       "X-Forwarded-For takes priority",
			xff:        "203.0.113.50",
			xri:        "198.51.100.25",
			remoteAddr: "127.0.0.1:12345",
			wantIP:     "203.0.113.50",
		},
		{
			name:       "fallback to RemoteAddr",
			remoteAddr: "192.168.1.100:54321",
			wantIP:     "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			ip := GetClientIP(req)
			if ip == nil {
				t.Fatal("GetClientIP returned nil")
			}
			if ip.String() != tt.wantIP {
				t.Errorf("GetClientIP() = %s, want %s", ip.String(), tt.wantIP)
			}
		})
	}
}

func TestFilter_HTTPMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		allowedIPs []string
		clientIP   string
		wantStatus int
	}{
		{
			name:       "empty filter allows all",
			allowedIPs: []string{},
			clientIP:   "1.2.3.4",
			wantStatus: http.StatusOK,
		},
		{
			name:       "allowed IP",
			allowedIPs: []string{"192.168.0.0/16"},
			clientIP:   "192.168.1.100",
			wantStatus: http.StatusOK,
		},
		{
			name:       "denied IP",
			allowedIPs: []string{"192.168.0.0/16"},
			clientIP:   "10.0.0.1",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(tt.allowedIPs, newTestLogger())
			middleware := f.HTTPMiddleware(handler)

			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.clientIP + ":12345"

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}
