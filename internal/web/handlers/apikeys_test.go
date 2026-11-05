package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
)

func postKeyForm(h http.HandlerFunc, path string, form url.Values, pathValues ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(pathValues); i += 2 {
		req.SetPathValue(pathValues[i], pathValues[i+1])
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestAPIKeyCreateAndEditPermissions(t *testing.T) {
	h, d := newCampaignTestHandlers(t, "")
	keys := repository.NewAPIKeyRepository(d)
	count := func() int {
		var n int
		d.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&n)
		return n
	}

	// No permission: rejected, nothing saved.
	if w := postKeyForm(h.APIKeyCreate, "/settings/api-keys", url.Values{"name": {"none"}}); w.Code != http.StatusBadRequest {
		t.Errorf("create without permissions: %d", w.Code)
	}
	if w := postKeyForm(h.APIKeyCreate, "/settings/api-keys", url.Values{"name": {"bad"}, "permissions": {"admin"}}); w.Code != http.StatusBadRequest {
		t.Errorf("create with unknown permission: %d", w.Code)
	}
	if n := count(); n != 0 {
		t.Fatalf("keys saved: %d", n)
	}

	w := postKeyForm(h.APIKeyCreate, "/settings/api-keys", url.Values{"name": {"both"}, "permissions": {"lists", "send"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var id string
	d.QueryRow(`SELECT id FROM api_keys WHERE name = 'both'`).Scan(&id)
	k, _ := keys.GetByID(id)
	if k == nil || k.Permissions != `["send","lists"]` {
		t.Fatalf("stored permissions: %+v", k)
	}

	// Edit: removing every permission is rejected and keeps the old ones.
	if w := postKeyForm(h.APIKeyEdit, "/settings/api-keys/"+id+"/edit", url.Values{"name": {"both"}}, "id", id); w.Code != http.StatusBadRequest {
		t.Errorf("edit without permissions: %d", w.Code)
	}
	if k, _ = keys.GetByID(id); !k.HasPermission(models.PermissionLists) {
		t.Errorf("permissions lost after a rejected edit: %s", k.Permissions)
	}
	if w := postKeyForm(h.APIKeyEdit, "/settings/api-keys/"+id+"/edit", url.Values{"name": {"both"}, "permissions": {"send"}}, "id", id); w.Code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", w.Code, w.Body.String())
	}
	if k, _ = keys.GetByID(id); k.HasPermission(models.PermissionLists) || !k.HasPermission(models.PermissionSend) {
		t.Errorf("after edit: %s", k.Permissions)
	}
}

// Keys created before permissions were enforced hold ["send"] (the column
// default); a missing or unreadable value means the same.
func TestAPIKeyLegacyPermissions(t *testing.T) {
	for _, stored := range []string{`["send"]`, ``, `garbage`, `[]`, `["status"]`, `["send","template","status"]`} {
		k := &models.APIKey{Permissions: stored}
		if !k.HasPermission(models.PermissionSend) || k.HasPermission(models.PermissionLists) {
			t.Errorf("%q: %v", stored, k.PermissionList())
		}
	}
}
