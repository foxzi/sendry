package template

import (
	"bytes"
	"fmt"
	htmlTemplate "html/template"
	textTemplate "text/template"
)

// Engine renders templates with data
type Engine struct{}

// NewEngine creates a new template engine
func NewEngine() *Engine {
	return &Engine{}
}

// Render renders a template with provided data
func (e *Engine) Render(tmpl *Template, data map[string]interface{}) (*RenderResult, error) {
	result := &RenderResult{}

	// Render subject (text template)
	subject, err := e.renderText("subject", tmpl.Subject, data)
	if err != nil {
		return nil, fmt.Errorf("failed to render subject: %w", err)
	}
	result.Subject = subject

	// Render HTML (html template with auto-escaping)
	if tmpl.HTML != "" {
		html, err := e.renderHTML("html", tmpl.HTML, data)
		if err != nil {
			return nil, fmt.Errorf("failed to render html: %w", err)
		}
		result.HTML = html
	}

	// Render plain text
	if tmpl.Text != "" {
		text, err := e.renderText("text", tmpl.Text, data)
		if err != nil {
			return nil, fmt.Errorf("failed to render text: %w", err)
		}
		result.Text = text
	}

	return result, nil
}

// Validate checks if template syntax is valid
func (e *Engine) Validate(tmpl *Template) error {
	// Parse subject
	if tmpl.Subject != "" {
		if _, err := textTemplate.New("subject").Parse(tmpl.Subject); err != nil {
			return fmt.Errorf("invalid subject template: %w", err)
		}
	}

	// Parse HTML
	if tmpl.HTML != "" {
		if _, err := htmlTemplate.New("html").Parse(tmpl.HTML); err != nil {
			return fmt.Errorf("invalid html template: %w", err)
		}
	}

	// Parse text
	if tmpl.Text != "" {
		if _, err := textTemplate.New("text").Parse(tmpl.Text); err != nil {
			return fmt.Errorf("invalid text template: %w", err)
		}
	}

	return nil
}

func (e *Engine) renderText(name, tmplStr string, data map[string]interface{}) (string, error) {
	t, err := textTemplate.New(name).Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (e *Engine) renderHTML(name, tmplStr string, data map[string]interface{}) (string, error) {
	t, err := htmlTemplate.New(name).Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
