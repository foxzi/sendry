package stats

import (
	"regexp"
	"strings"
)

// ErrorType is a delivery error category shown in the panel
type ErrorType string

const (
	ErrorAddressNotFound   ErrorType = "address_not_found"
	ErrorMailboxFull       ErrorType = "mailbox_full"
	ErrorTempUnavailable   ErrorType = "temp_unavailable"
	ErrorDomainUnreachable ErrorType = "domain_unreachable"
	ErrorRejected          ErrorType = "rejected"
	ErrorSpam              ErrorType = "spam"
	ErrorBlocked           ErrorType = "blocked"
	ErrorTechnical         ErrorType = "technical"
	ErrorUnknown           ErrorType = "unknown"
)

// ErrorTypes lists all error types in display order
var ErrorTypes = []ErrorType{
	ErrorAddressNotFound,
	ErrorMailboxFull,
	ErrorTempUnavailable,
	ErrorDomainUnreachable,
	ErrorRejected,
	ErrorSpam,
	ErrorBlocked,
	ErrorTechnical,
	ErrorUnknown,
}

var errorLabels = map[ErrorType]string{
	ErrorAddressNotFound:   "Address does not exist",
	ErrorMailboxFull:       "Mailbox full",
	ErrorTempUnavailable:   "Temporarily unavailable",
	ErrorDomainUnreachable: "Domain unreachable",
	ErrorRejected:          "Rejected by server",
	ErrorSpam:              "Rejected as spam",
	ErrorBlocked:           "Blocked",
	ErrorTechnical:         "Temporary technical error",
	ErrorUnknown:           "Unknown error",
}

// Label returns a human-readable name of the error type
func (t ErrorType) Label() string {
	if label, ok := errorLabels[t]; ok {
		return label
	}
	return string(t)
}

// ParseErrorType returns the error type for a query value, or "" if it is not known
func ParseErrorType(s string) ErrorType {
	if _, ok := errorLabels[ErrorType(s)]; ok {
		return ErrorType(s)
	}
	return ""
}

var (
	// basicCodePattern matches an SMTP reply code, e.g. "RCPT TO x failed: 550 ..."
	basicCodePattern = regexp.MustCompile(`(?:^|\s)([45]\d{2})[\s-]`)
	// enhancedCodePattern matches an RFC 3463 status code, e.g. "5.1.1"; IP addresses are not matched
	enhancedCodePattern = regexp.MustCompile(`(?:^|[\s-])([45])\.(\d{1,3})\.(\d{1,3})(?:[\s;:,)]|$)`)
	blocklistPattern    = regexp.MustCompile(`spamhaus|block ?list|black ?list|dnsbl|\brbl\b|barracuda|spamcop`)
)

// Texts produced by the MTA or the panel itself, without an SMTP reply
var (
	domainUnreachableTexts = []string{"mx lookup failed", "no mx hosts", "no such host", "connection failed to"}
	technicalTexts         = []string{
		"x509", "certificate", "tls:", "starttls",
		"recipient domain rate limit", "smtp client creation failed", "failed to write message data",
		// panel side: API calls, campaign configuration, template rendering
		"do request:", "api error:", "http ", "variant not found", "template not found", "server not found", "render ",
	}
)

var (
	spamTexts     = []string{"spam", "junk", "unsolicited"}
	blockedTexts  = []string{"blocked", "banned", "reputation", "access denied", "not allowed to send", "denied by policy"}
	fullTexts     = []string{"quota", "mailbox full", "mailbox is full", "insufficient storage", "out of storage", "exceeded storage"}
	notFoundTexts = []string{
		"user unknown", "unknown user", "no such user", "unknown recipient", "invalid recipient", "recipient not found",
		"user not found", "mailbox not found", "mailbox unavailable", "mailbox disabled", "account disabled",
		"account has been disabled", "does not exist", "doesn't exist", "no mailbox", "address rejected",
	}
	tempTexts      = []string{"greylist", "graylist", "try again later", "temporarily", "too many connections"}
	transientTexts = []string{"timeout", "timed out", "connection reset", "broken pipe", "eof"}
)

// ClassifyError maps a raw delivery error text to an error type. Empty text means no error and returns "".
func ClassifyError(text string) ErrorType {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lower := strings.ToLower(text)

	// Errors raised before any SMTP reply was received
	if containsAny(lower, domainUnreachableTexts) {
		return ErrorDomainUnreachable
	}
	if containsAny(lower, technicalTexts) {
		return ErrorTechnical
	}

	// Policy rejections are recognized by wording, codes vary between providers.
	// Blocklists go first because "spamhaus" also contains "spam".
	if blocklistPattern.MatchString(lower) {
		return ErrorBlocked
	}
	if containsAny(lower, spamTexts) {
		return ErrorSpam
	}
	if containsAny(lower, blockedTexts) {
		return ErrorBlocked
	}

	if m := enhancedCodePattern.FindStringSubmatch(lower); m != nil {
		class, subject, detail := m[1], m[2], m[3]
		switch {
		case subject == "2" && detail == "2":
			return ErrorMailboxFull
		case class == "5" && subject == "1" && detail == "2", subject == "4":
			return ErrorDomainUnreachable
		case class == "5" && ((subject == "1" && (detail == "0" || detail == "1" || detail == "3" || detail == "6")) ||
			(subject == "2" && detail == "1")):
			return ErrorAddressNotFound
		case class == "4":
			return ErrorTempUnavailable
		default:
			return ErrorRejected
		}
	}

	if containsAny(lower, fullTexts) {
		return ErrorMailboxFull
	}
	if containsAny(lower, notFoundTexts) {
		return ErrorAddressNotFound
	}
	if containsAny(lower, tempTexts) {
		return ErrorTempUnavailable
	}

	if m := basicCodePattern.FindStringSubmatch(lower); m != nil {
		switch code := m[1]; {
		case code == "552":
			return ErrorMailboxFull
		case code == "551" || code == "553":
			return ErrorAddressNotFound
		case code == "421" || code == "450" || code == "451" || code == "452":
			return ErrorTempUnavailable
		case code[0] == '4':
			return ErrorTechnical
		default:
			return ErrorRejected
		}
	}

	if containsAny(lower, transientTexts) {
		return ErrorTechnical
	}

	return ErrorUnknown
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
