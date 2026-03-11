package clickhouse

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// downloadURL returns the ClickHouse binary download URL for the current platform.
func downloadURL() (string, error) {
	const base = "https://builds.clickhouse.com/master"

	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return base + "/amd64/clickhouse", nil
		case "arm64":
			return base + "/aarch64/clickhouse", nil
		}
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return base + "/macos-aarch64/clickhouse", nil
		case "amd64":
			return base + "/macos/clickhouse", nil
		}
	}
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf(`ClickHouse does not provide a standalone Windows binary.

CrawlObserver needs Docker to run ClickHouse on Windows.

Step 1 — Install Docker Desktop (free):
         https://docs.docker.com/desktop/setup/install/windows-install/

Step 2 — Open a terminal in the CrawlObserver folder and run:
         docker compose up -d

Step 3 — Start CrawlObserver:
         crawlobserver serve

Full setup guide: https://github.com/SEObserver/crawlobserver#quick-start`)
	}
	return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

// DownloadProgress reports the state of the binary download.
type DownloadProgress struct {
	BytesDownloaded int64
	TotalBytes      int64
	Percent         int
}

// DownloadBinary downloads the ClickHouse binary to {dataDir}/bin/clickhouse.
// Returns the path to the downloaded binary.
// An optional onProgress callback is called at each percent change.
func DownloadBinary(dataDir string, onProgress ...func(DownloadProgress)) (string, error) {
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

	applog.Infof("clickhouse", "Downloading ClickHouse binary from %s ...", dlURL)

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
	var progressCb func(DownloadProgress)
	if len(onProgress) > 0 {
		progressCb = onProgress[0]
	}
	reader := &progressReader{r: resp.Body, total: size, onProgress: progressCb}
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing binary: %w", err)
	}
	f.Close()

	applog.Infof("clickhouse", "Download complete (%.1f MB)", float64(reader.written)/(1024*1024))

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
	applog.Infof("clickhouse", "ClickHouse binary verified: %s", string(out))

	// Atomic rename
	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("installing binary: %w", err)
	}

	return binPath, nil
}

type progressReader struct {
	r          io.Reader
	total      int64
	written    int64
	lastLogPct int
	lastCbPct  int
	onProgress func(DownloadProgress)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.written += int64(n)
	if pr.total > 0 {
		pct := int(pr.written * 100 / pr.total)
		if pct/10 > pr.lastLogPct/10 {
			applog.Infof("clickhouse", "downloading... %d%%", pct)
			pr.lastLogPct = pct
		}
		if pr.onProgress != nil && pct > pr.lastCbPct {
			pr.lastCbPct = pct
			pr.onProgress(DownloadProgress{
				BytesDownloaded: pr.written,
				TotalBytes:      pr.total,
				Percent:         pct,
			})
		}
	}
	return n, err
}
