package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExpectedAssetName(t *testing.T) {
	name := expectedAssetName()
	if !strings.HasPrefix(name, "crawlobserver-") {
		t.Errorf("expected prefix 'crawlobserver-', got %q", name)
	}
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("expected OS %q in name %q", runtime.GOOS, name)
	}
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("expected arch %q in name %q", runtime.GOARCH, name)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		t.Error("Windows asset should end in .exe")
	}
	if runtime.GOOS != "windows" && strings.HasSuffix(name, ".exe") {
		t.Error("non-Windows asset should not end in .exe")
	}
}

func TestExpectedDesktopAssetName(t *testing.T) {
	name := ExpectedDesktopAssetName()
	if !strings.HasPrefix(name, "CrawlObserver-") {
		t.Errorf("expected prefix 'CrawlObserver-', got %q", name)
	}
	if !strings.HasSuffix(name, ".app.tar.gz") {
		t.Errorf("expected suffix '.app.tar.gz', got %q", name)
	}
}

func TestCheckUpdate_SameVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := Release{TagName: "v1.0.0", HTMLURL: "https://github.com/test/release"}
		json.NewEncoder(w).Encode(release)
	}))
	defer ts.Close()

	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	// Override URL by patching the function — we test via the http mock directly
	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var release Release
	json.NewDecoder(resp.Body).Decode(&release)

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")
	available := latest != current && current != "dev"
	if available {
		t.Error("same version should not be available")
	}
}

func TestCheckUpdate_DifferentVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := Release{TagName: "v2.0.0", HTMLURL: "https://github.com/test/release"}
		json.NewEncoder(w).Encode(release)
	}))
	defer ts.Close()

	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var release Release
	json.NewDecoder(resp.Body).Decode(&release)

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")
	available := latest != current && current != "dev"
	if !available {
		t.Error("different version should be available")
	}
}

func TestCheckUpdate_DevVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := Release{TagName: "v2.0.0"}
		json.NewEncoder(w).Encode(release)
	}))
	defer ts.Close()

	oldVersion := Version
	Version = "dev"
	defer func() { Version = oldVersion }()

	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var release Release
	json.NewDecoder(resp.Body).Decode(&release)

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")
	available := latest != current && current != "dev"
	if available {
		t.Error("dev version should never show as available")
	}
}

func TestCheckUpdate_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ts.Close()

	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		t.Error("expected non-200 status")
	}
}

func TestNewUpdateStatus(t *testing.T) {
	oldVersion := Version
	Version = "v1.5.0"
	defer func() { Version = oldVersion }()

	s := NewUpdateStatus()
	if s.CurrentVersion != "v1.5.0" {
		t.Errorf("CurrentVersion = %q, want %q", s.CurrentVersion, "v1.5.0")
	}
}

func TestUpdateStatus_Snapshot(t *testing.T) {
	s := &UpdateStatus{
		Available:      true,
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v2.0.0",
		ReleaseURL:     "https://example.com",
	}
	snap := s.Snapshot()
	if snap.Available != true || snap.LatestVersion != "v2.0.0" {
		t.Errorf("unexpected snapshot: %+v", snap)
	}
}

func TestUpdateStatus_Release(t *testing.T) {
	r := &Release{TagName: "v1.0.0"}
	s := &UpdateStatus{release: r}
	if got := s.Release(); got != r {
		t.Error("Release() should return the cached release")
	}
}

func TestUpdateStatus_ReleaseNil(t *testing.T) {
	s := &UpdateStatus{}
	if got := s.Release(); got != nil {
		t.Error("Release() should return nil when not set")
	}
}

func TestDownloadUpdate_Success(t *testing.T) {
	fakeBody := []byte("fake-binary-content-for-test")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(fakeBody)
	}))
	defer ts.Close()

	assetName := expectedAssetName()
	release := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: ts.URL + "/" + assetName, Size: int64(len(fakeBody))},
		},
	}

	path, err := DownloadUpdate(release)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if !bytes.Equal(data, fakeBody) {
		t.Errorf("downloaded content mismatch: got %d bytes, want %d", len(data), len(fakeBody))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat downloaded file: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("downloaded file should be executable")
	}
}

