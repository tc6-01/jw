package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"jw/internal/app/localstore"
)

func decodeJSONBody[T any](t *testing.T, body *bytes.Buffer) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(body.Bytes(), &out); err != nil {
		t.Fatalf("decode json failed: %v, body=%s", err, body.String())
	}
	return out
}

func TestHealthHandler(t *testing.T) {
	h := newServerMux()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health status=%d", w.Code)
	}

	type healthResp struct {
		OK bool `json:"ok"`
	}
	resp := decodeJSONBody[healthResp](t, w.Body)
	if !resp.OK {
		t.Fatalf("health ok=false")
	}
}

func TestRecordAndJumpHandlersUseTreeStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	h := newServerMux()

	payload := map[string]string{
		"url":   "https://golang.org/doc",
		"title": "Go Docs",
	}
	b, _ := json.Marshal(payload)

	recordReq := httptest.NewRequest(http.MethodPost, "/record", bytes.NewReader(b))
	recordReq.Header.Set("Content-Type", "application/json")
	recordW := httptest.NewRecorder()
	h.ServeHTTP(recordW, recordReq)

	if recordW.Code != http.StatusOK {
		t.Fatalf("record status=%d body=%s", recordW.Code, recordW.Body.String())
	}
	type recordResp struct {
		OK  bool   `json:"ok"`
		URL string `json:"url"`
	}
	recordBody := decodeJSONBody[recordResp](t, recordW.Body)
	if !recordBody.OK {
		t.Fatalf("record ok=false")
	}
	if recordBody.URL != "https://golang.org/doc" {
		t.Fatalf("record url=%q", recordBody.URL)
	}

	storePath := filepath.Join(home, ".jw", "store.json")
	db, err := localstore.Load(storePath)
	if err != nil {
		t.Fatalf("load store failed: %v", err)
	}
	if len(db.Targets) != 1 {
		t.Fatalf("targets=%d", len(db.Targets))
	}
	beforeJumpCount := db.Targets[0].Count
	beforeJumpLastSeen := db.Targets[0].LastSeen

	jumpReq := httptest.NewRequest(http.MethodGet, "/jump?q=go", nil)
	jumpW := httptest.NewRecorder()
	h.ServeHTTP(jumpW, jumpReq)

	if jumpW.Code != http.StatusOK {
		t.Fatalf("jump status=%d body=%s", jumpW.Code, jumpW.Body.String())
	}
	type jumpResp struct {
		OK    bool    `json:"ok"`
		URL   string  `json:"url"`
		Score float64 `json:"score"`
	}
	jumpBody := decodeJSONBody[jumpResp](t, jumpW.Body)
	if !jumpBody.OK {
		t.Fatalf("jump ok=false")
	}
	if jumpBody.URL != "https://golang.org/doc" {
		t.Fatalf("jump url=%q", jumpBody.URL)
	}
	if jumpBody.Score <= 0 {
		t.Fatalf("jump score=%f", jumpBody.Score)
	}

	dbAfterJump, err := localstore.Load(storePath)
	if err != nil {
		t.Fatalf("load store after jump failed: %v", err)
	}
	if len(dbAfterJump.Targets) != 1 {
		t.Fatalf("targets after jump=%d", len(dbAfterJump.Targets))
	}
	if dbAfterJump.Targets[0].Count != beforeJumpCount {
		t.Fatalf("count after jump=%d want=%d", dbAfterJump.Targets[0].Count, beforeJumpCount)
	}
	if dbAfterJump.Targets[0].LastSeen != beforeJumpLastSeen {
		t.Fatalf("last_seen after jump=%d want=%d", dbAfterJump.Targets[0].LastSeen, beforeJumpLastSeen)
	}
}

func TestRecordJumpSelectionPersistsLearningForCLIFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, db, err := loadDB()
	if err != nil {
		t.Fatalf("loadDB failed: %v", err)
	}
	oldTime := int64(1_700_000_000)
	if _, err := db.AddAuto("https://github.com/root", "Root", oldTime+60); err != nil {
		t.Fatalf("add root failed: %v", err)
	}
	if _, err := db.AddAuto("https://github.com/docs", "Docs", oldTime); err != nil {
		t.Fatalf("add docs failed: %v", err)
	}
	if err := saveDB(path, db); err != nil {
		t.Fatalf("saveDB failed: %v", err)
	}

	best, err := resolveJumpMatch(db, "github")
	if err != nil {
		t.Fatalf("resolveJumpMatch failed: %v", err)
	}
	if best.Entry.URL != "https://github.com/root" {
		t.Fatalf("unexpected initial url=%q", best.Entry.URL)
	}

	if err := recordJumpSelection(path, db, "https://github.com/docs"); err != nil {
		t.Fatalf("recordJumpSelection failed: %v", err)
	}

	reloaded, err := localstore.Load(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	updatedBest, err := resolveJumpMatch(reloaded, "github")
	if err != nil {
		t.Fatalf("resolveJumpMatch after record failed: %v", err)
	}
	if updatedBest.Entry.URL != "https://github.com/docs" {
		t.Fatalf("updated url=%q want docs", updatedBest.Entry.URL)
	}
}

