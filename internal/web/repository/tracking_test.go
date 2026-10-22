package repository

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/foxzi/sendry/internal/web/models"
)

func jobItemIDs(t *testing.T, repo *JobRepository, jobID string) []string {
	t.Helper()
	items, _, err := repo.ListItems(models.JobItemFilter{JobID: jobID})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}

func jobEvent(typ, itemID string, linkID *int, machine bool) *models.EmailEvent {
	return &models.EmailEvent{
		Type: typ, SourceType: models.SourceJobItem, SourceID: itemID,
		CampaignID: "c1", JobID: "job-1", VariantID: "v1",
		Email: "u@example.org", LinkID: linkID, IsMachine: machine,
	}
}

func intp(i int) *int { return &i }

func TestTracking_RecordEventCounters(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{{"sent", ""}, {"sent", ""}})
	jobs := NewJobRepository(db)
	repo := NewTrackingRepository(db)
	ids := jobItemIDs(t, jobs, "job-1")

	// Machine events are logged but do not touch counters
	if err := repo.RecordEvent(jobEvent(models.EventOpen, ids[0], nil, true)); err != nil {
		t.Fatal(err)
	}
	items, _, _ := jobs.ListItems(models.JobItemFilter{JobID: "job-1"})
	for _, it := range items {
		if it.OpenedAt != nil || it.OpenCount != 0 {
			t.Fatalf("machine event changed counters: %+v", it)
		}
	}

	// Two opens
	for i := 0; i < 2; i++ {
		if err := repo.RecordEvent(jobEvent(models.EventOpen, ids[0], nil, false)); err != nil {
			t.Fatal(err)
		}
	}
	// A click on the second item implies an open without bumping open_count
	if err := repo.RecordEvent(jobEvent(models.EventClick, ids[1], intp(1), false)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordEvent(jobEvent(models.EventUnsubscribe, ids[1], nil, false)); err != nil {
		t.Fatal(err)
	}

	items, _, _ = jobs.ListItems(models.JobItemFilter{JobID: "job-1"})
	byID := map[string]models.SendJobItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	a, b := byID[ids[0]], byID[ids[1]]
	if a.OpenCount != 2 || a.OpenedAt == nil || a.ClickedAt != nil {
		t.Errorf("item a = %+v", a)
	}
	if b.OpenCount != 0 || b.OpenedAt == nil || b.ClickCount != 1 || b.ClickedAt == nil || b.UnsubscribedAt == nil {
		t.Errorf("item b = %+v", b)
	}

	evs, err := repo.SourceEvents(models.SourceJobItem, ids[0], 10)
	if err != nil || len(evs) != 3 {
		t.Fatalf("SourceEvents len=%d err=%v", len(evs), err)
	}
	evs, _ = repo.SourceEvents(models.SourceJobItem, ids[1], 1)
	if len(evs) != 1 || evs[0].Type != models.EventUnsubscribe {
		t.Errorf("limit/newest-first: %+v", evs)
	}
}

func TestTracking_RecordEventSend(t *testing.T) {
	db := setupTestDB(t)
	mustExec(t, db, `INSERT INTO sends (id, from_address, to_addresses, subject, sender_domain, server_name, status)
		VALUES ('s1', 'from@example.com', '["to@example.org","x@example.org"]', 'hi', 'example.com', 'mx1', 'sent')`)
	repo := NewTrackingRepository(db)

	src, err := repo.ResolveSource(models.SourceSend, "s1")
	if err != nil || src == nil || src.Email != "to@example.org" || src.FromEmail != "from@example.com" {
		t.Fatalf("ResolveSource send = %+v, %v", src, err)
	}

	e := &models.EmailEvent{Type: models.EventClick, SourceType: models.SourceSend, SourceID: "s1", Email: "to@example.org", LinkID: intp(2)}
	if err := repo.RecordEvent(e); err != nil {
		t.Fatal(err)
	}
	s, err := NewSendRepository(db).GetByID("s1")
	if err != nil || s == nil {
		t.Fatal(err)
	}
	if s.ClickCount != 1 || s.ClickedAt == nil || s.OpenedAt == nil || s.OpenCount != 0 {
		t.Errorf("send = %+v", s)
	}

	opened, _, err := NewSendRepository(db).List(models.SendFilter{Opened: "yes"})
	if err != nil || len(opened) != 1 {
		t.Errorf("Opened=yes: %d %v", len(opened), err)
	}
	none, _, _ := NewSendRepository(db).List(models.SendFilter{Clicked: "no"})
	if len(none) != 0 {
		t.Errorf("Clicked=no: %d", len(none))
	}
}

