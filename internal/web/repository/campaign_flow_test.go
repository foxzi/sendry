package repository

import (
	"errors"
	"testing"

	"github.com/foxzi/sendry/internal/web/models"
)

func TestJobGetStatsBatch(t *testing.T) {
	db := setupTestDB(t)
	id := setupJobWithItems(t, db, [][2]string{{"pending", ""}, {"sent", ""}, {"sent", ""}, {"failed", "x"}})
	mustExec(t, db, `INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status) VALUES ('job-2', 'c1', 'l1', 'pending')`)

	repo := NewJobRepository(db)
	got, err := repo.GetStatsBatch([]string{id, "job-2"})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := repo.GetStats(id)
	if got[id] != want || want.Total != 4 || want.Sent != 2 {
		t.Errorf("job-1 = %+v, GetStats = %+v", got[id], want)
	}
	if s, ok := got["job-2"]; ok || s != (models.JobStats{}) {
		t.Errorf("job without items = %+v, %v", s, ok)
	}
	if m, err := repo.GetStatsBatch(nil); err != nil || len(m) != 0 {
		t.Errorf("empty = %v, %v", m, err)
	}
}

func TestTemplateListSummaries(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO templates (id, name, subject, html, text, folder) VALUES ('t1', 'a', 's1', '<a href="{{ unsubscribe_url }}">u</a>', '', 'news')`)
	mustExec(t, db, `INSERT INTO templates (id, name, subject, html, text) VALUES ('t2', 'b', 's2', '', 'Bye: {{.unsubscribe_url}}')`)
	mustExec(t, db, `INSERT INTO templates (id, name, subject, html, text) VALUES ('t3', 'c', 's3', 'unsubscribe_url and unsubscribeXurl', '')`)

	list, err := NewTemplateRepository(db).ListSummaries()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TemplateSummary{}
	for _, s := range list {
		got[s.ID] = s
	}
	if len(got) != 3 || !got["t1"].HasUnsubscribe || !got["t2"].HasUnsubscribe || got["t3"].HasUnsubscribe {
		t.Errorf("summaries = %+v", list)
	}
	if got["t1"].Folder != "news" || got["t1"].Subject != "s1" || got["t1"].Name != "a" {
		t.Errorf("t1 = %+v", got["t1"])
	}
}

func TestCampaignInTx(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO templates (id, name, subject) VALUES ('t1', 'tpl', 's')`)
	repo := NewCampaignRepository(db)

	// an error rolls back everything written in the transaction
	c := &models.Campaign{Name: "rolled back", FromEmail: "a@example.com"}
	boom := errors.New("boom")
	err := repo.InTx(func(tx *CampaignRepository) error {
		if err := tx.Create(c); err != nil {
			return err
		}
		if err := tx.AddVariant(&models.CampaignVariant{CampaignID: c.ID, Name: "A", TemplateID: "t1", Weight: 100}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := repo.GetByID(c.ID); got != nil {
		t.Error("campaign kept after rollback")
	}

	ok := &models.Campaign{Name: "kept", FromEmail: "a@example.com"}
	if err := repo.InTx(func(tx *CampaignRepository) error { return tx.Create(ok) }); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetByID(ok.ID); got == nil {
		t.Error("committed campaign not found")
	}
}

func TestCampaignListSearchIgnoresCase(t *testing.T) {
	repo := NewCampaignRepository(setupTestDB(t))
	for _, name := range []string{"Демо: летняя распродажа", "Осень", "100% скидка"} {
		if err := repo.Create(&models.Campaign{Name: name, FromEmail: "a@example.com"}); err != nil {
			t.Fatal(err)
		}
	}
	for search, want := range map[string]int{"демо": 1, "ДЕМО": 1, "осень": 1, "%": 1, "_": 0} {
		_, total, err := repo.List(models.CampaignListFilter{Search: search, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if total != want {
			t.Errorf("search %q: total = %d, want %d", search, total, want)
		}
	}
}
