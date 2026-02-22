package state

import (
	"path/filepath"
	"testing"
)

func TestLockAndUpsert(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer st.Close()

	if err := st.AcquireLock("acc", "tester"); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if err := st.AcquireLock("acc", "tester"); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
	if err := st.ReleaseLock("acc"); err != nil {
		t.Fatalf("release lock: %v", err)
	}

	row := ResourceRow{AccountName: "acc", Kind: "campaign", LogicalKey: "k1", MetaID: "m1", LastAppliedHash: "h1"}
	if err := st.UpsertResource(row); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := st.GetResource("acc", "campaign", "k1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.LastAppliedHash != "h1" {
		t.Fatalf("unexpected resource: %+v", got)
	}

	if err := st.InsertSnapshot("acc", `{"rows":[]}`); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	if err := st.SetAccountMeta("acc", "import_completed_at", "2026-02-21T00:00:00Z"); err != nil {
		t.Fatalf("set account meta: %v", err)
	}
	meta, err := st.GetAccountMeta("acc", "import_completed_at")
	if err != nil {
		t.Fatalf("get account meta: %v", err)
	}
	if meta != "2026-02-21T00:00:00Z" {
		t.Fatalf("unexpected account meta: %q", meta)
	}

	if err := st.UpsertFeedCache(FeedCacheRow{
		AccountName: "acc",
		FeedURL:     "https://example.com/feed.xml",
		FetchedAt:   "2026-02-21T00:00:10Z",
		PayloadJSON: `{"123":{"id":"123"}}`,
	}); err != nil {
		t.Fatalf("upsert feed cache: %v", err)
	}
	cache, err := st.GetFeedCache("acc", "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("get feed cache: %v", err)
	}
	if cache == nil || cache.PayloadJSON == "" {
		t.Fatalf("unexpected feed cache row: %+v", cache)
	}
}
