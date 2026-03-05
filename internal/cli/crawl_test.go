package cli

import (
	"os"
	"path/filepath"
	"testing"
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
