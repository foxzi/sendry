package repository

import (
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestRecipientRepository_EmailNormalized(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`)
	repo := NewRecipientRepository(db)

	r := &models.Recipient{ListID: "l1", Email: "  User@Example.ORG ", Name: "One"}
	if err := repo.AddRecipient(r); err != nil {
		t.Fatal(err)
	}
	// the same address in another case is the same recipient; its status is kept
	mustExec(t, db, `UPDATE recipients SET status = 'unsubscribed'`)
	if err := repo.AddRecipient(&models.Recipient{ListID: "l1", Email: "user@example.org", Name: "Two"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ImportCSV("l1", strings.NewReader("email,name\nUSER@example.org,Three\nNew@Example.org,New\n")); err != nil {
		t.Fatal(err)
	}

	recs, total, err := repo.ListRecipients(models.RecipientFilter{ListID: "l1"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, rec := range recs {
		got[rec.Email] = rec.Name + "/" + rec.Status
	}
	if total != 2 || got["user@example.org"] != "Three/unsubscribed" || got["new@example.org"] != "New/active" {
		t.Errorf("recipients = %v", got)
	}
}