func TestTracking_ResolveSourceJobItem(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{{"pending", ""}})
	repo := NewTrackingRepository(db)
	ids := jobItemIDs(t, NewJobRepository(db), "job-1")

	src, err := repo.ResolveSource(models.SourceJobItem, ids[0])
	if err != nil || src == nil {
		t.Fatalf("ResolveSource = %v, %v", src, err)
	}
	if src.Email != "user0@example.org" || src.CampaignID != "c1" ||
		src.JobID != "job-1" || src.VariantID != "v1" || src.ListID != "l1" || src.RecipientID != "r0" || src.FromEmail != "a@example.com" || src.SentAt != nil {
		t.Errorf("src = %+v", src)
	}
	if s, err := repo.ResolveSource(models.SourceJobItem, "nope"); s != nil || err != nil {
		t.Errorf("missing: %v %v", s, err)
	}

	// SentAt is the hand-over to the MTA (queued_at), not the delivery report
	queuedAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	mustExec(t, db, `UPDATE send_job_items SET status = 'sent', queued_at = ?, sent_at = ? WHERE id = ?`,
		queuedAt, queuedAt.Add(time.Hour), ids[0])
	src, err = repo.ResolveSource(models.SourceJobItem, ids[0])
	if err != nil || src.SentAt == nil || !src.SentAt.Equal(queuedAt) {
		t.Errorf("SentAt = %v, %v; want %v", src.SentAt, err, queuedAt)
	}
}

