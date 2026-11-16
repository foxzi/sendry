package template

import (
	"strings"
	"testing"
)

func TestPreprocess(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain var", "Hello, {{Name}}!", "Hello, {{ .Name }}!"},
		{"snake_case var", "Hello, {{first_name}}", "Hello, {{ .first_name }}"},
		{"already dotted", "{{.Name}}", "{{ .Name }}"},
		{"$-binding", "{{$x.Name}}", "{{ $x.Name }}"},
		{"end action", "{{end}}", "{{ end }}"},
		{"range rewrites arg", "{{range Items}}{{.Name}}{{end}}", "{{ range .Items }}{{ .Name }}{{ end }}"},
		{"range already dotted", "{{range .Items}}{{.Name}}{{end}}", "{{ range .Items }}{{ .Name }}{{ end }}"},
		{"if rewrites", "{{if HasBonus}}!!!{{end}}", "{{ if .HasBonus }}!!!{{ end }}"},
		{"with rewrites", "{{with Order}}{{.ID}}{{end}}", "{{ with .Order }}{{ .ID }}{{ end }}"},
		{"pipeline preserves rest", "{{Name | upper}}", "{{ .Name | upper }}"},
		{"builtin not dotted", "{{len Items}}", "{{ len .Items }}"},
		{"trim markers", "{{- Name -}}", "{{- .Name -}}"},
		{"keep literal", `{{"hello"}}`, `{{ "hello" }}`},
		{"empty action stays", "{{}}", "{{}}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Preprocess(tc.in)
			if got != tc.want {
				t.Errorf("Preprocess(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderHTML_Flat(t *testing.T) {
	out, err := RenderHTML("t", "<p>Привет, {{Name}}!</p>", map[string]any{"Name": "Иван"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "<p>Привет, Иван!</p>" {
		t.Errorf("got %q", out)
	}
}

func TestRenderHTML_MissingKeyEmpty(t *testing.T) {
	out, err := RenderHTML("t", "Hello {{Missing}}!", map[string]any{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "Hello !" {
		t.Errorf("got %q", out)
	}
}

func TestRenderHTML_Range(t *testing.T) {
	tmpl := `{{range Items}}<li>{{.Name}}: {{.Price}}</li>{{end}}`
	data := map[string]any{
		"Items": []map[string]any{
			{"Name": "A", "Price": "10"},
			{"Name": "B", "Price": "20"},
		},
	}
	out, err := RenderHTML("t", tmpl, data)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "<li>A: 10</li><li>B: 20</li>"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestRenderHTML_If(t *testing.T) {
	tmpl := `{{if Bonus}}YES{{else}}NO{{end}}`
	out, _ := RenderHTML("t", tmpl, map[string]any{"Bonus": true})
	if out != "YES" {
		t.Errorf("got %q", out)
	}
	out, _ = RenderHTML("t", tmpl, map[string]any{})
	if out != "NO" {
		t.Errorf("got %q", out)
	}
}

func TestRenderHTML_NestedSections(t *testing.T) {
	tmpl := `
{{if Items}}<h1>Заказ</h1>{{range Items}}<p>{{.Name}}</p>{{end}}{{end}}
{{if Bonus}}<h2>Бонус</h2>{{range Bonus}}<p>{{.Name}}</p>{{end}}{{end}}`
	data := map[string]any{
		"Items": []map[string]any{{"Name": "Item1"}},
		// Bonus отсутствует → секция не рендерится
	}
	out, err := RenderHTML("t", tmpl, data)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "<h1>Заказ</h1>") {
		t.Errorf("expected h1, got %q", out)
	}
	if strings.Contains(out, "<h2>Бонус</h2>") {
		t.Errorf("expected no bonus section, got %q", out)
	}
}

func TestRenderText_Subject(t *testing.T) {
	out, err := RenderText("subj", "Заказ №{{OrderNumber}}", map[string]any{"OrderNumber": "256863"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "Заказ №256863" {
		t.Errorf("got %q", out)
	}
}

func TestRenderHTML_LegacyHTMLContent(t *testing.T) {
	tmpl := `<a href="{{TrackingURL}}">{{TrackingNumber}}</a>`
	data := map[string]any{"TrackingURL": "https://t/123", "TrackingNumber": "123"}
	out, err := RenderHTML("t", tmpl, data)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "123") {
		t.Errorf("got %q", out)
	}
}

// Outlook conditional comments survive rendering (html/template drops
// comments); the markup inside them is still rendered and escaped.
func TestRenderHTML_KeepsConditionalComments(t *testing.T) {
	src := `<!--[if mso]><v:roundrect href="{{url}}"><center>{{name}}</center></v:roundrect><![endif]-->` +
		`<!--[if !mso]><!--><a href="{{url}}">Go</a><!--<![endif]-->` +
		`<!-- plain comment -->`
	out, err := RenderHTML("t", src, map[string]any{"url": "https://x.com/?a=1&b=2", "name": "<b>"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<!--[if mso]><v:roundrect href="https://x.com/?a=1&amp;b=2"><center>&lt;b&gt;</center></v:roundrect><![endif]-->`,
		`<!--[if !mso]><!--><a href="https://x.com/?a=1&amp;b=2">Go</a><!--<![endif]-->`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if strings.Contains(out, "plain comment") || strings.ContainsAny(out, "") {
		t.Errorf("plain comment kept or placeholder left: %s", out)
	}
}

func TestInsertPreheader(t *testing.T) {
	const open = `<div style="display:none;font-size:1px;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;mso-hide:all;">`
	cases := []struct {
		name, html, text string
		wantPrefix       string // html expected before the block
		wantText         string // escaped text inside the block, "" = unchanged html
		wantSuffix       string // html expected after the block
	}{
		{"empty", "<body>x</body>", "", "", "", ""},
		{"blank", "<body>x</body>", "  \n\t", "", "", ""},
		{"with body", "<html><body><p>x</p></body></html>", "Hello", "<html><body>", "Hello", "<p>x</p></body></html>"},
		{"uppercase body with attributes", `<HTML><BODY class="a" style="margin:0"><p>x</p></BODY></HTML>`, "Hi", `<HTML><BODY class="a" style="margin:0">`, "Hi", "<p>x</p></BODY></HTML>"},
		{"no body", "<p>x</p>", "Hi", "", "Hi", "<p>x</p>"},
		{"escaping", "<body></body>", `<b>"A" & 'B'</b>`, "<body>", "&lt;b&gt;&#34;A&#34; &amp; &#39;B&#39;&lt;/b&gt;", "</body>"},
		{"not a body-prefixed tag", "<bodyguard>x</bodyguard>", "Hi", "", "Hi", "<bodyguard>x</bodyguard>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InsertPreheader(tc.html, tc.text)
			if tc.wantText == "" {
				if got != tc.html {
					t.Fatalf("html must stay unchanged, got %q", got)
				}
				return
			}
			if !strings.HasPrefix(got, tc.wantPrefix+open+tc.wantText+"&#847;&zwnj;&nbsp;") {
				t.Fatalf("prefix/block mismatch: %q", got)
			}
			if !strings.HasSuffix(got, "</div>"+tc.wantSuffix) {
				t.Fatalf("suffix mismatch: %q", got)
			}
			if n := strings.Count(got, "&#847;&zwnj;&nbsp;"); n < 60 || n > 90 {
				t.Errorf("filler repeated %d times", n)
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
			got := StripDarkModeCSS(tt.input)
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

func TestRenderHTML_StripsWrapperDarkModeCSS(t *testing.T) {
	src := `<style>.a{color:#000000;}@media (prefers-color-scheme: dark) {
      body, .force-page-bg { background-color: #1C1C1E !important; }
      .email-container [style*="color:#000000"] { color: #E4E4E4 !important; }
    }</style><p style="color:#000000;">{{Name}}</p>`
	out, err := RenderHTML("t", src, map[string]any{"Name": "Иван"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.Contains(out, "prefers-color-scheme") || strings.Contains(out, "#E4E4E4") {
		t.Errorf("wrapper dark-mode CSS left in output: %s", out)
	}
	if !strings.Contains(out, ".a{color:#000000;}") || !strings.Contains(out, "Иван") {
		t.Errorf("unexpected render: %s", out)
	}
}
