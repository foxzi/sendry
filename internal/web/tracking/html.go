package tracking

import (
	"html"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/foxzi/sendry/internal/web/models"
)

// LinkAttr is the attribute that carries the link number in template HTML.
const LinkAttr = "data-sl"

// maxLabelRunes is the maximum length of a link label
const maxLabelRunes = 80

// attr is one parsed attribute of a start tag. Offsets are relative to the tag.
type attr struct {
	name     string // lower-case
	val      string // raw value, quotes removed
	hasVal   bool
	wsStart  int // start of the whitespace preceding the attribute
	end      int // end of the attribute (after closing quote)
	valStart int
	valEnd   int
}

// aTag is a located <a ...> start tag
type aTag struct {
	start, end int // span in the document
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// lowerASCII lower-cases ASCII letters only, keeping byte offsets intact.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// tagEnd returns the index just after the '>' closing the tag that starts at
// i (s[i] == '<'), honouring quoted attribute values; -1 if unterminated.
func tagEnd(s string, i int) int {
	var q byte
	for j := i + 1; j < len(s); j++ {
		c := s[j]
		switch {
		case q != 0:
			if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			// a quote only opens a value right after '='
			k := j - 1
			for k > i && isSpace(s[k]) {
				k--
			}
			if s[k] == '=' {
				q = c
			}
		case c == '>':
			return j + 1
		}
	}
	return -1
}

// findATags returns all <a> start tags outside comments, script and style.
func findATags(s string) []aTag {
	var res []aTag
	low := lowerASCII(s)
	i := 0
	for i < len(s) {
		k := strings.IndexByte(s[i:], '<')
		if k < 0 {
			break
		}
		i += k
		switch {
		case strings.HasPrefix(s[i:], "<!--"):
			e := strings.Index(s[i+4:], "-->")
			if e < 0 {
				return res
			}
			i += 4 + e + 3
		case strings.HasPrefix(low[i:], "<script") || strings.HasPrefix(low[i:], "<style"):
			name := "</script"
			if low[i+2] == 't' {
				name = "</style"
			}
			e := strings.Index(low[i:], name)
			if e < 0 {
				return res
			}
			i += e + len(name)
		case len(s) > i+2 && (s[i+1] == 'a' || s[i+1] == 'A') && (isSpace(s[i+2]) || s[i+2] == '>' || s[i+2] == '/'):
			e := tagEnd(s, i)
			if e < 0 {
				return res
			}
			res = append(res, aTag{i, e})
			i = e
		default:
			i++
		}
	}
	return res
}

// parseAttrs parses the attributes of a start tag (text like `<a href="x">`).
func parseAttrs(tag string) []attr {
	var res []attr
	i := 2 // after "<a"
	n := len(tag)
	for i < n {
		ws := i
		for i < n && (isSpace(tag[i]) || tag[i] == '/') {
			i++
		}
		if i >= n || tag[i] == '>' {
			break
		}
		ns := i
		for i < n && !isSpace(tag[i]) && tag[i] != '=' && tag[i] != '>' && tag[i] != '/' {
			i++
		}
		if i == ns { // stray character, skip it
			i++
			continue
		}
		a := attr{name: lowerASCII(tag[ns:i]), wsStart: ws}
		j := i
		for j < n && isSpace(tag[j]) {
			j++
		}
		if j < n && tag[j] == '=' {
			j++
			for j < n && isSpace(tag[j]) {
				j++
			}
			a.hasVal = true
			if j < n && (tag[j] == '"' || tag[j] == '\'') {
				q := tag[j]
				vs := j + 1
				ve := strings.IndexByte(tag[vs:], q)
				if ve < 0 {
					ve = n - 1 - vs
					if ve < 0 {
						ve = 0
					}
				}
				a.valStart, a.valEnd = vs, vs+ve
				a.val = tag[a.valStart:a.valEnd]
				i = a.valEnd + 1
			} else {
				vs := j
				for j < n && !isSpace(tag[j]) && tag[j] != '>' {
					j++
				}
				a.valStart, a.valEnd = vs, j
				a.val = tag[vs:j]
				i = j
			}
		}
		if i > n {
			i = n
		}
		a.end = i
		res = append(res, a)
	}
	return res
}

func getAttr(attrs []attr, name string) (attr, bool) {
	for _, a := range attrs {
		if a.name == name {
			return a, true
		}
	}
	return attr{}, false
}

// editTag removes attributes mapped to nil and sets those mapped to a value;
// the remaining bytes of the tag are kept as they were.
func editTag(tag string, edits map[string]*string) string {
	attrs := parseAttrs(tag)
	var b strings.Builder
	pos := 0
	done := map[string]bool{}
	for _, a := range attrs {
		v, ok := edits[a.name]
		if !ok {
			continue
		}
		b.WriteString(tag[pos:a.wsStart])
		pos = a.end
		if v == nil || done[a.name] {
			continue
		}
		done[a.name] = true
		b.WriteString(" " + a.name + "=\"" + html.EscapeString(*v) + "\"")
	}
	b.WriteString(tag[pos:])
	out := b.String()
	// append attributes that were not present
	var add strings.Builder
	for _, name := range []string{"href", LinkAttr} {
		if v, ok := edits[name]; ok && v != nil && !done[name] {
			add.WriteString(" " + name + "=\"" + html.EscapeString(*v) + "\"")
		}
	}
	if add.Len() > 0 {
		cut := len(out) - 1 // before '>'
		if cut > 0 && out[cut-1] == '/' {
			cut--
		}
		out = out[:cut] + add.String() + out[cut:]
	}
	return out
}

func strp(s string) *string { return &s }

// isUnsubscribeVar reports whether href is exactly the unsubscribe variable.
func isUnsubscribeVar(href string) bool {
	h := strings.TrimSpace(href)
	if !strings.HasPrefix(h, "{{") || !strings.HasSuffix(h, "}}") {
		return false
	}
	name := strings.TrimSpace(h[2 : len(h)-2])
	return name == "unsubscribe_url" || name == ".unsubscribe_url"
}

// skipHref reports whether a link with this raw href is never tracked.
func skipHref(href string) bool {
	h := strings.TrimSpace(html.UnescapeString(href))
	if h == "" || h[0] == '#' || isUnsubscribeVar(h) {
		return true
	}
	l := strings.ToLower(h)
	for _, p := range []string{"mailto:", "tel:", "sms:", "javascript:"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// mapATags rewrites every <a> start tag with fn, keeping everything else.
func mapATags(s string, fn func(tag string, idx int) string) string {
	tags := findATags(s)
	if len(tags) == 0 {
		return s
	}
	var b strings.Builder
	pos := 0
	for i, t := range tags {
		b.WriteString(s[pos:t.start])
		b.WriteString(fn(s[t.start:t.end], i))
		pos = t.end
	}
	b.WriteString(s[pos:])
	return b.String()
}

// NumberLinks adds data-sl="N" (1-based, in source order) to every <a href> of
// a template's source HTML, before variables are rendered. Skipped: mailto:,
// tel:, sms:, "#..." anchors, links with data-notrack, and links whose href is
// the {{unsubscribe_url}} variable. Existing data-sl attributes are renumbered.
func NumberLinks(src string) string {
	n := 0
	return mapATags(src, func(tag string, _ int) string {
		attrs := parseAttrs(tag)
		href, ok := getAttr(attrs, "href")
		_, noTrack := getAttr(attrs, "data-notrack")
		if !ok || !href.hasVal || noTrack || skipHref(href.val) {
			if _, has := getAttr(attrs, LinkAttr); has {
				return editTag(tag, map[string]*string{LinkAttr: nil})
			}
			return tag
		}
		n++
		return editTag(tag, map[string]*string{LinkAttr: strp(strconv.Itoa(n))})
	})
}

var stripTagsBuf = func(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != '<' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "<!--") {
			e := strings.Index(s[i+4:], "-->")
			if e < 0 {
				break
			}
			i += 4 + e + 3
			continue
		}
		e := tagEnd(s, i)
		if e < 0 {
			break
		}
		b.WriteByte(' ')
		i = e
	}
	return b.String()
}

func cleanLabel(s string) string {
	s = html.UnescapeString(stripTagsBuf(s))
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	r := []rune(s)
	if len(r) > maxLabelRunes {
		s = string(r[:maxLabelRunes])
	}
	return s
}

func looksLikeButton(attrs []attr) bool {
	if c, ok := getAttr(attrs, "class"); ok {
		l := strings.ToLower(c.val)
		if strings.Contains(l, "btn") || strings.Contains(l, "button") {
			return true
		}
	}
	if st, ok := getAttr(attrs, "style"); ok {
		l := strings.ToLower(st.val)
		if strings.Contains(l, "background") ||
			(strings.Contains(l, "padding") && strings.Contains(l, "border-radius")) {
			return true
		}
	}
	return false
}

// ExtractLinks lists the numbered links of HTML produced by NumberLinks, with a
// human label (link text, image alt, or a generic name) and a kind
// (text | image | button).
func ExtractLinks(numberedHTML string) []models.LinkInfo {
	var res []models.LinkInfo
	low := lowerASCII(numberedHTML)
	for _, t := range findATags(numberedHTML) {
		tag := numberedHTML[t.start:t.end]
		attrs := parseAttrs(tag)
		sl, ok := getAttr(attrs, LinkAttr)
		if !ok {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(sl.val))
		if err != nil || id <= 0 {
			continue
		}
		href, _ := getAttr(attrs, "href")
		inner := ""
		if e := strings.Index(low[t.end:], "</a"); e >= 0 {
			inner = numberedHTML[t.end : t.end+e]
		} else {
			inner = numberedHTML[t.end:]
		}
		info := models.LinkInfo{ID: id, URL: href.val}
		if img := findImg(inner); img != nil {
			info.Kind = "image"
			alt := ""
			if a, ok := getAttr(*img, "alt"); ok {
				alt = cleanLabel(a.val)
			}
			if alt == "" {
				alt = "Image"
			}
			info.Label = alt
		} else {
			info.Kind = "text"
			if looksLikeButton(attrs) {
				info.Kind = "button"
			}
			info.Label = cleanLabel(inner)
			if info.Label == "" {
				info.Label = "Link"
			}
		}
		res = append(res, info)
	}
	return res
}

// findImg returns the attributes of the first <img> in s, or nil.
func findImg(s string) *[]attr {
	low := lowerASCII(s)
	i := 0
	for {
		k := strings.Index(low[i:], "<img")
		if k < 0 {
			return nil
		}
		i += k
		if i+4 < len(s) && (isSpace(s[i+4]) || s[i+4] == '/' || s[i+4] == '>') {
			e := tagEnd(s, i)
			if e < 0 {
				e = len(s)
			}
			// parseAttrs skips two bytes ("<a"); pad so "<img" is skipped too
			attrs := parseAttrs("<a" + s[i+4:e])
			return &attrs
		}
		i += 4
	}
}

// UTM holds query parameters appended to destination URLs of tracked links.
// Empty fields are not appended. The sc/sr parameters identify campaign and
// recipient (job item) for order attribution; sl is the link number.
type UTM struct {
	Source   string // utm_source
	Medium   string // utm_medium
	Campaign string // utm_campaign
	SC       string // campaign id
	SR       string // recipient / job item id
	AddSL    bool   // append sl=<link number>
}

// Options controls Apply
type Options struct {
	BaseURL     string // public origin of tracking endpoints, no trailing slash
	Signer      *Signer
	SourceType  string // models.SourceJobItem | models.SourceSend
	SourceID    string
	TrackOpens  bool
	TrackClicks bool
	UTM         *UTM // nil = do not touch destination URLs
}

// addUTM appends the UTM parameters to rawURL, keeping the existing query and
// fragment and never overriding parameters that are already present.
func addUTM(rawURL string, u *UTM, linkID int) string {
	if u == nil {
		return rawURL
	}
	frag := ""
	if i := strings.IndexByte(rawURL, '#'); i >= 0 {
		rawURL, frag = rawURL[:i], rawURL[i:]
	}
	query := ""
	if i := strings.IndexByte(rawURL, '?'); i >= 0 {
		rawURL, query = rawURL[:i], rawURL[i+1:]
	}
	present := map[string]bool{}
	for _, kv := range strings.Split(query, "&") {
		k, _, _ := strings.Cut(kv, "=")
		if uk, err := url.QueryUnescape(k); err == nil {
			k = uk
		}
		present[k] = true
	}
	params := []struct{ k, v string }{
		{"utm_source", u.Source}, {"utm_medium", u.Medium}, {"utm_campaign", u.Campaign},
		{"sc", u.SC}, {"sr", u.SR},
	}
	if u.AddSL {
		params = append(params, struct{ k, v string }{"sl", strconv.Itoa(linkID)})
	}
	for _, p := range params {
		if p.v == "" || present[p.k] {
			continue
		}
		if query != "" {
			query += "&"
		}
		query += p.k + "=" + url.QueryEscape(p.v)
	}
	if query != "" {
		rawURL += "?" + query
	}
	return rawURL + frag
}

func isHTTPURL(s string) bool {
	l := strings.ToLower(s)
	if strings.HasPrefix(l, "http://") {
		return len(l) > len("http://")
	}
	if strings.HasPrefix(l, "https://") {
		return len(l) > len("https://")
	}
	return false
}

// Apply post-processes rendered HTML before it goes to the MTA:
//   - with TrackClicks, each <a data-sl="N" href="URL"> gets href=<BaseURL>/t/c/<token>
//     where the token carries N and URL (with UTM params added if set);
//     links whose rendered href is not http(s) are left as is;
//   - all data-sl attributes are removed (also when click tracking is off);
//   - with TrackOpens, an open pixel <img> is inserted before </body>
//     (or appended when there is no body).
//
// If HTML has no data-sl attributes (e.g. raw API HTML), Apply numbers the
// links itself first so raw HTML can be click-tracked too.
func Apply(src string, opts Options) string {
	active := opts.Signer != nil && opts.BaseURL != ""
	clicks := active && opts.TrackClicks
	if clicks && !hasLinkAttr(src) {
		src = NumberLinks(src)
	}
	out := mapATags(src, func(tag string, _ int) string {
		attrs := parseAttrs(tag)
		sl, ok := getAttr(attrs, LinkAttr)
		if !ok {
			return tag
		}
		edits := map[string]*string{LinkAttr: nil}
		if clicks {
			id, err := strconv.Atoi(strings.TrimSpace(sl.val))
			href, hok := getAttr(attrs, "href")
			if err == nil && id > 0 && hok {
				dest := strings.TrimSpace(html.UnescapeString(href.val))
				if isHTTPURL(dest) && !strings.HasPrefix(dest, opts.BaseURL+"/u/") {
					dest = addUTM(dest, opts.UTM, id)
					tok := opts.Signer.Encode(Payload{
						Kind: KindClick, SourceType: opts.SourceType, SourceID: opts.SourceID,
						LinkID: id, URL: dest,
					})
					if tok != "" {
						edits["href"] = strp(opts.BaseURL + "/t/c/" + tok)
					}
				}
			}
		}
		return editTag(tag, edits)
	})
	if active && opts.TrackOpens {
		if u := PixelURL(opts.BaseURL, opts.Signer, opts.SourceType, opts.SourceID); u != "" {
			pixel := `<img src="` + html.EscapeString(u) + `" width="1" height="1" alt="" style="display:block;width:1px;height:1px;border:0" />`
			if i := strings.LastIndex(lowerASCII(out), "</body"); i >= 0 {
				out = out[:i] + pixel + out[i:]
			} else {
				out += pixel
			}
		}
	}
	return out
}

func hasLinkAttr(s string) bool {
	for _, t := range findATags(s) {
		if _, ok := getAttr(parseAttrs(s[t.start:t.end]), LinkAttr); ok {
			return true
		}
	}
	return false
}

// PixelURL returns <BaseURL>/t/o/<token>.gif
func PixelURL(baseURL string, s *Signer, sourceType, sourceID string) string {
	if s == nil {
		return ""
	}
	tok := s.Encode(Payload{Kind: KindOpen, SourceType: sourceType, SourceID: sourceID})
	if tok == "" {
		return ""
	}
	return baseURL + "/t/o/" + tok + ".gif"
}

// UnsubscribeURL returns <BaseURL>/u/<token>
func UnsubscribeURL(baseURL string, s *Signer, sourceType, sourceID string) string {
	if s == nil {
		return ""
	}
	tok := s.Encode(Payload{Kind: KindUnsubscribe, SourceType: sourceType, SourceID: sourceID})
	if tok == "" {
		return ""
	}
	return baseURL + "/u/" + tok
}

// ListUnsubscribeHeaders returns the RFC 2369 / RFC 8058 headers for url:
// List-Unsubscribe: <url> and List-Unsubscribe-Post: List-Unsubscribe=One-Click
func ListUnsubscribeHeaders(url string) map[string]string {
	return map[string]string{
		"List-Unsubscribe":      "<" + url + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}
}

// ResolveBaseURL picks the tracking origin: the tracking.base_url setting if
// set, otherwise server.public_url. Trailing slashes are trimmed.
func ResolveBaseURL(settingBaseURL, publicURL string) string {
	u := settingBaseURL
	if u == "" {
		u = publicURL
	}
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	return u
}
