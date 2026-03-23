package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe create failed: %v", err)
	}

	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout failed: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader failed: %v", err)
	}

	return string(out)
}

func TestHandleAboutOutput(t *testing.T) {
	out := captureStdout(t, handleAbout)

	checks := []string{
		"jw - 终端网页快速跳转工具",
		"zoxide-like for web",
		"jw tutorial",
		"需要完整命令入口请运行: jw help",
		"Repo: https://github.com/tc6-01/jw",
		"License: MIT",
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Fatalf("about output missing %q, got: %s", s, out)
		}
	}
}

func TestPrintHelpIncludesAbout(t *testing.T) {
	out := captureStdout(t, printHelp)
	if !strings.Contains(out, "jw about") {
		t.Fatalf("help output should include jw about, got: %s", out)
	}
}

func TestPrintHelpIncludesServerLifecycleAndConfig(t *testing.T) {
	out := captureStdout(t, printHelp)
	checks := []string{
		"jw server start",
		"jw server stop|status",
		"jw config auto-import-history on|off",
		"jw version",
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Fatalf("help output missing %q, got: %s", s, out)
		}
	}
}

func TestHandleVersionOutput(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "v0.2.0", "abc123", "2026-03-20T10:00:00Z"
	defer func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	}()

	out := captureStdout(t, handleVersion)
	checks := []string{
		"jw v0.2.0",
		"commit: abc123",
		"built: 2026-03-20T10:00:00Z",
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Fatalf("version output missing %q, got: %s", s, out)
		}
	}
}

func TestHandleAboutIncludesVersion(t *testing.T) {
	oldVersion := version
	version = "v0.2.0"
	defer func() { version = oldVersion }()

	out := captureStdout(t, handleAbout)
	if !strings.Contains(out, "Version: v0.2.0") {
		t.Fatalf("about output should include version, got: %s", out)
	}
}
