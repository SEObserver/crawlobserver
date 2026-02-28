package config

import (
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
	want := "clickhouse://user:pass@db.example.com:9000/mydb"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}
