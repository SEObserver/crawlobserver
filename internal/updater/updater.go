package updater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

const (
	repoOwner = "SEObserver"
	repoName  = "crawlobserver"
)

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
	HTMLURL string  `json:"html_url"`
}

// Asset represents a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Version is the current build version, set at build time via ldflags.
var Version = "dev"

// UpdateStatus represents the current update check state.
type UpdateStatus struct {
	mu             sync.RWMutex
	Available      bool      `json:"available"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version"`
	ReleaseURL     string    `json:"release_url"`
	CheckedAt      time.Time `json:"checked_at"`
	Error          string    `json:"error,omitempty"`
	release        *Release
}

// NewUpdateStatus creates a new UpdateStatus.
func NewUpdateStatus() *UpdateStatus {
	return &UpdateStatus{CurrentVersion: Version}
}

// Check performs a background update check and updates the status.
func (s *UpdateStatus) Check() {
	release, available, err := CheckUpdate()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.CheckedAt = time.Now()
	if err != nil {
		s.Error = err.Error()
		return
	}
	s.Error = ""
	s.Available = available
	s.release = release
	if release != nil {
		s.LatestVersion = strings.TrimPrefix(release.TagName, "v")
		s.ReleaseURL = release.HTMLURL
	}
}

// Snapshot returns a copy safe for JSON serialization.
func (s *UpdateStatus) Snapshot() UpdateStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return UpdateStatus{
		Available:      s.Available,
		CurrentVersion: s.CurrentVersion,
		LatestVersion:  s.LatestVersion,
		ReleaseURL:     s.ReleaseURL,
		CheckedAt:      s.CheckedAt,
		Error:          s.Error,
	}
}

// Release returns the cached release (may be nil).
func (s *UpdateStatus) Release() *Release {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.release
}

// CheckUpdate checks if a newer version is available on GitHub.
func CheckUpdate() (*Release, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, false, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, false, nil
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, false, fmt.Errorf("parsing release: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")

	if latest != current && current != "dev" {
		return &release, true, nil
	}

	return &release, false, nil
}

// DownloadUpdate downloads the appropriate binary for the current platform.
func DownloadUpdate(release *Release) (string, error) {
	assetName := expectedAssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return "", fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	applog.Infof("updater", "Downloading %s...", assetName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "crawlobserver-update-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("writing update: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("chmod: %w", err)
	}

	return tmpFile.Name(), nil
}

// SelfUpdate replaces the current binary with the downloaded update.
func SelfUpdate(newBinaryPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Rename current binary as backup
	backupPath := execPath + ".bak"
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Move new binary into place
	if err := os.Rename(newBinaryPath, execPath); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, execPath)
		return fmt.Errorf("installing update: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	applog.Info("updater", "Update installed. Restart to use the new version.")
	return nil
}

func expectedAssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("crawlobserver-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// ExpectedDesktopAssetName returns the expected .app.tar.gz asset name for desktop updates.
func ExpectedDesktopAssetName() string {
	return fmt.Sprintf("CrawlObserver-%s-%s.app.tar.gz", runtime.GOOS, runtime.GOARCH)
}

// DownloadDesktopUpdate downloads and extracts the .app.tar.gz for desktop mode.
// Returns the path to the extracted .app bundle.
func DownloadDesktopUpdate(release *Release) (string, error) {
	assetName := ExpectedDesktopAssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("no desktop bundle found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	applog.Infof("updater", "Downloading %s...", assetName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	tmpDir, err := os.MkdirTemp("", "crawlobserver-desktop-update-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var appPath string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("reading tar: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}
		dest := filepath.Join(tmpDir, cleanName)

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(dest, os.FileMode(hdr.Mode))
			if strings.HasSuffix(cleanName, ".app") && appPath == "" {
				appPath = dest
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(dest), 0755)
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("extracting %s: %w", cleanName, err)
			}
			io.Copy(f, tr)
			f.Close()
		}

		// Track top-level .app directory
		parts := strings.SplitN(cleanName, "/", 2)
		if strings.HasSuffix(parts[0], ".app") && appPath == "" {
			appPath = filepath.Join(tmpDir, parts[0])
		}
	}

	if appPath == "" {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("no .app bundle found in archive")
	}

	applog.Infof("updater", "Desktop update extracted to %s", appPath)
	return appPath, nil
}

// SelfUpdateDesktop replaces the current .app bundle with the new one.
func SelfUpdateDesktop(newAppPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Walk up from the binary to find the .app bundle
	// e.g. /path/to/CrawlObserver.app/Contents/MacOS/CrawlObserver -> /path/to/CrawlObserver.app
	currentApp := execPath
	for !strings.HasSuffix(currentApp, ".app") {
		parent := filepath.Dir(currentApp)
		if parent == currentApp {
			return fmt.Errorf("could not find .app bundle from executable path: %s", execPath)
		}
		currentApp = parent
	}

	backupPath := currentApp + ".bak"

	// Rename current .app as backup
	if err := os.Rename(currentApp, backupPath); err != nil {
		return fmt.Errorf("backing up current app: %w", err)
	}

	// Move new .app into place
	if err := os.Rename(newAppPath, currentApp); err != nil {
		os.Rename(backupPath, currentApp)
		return fmt.Errorf("installing update: %w", err)
	}

	// Remove backup
	os.RemoveAll(backupPath)

	applog.Info("updater", "Desktop update installed. Restart the application to use the new version.")
	return nil
}
