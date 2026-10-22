package tracking

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func stripPort(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return strings.Trim(addr, "[]")
}

// parseIPLoose parses an IP with an optional port or brackets.
func parseIPLoose(s string) net.IP {
	s = strings.TrimSpace(s)
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	return net.ParseIP(stripPort(s))
}

// TrustedProxies is a parsed list of reverse proxy addresses (server.trusted_proxies)
type TrustedProxies []*net.IPNet

// ParseTrustedProxies parses IPs and CIDRs; a bare IP is a single-host network.
func ParseTrustedProxies(list []string) (TrustedProxies, error) {
	var out TrustedProxies
	for _, t := range list {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !strings.Contains(t, "/") {
			ip := net.ParseIP(t)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy %q", t)
			}
			bits := 128
			if ip4 := ip.To4(); ip4 != nil {
				ip, bits = ip4, 32
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(t)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", t, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func (t TrustedProxies) contains(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range t {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP returns the reader's address. X-Real-IP and X-Forwarded-For are
// honoured only when the direct peer is a trusted proxy (otherwise anyone
// could forge them): X-Real-IP wins unless it names a trusted proxy itself
// (an inner proxy of a chain overwrote it), then the right-most
// X-Forwarded-For entry that is not a trusted proxy. Without a trusted peer
// the peer address is returned. The port is stripped.
func (t TrustedProxies) ClientIP(r *http.Request) string {
	peer := stripPort(r.RemoteAddr)
	if !t.contains(net.ParseIP(peer)) {
		return peer
	}
	if ip := parseIPLoose(r.Header.Get("X-Real-IP")); ip != nil && !t.contains(ip) {
		return ip.String()
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := parseIPLoose(parts[i])
		if ip == nil {
			continue
		}
		if !t.contains(ip) || i == 0 {
			return ip.String()
		}
	}
	return peer
}

var machineUA = []string{
	"bot", "crawler", "spider", "python-requests", "go-http-client", "curl/", "wget",
	"headlesschrome", "barracuda", "proofpoint", "mimecast", "symantec", "forcepoint",
	"trendmicro", "microsoft office existence discovery", "linkpreview", "slackbot",
	"telegrambot", "whatsapp", "facebookexternalhit", "skypeuripreview",
}

var imageProxyUA = []string{"googleimageproxy", "yandeximageresizer", "ggpht.com", "mail.ru"}

func matchesAny(ua string, list []string) bool {
	for _, s := range list {
		if strings.Contains(ua, s) {
			return true
		}
	}
	return false
}

// IsMachineOpen reports whether an open was made by software rather than a
// person (link/security scanners, uptime bots). Image proxies of mail
// providers (GoogleImageProxy, YandexImageResizer, Apple) are NOT machine
// opens: they fetch the image when the user opens the message.
func IsMachineOpen(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if matchesAny(ua, imageProxyUA) {
		return false
	}
	return matchesAny(ua, machineUA)
}

// MachineClickWindow: a click this soon after the message was handed to the
// MTA is made by a link scanner at the receiving side, not by a person.
const MachineClickWindow = 30 * time.Second

// IsMachineClick reports whether a click looks automated: a known scanner
// user agent, an empty user agent, or a click less than MachineClickWindow
// after the message was handed to the MTA (sentAt may be nil).
func IsMachineClick(userAgent string, sentAt *time.Time, now time.Time) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" || matchesAny(ua, machineUA) {
		return true
	}
	return sentAt != nil && now.Sub(*sentAt) < MachineClickWindow
}

// TransparentGIF is a 1x1 transparent GIF image
var TransparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
	0x44, 0x01, 0x00, 0x3b,
}
