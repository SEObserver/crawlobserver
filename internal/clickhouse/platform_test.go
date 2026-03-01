package clickhouse

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultDataDir(t *testing.T) {
	dir := DefaultDataDir()
	if dir == "" {
		t.Fatal("DefaultDataDir() returned empty string")
	}

	switch runtime.GOOS {
	case "darwin":
		if filepath.Base(dir) != "CrawlObserver" {
			t.Errorf("darwin: expected CrawlObserver dir, got %q", dir)
		}
	case "linux":
		if filepath.Base(dir) != "crawlobserver" {
			t.Errorf("linux: expected crawlobserver dir, got %q", dir)
		}
	}
}

func TestFindFreePort(t *testing.T) {
	port, err := findFreePort()
	if err != nil {
		t.Fatal(err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("port %d out of range", port)
	}
}

func TestFindAvailablePorts(t *testing.T) {
	tcp, http, err := findAvailablePorts()
	if err != nil {
		t.Fatal(err)
	}
	if tcp <= 0 || http <= 0 {
		t.Errorf("invalid ports: tcp=%d http=%d", tcp, http)
	}
	if tcp == http {
		t.Errorf("tcp and http ports should differ: both %d", tcp)
	}
}

func TestFindBinary_ConfiguredPath(t *testing.T) {
	// Create a fake binary
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "clickhouse")
	os.WriteFile(fakeBin, []byte("#!/bin/sh"), 0755)

	got := FindBinary(fakeBin, dir)
	if got != fakeBin {
		t.Errorf("got %q, want %q", got, fakeBin)
	}
}

func TestFindBinary_LocalDataDir(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0755)
	localBin := filepath.Join(binDir, "clickhouse")
	os.WriteFile(localBin, []byte("#!/bin/sh"), 0755)

	got := FindBinary("", dir)
	if got != localBin {
		t.Errorf("got %q, want %q", got, localBin)
	}
}

func TestFindBinary_NotFound(t *testing.T) {
	dir := t.TempDir()
	got := FindBinary("/nonexistent/path", dir)
	// May find system clickhouse or return empty
	// Just verify no panic
	_ = got
}

func TestDownloadURL(t *testing.T) {
	url, err := downloadURL()
	switch runtime.GOOS {
	case "darwin", "linux":
		if err != nil {
			t.Fatalf("unexpected error on %s: %v", runtime.GOOS, err)
		}
		if url == "" {
			t.Error("expected non-empty URL")
		}
	case "windows":
		if err == nil {
			t.Error("expected error on windows")
		}
	}
}
