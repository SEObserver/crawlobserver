package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SQLBackupOptions configures a live SQL-based backup (no ClickHouse restart needed).
type SQLBackupOptions struct {
	CHURL      string // ClickHouse HTTP URL, e.g. "http://localhost:18123"
	Database   string // database name
	Username   string
	Password   string
	SQLitePath string // crawlobserver.db
	ConfigPath string // config.yaml
	BackupDir  string // where to write backup archives
}

// sqlBackupMetadata stores backup metadata for SQL-based backups.
type sqlBackupMetadata struct {
	Version   string   `json:"version"`
	Format    string   `json:"format"` // "sql-jsonl"
	Timestamp string   `json:"timestamp"`
	Database  string   `json:"database"`
	Tables    []string `json:"tables"`
}

// validTableName matches safe ClickHouse table identifiers.
var validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// chHTTPClient is a shared HTTP client with a reasonable timeout for backup operations.
var chHTTPClient = &http.Client{Timeout: 30 * time.Minute}

// CreateSQLBackup performs a live backup by querying each table via the ClickHouse HTTP
// interface using FORMAT JSONEachRow. Each table is streamed to a temp file first to
// avoid loading the entire dataset in memory. No ClickHouse restart is required.
func CreateSQLBackup(ctx context.Context, opts SQLBackupOptions, version string) (*BackupInfo, error) {
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

	success := false
	defer func() {
		if !success {
			f.Close()
			os.Remove(archivePath)
		}
	}()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// List tables
	tables, err := listTables(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing tables: %w", err)
	}

	// Export DDL first — restore needs schema before inserting data
	ddl, err := exportDDL(ctx, opts, tables)
	if err == nil && len(ddl) > 0 {
		if err := writeToTar(tw, "schema.sql", ddl); err != nil {
			return nil, fmt.Errorf("writing schema: %w", err)
		}
	}

	// Export each table as JSONL via temp file (streaming, no OOM)
	var exportedTables []string
	for _, table := range tables {
		exported, err := exportTableToTar(ctx, opts, table, tw)
		if err != nil {
			return nil, fmt.Errorf("exporting table %s: %w", table, err)
		}
		if exported {
			exportedTables = append(exportedTables, table)
		}
	}

	// Write metadata last
	meta := sqlBackupMetadata{
		Version:   version,
		Format:    "sql-jsonl",
		Timestamp: ts.Format(time.RFC3339),
		Database:  opts.Database,
		Tables:    exportedTables,
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := writeToTar(tw, "metadata.json", metaBytes); err != nil {
		return nil, fmt.Errorf("writing metadata: %w", err)
	}

	// Backup SQLite
	if opts.SQLitePath != "" {
		if _, statErr := os.Stat(opts.SQLitePath); statErr == nil {
			if err := addFileToTar(tw, opts.SQLitePath, "crawlobserver.db"); err != nil {
				return nil, fmt.Errorf("archiving SQLite: %w", err)
			}
		}
	}

	// Backup config
	if opts.ConfigPath != "" {
		if _, statErr := os.Stat(opts.ConfigPath); statErr == nil {
			if err := addFileToTar(tw, opts.ConfigPath, "config.yaml"); err != nil {
				return nil, fmt.Errorf("archiving config: %w", err)
			}
		}
	}

	// Close writers in order, checking errors
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("closing file: %w", err)
	}

	fi, err := os.Stat(archivePath)
	if err != nil {
		return nil, err
	}

	success = true
	return &BackupInfo{
		Filename:  name,
		Path:      archivePath,
		Version:   version,
		CreatedAt: ts,
		Size:      fi.Size(),
	}, nil
}

