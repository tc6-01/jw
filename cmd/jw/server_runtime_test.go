package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jw/internal/app/localstore"
)

func TestParseOnOff(t *testing.T) {
	cases := []struct {
		in      string
		wantVal bool
		wantOK  bool
	}{
		{in: "on", wantVal: true, wantOK: true},
		{in: "OFF", wantVal: false, wantOK: true},
		{in: "enabled", wantVal: true, wantOK: true},
		{in: "disabled", wantVal: false, wantOK: true},
		{in: "maybe", wantVal: false, wantOK: false},
	}

	for _, tc := range cases {
		gotVal, gotOK := parseOnOff(tc.in)
		if gotVal != tc.wantVal || gotOK != tc.wantOK {
			t.Fatalf("parseOnOff(%q)=(%v,%v), want=(%v,%v)", tc.in, gotVal, gotOK, tc.wantVal, tc.wantOK)
		}
	}
}

func TestLoadAppConfigDefaultWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, path, err := loadAppConfig()
	if err != nil {
		t.Fatalf("loadAppConfig failed: %v", err)
	}
	if cfg.AutoImportHistory {
		t.Fatalf("default auto_import_history should be false")
	}
	wantPath := filepath.Join(home, ".jw", "config.json")
	if path != wantPath {
		t.Fatalf("config path=%q want=%q", path, wantPath)
	}
}

func TestSaveAndLoadAppConfigRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath failed: %v", err)
	}

	in := appConfig{AutoImportHistory: true, LastChromeVisitUS: 12345}
	if err := saveAppConfig(cfgPath, in); err != nil {
		t.Fatalf("saveAppConfig failed: %v", err)
	}

	out, loadedPath, err := loadAppConfig()
	if err != nil {
		t.Fatalf("loadAppConfig failed: %v", err)
	}
	if loadedPath != cfgPath {
		t.Fatalf("loadedPath=%q want=%q", loadedPath, cfgPath)
	}
	if out.AutoImportHistory != in.AutoImportHistory || out.LastChromeVisitUS != in.LastChromeVisitUS {
		t.Fatalf("loaded config=%+v want=%+v", out, in)
	}
}

func TestWriteAndReadServerStateRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	in := serverState{PID: os.Getpid(), Addr: "127.0.0.1:18888", StartedAt: 1700000000}
	if err := writeServerState(in); err != nil {
		t.Fatalf("writeServerState failed: %v", err)
	}

	out, running, err := readServerState()
	if err != nil {
		t.Fatalf("readServerState failed: %v", err)
	}
	if !running {
		t.Fatalf("running=false, want true")
	}
	if out.PID != in.PID || out.Addr != in.Addr || out.StartedAt != in.StartedAt {
		t.Fatalf("state=%+v want=%+v", out, in)
	}
}

func TestChromeVisitTimeToUnix(t *testing.T) {
	// 2020-01-01 00:00:00 UTC in Chrome microseconds since 1601-01-01 UTC.
	const chromeUS = int64(13222310400000000)
	got := chromeVisitTimeToUnix(chromeUS)
	want := int64(1577836800)
	if got != want {
		t.Fatalf("chromeVisitTimeToUnix=%d want=%d", got, want)
	}

	zero := chromeVisitTimeToUnix(0)
	now := time.Now().Unix()
	if zero < now-5 || zero > now+5 {
		t.Fatalf("zero visit fallback out of range: %d now=%d", zero, now)
	}
}

func TestImportChromeHistoryOnceDoesNotAdvanceCheckpointWhenSaveStoreFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath failed: %v", err)
	}
	cfg := appConfig{AutoImportHistory: true, LastChromeVisitUS: 100}
	if err := saveAppConfig(cfgPath, cfg); err != nil {
		t.Fatalf("saveAppConfig failed: %v", err)
	}

	originalRead := readChromeHistoryRowsFunc
	originalSaveStore := saveStoreFunc
	defer func() {
		readChromeHistoryRowsFunc = originalRead
		saveStoreFunc = originalSaveStore
	}()

	readChromeHistoryRowsFunc = func(sinceVisitUS int64, limit int) ([]historyRow, int64, error) {
		if sinceVisitUS != 100 {
			t.Fatalf("sinceVisitUS=%d want 100", sinceVisitUS)
		}
		if limit != autoImportBatch {
			t.Fatalf("limit=%d want %d", limit, autoImportBatch)
		}
		return []historyRow{{URL: "https://example.com/docs", Title: "Docs", LastVisit: 200}}, 200, nil
	}
	wantErr := errors.New("save failed")
	saveStoreFunc = func(path string, store localstore.Store) error {
		return wantErr
	}

	imported, err := importChromeHistoryOnce(cfgPath)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v want=%v", err, wantErr)
	}
	if imported != 0 {
		t.Fatalf("imported=%d want 0", imported)
	}

	out, _, err := loadAppConfig()
	if err != nil {
		t.Fatalf("loadAppConfig failed: %v", err)
	}
	if out.LastChromeVisitUS != 100 {
		t.Fatalf("LastChromeVisitUS=%d want 100", out.LastChromeVisitUS)
	}
}

