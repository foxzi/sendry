package template

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"regexp"
	"strconv"
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
	protected, markers := protectConditionalComments(src)
	t, err := htmltpl.New(name).Option("missingkey=zero").Parse(Preprocess(protected))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", name, err)
	}
	data = ensureNestedKeys(src, data)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute %s: %w", name, err)
	}
	return restoreConditionalComments(buf.String(), markers), nil
}

// html/template drops HTML comments, and with them Outlook conditional
// comments (<!--[if mso]> ... <![endif]-->) that carry bulletproof buttons.
// Their markers are swapped for placeholders before parsing and put back
// after rendering: literal template text passes through unchanged, while
// the markup between the markers is still rendered and escaped as usual.
// Plain comments are still dropped.
const (
	ccOpen  = "\ue000"
	ccClose = "\ue001"
)

func ccPlaceholder(i int) string { return ccOpen + strconv.Itoa(i) + ccClose }

// protectConditionalComments replaces the markers of conditional comments
// with placeholders and returns the markers in placeholder order.
func protectConditionalComments(src string) (string, []string) {
	if !strings.Contains(src, "<!--") {
		return src, nil
	}
	var b strings.Builder
	var markers []string
	keep := func(m string) {
		b.WriteString(ccPlaceholder(len(markers)))
		markers = append(markers, m)
	}
	i := 0
	for {
		k := strings.Index(src[i:], "<!--")
		if k < 0 {
			break
		}
		start := i + k
		e := strings.Index(src[start+4:], "-->")
		if e < 0 {
			break
		}
		bodyStart, bodyEnd, end := start+4, start+4+e, start+4+e+3
		body := src[bodyStart:bodyEnd]
		lower := strings.ToLower(body)
		switch {
		case strings.HasPrefix(lower, "[if") && strings.HasSuffix(body, "<!"),
			strings.HasPrefix(lower, "<![endif]"):
			// downlevel-revealed <!--[if !mso]><!--> and its <!--<![endif]-->
			b.WriteString(src[i:start])
			keep(src[start:end])
		case strings.HasPrefix(lower, "[if") && strings.HasSuffix(lower, "<![endif]"):
			// downlevel-hidden <!--[if mso]> markup <![endif]-->
			gt := strings.IndexByte(body, '>')
			closeAt := bodyEnd - len("<![endif]")
			if gt < 0 || bodyStart+gt+1 > closeAt {
				b.WriteString(src[i:end])
				break
			}
			b.WriteString(src[i:start])
			keep(src[start : bodyStart+gt+1])
			b.WriteString(src[bodyStart+gt+1 : closeAt])
			keep(src[closeAt:end])
		default:
			b.WriteString(src[i:end])
		}
		i = end
	}
	b.WriteString(src[i:])
	return b.String(), markers
}

func restoreConditionalComments(out string, markers []string) string {
	for i := len(markers) - 1; i >= 0; i-- {
		out = strings.Replace(out, ccPlaceholder(i), markers[i], 1)
	}
	return out
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
