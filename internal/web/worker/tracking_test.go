package worker

import (
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/tracking"
)

func testEnv() *trackingEnv {
	return &trackingEnv{baseURL: "https://lnk.example.com", signer: tracking.NewSigner([]byte("test-secret"))}
}

const tplHTML = `<html><body><a href="https://shop.example.com/{{name}}">Shop</a> <a href="mailto:a@b.c">Mail</a> <a href="{{unsubscribe_url}}">Unsubscribe</a></body></html>`

func buildFor(t *testing.T, campaign *models.Campaign, env *trackingEnv) (string, error) {
	t.Helper()
	vars := map[string]any{"name": "bob"}
	vars["unsubscribe_url"] = env.unsubscribeURL("item1")
	return buildCampaignHTML(tracking.NumberLinks(tplHTML), vars, env, campaign, "item1")
}

func TestBuildCampaignHTML_Tracking(t *testing.T) {
	c := &models.Campaign{ID: "c1", Name: "Spring", TrackOpens: true, TrackClicks: true}
	html, err := buildFor(t, c, testEnv())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "data-sl") {
		t.Errorf("data-sl must be removed: %s", html)
	}
	if strings.Contains(html, "https://shop.example.com/bob") {
		t.Errorf("shop link must be rewritten: %s", html)
	}
	if !strings.Contains(html, "https://lnk.example.com/t/c/") {
		t.Errorf("click link missing: %s", html)
	}
	if !strings.Contains(html, "https://lnk.example.com/t/o/") {
		t.Errorf("open pixel missing: %s", html)
	}
	if !strings.Contains(html, "https://lnk.example.com/u/") {
		t.Errorf("unsubscribe link missing: %s", html)
	}
	if !strings.Contains(html, "mailto:a@b.c") {
		t.Errorf("mailto must stay: %s", html)
	}
}

func TestBuildCampaignHTML_Disabled(t *testing.T) {
	c := &models.Campaign{ID: "c1", Name: "Spring"}
	html, err := buildFor(t, c, testEnv())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "data-sl") || strings.Contains(html, "/t/o/") || strings.Contains(html, "/t/c/") {
		t.Errorf("no tracking expected: %s", html)
	}
	if !strings.Contains(html, "https://shop.example.com/bob") {
		t.Errorf("link must stay as is: %s", html)
	}
	if !strings.Contains(html, "https://lnk.example.com/u/") {
		t.Errorf("unsubscribe link must be present: %s", html)
	}
}

func TestBuildCampaignHTML_UTMOnlyWhenEnabled(t *testing.T) {
	// UTM parameters live inside the click token, so check the destination
	// through the token payload.
	env := testEnv()
	for _, addUTM := range []bool{false, true} {
		c := &models.Campaign{ID: "c1", Name: "Spring", TrackClicks: true, AddUTM: addUTM}
		html, err := buildFor(t, c, env)
		if err != nil {
			t.Fatal(err)
		}
		const marker = "/t/c/"
		i := strings.Index(html, marker)
		if i < 0 {
			t.Fatalf("no click link: %s", html)
		}
		tok := html[i+len(marker):]
		tok = tok[:strings.IndexAny(tok, `"'`)]
		p, err := env.signer.Decode(tok)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(p.URL, "utm_source=email"); got != addUTM {
			t.Errorf("addUTM=%v: url %q", addUTM, p.URL)
		}
		if addUTM && (!strings.Contains(p.URL, "sc=c1") || !strings.Contains(p.URL, "sr=item1") || !strings.Contains(p.URL, "sl=1")) {
			t.Errorf("missing sc/sr/sl: %q", p.URL)
		}
	}
}

func TestCampaignHeaders(t *testing.T) {
	h := campaignHeaders(&models.Campaign{ReplyTo: "r@example.com"}, testEnv(), "item1")
	if h["Reply-To"] != "r@example.com" {
		t.Errorf("Reply-To lost: %v", h)
	}
	if !strings.HasPrefix(h["List-Unsubscribe"], "<https://lnk.example.com/u/") {
		t.Errorf("List-Unsubscribe: %q", h["List-Unsubscribe"])
	}
	if h["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Errorf("List-Unsubscribe-Post: %q", h["List-Unsubscribe-Post"])
	}

	if h := campaignHeaders(&models.Campaign{}, nil, "item1"); h != nil {
		t.Errorf("expected no headers without env and reply-to, got %v", h)
	}
}

func TestBuildCampaignHTML_UTMWithoutClickTracking(t *testing.T) {
	c := &models.Campaign{ID: "c1", Name: "Spring", AddUTM: true}
	html, err := buildFor(t, c, testEnv())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "/t/c/") || strings.Contains(html, "data-sl") {
		t.Errorf("no click tracking expected: %s", html)
	}
	if !strings.Contains(html, `href="https://shop.example.com/bob?utm_source=email&amp;utm_medium=email&amp;utm_campaign=Spring&amp;sc=c1&amp;sr=item1&amp;sl=1"`) {
		t.Errorf("UTM missing: %s", html)
	}
	if !strings.Contains(html, `href="https://lnk.example.com/u/`) || strings.Contains(html, "/u/?utm") {
		t.Errorf("unsubscribe link must stay without UTM: %s", html)
	}
}

func TestBuildCampaignHTML_Empty(t *testing.T) {
	c := &models.Campaign{ID: "c1", TrackOpens: true, TrackClicks: true}
	for _, src := range []string{"", "  \n"} {
		html, err := buildCampaignHTML(src, map[string]any{}, testEnv(), c, "item1")
		if err != nil || html != "" {
			t.Errorf("%q: got %q, %v; want empty HTML without a pixel", src, html, err)
		}
	}
}

func TestSnapshotVarsHaveNoRecipientData(t *testing.T) {
	src := tracking.NumberLinks(`<p>Hi {{name}} ({{email}}), {{promo}} {{city}}</p><a href="{{unsubscribe_url}}">u</a>`)
	html, err := renderTemplate("html", src, snapshotVars(map[string]string{"promo": "SALE"}, map[string]string{"city": "Kazan"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Hi {{name}} ({{email}}), SALE Kazan") || !strings.Contains(html, `href="#"`) {
		t.Errorf("snapshot = %s", html)
	}
}
