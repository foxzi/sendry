package handlers

import "testing"

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
