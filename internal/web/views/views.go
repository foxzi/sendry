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

type Engine struct {
	templates map[string]*template.Template
}

func New() (*Engine, error) {
	e := &Engine{
		templates: make(map[string]*template.Template),
	}

	// Parse layout
	layoutTmpl, err := template.ParseFS(templatesFS, "layout.html")
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
	tmpl, ok := e.templates[name]
	if !ok {
		// Try to render without layout (like login page)
		tmpl, err := template.ParseFS(templatesFS, name+".html")
		if err != nil {
			return err
		}
		return tmpl.Execute(w, data)
	}
	return tmpl.Execute(w, data)
}

// RenderPartial renders a template without layout (for HTMX responses)
func (e *Engine) RenderPartial(w io.Writer, name string, data any) error {
	tmpl, err := template.ParseFS(templatesFS, name+".html")
	if err != nil {
		return err
	}
	return tmpl.Execute(w, data)
}