func TestTracking_JobAndCampaignStats(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{{"sent", ""}, {"sent", ""}, {"sent", ""}, {"failed", "boom"}})
	mustExec(t, db, `UPDATE campaigns SET track_opens = 1, track_clicks = 1 WHERE id = 'c1'`)
	jobs := NewJobRepository(db)
	repo := NewTrackingRepository(db)
	ids := jobItemIDs(t, jobs, "job-1")

	rec := func(e *models.EmailEvent) {
		t.Helper()
		if err := repo.RecordEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	rec(jobEvent(models.EventOpen, ids[0], nil, false))
	rec(jobEvent(models.EventOpen, ids[0], nil, false))
	rec(jobEvent(models.EventOpen, ids[1], nil, true)) // machine, ignored
	rec(jobEvent(models.EventClick, ids[0], intp(1), false))
	rec(jobEvent(models.EventClick, ids[0], intp(1), false))
	rec(jobEvent(models.EventClick, ids[1], intp(2), false))
	rec(jobEvent(models.EventUnsubscribe, ids[2], nil, false))
	// skipped items are not messages; unsubscribed ones are counted per recipient
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES ('rs', 'l1', 'skipped@example.org')`)
	mustExec(t, db, `INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, error) VALUES ('sk-1', 'job-1', 'rs', 'v1', 'mx1', 'skipped', ?)`,
		models.SkipReasonUnsubscribed)
	mustExec(t, db, `INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, error) VALUES ('sk-2', 'job-1', 'r3', 'v1', 'mx1', 'skipped', ?)`,
		models.SkipReasonDeleted)

	want := models.TrackingStats{Total: 4, Delivered: 3, UniqueOpens: 2, TotalOpens: 2,
		UniqueClicks: 2, TotalClicks: 3, Unsubscribed: 1, SkippedUnsub: 1, Jobs: 1}
	got, err := repo.JobStats("job-1")
	if err != nil || got != want {
		t.Errorf("JobStats = %+v, %v; want %+v", got, err, want)
	}
	got, err = repo.CampaignStats("c1")
	if err != nil || got != want {
		t.Errorf("CampaignStats = %+v, %v; want %+v", got, err, want)
	}
	if got.OpenRate() < 66 || got.OpenRate() > 67 {
		t.Errorf("OpenRate = %v", got.OpenRate())
	}

	empty, err := repo.JobStats("missing")
	if err != nil || empty != (models.TrackingStats{}) {
		t.Errorf("empty = %+v, %v", empty, err)
	}

	// ListItems engagement filters
	for filter, want := range map[models.JobItemFilter]int{
		{JobID: "job-1", Opened: "yes"}:                2,
		{JobID: "job-1", Opened: "no"}:                 2,
		{JobID: "job-1", Clicked: "yes"}:               2,
		{JobID: "job-1", Clicked: "no"}:                2,
		{JobID: "job-1", Opened: "yes", Clicked: "no"}: 0,
	} {
		_, total, err := jobs.ListItems(filter)
		if err != nil || total != want {
			t.Errorf("ListItems(%+v) total = %d, %v; want %d", filter, total, err, want)
		}
	}
	// Opened items are the only ones carrying counters
	items, _, _ := jobs.ListItems(models.JobItemFilter{JobID: "job-1", Opened: "yes", Limit: 10})
	for _, it := range items {
		if it.OpenedAt == nil {
			t.Errorf("filtered item without opened_at: %+v", it)
		}
	}
}

func TestTracking_JobLinkStats(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{{"sent", ""}, {"sent", ""}})
	mustExec(t, db, `INSERT INTO campaign_variants (id, campaign_id, name, template_id) VALUES ('v2', 'c1', 'B', 't1')`)
	jobs := NewJobRepository(db)
	repo := NewTrackingRepository(db)
	ids := jobItemIDs(t, jobs, "job-1")

	snapA := &models.JobVariantSnapshot{JobID: "job-1", VariantID: "v1", VariantName: "A", HTML: "<a>", Links: []models.LinkInfo{
		{ID: 1, URL: "https://a.example/1", Label: "First", Kind: "text"},
		{ID: 2, URL: "https://a.example/2", Label: "Second", Kind: "button"},
	}}
	snapB := &models.JobVariantSnapshot{JobID: "job-1", VariantID: "v2", VariantName: "B", HTML: "<b>", Links: []models.LinkInfo{
		{ID: 2, URL: "https://b.example/2", Label: "Other", Kind: "text"},
		{ID: 3, URL: "https://b.example/3", Label: "Third", Kind: "image"},
	}}
	for _, s := range []*models.JobVariantSnapshot{snapA, snapB} {
		if err := repo.SaveSnapshot(s); err != nil {
			t.Fatal(err)
		}
	}
	// Upsert replaces
	snapA.HTML = "<a2>"
	if err := repo.SaveSnapshot(snapA); err != nil {
		t.Fatal(err)
	}
	snaps, err := repo.GetSnapshots("job-1")
	if err != nil || len(snaps) != 2 || snaps[0].VariantName != "A" || snaps[0].HTML != "<a2>" || len(snaps[0].Links) != 2 {
		t.Fatalf("snapshots = %+v, %v", snaps, err)
	}

	for _, e := range []*models.EmailEvent{
		jobEvent(models.EventClick, ids[0], intp(1), false),
		jobEvent(models.EventClick, ids[0], intp(1), false),
		jobEvent(models.EventClick, ids[1], intp(1), false),
		jobEvent(models.EventClick, ids[1], intp(1), true), // machine
		jobEvent(models.EventClick, ids[1], intp(9), false),
	} {
		if err := repo.RecordEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	// the unknown link carries an URL
	mustExec(t, db, `UPDATE email_events SET url = 'https://gone.example' WHERE link_id = 9`)

	stats, err := repo.JobLinkStats("job-1", "", nil)
	if err != nil || len(stats) != 4 {
		t.Fatalf("stats = %+v, %v", stats, err)
	}
	if s := stats[0]; s.LinkID != 1 || s.Label != "First" || s.UniqueClicks != 2 || s.TotalClicks != 3 {
		t.Errorf("link 1 = %+v", s)
	}
	if s := stats[1]; s.LinkID != 2 || s.Label != "Second" || s.Kind != "button" || s.UniqueClicks != 0 {
		t.Errorf("link 2 = %+v (first label must win)", s)
	}
	if s := stats[2]; s.LinkID != 3 || s.Label != "Third" || s.TotalClicks != 0 {
		t.Errorf("link 3 = %+v", s)
	}
	if s := stats[3]; s.LinkID != 9 || s.URL != "https://gone.example" || s.UniqueClicks != 1 {
		t.Errorf("link 9 = %+v", s)
	}

	// Variant scope: events carry v1, so v2 sees only its snapshot links with zeros
	stats, err = repo.JobLinkStats("job-1", "v2", nil)
	if err != nil || len(stats) != 2 || stats[0].Label != "Other" || stats[0].TotalClicks != 0 {
		t.Errorf("v2 stats = %+v, %v", stats, err)
	}
}

func TestTracking_EventCreatedAtDefault(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrackingRepository(db)
	e := &models.EmailEvent{Type: models.EventOpen, SourceType: "other", SourceID: "x", Email: "a@b.c"}
	before := time.Now().Add(-time.Second)
	if err := repo.RecordEvent(e); err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 || e.CreatedAt.Before(before) {
		t.Errorf("event = %+v", e)
	}
}

func TestCampaignAndAPIKeyTrackingFlags(t *testing.T) {
	db := setupTestDB(t)
	camps := NewCampaignRepository(db)
	c := &models.Campaign{Name: "n", FromEmail: "a@example.com", TrackOpens: true, AddUTM: true}
	if err := camps.Create(c); err != nil {
		t.Fatal(err)
	}
	got, err := camps.GetByID(c.ID)
	if err != nil || !got.TrackOpens || got.TrackClicks || !got.AddUTM {
		t.Fatalf("GetByID = %+v, %v", got, err)
	}
	got.TrackOpens, got.TrackClicks, got.AddUTM = false, true, false
	if err := camps.Update(got); err != nil {
		t.Fatal(err)
	}
	list, _, err := camps.List(models.CampaignListFilter{})
	if err != nil || len(list) != 1 || list[0].TrackOpens || !list[0].TrackClicks || list[0].AddUTM {
		t.Fatalf("List = %+v, %v", list, err)
	}

	keys := NewAPIKeyRepository(db)
	res, err := keys.Create(APIKeyCreateOptions{Name: "k", TrackOpens: true})
	if err != nil {
		t.Fatal(err)
	}
	byID, _ := keys.GetByID(res.ID)
	byHash, _ := keys.GetByHash(HashKey(res.Key))
	for _, k := range []*models.APIKey{byID, byHash} {
		if k == nil || !k.TrackOpens || k.TrackClicks {
			t.Fatalf("key = %+v", k)
		}
	}
	if err := keys.UpdateTracking(res.ID, false, true); err != nil {
		t.Fatal(err)
	}
	klist, _, err := keys.List(models.APIKeyFilter{})
	if err != nil || len(klist) != 1 || klist[0].TrackOpens || !klist[0].TrackClicks {
		t.Fatalf("keys list = %+v, %v", klist, err)
	}
}

func TestTracking_StatsBase(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{{"queued", ""}, {"sent", ""}, {"failed", "boom"}, {"pending", ""}})
	mustExec(t, db, `UPDATE campaigns SET track_opens = 1, track_clicks = 1 WHERE id = 'c1'`)
	jobs := NewJobRepository(db)
	repo := NewTrackingRepository(db)
	byRecipient := map[string]string{}
	items, _, _ := jobs.ListItems(models.JobItemFilter{JobID: "job-1"})
	for _, it := range items {
		byRecipient[it.RecipientID] = it.ID
	}
	queued, failed, pending := byRecipient["r0"], byRecipient["r2"], byRecipient["r3"]

	// Opens on items the MTA has not accepted are outside the base
	for _, id := range []string{queued, failed, pending} {
		if err := repo.RecordEvent(jobEvent(models.EventOpen, id, nil, false)); err != nil {
			t.Fatal(err)
		}
	}
	st, err := repo.JobStats("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 4 || st.Delivered != 2 || st.UniqueOpens != 1 || st.TotalOpens != 1 || st.OpenRate() > 100 {
		t.Errorf("stats = %+v, open rate %v", st, st.OpenRate())
	}

	// A campaign without the pixel has no opens, even though clicks imply them
	if err := repo.RecordEvent(jobEvent(models.EventClick, queued, intp(1), false)); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `UPDATE campaigns SET track_opens = 0 WHERE id = 'c1'`)
	st, _ = repo.JobStats("job-1")
	if st.UniqueOpens != 0 || st.TotalOpens != 0 || st.UniqueClicks != 1 || st.TotalClicks != 1 {
		t.Errorf("no-pixel stats = %+v", st)
	}
	mustExec(t, db, `UPDATE campaigns SET track_opens = 1 WHERE id = 'c1'`)

	// Dry-run jobs do not count in campaign stats; a recipient skipped as
	// unsubscribed by several jobs counts once
	skip := models.SkipReasonUnsubscribed
	mustExec(t, db, `INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status, dry_run)
		VALUES ('job-dry', 'c1', 'l1', 'completed', 1)`)
	mustExec(t, db, `INSERT INTO send_job_items (id, job_id, recipient_id, status) VALUES ('dry-1', 'job-dry', 'r0', 'sent')`)
	mustExec(t, db, `INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, error) VALUES ('dry-2', 'job-dry', 'r1', 'v1', 'mx1', 'skipped', ?)`, skip)
	mustExec(t, db, `INSERT INTO send_jobs (id, campaign_id, recipient_list_id, status)
		VALUES ('job-2', 'c1', 'l1', 'completed')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES ('r4', 'l1', 'user4@example.org')`)
	mustExec(t, db, `INSERT INTO recipients (id, list_id, email) VALUES ('r5', 'l1', 'user5@example.org')`)
	mustExec(t, db, `INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, error) VALUES ('s1', 'job-1', 'r4', 'v1', 'mx1', 'skipped', ?)`, skip)
	mustExec(t, db, `INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, error) VALUES ('s2', 'job-2', 'r4', 'v1', 'mx1', 'skipped', ?)`, skip)
	mustExec(t, db, `INSERT INTO send_job_items (id, job_id, recipient_id, variant_id, server_name, status, error) VALUES ('s3', 'job-2', 'r5', 'v1', 'mx1', 'skipped', ?)`, skip)
	cs, err := repo.CampaignStats("c1")
	if err != nil || cs.Total != 4 || cs.Delivered != 2 || cs.Jobs != 2 || cs.SkippedUnsub != 2 {
		t.Errorf("campaign stats = %+v, %v", cs, err)
	}
	if ds, _ := repo.JobStats("job-dry"); ds.Total != 1 || ds.Delivered != 1 {
		t.Errorf("dry-run job stats = %+v", ds)
	}
}

func TestTracking_RecordEventDedupAndTruncate(t *testing.T) {
	db := setupTestDB(t)
	setupJobWithItems(t, db, [][2]string{{"sent", ""}})
	jobs := NewJobRepository(db)
	repo := NewTrackingRepository(db)
	id := jobItemIDs(t, jobs, "job-1")[0]

	at := time.Now().UTC()
	ev := func(typ string, link *int, ip string, d time.Duration) *models.EmailEvent {
		e := jobEvent(typ, id, link, false)
		e.IP = ip
		e.CreatedAt = at.Add(d)
		return e
	}
	count := func() int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM email_events`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, e := range []*models.EmailEvent{
		ev(models.EventOpen, nil, "1.1.1.1", 0),
		ev(models.EventOpen, nil, "1.1.1.1", 5*time.Second),                // dup
		ev(models.EventOpen, nil, "2.2.2.2", 6*time.Second),                // other IP
		ev(models.EventOpen, nil, "1.1.1.1", EventDedupWindow+time.Second), // window passed
		ev(models.EventClick, intp(1), "1.1.1.1", 7*time.Second),           // other type
		ev(models.EventClick, intp(1), "1.1.1.1", 8*time.Second),           // dup
		ev(models.EventClick, intp(2), "1.1.1.1", 9*time.Second),           // other link
	} {
		if err := repo.RecordEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(); n != 5 {
		t.Errorf("events = %d, want 5", n)
	}
	items, _, _ := jobs.ListItems(models.JobItemFilter{JobID: "job-1"})
	if it := items[0]; it.OpenCount != 3 || it.ClickCount != 2 {
		t.Errorf("counters must skip duplicates: open=%d click=%d", it.OpenCount, it.ClickCount)
	}

	// Long user agent and URL are cut, without splitting a UTF-8 rune
	e := ev(models.EventClick, intp(3), "3.3.3.3", 0)
	e.UserAgent = strings.Repeat("я", 400) // 800 bytes
	e.URL = "https://x.example/" + strings.Repeat("a", 5000)
	if err := repo.RecordEvent(e); err != nil {
		t.Fatal(err)
	}
	var ua, u string
	if err := db.QueryRow(`SELECT user_agent, url FROM email_events WHERE id = ?`, e.ID).Scan(&ua, &u); err != nil {
		t.Fatal(err)
	}
	if len(ua) != 512 || !utf8.ValidString(ua) || len(u) != 2048 {
		t.Errorf("stored ua=%d bytes (valid %v), url=%d bytes", len(ua), utf8.ValidString(ua), len(u))
	}
}

func TestTracking_DeleteEvents(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrackingRepository(db)
	for _, ids := range [][2]string{{"c1", "j1"}, {"c1", "j2"}, {"c2", "j3"}} {
		e := &models.EmailEvent{Type: models.EventOpen, SourceType: models.SourceJobItem, SourceID: ids[1] + "-i",
			CampaignID: ids[0], JobID: ids[1], Email: "a@b.c"}
		if err := repo.RecordEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	left := func() int {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM email_events`).Scan(&n)
		return n
	}
	if err := repo.DeleteCampaignEvents("c1"); err != nil || left() != 1 {
		t.Fatalf("after campaign delete: %d, %v", left(), err)
	}
}
