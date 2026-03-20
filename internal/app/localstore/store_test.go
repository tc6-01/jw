package localstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func hasNodeKey(db *DB, key string) bool {
	for _, node := range db.Nodes {
		if node.Key == key {
			return true
		}
	}
	return false
}

func TestLoadAndSaveRoundTripNewModel(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.json")

	db, err := Load(storePath)
	if err != nil {
		t.Fatalf("load missing store failed: %v", err)
	}
	if len(db.Targets) != 0 {
		t.Fatalf("targets=%d", len(db.Targets))
	}

	entry, err := db.Add("https://example.com/docs", "Docs")
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if entry.Source != SourceManual {
		t.Fatalf("source=%q want manual", entry.Source)
	}
	if err := db.Save(storePath); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reloaded, err := Load(storePath)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(reloaded.Targets) != 1 {
		t.Fatalf("targets=%d", len(reloaded.Targets))
	}
	if reloaded.Targets[0].URL != "https://example.com/docs" {
		t.Fatalf("url=%q", reloaded.Targets[0].URL)
	}
	if reloaded.Metadata.SchemaVersion != schemaVersion {
		t.Fatalf("schema_version=%d", reloaded.Metadata.SchemaVersion)
	}
}

func TestLoadLegacyEntriesMigratesToTargetsAndNodes(t *testing.T) {
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
	if !db.Metadata.MigratedFromEntries {
		t.Fatalf("expected migration metadata")
	}
	if len(db.Targets) != 1 {
		t.Fatalf("targets=%d", len(db.Targets))
	}
	if db.Targets[0].Source != SourceLegacy {
		t.Fatalf("source=%q want legacy", db.Targets[0].Source)
	}
	if len(db.Nodes) != 2 {
		t.Fatalf("nodes=%d want root+path", len(db.Nodes))
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

func TestCanonicalHostPathTreeNodesFromDeepPath(t *testing.T) {
	db := &DB{}
	if _, err := db.AddManual("https://docs.github.com/actions/workflows", "Actions"); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	expected := []string{
		"docs.github.com",
		"docs.github.com/actions",
		"docs.github.com/actions/workflows",
	}
	for _, key := range expected {
		if !hasNodeKey(db, key) {
			t.Fatalf("missing node %q", key)
		}
	}
}

func TestAddressResolutionPrefersPathNode(t *testing.T) {
	db := &DB{}
	if _, err := db.AddManual("https://github.com/docs", "GitHub Docs"); err != nil {
		t.Fatalf("add github docs failed: %v", err)
	}
	if _, err := db.AddManual("https://example.com/docs", "Example Docs"); err != nil {
		t.Fatalf("add example docs failed: %v", err)
	}

	best, err := db.Best("github/docs")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://github.com/docs" {
		t.Fatalf("best url=%q", best.Entry.URL)
	}
	if best.Reason != "address" {
		t.Fatalf("reason=%q want address", best.Reason)
	}
	if best.NodeKey != "github.com/docs" {
		t.Fatalf("node=%q", best.NodeKey)
	}
}

func TestManualLearningProvidesDefaultLanding(t *testing.T) {
	db := &DB{}
	oldTime := time.Now().Add(-time.Hour).Unix()
	if _, err := db.AddAuto("https://github.com", "Root", oldTime); err != nil {
		t.Fatalf("add root failed: %v", err)
	}
	if _, err := db.AddManual("https://github.com/docs/actions", "Actions"); err != nil {
		t.Fatalf("add docs failed: %v", err)
	}

	best, err := db.Best("github")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://github.com/docs/actions" {
		t.Fatalf("best url=%q want docs actions", best.Entry.URL)
	}
}

func TestDefaultRuleOverridesLearnedLanding(t *testing.T) {
	db := &DB{
		Rules: []Rule{{Type: RuleDefault, Pattern: "github.com", Value: "github.com/pulls"}},
	}
	if _, err := db.AddAuto("https://github.com/docs", "Docs", time.Now().Unix()); err != nil {
		t.Fatalf("add docs failed: %v", err)
	}
	if _, err := db.AddAuto("https://github.com/pulls", "Pulls", time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatalf("add pulls failed: %v", err)
	}

	best, err := db.Best("github")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://github.com/pulls" {
		t.Fatalf("best url=%q want pulls", best.Entry.URL)
	}
}

func TestAliasRuleExpandsBeforeResolution(t *testing.T) {
	db := &DB{
		Rules: []Rule{{Type: RuleAlias, Pattern: "gh", Value: "github.com"}},
	}
	if _, err := db.AddManual("https://github.com/docs", "Docs"); err != nil {
		t.Fatalf("add docs failed: %v", err)
	}

	best, err := db.Best("gh/docs")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://github.com/docs" {
		t.Fatalf("best url=%q", best.Entry.URL)
	}
}

