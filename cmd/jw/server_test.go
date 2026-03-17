package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	h := newServerMux()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health status=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"ok\":true") {
		t.Fatalf("unexpected health body: %s", w.Body.String())
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
	if !strings.Contains(recordW.Body.String(), "golang.org") {
		t.Fatalf("unexpected record body: %s", recordW.Body.String())
	}

	jumpReq := httptest.NewRequest(http.MethodGet, "/jump?q=go", nil)
	jumpW := httptest.NewRecorder()
	h.ServeHTTP(jumpW, jumpReq)

	if jumpW.Code != http.StatusOK {
		t.Fatalf("jump status=%d body=%s", jumpW.Code, jumpW.Body.String())
	}
	if !strings.Contains(jumpW.Body.String(), "golang.org") {
		t.Fatalf("unexpected jump body: %s", jumpW.Body.String())
	}

	storePath := filepath.Join(home, ".jw", "store.json")
	if !strings.Contains(recordW.Body.String(), "ok") {
		t.Fatalf("record response not ok")
	}
	if storePath == "" {
		t.Fatalf("store path should not be empty")
	}
}
