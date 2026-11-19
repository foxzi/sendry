package repository

import (
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

	if err := repo.UpdateRecipientName("l1", "r2", "wrong list"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateRecipientName("l2", "r2", "Anna"); err != nil {
		t.Fatal(err)
	}
	if rec, _ := repo.GetRecipient("l2", "r2"); rec.Name != "Anna" {
		t.Errorf("name = %q", rec.Name)
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

	evs, err := repo.Events("a@example.org", 10)
	if err != nil {
		t.Fatal(err)
	}
	// newest first; events of API sends and of other addresses are left out
	if len(evs) != 2 || evs[0].Type != models.EventClick || evs[0].CampaignName != "camp" || evs[1].Type != models.EventOpen {
		t.Errorf("events = %+v", evs)
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
