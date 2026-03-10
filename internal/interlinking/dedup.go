package interlinking

import (
	"net/url"
	"strings"

	"github.com/SEObserver/crawlobserver/internal/storage"
)

// trackingParams lists query parameters that are tracking/session artifacts
// and do not change the page content. Sorted alphabetically for readability.
var trackingParams = map[string]bool{
	"_ga":          true, // Google Analytics cross-domain
	"_gl":          true, // Google Analytics linker
	"fbclid":       true, // Facebook click ID
	"gclid":        true, // Google Ads click ID
	"mc_cid":       true, // Mailchimp campaign ID
	"mc_eid":       true, // Mailchimp email ID
	"msclkid":      true, // Microsoft Ads click ID
	"pk_campaign":  true, // Matomo campaign
	"pk_kwd":       true, // Matomo keyword
	"pk_source":    true, // Matomo source
	"srsltid":      true, // Google SERP result click ID
	"utm_campaign": true, // Google Analytics
	"utm_content":  true,
	"utm_medium":   true,
	"utm_source":   true,
	"utm_term":     true,
}

// NormalizeURL strips known tracking parameters and normalizes trailing slashes.
// Returns the cleaned URL string. If parsing fails, returns the original.
func NormalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	changed := false

	// Strip trailing slash (keep "/" for root path)
	if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimRight(u.Path, "/")
		changed = true
	}

	// Strip tracking parameters
	q := u.Query()
	for param := range q {
		if trackingParams[strings.ToLower(param)] {
			q.Del(param)
			changed = true
		}
	}

	if !changed {
		return rawURL
	}

	if len(q) == 0 {
		u.RawQuery = ""
	} else {
		u.RawQuery = q.Encode()
	}
	u.Fragment = ""
	return u.String()
}

// DeduplicatePages returns a set of URLs to skip in the corpus.
// Two deduplication mechanisms:
//  1. Canonical: if page A has a non-self canonical pointing to page B (which exists
//     in the crawl), skip A.
//  2. Tracking params: after stripping tracking parameters, if multiple URLs
//     resolve to the same normalized URL, keep only the best one (prefer the
//     canonical/clean URL, then highest PageRank).
func DeduplicatePages(pageMeta map[string]storage.PageMetadata) map[string]bool {
	skip := make(map[string]bool)

	// Phase 1: skip pages with a non-self canonical pointing to another crawled page
	for rawURL, meta := range pageMeta {
		if meta.Canonical != "" && !meta.CanonicalSelf && meta.Canonical != rawURL {
			if _, exists := pageMeta[meta.Canonical]; exists {
				skip[rawURL] = true
			}
		}
	}

	// Phase 2: group remaining pages by normalized URL, keep only the best
	type candidate struct {
		url      string
		pageRank float64
		isClean  bool // URL equals its own normalized form
	}

	groups := make(map[string][]candidate)
	for rawURL := range pageMeta {
		if skip[rawURL] {
			continue
		}
		norm := NormalizeURL(rawURL)
		clean := norm == rawURL
		groups[norm] = append(groups[norm], candidate{
			url:      rawURL,
			pageRank: pageMeta[rawURL].PageRank,
			isClean:  clean,
		})
	}

	for _, candidates := range groups {
		if len(candidates) <= 1 {
			continue
		}
		// Pick the best: prefer clean URL, then highest PageRank
		bestIdx := 0
		for i := 1; i < len(candidates); i++ {
			c := candidates[i]
			b := candidates[bestIdx]
			if c.isClean && !b.isClean {
				bestIdx = i
			} else if c.isClean == b.isClean && c.pageRank > b.pageRank {
				bestIdx = i
			}
		}
		for i, c := range candidates {
			if i != bestIdx {
				skip[c.url] = true
			}
		}
	}

	return skip
}