func TestImportChromeHistoryOnceDoesNotAdvanceCheckpointOnPartialFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath failed: %v", err)
	}
	cfg := appConfig{AutoImportHistory: true, LastChromeVisitUS: 100}
	if err := saveAppConfig(cfgPath, cfg); err != nil {
		t.Fatalf("saveAppConfig failed: %v", err)
	}

	path, db, err := loadDB()
	if err != nil {
		t.Fatalf("loadDB failed: %v", err)
	}
	if _, err := db.AddManual("https://example.com/ok", "Manual"); err != nil {
		t.Fatalf("AddManual failed: %v", err)
	}
	if err := saveDB(path, db); err != nil {
		t.Fatalf("saveDB failed: %v", err)
	}

	originalRead := readChromeHistoryRowsFunc
	originalSaveStore := saveStoreFunc
	defer func() {
		readChromeHistoryRowsFunc = originalRead
		saveStoreFunc = originalSaveStore
	}()

	readChromeHistoryRowsFunc = func(sinceVisitUS int64, limit int) ([]historyRow, int64, error) {
		return []historyRow{
			{URL: "https://example.com/ok", Title: "OK", LastVisit: 200},
			{URL: "javascript:alert(1)", Title: "Bad", LastVisit: 300},
		}, 300, nil
	}

	saveCalled := false
	saveStoreFunc = func(path string, store localstore.Store) error {
		saveCalled = true
		return nil
	}

	imported, err := importChromeHistoryOnce(cfgPath)
	if err != nil {
		t.Fatalf("importChromeHistoryOnce failed: %v", err)
	}
	if imported != 1 {
		t.Fatalf("imported=%d want 1", imported)
	}
	if saveCalled {
		t.Fatalf("saveStore should not be called on partial failure")
	}

	out, _, err := loadAppConfig()
	if err != nil {
		t.Fatalf("loadAppConfig failed: %v", err)
	}
	if out.LastChromeVisitUS != 100 {
		t.Fatalf("LastChromeVisitUS=%d want 100", out.LastChromeVisitUS)
	}
}

func TestImportChromeHistoryOnceAdvancesCheckpointAfterSuccessfulSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath failed: %v", err)
	}
	cfg := appConfig{AutoImportHistory: true, LastChromeVisitUS: 100}
	if err := saveAppConfig(cfgPath, cfg); err != nil {
		t.Fatalf("saveAppConfig failed: %v", err)
	}

	originalRead := readChromeHistoryRowsFunc
	originalSaveStore := saveStoreFunc
	defer func() {
		readChromeHistoryRowsFunc = originalRead
		saveStoreFunc = originalSaveStore
	}()

	readChromeHistoryRowsFunc = func(sinceVisitUS int64, limit int) ([]historyRow, int64, error) {
		return []historyRow{{URL: "https://example.com/docs", Title: "Docs", LastVisit: 250}}, 250, nil
	}

	saveStoreFunc = func(path string, store localstore.Store) error {
		return store.Save(path)
	}

	imported, err := importChromeHistoryOnce(cfgPath)
	if err != nil {
		t.Fatalf("importChromeHistoryOnce failed: %v", err)
	}
	if imported != 1 {
		t.Fatalf("imported=%d want 1", imported)
	}

	out, _, err := loadAppConfig()
	if err != nil {
		t.Fatalf("loadAppConfig failed: %v", err)
	}
	if out.LastChromeVisitUS != 250 {
		t.Fatalf("LastChromeVisitUS=%d want 250", out.LastChromeVisitUS)
	}
}
