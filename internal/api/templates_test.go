package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/template"
)

// setupTemplateTestServer returns an API server with template storage holding
// one template named "welcome".
func setupTemplateTestServer(t *testing.T) (*Server, *mockQueue, *template.Template) {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), "templates.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	storage, err := template.NewStorage(db)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &template.Template{Name: "welcome", Subject: "Hello", Text: "Body"}
	if err := storage.Create(context.Background(), tmpl); err != nil {
		t.Fatal(err)
	}

	q := newMockQueue()
	server := NewServerWithOptions(ServerOptions{
		Queue:           q,
		Config:          &config.APIConfig{ListenAddr: ":8080", APIKey: "test-api-key"},
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		TemplateStorage: storage,
	})
	return server, q, tmpl
}

func postSendTemplate(t *testing.T, server *Server, req SendTemplateRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/send/template", bytes.NewBuffer(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, r)
	return w
}

func TestSendTemplateFromName(t *testing.T) {
	for _, tc := range fromNameCases {
		t.Run(tc.name, func(t *testing.T) {
			server, q, tmpl := setupTemplateTestServer(t)
			w := postSendTemplate(t, server, SendTemplateRequest{
				TemplateID: tmpl.ID,
				From:       tc.from,
				FromName:   tc.fromName,
				To:         []string{"to@example.com"},
			})

			if w.Code != http.StatusAccepted {
				t.Fatalf("Status = %d, want %d. Body: %s", w.Code, http.StatusAccepted, w.Body.String())
			}
			if len(q.messages) != 1 {
				t.Fatalf("queued %d messages, want 1", len(q.messages))
			}
			for _, msg := range q.messages {
				checkFromHeaders(t, msg, tc.wantRaw, tc.wantName)
			}
		})
	}
}

// The template may be addressed by name, and from_name works there too.
func TestSendTemplateFromNameByTemplateName(t *testing.T) {
	server, q, _ := setupTemplateTestServer(t)
	w := postSendTemplate(t, server, SendTemplateRequest{
		TemplateName: "welcome",
		From:         "Old <shop@example.com>",
		FromName:     "Магазин",
		To:           []string{"to@example.com"},
	})

	if w.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d. Body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}
	if len(q.messages) != 1 {
		t.Fatalf("queued %d messages, want 1", len(q.messages))
	}
	for _, msg := range q.messages {
		checkFromHeaders(t, msg, "From: =?utf-8?q?", "Магазин")
	}
}

// from_name does not make an invalid from acceptable.
func TestSendTemplateFromNameInvalidFrom(t *testing.T) {
	server, q, tmpl := setupTemplateTestServer(t)
	w := postSendTemplate(t, server, SendTemplateRequest{
		TemplateID: tmpl.ID,
		From:       "not-an-address",
		FromName:   "Shop",
		To:         []string{"to@example.com"},
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Status = %d, want %d. Body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if len(q.messages) != 0 {
		t.Errorf("queued %d messages, want 0", len(q.messages))
	}
}