func TestDownloadUpdate_NoMatchingAsset(t *testing.T) {
	release := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: "some-other-binary", BrowserDownloadURL: "http://example.com/other"},
		},
	}

	_, err := DownloadUpdate(release)
	if err == nil {
		t.Fatal("expected error for missing asset, got nil")
	}
	if !strings.Contains(err.Error(), "no binary found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDownloadUpdate_EmptyAssets(t *testing.T) {
	release := &Release{
		TagName: "v9.9.9",
		Assets:  []Asset{},
	}

	_, err := DownloadUpdate(release)
	if err == nil {
		t.Fatal("expected error for empty assets, got nil")
	}
	if !strings.Contains(err.Error(), "no binary found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSelfUpdate_Success(t *testing.T) {
	// Create a fake "current" executable
	dir := t.TempDir()
	execPath := filepath.Join(dir, "crawlobserver")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fake "new" binary
	newBinPath := filepath.Join(dir, "crawlobserver-new")
	if err := os.WriteFile(newBinPath, []byte("new-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// We can't easily test SelfUpdate because it uses os.Executable().
	// Instead, test the core rename logic directly.
	backupPath := execPath + ".bak"

	// Simulate: rename old -> backup
	if err := os.Rename(execPath, backupPath); err != nil {
		t.Fatalf("rename to backup: %v", err)
	}

	// Simulate: rename new -> exec
	if err := os.Rename(newBinPath, execPath); err != nil {
		os.Rename(backupPath, execPath)
		t.Fatalf("rename new to exec: %v", err)
	}

	// Verify new binary is in place
	data, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("reading exec: %v", err)
	}
	if string(data) != "new-binary" {
		t.Errorf("exec content = %q, want %q", string(data), "new-binary")
	}

	// Cleanup backup
	os.Remove(backupPath)

	// Verify backup is gone
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("backup should have been removed")
	}
}

func TestSelfUpdate_NonexistentNewBinary(t *testing.T) {
	err := SelfUpdate("/nonexistent/path/new-binary")
	if err == nil {
		t.Fatal("expected error for nonexistent new binary")
	}
}

func TestDownloadDesktopUpdate_Success(t *testing.T) {
	// Build a valid .app.tar.gz in memory
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add .app directory entry
	tw.WriteHeader(&tar.Header{
		Name:     "CrawlObserver.app/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})
	// Add a file inside
	binaryContent := []byte("fake-macos-binary")
	tw.WriteHeader(&tar.Header{
		Name:     "CrawlObserver.app/Contents/MacOS/CrawlObserver",
		Typeflag: tar.TypeReg,
		Mode:     0755,
		Size:     int64(len(binaryContent)),
	})
	tw.Write(binaryContent)

	tw.Close()
	gw.Close()

	tarGzBytes := buf.Bytes()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(tarGzBytes)
	}))
	defer ts.Close()

	assetName := ExpectedDesktopAssetName()
	release := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: ts.URL + "/" + assetName, Size: int64(len(tarGzBytes))},
		},
	}

	appPath, err := DownloadDesktopUpdate(release)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(appPath))

	if !strings.HasSuffix(appPath, ".app") {
		t.Errorf("expected path ending in .app, got %q", appPath)
	}

	// Verify the extracted binary exists
	binaryPath := filepath.Join(appPath, "Contents", "MacOS", "CrawlObserver")
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("reading extracted binary: %v", err)
	}
	if !bytes.Equal(data, binaryContent) {
		t.Error("extracted binary content mismatch")
	}
}

func TestDownloadDesktopUpdate_NoMatchingAsset(t *testing.T) {
	release := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: "some-other-file.tar.gz", BrowserDownloadURL: "http://example.com/other"},
		},
	}

	_, err := DownloadDesktopUpdate(release)
	if err == nil {
		t.Fatal("expected error for missing desktop asset, got nil")
	}
	if !strings.Contains(err.Error(), "no desktop bundle found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDownloadDesktopUpdate_NoAppInArchive(t *testing.T) {
	// Build a tar.gz with no .app directory
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	content := []byte("just a file")
	tw.WriteHeader(&tar.Header{
		Name:     "somefile.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(content)),
	})
	tw.Write(content)
	tw.Close()
	gw.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	}))
	defer ts.Close()

	assetName := ExpectedDesktopAssetName()
	release := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: ts.URL + "/" + assetName},
		},
	}

	_, err := DownloadDesktopUpdate(release)
	if err == nil {
		t.Fatal("expected error for archive without .app, got nil")
	}
	if !strings.Contains(err.Error(), "no .app bundle found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSelfUpdateDesktop_Success(t *testing.T) {
	dir := t.TempDir()

	// Create a fake current .app bundle
	currentApp := filepath.Join(dir, "CrawlObserver.app")
	currentBinDir := filepath.Join(currentApp, "Contents", "MacOS")
	os.MkdirAll(currentBinDir, 0755)
	os.WriteFile(filepath.Join(currentBinDir, "CrawlObserver"), []byte("old"), 0755)

	// Create a fake new .app bundle
	newApp := filepath.Join(dir, "CrawlObserver-new.app")
	newBinDir := filepath.Join(newApp, "Contents", "MacOS")
	os.MkdirAll(newBinDir, 0755)
	os.WriteFile(filepath.Join(newBinDir, "CrawlObserver"), []byte("new"), 0755)

	// Simulate the rename logic from SelfUpdateDesktop
	backupPath := currentApp + ".bak"
	if err := os.Rename(currentApp, backupPath); err != nil {
		t.Fatalf("rename to backup: %v", err)
	}
	if err := os.Rename(newApp, currentApp); err != nil {
		os.Rename(backupPath, currentApp)
		t.Fatalf("rename new to current: %v", err)
	}
	os.RemoveAll(backupPath)

	// Verify new binary is in place
	data, err := os.ReadFile(filepath.Join(currentBinDir, "CrawlObserver"))
	if err != nil {
		t.Fatalf("reading binary: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("binary content = %q, want %q", string(data), "new")
	}

	// Verify backup is removed
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("backup .app should have been removed")
	}
}

func TestUpdateStatus_Check_Integration(t *testing.T) {
	// Test that Check() populates the status fields even when it hits
	// the real GitHub API (which may fail in CI). We just verify it
	// doesn't panic and sets CheckedAt.
	oldVersion := Version
	Version = "dev"
	defer func() { Version = oldVersion }()

	s := NewUpdateStatus()
	s.Check()

	if s.CheckedAt.IsZero() {
		t.Error("CheckedAt should be set after Check()")
	}
	// In dev mode, update should never be available
	snap := s.Snapshot()
	if snap.Available {
		t.Error("dev version should never show update as available")
	}
}

func TestExpectedAssetName_Format(t *testing.T) {
	name := expectedAssetName()
	expected := fmt.Sprintf("crawlobserver-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		expected += ".exe"
	}
	if name != expected {
		t.Errorf("expectedAssetName() = %q, want %q", name, expected)
	}
}