// exportTableToTar streams a table export to a temp file, then adds it to the tar.
// Returns true if the table had data, false if empty.
func exportTableToTar(ctx context.Context, opts SQLBackupOptions, table string, tw *tar.Writer) (bool, error) {
	if !validTableName.MatchString(table) {
		return false, fmt.Errorf("invalid table name: %q", table)
	}

	// Stream table data to a temp file
	tmp, err := os.CreateTemp("", "chbackup-*.jsonl")
	if err != nil {
		return false, fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	query := fmt.Sprintf("SELECT * FROM %s.%s FORMAT JSONEachRow", opts.Database, table)
	if err := chQueryToWriter(ctx, opts, query, tmp); err != nil {
		return false, err
	}

	// Check size
	fi, err := tmp.Stat()
	if err != nil {
		return false, err
	}
	if fi.Size() == 0 {
		return false, nil
	}

	// Seek back to start and add to tar
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return false, err
	}

	archName := fmt.Sprintf("tables/%s.jsonl", table)
	hdr := &tar.Header{
		Name:    archName,
		Size:    fi.Size(),
		Mode:    0644,
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return false, err
	}
	if _, err := io.Copy(tw, tmp); err != nil {
		return false, err
	}

	return true, nil
}

// RestoreSQLBackup restores a SQL-based backup by reading JSONL and inserting rows.
// Tables are truncated before insertion to ensure idempotent restores.
func RestoreSQLBackup(ctx context.Context, archivePath string, opts SQLBackupOptions) error {
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

	// Single pass: process entries in order. Schema and metadata are small and
	// appear before table data. Table data is streamed to temp files then inserted.
	var schemaDDL []byte

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}

		switch {
		case cleanName == "metadata.json":
			// Read but don't need to act on it
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return fmt.Errorf("reading metadata: %w", err)
			}

		case cleanName == "schema.sql":
			schemaDDL, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("reading schema: %w", err)
			}
			// Apply schema immediately
			if err := applySchema(ctx, opts, schemaDDL); err != nil {
				return fmt.Errorf("applying schema: %w", err)
			}

		case strings.HasPrefix(cleanName, "tables/") && strings.HasSuffix(cleanName, ".jsonl"):
			tableName := strings.TrimPrefix(cleanName, "tables/")
			tableName = strings.TrimSuffix(tableName, ".jsonl")
			if !validTableName.MatchString(tableName) {
				continue
			}

			// Stream table data to temp file, then insert
			if err := restoreTableFromTar(ctx, opts, tableName, tr); err != nil {
				return fmt.Errorf("restoring table %s: %w", tableName, err)
			}

		case cleanName == "crawlobserver.db":
			if opts.SQLitePath != "" {
				if err := extractEntry(hdr, tr, opts.SQLitePath); err != nil {
					return fmt.Errorf("extracting SQLite: %w", err)
				}
			}

		case cleanName == "config.yaml":
			if opts.ConfigPath != "" {
				if err := extractEntry(hdr, tr, opts.ConfigPath); err != nil {
					return fmt.Errorf("extracting config: %w", err)
				}
			}
		}
	}

	return nil
}

// restoreTableFromTar streams table data from the tar entry to a temp file,
// truncates the table, then inserts the data via ClickHouse HTTP interface.
func restoreTableFromTar(ctx context.Context, opts SQLBackupOptions, table string, r io.Reader) error {
	// Write to temp file to avoid loading everything in memory
	tmp, err := os.CreateTemp("", "chrestore-*.jsonl")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, r); err != nil {
		return fmt.Errorf("copying data: %w", err)
	}

	fi, err := tmp.Stat()
	if err != nil {
		return err
	}
	if fi.Size() == 0 {
		return nil
	}

	// Truncate table for idempotent restore
	truncateQuery := fmt.Sprintf("TRUNCATE TABLE IF EXISTS %s.%s", opts.Database, table)
	if err := chExec(ctx, opts, truncateQuery); err != nil {
		// Non-fatal: table might not exist yet, DDL should have created it
	}

	// Seek back and stream insert
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	query := fmt.Sprintf("INSERT INTO %s.%s FORMAT JSONEachRow", opts.Database, table)
	return chPostStream(ctx, opts, query, tmp, fi.Size())
}

