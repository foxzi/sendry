package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/views"
)

// The campaign test send must carry the template preheader, rendered with the
// campaign variables, like the real campaign send does.
func TestCampaignWizardTestSendsPreheader(t *testing.T) {
	var mu sync.Mutex
	var sent map[string]any
	mta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(raw, &sent)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"m1","status":"queued"}`))
	}))
	defer mta.Close()

	database, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	v, err := views.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Sendry.Servers = []config.SendryServer{{Name: "mta-1", BaseURL: mta.URL}}
	h := New(cfg, database, testLogger(), v, nil)

	tpl := &models.Template{Name: "t", Subject: "Hi", Preheader: "Sale for {{Name}}", HTML: "<html><body><p>Body</p></body></html>"}
	if err := repository.NewTemplateRepository(database.DB).Create(tpl, "u"); err != nil {
		t.Fatal(err)
	}

	body := `{"template_id":"` + tpl.ID + `","from_email":"a@shop.example","to":"b@example.org","variables":"{\"Name\":\"Ann\"}"}`
	req := httptest.NewRequest(http.MethodPost, "/campaigns/wizard/test", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CampaignWizardTest(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("test send: %d %s", w.Code, w.Body.String())
	}

	mu.Lock()
	html, _ := sent["html"].(string)
	mu.Unlock()
	if !strings.Contains(html, "<body><div style=\"display:none;") || !strings.Contains(html, "Sale for Ann") {
		t.Errorf("sent HTML has no rendered preheader: %s", html)
	}
}
