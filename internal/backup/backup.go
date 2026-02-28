package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupOptions configures what to include in a backup.
type BackupOptions struct {
	DataDir    string // ClickHouse data directory
	SQLitePath string // Path to crawlobserver.db
	ConfigPath string // Path to config.yaml
	BackupDir  string // Where to store backups
}

// BackupInfo describes a backup archive.
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
}

type backupMetadata struct {
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	DataDir   string `json:"data_dir"`
}

// Create creates a tar.gz backup archive with ClickHouse data, SQLite, and config.
func Create(opts BackupOptions, version string) (*BackupInfo, error) {
	if err := os.MkdirAll(opts.BackupDir, 0755); err != nil {
		return nil, fmt.Errorf("creating backup dir: %w", err)
	}

	ts := time.Now()
	name := fmt.Sprintf("backup-%s-%s.tar.gz", version, ts.Format("20060102T150405"))
	archivePath := filepath.Join(opts.BackupDir, name)

	f, err := os.Create(archivePath)
	if err != nil {
		return nil, fmt.Errorf("creating archive: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Write metadata.json
	meta := backupMetadata{
		Version:   version,
		Timestamp: ts.Format(time.RFC3339),
		DataDir:   opts.DataDir,
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := writeToTar(tw, "metadata.json", metaBytes); err != nil {
		return nil, fmt.Errorf("writing metadata: %w", err)
	}

	// Backup ClickHouse data directory
	if opts.DataDir != "" {
		if _, err := os.Stat(opts.DataDir); err == nil {
			if err := addDirToTar(tw, opts.DataDir, "clickhouse-data"); err != nil {
				return nil, fmt.Errorf("archiving ClickHouse data: %w", err)
			}
		}
	}

	// Backup SQLite
	if opts.SQLitePath != "" {
		if _, err := os.Stat(opts.SQLitePath); err == nil {
			if err := addFileToTar(tw, opts.SQLitePath, "crawlobserver.db"); err != nil {
				return nil, fmt.Errorf("archiving SQLite: %w", err)
			}
		}
	}

	// Backup config
	if opts.ConfigPath != "" {
		if _, err := os.Stat(opts.ConfigPath); err == nil {
			if err := addFileToTar(tw, opts.ConfigPath, "config.yaml"); err != nil {
				return nil, fmt.Errorf("archiving config: %w", err)
			}
		}
	}

	// Get final size
	tw.Close()
	gw.Close()
	f.Close()
	fi, err := os.Stat(archivePath)
	if err != nil {
		return nil, err
	}

	return &BackupInfo{
		Filename:  name,
		Path:      archivePath,
		Version:   version,
		CreatedAt: ts,
		Size:      fi.Size(),
	}, nil
}

// ListBackups returns backups sorted newest first.
func ListBackups(backupDir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var backups []BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") || !strings.HasPrefix(e.Name(), "backup-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		bi := BackupInfo{
			Filename:  e.Name(),
			Path:      filepath.Join(backupDir, e.Name()),
			CreatedAt: info.ModTime(),
			Size:      info.Size(),
		}
		// Try to extract version from filename: backup-v1.2.0-20260228T143000.tar.gz
		parts := strings.SplitN(strings.TrimPrefix(e.Name(), "backup-"), "-", 2)
		if len(parts) >= 1 {
			bi.Version = parts[0]
		}
		backups = append(backups, bi)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})
	return backups, nil
}

// Restore extracts a backup archive, replacing existing data.
func Restore(archivePath string, opts BackupOptions) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Sanitize path to prevent traversal
		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}

		switch {
		case strings.HasPrefix(cleanName, "clickhouse-data/") || cleanName == "clickhouse-data":
			if opts.DataDir == "" {
				continue
			}
			relPath := strings.TrimPrefix(cleanName, "clickhouse-data")
			relPath = strings.TrimPrefix(relPath, "/")
			destPath := filepath.Join(opts.DataDir, relPath)
			if err := extractEntry(hdr, tr, destPath); err != nil {
				return fmt.Errorf("extracting %s: %w", cleanName, err)
			}

		case cleanName == "crawlobserver.db":
			if opts.SQLitePath == "" {
				continue
			}
			if err := extractEntry(hdr, tr, opts.SQLitePath); err != nil {
				return fmt.Errorf("extracting SQLite: %w", err)
			}

		case cleanName == "config.yaml":
			if opts.ConfigPath == "" {
				continue
			}
			if err := extractEntry(hdr, tr, opts.ConfigPath); err != nil {
				return fmt.Errorf("extracting config: %w", err)
			}

		case cleanName == "metadata.json":
			// Skip metadata during restore
			continue
		}
	}

	return nil
}

// DeleteBackup removes a backup archive.
func DeleteBackup(archivePath string) error {
	return os.Remove(archivePath)
}

func writeToTar(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func addFileToTar(tw *tar.Writer, srcPath, archiveName string) error {
	fi, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:    archiveName,
		Size:    fi.Size(),
		Mode:    int64(fi.Mode()),
		ModTime: fi.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

func addDirToTar(tw *tar.Writer, srcDir, archivePrefix string) error {
	return filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		archivePath := filepath.Join(archivePrefix, rel)

		if fi.IsDir() {
			hdr := &tar.Header{
				Name:     archivePath + "/",
				Mode:     int64(fi.Mode()),
				ModTime:  fi.ModTime(),
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(hdr)
		}

		// Skip sockets, pipes, etc.
		if !fi.Mode().IsRegular() {
			return nil
		}

		hdr := &tar.Header{
			Name:    archivePath,
			Size:    fi.Size(),
			Mode:    int64(fi.Mode()),
			ModTime: fi.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

func extractEntry(hdr *tar.Header, tr *tar.Reader, destPath string) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(destPath, os.FileMode(hdr.Mode))
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, tr)
		return err
	default:
		return nil
	}
}
