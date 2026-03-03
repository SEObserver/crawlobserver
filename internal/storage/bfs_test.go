package storage

import (
	"testing"
)

func TestComputeBFSDepths_SingleSeed(t *testing.T) {
	// Simple graph: seed -> A -> B, seed -> C
	seedURLs := []string{"https://example.com"}
	crawledSet := map[string]bool{
		"https://example.com":   true,
		"https://example.com/a": true,
		"https://example.com/b": true,
		"https://example.com/c": true,
	}
	adj := map[string][]string{
		"https://example.com":   {"https://example.com/a", "https://example.com/c"},
		"https://example.com/a": {"https://example.com/b"},
	}

	result := ComputeBFSDepths(seedURLs, crawledSet, adj)

	tests := []struct {
		url       string
		wantDepth uint16
	}{
		{"https://example.com", 0},
		{"https://example.com/a", 1},
		{"https://example.com/b", 2},
		{"https://example.com/c", 1},
	}
	for _, tt := range tests {
		if got := result.Depths[tt.url]; got != tt.wantDepth {
			t.Errorf("depth(%s) = %d, want %d", tt.url, got, tt.wantDepth)
		}
	}

	// Seed should have empty found_on
	if result.FoundOn["https://example.com"] != "" {
		t.Errorf("seed found_on = %q, want empty", result.FoundOn["https://example.com"])
	}
	// A should be found on seed
	if result.FoundOn["https://example.com/a"] != "https://example.com" {
		t.Errorf("a found_on = %q, want seed", result.FoundOn["https://example.com/a"])
	}
}

func TestComputeBFSDepths_OnlySeedGetsDepthZero(t *testing.T) {
	// This is the regression test for the bug where resume/retry
	// passed all uncrawled URLs as seeds, causing 1600+ pages at depth 0.
	// Only the REAL seed should have depth 0.
	seedURLs := []string{"https://example.com"}
	crawledSet := map[string]bool{
		"https://example.com":   true,
		"https://example.com/a": true,
		"https://example.com/b": true,
		"https://example.com/c": true,
		"https://example.com/d": true,
		"https://example.com/e": true,
		"https://example.com/f": true,
		"https://example.com/g": true,
		"https://example.com/h": true,
		"https://example.com/i": true,
		"https://example.com/j": true,
	}
	adj := map[string][]string{
		"https://example.com":   {"https://example.com/a", "https://example.com/b"},
		"https://example.com/a": {"https://example.com/c", "https://example.com/d"},
		"https://example.com/b": {"https://example.com/e", "https://example.com/f"},
		"https://example.com/c": {"https://example.com/g"},
		"https://example.com/d": {"https://example.com/h"},
		"https://example.com/e": {"https://example.com/i"},
		"https://example.com/f": {"https://example.com/j"},
	}

	result := ComputeBFSDepths(seedURLs, crawledSet, adj)

	// Count how many pages have depth 0
	depthZeroCount := 0
	for _, d := range result.Depths {
		if d == 0 {
			depthZeroCount++
		}
	}
	if depthZeroCount != 1 {
		t.Errorf("got %d pages at depth 0, want exactly 1 (the seed)", depthZeroCount)
	}

	// Verify the seed is the one at depth 0
	if result.Depths["https://example.com"] != 0 {
		t.Errorf("seed depth = %d, want 0", result.Depths["https://example.com"])
	}
}

func TestComputeBFSDepths_CorruptSeedsWouldCauseMultipleDepthZero(t *testing.T) {
	// Simulate the bug scenario: if someone accidentally passes many URLs as seeds,
	// they ALL get depth 0. This test documents the expected (bad) behavior
	// when seeds are wrong, to show why the engine must preserve original seeds.
	corruptSeeds := []string{
		"https://example.com",
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}
	crawledSet := map[string]bool{
		"https://example.com":   true,
		"https://example.com/a": true,
		"https://example.com/b": true,
		"https://example.com/c": true,
		"https://example.com/d": true,
	}
	adj := map[string][]string{
		"https://example.com":   {"https://example.com/a"},
		"https://example.com/a": {"https://example.com/b"},
		"https://example.com/b": {"https://example.com/c"},
		"https://example.com/c": {"https://example.com/d"},
	}

	result := ComputeBFSDepths(corruptSeeds, crawledSet, adj)

	depthZeroCount := 0
	for _, d := range result.Depths {
		if d == 0 {
			depthZeroCount++
		}
	}
	// With corrupt seeds, 4 pages get depth 0 instead of 1
	if depthZeroCount != 4 {
		t.Errorf("with corrupt seeds: got %d at depth 0, want 4", depthZeroCount)
	}
}

