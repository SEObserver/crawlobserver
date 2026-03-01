package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreate_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Setup source files
	dataDir := filepath.Join(dir, "chdata")
	os.MkdirAll(dataDir, 0755)
	os.WriteFile(filepath.Join(dataDir, "table.bin"), []byte("clickhouse-data"), 0644)

	sqlitePath := filepath.Join(dir, "crawlobserver.db")
	os.WriteFile(sqlitePath, []byte("sqlite-data"), 0644)

	configPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(configPath, []byte("port: 8080"), 0644)

	backupDir := filepath.Join(dir, "backups")

	opts := BackupOptions{
		DataDir:    dataDir,
		SQLitePath: sqlitePath,
		ConfigPath: configPath,
		BackupDir:  backupDir,
	}

	// Create backup
	info, err := Create(opts, "v1.0.0")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Version != "v1.0.0" {
		t.Errorf("Version = %q, want %q", info.Version, "v1.0.0")
	}
	if info.Size == 0 {
		t.Error("expected non-zero size")
	}
	if !strings.HasPrefix(info.Filename, "backup-v1.0.0-") {
		t.Errorf("unexpected filename: %s", info.Filename)
	}

	// Restore to a different location
	restoreDir := filepath.Join(dir, "restored")
	restoreData := filepath.Join(restoreDir, "chdata")
	restoreSQLite := filepath.Join(restoreDir, "crawlobserver.db")
	restoreConfig := filepath.Join(restoreDir, "config.yaml")

	restoreOpts := BackupOptions{
		DataDir:    restoreData,
		SQLitePath: restoreSQLite,
		ConfigPath: restoreConfig,
	}

	if err := Restore(info.Path, restoreOpts); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify restored files
	got, err := os.ReadFile(filepath.Join(restoreData, "table.bin"))
	if err != nil {
		t.Fatalf("reading restored CH data: %v", err)
	}
	if string(got) != "clickhouse-data" {
		t.Errorf("CH data = %q, want %q", got, "clickhouse-data")
	}

	got, err = os.ReadFile(restoreSQLite)
	if err != nil {
		t.Fatalf("reading restored SQLite: %v", err)
	}
	if string(got) != "sqlite-data" {
		t.Errorf("SQLite data = %q, want %q", got, "sqlite-data")
	}

	got, err = os.ReadFile(restoreConfig)
	if err != nil {
		t.Fatalf("reading restored config: %v", err)
	}
	if string(got) != "port: 8080" {
		t.Errorf("config = %q, want %q", got, "port: 8080")
	}
}

func TestCreate_MissingOptionalFiles(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")

	opts := BackupOptions{
		DataDir:    filepath.Join(dir, "nonexistent-data"),
		SQLitePath: filepath.Join(dir, "nonexistent.db"),
		ConfigPath: filepath.Join(dir, "nonexistent.yaml"),
		BackupDir:  backupDir,
	}

	info, err := Create(opts, "v1.0.0")
	if err != nil {
		t.Fatalf("Create with missing files should still succeed: %v", err)
	}
	if info.Size == 0 {
		t.Error("expected non-zero size (at least metadata)")
	}
}

func TestCreate_Metadata(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")

	opts := BackupOptions{BackupDir: backupDir}
	info, err := Create(opts, "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// Open and verify metadata.json exists
	f, err := os.Open(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	found := false
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "metadata.json" {
			found = true
			var meta backupMetadata
			if err := json.NewDecoder(tr).Decode(&meta); err != nil {
				t.Fatalf("parsing metadata: %v", err)
			}
			if meta.Version != "v2.0.0" {
				t.Errorf("metadata version = %q, want %q", meta.Version, "v2.0.0")
			}
		}
	}
	if !found {
		t.Error("metadata.json not found in archive")
	}
}

func TestListBackups(t *testing.T) {
	dir := t.TempDir()

	// Create fake backup files
	files := []struct {
		name string
		age  time.Duration
	}{
		{"backup-v1.0.0-20250101T120000.tar.gz", 2 * time.Hour},
		{"backup-v1.1.0-20250102T120000.tar.gz", 1 * time.Hour},
		{"not-a-backup.txt", 0},
		{"backup-v1.2.0-20250103T120000.tar.gz", 0},
	}
	for _, f := range files {
		p := filepath.Join(dir, f.name)
		os.WriteFile(p, []byte("data"), 0644)
		if f.age > 0 {
			os.Chtimes(p, time.Now().Add(-f.age), time.Now().Add(-f.age))
		}
	}

	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 3 {
		t.Fatalf("got %d backups, want 3", len(backups))
	}

	// Should be sorted newest first
	if backups[0].Version != "v1.2.0" {
		t.Errorf("first backup version = %q, want %q", backups[0].Version, "v1.2.0")
	}
}

func TestListBackups_NonexistentDir(t *testing.T) {
	backups, err := ListBackups(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if backups != nil {
		t.Errorf("expected nil, got %v", backups)
	}
}

func TestRestore_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	// Create a malicious archive with ../ path
	archivePath := filepath.Join(dir, "evil.tar.gz")
	f, _ := os.Create(archivePath)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	tw.WriteHeader(&tar.Header{
		Name: "../../../etc/evil",
		Size: 4,
		Mode: 0644,
	})
	tw.Write([]byte("evil"))
	tw.Close()
	gw.Close()
	f.Close()

	restoreDir := filepath.Join(dir, "restored")
	opts := BackupOptions{
		DataDir:    filepath.Join(restoreDir, "data"),
		SQLitePath: filepath.Join(restoreDir, "db"),
		ConfigPath: filepath.Join(restoreDir, "cfg"),
	}

	// Should not error, but should skip the traversal entry
	err := Restore(archivePath, opts)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify the evil file was NOT created
	if _, err := os.Stat(filepath.Join(dir, "..", "..", "..", "etc", "evil")); err == nil {
		t.Error("path traversal was not prevented")
	}
}

func TestDeleteBackup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "backup.tar.gz")
	os.WriteFile(p, []byte("data"), 0644)

	if err := DeleteBackup(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}
