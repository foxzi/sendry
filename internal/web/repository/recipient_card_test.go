package repository

import (
	"strings"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestRecipientCard(t *testing.T) {
	db := setupTestDB(t)
	for _, q := range []string{
		`INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'subs', 'manual'), ('l2', 'buyers', 'manual')`,
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES
			('r1', 'l1', 'a@example.org', 'Ann', '{"city":"Moscow"}', '["vip"]', 'active', '2026-01-01'),
			('r2', 'l2', 'a@example.org', NULL, NULL, NULL, 'unsubscribed', '2026-01-02'),
			('r3', 'l1', 'b@example.org', '', '', '', 'active', '2026-01-03')`,
		`INSERT INTO campaigns (id, name, description, from_email, from_name, reply_to, variables, tags) VALUES ('c1', 'camp', '', 'n@example.com', '', '', '', '')`,
		`INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status) VALUES ('j1', 'c1', 'l1', 'completed'), ('j2', 'c1', 'l2', 'completed')`,
		`INSERT INTO send_job_items (id, job_id, recipient_id, status, sent_at, created_at, opened_at, open_count, clicked_at, click_count) VALUES
			('i1', 'j1', 'r1', 'sent', '2026-02-01 10:00:00', '2026-02-01 09:00:00', '2026-02-01 11:00:00', 2, '2026-02-01 11:05:00', 1),
			('i2', 'j2', 'r2', 'sent', '2026-02-03 10:00:00', '2026-02-03 09:00:00', NULL, 0, NULL, 0),
			('i3', 'j2', 'r2', 'skipped', NULL, '2026-02-04 09:00:00', NULL, 0, NULL, 0),
			('i4', 'j1', 'r3', 'sent', '2026-02-05 10:00:00', '2026-02-05 09:00:00', NULL, 0, NULL, 0)`,
		`INSERT INTO email_events (type, source_type, source_id, campaign_id, job_id, email, url, created_at) VALUES
			('open', 'job_item', 'i1', 'c1', 'j1', 'a@example.org', '', '2026-02-01 11:00:00'),
			('click', 'job_item', 'i1', 'c1', 'j1', 'a@example.org', 'https://example.com', '2026-02-01 11:05:00'),
			('open', 'job_item', 'i4', 'c1', 'j1', 'b@example.org', '', '2026-02-05 11:00:00'),
			('open', 'send', 'i1', NULL, NULL, 'a@example.org', '', '2026-02-06 11:00:00')`,
	} {
		mustExec(t, db, q)
	}
	repo := NewRecipientRepository(db)

	rec, err := repo.GetRecipient("l2", "r2")
	if err != nil || rec == nil {
		t.Fatalf("GetRecipient = %v, %v", rec, err)
	}
	if rec.Email != "a@example.org" || rec.Name != "" || rec.Status != "unsubscribed" {
		t.Errorf("recipient = %+v", rec)
	}
	// a recipient of another list is not found
	if rec, err := repo.GetRecipient("l1", "r2"); err != nil || rec != nil {
		t.Errorf("GetRecipient(other list) = %v, %v", rec, err)
	}

	name, vars, tags := "Anna", `{"city":"Kazan"}`, `["new"]`
	if err := repo.UpdateRecipientFields("l1", "r2", RecipientFields{Name: &name}); err != nil { // wrong list
		t.Fatal(err)
	}
	if rec, _ := repo.GetRecipient("l2", "r2"); rec.Name != "" {
		t.Errorf("renamed through another list: %q", rec.Name)
	}
	if err := repo.UpdateRecipientFields("l2", "r2", RecipientFields{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateRecipientFields("l2", "r2", RecipientFields{}); err != nil {
		t.Fatal(err)
	}
	if rec, _ := repo.GetRecipient("l2", "r2"); rec.Name != "Anna" || rec.Variables != "" || rec.Tags != "" {
		t.Errorf("after rename = %+v", rec)
	}
	// a nil field is left as it is
	if err := repo.UpdateRecipientFields("l1", "r1", RecipientFields{Variables: &vars, Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	if rec, _ := repo.GetRecipient("l1", "r1"); rec.Name != "Ann" || rec.Variables != vars || rec.Tags != tags {
		t.Errorf("after fields = %+v", rec)
	}

	if rec, err := repo.GetRecipientByEmail("l1", " A@Example.ORG "); err != nil || rec == nil || rec.ID != "r1" {
		t.Errorf("GetRecipientByEmail = %+v, %v", rec, err)
	}
	if rec, err := repo.GetRecipientByEmail("l1", "nobody@example.org"); err != nil || rec != nil {
		t.Errorf("GetRecipientByEmail(unknown) = %+v, %v", rec, err)
	}

	// part of an address, any case, in all lists; LIKE wildcards are literal
	found, err := repo.FindByEmail("A@EXAMPLE", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].ListName != "buyers" || found[0].Name != "Anna" || found[1].RecipientID != "r1" || found[1].Email != "a@example.org" {
		t.Errorf("found = %+v", found)
	}
	if found, _ := repo.FindByEmail("example.org", 2); len(found) != 2 {
		t.Errorf("limit: %d rows", len(found))
	}
	if found, _ := repo.FindByEmail("%", 10); len(found) != 0 {
		t.Errorf("percent matched %d rows", len(found))
	}

	ms, err := repo.Memberships("A@Example.org")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].ListName != "buyers" || ms[0].Status != "unsubscribed" || ms[1].ListName != "subs" {
		t.Errorf("memberships = %+v", ms)
	}

	tot, err := repo.MessageTotals("a@example.org")
	if err != nil {
		t.Fatal(err)
	}
	if tot.Messages != 3 || tot.Sent != 2 || tot.Opened != 1 || tot.Clicked != 1 {
		t.Errorf("totals = %+v", tot)
	}
	if tot.LastSent == nil || tot.LastSent.Format("2006-01-02") != "2026-02-03" {
		t.Errorf("last sent = %v", tot.LastSent)
	}
	if tot, err := repo.MessageTotals("nobody@example.org"); err != nil || tot.Messages != 0 || tot.LastSent != nil {
		t.Errorf("totals of unknown = %+v, %v", tot, err)
	}

	msgs, err := repo.Messages("a@example.org", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[0].ItemID != "i3" || msgs[1].ItemID != "i2" || msgs[2].ItemID != "i1" {
		t.Fatalf("messages = %+v", msgs)
	}
	m := msgs[2]
	if m.CampaignName != "camp" || m.ListName != "subs" || m.OpenCount != 2 || m.OpenedAt == nil || m.ClickCount != 1 ||
		m.At.Format("15:04") != "10:00" {
		t.Errorf("message = %+v", m)
	}
	if msgs, _ := repo.Messages("a@example.org", 1); len(msgs) != 1 {
		t.Errorf("limit: %d messages", len(msgs))
	}

	if evs, err := repo.Events(nil, 10); err != nil || len(evs) != 0 {
		t.Errorf("events of no messages = %v, %v", evs, err)
	}
	// i4 is a message to another address: its events are not asked for
	evs, err := repo.Events([]string{"i1", "i2", "i3"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	// newest first; events of API sends are left out
	if len(evs) != 2 || evs[0].Type != models.EventClick || evs[0].CampaignName != "camp" || evs[1].Type != models.EventOpen {
		t.Errorf("events = %+v", evs)
	}
	// the message knows all its events, the card gets the latest of each message
	if m.EventTotal != 2 || msgs[1].EventTotal != 0 {
		t.Errorf("event totals = %d, %d", m.EventTotal, msgs[1].EventTotal)
	}
	mustExec(t, db, `INSERT INTO email_events (type, source_type, source_id, campaign_id, job_id, email, url, created_at) VALUES
		('open', 'job_item', 'i2', 'c1', 'j2', 'a@example.org', '', '2026-02-03 11:00:00'),
		('open', 'job_item', 'i2', 'c1', 'j2', 'a@example.org', '', '2026-02-03 12:00:00'),
		('click', 'job_item', 'i2', 'c1', 'j2', 'a@example.org', 'https://example.com', '2026-02-03 13:00:00')`)
	evs, err = repo.Events([]string{"i1", "i2", "i3"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].SourceID != "i2" || evs[0].Type != models.EventClick || evs[1].SourceID != "i1" || evs[1].Type != models.EventClick {
		t.Errorf("one event per message = %+v", evs)
	}
}

func TestUnsubscribeGet(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email, status) VALUES ('r1', 'l1', 'a@example.org', 'active')`)
	repo := NewUnsubscribeRepository(db)

	if u, err := repo.Get("l1", "a@example.org"); err != nil || u != nil {
		t.Fatalf("Get before = %v, %v", u, err)
	}
	if _, err := repo.Add(&models.Unsubscribe{ListID: "l1", Email: "A@example.org", SourceType: models.UnsubSourceManual}); err != nil {
		t.Fatal(err)
	}
	u, err := repo.Get("l1", "A@Example.org")
	if err != nil || u == nil {
		t.Fatalf("Get = %v, %v", u, err)
	}
	if u.SourceType != models.UnsubSourceManual || u.RecipientID != "r1" || u.CampaignID != "" || u.CreatedAt.IsZero() {
		t.Errorf("unsubscribe = %+v", u)
	}
}

