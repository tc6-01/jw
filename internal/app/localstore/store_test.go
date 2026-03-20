package localstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAndSaveRoundTrip(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.json")

	db, err := Load(storePath)
	if err != nil {
		t.Fatalf("load missing store failed: %v", err)
	}
	if len(db.Entries) != 0 {
		t.Fatalf("entries=%d", len(db.Entries))
	}

	entry, err := db.Add("https://example.com/docs", "Docs")
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if entry.Count != 1 {
		t.Fatalf("count=%d", entry.Count)
	}
	if entry.Source != SourceManual {
		t.Fatalf("source=%q want=%q", entry.Source, SourceManual)
	}
	if err := db.Save(storePath); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reloaded, err := Load(storePath)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(reloaded.Entries) != 1 {
		t.Fatalf("entries=%d", len(reloaded.Entries))
	}
	if reloaded.Entries[0].URL != "https://example.com/docs" {
		t.Fatalf("url=%q", reloaded.Entries[0].URL)
	}
	if reloaded.Entries[0].Title != "Docs" {
		t.Fatalf("title=%q", reloaded.Entries[0].Title)
	}
	if reloaded.Entries[0].Source != SourceManual {
		t.Fatalf("source=%q want=%q", reloaded.Entries[0].Source, SourceManual)
	}
}

