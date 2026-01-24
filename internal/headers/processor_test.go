package headers

import (
	"strings"
	"testing"
)

func TestProcessor_RemoveHeaders(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: Test\r\n" +
		"X-Mailer: MyApp/1.0\r\n" +
		"X-Originating-IP: 192.168.1.1\r\n" +
		"\r\n" +
		"Body text"

	cfg := &Config{
		Global: []Rule{
			{
				Action:  ActionRemove,
				Headers: []string{"X-Mailer", "X-Originating-IP"},
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if strings.Contains(resultStr, "X-Mailer") {
		t.Error("X-Mailer header should be removed")
	}
	if strings.Contains(resultStr, "X-Originating-IP") {
		t.Error("X-Originating-IP header should be removed")
	}
	if !strings.Contains(resultStr, "From: sender@example.com") {
		t.Error("From header should be preserved")
	}
	if !strings.Contains(resultStr, "Subject: Test") {
		t.Error("Subject header should be preserved")
	}
	if !strings.Contains(resultStr, "Body text") {
		t.Error("Body should be preserved")
	}
}

func TestProcessor_ReplaceHeader(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"X-Mailer: OldMailer/1.0\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	cfg := &Config{
		Global: []Rule{
			{
				Action: ActionReplace,
				Header: "X-Mailer",
				Value:  "Sendry/1.0",
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if !strings.Contains(resultStr, "X-Mailer: Sendry/1.0") {
		t.Error("X-Mailer should be replaced with Sendry/1.0")
	}
	if strings.Contains(resultStr, "OldMailer") {
		t.Error("Old X-Mailer value should be replaced")
	}
}

func TestProcessor_ReplaceNonExistent(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	cfg := &Config{
		Global: []Rule{
			{
				Action: ActionReplace,
				Header: "X-Custom",
				Value:  "CustomValue",
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if !strings.Contains(resultStr, "X-Custom: CustomValue") {
		t.Error("Replace should add header if not exists")
	}
}

func TestProcessor_AddHeader(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	cfg := &Config{
		Global: []Rule{
			{
				Action: ActionAdd,
				Header: "X-Processed-By",
				Value:  "Sendry",
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if !strings.Contains(resultStr, "X-Processed-By: Sendry") {
		t.Error("X-Processed-By header should be added")
	}
}

func TestProcessor_DomainSpecificRules(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"X-Internal: secret\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	cfg := &Config{
		Domains: map[string][]Rule{
			"example.com": {
				{
					Action:  ActionRemove,
					Headers: []string{"X-Internal"},
				},
				{
					Action: ActionAdd,
					Header: "X-Company",
					Value:  "Example Inc",
				},
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if strings.Contains(resultStr, "X-Internal") {
		t.Error("X-Internal should be removed for example.com")
	}
	if !strings.Contains(resultStr, "X-Company: Example Inc") {
		t.Error("X-Company should be added for example.com")
	}

	// Test with different domain - rules should not apply
	result2 := p.Process([]byte(email), "other.com")
	resultStr2 := string(result2)

	if !strings.Contains(resultStr2, "X-Internal") {
		t.Error("X-Internal should NOT be removed for other.com")
	}
}

func TestProcessor_GlobalAndDomainRules(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"X-Global-Remove: value\r\n" +
		"X-Domain-Remove: value\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	cfg := &Config{
		Global: []Rule{
			{
				Action:  ActionRemove,
				Headers: []string{"X-Global-Remove"},
			},
		},
		Domains: map[string][]Rule{
			"example.com": {
				{
					Action:  ActionRemove,
					Headers: []string{"X-Domain-Remove"},
				},
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if strings.Contains(resultStr, "X-Global-Remove") {
		t.Error("X-Global-Remove should be removed (global rule)")
	}
	if strings.Contains(resultStr, "X-Domain-Remove") {
		t.Error("X-Domain-Remove should be removed (domain rule)")
	}
}

func TestProcessor_CaseInsensitive(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"x-mailer: MyApp\r\n" +
		"X-CUSTOM: Value\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	cfg := &Config{
		Global: []Rule{
			{
				Action:  ActionRemove,
				Headers: []string{"X-Mailer", "x-custom"},
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if strings.Contains(strings.ToLower(resultStr), "x-mailer") {
		t.Error("X-Mailer should be removed (case insensitive)")
	}
	if strings.Contains(strings.ToLower(resultStr), "x-custom") {
		t.Error("X-Custom should be removed (case insensitive)")
	}
}

func TestProcessor_PreservesBody(t *testing.T) {
	body := "This is the email body.\r\nWith multiple lines.\r\nAnd special chars: <>&"
	email := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		body

	cfg := &Config{
		Global: []Rule{
			{
				Action: ActionAdd,
				Header: "X-Processed",
				Value:  "true",
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	if !strings.Contains(string(result), body) {
		t.Error("Body should be preserved exactly")
	}
}

func TestProcessor_NoRules(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	p := NewProcessor(nil)
	result := p.Process([]byte(email), "example.com")

	if string(result) != email {
		t.Error("With no rules, email should be unchanged")
	}

	p2 := NewProcessor(&Config{})
	result2 := p2.Process([]byte(email), "example.com")

	if string(result2) != email {
		t.Error("With empty config, email should be unchanged")
	}
}

func TestProcessor_MultilineHeaders(t *testing.T) {
	email := "From: sender@example.com\r\n" +
		"Received: from mail.example.com\r\n" +
		"\tby mx.example.com\r\n" +
		"\twith SMTP\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	cfg := &Config{
		Global: []Rule{
			{
				Action:  ActionRemove,
				Headers: []string{"Received"},
			},
		},
	}

	p := NewProcessor(cfg)
	result := p.Process([]byte(email), "example.com")

	resultStr := string(result)

	if strings.Contains(resultStr, "Received") {
		t.Error("Received header (with continuations) should be removed")
	}
	if strings.Contains(resultStr, "mx.example.com") {
		t.Error("Received header continuation should be removed")
	}
	if !strings.Contains(resultStr, "Subject: Test") {
		t.Error("Subject should be preserved")
	}
}

func TestConfig_HasRules(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{"nil config", nil, false},
		{"empty config", &Config{}, false},
		{"global rules only", &Config{Global: []Rule{{Action: ActionAdd}}}, true},
		{"domain rules only", &Config{Domains: map[string][]Rule{"example.com": {{Action: ActionAdd}}}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.HasRules() != tt.expected {
				t.Errorf("HasRules() = %v, want %v", tt.config.HasRules(), tt.expected)
			}
		})
	}
}
