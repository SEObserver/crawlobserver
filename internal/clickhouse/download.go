package clickhouse

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// downloadURL returns the ClickHouse binary download URL for the current platform.
func downloadURL() (string, error) {
	const base = "https://github.com/ClickHouse/ClickHouse/releases/latest/download"

	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return base + "/clickhouse-macos-aarch64", nil
		case "amd64":
			return base + "/clickhouse-macos-x86_64", nil
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return base + "/clickhouse-linux-x86_64", nil
		case "arm64":
			return base + "/clickhouse-linux-aarch64", nil
		}
	}
	return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

// DownloadBinary downloads the ClickHouse binary to {dataDir}/bin/clickhouse.
// Returns the path to the downloaded binary.
func DownloadBinary(dataDir string) (string, error) {
	dlURL, err := downloadURL()
	if err != nil {
		return "", err
	}

	binDir := filepath.Join(dataDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("creating bin directory: %w", err)
	}

	binPath := filepath.Join(binDir, "clickhouse")
	tmpPath := binPath + ".tmp"

	log.Printf("Downloading ClickHouse binary from %s ...", dlURL)

	resp, err := http.Get(dlURL)
	if err != nil {
		return "", fmt.Errorf("downloading ClickHouse: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	// Progress logging
	size := resp.ContentLength
	reader := &progressReader{r: resp.Body, total: size}
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing binary: %w", err)
	}
	f.Close()

	log.Printf("Download complete (%.1f MB)", float64(reader.written)/(1024*1024))

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("chmod: %w", err)
	}

	// Verify the binary works
	cmd := exec.Command(tmpPath, "local", "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("binary verification failed: %w\noutput: %s", err, string(out))
	}
	log.Printf("ClickHouse binary verified: %s", string(out))

	// Atomic rename
	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("installing binary: %w", err)
	}

	return binPath, nil
}

type progressReader struct {
	r       io.Reader
	total   int64
	written int64
	lastPct int
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.written += int64(n)
	if pr.total > 0 {
		pct := int(pr.written * 100 / pr.total)
		if pct/10 > pr.lastPct/10 {
			log.Printf("  downloading... %d%%", pct)
			pr.lastPct = pct
		}
	}
	return n, err
}
