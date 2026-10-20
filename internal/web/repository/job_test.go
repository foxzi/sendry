package repository

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/stats"
)

// setupJobWithItems creates a job with one item per given status/error pair and returns the job ID
func setupJobWithItems(t *testing.T, db *sql.DB, items [][2]string) string {
	t.Helper()
	mustExec(t, db, `INSERT INTO recipient_lists (id, name, source_type) VALUES ('l1', 'list', 'manual')`)
	mustExec(t, db, `INSERT INTO campaigns (id, name, from_email) VALUES ('c1', 'camp', 'a@example.com')`)
	mustExec(t, db, `INSERT INTO templates (id, name, subject) VALUES ('t1', 'tpl', 's')`)
	mustExec(t, db, `INSERT INTO campaign_variants (id, campaign_id, name, template_id) VALUES ('v1', 'c1', 'A', 't1')`)
	mustExec(t, db, `INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status) VALUES ('job-1', 'c1', 'l1', 'running')`)

	repo := NewJobRepository(db)
	for i, it := range items {
		rid := fmt.Sprintf("r%d", i)
		mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES (?, 'l1', ?)`, rid, fmt.Sprintf("user%d@example.org", i))
		item := &models.SendJobItem{JobID: "job-1", RecipientID: rid, VariantID: "v1", ServerName: "mx1"}
		if err := repo.CreateItem(item); err != nil {
			t.Fatalf("CreateItem: %v", err)
		}
		if it[0] != "pending" {
			if err := repo.UpdateItemStatus(item.ID, it[0], "msg-"+rid, it[1]); err != nil {
				t.Fatalf("UpdateItemStatus: %v", err)
			}
		}
	}
	return "job-1"
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestJobRepository_ListItemsWithPending(t *testing.T) {
	db := setupTestDB(t)
	jobID := setupJobWithItems(t, db, [][2]string{{"pending", ""}, {"sent", ""}})

	items, total, err := NewJobRepository(db).ListItems(models.JobItemFilter{JobID: jobID, Limit: 10})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("got total=%d len=%d, want 2 and 2", total, len(items))
	}
}

const (
	errNotFound = "RCPT TO x failed: 550 5.1.1 User unknown"
	errFull     = "RCPT TO x failed: 552 5.2.2 Mailbox full"
	errCert     = "MAIL FROM failed: tls: failed to verify certificate: x509: certificate has expired"
)

func TestJobRepository_ErrorCounts(t *testing.T) {
	db := setupTestDB(t)
	jobID := setupJobWithItems(t, db, [][2]string{
		{"failed", errNotFound}, {"failed", errNotFound}, {"failed", errFull},
		{"queued", errCert}, {"sent", ""}, {"pending", ""},
	})

	counts, err := NewJobRepository(db).ErrorCounts(jobID)
	if err != nil {
		t.Fatalf("ErrorCounts() error = %v", err)
	}

	got := map[string]int{}
	for _, c := range counts {
		got[c.Status+"|"+c.Error] = c.Count
	}
	want := map[string]int{"failed|" + errNotFound: 2, "failed|" + errFull: 1, "queued|" + errCert: 1}
	if len(got) != len(want) {
		t.Fatalf("ErrorCounts() = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("count[%q] = %d, want %d", k, got[k], v)
		}
	}
}

func TestJobRepository_ListItemsByErrorType(t *testing.T) {
	db := setupTestDB(t)
	jobID := setupJobWithItems(t, db, [][2]string{
		{"failed", errNotFound}, {"failed", errFull}, {"failed", errNotFound},
		{"failed", errNotFound}, {"queued", errNotFound}, {"sent", ""}, {"pending", ""},
	})
	repo := NewJobRepository(db)

	items, total, err := repo.ListItems(models.JobItemFilter{JobID: jobID, Status: "failed", ErrorType: stats.ErrorAddressNotFound, Limit: 2})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("page 1: total=%d len=%d, want 3 and 2", total, len(items))
	}
	for _, it := range items {
		if it.Error != errNotFound || it.Status != "failed" {
			t.Errorf("unexpected item %+v", it)
		}
	}

	items, total, err = repo.ListItems(models.JobItemFilter{JobID: jobID, Status: "failed", ErrorType: stats.ErrorAddressNotFound, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListItems() page 2 error = %v", err)
	}
	if total != 3 || len(items) != 1 {
		t.Errorf("page 2: total=%d len=%d, want 3 and 1", total, len(items))
	}

	items, total, err = repo.ListItems(models.JobItemFilter{JobID: jobID, ErrorType: stats.ErrorAddressNotFound, Limit: 10, Offset: 50})
	if err != nil {
		t.Fatalf("ListItems() past end error = %v", err)
	}
	if total != 4 || len(items) != 0 {
		t.Errorf("past end: total=%d len=%d, want 4 and 0", total, len(items))
	}
}

func TestJobRepository_GetQueuedItemsCursor(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{
		{"queued", ""}, {"sent", ""}, {"queued", errCert}, {"queued", ""}, {"pending", ""},
	})
	repo := NewJobRepository(db)

	first, cursor, err := repo.GetQueuedItems(0, 2)
	if err != nil {
		t.Fatalf("GetQueuedItems() error = %v", err)
	}
	if len(first) != 2 || first[1].Error != errCert {
		t.Fatalf("first batch = %+v, want 2 items, second with error", first)
	}

	rest, _, err := repo.GetQueuedItems(cursor, 2)
	if err != nil {
		t.Fatalf("GetQueuedItems() error = %v", err)
	}
	if len(rest) != 1 || rest[0].ID == first[0].ID || rest[0].ID == first[1].ID {
		t.Errorf("second batch = %+v, want the one remaining item", rest)
	}
}

func TestJobRepository_UpdateItemStatusKeepsQueuedAt(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{{"queued", ""}})
	repo := NewJobRepository(db)

	items, _, err := repo.ListItems(models.JobItemFilter{JobID: "job-1"})
	if err != nil || len(items) != 1 || items[0].QueuedAt == nil {
		t.Fatalf("ListItems() = %+v, %v", items, err)
	}
	queuedAt := *items[0].QueuedAt

	time.Sleep(10 * time.Millisecond)
	if err := repo.UpdateItemStatus(items[0].ID, "queued", items[0].SendryMsgID, errCert); err != nil {
		t.Fatal(err)
	}

	items, _, _ = repo.ListItems(models.JobItemFilter{JobID: "job-1"})
	if !items[0].QueuedAt.Equal(queuedAt) {
		t.Errorf("queued_at changed from %v to %v", queuedAt, items[0].QueuedAt)
	}
	if items[0].Error != errCert {
		t.Errorf("error = %q, want %q", items[0].Error, errCert)
	}
}