func TestLoadLegacyCompatibilityForMissingSource(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.json")
	legacyPayload := map[string]any{
		"entries": []map[string]any{{
			"url":       "https://legacy.example.com/path",
			"title":     "Legacy",
			"count":     2,
			"last_seen": time.Now().Unix(),
		}},
	}
	b, _ := json.Marshal(legacyPayload)
	if err := os.WriteFile(storePath, b, 0o644); err != nil {
		t.Fatalf("write legacy payload failed: %v", err)
	}

	db, err := Load(storePath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(db.Entries) != 1 {
		t.Fatalf("entries=%d", len(db.Entries))
	}
	if db.Entries[0].Source != SourceLegacy {
		t.Fatalf("source=%q want=%q", db.Entries[0].Source, SourceLegacy)
	}
}

func TestAddExistingEntryIncrementsCountAndUpdatesTitle(t *testing.T) {
	db := &DB{}

	first, err := db.Add("https://example.com/path#frag", "Old")
	if err != nil {
		t.Fatalf("first add failed: %v", err)
	}
	if first.Count != 1 {
		t.Fatalf("first count=%d", first.Count)
	}

	second, err := db.Add("https://example.com/path", "  New Title  ")
	if err != nil {
		t.Fatalf("second add failed: %v", err)
	}
	if second.Count != 2 {
		t.Fatalf("second count=%d", second.Count)
	}
	if second.Title != "New Title" {
		t.Fatalf("title=%q", second.Title)
	}
	if len(db.Entries) != 1 {
		t.Fatalf("entries=%d", len(db.Entries))
	}
}

func TestAddAutoDoesNotOverwriteManualSource(t *testing.T) {
	db := &DB{}
	manual, err := db.AddManual("https://example.com/docs/page", "Manual")
	if err != nil {
		t.Fatalf("add manual failed: %v", err)
	}
	if manual.Source != SourceManual {
		t.Fatalf("source=%q", manual.Source)
	}

	updated, err := db.AddAuto("https://example.com/docs/page", "Auto", time.Now().Unix())
	if err != nil {
		t.Fatalf("add auto failed: %v", err)
	}
	if updated.Source != SourceManual {
		t.Fatalf("source after auto=%q want manual", updated.Source)
	}
}

func TestAddRejectsDangerousURL(t *testing.T) {
	db := &DB{}
	_, err := db.Add("javascript:alert(1)", "")
	if err == nil {
		t.Fatalf("expected error for dangerous scheme")
	}
}

func TestDeriveCurationMetadataDeterministic(t *testing.T) {
	meta, err := DeriveCurationMetadata("https://docs.example.com/docs/setup/install?x=1")
	if err != nil {
		t.Fatalf("derive metadata failed: %v", err)
	}
	if meta.HostKey != "docs.example.com" {
		t.Fatalf("host=%q", meta.HostKey)
	}
	if meta.DepthBucket != DepthBucketMedium {
		t.Fatalf("bucket=%q", meta.DepthBucket)
	}
	if meta.TopicKey != "docs" {
		t.Fatalf("topic=%q", meta.TopicKey)
	}
	if meta.GroupKey == "" {
		t.Fatalf("group key should not be empty")
	}
}

func TestAutoRepresentativeUniquenessInGroup(t *testing.T) {
	db := &DB{}
	oldTime := time.Now().Add(-2 * time.Hour).Unix()
	newTime := time.Now().Unix()

	_, err := db.AddAuto("https://example.com/docs/a", "A", oldTime)
	if err != nil {
		t.Fatalf("add auto A failed: %v", err)
	}
	latest, err := db.AddAuto("https://example.com/docs/b", "B", newTime)
	if err != nil {
		t.Fatalf("add auto B failed: %v", err)
	}

	reps := 0
	for _, e := range db.Entries {
		if e.Source == SourceAuto && e.GroupKey == latest.GroupKey && e.Representative {
			reps++
		}
	}
	if reps != 1 {
		t.Fatalf("representatives=%d want=1", reps)
	}
}

func TestDifferentDepthBucketsRemainIndependent(t *testing.T) {
	db := &DB{}
	_, err := db.AddAuto("https://example.com/docs", "Docs", time.Now().Add(-time.Minute).Unix())
	if err != nil {
		t.Fatalf("add shallow failed: %v", err)
	}
	_, err = db.AddAuto("https://example.com/docs/a/b", "Docs Deep", time.Now().Unix())
	if err != nil {
		t.Fatalf("add deep failed: %v", err)
	}

	repByBucket := map[string]int{}
	for _, e := range db.Entries {
		if e.Source == SourceAuto && e.Representative {
			repByBucket[e.DepthBucket]++
		}
	}
	if repByBucket[DepthBucketShallow] != 1 {
		t.Fatalf("shallow reps=%d", repByBucket[DepthBucketShallow])
	}
	if repByBucket[DepthBucketMedium] != 1 {
		t.Fatalf("medium reps=%d", repByBucket[DepthBucketMedium])
	}
}

func TestRemoveByURLAndTitle(t *testing.T) {
	db := &DB{Entries: []Entry{
		{URL: "https://example.com/a", Title: "A", Source: SourceLegacy},
		{URL: "https://example.com/b", Title: "B", Source: SourceLegacy},
	}}

	if !db.Remove("A") {
		t.Fatalf("remove by title failed")
	}
	if len(db.Entries) != 1 {
		t.Fatalf("entries=%d", len(db.Entries))
	}
	if db.Entries[0].URL != "https://example.com/b" {
		t.Fatalf("remaining url=%q", db.Entries[0].URL)
	}

	if !db.Remove("https://example.com/b") {
		t.Fatalf("remove by url failed")
	}
	if len(db.Entries) != 0 {
		t.Fatalf("entries=%d", len(db.Entries))
	}
	if db.Remove("   ") {
		t.Fatalf("blank target should not be removable")
	}
}

func TestQueryPrefersManualOverComparableAuto(t *testing.T) {
	now := time.Now().Unix()
	db := &DB{Entries: []Entry{
		{URL: "https://example.com/docs/manual", Title: "Manual", Count: 5, LastSeen: now, Source: SourceManual},
		{URL: "https://example.com/docs/auto", Title: "Auto", Count: 5, LastSeen: now, Source: SourceAuto, GroupKey: "example.com|d2-3|docs", DepthBucket: DepthBucketMedium, TopicKey: "docs", Representative: true},
	}}

	matches := db.Query("docs", 5)
	if len(matches) != 2 {
		t.Fatalf("matches=%d", len(matches))
	}
	if matches[0].Entry.Source != SourceManual {
		t.Fatalf("top source=%q want manual", matches[0].Entry.Source)
	}
}

func TestQueryAllowsAutoToDominateWhenManualWeak(t *testing.T) {
	now := time.Now().Unix()
	db := &DB{Entries: []Entry{
		{URL: "https://example.com/manual-home", Title: "Home", Count: 5, LastSeen: now, Source: SourceManual},
		{URL: "https://example.com/docs/guide", Title: "Guide", Count: 3, LastSeen: now, Source: SourceAuto, GroupKey: "example.com|d2-3|docs", DepthBucket: DepthBucketMedium, TopicKey: "docs", Representative: true},
	}}

	matches := db.Query("guide", 5)
	if len(matches) == 0 {
		t.Fatalf("expected matches")
	}
	if matches[0].Entry.Source != SourceAuto {
		t.Fatalf("top source=%q want auto", matches[0].Entry.Source)
	}
}

func TestQueryDropsNoisyAutoNonRepresentativeEntries(t *testing.T) {
	now := time.Now().Unix()
	db := &DB{Entries: []Entry{
		{
			URL:            "https://example.com/docs/a",
			Title:          "A",
			Count:          4,
			LastSeen:       now,
			Source:         SourceAuto,
			GroupKey:       "example.com|d2-3|docs",
			DepthBucket:    DepthBucketMedium,
			TopicKey:       "docs",
			Representative: true,
		},
		{
			URL:            "https://example.com/docs/b",
			Title:          "B",
			Count:          4,
			LastSeen:       now,
			Source:         SourceAuto,
			GroupKey:       "example.com|d2-3|docs",
			DepthBucket:    DepthBucketMedium,
			TopicKey:       "docs",
			Representative: false,
		},
	}}

	matches := db.Query("docs", 10)
	if len(matches) != 1 {
		t.Fatalf("matches=%d want=1 representative-only", len(matches))
	}
	if matches[0].Entry.URL != "https://example.com/docs/a" {
		t.Fatalf("top url=%q", matches[0].Entry.URL)
	}
}

func TestBestPrefersRecentDeepPageWithinTopBand(t *testing.T) {
	now := time.Now().Unix()
	db := &DB{Entries: []Entry{
		{URL: "https://example.com", Title: "Root", Count: 10, LastSeen: now, Source: SourceLegacy},
		{URL: "https://example.com/docs/a", Title: "A", Count: 10, LastSeen: now - 30, Source: SourceAuto, GroupKey: "example.com|d2-3|docs", DepthBucket: DepthBucketMedium, TopicKey: "docs", Representative: true},
		{URL: "https://example.com/docs/b", Title: "B", Count: 10, LastSeen: now - 5, Source: SourceAuto, GroupKey: "example.com|d2-3|docs-b", DepthBucket: DepthBucketMedium, TopicKey: "docs", Representative: true},
	}}

	best, err := db.Best("example")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://example.com/docs/b" {
		t.Fatalf("best url=%q want deep recent page", best.Entry.URL)
	}
}

