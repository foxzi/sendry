package repository

import (
	"database/sql"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/stats"
)

// createSends creates one send per given status/error pair, the way the API router does it
func createSends(t *testing.T, db *sql.DB, sends [][2]string) []string {
	t.Helper()
	repo := NewSendRepository(db)
	ids := make([]string, 0, len(sends))
	for _, s := range sends {
		send := &models.Send{
			FromAddress:  "shop@example.com",
			ToAddresses:  `["user@example.org"]`,
			SenderDomain: "example.com",
			ServerName:   "mx1",
			Status:       models.SendStatusPending,
		}
		if err := repo.Create(send); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if s[0] != models.SendStatusPending {
			if err := repo.UpdateStatus(send.ID, s[0], s[1], "msg-"+send.ID, nil); err != nil {
				t.Fatalf("UpdateStatus: %v", err)
			}
		}
		ids = append(ids, send.ID)
	}
	return ids
}

func TestSendRepository_ErrorCountsAndStats(t *testing.T) {
	db := setupTestDB(t)
	createSends(t, db, [][2]string{
		{"failed", errNotFound}, {"failed", errNotFound}, {"queued", errCert},
		{"queued", ""}, {"sent", ""}, {"pending", ""},
	})
	repo := NewSendRepository(db)

	counts, err := repo.ErrorCounts()
	if err != nil {
		t.Fatalf("ErrorCounts() error = %v", err)
	}
	got := map[string]int{}
	for _, c := range counts {
		got[c.Status+"|"+c.Error] = c.Count
	}
	if len(got) != 2 || got["failed|"+errNotFound] != 2 || got["queued|"+errCert] != 1 {
		t.Errorf("ErrorCounts() = %v", got)
	}

	st, err := repo.GetStats(models.SendFilter{})
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if st.Total != 6 || st.Pending != 1 || st.Queued != 2 || st.Sent != 1 || st.Failed != 2 {
		t.Errorf("GetStats() = %+v", st)
	}
}

func TestSendRepository_ListByErrorType(t *testing.T) {
	db := setupTestDB(t)
	createSends(t, db, [][2]string{
		{"failed", errNotFound}, {"failed", errFull}, {"failed", errNotFound},
		{"failed", errNotFound}, {"queued", errNotFound}, {"sent", ""},
	})
	repo := NewSendRepository(db)

	sends, total, err := repo.List(models.SendFilter{Status: "failed", ErrorType: stats.ErrorAddressNotFound, Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 3 || len(sends) != 2 {
		t.Fatalf("page 1: total=%d len=%d, want 3 and 2", total, len(sends))
	}

	sends, total, err = repo.List(models.SendFilter{ErrorType: stats.ErrorAddressNotFound, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List() page 2 error = %v", err)
	}
	if total != 4 || len(sends) != 2 {
		t.Errorf("page 2: total=%d len=%d, want 4 and 2", total, len(sends))
	}
}

func TestSendRepository_GetQueuedSendsCursor(t *testing.T) {
	db := setupTestDB(t)
	ids := createSends(t, db, [][2]string{
		{"queued", ""}, {"sent", ""}, {"queued", errCert}, {"failed", errFull}, {"queued", ""}, {"pending", ""},
	})
	repo := NewSendRepository(db)

	first, cursor, err := repo.GetQueuedSends(0, 2)
	if err != nil {
		t.Fatalf("GetQueuedSends() error = %v", err)
	}
	if len(first) != 2 || first[0].ID != ids[0] || first[1].ID != ids[2] || first[1].ErrorMessage != errCert {
		t.Fatalf("first batch = %+v", first)
	}
	if first[0].ServerMsgID != "msg-"+ids[0] {
		t.Errorf("ServerMsgID = %q", first[0].ServerMsgID)
	}

	rest, _, err := repo.GetQueuedSends(cursor, 2)
	if err != nil {
		t.Fatalf("GetQueuedSends() error = %v", err)
	}
	if len(rest) != 1 || rest[0].ID != ids[4] {
		t.Errorf("second batch = %+v, want only %s", rest, ids[4])
	}
}
