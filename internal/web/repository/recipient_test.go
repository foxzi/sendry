package repository

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

// The app lowers emails the way SQLite lower() does (ASCII only), so the
// unique (list_id, lower(email)) index and the lookups agree on non-ASCII.
func TestNormalizeEmailMatchesSQLiteLower(t *testing.T) {
	db := setupTestDB(t)
	for _, e := range []string{"  User@Example.COM ", "Ä@X.com", "ÄÖ@Пример.РФ"} {
		var lower string
		if err := db.QueryRow(`SELECT lower(trim(?))`, e).Scan(&lower); err != nil {
			t.Fatal(err)
		}
		if got := NormalizeEmail(e); got != lower {
			t.Errorf("NormalizeEmail(%q) = %q, SQLite lower = %q", e, got, lower)
		}
	}
}

func TestRecipientRepository_UpsertAndCounts(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`)
	repo := NewRecipientRepository(db)
	repo.AddRecipient(&models.Recipient{ListID: "l1", Email: "u@x.com", Name: "U", Variables: `{"a":"1"}`})
	repo.AddRecipient(&models.Recipient{ListID: "l1", Email: "gone@x.com", Status: "unsubscribed"})

	res, err := repo.UpsertRecipients("l1", []RecipientUpsert{
		{Email: "u@x.com", Name: "", Variables: map[string]any{"b": "2"}},
		{Email: "gone@x.com", Name: "N"},
		{Email: "new@x.com", Name: "New"},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if *res != (UpsertResult{Added: 1, Updated: 1, Unsubscribed: 1}) {
		t.Errorf("res = %+v", res)
	}
	var name, vars, status string
	db.QueryRow(`SELECT name, variables, status FROM recipients WHERE email='u@x.com'`).Scan(&name, &vars, &status)
	if name != "U" || !strings.Contains(vars, `"a":"1"`) || !strings.Contains(vars, `"b":"2"`) {
		t.Errorf("merge: %q %q", name, vars)
	}
	db.QueryRow(`SELECT status FROM recipients WHERE email='gone@x.com'`).Scan(&status)
	if status != "unsubscribed" {
		t.Errorf("reactivated: %s", status)
	}

	counts, err := repo.ListCountsByList()
	if err != nil {
		t.Fatal(err)
	}
	if counts["l1"] != (ListCounts{Total: 3, Active: 2, Unsubscribed: 1}) {
		t.Errorf("counts = %+v", counts)
	}

	if err := repo.DeleteRecipientByEmail("l1", "NEW@x.com"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteRecipientByEmail("l1", "new@x.com"); err != ErrRecipientNotInList {
		t.Errorf("err = %v", err)
	}
	if err := repo.DeleteRecipientByEmail("l1", "gone@x.com"); err != ErrRecipientUnsubscribed {
		t.Errorf("err = %v", err)
	}
	var total int
	db.QueryRow(`SELECT total_count FROM recipient_lists WHERE id='l1'`).Scan(&total)
	if total != 2 {
		t.Errorf("total_count = %d", total)
	}
}

// A file of tens of thousands of rows must import well within the 30s
// request timeout; rows past a batch boundary and duplicates must be kept right.
func TestRecipientRepository_ImportCSVLarge(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`)
	repo := NewRecipientRepository(db)

	const n = 15000
	var b strings.Builder
	b.WriteString("email,name\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "user%d@example.org,User %d\n", i, i)
	}
	// the same addresses again in another case, and an empty email
	b.WriteString("USER0@example.org,Renamed\nUser14999@Example.org,\n,nobody\n")

	start := time.Now()
	res, err := repo.ImportCSV("l1", strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("imported %d rows in %s", res.Total, time.Since(start))

	if res.Total != n+3 || res.Imported != n+2 || res.Skipped != 1 || len(res.Errors) != 0 {
		t.Fatalf("result: total=%d imported=%d skipped=%d errors=%v", res.Total, res.Imported, res.Skipped, res.Errors)
	}
	var total, active int
	if err := db.QueryRow(`SELECT total_count, active_count FROM recipient_lists WHERE id = 'l1'`).Scan(&total, &active); err != nil {
		t.Fatal(err)
	}
	if total != n || active != n {
		t.Errorf("list counts: total=%d active=%d, want %d", total, active, n)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM recipients WHERE email = 'user0@example.org'`).Scan(&name); err != nil || name != "Renamed" {
		t.Errorf("duplicate row should update the name: %q %v", name, err)
	}
	if err := db.QueryRow(`SELECT name FROM recipients WHERE email = 'user14999@example.org'`).Scan(&name); err != nil || name != "User 14999" {
		t.Errorf("empty name should keep the old one: %q %v", name, err)
	}
}