func applySchema(ctx context.Context, opts SQLBackupOptions, ddl []byte) error {
	statements := strings.Split(string(ddl), ";\n")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// Non-fatal: table might already exist
		_ = chExec(ctx, opts, stmt)
	}
	return nil
}

// listTables returns all user tables in the database.
func listTables(ctx context.Context, opts SQLBackupOptions) ([]string, error) {
	if !validTableName.MatchString(opts.Database) {
		return nil, fmt.Errorf("invalid database name: %q", opts.Database)
	}
	query := fmt.Sprintf(
		"SELECT name FROM system.tables WHERE database = '%s' AND name NOT LIKE '.%%' ORDER BY name",
		opts.Database,
	)
	body, err := chQueryBytes(ctx, opts, query)
	if err != nil {
		return nil, err
	}
	var tables []string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && validTableName.MatchString(line) {
			tables = append(tables, line)
		}
	}
	return tables, nil
}

// exportDDL exports CREATE TABLE statements for all tables.
func exportDDL(ctx context.Context, opts SQLBackupOptions, tables []string) ([]byte, error) {
	var buf strings.Builder
	for _, table := range tables {
		if !validTableName.MatchString(table) {
			continue
		}
		query := fmt.Sprintf("SHOW CREATE TABLE %s.%s", opts.Database, table)
		body, err := chQueryBytes(ctx, opts, query)
		if err != nil {
			continue
		}
		stmt := strings.TrimSpace(string(body))
		if stmt != "" {
			buf.WriteString(stmt)
			buf.WriteString(";\n\n")
		}
	}
	return []byte(buf.String()), nil
}

// --- ClickHouse HTTP helpers ---
// Credentials are sent via X-ClickHouse-User / X-ClickHouse-Key headers,
// NOT in the URL query string, to avoid leaking them in logs and error messages.

// chNewRequest creates an HTTP request with proper auth headers.
func chNewRequest(ctx context.Context, method string, opts SQLBackupOptions, query string, body io.Reader) (*http.Request, error) {
	u, err := url.Parse(opts.CHURL)
	if err != nil {
		return nil, fmt.Errorf("parsing CH URL: %w", err)
	}
	if query != "" {
		q := u.Query()
		q.Set("query", query)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	if opts.Username != "" {
		req.Header.Set("X-ClickHouse-User", opts.Username)
	}
	if opts.Password != "" {
		req.Header.Set("X-ClickHouse-Key", opts.Password)
	}

	return req, nil
}

// chQueryBytes sends a query and returns the full response body (for small results like DDL, table lists).
func chQueryBytes(ctx context.Context, opts SQLBackupOptions, query string) ([]byte, error) {
	req, err := chNewRequest(ctx, http.MethodGet, opts, query, nil)
	if err != nil {
		return nil, err
	}

	resp, err := chHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ClickHouse error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

// chQueryToWriter sends a query and streams the response body to a writer (for large table exports).
func chQueryToWriter(ctx context.Context, opts SQLBackupOptions, query string, w io.Writer) error {
	req, err := chNewRequest(ctx, http.MethodGet, opts, query, nil)
	if err != nil {
		return err
	}

	resp, err := chHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ClickHouse error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("streaming response: %w", err)
	}

	return nil
}

// chPostStream sends a POST with a streaming body to ClickHouse.
func chPostStream(ctx context.Context, opts SQLBackupOptions, query string, body io.Reader, contentLength int64) error {
	req, err := chNewRequest(ctx, http.MethodPost, opts, query, body)
	if err != nil {
		return err
	}
	req.ContentLength = contentLength

	resp, err := chHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ClickHouse error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	return nil
}

// chExec executes a DDL statement via POST.
func chExec(ctx context.Context, opts SQLBackupOptions, stmt string) error {
	req, err := chNewRequest(ctx, http.MethodPost, opts, "", bytes.NewReader([]byte(stmt)))
	if err != nil {
		return err
	}

	resp, err := chHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ClickHouse error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}
