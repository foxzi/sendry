package tracking

import (
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

const uid = "0b7e3c1a-5d2f-4c8e-9a41-7f6d2e8b9c10"

func TestTokenRoundTrip(t *testing.T) {
	s := NewSigner([]byte("secret"))
	cases := []Payload{
		{Kind: KindOpen, SourceType: models.SourceJobItem, SourceID: uid},
		{Kind: KindUnsubscribe, SourceType: models.SourceSend, SourceID: uid},
		{Kind: KindClick, SourceType: models.SourceJobItem, SourceID: uid, LinkID: 7, URL: "https://example.com/a?b=c&d=e#f"},
		{Kind: KindClick, SourceType: models.SourceSend, SourceID: "not-a-uuid", LinkID: 300, URL: "https://e.com/ü"},
		{Kind: KindOpen, SourceType: models.SourceSend, SourceID: ""},
		{Kind: KindClick, SourceType: models.SourceSend, SourceID: strings.ToUpper(uid), LinkID: 1, URL: ""},
	}
	for _, p := range cases {
		tok := s.Encode(p)
		if tok == "" || strings.ContainsAny(tok, "+/=") {
			t.Fatalf("bad token %q", tok)
		}
		got, err := s.Decode(tok)
		if err != nil || got != p {
			t.Errorf("roundtrip %+v -> %+v, %v", p, got, err)
		}
	}
	if len(s.Encode(cases[0])) > 60 {
		t.Errorf("open token too long: %d", len(s.Encode(cases[0])))
	}
}

func TestTokenInvalid(t *testing.T) {
	s := NewSigner([]byte("secret"))
	if s.Encode(Payload{Kind: 'x', SourceType: models.SourceSend}) != "" ||
		s.Encode(Payload{Kind: KindOpen, SourceType: "zzz"}) != "" {
		t.Error("expected empty token for invalid payload")
	}
	tok := s.Encode(Payload{Kind: KindClick, SourceType: models.SourceSend, SourceID: uid, LinkID: 2, URL: "https://x.y"})
	other := NewSigner([]byte("other"))
	if _, err := other.Decode(tok); err == nil {
		t.Error("wrong key accepted")
	}
	// tampered payload
	b := []byte(tok)
	b[2] ^= 1
	if _, err := s.Decode(string(b)); err == nil {
		t.Error("tampered accepted")
	}
	// truncations
	for i := 0; i < len(tok); i++ {
		if _, err := s.Decode(tok[:i]); err == nil {
			t.Errorf("truncated at %d accepted", i)
		}
	}
	// correctly signed but malformed payloads
	for _, raw := range [][]byte{{}, {'o'}, {'o', 'j'}, {'x', 'j', 0, 0}, {'o', 'q', 0, 0}, {'o', 'j', 1, 1, 2}, {'o', 'j', 0, 200}, {'o', 'j', 9, 0}, {'c', 'j', 0, 0}, {'o', 'j', 0, 0, 5}} {
		tk := b64.EncodeToString(raw) + "." + b64.EncodeToString(s.sign(raw))
		if _, err := s.Decode(tk); err == nil {
			t.Errorf("malformed payload %v accepted", raw)
		}
	}
}

func TestTokenGarbage(t *testing.T) {
	s := NewSigner([]byte("secret"))
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 5000; i++ {
		b := make([]byte, rnd.Intn(80))
		rnd.Read(b)
		_, _ = s.Decode(string(b))
		_, _ = s.Decode(b64.EncodeToString(b) + "." + b64.EncodeToString(b))
	}
	for _, g := range []string{"", ".", "..", "a.b.c", "!!.!!", "AAAA.", ".AAAA"} {
		if _, err := s.Decode(g); err == nil {
			t.Errorf("%q accepted", g)
		}
	}
	var nilS *Signer
	if _, err := nilS.Decode("a.b"); err == nil {
		t.Error("nil signer accepted")
	}
}

func TestNumberLinks(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"basic", `<a href="https://a.com">A</a> <A HREF='https://b.com' class="x">B</A>`,
			`<a href="https://a.com" data-sl="1">A</a> <A HREF='https://b.com' class="x" data-sl="2">B</A>`},
		{"skipped", `<a href="mailto:x@y.z">m</a><a href="tel:1">t</a><a href="sms:1">s</a><a href="#top">h</a><a name="x">n</a><a href="https://a.com" data-notrack>nt</a><a href="{{unsubscribe_url}}">u</a><a href=" {{ .unsubscribe_url }} ">u2</a><a href="https://z.com">z</a>`,
			`<a href="mailto:x@y.z">m</a><a href="tel:1">t</a><a href="sms:1">s</a><a href="#top">h</a><a name="x">n</a><a href="https://a.com" data-notrack>nt</a><a href="{{unsubscribe_url}}">u</a><a href=" {{ .unsubscribe_url }} ">u2</a><a href="https://z.com" data-sl="1">z</a>`},
		{"renumber", `<a data-sl="9" href="https://a.com">A</a><a href="mailto:a@b.c" data-sl="4">M</a><a href="https://b.com" data-sl="1">B</a>`,
			`<a data-sl="1" href="https://a.com">A</a><a href="mailto:a@b.c">M</a><a href="https://b.com" data-sl="2">B</a>`},
		{"variables", `<a href="{{product_url}}">p</a><a href="{{.Url}}?x=1">q</a>`,
			`<a href="{{product_url}}" data-sl="1">p</a><a href="{{.Url}}?x=1" data-sl="2">q</a>`},
		{"comments and mso", `<!--[if mso]><a href="https://hidden.com">h</a><![endif]--><a href="https://a.com">A</a><style>a{}</style>`,
			`<!--[if mso]><a href="https://hidden.com">h</a><![endif]--><a href="https://a.com" data-sl="1">A</a><style>a{}</style>`},
		{"unquoted and selfclose", `<a href=https://a.com>A</a><a href="https://b.com"/>`,
			`<a href=https://a.com data-sl="1">A</a><a href="https://b.com" data-sl="2"/>`},
		{"not a tag", `<abbr title="x">t</abbr><b>x</b> a < b`, `<abbr title="x">t</abbr><b>x</b> a < b`},
		{"gt in attr", `<a title="a>b" href="https://a.com">A</a>`, `<a title="a>b" href="https://a.com" data-sl="1">A</a>`},
		{"multiline", "<a\n  href=\"https://a.com\"\n>A</a>", "<a\n  href=\"https://a.com\"\n data-sl=\"1\">A</a>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NumberLinks(c.in); got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestNumberingStableAcrossRecipients(t *testing.T) {
	tpl := `<p><a href="{{product_url}}">Buy</a> {{#if}}<a href="https://b.com">B</a>` + `<a href="{{unsubscribe_url}}">u</a><a href="https://c.com">C</a>`
	numbered := NumberLinks(tpl)
	// rendering differs: one recipient has no product_url (empty href), different URLs
	r1 := strings.ReplaceAll(numbered, "{{product_url}}", "https://shop.com/1")
	r2 := strings.ReplaceAll(numbered, "{{product_url}}", "mailto:x@y.z")
	r2 = strings.ReplaceAll(r2, "{{unsubscribe_url}}", "https://x.com/u/tok")
	s := NewSigner([]byte("k"))
	o := Options{BaseURL: "https://t.io", Signer: s, SourceType: models.SourceJobItem, SourceID: uid, TrackClicks: true}
	for _, h := range []string{r1, r2} {
		out := Apply(h, o)
		if !strings.Contains(out, "C</a>") || strings.Contains(out, LinkAttr) {
			t.Fatalf("bad output %s", out)
		}
	}
	links := ExtractLinks(numbered)
	if len(links) != 3 || links[2].ID != 3 || links[2].URL != "https://c.com" {
		t.Errorf("links: %+v", links)
	}
	// "C" link id is 3 in both renderings
	for _, h := range []string{r1, r2} {
		out := Apply(h, o)
		i := strings.Index(out, "/t/c/")
		var last string
		for i >= 0 {
			rest := out[i+5:]
			last = rest[:strings.IndexByte(rest, '"')]
			j := strings.Index(rest, "/t/c/")
			if j < 0 {
				break
			}
			i = i + 5 + j
		}
		p, err := s.Decode(last)
		if err != nil || p.LinkID != 3 || p.URL != "https://c.com" {
			t.Errorf("last link: %+v %v", p, err)
		}
	}
}

func TestExtractLinks(t *testing.T) {
	in := NumberLinks(`
<a href="https://a.com">  Hello
   <b>big</b>   world &amp; co </a>
<a href="https://b.com"><img src="x.png" alt="Logo &amp; more"></a>
<a href="https://c.com"><img src="x.png"></a>
<a href="https://d.com" class="my-btn primary">Go</a>
<a href="https://e.com" style="display:block;Background-Color:#f00">Buy</a>
<a href="https://f.com" style="padding:4px;border-radius:3px">Pad</a>
<a href="https://g.com" style="padding:4px">NotBtn</a>
<a href="https://h.com"></a>
<a href="https://i.com">` + strings.Repeat("ж", 100) + `</a>`)
	got := ExtractLinks(in)
	want := []models.LinkInfo{
		{ID: 1, URL: "https://a.com", Label: "Hello big world & co", Kind: "text"},
		{ID: 2, URL: "https://b.com", Label: "Logo & more", Kind: "image"},
		{ID: 3, URL: "https://c.com", Label: "Image", Kind: "image"},
		{ID: 4, URL: "https://d.com", Label: "Go", Kind: "button"},
		{ID: 5, URL: "https://e.com", Label: "Buy", Kind: "button"},
		{ID: 6, URL: "https://f.com", Label: "Pad", Kind: "button"},
		{ID: 7, URL: "https://g.com", Label: "NotBtn", Kind: "text"},
		{ID: 8, URL: "https://h.com", Label: "Link", Kind: "text"},
		{ID: 9, URL: "https://i.com", Label: strings.Repeat("ж", 80), Kind: "text"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d links: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("link %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func testOpts() (Options, *Signer) {
	s := NewSigner([]byte("k"))
	return Options{BaseURL: "https://t.io", Signer: s, SourceType: models.SourceJobItem, SourceID: uid}, s
}

func firstClick(t *testing.T, s *Signer, out string) Payload {
	t.Helper()
	i := strings.Index(out, `href="https://t.io/t/c/`)
	if i < 0 {
		t.Fatalf("no tracked link in %s", out)
	}
	rest := out[i+len(`href="https://t.io/t/c/`):]
	p, err := s.Decode(rest[:strings.IndexByte(rest, '"')])
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyClicks(t *testing.T) {
	o, s := testOpts()
	o.TrackClicks = true
	in := `<html><body><a data-sl="1" href="https://a.com/x?y=1&amp;z=2#frag">A</a><a data-sl="2" href="mailto:a@b.c">m</a><a data-sl="3" href="/rel">r</a><a data-sl="4" href="">e</a><a href="https://notracked.com">nt</a></body></html>`
	out := Apply(in, o)
	if strings.Contains(out, "data-sl") {
		t.Errorf("data-sl left: %s", out)
	}
	p := firstClick(t, s, out)
	if p.Kind != KindClick || p.LinkID != 1 || p.URL != "https://a.com/x?y=1&z=2#frag" || p.SourceID != uid {
		t.Errorf("payload %+v", p)
	}
	for _, keep := range []string{`href="mailto:a@b.c"`, `href="/rel"`, `href=""`, `href="https://notracked.com"`} {
		if !strings.Contains(out, keep) {
			t.Errorf("%s changed: %s", keep, out)
		}
	}
	if strings.Count(out, "/t/c/") != 1 {
		t.Errorf("expected 1 tracked: %s", out)
	}
}

func TestApplyRawHTMLWithoutDataSL(t *testing.T) {
	o, s := testOpts()
	o.TrackClicks = true
	out := Apply(`<p><a href="mailto:x@y.z">m</a> <a href="https://a.com?q=1">A</a></p>`, o)
	p := firstClick(t, s, out)
	if p.LinkID != 1 || p.URL != "https://a.com?q=1" {
		t.Errorf("payload %+v", p)
	}
	if !strings.Contains(out, `href="mailto:x@y.z"`) {
		t.Error(out)
	}
}

func TestApplyUnsubscribeLinkNotTracked(t *testing.T) {
	o, _ := testOpts()
	o.TrackClicks = true
	in := `<a href="https://t.io/u/abc">u</a>`
	if out := Apply(in, o); out != in {
		t.Errorf("unsub link rewritten: %s", out)
	}
}

func TestApplyUTM(t *testing.T) {
	o, s := testOpts()
	o.TrackClicks = true
	o.UTM = &UTM{Source: "email", Medium: "email", Campaign: "spring sale", SC: "c1", SR: "r1", AddSL: true}
	cases := []struct{ href, want string }{
		{"https://a.com", "https://a.com?utm_source=email&utm_medium=email&utm_campaign=spring+sale&sc=c1&sr=r1&sl=1"},
		{"https://a.com/p?x=1#top", "https://a.com/p?x=1&utm_source=email&utm_medium=email&utm_campaign=spring+sale&sc=c1&sr=r1&sl=1#top"},
		{"https://a.com/?utm_source=ads&sl=9", "https://a.com/?utm_source=ads&sl=9&utm_medium=email&utm_campaign=spring+sale&sc=c1&sr=r1"},
		{"https://a.com/?a=1&amp;b=2", "https://a.com/?a=1&b=2&utm_source=email&utm_medium=email&utm_campaign=spring+sale&sc=c1&sr=r1&sl=1"},
	}
	for _, c := range cases {
		p := firstClick(t, s, Apply(`<a href="`+c.href+`">x</a>`, o))
		if p.URL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.href, p.URL, c.want)
		}
	}
	o.UTM = &UTM{Source: "email"}
	p := firstClick(t, s, Apply(`<a href="https://a.com">x</a>`, o))
	if p.URL != "https://a.com?utm_source=email" {
		t.Error(p.URL)
	}
}

func TestApplyPixel(t *testing.T) {
	o, s := testOpts()
	o.TrackOpens = true
	pix := `<img src="` + PixelURL(o.BaseURL, s, o.SourceType, o.SourceID) + `" width="1" height="1" alt="" style="display:block;width:1px;height:1px;border:0" />`
	cases := []struct{ name, in, want string }{
		{"body", "<html><BODY>x</BODY></html>", "<html><BODY>x" + pix + "</BODY></html>"},
		{"last body", "<body>a</body><!-- </body> -->", "<body>a</body><!-- " + pix + "</body> -->"},
		{"no body", "<p>hi</p>", "<p>hi</p>" + pix},
		{"empty", "", pix},
	}
	for _, c := range cases {
		if got := Apply(c.in, o); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	if !strings.Contains(pix, "https://t.io/t/o/") || !strings.Contains(pix, ".gif\"") {
		t.Error(pix)
	}
}

func TestApplyDisabled(t *testing.T) {
	o, _ := testOpts()
	in := `<a data-sl="1" href="https://a.com">A</a><a href="https://b.com" data-sl="2" class="x">B</a></body>`
	want := `<a href="https://a.com">A</a><a href="https://b.com" class="x">B</a></body>`
	if got := Apply(in, o); got != want {
		t.Errorf("clicks off: %s", got)
	}
	o.TrackClicks, o.TrackOpens = true, true
	o.Signer = nil
	if got := Apply(in, o); got != want {
		t.Errorf("nil signer: %s", got)
	}
	o2, _ := testOpts()
	o2.TrackClicks, o2.TrackOpens, o2.BaseURL = true, true, ""
	if got := Apply(in, o2); got != want {
		t.Errorf("empty base: %s", got)
	}
}

func TestApplyPreservesHTML(t *testing.T) {
	o, _ := testOpts()
	o.TrackClicks, o.TrackOpens = true, true
	in := "<!DOCTYPE html>\n<html xmlns:v=\"urn:schemas-microsoft-com:vml\"><head><!--[if mso]><xml><o:OfficeDocumentSettings/></xml><![endif]--></head>\n<body>\n<p>&nbsp;&copy; {{name}}</p><a href=\"{{url}}\">x</a></body></html>"
	out := Apply(in, o)
	// href is an unrendered variable (not http) so only the pixel is added
	want := strings.Replace(in, "</body>", out[strings.Index(out, "<img"):strings.Index(out, "</body>")]+"</body>", 1)
	if out != want || !strings.Contains(out, "<img") {
		t.Errorf("html changed:\n%s", out)
	}
}

func TestUnsubscribeURL(t *testing.T) {
	s := NewSigner([]byte("k"))
	u := UnsubscribeURL("https://t.io", s, models.SourceSend, uid)
	if !strings.HasPrefix(u, "https://t.io/u/") {
		t.Fatal(u)
	}
	p, err := s.Decode(strings.TrimPrefix(u, "https://t.io/u/"))
	if err != nil || p.Kind != KindUnsubscribe || p.SourceID != uid {
		t.Errorf("%+v %v", p, err)
	}
	if UnsubscribeURL("x", nil, models.SourceSend, uid) != "" {
		t.Error("nil signer")
	}
}

func req(remote string, hdr map[string]string) *http.Request {
	r, _ := http.NewRequest("GET", "http://x/", nil)
	r.RemoteAddr = remote
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"peer only", req("1.2.3.4:5555", nil), "1.2.3.4"},
		{"real ip from proxying domain", req("10.1.2.3:80", map[string]string{"X-Real-IP": "6.6.6.6"}), "6.6.6.6"},
		{"real ip beats xff", req("127.0.0.1:80", map[string]string{"X-Real-IP": "6.6.6.6", "X-Forwarded-For": "5.5.5.5"}), "6.6.6.6"},
		{"xff left-most", req("10.1.2.3:80", map[string]string{"X-Forwarded-For": "9.9.9.9, 10.1.2.3"}), "9.9.9.9"},
		{"skip invalid xff", req("127.0.0.1:80", map[string]string{"X-Forwarded-For": "unknown, 7.7.7.7"}), "7.7.7.7"},
		{"xff with port", req("127.0.0.1:80", map[string]string{"X-Forwarded-For": "7.7.7.7:1234"}), "7.7.7.7"},
		{"garbage headers", req("127.0.0.1:80", map[string]string{"X-Forwarded-For": "x,y", "X-Real-IP": "z"}), "127.0.0.1"},
		{"ipv6 peer", req("[2001:db8::1]:443", nil), "2001:db8::1"},
		{"ipv6 xff bracket port", req("[::1]:1", map[string]string{"X-Forwarded-For": "[2001:db8::2]:99"}), "2001:db8::2"},
		{"no port", req("1.2.3.4", nil), "1.2.3.4"},
	}
	for _, c := range cases {
		if got := ClientIP(c.r); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestMachine(t *testing.T) {
	chrome := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	opens := []struct {
		ua   string
		want bool
	}{
		{"", false},
		{chrome, false},
		{"Mozilla/5.0 (Windows NT 5.1; rv:11.0) Gecko Firefox/11.0 (via ggpht.com GoogleImageProxy)", false},
		{"YandexImageResizer", false},
		{"Mozilla/5.0 (compatible; Mail.RU_Bot/Img/2.0)", false},
		{"Googlebot/2.1", true},
		{"python-requests/2.31", true},
		{"Go-http-client/1.1", true},
		{"curl/8.0", true},
		{"Mozilla HeadlessChrome/120", true},
		{"Barracuda Sentinel", true},
		{"Slackbot-LinkExpanding", true},
		{"WhatsApp/2.23", true},
	}
	for _, c := range opens {
		if got := IsMachineOpen(c.ua); got != c.want {
			t.Errorf("open %q: got %v", c.ua, got)
		}
	}
	now := time.Now()
	recent := now.Add(-time.Second)
	old := now.Add(-time.Minute)
	clicks := []struct {
		name string
		ua   string
		sent *time.Time
		want bool
	}{
		{"human", chrome, &old, false},
		{"human nil sent", chrome, nil, false},
		{"empty ua", "", &old, true},
		{"spaces ua", "  ", nil, true},
		{"scanner", "Mozilla/5.0 Proofpoint URL Defense", &old, true},
		{"too fast", chrome, &recent, true},
		{"exactly 2s", chrome, ptr(now.Add(-2 * time.Second)), false},
		{"proxy ua click", "GoogleImageProxy", &old, false},
	}
	for _, c := range clicks {
		if got := IsMachineClick(c.ua, c.sent, now); got != c.want {
			t.Errorf("click %s: got %v", c.name, got)
		}
	}
}

func ptr(t time.Time) *time.Time { return &t }
