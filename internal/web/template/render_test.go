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
