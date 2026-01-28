// Package ipfilter provides IP-based access control for network services
package ipfilter

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// Filter checks if IP addresses are allowed
type Filter struct {
	allowedNets []*net.IPNet
	logger      *slog.Logger
}

// New creates a new IP filter from a list of IPs/CIDRs
// Empty list means allow all
func New(allowedIPs []string, logger *slog.Logger) *Filter {
	f := &Filter{
		logger: logger,
	}

	for _, ipStr := range allowedIPs {
		ipStr = strings.TrimSpace(ipStr)
		if ipStr == "" {
			continue
		}

		// Check if it's a CIDR
		if strings.Contains(ipStr, "/") {
			_, ipNet, err := net.ParseCIDR(ipStr)
			if err != nil {
				logger.Warn("invalid CIDR in allowed_ips", "cidr", ipStr, "error", err)
				continue
			}
			f.allowedNets = append(f.allowedNets, ipNet)
		} else {
			// Single IP - convert to /32 or /128
			ip := net.ParseIP(ipStr)
			if ip == nil {
				logger.Warn("invalid IP in allowed_ips", "ip", ipStr)
				continue
			}
			var mask net.IPMask
			if ip.To4() != nil {
				mask = net.CIDRMask(32, 32)
			} else {
				mask = net.CIDRMask(128, 128)
			}
			f.allowedNets = append(f.allowedNets, &net.IPNet{IP: ip, Mask: mask})
		}
	}

	return f
}

// Enabled returns true if IP filtering is active
func (f *Filter) Enabled() bool {
	return len(f.allowedNets) > 0
}

// Count returns the number of allowed networks
func (f *Filter) Count() int {
	return len(f.allowedNets)
}

// IsAllowed checks if the IP is allowed
// Returns true if filter is empty (allow all) or IP is in allowed list
func (f *Filter) IsAllowed(ip net.IP) bool {
	if len(f.allowedNets) == 0 {
		return true
	}

	for _, ipNet := range f.allowedNets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// IsAllowedString parses and checks if the IP string is allowed
func (f *Filter) IsAllowedString(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return f.IsAllowed(ip)
}

// IsAllowedAddr checks if the address (host:port) is allowed
func (f *Filter) IsAllowedAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Maybe no port?
		return f.IsAllowedString(addr)
	}
	return f.IsAllowedString(host)
}

// GetClientIP extracts the client IP from an HTTP request
// Checks X-Forwarded-For and X-Real-IP headers before RemoteAddr
func GetClientIP(r *http.Request) net.IP {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := net.ParseIP(strings.TrimSpace(parts[0]))
			if ip != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := net.ParseIP(strings.TrimSpace(xri))
		if ip != nil {
			return ip
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// Maybe no port?
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

// HTTPMiddleware returns an HTTP middleware that filters requests by IP
func (f *Filter) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If no IPs configured, allow all
		if !f.Enabled() {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := GetClientIP(r)
		if clientIP == nil {
			f.logger.Warn("could not parse client IP", "remote_addr", r.RemoteAddr)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !f.IsAllowed(clientIP) {
			f.logger.Warn("access denied by IP filter", "ip", clientIP.String(), "path", r.URL.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
