package headers

import (
	"bytes"
	"strings"
)

// Processor applies header rules to email data
type Processor struct {
	config *Config
}

// NewProcessor creates a new header processor
func NewProcessor(cfg *Config) *Processor {
	return &Processor{config: cfg}
}

// Process applies header rules to email data for a given sender domain
func (p *Processor) Process(data []byte, domain string) []byte {
	if p.config == nil || !p.config.HasRules() {
		return data
	}

	rules := p.config.GetRulesForDomain(domain)
	if len(rules) == 0 {
		return data
	}

	return applyRules(data, rules)
}

// applyRules applies a list of rules to email data
func applyRules(data []byte, rules []Rule) []byte {
	// Split headers and body
	headers, body := splitHeadersBody(data)

	// Parse headers into map (preserving order with slice)
	headerList := parseHeaders(headers)

	// Apply each rule
	for _, rule := range rules {
		headerList = applyRule(headerList, rule)
	}

	// Rebuild email data
	return buildEmail(headerList, body)
}

// header represents a single header with name and value
type header struct {
	name  string
	value string
}

// splitHeadersBody splits email data into headers and body
func splitHeadersBody(data []byte) ([]byte, []byte) {
	// Find empty line that separates headers from body
	// RFC 5322: headers and body are separated by CRLF CRLF or LF LF

	// Try CRLF CRLF first
	if idx := bytes.Index(data, []byte("\r\n\r\n")); idx != -1 {
		return data[:idx], data[idx+4:]
	}

	// Try LF LF
	if idx := bytes.Index(data, []byte("\n\n")); idx != -1 {
		return data[:idx], data[idx+2:]
	}

	// No body found, all headers
	return data, nil
}

// parseHeaders parses raw headers into a list
func parseHeaders(data []byte) []header {
	var headers []header

	lines := bytes.Split(data, []byte("\n"))
	var currentHeader *header

	for _, line := range lines {
		// Remove trailing CR if present
		line = bytes.TrimSuffix(line, []byte("\r"))

		if len(line) == 0 {
			continue
		}

		// Check if this is a continuation line (starts with whitespace)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if currentHeader != nil {
				// Append to current header value
				currentHeader.value += "\r\n" + string(line)
			}
			continue
		}

		// New header line
		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx <= 0 {
			continue
		}

		name := string(bytes.TrimSpace(line[:colonIdx]))
		value := string(bytes.TrimSpace(line[colonIdx+1:]))

		h := header{name: name, value: value}
		headers = append(headers, h)
		currentHeader = &headers[len(headers)-1]
	}

	return headers
}

// applyRule applies a single rule to the header list
func applyRule(headers []header, rule Rule) []header {
	switch rule.Action {
	case ActionRemove:
		return removeHeaders(headers, rule.Headers)
	case ActionReplace:
		return replaceHeader(headers, rule.Header, rule.Value)
	case ActionAdd:
		return addHeader(headers, rule.Header, rule.Value)
	}
	return headers
}

// removeHeaders removes headers matching the given names (case-insensitive)
func removeHeaders(headers []header, names []string) []header {
	if len(names) == 0 {
		return headers
	}

	// Build set of names to remove (lowercase)
	removeSet := make(map[string]bool)
	for _, name := range names {
		removeSet[strings.ToLower(name)] = true
	}

	var result []header
	for _, h := range headers {
		if !removeSet[strings.ToLower(h.name)] {
			result = append(result, h)
		}
	}

	return result
}

// replaceHeader replaces the value of a header (or adds if not exists)
func replaceHeader(headers []header, name, value string) []header {
	if name == "" {
		return headers
	}

	nameLower := strings.ToLower(name)
	found := false

	for i := range headers {
		if strings.ToLower(headers[i].name) == nameLower {
			headers[i].value = value
			found = true
			break // Only replace first occurrence
		}
	}

	if !found {
		// Add as new header if not found
		headers = append(headers, header{name: name, value: value})
	}

	return headers
}

// addHeader adds a new header (always appends, even if exists)
func addHeader(headers []header, name, value string) []header {
	if name == "" {
		return headers
	}
	return append(headers, header{name: name, value: value})
}

// buildEmail reconstructs email data from headers and body
func buildEmail(headers []header, body []byte) []byte {
	var buf bytes.Buffer

	for _, h := range headers {
		buf.WriteString(h.name)
		buf.WriteString(": ")
		buf.WriteString(h.value)
		buf.WriteString("\r\n")
	}

	buf.WriteString("\r\n")

	if len(body) > 0 {
		buf.Write(body)
	}

	return buf.Bytes()
}