func TestRecordJumpSelectionIsNoopForUnknownURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	db := &localstore.DB{}

	if err := recordJumpSelection(path, db, "https://example.com/missing"); err != nil {
		t.Fatalf("recordJumpSelection failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no store file, err=%v", err)
	}
}

func TestRecordHandlerValidation(t *testing.T) {
	h := newServerMux()

	methodReq := httptest.NewRequest(http.MethodGet, "/record", nil)
	methodW := httptest.NewRecorder()
	h.ServeHTTP(methodW, methodReq)
	if methodW.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status=%d", methodW.Code)
	}

	invalidJSONReq := httptest.NewRequest(http.MethodPost, "/record", bytes.NewBufferString("{"))
	invalidJSONW := httptest.NewRecorder()
	h.ServeHTTP(invalidJSONW, invalidJSONReq)
	if invalidJSONW.Code != http.StatusBadRequest {
		t.Fatalf("invalid json status=%d", invalidJSONW.Code)
	}

	invalidURLBody, _ := json.Marshal(map[string]string{"url": "javascript:alert(1)"})
	invalidURLReq := httptest.NewRequest(http.MethodPost, "/record", bytes.NewReader(invalidURLBody))
	invalidURLW := httptest.NewRecorder()
	h.ServeHTTP(invalidURLW, invalidURLReq)
	if invalidURLW.Code != http.StatusBadRequest {
		t.Fatalf("invalid url status=%d", invalidURLW.Code)
	}
}

func TestJumpHandlerValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := newServerMux()

	missingReq := httptest.NewRequest(http.MethodGet, "/jump", nil)
	missingW := httptest.NewRecorder()
	h.ServeHTTP(missingW, missingReq)
	if missingW.Code != http.StatusBadRequest {
		t.Fatalf("missing q status=%d", missingW.Code)
	}

	noMatchReq := httptest.NewRequest(http.MethodGet, "/jump?q=none", nil)
	noMatchW := httptest.NewRecorder()
	h.ServeHTTP(noMatchW, noMatchReq)
	if noMatchW.Code != http.StatusNotFound {
		t.Fatalf("no match status=%d", noMatchW.Code)
	}
}

func TestJumpHTTPAndCLIResolverSharePolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, db, err := loadDB()
	if err != nil {
		t.Fatalf("loadDB failed: %v", err)
	}
	db.Rules = []localstore.Rule{
		{Type: localstore.RuleAlias, Pattern: "gh", Value: "github.com"},
		{Type: localstore.RuleDefault, Pattern: "github.com", Value: "github.com/pulls"},
	}
	if _, err := db.AddAuto("https://github.com/docs", "Docs", 1_700_000_000); err != nil {
		t.Fatalf("add docs failed: %v", err)
	}
	if _, err := db.AddAuto("https://github.com/pulls", "Pulls", 1_700_000_100); err != nil {
		t.Fatalf("add pulls failed: %v", err)
	}
	if err := saveDB(path, db); err != nil {
		t.Fatalf("saveDB failed: %v", err)
	}

	h := newServerMux()
	jumpReq := httptest.NewRequest(http.MethodGet, "/jump?q=gh", nil)
	jumpW := httptest.NewRecorder()
	h.ServeHTTP(jumpW, jumpReq)
	if jumpW.Code != http.StatusOK {
		t.Fatalf("jump status=%d body=%s", jumpW.Code, jumpW.Body.String())
	}
	type jumpResp struct {
		URL string `json:"url"`
	}
	jumpBody := decodeJSONBody[jumpResp](t, jumpW.Body)

	_, db2, err := loadDB()
	if err != nil {
		t.Fatalf("reload db failed: %v", err)
	}
	best, err := resolveJumpMatch(db2, "gh")
	if err != nil {
		t.Fatalf("resolveJumpMatch failed: %v", err)
	}

	if jumpBody.URL != best.Entry.URL {
		t.Fatalf("http jump url=%q resolver url=%q", jumpBody.URL, best.Entry.URL)
	}
	if jumpBody.URL != "https://github.com/pulls" {
		t.Fatalf("unexpected jump target=%q", jumpBody.URL)
	}
}
