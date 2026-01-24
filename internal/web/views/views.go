package views

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
)

//go:embed *.html
var templatesFS embed.FS

// Standalone templates that don't use the layout
var standaloneTemplates = map[string]bool{
	"login": true,
}

// Template functions
var funcs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
}

type Engine struct {
	templates   map[string]*template.Template
	standalone  map[string]*template.Template
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

	// Parse each page template
	entries, err := fs.ReadDir(templatesFS, ".")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "layout.html" {
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
	tmpl, err := template.New(name + ".html").Funcs(funcs).ParseFS(templatesFS, name+".html")
	if err != nil {
		return err
	}
	return tmpl.Execute(w, data)
}

// RenderPartial renders a template without layout (for HTMX responses)
func (e *Engine) RenderPartial(w io.Writer, name string, data any) error {
	tmpl, err := template.New(name + ".html").Funcs(funcs).ParseFS(templatesFS, name+".html")
	if err != nil {
		return err
	}
	return tmpl.Execute(w, data)
}
