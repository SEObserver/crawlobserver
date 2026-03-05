package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Crawler       CrawlerConfig    `mapstructure:"crawler"`
	ClickHouse    ClickHouseConfig `mapstructure:"clickhouse"`
	Storage       StorageConfig    `mapstructure:"storage"`
	Resources     ResourcesConfig  `mapstructure:"resources"`
	Server        ServerConfig     `mapstructure:"server"`
	Theme         ThemeConfig      `mapstructure:"theme"`
	GSC           GSCConfig        `mapstructure:"gsc"`
	Telemetry     TelemetryConfig  `mapstructure:"telemetry"`
	SetupComplete bool             `mapstructure:"setup_complete"`
}

type TelemetryConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	InstanceID string `mapstructure:"instance_id"`
	AskedAt    string `mapstructure:"asked_at"` // ISO timestamp when user was asked about telemetry
}

type CrawlerConfig struct {
	Workers               int            `mapstructure:"workers"`
	Delay                 time.Duration  `mapstructure:"delay"`
	MaxPages              int            `mapstructure:"max_pages"`
	MaxDepth              int            `mapstructure:"max_depth"`
	Timeout               time.Duration  `mapstructure:"timeout"`
	UserAgent             string         `mapstructure:"user_agent"`
	MaxBodySize           int64          `mapstructure:"max_body_size"`
	RespectRobots         bool           `mapstructure:"respect_robots"`
	StoreHTML             bool           `mapstructure:"store_html"`
	CrawlScope            string         `mapstructure:"crawl_scope"`             // "host" (default), "domain" (eTLD+1), or "subdirectory"
	AllowPrivateIPs       bool           `mapstructure:"allow_private_ips"`       // allow crawling private/reserved IPs (default: false)
	TLSProfile            string         `mapstructure:"tls_profile"`             // "", "chrome", "firefox", "edge"
	SourceIP              string         `mapstructure:"source_ip"`               // local IP to bind outgoing connections
	ForceIPv4             bool           `mapstructure:"force_ipv4"`              // force IPv4-only DNS and connections
	MaxConcurrentSessions int            `mapstructure:"max_concurrent_sessions"` // 0 = 20
	MaxFrontierSize       int            `mapstructure:"max_frontier_size"`       // 0 = 5_000_000
	MaxWorkers            int            `mapstructure:"max_workers"`             // 0 = 100
	Retry                 RetryConfig    `mapstructure:"retry"`
	JSRender              JSRenderConfig `mapstructure:"js_render"`
}

type JSRenderConfig struct {
	Mode           string        `mapstructure:"mode"`            // "off" (default), "auto", "always"
	MaxPages       int           `mapstructure:"max_pages"`       // concurrent Chrome pages (default: 4)
	PageTimeout    time.Duration `mapstructure:"page_timeout"`    // per-page timeout (default: 15s)
	BlockResources bool          `mapstructure:"block_resources"` // block images/fonts (default: true)
}

type RetryConfig struct {
	MaxRetries          int           `mapstructure:"max_retries"`
	BaseDelay           time.Duration `mapstructure:"base_delay"`
	MaxDelay            time.Duration `mapstructure:"max_delay"`
	MaxConsecutiveFails int           `mapstructure:"max_consecutive_fails"`
	MaxGlobalErrorRate  float64       `mapstructure:"max_global_error_rate"`
}

type ClickHouseConfig struct {
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	Database   string `mapstructure:"database"`
	Username   string `mapstructure:"username"`
	Password   string `mapstructure:"password"`
	Mode       string `mapstructure:"mode"`        // "managed" | "external" | "" (auto-detect)
	BinaryPath string `mapstructure:"binary_path"` // path to clickhouse binary, "" = auto-detect
	DataDir    string `mapstructure:"data_dir"`    // data directory, "" = platform default
}

// DSN returns a redacted connection string safe for logging.
func (c ClickHouseConfig) DSN() string {
	pw := "***"
	if c.Password == "" {
		pw = ""
	}
	return fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s",
		c.Username, pw, c.Host, c.Port, c.Database)
}

