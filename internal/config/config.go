package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Crawler    CrawlerConfig    `mapstructure:"crawler"`
	ClickHouse ClickHouseConfig `mapstructure:"clickhouse"`
	Storage    StorageConfig    `mapstructure:"storage"`
	Resources  ResourcesConfig  `mapstructure:"resources"`
	Server     ServerConfig     `mapstructure:"server"`
	Theme      ThemeConfig      `mapstructure:"theme"`
}

type CrawlerConfig struct {
	Workers       int           `mapstructure:"workers"`
	Delay         time.Duration `mapstructure:"delay"`
	MaxPages      int           `mapstructure:"max_pages"`
	MaxDepth      int           `mapstructure:"max_depth"`
	Timeout       time.Duration `mapstructure:"timeout"`
	UserAgent     string        `mapstructure:"user_agent"`
	MaxBodySize   int64         `mapstructure:"max_body_size"`
	RespectRobots bool          `mapstructure:"respect_robots"`
	StoreHTML     bool          `mapstructure:"store_html"`
}

type ClickHouseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

func (c ClickHouseConfig) DSN() string {
	return fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s",
		c.Username, c.Password, c.Host, c.Port, c.Database)
}

type StorageConfig struct {
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
}

type ResourcesConfig struct {
	MaxMemoryMB int `mapstructure:"max_memory_mb"` // soft limit, 0 = auto (75% of system RAM)
	MaxCPU      int `mapstructure:"max_cpu"`        // GOMAXPROCS, 0 = all available
}

type ServerConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type ThemeConfig struct {
	AppName     string `mapstructure:"app_name" json:"app_name"`
	LogoURL     string `mapstructure:"logo_url" json:"logo_url"`
	AccentColor string `mapstructure:"accent_color" json:"accent_color"`
	Mode        string `mapstructure:"mode" json:"mode"` // "light" or "dark"
}

func SetDefaults() {
	viper.SetDefault("crawler.workers", 10)
	viper.SetDefault("crawler.delay", "1s")
	viper.SetDefault("crawler.max_pages", 0)
	viper.SetDefault("crawler.max_depth", 0)
	viper.SetDefault("crawler.timeout", "30s")
	viper.SetDefault("crawler.user_agent", "SEOCrawler/1.0")
	viper.SetDefault("crawler.max_body_size", 10*1024*1024) // 10MB
	viper.SetDefault("crawler.respect_robots", true)
	viper.SetDefault("crawler.store_html", false)

	viper.SetDefault("clickhouse.host", "localhost")
	viper.SetDefault("clickhouse.port", 19000)
	viper.SetDefault("clickhouse.database", "seocrawler")
	viper.SetDefault("clickhouse.username", "default")
	viper.SetDefault("clickhouse.password", "")

	viper.SetDefault("storage.batch_size", 1000)
	viper.SetDefault("storage.flush_interval", "5s")

	viper.SetDefault("resources.max_memory_mb", 0) // auto
	viper.SetDefault("resources.max_cpu", 0)        // all available

	viper.SetDefault("server.host", "127.0.0.1")
	viper.SetDefault("server.port", 8899)
	viper.SetDefault("server.username", "admin")
	viper.SetDefault("server.password", "seocrawler")

	viper.SetDefault("theme.app_name", "SEOCrawler")
	viper.SetDefault("theme.logo_url", "")
	viper.SetDefault("theme.accent_color", "#7c3aed")
	viper.SetDefault("theme.mode", "light")
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

	return &cfg, nil
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
	if cfg.ClickHouse.Host == "" {
		return fmt.Errorf("clickhouse.host must not be empty")
	}
	if cfg.ClickHouse.Port <= 0 || cfg.ClickHouse.Port > 65535 {
		return fmt.Errorf("clickhouse.port must be 1-65535")
	}
	if cfg.Storage.BatchSize < 1 {
		return fmt.Errorf("storage.batch_size must be >= 1")
	}
	if cfg.Storage.FlushInterval <= 0 {
		return fmt.Errorf("storage.flush_interval must be > 0")
	}
	return nil
}
