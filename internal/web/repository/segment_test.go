package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestSegment(t *testing.T) {
	db := setupTestDB(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for _, q := range []string{
		`INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'subs', 'manual'), ('l2', 'buyers', 'manual')`,
		// created_at in the forms the panel stores: a Go time with an offset, an SQLite timestamp, a plain date
		`INSERT INTO recipients (id, list_id, email, name, variables, tags, status, created_at) VALUES
			('ra', 'l1', 'ann@gmail.com', 'Ann', '{"city":"Москва","orders":12}', '["vip","new"]', 'active', '2026-02-25 10:00:00.5+03:00'),
			('rb', 'l1', 'bob@example.org', '', '{"city":"Kazan","orders":"3"}', '["new"]', 'active', '2026-01-10 08:00:00'),
			('rc', 'l1', 'cat@gmail.com', NULL, NULL, NULL, 'active', '2025-06-01'),
			('rd', 'l1', 'dan@example.org', '', 'not json', 'not json', 'unsubscribed', '2026-02-20 00:00:00'),
			('rx', 'l2', 'ann@gmail.com', '', '', '', 'active', '2025-01-01')`,
		`INSERT INTO campaigns (id, name, description, from_email, from_name, reply_to, variables, tags) VALUES ('c1', 'camp', '', 'n@example.com', '', '', '', '')`,
		`INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status) VALUES ('j1', 'c1', 'l1', 'completed'), ('j2', 'c1', 'l2', 'completed')`,
		// ann opened a letter sent through the other list; bob got one long ago and clicked; cat got nothing
		`INSERT INTO send_job_items (id, job_id, recipient_id, status, sent_at, opened_at, clicked_at) VALUES
			('i1', 'j2', 'rx', 'sent', '2026-02-20 10:00:00', '2026-02-21 09:00:00', NULL),
			('i2', 'j1', 'rb', 'sent', '2025-11-01 10:00:00', '2025-11-02 10:00:00', '2025-11-02 10:01:00'),
			('i3', 'j1', 'rd', 'skipped', NULL, NULL, NULL)`,
	} {
		mustExec(t, db, q)
	}
	repo := NewRecipientRepository(db)

	cond := func(field, op, key, value string) models.SegmentCondition {
		return models.SegmentCondition{Field: field, Op: op, Key: key, Value: value}
	}
	cases := []struct {
		name  string
		match string
		conds []models.SegmentCondition
		want  string // matching recipient ids of l1, any status, sorted
	}{
		{"added last 10 days", "all", []models.SegmentCondition{cond("added", "last_days", "", "10")}, "ra rd"},
		{"added after", "all", []models.SegmentCondition{cond("added", "after", "", "2026-01-10")}, "ra rb rd"},
		{"added before", "all", []models.SegmentCondition{cond("added", "before", "", "2026-01-10")}, "rc"},
		{"tag has", "all", []models.SegmentCondition{cond("tag", "has", "", "vip")}, "ra"},
		{"tag not has", "all", []models.SegmentCondition{cond("tag", "not_has", "", "vip")}, "rb rc rd"},
		{"var eq ignores case", "all", []models.SegmentCondition{cond("var", "eq", "city", "москва")}, "ra"},
		{"var ne counts unset", "all", []models.SegmentCondition{cond("var", "ne", "city", "Kazan")}, "ra rc rd"},
		{"var contains", "all", []models.SegmentCondition{cond("var", "contains", "city", "AZ")}, "rb"},
		{"var gt number and numeric string", "all", []models.SegmentCondition{cond("var", "gt", "orders", "2")}, "ra rb"},
		{"var lt skips unset", "all", []models.SegmentCondition{cond("var", "lt", "orders", "5")}, "rb"},
		{"var set", "all", []models.SegmentCondition{cond("var", "set", "city", "")}, "ra rb"},
		{"var not set", "all", []models.SegmentCondition{cond("var", "not_set", "city", "")}, "rc rd"},
		{"email contains", "all", []models.SegmentCondition{cond("email", "contains", "", "@GMAIL.com")}, "ra rc"},
		{"email not contains", "all", []models.SegmentCondition{cond("email", "not_contains", "", "gmail")}, "rb rd"},
		{"opened in any list", "all", []models.SegmentCondition{cond("opened", "last_days", "", "30")}, "ra"},
		{"not opened lately", "all", []models.SegmentCondition{cond("opened", "not_last_days", "", "30")}, "rb rc rd"},
		{"opened never", "all", []models.SegmentCondition{cond("opened", "never", "", "")}, "rc rd"},
		{"opened before", "all", []models.SegmentCondition{cond("opened", "before", "", "2026-01-01")}, "rb"},
		{"clicked after", "all", []models.SegmentCondition{cond("clicked", "after", "", "2025-01-01")}, "rb"},
		{"received never", "all", []models.SegmentCondition{cond("received", "never", "", "")}, "rc rd"},
		{"all of two", "all", []models.SegmentCondition{cond("tag", "has", "", "new"), cond("email", "contains", "", "gmail")}, "ra"},
		{"any of two", "any", []models.SegmentCondition{cond("tag", "has", "", "vip"), cond("added", "before", "", "2026-01-01")}, "ra rc"},
	}
	for _, c := range cases {
		seg := &models.Segment{Match: c.match, Conditions: c.conds}
		if err := seg.Validate(); err != nil {
			t.Errorf("%s: validate: %v", c.name, err)
			continue
		}
		where, args, err := segmentWhere(seg, now)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		rows, err := db.Query(`SELECT r.id FROM recipients r WHERE r.list_id = ?`+where+` ORDER BY r.id`, append([]any{"l1"}, args...)...)
		if err != nil {
			t.Errorf("%s: query: %v", c.name, err)
			continue
		}
		var got []string
		for rows.Next() {
			var id string
			rows.Scan(&id)
			got = append(got, id)
		}
		rows.Close()
		if strings.Join(got, " ") != c.want {
			t.Errorf("%s: got %v, want %s", c.name, got, c.want)
		}
	}

	// the count is of active recipients only; no segment is the whole list
	seg := &models.Segment{Match: "all", Conditions: []models.SegmentCondition{cond("email", "contains", "", "example.org")}}
	if n, err := repo.CountSegment("l1", seg); err != nil || n != 1 {
		t.Errorf("CountSegment = %d, %v; want 1", n, err)
	}
	if n, err := repo.CountSegment("l1", nil); err != nil || n != 3 {
		t.Errorf("CountSegment(nil) = %d, %v; want 3", n, err)
	}

	stream, err := repo.StreamSegment("l1", seg)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for stream.Next() {
		rec, err := repo.ScanRecipient(stream)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ID)
	}
	stream.Close()
	// list order (created_at DESC), unsubscribed included: the job skips them itself
	if strings.Join(ids, " ") != "rd rb" {
		t.Errorf("StreamSegment = %v, want rd rb", ids)
	}

	keys, err := repo.GetVariableKeys("l1")
	if err != nil || strings.Join(keys, " ") != "city orders" {
		t.Errorf("GetVariableKeys = %v, %v", keys, err)
	}
}

