package config

import (
	"os"
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
