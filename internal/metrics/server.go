package metrics

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server serves Prometheus metrics over HTTP
type Server struct {
	httpServer *http.Server
	metrics    *Metrics
	addr       string
	path       string
	logger     *slog.Logger
	allowedIPs []*net.IPNet
}

// NewServer creates a new metrics HTTP server
func NewServer(m *Metrics, addr, path string, logger *slog.Logger) *Server {
	return NewServerWithAllowedIPs(m, addr, path, nil, logger)
}

// NewServerWithAllowedIPs creates a new metrics HTTP server with IP filtering
func NewServerWithAllowedIPs(m *Metrics, addr, path string, allowedIPs []string, logger *slog.Logger) *Server {
	if addr == "" {
		addr = ":9090"
	}
	if path == "" {
		path = "/metrics"
	}

	s := &Server{
		metrics: m,
		addr:    addr,
		path:    path,
		logger:  logger,
	}

	// Parse allowed IPs/CIDRs
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
			s.allowedIPs = append(s.allowedIPs, ipNet)
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
			s.allowedIPs = append(s.allowedIPs, &net.IPNet{IP: ip, Mask: mask})
		}
	}

	if len(s.allowedIPs) > 0 {
		logger.Info("metrics IP filtering enabled", "allowed_networks", len(s.allowedIPs))
	}

	return s
}

// ListenAndServe starts the metrics HTTP server
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint with IP filtering
	handler := promhttp.HandlerFor(
		s.metrics.Registry(),
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)
	mux.Handle(s.path, s.ipFilterMiddleware(handler))

	// Health check endpoint (no IP filtering - useful for load balancers)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	s.logger.Info("starting metrics server", "addr", s.addr, "path", s.path)
	return s.httpServer.ListenAndServe()
}

// ipFilterMiddleware checks if the client IP is allowed
func (s *Server) ipFilterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If no IPs configured, allow all
		if len(s.allowedIPs) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := s.getClientIP(r)
		if clientIP == nil {
			s.logger.Warn("could not parse client IP", "remote_addr", r.RemoteAddr)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !s.isIPAllowed(clientIP) {
			s.logger.Warn("metrics access denied", "ip", clientIP.String())
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts the client IP from the request
func (s *Server) getClientIP(r *http.Request) net.IP {
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

// isIPAllowed checks if the IP is in the allowed list
func (s *Server) isIPAllowed(ip net.IP) bool {
	for _, ipNet := range s.allowedIPs {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// Shutdown gracefully shuts down the metrics server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	s.logger.Info("shutting down metrics server")
	return s.httpServer.Shutdown(ctx)
}