// An unsubscribe made for the reader (panel, API) never rewrites the
// reader's own one and never touches another status; the API takes back
// only what was not the reader's doing.
func TestUnsubscribeGuards(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email, status) VALUES
		('r1', 'l1', 'a@example.org', 'active'), ('r2', 'l1', 'b@example.org', 'bounced'), ('r3', 'l1', 'c@example.org', NULL)`)
	repo := NewUnsubscribeRepository(db)
	status := func(id string) string {
		t.Helper()
		var s string
		if err := db.QueryRow(`SELECT COALESCE(status, '') FROM recipients WHERE id = ?`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// the reader unsubscribes first; the API call that saw "active" comes second
	if _, err := repo.Add(&models.Unsubscribe{ListID: "l1", Email: "a@example.org", SourceType: models.UnsubSourceLink, CampaignID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.AddIfActive(&models.Unsubscribe{ListID: "l1", Email: "a@example.org", SourceType: models.UnsubSourceAPI}); err != nil || changed {
		t.Fatalf("AddIfActive on an unsubscribed address = %v, %v", changed, err)
	}
	if u, _ := repo.Get("l1", "a@example.org"); u == nil || u.SourceType != models.UnsubSourceLink || u.CampaignID != "c1" {
		t.Errorf("the reader's journal row was rewritten: %+v", u)
	}
	if removed, err := repo.RemoveIfSource("l1", "a@example.org", models.UnsubSourceAPI); err != nil || removed {
		t.Errorf("RemoveIfSource undid the reader's unsubscribe: %v, %v", removed, err)
	}
	if status("r1") != "unsubscribed" {
		t.Errorf("status = %q", status("r1"))
	}

	// bounced is left alone; a row without a status counts as active
	if changed, err := repo.AddIfActive(&models.Unsubscribe{ListID: "l1", Email: "b@example.org", SourceType: models.UnsubSourceManual}); err != nil || changed || status("r2") != "bounced" {
		t.Errorf("bounced: changed %v, err %v, status %q", changed, err, status("r2"))
	}
	if u, _ := repo.Get("l1", "b@example.org"); u != nil {
		t.Errorf("journal row for a bounced address: %+v", u)
	}
	if changed, err := repo.AddIfActive(&models.Unsubscribe{ListID: "l1", Email: "c@example.org", SourceType: models.UnsubSourceAPI}); err != nil || !changed {
		t.Fatalf("AddIfActive = %v, %v", changed, err)
	}
	if removed, err := repo.RemoveIfSource("l1", "c@example.org", models.UnsubSourceAPI); err != nil || !removed || status("r3") != "active" {
		t.Errorf("RemoveIfSource: removed %v, err %v, status %q", removed, err, status("r3"))
	}
	if removed, err := repo.RemoveIfSource("l1", "c@example.org", models.UnsubSourceAPI); err != nil || removed {
		t.Errorf("repeat: removed %v, err %v", removed, err)
	}
	if _, err := repo.AddIfActive(&models.Unsubscribe{ListID: "l1", Email: "nobody@example.org", SourceType: models.UnsubSourceAPI}); err != ErrRecipientNotInList {
		t.Errorf("unknown address: %v", err)
	}
}

// An import of a file with only addresses and names keeps the variables and
// the tags set on the address card.
func TestImportCSV_KeepsVariablesAndTags(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email, name, variables, tags, status) VALUES
		('r1', 'l1', 'a@example.org', 'Ann', '{"city":"Kazan"}', '["vip"]', 'unsubscribed'),
		('r2', 'l1', 'b@example.org', 'Bob', '{"city":"Omsk"}', '["new"]', 'active')`)
	repo := NewRecipientRepository(db)

	res, err := repo.ImportCSV("l1", strings.NewReader("email,name\nA@Example.org,Anna\nb@example.org,\nc@example.org,Carl\n"))
	if err != nil || res.Imported != 3 {
		t.Fatalf("import = %+v, %v", res, err)
	}
	a, _ := repo.GetRecipient("l1", "r1")
	b, _ := repo.GetRecipient("l1", "r2")
	if a.Name != "Anna" || a.Variables != `{"city":"Kazan"}` || a.Tags != `["vip"]` || a.Status != "unsubscribed" {
		t.Errorf("a = %+v", a)
	}
	if b.Name != "Bob" || b.Variables != `{"city":"Omsk"}` || b.Tags != `["new"]` {
		t.Errorf("b = %+v", b)
	}
}
