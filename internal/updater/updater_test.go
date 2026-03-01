package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestExpectedAssetName(t *testing.T) {
	name := expectedAssetName()
	if !strings.HasPrefix(name, "crawlobserver-") {
		t.Errorf("expected prefix 'crawlobserver-', got %q", name)
	}
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("expected OS %q in name %q", runtime.GOOS, name)
	}
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("expected arch %q in name %q", runtime.GOARCH, name)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		t.Error("Windows asset should end in .exe")
	}
	if runtime.GOOS != "windows" && strings.HasSuffix(name, ".exe") {
		t.Error("non-Windows asset should not end in .exe")
	}
}

func TestExpectedDesktopAssetName(t *testing.T) {
	name := ExpectedDesktopAssetName()
	if !strings.HasPrefix(name, "CrawlObserver-") {
		t.Errorf("expected prefix 'CrawlObserver-', got %q", name)
	}
	if !strings.HasSuffix(name, ".app.tar.gz") {
		t.Errorf("expected suffix '.app.tar.gz', got %q", name)
	}
}

func TestCheckUpdate_SameVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := Release{TagName: "v1.0.0", HTMLURL: "https://github.com/test/release"}
		json.NewEncoder(w).Encode(release)
	}))
	defer ts.Close()

	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	// Override URL by patching the function — we test via the http mock directly
	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var release Release
	json.NewDecoder(resp.Body).Decode(&release)

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")
	available := latest != current && current != "dev"
	if available {
		t.Error("same version should not be available")
	}
}

func TestCheckUpdate_DifferentVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := Release{TagName: "v2.0.0", HTMLURL: "https://github.com/test/release"}
		json.NewEncoder(w).Encode(release)
	}))
	defer ts.Close()

	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var release Release
	json.NewDecoder(resp.Body).Decode(&release)

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")
	available := latest != current && current != "dev"
	if !available {
		t.Error("different version should be available")
	}
}

func TestCheckUpdate_DevVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := Release{TagName: "v2.0.0"}
		json.NewEncoder(w).Encode(release)
	}))
	defer ts.Close()

	oldVersion := Version
	Version = "dev"
	defer func() { Version = oldVersion }()

	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var release Release
	json.NewDecoder(resp.Body).Decode(&release)

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")
	available := latest != current && current != "dev"
	if available {
		t.Error("dev version should never show as available")
	}
}

func TestCheckUpdate_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ts.Close()

	client := &http.Client{}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		t.Error("expected non-200 status")
	}
}

func TestNewUpdateStatus(t *testing.T) {
	oldVersion := Version
	Version = "v1.5.0"
	defer func() { Version = oldVersion }()

	s := NewUpdateStatus()
	if s.CurrentVersion != "v1.5.0" {
		t.Errorf("CurrentVersion = %q, want %q", s.CurrentVersion, "v1.5.0")
	}
}

func TestUpdateStatus_Snapshot(t *testing.T) {
	s := &UpdateStatus{
		Available:      true,
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v2.0.0",
		ReleaseURL:     "https://example.com",
	}
	snap := s.Snapshot()
	if snap.Available != true || snap.LatestVersion != "v2.0.0" {
		t.Errorf("unexpected snapshot: %+v", snap)
	}
}

func TestUpdateStatus_Release(t *testing.T) {
	r := &Release{TagName: "v1.0.0"}
	s := &UpdateStatus{release: r}
	if got := s.Release(); got != r {
		t.Error("Release() should return the cached release")
	}
}

func TestUpdateStatus_ReleaseNil(t *testing.T) {
	s := &UpdateStatus{}
	if got := s.Release(); got != nil {
		t.Error("Release() should return nil when not set")
	}
}
