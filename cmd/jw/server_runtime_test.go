package main

import (
	"os"
	"path/filepath"
	"testing"
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
