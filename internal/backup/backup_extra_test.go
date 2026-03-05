package backup

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// writeToTar
// ---------------------------------------------------------------------------

func TestWriteToTar(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	data := []byte("hello world")
	if err := writeToTar(tw, "test.txt", data); err != nil {
		t.Fatalf("writeToTar: %v", err)
	}
	tw.Close()

	// Read back
	tr := tar.NewReader(&buf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("reading tar: %v", err)
	}
	if hdr.Name != "test.txt" {
		t.Errorf("Name = %q, want %q", hdr.Name, "test.txt")
	}
	if hdr.Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d", hdr.Size, len(data))
	}
	if hdr.Mode != 0644 {
		t.Errorf("Mode = %o, want %o", hdr.Mode, 0644)
	}

	content := make([]byte, hdr.Size)
	if _, err := tr.Read(content); err != nil && err.Error() != "EOF" {
		t.Fatalf("reading content: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("content = %q, want %q", content, "hello world")
	}
}

func TestWriteToTar_EmptyData(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := writeToTar(tw, "empty.txt", []byte{}); err != nil {
		t.Fatalf("writeToTar empty: %v", err)
	}
	tw.Close()

	tr := tar.NewReader(&buf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("reading tar: %v", err)
	}
	if hdr.Size != 0 {
		t.Errorf("Size = %d, want 0", hdr.Size)
	}
}

// ---------------------------------------------------------------------------
// addFileToTar
// ---------------------------------------------------------------------------

func TestAddFileToTar(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "test.txt")
	os.WriteFile(srcPath, []byte("file content"), 0644)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := addFileToTar(tw, srcPath, "archived.txt"); err != nil {
		t.Fatalf("addFileToTar: %v", err)
	}
	tw.Close()

	tr := tar.NewReader(&buf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("reading tar: %v", err)
	}
	if hdr.Name != "archived.txt" {
		t.Errorf("Name = %q, want %q", hdr.Name, "archived.txt")
	}
	if hdr.Size != 12 {
		t.Errorf("Size = %d, want 12", hdr.Size)
	}
}

func TestAddFileToTar_NonexistentFile(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	err := addFileToTar(tw, "/nonexistent/path/file.txt", "out.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// ---------------------------------------------------------------------------
// addDirToTar
// ---------------------------------------------------------------------------

func TestAddDirToTar(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("bbb"), 0644)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := addDirToTar(tw, dir, "prefix"); err != nil {
		t.Fatalf("addDirToTar: %v", err)
	}
	tw.Close()

	// Count entries
	tr := tar.NewReader(&buf)
	names := make(map[string]int64)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names[hdr.Name] = hdr.Size
	}

	// Should have: prefix/ (dir), prefix/a.txt, prefix/sub/ (dir), prefix/sub/b.txt
	if len(names) != 4 {
		t.Errorf("expected 4 entries, got %d: %v", len(names), names)
	}

	if _, ok := names["prefix/a.txt"]; !ok {
		t.Error("missing prefix/a.txt")
	}
	if _, ok := names["prefix/sub/b.txt"]; !ok {
		t.Error("missing prefix/sub/b.txt")
	}
}

// ---------------------------------------------------------------------------
// ListBackups version extraction
// ---------------------------------------------------------------------------

func TestListBackups_VersionExtraction(t *testing.T) {
	dir := t.TempDir()

	files := []struct {
		name    string
		wantVer string
	}{
		{"backup-v1.0.0-20250101T120000.tar.gz", "v1.0.0"},
		{"backup-v2.3.1-20250202T130000.tar.gz", "v2.3.1"},
		{"backup-dev-20250303T140000.tar.gz", "dev"},
	}

	for _, f := range files {
		os.WriteFile(filepath.Join(dir, f.name), []byte("data"), 0644)
	}

	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range files {
		found := false
		for _, b := range backups {
			if b.Filename == want.name {
				found = true
				if b.Version != want.wantVer {
					t.Errorf("backup %q: version = %q, want %q", want.name, b.Version, want.wantVer)
				}
				break
			}
		}
		if !found {
			t.Errorf("backup %q not found in results", want.name)
		}
	}
}

func TestListBackups_IgnoresNonBackupFiles(t *testing.T) {
	dir := t.TempDir()

	// These should be ignored
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dir, "backup.tar.gz"), []byte("data"), 0644)        // missing "backup-" prefix
	os.WriteFile(filepath.Join(dir, "backup-v1.0.0.zip"), []byte("data"), 0644)    // wrong extension
	os.MkdirAll(filepath.Join(dir, "backup-v1.0.0-dir.tar.gz"), 0755)             // directory, not file

	// This should be found
	os.WriteFile(filepath.Join(dir, "backup-v1.0.0-20250101T120000.tar.gz"), []byte("data"), 0644)

	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
		for _, b := range backups {
			t.Logf("  found: %s", b.Filename)
		}
	}
}

// ---------------------------------------------------------------------------
// extractEntry
// ---------------------------------------------------------------------------

func TestExtractEntry_Directory(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "newdir")

	hdr := &tar.Header{
		Name:     "newdir/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	}

	err := extractEntry(hdr, nil, destPath)
	if err != nil {
		t.Fatalf("extractEntry dir: %v", err)
	}

	fi, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !fi.IsDir() {
		t.Error("expected directory")
	}
}

func TestExtractEntry_UnknownType(t *testing.T) {
	hdr := &tar.Header{
		Name:     "link",
		Typeflag: tar.TypeSymlink,
		Mode:     0644,
	}

	// Should return nil (skip unknown types)
	err := extractEntry(hdr, nil, "/tmp/test-extract-link")
	if err != nil {
		t.Errorf("expected nil error for unknown type, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Restore with empty opts
// ---------------------------------------------------------------------------

func TestRestore_EmptyOpts(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")

	// Create a minimal backup
	opts := BackupOptions{BackupDir: backupDir}
	info, err := Create(opts, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// Restore with empty opts - should skip all entries except metadata
	err = Restore(info.Path, BackupOptions{})
	if err != nil {
		t.Fatalf("Restore with empty opts: %v", err)
	}
}
