package handlers

import (
	"bytes"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/tracking"
)

type trackEnv struct {
	h      *Handlers
	mux    *http.ServeMux
	db     *db.DB
	signer *tracking.Signer
}

func newTrackEnv(t *testing.T) *trackEnv {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO campaigns (id, name, from_email, from_name) VALUES ('c1','Camp','news@example.com','Example News')`,
		`INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1','List','manual')`,
		`INSERT INTO recipients (id, list_id, email) VALUES ('r1','l1','User@Example.com')`,
		`INSERT INTO recipient_lists (id, name, source_type) VALUES ('l2','Other list','manual')`,
		`INSERT INTO recipients (id, list_id, email) VALUES ('r2','l2','user@example.com')`,
		`INSERT INTO send_jobs (id, campaign_id, recipient_list_id) VALUES ('j1','c1','l1')`,
		`INSERT INTO send_job_items (id, job_id, recipient_id, status) VALUES ('i1','j1','r1','sent')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	h := &Handlers{
		db:       d,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		settings: repository.NewSettingsRepository(d.DB),
		tracking: repository.NewTrackingRepository(d.DB),
		unsubs:   repository.NewUnsubscribeRepository(d.DB),
	}
	ts, err := h.settings.GetTrackingSettings()
	if err != nil || ts.Secret == "" {
		t.Fatalf("tracking settings not ready (err=%v)", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /t/o/{token}", h.TrackOpen)
	mux.HandleFunc("GET /t/c/{token}", h.TrackClick)
	mux.HandleFunc("GET /u/{token}", h.UnsubscribePage)
	mux.HandleFunc("POST /u/{token}", h.UnsubscribeConfirm)
	return &trackEnv{h: h, mux: mux, db: d, signer: tracking.NewSigner([]byte(ts.Secret))}
}

func (e *trackEnv) do(method, path, body string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("User-Agent", "Mozilla/5.0 test")
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, req)
	return w
}

func (e *trackEnv) count(t *testing.T, typ string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM email_events WHERE type=?`, typ).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *trackEnv) status(t *testing.T, recipientID string) string {
	t.Helper()
	var s string
	if err := e.db.QueryRow(`SELECT status FROM recipients WHERE id=?`, recipientID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *trackEnv) token(k tracking.Kind, srcType, srcID string) string {
	return e.signer.Encode(tracking.Payload{Kind: k, SourceType: srcType, SourceID: srcID})
}

func TestTrackOpen(t *testing.T) {
	e := newTrackEnv(t)
	tok := e.token(tracking.KindOpen, "job_item", "i1")

	for _, path := range []string{"/t/o/" + tok + ".gif", "/t/o/" + tok} {
		w := e.do("GET", path, "")
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/gif" {
			t.Fatalf("%s: code=%d ct=%s", path, w.Code, w.Header().Get("Content-Type"))
		}
		if !bytes.Equal(w.Body.Bytes(), tracking.TransparentGIF) {
			t.Fatalf("%s: body is not the GIF", path)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("missing no-store")
		}
	}
	// The second load from the same IP within the dedup window is dropped
	if n := e.count(t, "open"); n != 1 {
		t.Fatalf("open events = %d, want 1", n)
	}
	var email, camp, job string
	if err := e.db.QueryRow(`SELECT email, campaign_id, job_id FROM email_events WHERE type='open' LIMIT 1`).Scan(&email, &camp, &job); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(email) != "user@example.com" || camp != "c1" || job != "j1" {
		t.Fatalf("event = %s %s %s", email, camp, job)
	}

	// Bad token and wrong kind: GIF, nothing recorded.
	for _, path := range []string{"/t/o/garbage.gif", "/t/o/" + e.token(tracking.KindClick, "job_item", "i1")} {
		w := e.do("GET", path, "")
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), tracking.TransparentGIF) {
			t.Fatalf("%s: code=%d", path, w.Code)
		}
	}
	// Unknown source: GIF, nothing recorded.
	e.do("GET", "/t/o/"+e.token(tracking.KindOpen, "job_item", "nope"), "")
	if n := e.count(t, "open"); n != 1 {
		t.Fatalf("open events = %d, want 1", n)
	}
}

