package interlinking

import (
	"testing"

	"github.com/SEObserver/crawlobserver/internal/storage"
)

func TestNormalizeURL_StripsTrackingParams(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// UTM parameters
		{"https://example.com/page?utm_source=google&utm_medium=cpc", "https://example.com/page"},
		{"https://example.com/page?utm_campaign=sale&key=val", "https://example.com/page?key=val"},
		// Google click IDs
		{"https://example.com/page?gclid=abc123", "https://example.com/page"},
		{"https://example.com/page?srsltid=xyz", "https://example.com/page"},
		// Facebook
		{"https://example.com/page?fbclid=fb123", "https://example.com/page"},
		// Microsoft
		{"https://example.com/page?msclkid=ms456", "https://example.com/page"},
		// GA cross-domain
		{"https://example.com/page?_ga=2.123&_gl=1*abc", "https://example.com/page"},
		// Matomo
		{"https://example.com/page?pk_campaign=test&pk_source=news", "https://example.com/page"},
		// Mailchimp
		{"https://example.com/page?mc_cid=abc&mc_eid=def", "https://example.com/page"},
		// Mixed: tracking + real params
		{"https://example.com/search?q=test&utm_source=google&page=2", "https://example.com/search?page=2&q=test"},
		// No tracking params — unchanged (original param order preserved)
		{"https://example.com/page?id=42&cat=shoes", "https://example.com/page?id=42&cat=shoes"},
		// No params at all
		{"https://example.com/page", "https://example.com/page"},
		// Case insensitive param names
		{"https://example.com/page?UTM_SOURCE=google", "https://example.com/page"},
		// Multiple tracking params mixed with legitimate ones
		{"https://example.com/p?a=1&gclid=x&b=2&fbclid=y&c=3", "https://example.com/p?a=1&b=2&c=3"},
	}

	for _, tc := range tests {
		got := NormalizeURL(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeURL_InvalidURL(t *testing.T) {
	// Invalid URL should be returned as-is
	bad := "://not-a-url"
	got := NormalizeURL(bad)
	if got != bad {
		t.Errorf("NormalizeURL(%q) = %q, want unchanged", bad, got)
	}
}

func TestDeduplicatePages_CanonicalSkip(t *testing.T) {
	meta := map[string]storage.PageMetadata{
		"https://example.com/page": {
			PageRank:      10,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page",
		},
		"https://example.com/page?ref=123": {
			PageRank:      5,
			CanonicalSelf: false,
			Canonical:     "https://example.com/page",
		},
	}

	skip := DeduplicatePages(meta)

	if skip["https://example.com/page"] {
		t.Error("canonical target should NOT be skipped")
	}
	if !skip["https://example.com/page?ref=123"] {
		t.Error("non-self canonical page should be skipped")
	}
}

func TestDeduplicatePages_CanonicalNotInCrawl(t *testing.T) {
	// If canonical points to a URL not in the crawl, don't skip
	meta := map[string]storage.PageMetadata{
		"https://example.com/page?ref=123": {
			PageRank:      5,
			CanonicalSelf: false,
			Canonical:     "https://other.com/page", // not in crawl
		},
	}

	skip := DeduplicatePages(meta)

	if skip["https://example.com/page?ref=123"] {
		t.Error("should NOT skip when canonical target is not in crawl")
	}
}

func TestDeduplicatePages_TrackingParamDedup(t *testing.T) {
	meta := map[string]storage.PageMetadata{
		"https://example.com/page": {
			PageRank:      10,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page",
		},
		"https://example.com/page?utm_source=google": {
			PageRank:      8,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page?utm_source=google",
		},
		"https://example.com/page?gclid=abc": {
			PageRank:      3,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page?gclid=abc",
		},
	}

	skip := DeduplicatePages(meta)

	// The clean URL (no tracking params) should be kept
	if skip["https://example.com/page"] {
		t.Error("clean URL should NOT be skipped")
	}
	if !skip["https://example.com/page?utm_source=google"] {
		t.Error("utm variant should be skipped")
	}
	if !skip["https://example.com/page?gclid=abc"] {
		t.Error("gclid variant should be skipped")
	}
}

func TestDeduplicatePages_PreferHighestPR(t *testing.T) {
	// When no clean URL exists, keep the one with highest PageRank
	meta := map[string]storage.PageMetadata{
		"https://example.com/page?utm_source=google": {
			PageRank:      10,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page?utm_source=google",
		},
		"https://example.com/page?gclid=abc": {
			PageRank:      5,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page?gclid=abc",
		},
	}

	skip := DeduplicatePages(meta)

	// utm_source variant has higher PR → kept
	if skip["https://example.com/page?utm_source=google"] {
		t.Error("highest PR variant should NOT be skipped")
	}
	if !skip["https://example.com/page?gclid=abc"] {
		t.Error("lower PR variant should be skipped")
	}
}

func TestDeduplicatePages_NoSkipForDistinctPages(t *testing.T) {
	meta := map[string]storage.PageMetadata{
		"https://example.com/page-a": {
			PageRank:      10,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page-a",
		},
		"https://example.com/page-b": {
			PageRank:      8,
			CanonicalSelf: true,
			Canonical:     "https://example.com/page-b",
		},
	}

	skip := DeduplicatePages(meta)

	if len(skip) != 0 {
		t.Errorf("distinct pages should not be skipped, got %d skips", len(skip))
	}
}

func TestDeduplicatePages_BothMechanismsCombined(t *testing.T) {
	meta := map[string]storage.PageMetadata{
		// Clean canonical page
		"https://example.com/product": {
			PageRank:      15,
			CanonicalSelf: true,
			Canonical:     "https://example.com/product",
		},
		// Has canonical pointing to /product
		"https://example.com/product?variant=blue": {
			PageRank:      5,
			CanonicalSelf: false,
			Canonical:     "https://example.com/product",
		},
		// Self-canonical but with tracking param → DUST
		"https://example.com/product?utm_source=email": {
			PageRank:      3,
			CanonicalSelf: true,
			Canonical:     "https://example.com/product?utm_source=email",
		},
		// Completely different page — kept
		"https://example.com/about": {
			PageRank:      8,
			CanonicalSelf: true,
			Canonical:     "https://example.com/about",
		},
	}

	skip := DeduplicatePages(meta)

	if skip["https://example.com/product"] {
		t.Error("canonical target should NOT be skipped")
	}
	if !skip["https://example.com/product?variant=blue"] {
		t.Error("non-self canonical should be skipped")
	}
	if !skip["https://example.com/product?utm_source=email"] {
		t.Error("tracking param DUST should be skipped")
	}
	if skip["https://example.com/about"] {
		t.Error("distinct page should NOT be skipped")
	}
}

func TestDeduplicatePages_RealParamsNotStripped(t *testing.T) {
	// Pages with real query params (not tracking) should NOT be deduped
	meta := map[string]storage.PageMetadata{
		"https://example.com/search?q=shoes": {
			PageRank:      10,
			CanonicalSelf: true,
			Canonical:     "https://example.com/search?q=shoes",
		},
		"https://example.com/search?q=boots": {
			PageRank:      8,
			CanonicalSelf: true,
			Canonical:     "https://example.com/search?q=boots",
		},
	}

	skip := DeduplicatePages(meta)

	if len(skip) != 0 {
		t.Errorf("different real query params should not be deduped, got %d skips", len(skip))
	}
}

func TestDeduplicatePages_EmptyInput(t *testing.T) {
	skip := DeduplicatePages(map[string]storage.PageMetadata{})
	if len(skip) != 0 {
		t.Errorf("empty input should return empty skip set, got %d", len(skip))
	}
}
