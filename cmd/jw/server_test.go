package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestRecordAndJumpHandlers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	h := newServerMux()

	payload := map[string]string{
		"url":   "https://golang.org",
		"title": "Go",
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
	if recordBody.URL != "https://golang.org" {
		t.Fatalf("record url=%q", recordBody.URL)
	}

	storePath := filepath.Join(home, ".jw", "store.json")
	db, err := localstore.Load(storePath)
	if err != nil {
		t.Fatalf("load store failed: %v", err)
	}
	if len(db.Entries) != 1 {
		t.Fatalf("entries=%d", len(db.Entries))
	}
	if db.Entries[0].URL != "https://golang.org" {
		t.Fatalf("stored url=%q", db.Entries[0].URL)
	}
	beforeJumpCount := db.Entries[0].Count
	beforeJumpLastSeen := db.Entries[0].LastSeen

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
	if jumpBody.URL != "https://golang.org" {
		t.Fatalf("jump url=%q", jumpBody.URL)
	}
	if jumpBody.Score <= 0 {
		t.Fatalf("jump score=%f", jumpBody.Score)
	}

	dbAfterJump, err := localstore.Load(storePath)
	if err != nil {
		t.Fatalf("load store after jump failed: %v", err)
	}
	if len(dbAfterJump.Entries) != 1 {
		t.Fatalf("entries after jump=%d", len(dbAfterJump.Entries))
	}
	if dbAfterJump.Entries[0].Count != beforeJumpCount+1 {
		t.Fatalf("count after jump=%d want=%d", dbAfterJump.Entries[0].Count, beforeJumpCount+1)
	}
	if dbAfterJump.Entries[0].LastSeen < beforeJumpLastSeen {
		t.Fatalf("last_seen after jump=%d before=%d", dbAfterJump.Entries[0].LastSeen, beforeJumpLastSeen)
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