func TestComputeBFSDepths_SeedTrailingSlash(t *testing.T) {
	// Seed with trailing slash should match URL without trailing slash
	seedURLs := []string{"https://example.com/"}
	crawledSet := map[string]bool{
		"https://example.com":   true,
		"https://example.com/a": true,
	}
	adj := map[string][]string{
		"https://example.com": {"https://example.com/a"},
	}

	result := ComputeBFSDepths(seedURLs, crawledSet, adj)

	if result.Depths["https://example.com"] != 0 {
		t.Errorf("seed depth = %d, want 0", result.Depths["https://example.com"])
	}
	if result.Depths["https://example.com/a"] != 1 {
		t.Errorf("/a depth = %d, want 1", result.Depths["https://example.com/a"])
	}
}

func TestComputeBFSDepths_SeedWithoutTrailingSlash(t *testing.T) {
	// Seed without trailing slash should match URL with trailing slash
	seedURLs := []string{"https://example.com"}
	crawledSet := map[string]bool{
		"https://example.com/":  true,
		"https://example.com/a": true,
	}
	adj := map[string][]string{
		"https://example.com/": {"https://example.com/a"},
	}

	result := ComputeBFSDepths(seedURLs, crawledSet, adj)

	if result.Depths["https://example.com/"] != 0 {
		t.Errorf("seed depth = %d, want 0", result.Depths["https://example.com/"])
	}
	if result.Depths["https://example.com/a"] != 1 {
		t.Errorf("/a depth = %d, want 1", result.Depths["https://example.com/a"])
	}
}

func TestComputeBFSDepths_Orphans(t *testing.T) {
	seedURLs := []string{"https://example.com"}
	crawledSet := map[string]bool{
		"https://example.com":        true,
		"https://example.com/a":      true,
		"https://example.com/orphan": true,
	}
	adj := map[string][]string{
		"https://example.com": {"https://example.com/a"},
	}

	result := ComputeBFSDepths(seedURLs, crawledSet, adj)

	// Orphan should get maxDepth+1
	if result.Depths["https://example.com/orphan"] != 2 {
		t.Errorf("orphan depth = %d, want 2 (maxDepth=1, orphan=2)", result.Depths["https://example.com/orphan"])
	}
	// Orphan found_on should be empty
	if result.FoundOn["https://example.com/orphan"] != "" {
		t.Errorf("orphan found_on = %q, want empty", result.FoundOn["https://example.com/orphan"])
	}
}

func TestComputeBFSDepths_ShortestPath(t *testing.T) {
	// BFS should assign the shortest path depth
	// seed -> A -> B -> C (depth 3 via this path)
	// seed -> C            (depth 1 via this shortcut)
	seedURLs := []string{"https://example.com"}
	crawledSet := map[string]bool{
		"https://example.com":   true,
		"https://example.com/a": true,
		"https://example.com/b": true,
		"https://example.com/c": true,
	}
	adj := map[string][]string{
		"https://example.com":   {"https://example.com/a", "https://example.com/c"},
		"https://example.com/a": {"https://example.com/b"},
		"https://example.com/b": {"https://example.com/c"},
	}

	result := ComputeBFSDepths(seedURLs, crawledSet, adj)

	// C should be at depth 1 (shortcut from seed), not depth 3
	if result.Depths["https://example.com/c"] != 1 {
		t.Errorf("/c depth = %d, want 1 (shortest path)", result.Depths["https://example.com/c"])
	}
}

func TestComputeBFSDepths_OrphanDepthNotInflatedByNonCrawledURLs(t *testing.T) {
	// Regression: non-crawled link targets were included in maxDepth calculation,
	// inflating orphan depth. E.g., if maxCrawled=2 but a non-crawled target got
	// depth 5, orphans would be assigned depth 6 instead of 3.
	seedURLs := []string{"https://example.com"}
	crawledSet := map[string]bool{
		"https://example.com":        true,
		"https://example.com/a":      true,
		"https://example.com/orphan": true,
	}
	adj := map[string][]string{
		"https://example.com": {"https://example.com/a"},
		// /a links to external targets that weren't crawled
		"https://example.com/a": {"https://external.com/x", "https://external.com/y"},
	}

	result := ComputeBFSDepths(seedURLs, crawledSet, adj)

	// Max crawled depth is 1 (/a), so orphan depth should be 2
	if result.Depths["https://example.com/orphan"] != 2 {
		t.Errorf("orphan depth = %d, want 2 (maxCrawled=1, orphan=2)", result.Depths["https://example.com/orphan"])
	}
	// Non-crawled URLs should NOT appear in depths map
	if _, ok := result.Depths["https://external.com/x"]; ok {
		t.Error("non-crawled URL https://external.com/x should not be in depths map")
	}
	if _, ok := result.Depths["https://external.com/y"]; ok {
		t.Error("non-crawled URL https://external.com/y should not be in depths map")
	}
}
