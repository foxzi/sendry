package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	emailtpl "github.com/foxzi/sendry/internal/web/template"
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

func TestBuildSampleDataRanges(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain range",
			input:    `{{range .Items}}{{.Name}}{{end}}`,
			expected: `{"Items":[{"Name":"Sample Name"}]}`,
		},
		{
			name:     "range with element var",
			input:    `{{range $p := .Products}}{{$p.Title}}{{end}}`,
			expected: `{"Products":[{"Title":"Sample Title"}]}`,
		},
		{
			name:     "range with index and element vars",
			input:    `{{range $i, $s := .Sections}}{{if $i}}-{{end}}{{$s.Title}}{{end}}`,
			expected: `{"Sections":[{"Title":"Sample Title"}]}`,
		},
		{
			name:     "nested ranges over element fields",
			input:    `{{range $i, $s := .InfoSections}}{{$s.Title}}{{if $s.Items}}{{range $j, $item := $s.Items}}{{$item}}{{end}}{{end}}{{range $s.Notes}}{{.Text}}{{end}}{{end}}`,
			expected: `{"InfoSections":[{"Items":["Sample Items"],"Notes":[{"Text":"Sample Text"}],"Title":"Sample Title"}]}`,
		},
		{
			name:     "outer var used inside nested range belongs to the outer element",
			input:    `{{range $i, $s := .Sections}}{{range $j, $item := $s.Items}}{{if $s.Ordered}}{{$j}}{{end}}{{$item}}{{end}}{{end}}`,
			expected: `{"Sections":[{"Items":["Sample Items"],"Ordered":"Sample Ordered"}]}`,
		},
		{
			name:     "var with a longer name sharing a prefix is not rewritten",
			input:    `{{range $s := .Sections}}{{$s.Title}}{{range $sx := $s.Rows}}{{$sx.Cell}}{{end}}{{end}}`,
			expected: `{"Sections":[{"Rows":[{"Cell":"Sample Cell"}],"Title":"Sample Title"}]}`,
		},
		{
			name:     "if before range keeps the list",
			input:    `{{if .InfoSections}}{{range $i, $s := .InfoSections}}{{$s.Title}}{{end}}{{end}}`,
			expected: `{"InfoSections":[{"Title":"Sample Title"}]}`,
		},
		{
			name:     "field access on a list does not replace it",
			input:    `{{range .Items}}{{.Name}}{{end}}{{.Items.Count}}`,
			expected: `{"Items":[{"Name":"Sample Name"}]}`,
		},
		{
			name:     "scalar if stays scalar",
			input:    `{{if .ShowPayment}}{{.PaymentMethod}}{{end}}`,
			expected: `{"PaymentMethod":"Sample PaymentMethod","ShowPayment":"Sample ShowPayment"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(buildSampleData(tt.input))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != tt.expected {
				t.Errorf("buildSampleData() = %s, want %s", b, tt.expected)
			}
		})
	}
}

func TestBuildSampleDataRendersConditionalRange(t *testing.T) {
	src := `{{if .InfoSections}}{{range $i, $s := .InfoSections}}<b>{{$s.Title}}</b>{{range $j, $item := $s.Items}}<i>{{$item}}</i>{{end}}{{end}}{{end}}`
	out, err := emailtpl.RenderHTML("t", src, buildSampleData(src))
	if err != nil {
		t.Fatalf("render with sample data: %v", err)
	}
	if !strings.Contains(out, "<b>Sample Title</b><i>Sample Items</i>") {
		t.Errorf("unexpected render: %s", out)
	}
}