type StorageConfig struct {
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
}

type ResourcesConfig struct {
	MaxMemoryMB int `mapstructure:"max_memory_mb"` // soft limit, 0 = auto (75% of system RAM)
	MaxCPU      int `mapstructure:"max_cpu"`       // GOMAXPROCS, 0 = all available
}

type ServerConfig struct {
	Host       string          `mapstructure:"host"`
	Port       int             `mapstructure:"port"`
	Username   string          `mapstructure:"username"`
	Password   string          `mapstructure:"password"`
	SQLitePath string          `mapstructure:"sqlite_path"`
	RateLimit  RateLimitConfig `mapstructure:"rate_limit"`
}

type RateLimitConfig struct {
	Enabled            bool    `mapstructure:"enabled"`
	RequestsPerSecond  float64 `mapstructure:"requests_per_second"`
	Burst              int     `mapstructure:"burst"`
	AuthRequestsPerMin int     `mapstructure:"auth_requests_per_minute"`
}

type ThemeConfig struct {
	AppName     string `mapstructure:"app_name" json:"app_name"`
	LogoURL     string `mapstructure:"logo_url" json:"logo_url"`
	AccentColor string `mapstructure:"accent_color" json:"accent_color"`
	Mode        string `mapstructure:"mode" json:"mode"` // "light" or "dark"
}

type GSCConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
}

func SetDefaults() {
	viper.SetDefault("crawler.workers", 10)
	viper.SetDefault("crawler.delay", "1s")
	viper.SetDefault("crawler.max_pages", 0)
	viper.SetDefault("crawler.max_depth", 0)
	viper.SetDefault("crawler.timeout", "30s")
	viper.SetDefault("crawler.user_agent", "CrawlObserver/1.0")
	viper.SetDefault("crawler.max_body_size", 10*1024*1024) // 10MB
	viper.SetDefault("crawler.respect_robots", true)
	viper.SetDefault("crawler.store_html", false)
	viper.SetDefault("crawler.crawl_scope", "host")
	viper.SetDefault("crawler.allow_private_ips", false)
	viper.SetDefault("crawler.max_concurrent_sessions", 20)
	viper.SetDefault("crawler.max_frontier_size", 5000000)
	viper.SetDefault("crawler.max_workers", 100)
	viper.SetDefault("crawler.retry.max_retries", 3)
	viper.SetDefault("crawler.retry.base_delay", "2s")
	viper.SetDefault("crawler.retry.max_delay", "60s")
	viper.SetDefault("crawler.retry.max_consecutive_fails", 10)
	viper.SetDefault("crawler.retry.max_global_error_rate", 0.8)
	viper.SetDefault("crawler.js_render.mode", "off")
	viper.SetDefault("crawler.js_render.max_pages", 4)
	viper.SetDefault("crawler.js_render.page_timeout", "15s")
	viper.SetDefault("crawler.js_render.block_resources", true)

	viper.SetDefault("clickhouse.host", "localhost")
	viper.SetDefault("clickhouse.port", 19000)
	viper.SetDefault("clickhouse.database", "crawlobserver")
	viper.SetDefault("clickhouse.username", "default")
	viper.SetDefault("clickhouse.password", "")
	viper.SetDefault("clickhouse.mode", "")
	viper.SetDefault("clickhouse.binary_path", "")
	viper.SetDefault("clickhouse.data_dir", "")

	viper.SetDefault("storage.batch_size", 1000)
	viper.SetDefault("storage.flush_interval", "5s")

	viper.SetDefault("resources.max_memory_mb", 0) // auto
	viper.SetDefault("resources.max_cpu", 0)       // all available

	viper.SetDefault("server.host", "127.0.0.1")
	viper.SetDefault("server.port", 8899)
	viper.SetDefault("server.username", "admin")
	viper.SetDefault("server.password", "")
	viper.SetDefault("server.sqlite_path", "crawlobserver.db")
	viper.SetDefault("server.rate_limit.enabled", false)
	viper.SetDefault("server.rate_limit.requests_per_second", 10)
	viper.SetDefault("server.rate_limit.burst", 20)
	viper.SetDefault("server.rate_limit.auth_requests_per_minute", 20)

	viper.SetDefault("theme.app_name", "CrawlObserver")
	viper.SetDefault("theme.logo_url", "")
	viper.SetDefault("theme.accent_color", "#7c3aed")
	viper.SetDefault("theme.mode", "light")

	viper.SetDefault("gsc.client_id", "")
	viper.SetDefault("gsc.client_secret", "")
	viper.SetDefault("gsc.redirect_uri", "http://127.0.0.1:8899/api/gsc/callback")

	viper.SetDefault("telemetry.enabled", false)
	viper.SetDefault("telemetry.instance_id", "")
	viper.SetDefault("telemetry.asked_at", "")
	viper.SetDefault("setup_complete", false)
}