func TestTouchNormalizedIncrementsCount(t *testing.T) {
	db := &DB{Entries: []Entry{{URL: "https://example.com/a", Count: 1, LastSeen: time.Now().Unix(), Source: SourceLegacy}}}
	if !db.TouchNormalized("https://example.com/a") {
		t.Fatalf("touch should succeed")
	}
	if db.Entries[0].Count != 2 {
		t.Fatalf("count=%d", db.Entries[0].Count)
	}
}

func TestQueryAndBestOrderingLegacyFallback(t *testing.T) {
	now := time.Now().Unix()
	db := &DB{Entries: []Entry{
		{URL: "https://github.com", Title: "GitHub", Count: 2, LastSeen: now, Source: SourceLegacy},
		{URL: "https://docs.github.com", Title: "Docs", Count: 5, LastSeen: now - int64((48 * time.Hour).Seconds()), Source: SourceLegacy},
		{URL: "https://go.dev", Title: "Go", Count: 9, LastSeen: now, Source: SourceLegacy},
	}}

	matches := db.Query("github", 5)
	if len(matches) != 2 {
		t.Fatalf("matches=%d", len(matches))
	}
	if matches[0].Entry.URL != "https://docs.github.com" {
		t.Fatalf("top url=%q", matches[0].Entry.URL)
	}
	if matches[0].Score <= matches[1].Score {
		t.Fatalf("scores not sorted: %f <= %f", matches[0].Score, matches[1].Score)
	}

	best, err := db.Best("github")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://docs.github.com" {
		t.Fatalf("best url=%q", best.Entry.URL)
	}

	_, err = db.Best("not-exist")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("expected ErrNoMatch, got %v", err)
	}
}

func TestMixedSourceRoundTripPersistence(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.json")
	db := &DB{Entries: []Entry{
		{URL: "https://example.com/manual", Title: "Manual", Count: 3, LastSeen: time.Now().Unix(), Source: SourceManual},
		{URL: "https://example.com/docs/a", Title: "Auto", Count: 2, LastSeen: time.Now().Unix(), Source: SourceAuto, GroupKey: "example.com|d2-3|docs", DepthBucket: DepthBucketMedium, TopicKey: "docs", Representative: true},
		{URL: "https://example.com/legacy", Title: "Legacy", Count: 1, LastSeen: time.Now().Unix(), Source: SourceLegacy},
	}}
	if err := db.Save(storePath); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load(storePath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded.Entries) != 3 {
		t.Fatalf("entries=%d", len(loaded.Entries))
	}
	byURL := map[string]Entry{}
	for _, e := range loaded.Entries {
		byURL[e.URL] = e
	}
	if byURL["https://example.com/manual"].Source != SourceManual {
		t.Fatalf("manual source lost")
	}
	if byURL["https://example.com/docs/a"].Source != SourceAuto || !byURL["https://example.com/docs/a"].Representative {
		t.Fatalf("auto metadata lost")
	}
	if byURL["https://example.com/legacy"].Source != SourceLegacy {
		t.Fatalf("legacy source lost")
	}
}
