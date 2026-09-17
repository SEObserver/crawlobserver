package config

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadDefaults(t *testing.T) {
	viper.Reset()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Crawler.Workers != 10 {
		t.Errorf("Workers = %d, want 10", cfg.Crawler.Workers)
	}
	if cfg.Crawler.UserAgent != "CrawlObserver/1.0" {
		t.Errorf("UserAgent = %q, want CrawlObserver/1.0", cfg.Crawler.UserAgent)
	}
	if cfg.Crawler.MaxBodySize != 10*1024*1024 {
		t.Errorf("MaxBodySize = %d, want 10MB", cfg.Crawler.MaxBodySize)
	}
	if !cfg.Crawler.RespectRobots {
		t.Error("RespectRobots should default to true")
	}
	if cfg.Crawler.StoreHTML {
		t.Error("StoreHTML should default to false")
	}
	if !cfg.Crawler.StoreLinkPosition {
		t.Error("StoreLinkPosition should default to true")
	}
	if cfg.Crawler.MaxLinkPositionsPerPage != 1000 {
		t.Errorf("MaxLinkPositionsPerPage = %d, want 1000", cfg.Crawler.MaxLinkPositionsPerPage)
	}
	if cfg.ClickHouse.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", cfg.ClickHouse.Host)
	}
	if cfg.ClickHouse.Port != 19000 {
		t.Errorf("Port = %d, want 19000", cfg.ClickHouse.Port)
	}
	if cfg.Storage.BatchSize != 1000 {
		t.Errorf("BatchSize = %d, want 1000", cfg.Storage.BatchSize)
	}
}

func TestValidateRejectsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
	}{
		{"zero workers", func(c *Config) { c.Crawler.Workers = 0 }},
		{"zero link position limit", func(c *Config) { c.Crawler.MaxLinkPositionsPerPage = 0 }},
		{"negative link position limit", func(c *Config) { c.Crawler.MaxLinkPositionsPerPage = -1 }},
		{"negative delay", func(c *Config) { c.Crawler.Delay = -1 }},
		{"zero timeout", func(c *Config) { c.Crawler.Timeout = 0 }},
		{"zero max_body_size", func(c *Config) { c.Crawler.MaxBodySize = 0 }},
		{"empty user_agent", func(c *Config) { c.Crawler.UserAgent = "" }},
		{"empty host", func(c *Config) { c.ClickHouse.Host = "" }},
		{"invalid port", func(c *Config) { c.ClickHouse.Port = 0 }},
		{"zero batch_size", func(c *Config) { c.Storage.BatchSize = 0 }},
		{"zero flush_interval", func(c *Config) { c.Storage.FlushInterval = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			cfg, _ := Load()
			tt.modify(cfg)
			if err := validate(cfg); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestClickHouseDSN(t *testing.T) {
	cfg := ClickHouseConfig{
		Host:     "db.example.com",
		Port:     9000,
		Database: "mydb",
		Username: "user",
		Password: "pass",
	}
	want := "clickhouse://user:***@db.example.com:9000/mydb"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestClickHouseDSN_EmptyPassword(t *testing.T) {
	cfg := ClickHouseConfig{
		Host:     "localhost",
		Port:     9000,
		Database: "testdb",
		Username: "admin",
		Password: "", // empty password → should show empty, not "***"
	}
	want := "clickhouse://admin:@localhost:9000/testdb"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Create a temporary config file
	dir := t.TempDir()
	configPath := dir + "/config.yaml"

	content := `
crawler:
  workers: 42
  user_agent: "CustomBot/2.0"
  timeout: "15s"
  max_body_size: 5242880
clickhouse:
  host: "db.test.com"
  port: 9001
  database: "testcrawl"
  mode: "managed"
storage:
  batch_size: 500
  flush_interval: "3s"
server:
  password: "strong-test-password"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	viper.Reset()
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Crawler.Workers != 42 {
		t.Errorf("Workers = %d, want 42", cfg.Crawler.Workers)
	}
	if cfg.Crawler.UserAgent != "CustomBot/2.0" {
		t.Errorf("UserAgent = %q, want CustomBot/2.0", cfg.Crawler.UserAgent)
	}
	if cfg.ClickHouse.Host != "db.test.com" {
		t.Errorf("Host = %q, want db.test.com", cfg.ClickHouse.Host)
	}
	if cfg.ClickHouse.Port != 9001 {
		t.Errorf("Port = %d, want 9001", cfg.ClickHouse.Port)
	}
	if cfg.Storage.BatchSize != 500 {
		t.Errorf("BatchSize = %d, want 500", cfg.Storage.BatchSize)
	}
}

func TestLoad_EnvVarOverride(t *testing.T) {
	viper.Reset()

	// Viper supports env variable binding; bind the key and set the env var
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set env variable to override crawler workers
	t.Setenv("CRAWLER_WORKERS", "77")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Crawler.Workers != 77 {
		t.Errorf("Workers = %d, want 77 (from env var)", cfg.Crawler.Workers)
	}
}

func TestLoad_GeneratesPasswordWhenEmpty(t *testing.T) {
	viper.Reset()
	// By default, server.password is empty and server.username is "admin"
	// Load() should generate a random password

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Password == "" {
		t.Error("Load() should generate a random password when username is set but password is empty")
	}
	// Generated password should be a 32-char hex string (16 bytes)
	if len(cfg.Server.Password) != 32 {
		t.Errorf("generated password length = %d, want 32", len(cfg.Server.Password))
	}
}

func TestLoad_KeepsExplicitPassword(t *testing.T) {
	viper.Reset()
	viper.Set("server.password", "my-explicit-password")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Password != "my-explicit-password" {
		t.Errorf("Password = %q, want 'my-explicit-password'", cfg.Server.Password)
	}
}

func TestIsWeakPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		// Short passwords → weak
		{"empty string", "", true},
		{"3 chars", "abc", true},
		{"7 chars", "1234567", true},

		// Known weak passwords → weak
		{"password", "password", true},
		{"12345678", "12345678", true},
		{"123456789", "123456789", true},
		{"1234567890", "1234567890", true},
		{"crawlobserver", "crawlobserver", true},
		{"admin123", "admin123", true},
		{"changeme", "changeme", true},
		{"qwerty123", "qwerty123", true},
		{"letmein", "letmein", true},
		{"welcome1", "welcome1", true},

		// Case insensitive
		{"PASSWORD uppercase", "PASSWORD", true},
		{"CrawlObserver mixed", "CrawlObserver", true},

		// Strong passwords → not weak
		{"strong with symbols", "MyStr0ng!Pass", false},
		{"random chars", "xK9#mPq2vR", false},
		{"long random pass", "a-very-long-random-pass", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWeakPassword(tt.password); got != tt.want {
				t.Errorf("isWeakPassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

func TestValidateEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "negative max_concurrent_sessions",
			modify:  func(c *Config) { c.Crawler.MaxConcurrentSessions = -1 },
			wantErr: true,
		},
		{
			name:    "negative max_frontier_size",
			modify:  func(c *Config) { c.Crawler.MaxFrontierSize = -1 },
			wantErr: true,
		},
		{
			name:    "negative max_workers",
			modify:  func(c *Config) { c.Crawler.MaxWorkers = -1 },
			wantErr: true,
		},
		{
			name:    "negative retry max_retries",
			modify:  func(c *Config) { c.Crawler.Retry.MaxRetries = -1 },
			wantErr: true,
		},
		{
			name: "retry enabled with zero base_delay",
			modify: func(c *Config) {
				c.Crawler.Retry.MaxRetries = 3
				c.Crawler.Retry.BaseDelay = 0
			},
			wantErr: true,
		},
		{
			name: "retry enabled with max_delay less than base_delay",
			modify: func(c *Config) {
				c.Crawler.Retry.MaxRetries = 3
				c.Crawler.Retry.BaseDelay = 10_000_000_000 // 10s
				c.Crawler.Retry.MaxDelay = 1_000_000_000   // 1s
			},
			wantErr: true,
		},
		{
			name: "managed mode skips host/port validation",
			modify: func(c *Config) {
				c.ClickHouse.Host = ""
				c.ClickHouse.Port = 0
				c.ClickHouse.Mode = "managed"
			},
			wantErr: false,
		},
		{
			name:    "port above 65535",
			modify:  func(c *Config) { c.ClickHouse.Port = 70000 },
			wantErr: true,
		},
		{
			name:    "valid config passes",
			modify:  func(c *Config) {},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			tt.modify(cfg)
			err = validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMigrateLegacySQLite_ReplacesSchemaOnlyDestination(t *testing.T) {
	viper.Reset()
	root := t.TempDir()
	dest := filepath.Join(root, "data", "crawlobserver.db")
	legacyDir := filepath.Join(root, "legacy")
	src := filepath.Join(legacyDir, "crawlobserver.db")

	createTestSQLite(t, dest, projectTableSQL)
	createTestSQLite(t, src,
		projectTableSQL,
		apiKeyTableSQL,
		`INSERT INTO projects (id, name) VALUES ('project-1', 'Recovered project')`,
		`INSERT INTO api_keys (id, name, key_hash, key_prefix, type, active) VALUES ('key-1', 'Recovered key', 'hash-1', 'co_live_', 'general', 1)`,
	)

	t.Chdir(legacyDir)
	migrateLegacySQLite(dest, "crawlobserver.db")

	if got := countTestRows(t, dest, "projects"); got != 1 {
		t.Fatalf("projects rows after migration = %d, want 1", got)
	}
	if got := countTestRows(t, dest, "api_keys"); got != 1 {
		t.Fatalf("api_keys rows after migration = %d, want 1", got)
	}
}

func TestMigrateLegacySQLite_DoesNotReplaceDestinationWithUserData(t *testing.T) {
	viper.Reset()
	root := t.TempDir()
	dest := filepath.Join(root, "data", "crawlobserver.db")
	legacyDir := filepath.Join(root, "legacy")
	src := filepath.Join(legacyDir, "crawlobserver.db")

	createTestSQLite(t, dest,
		projectTableSQL,
		`INSERT INTO projects (id, name) VALUES ('active-project', 'Active project')`,
	)
	createTestSQLite(t, src,
		projectTableSQL,
		`INSERT INTO projects (id, name) VALUES ('legacy-project', 'Legacy project')`,
	)

	t.Chdir(legacyDir)
	migrateLegacySQLite(dest, "crawlobserver.db")

	if got := firstTestString(t, dest, `SELECT name FROM projects`); got != "Active project" {
		t.Fatalf("destination project name = %q, want active data to be preserved", got)
	}
}

func TestMigrateLegacySQLite_RecoversFromPreUpdateBackup(t *testing.T) {
	viper.Reset()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))

	dataDir, err := DefaultDataDir()
	if err != nil {
		t.Fatalf("DefaultDataDir() error = %v", err)
	}
	dest := filepath.Join(dataDir, "crawlobserver.db")
	source := filepath.Join(root, "backup-source.db")
	backupPath := filepath.Join(dataDir, "backups", "backup-v0.12.5-20260519T120000.tar.gz")

	createTestSQLite(t, dest, projectTableSQL)
	createTestSQLite(t, source,
		projectTableSQL,
		providerConnectionTableSQL,
		`INSERT INTO projects (id, name) VALUES ('project-1', 'Recovered backup project')`,
		`INSERT INTO provider_connections (id, project_id, provider, domain, api_key) VALUES ('provider-1', 'project-1', 'seobserver', 'example.com', 'secret-key')`,
	)
	createTestBackupArchive(t, backupPath, source)

	cwd := filepath.Join(root, "cwd")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatalf("creating cwd: %v", err)
	}
	t.Chdir(cwd)
	migrateLegacySQLite(dest, "crawlobserver.db")

	if got := countTestRows(t, dest, "projects"); got != 1 {
		t.Fatalf("projects rows after backup recovery = %d, want 1", got)
	}
	if got := countTestRows(t, dest, "provider_connections"); got != 1 {
		t.Fatalf("provider_connections rows after backup recovery = %d, want 1", got)
	}
}

const projectTableSQL = `
	CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

const apiKeyTableSQL = `
	CREATE TABLE api_keys (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		key_hash TEXT NOT NULL UNIQUE,
		key_prefix TEXT NOT NULL,
		type TEXT NOT NULL,
		project_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME,
		active INTEGER DEFAULT 1
	)`

const providerConnectionTableSQL = `
	CREATE TABLE provider_connections (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		domain TEXT NOT NULL,
		api_key TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

func createTestSQLite(t *testing.T, path string, statements ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating sqlite dir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening sqlite %s: %v", path, err)
	}
	defer db.Close()

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("executing sqlite statement %q: %v", statement, err)
		}
	}
}

func countTestRows(t *testing.T, path, table string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening sqlite %s: %v", path, err)
	}
	defer db.Close()

	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&rows); err != nil {
		t.Fatalf("counting %s rows: %v", table, err)
	}
	return rows
}

func firstTestString(t *testing.T, path, query string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening sqlite %s: %v", path, err)
	}
	defer db.Close()

	var value string
	if err := db.QueryRow(query).Scan(&value); err != nil {
		t.Fatalf("querying sqlite string: %v", err)
	}
	return value
}

func createTestBackupArchive(t *testing.T, archivePath, sqlitePath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(archivePath), 0755); err != nil {
		t.Fatalf("creating backup dir: %v", err)
	}
	out, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating backup archive: %v", err)
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	info, err := os.Stat(sqlitePath)
	if err != nil {
		t.Fatalf("stating sqlite source: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name: "crawlobserver.db",
		Mode: 0600,
		Size: info.Size(),
	}); err != nil {
		t.Fatalf("writing backup header: %v", err)
	}

	in, err := os.Open(sqlitePath)
	if err != nil {
		t.Fatalf("opening sqlite source: %v", err)
	}
	defer in.Close()
	if _, err := io.Copy(tw, in); err != nil {
		t.Fatalf("writing sqlite to backup: %v", err)
	}
}
