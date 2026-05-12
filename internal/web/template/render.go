package template

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"regexp"
	"strings"
	texttpl "text/template"
)

var actionRe = regexp.MustCompile(`\{\{[-]?\s*([\s\S]*?)\s*[-]?\}\}`)

var firstWordRe = regexp.MustCompile(`^\S+`)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var keywords = map[string]struct{}{
	"if":       {},
	"else":     {},
	"end":      {},
	"range":    {},
	"with":     {},
	"define":   {},
	"template": {},
	"block":    {},
	"break":    {},
	"continue": {},
}

func Preprocess(src string) string {
	return actionRe.ReplaceAllStringFunc(src, func(action string) string {
		open, close := "{{", "}}"
		body := action[2 : len(action)-2]
		if strings.HasPrefix(body, "-") {
			open = "{{-"
			body = body[1:]
		}
		if strings.HasSuffix(body, "-") {
			close = "-}}"
			body = body[:len(body)-1]
		}
		body = strings.TrimSpace(body)
		if body == "" {
			return action
		}

		first := firstToken(body)
		_, isKw := keywords[first]
		_, isBuiltin := builtins[first]
		if isKw || isBuiltin {
			rest := strings.TrimSpace(body[len(first):])
			rest = rewriteFirstIdent(rest)
			out := first
			if rest != "" {
				out = first + " " + rest
			}
			return open + " " + out + " " + close
		}

		return open + " " + rewriteFirstIdent(body) + " " + close
	})
}

func rewriteFirstIdent(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return expr
	}
	first := firstToken(expr)
	rest := expr[len(first):]

	switch first[0] {
	case '.', '$', '"', '\'', '`', '(', '!':
		return expr
	}
	if _, ok := keywords[first]; ok {
		return expr
	}
	if _, ok := builtins[first]; ok {
		return expr
	}

	if identRe.MatchString(first) {
		return "." + first + rest
	}
	return expr
}

var builtins = map[string]struct{}{
	"and":      {},
	"call":     {},
	"html":     {},
	"index":    {},
	"slice":    {},
	"js":       {},
	"len":      {},
	"not":      {},
	"or":       {},
	"print":    {},
	"printf":   {},
	"println":  {},
	"urlquery": {},
	"eq":       {},
	"ne":       {},
	"lt":       {},
	"le":       {},
	"gt":       {},
	"ge":       {},
}

func firstToken(s string) string {
	m := firstWordRe.FindString(s)
	return m
}

func RenderHTML(name, src string, data map[string]any) (string, error) {
	if src == "" {
		return "", nil
	}
	t, err := htmltpl.New(name).Option("missingkey=zero").Parse(Preprocess(src))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", name, err)
	}
	data = ensureNestedKeys(src, data)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute %s: %w", name, err)
	}
	return buf.String(), nil
}

func ensureNestedKeys(src string, data map[string]any) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = v
	}
	for _, m := range nestedPathRE.FindAllStringSubmatch(src, -1) {
		path := m[1]
		if !strings.Contains(path, ".") {
			continue
		}
		parts := strings.Split(path, ".")
		cur := out
		for i := 0; i < len(parts)-1; i++ {
			next, ok := cur[parts[i]].(map[string]any)
			if !ok {
				next = map[string]any{}
				cur[parts[i]] = next
			}
			cur = next
		}
	}
	return out
}

var nestedPathRE = regexp.MustCompile(`\{\{[^}]*?\.([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+)`)

func ValidateHTML(name, src string) error {
	if src == "" {
		return nil
	}
	if _, err := htmltpl.New(name).Option("missingkey=zero").Parse(Preprocess(src)); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}

func RenderText(name, src string, data map[string]any) (string, error) {
	if src == "" {
		return "", nil
	}
	t, err := texttpl.New(name).Option("missingkey=zero").Parse(Preprocess(src))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute %s: %w", name, err)
	}
	return buf.String(), nil
}