func Load() (*Config, error) {
	SetDefaults()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Generate random password if username is set but password is empty
	if cfg.Server.Username != "" && cfg.Server.Password == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generating random password: %w", err)
		}
		cfg.Server.Password = hex.EncodeToString(b)
		fmt.Fprintf(os.Stderr, "\n  *** No password configured. Generated random password: %s ***\n  *** Set server.password in config.yaml to silence this message. ***\n\n", cfg.Server.Password)
	}

	// Resolve relative SQLite path to a stable location so that all modes
	// (serve, crawl, gui) use the same database regardless of the working directory.
	if !filepath.IsAbs(cfg.Server.SQLitePath) {
		origName := cfg.Server.SQLitePath
		dataDir, err := DefaultDataDir()
		if err == nil {
			_ = os.MkdirAll(dataDir, 0755)
			cfg.Server.SQLitePath = filepath.Join(dataDir, origName)

			// Migrate legacy SQLite from old locations (pre-v1.1 stored it in cwd or next to config).
			migrateLegacySQLite(cfg.Server.SQLitePath, origName)
		}
	}

	// Warn about weak password when exposed on all interfaces
	if cfg.Server.Host == "0.0.0.0" && isWeakPassword(cfg.Server.Password) {
		fmt.Fprintf(os.Stderr, "\n  *** WARNING: server is listening on 0.0.0.0 with a weak password! ***\n  *** Set a strong password (>= 8 chars) in server.password before exposing to the internet. ***\n\n")
	}

	// Existing user upgrade: if config file existed BEFORE this Load() call
	// with real content but no setup_complete key, auto-set setup_complete to true
	// so they skip the full onboarding (they'll still get the telemetry opt-in).
	// This check must run BEFORE instance_id generation, which creates the file on fresh installs.
	if !cfg.SetupComplete && viper.ConfigFileUsed() != "" {
		if info, err := os.Stat(viper.ConfigFileUsed()); err == nil && info.Size() > 0 {
			if !viper.IsSet("setup_complete") {
				cfg.SetupComplete = true
				viper.Set("setup_complete", true)
				_ = writeConfig()
			}
		}
	}

	// Generate instance_id if not set
	if cfg.Telemetry.InstanceID == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generating instance_id: %w", err)
		}
		// Format as UUID v4
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		cfg.Telemetry.InstanceID = fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
		viper.Set("telemetry.instance_id", cfg.Telemetry.InstanceID)
		_ = writeConfig()
	}

	return &cfg, nil
}

// writeConfig writes the current viper config to disk, creating it if needed.
func writeConfig() error {
	if err := viper.WriteConfig(); err != nil {
		return viper.SafeWriteConfig()
	}
	return nil
}

// isWeakPassword checks if a password is too simple for internet-exposed deployments.
func isWeakPassword(password string) bool {
	if len(password) < 8 {
		return true
	}
	weak := []string{
		"password", "12345678", "123456789", "1234567890",
		"crawlobserver", "admin123", "changeme",
		"qwerty123", "letmein", "welcome1",
	}
	lower := strings.ToLower(password)
	for _, w := range weak {
		if lower == w {
			return true
		}
	}
	return false
}