func TestIgnoreRuleSkipsLearning(t *testing.T) {
	db := &DB{
		Rules: []Rule{{Type: RuleIgnore, Host: "accounts.example.com", Pattern: "/oauth"}},
	}

	_, err := db.AddAuto("https://accounts.example.com/oauth/callback?code=abc", "Callback", time.Now().Unix())
	if !errors.Is(err, ErrIgnoredByRule) {
		t.Fatalf("err=%v want ErrIgnoredByRule", err)
	}
	if len(db.Targets) != 0 {
		t.Fatalf("targets=%d", len(db.Targets))
	}
}

func TestCollapseRuleMapsDynamicTargetsToSameNode(t *testing.T) {
	db := &DB{
		Rules: []Rule{{Type: RuleCollapse, Host: "github.com", Pattern: "/pull/*", Value: "/pull/:id"}},
	}
	oldTime := time.Now().Add(-time.Hour).Unix()
	if _, err := db.AddAuto("https://github.com/pull/123", "PR 123", oldTime); err != nil {
		t.Fatalf("add first pr failed: %v", err)
	}
	if _, err := db.AddAuto("https://github.com/pull/456", "PR 456", time.Now().Unix()); err != nil {
		t.Fatalf("add second pr failed: %v", err)
	}

	if len(db.Targets) != 2 {
		t.Fatalf("targets=%d want 2 concrete targets", len(db.Targets))
	}
	for _, target := range db.Targets {
		if target.NodeKey != "github.com/pull/:id" {
			t.Fatalf("node_key=%q want github.com/pull/:id", target.NodeKey)
		}
	}

	best, err := db.Best("github/pull/123")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://github.com/pull/456" {
		t.Fatalf("best url=%q want latest concrete target", best.Entry.URL)
	}
}

func TestPreserveQueryRuleKeepsDistinctTargets(t *testing.T) {
	db := &DB{
		Rules: []Rule{{Type: RulePreserveQuery, Host: "calendar.example.com", Value: "view"}},
	}
	if _, err := db.AddAuto("https://calendar.example.com?view=week", "Week", time.Now().Unix()); err != nil {
		t.Fatalf("add week failed: %v", err)
	}
	if _, err := db.AddAuto("https://calendar.example.com?view=month", "Month", time.Now().Unix()); err != nil {
		t.Fatalf("add month failed: %v", err)
	}

	if len(db.Targets) != 2 {
		t.Fatalf("targets=%d want 2", len(db.Targets))
	}
	urls := map[string]struct{}{}
	for _, target := range db.Targets {
		urls[target.URL] = struct{}{}
	}
	if _, ok := urls["https://calendar.example.com?view=week"]; !ok {
		t.Fatalf("missing week target")
	}
	if _, ok := urls["https://calendar.example.com?view=month"]; !ok {
		t.Fatalf("missing month target")
	}
}

func TestTouchNormalizedPromotesRecentLanding(t *testing.T) {
	db := &DB{}
	oldTime := time.Now().Add(-2 * time.Hour).Unix()
	if _, err := db.AddAuto("https://github.com/root", "Root", oldTime); err != nil {
		t.Fatalf("add root failed: %v", err)
	}
	if _, err := db.AddAuto("https://github.com/docs", "Docs", oldTime); err != nil {
		t.Fatalf("add docs failed: %v", err)
	}

	if !db.TouchNormalized("https://github.com/docs") {
		t.Fatalf("touch should succeed")
	}

	best, err := db.Best("github")
	if err != nil {
		t.Fatalf("best failed: %v", err)
	}
	if best.Entry.URL != "https://github.com/docs" {
		t.Fatalf("best url=%q want docs", best.Entry.URL)
	}
}

func TestQueryReportsAddressReasonAndListNodes(t *testing.T) {
	db := &DB{}
	if _, err := db.AddManual("https://github.com/docs/actions", "Actions"); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	matches := db.Query("github/docs", 5)
	if len(matches) != 1 {
		t.Fatalf("matches=%d want 1", len(matches))
	}
	if matches[0].Reason != "address" {
		t.Fatalf("reason=%q", matches[0].Reason)
	}

	nodes := db.ListNodes()
	if len(nodes) < 2 {
		t.Fatalf("nodes=%d want at least root+path", len(nodes))
	}
	if nodes[0].Key != "github.com" {
		t.Fatalf("first node=%q want github.com", nodes[0].Key)
	}
}
