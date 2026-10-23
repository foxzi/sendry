package views

import (
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/foxzi/sendry/internal/web/stats"
)

//go:embed *.html
var templatesFS embed.FS

// Standalone templates that don't use the layout
var standaloneTemplates = map[string]bool{
	"login": true,
}

// Template functions
var funcs = template.FuncMap{
	"navSection": navSection,
	"add":        func(a, b int) int { return a + b },
	"sub":        func(a, b int) int { return a - b },
	// pct is the share of a in b, percent; 0 when b is empty
	"pct": func(a, b int) float64 {
		if b <= 0 {
			return 0
		}
		return float64(a) * 100 / float64(b)
	},
	"slice": func(s string, start, end int) string {
		if start >= len(s) {
			return ""
		}
		if end > len(s) {
			end = len(s)
		}
		return s[start:end]
	},
	"toJSON": func(v any) template.JS {
		b, err := json.Marshal(v)
		if err != nil {
			return template.JS("null")
		}
		s := strings.ReplaceAll(string(b), "</", `<\/`)
		return template.JS(s)
	},
	// softBreaks escapes s and allows line breaks after commas, so compact
	// JSON wraps between fields instead of in the middle of a key.
	"softBreaks": func(s string) template.HTML {
		return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), ",", ",<wbr>"))
	},
	// emailBreak escapes an address and lets it wrap before the "@",
	// so a narrow column shows "name" / "@domain" instead of a cut domain.
	"emailBreak": func(s string) template.HTML {
		return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "@", "<wbr>@"))
	},
	// jsonList decodes a JSON string array stored in a text column;
	// anything else comes back as a single item.
	"jsonList": func(s string) []string {
		var list []string
		if err := json.Unmarshal([]byte(s), &list); err != nil {
			return []string{s}
		}
		return list
	},
	"errorType":   stats.ClassifyError,
	"currentYear": func() int { return time.Now().Year() },
}

type Engine struct {
	templates  map[string]*template.Template
	standalone map[string]*template.Template
}

func New() (*Engine, error) {
	e := &Engine{
		templates:  make(map[string]*template.Template),
		standalone: make(map[string]*template.Template),
	}

	// Parse layout with functions
	layoutTmpl, err := template.New("layout.html").Funcs(funcs).ParseFS(templatesFS, "layout.html")
	if err != nil {
		return nil, err
	}

	// Files starting with "_" hold shared definitions: they are parsed into the
	// layout set (so every page sees them) and are not pages themselves.
	if _, err := layoutTmpl.ParseFS(templatesFS, "_*.html"); err != nil {
		return nil, err
	}

	// Parse each page template
	entries, err := fs.ReadDir(templatesFS, ".")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "layout.html" || strings.HasPrefix(entry.Name(), "_") {
			continue
		}

		name := entry.Name()
		baseName := name[:len(name)-len(filepath.Ext(name))]

		// Check if standalone template
		if standaloneTemplates[baseName] {
			tmpl, err := template.New(name).Funcs(funcs).ParseFS(templatesFS, name)
			if err != nil {
				return nil, err
			}
			e.standalone[baseName] = tmpl
			continue
		}

		// Clone layout and parse page template
		tmpl, err := layoutTmpl.Clone()
		if err != nil {
			return nil, err
		}

		_, err = tmpl.ParseFS(templatesFS, name)
		if err != nil {
			return nil, err
		}

		e.templates[baseName] = tmpl
	}

	return e, nil
}

func (e *Engine) Render(w io.Writer, name string, data any) error {
	// Check standalone first
	if tmpl, ok := e.standalone[name]; ok {
		return tmpl.Execute(w, data)
	}

	// Then check layout templates
	if tmpl, ok := e.templates[name]; ok {
		return tmpl.Execute(w, data)
	}

	// Try to parse on the fly
	tmpl, err := template.New(name+".html").Funcs(funcs).ParseFS(templatesFS, name+".html")
	if err != nil {
		return err
	}
	return tmpl.Execute(w, data)
}

// navSection maps a page's Active value to its navigation section id.
// Unknown or non-string values return an empty string.
func navSection(v any) string {
	active, _ := v.(string)
	switch active {
	case "dashboard":
		return "overview"
	case "campaigns", "jobs":
		return "campaigns"
	case "templates", "blocks", "media", "variables":
		return "templates"
	case "recipients":
		return "audience"
	case "sends":
		return "sends"
	case "servers", "queue", "domains", "dkim", "monitoring", "test_email":
		return "infra"
	case "settings", "users", "apikeys", "tracking", "audit":
		return "admin"
	}
	return ""
}
