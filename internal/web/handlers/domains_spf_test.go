package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/foxzi/sendry/internal/web/views"
)

func TestCentralDomainViewRecommendedDNS_SPFIncludeFromGlobalVariables(t *testing.T) {
	t.Run("uses include when spf_include is set", func(t *testing.T) {
		h, database, cleanup := newDomainViewTestHandlers(t)
		defer cleanup()

		domainID := createTestDomain(t, database, "with-include.example")

		if err := repository.NewSettingsRepository(database.DB).SetVariable("spf_include", "_spf.mailgun.org", "SPF include"); err != nil {
			t.Fatalf("SetVariable(spf_include) error = %v", err)
		}

		body := renderCentralDomainViewHTML(t, h, domainID)
		if !strings.Contains(body, "v=spf1 a mx include:_spf.mailgun.org ~all") {
			t.Fatalf("response does not contain SPF with include, body: %s", body)
		}
		if !strings.Contains(body, "spf_include=_spf.mailgun.org") {
			t.Fatalf("response does not show spf_include hint, body: %s", body)
		}
	})

	t.Run("uses default SPF when spf_include is not set", func(t *testing.T) {
		h, database, cleanup := newDomainViewTestHandlers(t)
		defer cleanup()

		domainID := createTestDomain(t, database, "no-include.example")

		body := renderCentralDomainViewHTML(t, h, domainID)
		if !strings.Contains(body, "v=spf1 a mx ~all") {
			t.Fatalf("response does not contain default SPF, body: %s", body)
		}
		if strings.Contains(body, "include:") {
			t.Fatalf("response unexpectedly contains include, body: %s", body)
		}
	})
}

func newDomainViewTestHandlers(t *testing.T) (*Handlers, *db.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	if err := database.Migrate(); err != nil {
		database.Close()
		t.Fatalf("database.Migrate() error = %v", err)
	}

	viewEngine, err := views.New()
	if err != nil {
		database.Close()
		t.Fatalf("views.New() error = %v", err)
	}

	cfg := &config.Config{}
	h := New(cfg, database, testLogger(), viewEngine, nil)

	cleanup := func() {
		database.Close()
		_ = os.Remove(dbPath)
	}

	return h, database, cleanup
}

func createTestDomain(t *testing.T, database *db.DB, domainName string) string {
	t.Helper()

	repo := repository.NewDomainRepository(database.DB)
	domain := &models.Domain{
		Domain: domainName,
		Mode:   "production",
	}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("DomainRepository.Create() error = %v", err)
	}
	return domain.ID
}

func renderCentralDomainViewHTML(t *testing.T, h *Handlers, domainID string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/domains/"+domainID, nil)
	req.SetPathValue("id", domainID)
	w := httptest.NewRecorder()

	h.CentralDomainsView(w, req)

	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("CentralDomainsView() status = %d, body = %s", res.StatusCode, string(body))
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return string(body)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
