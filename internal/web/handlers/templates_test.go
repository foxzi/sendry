package handlers

import (
	"strings"
	"testing"
)

func TestConvertToGoTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple variable",
			input:    "Hello {{name}}!",
			expected: "Hello {{.name}}!",
		},
		{
			name:     "multiple variables",
			input:    "Hello {{name}}, your order {{order_id}} is ready",
			expected: "Hello {{.name}}, your order {{.order_id}} is ready",
		},
		{
			name:     "already has dot",
			input:    "Hello {{.name}}!",
			expected: "Hello {{.name}}!",
		},
		{
			name:     "mixed syntax",
			input:    "Hello {{.name}}, your code is {{promo_code}}",
			expected: "Hello {{.name}}, your code is {{.promo_code}}",
		},
		{
			name:     "with whitespace",
			input:    "Hello {{ name }}!",
			expected: "Hello {{ .name }}!",
		},
		{
			name:     "empty template",
			input:    "",
			expected: "",
		},
		{
			name:     "no variables",
			input:    "Hello World!",
			expected: "Hello World!",
		},
		{
			name:     "skip if keyword",
			input:    "{{if .condition}}yes{{end}}",
			expected: "{{if .condition}}yes{{end}}",
		},
		{
			name:     "skip range keyword",
			input:    "{{range .items}}{{.}}{{end}}",
			expected: "{{range .items}}{{.}}{{end}}",
		},
		{
			name:     "skip else keyword",
			input:    "{{if .x}}a{{else}}b{{end}}",
			expected: "{{if .x}}a{{else}}b{{end}}",
		},
		{
			name:     "skip pipe functions",
			input:    "{{name | html}}",
			expected: "{{name | html}}",
		},
		{
			name:     "skip dollar variable",
			input:    "{{$x := .name}}{{$x}}",
			expected: "{{$x := .name}}{{$x}}",
		},
		{
			name:     "html with variables",
			input:    "<p>Hello {{name}}</p><a href=\"{{link}}\">Click</a>",
			expected: "<p>Hello {{.name}}</p><a href=\"{{.link}}\">Click</a>",
		},
		{
			name:     "subject line",
			input:    "Order {{order_id}} - Thank you {{name}}!",
			expected: "Order {{.order_id}} - Thank you {{.name}}!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToGoTemplate(tt.input)
			if result != tt.expected {
				t.Errorf("convertToGoTemplate(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStripDarkModeCSS(t *testing.T) {
	wrapperBlock := `@media (prefers-color-scheme: dark) {
      body, .force-page-bg {
        background-color: #1C1C1E !important;
      }
      .email-container {
        background-color: #2C2C2E !important;
      }
    }`

	tests := []struct {
		name        string
		input       string
		wantContain []string
		wantMissing []string
	}{
		{
			name:        "removes wrapper dark-mode block",
			input:       "<style>.a{color:red;}" + wrapperBlock + ".b{color:blue;}</style>",
			wantContain: []string{".a{color:red;}", ".b{color:blue;}"},
			wantMissing: []string{"force-page-bg", "prefers-color-scheme"},
		},
		{
			name: "preserves other dark-mode blocks without wrapper marker",
			input: `<style>@media (prefers-color-scheme: dark) {
      .custom-block { color: #fff; }
    }</style>`,
			wantContain: []string{"prefers-color-scheme", ".custom-block"},
		},
		{
			name:        "no dark-mode block leaves html untouched",
			input:       "<style>.a{color:red;}</style>",
			wantContain: []string{"<style>.a{color:red;}</style>"},
		},
		{
			name:        "empty input",
			input:       "",
			wantContain: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDarkModeCSS(tt.input)
			for _, s := range tt.wantContain {
				if !strings.Contains(got, s) {
					t.Errorf("expected result to contain %q, got %q", s, got)
				}
			}
			for _, s := range tt.wantMissing {
				if strings.Contains(got, s) {
					t.Errorf("expected result to NOT contain %q, got %q", s, got)
				}
			}
		})
	}
}