func TestTrackOpenClientIP(t *testing.T) {
	e := newTrackEnv(t)
	tok := e.token(tracking.KindOpen, "job_item", "i1")
	open := func(remote, realIP string) {
		req := httptest.NewRequest("GET", "/t/o/"+tok, nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Real-IP", realIP)
		e.mux.ServeHTTP(httptest.NewRecorder(), req)
	}
	lastIP := func() string {
		var ip string
		if err := e.db.QueryRow(`SELECT ip FROM email_events ORDER BY id DESC LIMIT 1`).Scan(&ip); err != nil {
			t.Fatal(err)
		}
		return ip
	}

	// Without trusted proxies the header is ignored
	open("203.0.113.5:1000", "6.6.6.6")
	if ip := lastIP(); ip != "203.0.113.5" {
		t.Fatalf("ip = %s, want the peer", ip)
	}
	e.h.proxies, _ = tracking.ParseTrustedProxies([]string{"127.0.0.1"})
	open("127.0.0.1:1000", "6.6.6.6")
	if ip := lastIP(); ip != "6.6.6.6" {
		t.Fatalf("ip = %s, want the header from a trusted proxy", ip)
	}
	open("203.0.113.9:1000", "7.7.7.7")
	if ip := lastIP(); ip != "203.0.113.9" {
		t.Fatalf("ip = %s, want the untrusted peer", ip)
	}
}

func TestTrackTokenExpiryAndRotation(t *testing.T) {
	e := newTrackEnv(t)
	click := func(s *tracking.Signer, issued int64) string {
		return "/t/c/" + s.Encode(tracking.Payload{Kind: tracking.KindClick, SourceType: "job_item", SourceID: "i1",
			LinkID: 1, URL: "https://shop.example.com/", IssuedAt: issued})
	}

	// Expired click token: no redirect, nothing recorded
	old := time.Now().Add(-tracking.MaxAgeOpenClick - time.Hour).Unix()
	if w := e.do("GET", click(e.signer, old), ""); w.Code != http.StatusGone || w.Header().Get("Location") != "" {
		t.Fatalf("expired click: code=%d loc=%q", w.Code, w.Header().Get("Location"))
	}
	if n := e.count(t, "click"); n != 0 {
		t.Fatalf("expired click recorded")
	}
	// An unsubscribe link of the same age still works
	unsub := e.signer.Encode(tracking.Payload{Kind: tracking.KindUnsubscribe, SourceType: "job_item", SourceID: "i1", IssuedAt: old})
	if w := e.do("GET", "/u/"+unsub, ""); w.Code != 200 {
		t.Fatalf("old unsubscribe link: code=%d", w.Code)
	}

	// After a rotation mail signed with the previous secret keeps working
	before := click(e.signer, 0)
	if err := e.h.settings.RotateTrackingSecret(); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", before, ""); w.Code != http.StatusFound {
		t.Fatalf("previous-secret click after rotation: code=%d", w.Code)
	}
	ts, _ := e.h.settings.GetTrackingSettings()
	if w := e.do("GET", click(tracking.NewSigner([]byte(ts.Secret)), 0), ""); w.Code != http.StatusFound {
		t.Fatalf("current-secret click: code=%d", w.Code)
	}
	// A second rotation drops the oldest secret
	if err := e.h.settings.RotateTrackingSecret(); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", before, ""); w.Code != http.StatusNotFound {
		t.Fatalf("dropped-secret click: code=%d", w.Code)
	}
}

func TestTrackClick(t *testing.T) {
	e := newTrackEnv(t)
	click := func(u string) string {
		return "/t/c/" + e.signer.Encode(tracking.Payload{Kind: tracking.KindClick, SourceType: "job_item", SourceID: "i1", LinkID: 3, URL: u})
	}

	w := e.do("GET", click("https://shop.example.com/p?a=1"), "")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://shop.example.com/p?a=1" {
		t.Fatalf("code=%d loc=%s", w.Code, w.Header().Get("Location"))
	}
	if n := e.count(t, "click"); n != 1 {
		t.Fatalf("click events = %d", n)
	}
	var linkID int
	var u string
	if err := e.db.QueryRow(`SELECT link_id, url FROM email_events WHERE type='click'`).Scan(&linkID, &u); err != nil || linkID != 3 || u != "https://shop.example.com/p?a=1" {
		t.Fatalf("link=%d url=%s err=%v", linkID, u, err)
	}

	// HEAD redirects but does not record.
	if w := e.do("HEAD", click("https://shop.example.com/"), ""); w.Code != http.StatusFound {
		t.Fatalf("HEAD code=%d", w.Code)
	}
	if n := e.count(t, "click"); n != 1 {
		t.Fatalf("HEAD recorded an event")
	}

	// Source gone: still redirects.
	gone := "/t/c/" + e.signer.Encode(tracking.Payload{Kind: tracking.KindClick, SourceType: "job_item", SourceID: "gone", LinkID: 1, URL: "https://x.example.com/"})
	if w := e.do("GET", gone, ""); w.Code != http.StatusFound {
		t.Fatalf("gone source code=%d", w.Code)
	}

	// Bad token, bad scheme, wrong kind.
	if w := e.do("GET", "/t/c/garbage", ""); w.Code != 404 {
		t.Fatalf("bad token code=%d", w.Code)
	}
	if w := e.do("GET", click("javascript:alert(1)"), ""); w.Code != 404 {
		t.Fatalf("javascript: code=%d", w.Code)
	}
	if w := e.do("GET", "/t/c/"+e.token(tracking.KindOpen, "job_item", "i1"), ""); w.Code != 404 {
		t.Fatalf("wrong kind code=%d", w.Code)
	}
}

