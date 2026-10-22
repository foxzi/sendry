package tracking

import (
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

func isTrusted(ip net.IP, trusted []string) bool {
	if ip == nil {
		return false
	}
	for _, t := range trusted {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if strings.Contains(t, "/") {
			if _, n, err := net.ParseCIDR(t); err == nil && n.Contains(ip) {
				return true
			}
		} else if tip := net.ParseIP(t); tip != nil && tip.Equal(ip) {
			return true
		}
	}
	return false
}

// ClientIP returns the reader's address. Tracking traffic comes through the
// proxying domain, so the direct peer is the proxy: X-Real-IP (set by the proxy
// to the reader address) wins, then the left-most valid X-Forwarded-For entry,
// then the peer itself. Headers are not authenticated; a forged value only
// skews engagement statistics. The port is stripped.
func ClientIP(r *http.Request) string {
	if ip := parseIPLoose(r.Header.Get("X-Real-IP")); ip != nil {
		return ip.String()
	}
	for _, part := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if ip := parseIPLoose(part); ip != nil {
			return ip.String()
		}
	}
	return stripPort(r.RemoteAddr)
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

// IsMachineClick reports whether a click looks automated: a known scanner
// user agent, an empty user agent, or a click less than 2 seconds after the
// message was sent (sentAt may be nil).
func IsMachineClick(userAgent string, sentAt *time.Time, now time.Time) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" || matchesAny(ua, machineUA) {
		return true
	}
	return sentAt != nil && now.Sub(*sentAt) < 2*time.Second
}

// TransparentGIF is a 1x1 transparent GIF image
var TransparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
	0x44, 0x01, 0x00, 0x3b,
}
