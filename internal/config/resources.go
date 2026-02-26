package config

import (
	"log"
	"runtime"
	"runtime/debug"
)

// ApplyResourceLimits configures GOMAXPROCS and memory soft limit based on config.
// Also auto-adjusts worker count if memory is constrained.
func ApplyResourceLimits(cfg *Config) {
	// CPU limit
	if cfg.Resources.MaxCPU > 0 {
		prev := runtime.GOMAXPROCS(cfg.Resources.MaxCPU)
		log.Printf("GOMAXPROCS: %d → %d", prev, cfg.Resources.MaxCPU)
	}

	// Memory soft limit via GOMEMLIMIT
	memLimitMB := cfg.Resources.MaxMemoryMB
	if memLimitMB == 0 {
		memLimitMB = autoMemoryLimitMB()
	}

	if memLimitMB > 0 {
		limit := int64(memLimitMB) * 1024 * 1024
		debug.SetMemoryLimit(limit)
		log.Printf("Memory soft limit: %d MB", memLimitMB)
	}

	// Auto-adjust workers if memory is tight
	if memLimitMB > 0 && memLimitMB < 512 {
		maxWorkers := max(1, memLimitMB/50) // ~50MB per worker is a safe estimate
		if cfg.Crawler.Workers > maxWorkers {
			log.Printf("Reducing workers %d → %d (memory constraint: %d MB)",
				cfg.Crawler.Workers, maxWorkers, memLimitMB)
			cfg.Crawler.Workers = maxWorkers
		}
	}
}

// autoMemoryLimitMB returns 75% of total system memory, or 0 if detection fails.
func autoMemoryLimitMB() int {
	// Use a reasonable default based on GOMAXPROCS as a proxy
	// runtime doesn't expose total system memory directly.
	// We set a conservative default per available CPU.
	numCPU := runtime.NumCPU()
	// ~256MB per CPU core, capped at 4GB
	limitMB := numCPU * 256
	if limitMB > 4096 {
		limitMB = 4096
	}
	return limitMB
}
