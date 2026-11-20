package motion

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBrowser writes an executable that prints a Chrome-like version line.
func fakeBrowser(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho 'Google Chrome 126.0.6478.55'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindPrefersTheStableNameInPATH(t *testing.T) {
	dir := t.TempDir()
	fakeBrowser(t, dir, "chromium")
	want := fakeBrowser(t, dir, "google-chrome-stable")
	t.Setenv("PATH", dir)
	b, err := Find("")
	if err != nil {
		t.Fatal(err)
	}
	if b.Path != want || b.Version != "126.0.6478.55" {
		t.Fatalf("got %+v, want %s 126.0.6478.55", b, want)
	}
}

func TestFindFallsBackThroughTheNames(t *testing.T) {
	dir := t.TempDir()
	want := fakeBrowser(t, dir, "chromium-browser")
	t.Setenv("PATH", dir)
	b, err := Find("")
	if err != nil || b.Path != want {
		t.Fatalf("got %+v, %v", b, err)
	}
}

func TestConfiguredPathWins(t *testing.T) {
	dir := t.TempDir()
	fakeBrowser(t, dir, "google-chrome-stable")
	mine := fakeBrowser(t, t.TempDir(), "my-chrome")
	t.Setenv("PATH", dir)
	b, err := Find(mine)
	if err != nil || b.Path != mine {
		t.Fatalf("got %+v, %v", b, err)
	}
}

func TestInvalidConfiguredPathIsReported(t *testing.T) {
	dir := t.TempDir()
	fakeBrowser(t, dir, "google-chrome-stable")
	t.Setenv("PATH", dir)
	if _, err := Find(filepath.Join(dir, "missing")); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("an invalid path must be an error naming it, got %v", err)
	}
}

func TestNoBrowser(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Find("")
	if !errors.Is(err, ErrNoBrowser) || !strings.Contains(err.Error(), "Settings") {
		t.Fatalf("got %v", err)
	}
}