func TestUnsubscribeFlow(t *testing.T) {
	e := newTrackEnv(t)
	tok := e.token(tracking.KindUnsubscribe, "job_item", "i1")
	path := "/u/" + tok

	w := e.do("GET", path, "")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(strings.ToLower(body), "user@example.com") || !strings.Contains(body, "Example News") ||
		!strings.Contains(body, "Отписаться") || !strings.Contains(body, "этого списка рассылки") {
		t.Fatalf("GET page code=%d body=%s", w.Code, body)
	}
	if strings.Contains(body, "всех рассылок") || strings.Contains(body, "Other list") || strings.Contains(body, ">List<") {
		t.Fatalf("page must not mention all mailings or list names: %s", body)
	}
	if unsub, _ := e.h.unsubs.IsUnsubscribed("l1", "user@example.com"); unsub {
		t.Fatalf("GET must not unsubscribe")
	}

	form := url.Values{"action": {"unsubscribe"}}.Encode()
	w = e.do("POST", path, form)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Вы отписались") {
		t.Fatalf("POST code=%d body=%s", w.Code, w.Body.String())
	}
	if ok, _ := e.h.unsubs.IsUnsubscribed("l1", "user@example.com"); !ok {
		t.Fatalf("not unsubscribed in l1")
	}
	if e.status(t, "r1") != "unsubscribed" {
		t.Fatalf("r1 status = %s", e.status(t, "r1"))
	}
	// The same address in another list keeps receiving mail.
	if e.status(t, "r2") != "active" {
		t.Fatalf("r2 status = %s, want active", e.status(t, "r2"))
	}
	if ok, _ := e.h.unsubs.IsUnsubscribed("l2", "user@example.com"); ok {
		t.Fatalf("unsubscribed in l2 too")
	}
	if n, src, camp, job := e.journal(t, "l1"); n != 1 || src != "link" || camp != "c1" || job != "j1" {
		t.Fatalf("journal = %d %s %s %s", n, src, camp, job)
	}
	if n := e.count(t, "unsubscribe"); n != 1 {
		t.Fatalf("unsubscribe events = %d", n)
	}

	// Double submit is harmless.
	e.do("POST", path, form)
	if n := e.count(t, "unsubscribe"); n != 1 {
		t.Fatalf("unsubscribe events after repeat = %d", n)
	}

	if w := e.do("GET", path, ""); !strings.Contains(w.Body.String(), "Вы уже отписаны") || !strings.Contains(w.Body.String(), "Вернуть подписку") {
		t.Fatalf("already state: %s", w.Body.String())
	}

	w = e.do("POST", path, url.Values{"action": {"resubscribe"}}.Encode())
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Подписка восстановлена") {
		t.Fatalf("resubscribe code=%d body=%s", w.Code, w.Body.String())
	}
	if ok, _ := e.h.unsubs.IsUnsubscribed("l1", "user@example.com"); ok || e.status(t, "r1") != "active" {
		t.Fatalf("still unsubscribed")
	}
}

func TestUnsubscribeOneClick(t *testing.T) {
	e := newTrackEnv(t)
	path := "/u/" + e.token(tracking.KindUnsubscribe, "job_item", "i1")
	w := e.do("POST", path, "List-Unsubscribe=One-Click")
	if w.Code != 200 || strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if n, src, _, _ := e.journal(t, "l1"); n != 1 || src != "one_click" {
		t.Fatalf("entry = %d %s", n, src)
	}
	if n := e.count(t, "unsubscribe"); n != 1 {
		t.Fatalf("events = %d", n)
	}
}

func TestUnsubscribeInvalid(t *testing.T) {
	e := newTrackEnv(t)
	if w := e.do("GET", "/u/garbage", ""); w.Code != 404 || !strings.Contains(w.Body.String(), "Ссылка недействительна") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	// Send-source tokens are not unsubscribable at this stage.
	sendTok := e.token(tracking.KindUnsubscribe, "send", "s1")
	if w := e.do("GET", "/u/"+sendTok, ""); w.Code != 404 {
		t.Fatalf("send GET code=%d", w.Code)
	}
	if w := e.do("POST", "/u/"+sendTok, "action=unsubscribe"); w.Code != 404 {
		t.Fatalf("send POST code=%d", w.Code)
	}
	// Unknown job item.
	if w := e.do("GET", "/u/"+e.token(tracking.KindUnsubscribe, "job_item", "nope"), ""); w.Code != 404 {
		t.Fatalf("unknown item code=%d", w.Code)
	}
}

// journal returns the number of journal rows of a list and the fields of the last one.
func (e *trackEnv) journal(t *testing.T, listID string) (n int, source, campaign, job string) {
	t.Helper()
	if err := e.h.db.QueryRow(`SELECT COUNT(*) FROM unsubscribes WHERE list_id = ?`, listID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	var src, c, j sql.NullString
	_ = e.h.db.QueryRow(`SELECT source_type, campaign_id, job_id FROM unsubscribes WHERE list_id = ? ORDER BY id DESC LIMIT 1`, listID).Scan(&src, &c, &j)
	return n, src.String, c.String, j.String
}
