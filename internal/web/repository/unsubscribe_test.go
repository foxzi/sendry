package repository

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

// setupUnsubLists creates two lists with overlapping addresses and a campaign.
func setupUnsubLists(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list one', 'manual')`)
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l2', 'list two', 'manual')`)
	mustExec(t, db, `INSERT INTO campaigns (id, name, from_email) VALUES ('c1', 'camp one', 'a@example.com')`)
	mustExec(t, db, `INSERT INTO campaigns (id, name, from_email) VALUES ('c2', 'camp two', 'a@example.com')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES ('r1', 'l1', 'User@Example.org')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES ('r2', 'l1', 'other@example.org')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES ('r3', 'l2', 'user@example.org')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES ('r4', 'l2', 'third@example.org')`)
	for _, l := range []string{"l1", "l2"} {
		if err := NewRecipientRepository(db).UpdateListCounts(l); err != nil {
			t.Fatal(err)
		}
	}
}

func recipientStatus(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT status FROM recipients WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func activeCount(t *testing.T, db *sql.DB, listID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT active_count FROM recipient_lists WHERE id = ?`, listID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUnsubscribe_AddPerList(t *testing.T) {
	db := setupTestDB(t)
	setupUnsubLists(t, db)
	repo := NewUnsubscribeRepository(db)

	changed, err := repo.Add(&models.Unsubscribe{ListID: "l1", Email: "  USER@example.ORG ", CampaignID: "c1", SourceType: models.UnsubSourceLink})
	if err != nil || !changed {
		t.Fatalf("first Add = %v, %v", changed, err)
	}
	if s := recipientStatus(t, db, "r1"); s != "unsubscribed" {
		t.Errorf("r1 status = %s", s)
	}
	if s := recipientStatus(t, db, "r3"); s != "active" {
		t.Errorf("same email in other list must stay active, got %s", s)
	}
	if got := activeCount(t, db, "l1"); got != 1 {
		t.Errorf("l1 active_count = %d", got)
	}
	if got := activeCount(t, db, "l2"); got != 2 {
		t.Errorf("l2 active_count = %d", got)
	}

	// Repeat with another campaign: not changed, journal keeps the latest.
	u := &models.Unsubscribe{ListID: "l1", Email: "user@example.org", CampaignID: "c2", JobID: "j2", SourceType: models.UnsubSourceOneClick}
	changed, err = repo.Add(u)
	if err != nil || changed {
		t.Fatalf("second Add = %v, %v", changed, err)
	}
	if u.RecipientID != "r1" {
		t.Errorf("RecipientID = %q", u.RecipientID)
	}
	var n int
	var camp, src string
	if err := db.QueryRow(`SELECT COUNT(*), MAX(campaign_id), MAX(source_type) FROM unsubscribes`).Scan(&n, &camp, &src); err != nil {
		t.Fatal(err)
	}
	if n != 1 || camp != "c2" || src != models.UnsubSourceOneClick {
		t.Errorf("journal: n=%d campaign=%s source=%s", n, camp, src)
	}

	_, err = repo.Add(&models.Unsubscribe{ListID: "l2", Email: "nobody@example.org", SourceType: models.UnsubSourceLink})
	if !errors.Is(err, ErrRecipientNotInList) {
		t.Errorf("missing recipient err = %v", err)
	}
	_, err = repo.Add(&models.Unsubscribe{ListID: "l1", Email: "third@example.org", SourceType: models.UnsubSourceLink})
	if !errors.Is(err, ErrRecipientNotInList) {
		t.Errorf("address of another list err = %v", err)
	}
}

func TestUnsubscribe_RemoveAndIsUnsubscribed(t *testing.T) {
	db := setupTestDB(t)
	setupUnsubLists(t, db)
	repo := NewUnsubscribeRepository(db)

	if _, err := repo.Add(&models.Unsubscribe{ListID: "l1", Email: "user@example.org", SourceType: models.UnsubSourceLink}); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.IsUnsubscribed("l1", "USER@example.org"); err != nil || !ok {
		t.Errorf("IsUnsubscribed l1 = %v, %v", ok, err)
	}
	if ok, err := repo.IsUnsubscribed("l2", "user@example.org"); err != nil || ok {
		t.Errorf("IsUnsubscribed l2 = %v, %v", ok, err)
	}

	if err := repo.Remove("l1", "User@Example.org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Remove("l1", "User@Example.org"); err != nil {
		t.Fatalf("Remove absent: %v", err)
	}
	if s := recipientStatus(t, db, "r1"); s != "active" {
		t.Errorf("status after Remove = %s", s)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM unsubscribes`).Scan(&n)
	if n != 0 {
		t.Errorf("journal rows = %d", n)
	}
	if got := activeCount(t, db, "l1"); got != 2 {
		t.Errorf("l1 active_count = %d", got)
	}
	if ok, _ := repo.IsUnsubscribed("l1", "user@example.org"); ok {
		t.Error("still unsubscribed")
	}
}