func TestParseSegment(t *testing.T) {
	if seg, err := models.ParseSegment("  "); seg != nil || err != nil {
		t.Errorf("empty = %v, %v", seg, err)
	}
	seg, err := models.ParseSegment(`{"conditions":[{"field":"tag","op":"has","key":"x","value":" vip "}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := seg.JSON(); got != `{"match":"all","conditions":[{"field":"tag","op":"has","value":"vip"}]}` {
		t.Errorf("normalized = %s", got)
	}
	for _, raw := range []string{
		`[]`,
		`{"match":"all","conditions":[]}`,
		`{"match":"some","conditions":[{"field":"tag","op":"has","value":"a"}]}`,
		`{"conditions":[{"field":"status","op":"eq","value":"a"}]}`,
		`{"conditions":[{"field":"tag","op":"eq","value":"a"}]}`,
		`{"conditions":[{"field":"tag","op":"has","value":""}]}`,
		`{"conditions":[{"field":"added","op":"last_days","value":"0"}]}`,
		`{"conditions":[{"field":"added","op":"after","value":"01.06.2025"}]}`,
		`{"conditions":[{"field":"var","op":"eq","value":"a"}]}`,
		`{"conditions":[{"field":"var","op":"eq","key":"a\"b","value":"a"}]}`,
		`{"conditions":[{"field":"var","op":"gt","key":"n","value":"many"}]}`,
		`{"conditions":[{"field":"var","op":"gt","key":"n","value":"NaN"}]}`,
		`{"conditions":[{"field":"var","op":"lt","key":"n","value":"Inf"}]}`,
		`{"conditions":[{"field":"tag","op":"has","value":"` + strings.Repeat("я", 201) + `"}]}`,
	} {
		if _, err := models.ParseSegment(raw); err == nil {
			t.Errorf("%s: accepted", raw)
		}
	}
	// the limit is in characters, not bytes
	if _, err := models.ParseSegment(`{"conditions":[{"field":"tag","op":"has","value":"` + strings.Repeat("я", 200) + `"}]}`); err != nil {
		t.Errorf("200 Cyrillic characters: %v", err)
	}
}

// Variable conditions compare the value as the recipient card shows it, and
// "greater/less than" only numbers
func TestSegmentVariableValues(t *testing.T) {
	db := setupTestDB(t)
	for _, q := range []string{
		`INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'subs', 'manual')`,
		`INSERT INTO recipients (id, list_id, email, variables, status) VALUES
			('r1', 'l1', 'a@x.com', '{"price":10.00,"vip":true,"id":12345678901234567890,"age":"abc"}', 'active'),
			('r2', 'l1', 'b@x.com', '{"price":"10","vip":false,"age":"10,5"}', 'active'),
			('r3', 'l1', 'c@x.com', '{"price":null,"age":"  7 "}', 'active'),
			('r4', 'l1', 'd@x.com', '{"age":"NaN"}', 'active')`,
	} {
		mustExec(t, db, q)
	}
	cases := []struct{ op, key, value, want string }{
		{"eq", "price", "10.00", "r1"},
		{"eq", "price", "10", "r2"},
		{"eq", "vip", "TRUE", "r1"},
		{"ne", "vip", "true", "r2 r3 r4"},
		{"eq", "id", "12345678901234567890", "r1"},
		{"not_set", "price", "", "r3 r4"},
		{"lt", "age", "100", "r2 r3"},
		{"gt", "age", "10,2", "r2"},
		{"lt", "age", "1", ""},
	}
	for _, c := range cases {
		seg := &models.Segment{Conditions: []models.SegmentCondition{{Field: "var", Op: c.op, Key: c.key, Value: c.value}}}
		if err := seg.Validate(); err != nil {
			t.Fatalf("%s %s %s: %v", c.key, c.op, c.value, err)
		}
		where, args, err := segmentWhere(seg, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		rows, err := db.Query(`SELECT r.id FROM recipients r WHERE r.list_id = 'l1'`+where+` ORDER BY r.id`, args...)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var id string
			rows.Scan(&id)
			got = append(got, id)
		}
		rows.Close()
		if strings.Join(got, " ") != c.want {
			t.Errorf("%s %s %q: got %v, want %q", c.key, c.op, c.value, got, c.want)
		}
	}
}
