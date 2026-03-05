package config

import (
	"runtime"
	"testing"
	"time"
)

func TestAutoMemoryLimitMB(t *testing.T) {
	limit := autoMemoryLimitMB()
	if limit <= 0 {
		t.Errorf("autoMemoryLimitMB() = %d, want > 0", limit)
	}

	// Should be numCPU * 256 (capped at 4096)
	expected := runtime.NumCPU() * 256
	if expected > 4096 {
		expected = 4096
	}
	if limit != expected {
		t.Errorf("autoMemoryLimitMB() = %d, want %d", limit, expected)
	}
}

func TestAutoMemoryLimitMB_CappedAt4GB(t *testing.T) {
	// The function caps at 4096 MB regardless of CPU count
	limit := autoMemoryLimitMB()
	if limit > 4096 {
		t.Errorf("autoMemoryLimitMB() = %d, should be capped at 4096", limit)
	}
}

func TestApplyResourceLimits_ExplicitMemory(t *testing.T) {
	cfg := &Config{
		Crawler: CrawlerConfig{
			Workers:   10,
			UserAgent: "Test/1.0",
			Timeout:   10 * time.Second,
		},
		Resources: ResourcesConfig{
			MaxMemoryMB: 1024,
			MaxCPU:      0, // don't change GOMAXPROCS
		},
	}

	// Should not panic
	ApplyResourceLimits(cfg)

	// Workers should remain unchanged (1024 MB is not < 512)
	if cfg.Crawler.Workers != 10 {
		t.Errorf("Workers = %d, want 10 (should not be reduced for 1024 MB)", cfg.Crawler.Workers)
	}
}

func TestApplyResourceLimits_LowMemoryReducesWorkers(t *testing.T) {
	cfg := &Config{
		Crawler: CrawlerConfig{
			Workers:   20,
			UserAgent: "Test/1.0",
			Timeout:   10 * time.Second,
		},
		Resources: ResourcesConfig{
			MaxMemoryMB: 200, // < 512 triggers worker reduction
			MaxCPU:      0,
		},
	}

	ApplyResourceLimits(cfg)

	// maxWorkers = max(1, 200/50) = 4
	// 20 > 4, so workers should be reduced to 4
	if cfg.Crawler.Workers != 4 {
		t.Errorf("Workers = %d, want 4 (should be reduced for 200 MB constraint)", cfg.Crawler.Workers)
	}
}

func TestApplyResourceLimits_VeryLowMemory(t *testing.T) {
	cfg := &Config{
		Crawler: CrawlerConfig{
			Workers:   10,
			UserAgent: "Test/1.0",
			Timeout:   10 * time.Second,
		},
		Resources: ResourcesConfig{
			MaxMemoryMB: 30, // max(1, 30/50) = max(1, 0) = 1
			MaxCPU:      0,
		},
	}

	ApplyResourceLimits(cfg)

	if cfg.Crawler.Workers != 1 {
		t.Errorf("Workers = %d, want 1 (minimum for very low memory)", cfg.Crawler.Workers)
	}
}

func TestApplyResourceLimits_AutoMemory(t *testing.T) {
	cfg := &Config{
		Crawler: CrawlerConfig{
			Workers:   5,
			UserAgent: "Test/1.0",
			Timeout:   10 * time.Second,
		},
		Resources: ResourcesConfig{
			MaxMemoryMB: 0, // auto-detect
			MaxCPU:      0,
		},
	}

	// Should not panic with auto-detect
	ApplyResourceLimits(cfg)
}

func TestApplyResourceLimits_ExplicitCPU(t *testing.T) {
	cfg := &Config{
		Crawler: CrawlerConfig{
			Workers:   5,
			UserAgent: "Test/1.0",
			Timeout:   10 * time.Second,
		},
		Resources: ResourcesConfig{
			MaxMemoryMB: 0,
			MaxCPU:      2,
		},
	}

	prevGOMAXPROCS := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(prevGOMAXPROCS) // restore after test

	ApplyResourceLimits(cfg)

	if got := runtime.GOMAXPROCS(0); got != 2 {
		t.Errorf("GOMAXPROCS = %d, want 2", got)
	}
}

func TestApplyResourceLimits_WorkersAlreadyLow(t *testing.T) {
	cfg := &Config{
		Crawler: CrawlerConfig{
			Workers:   2,
			UserAgent: "Test/1.0",
			Timeout:   10 * time.Second,
		},
		Resources: ResourcesConfig{
			MaxMemoryMB: 200, // max workers = 4, but current is already 2
			MaxCPU:      0,
		},
	}

	ApplyResourceLimits(cfg)

	// Workers should remain at 2 (already below the limit of 4)
	if cfg.Crawler.Workers != 2 {
		t.Errorf("Workers = %d, want 2 (should not increase workers)", cfg.Crawler.Workers)
	}
}