// DefaultDataDir returns the platform-specific application data directory.
// macOS: ~/Library/Application Support/CrawlObserver
// Linux: ~/.local/share/crawlobserver
// Windows: %APPDATA%/CrawlObserver
func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "CrawlObserver"), nil
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "CrawlObserver"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "CrawlObserver"), nil
	default:
		return filepath.Join(home, ".local", "share", "crawlobserver"), nil
	}
}

func validate(cfg *Config) error {
	if cfg.Crawler.Workers < 1 {
		return fmt.Errorf("crawler.workers must be >= 1")
	}
	if cfg.Crawler.Delay < 0 {
		return fmt.Errorf("crawler.delay must be >= 0")
	}
	if cfg.Crawler.Timeout <= 0 {
		return fmt.Errorf("crawler.timeout must be > 0")
	}
	if cfg.Crawler.MaxBodySize <= 0 {
		return fmt.Errorf("crawler.max_body_size must be > 0")
	}
	if cfg.Crawler.UserAgent == "" {
		return fmt.Errorf("crawler.user_agent must not be empty")
	}
	// Skip host/port validation when managed mode (ports assigned dynamically)
	if cfg.ClickHouse.Mode != "managed" {
		if cfg.ClickHouse.Host == "" {
			return fmt.Errorf("clickhouse.host must not be empty")
		}
		if cfg.ClickHouse.Port <= 0 || cfg.ClickHouse.Port > 65535 {
			return fmt.Errorf("clickhouse.port must be 1-65535")
		}
	}
	if cfg.Crawler.MaxConcurrentSessions < 0 {
		return fmt.Errorf("crawler.max_concurrent_sessions must be >= 0")
	}
	if cfg.Crawler.MaxFrontierSize < 0 {
		return fmt.Errorf("crawler.max_frontier_size must be >= 0")
	}
	if cfg.Crawler.MaxWorkers < 0 {
		return fmt.Errorf("crawler.max_workers must be >= 0")
	}
	if cfg.Crawler.Retry.MaxRetries < 0 {
		return fmt.Errorf("crawler.retry.max_retries must be >= 0")
	}
	if cfg.Crawler.Retry.MaxRetries > 0 {
		if cfg.Crawler.Retry.BaseDelay <= 0 {
			return fmt.Errorf("crawler.retry.base_delay must be > 0 when retries enabled")
		}
		if cfg.Crawler.Retry.MaxDelay < cfg.Crawler.Retry.BaseDelay {
			return fmt.Errorf("crawler.retry.max_delay must be >= base_delay")
		}
	}
	if cfg.Storage.BatchSize < 1 {
		return fmt.Errorf("storage.batch_size must be >= 1")
	}
	if cfg.Storage.FlushInterval <= 0 {
		return fmt.Errorf("storage.flush_interval must be > 0")
	}
	return nil
}

// migrateLegacySQLite copies a legacy SQLite database to the new location if needed.
// It checks the working directory and the directory containing the config file.
// The copy only happens when the destination doesn't exist or is empty (0 bytes).
func migrateLegacySQLite(destPath, baseName string) {
	// Skip if destination already has data
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		return
	}

	// Candidate locations where the old database might live
	var candidates []string

	// 1. Current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, baseName))
	}

	// 2. Next to the config file
	if cfgFile := viper.ConfigFileUsed(); cfgFile != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(cfgFile), baseName))
	}

	for _, src := range candidates {
		// Don't copy onto itself
		if src == destPath {
			continue
		}
		info, err := os.Stat(src)
		if err != nil || info.Size() == 0 {
			continue
		}
		// Found a legacy database — copy it
		if err := copyFile(src, destPath); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to migrate database from %s: %v\n", src, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "  Migrated database from %s to %s\n", src, destPath)
		return
	}
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
