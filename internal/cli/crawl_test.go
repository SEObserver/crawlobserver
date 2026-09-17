package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/spf13/viper"
)

func TestReadSeedsFile_ValidURLs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "seeds.txt")
	content := "https://example.com\nhttps://example.org\nhttps://example.net\n"
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	seeds, err := readSeedsFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 3 {
		t.Fatalf("expected 3 seeds, got %d", len(seeds))
	}
	want := []string{"https://example.com", "https://example.org", "https://example.net"}
	for i, s := range seeds {
		if s != want[i] {
			t.Errorf("seed[%d] = %q, want %q", i, s, want[i])
		}
	}
}

func TestReadSeedsFile_CommentsAndEmptyLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "seeds.txt")
	content := `# This is a comment
https://example.com

# Another comment

https://example.org
# trailing comment
`
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	seeds, err := readSeedsFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds, got %d: %v", len(seeds), seeds)
	}
	if seeds[0] != "https://example.com" {
		t.Errorf("seed[0] = %q, want %q", seeds[0], "https://example.com")
	}
	if seeds[1] != "https://example.org" {
		t.Errorf("seed[1] = %q, want %q", seeds[1], "https://example.org")
	}
}

func TestReadSeedsFile_TabSeparatedFormat(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "seeds.txt")
	content := "https://example.com\t1.0\nhttps://example.org\t0.5\nhttps://example.net\t0.8\n"
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	seeds, err := readSeedsFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 3 {
		t.Fatalf("expected 3 seeds, got %d", len(seeds))
	}
	want := []string{"https://example.com", "https://example.org", "https://example.net"}
	for i, s := range seeds {
		if s != want[i] {
			t.Errorf("seed[%d] = %q, want %q", i, s, want[i])
		}
	}
}

func TestReadSeedsFile_FileNotFound(t *testing.T) {
	_, err := readSeedsFile("/nonexistent/path/seeds.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestReadSeedsFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "seeds.txt")
	if err := os.WriteFile(f, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	seeds, err := readSeedsFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 0 {
		t.Fatalf("expected 0 seeds, got %d", len(seeds))
	}
}

func TestReadSeedsFile_WhitespaceAroundURLs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "seeds.txt")
	content := "  https://example.com  \n\thttps://example.org\t\n"
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	seeds, err := readSeedsFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds, got %d: %v", len(seeds), seeds)
	}
	if seeds[0] != "https://example.com" {
		t.Errorf("seed[0] = %q, want %q", seeds[0], "https://example.com")
	}
	if seeds[1] != "https://example.org" {
		t.Errorf("seed[1] = %q, want %q", seeds[1], "https://example.org")
	}
}

func TestReadSeedsFile_MixedFormatFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "seeds.txt")
	content := `# Seed URLs for crawl
https://example.com
https://example.org	0.9

# Tab-separated with priority
https://example.net	0.5
   https://example.edu	1.0
`
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	seeds, err := readSeedsFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{
		"https://example.com",
		"https://example.org",
		"https://example.net",
		"https://example.edu",
	}
	if len(seeds) != len(want) {
		t.Fatalf("expected %d seeds, got %d: %v", len(want), len(seeds), seeds)
	}
	for i, s := range seeds {
		if s != want[i] {
			t.Errorf("seed[%d] = %q, want %q", i, s, want[i])
		}
	}
}

// The --store-link-position flag defaults to true, which must not shadow a
// config file that turns the feature off: viper only lets a flag win once it is
// actually set. Both directions are checked, so that a binding that stopped
// reading the config key at all cannot pass by landing on false.
func TestStoreLinkPositionFollowsConfigFile(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{"absent from config", "crawler:\n  workers: 4\n", true},
		{"turned off", "crawler:\n  store_link_position: false\n", false},
		{"turned on", "crawler:\n  store_link_position: true\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			bindCrawlFlags()
			viper.SetConfigType("yaml")
			if err := viper.ReadConfig(strings.NewReader(tt.yaml)); err != nil {
				t.Fatalf("ReadConfig() error = %v", err)
			}

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Crawler.StoreLinkPosition != tt.want {
				t.Errorf("StoreLinkPosition = %v, want %v", cfg.Crawler.StoreLinkPosition, tt.want)
			}
		})
	}
}

// The flag itself must exist, default to on, and reach the config through the
// command's own binding. Asserting through config.Load means a binding pointing
// at the wrong key fails here rather than silently doing nothing.
func TestStoreLinkPositionFlagReachesConfig(t *testing.T) {
	flag := crawlCmd.Flags().Lookup("store-link-position")
	if flag == nil {
		t.Fatal("crawl command has no --store-link-position flag")
	}
	if flag.DefValue != "true" {
		t.Errorf("--store-link-position default = %q, want %q", flag.DefValue, "true")
	}

	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	})

	bindCrawlFlags()
	if err := crawlCmd.Flags().Set("store-link-position", "false"); err != nil {
		t.Fatalf("setting the flag: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Crawler.StoreLinkPosition {
		t.Error("StoreLinkPosition = true, want the false passed on the command line")
	}
}

func TestLinkPositionLimitFromConfigAndFlag(t *testing.T) {
	flag := crawlCmd.Flags().Lookup("max-link-positions-per-page")
	if flag == nil {
		t.Fatal("missing --max-link-positions-per-page")
	}
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	})
	bindCrawlFlags()
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader("crawler:\n  max_link_positions_per_page: 250\n")); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Crawler.MaxLinkPositionsPerPage != 250 {
		t.Fatalf("config limit = %d", cfg.Crawler.MaxLinkPositionsPerPage)
	}
	if err := crawlCmd.Flags().Set("max-link-positions-per-page", "50"); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Crawler.MaxLinkPositionsPerPage != 50 {
		t.Fatalf("flag limit = %d", cfg.Crawler.MaxLinkPositionsPerPage)
	}
}
