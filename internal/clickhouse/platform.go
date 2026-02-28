package clickhouse

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// DefaultDataDir returns the platform-specific default data directory.
func DefaultDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "CrawlObserver")
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			home, _ := os.UserHomeDir()
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appdata, "CrawlObserver")
	default: // linux and others
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "crawlobserver")
	}
}

// findAvailablePorts returns two available TCP ports by binding to port 0.
func findAvailablePorts() (tcpPort, httpPort int, err error) {
	tcpPort, err = findFreePort()
	if err != nil {
		return 0, 0, fmt.Errorf("finding TCP port: %w", err)
	}
	httpPort, err = findFreePort()
	if err != nil {
		return 0, 0, fmt.Errorf("finding HTTP port: %w", err)
	}
	return tcpPort, httpPort, nil
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// FindBinary locates the ClickHouse binary. Search order:
// 1. configured path (explicit)
// 2. {dataDir}/bin/clickhouse
// 3. $PATH
func FindBinary(configured, dataDir string) string {
	if configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured
		}
	}

	local := filepath.Join(dataDir, "bin", "clickhouse")
	if _, err := os.Stat(local); err == nil {
		return local
	}

	if p, err := exec.LookPath("clickhouse"); err == nil {
		return p
	}

	return ""
}
