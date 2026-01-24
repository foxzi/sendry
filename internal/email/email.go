// Package email provides common email utility functions.
package email

import (
	"net/mail"
	"strings"
)

// ExtractDomain extracts the domain part from an email address.
// Returns empty string if the email is invalid.
func ExtractDomain(email string) string {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		// Try simple extraction for malformed addresses
		at := strings.LastIndex(email, "@")
		if at <= 0 || at == len(email)-1 {
			return ""
		}
		return strings.ToLower(email[at+1:])
	}
	at := strings.LastIndex(addr.Address, "@")
	if at <= 0 || at == len(addr.Address)-1 {
		return ""
	}
	return strings.ToLower(addr.Address[at+1:])
}

// ExtractDomainOrDefault extracts the domain part from an email address.
// Returns the provided default value if the email is invalid or domain is empty.
func ExtractDomainOrDefault(email, defaultDomain string) string {
	domain := ExtractDomain(email)
	if domain == "" {
		return defaultDomain
	}
	return domain
}
