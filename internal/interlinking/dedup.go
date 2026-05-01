package interlinking

import (
	"net/url"
	"regexp"
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
	"gclsrc":       true, // Google Ads click source
	"dclid":        true, // DoubleClick click ID
	"wbraid":       true, // Google Ads web-to-app
	"gbraid":       true, // Google Ads app-to-web
	"_bg":          true, // Google Ads background
	"_bk":          true, // Google Ads keyword
	"_bm":          true, // Google Ads match type
	"_bn":          true, // Google Ads network
	"_bt":          true, // Google Ads creative
	"_dv":          true, // Google Ads device
	"_loc":         true, // Google Ads location
	"_placement":   true, // Google Ads placement
	"_target":      true, // Google Ads target
	"awsearchcpc":  true, // Google Ads CPC flag
	"yclid":        true, // Yandex click ID
	"twclid":       true, // Twitter click ID
	"ttclid":       true, // TikTok click ID
	"li_fat_id":    true, // LinkedIn ad tracking
	"igshid":       true, // Instagram share ID
	"utm_campaign": true, // Google Analytics
	"utm_content":  true,
	"utm_medium":   true,
	"utm_source":   true,
	"utm_term":     true,
}

// paginationQueryParams lists query parameters that indicate a paginated variant
// of a main page. A page with any of these set to a value > "1" is a paginated
// duplicate and should be skipped for content similarity analysis.
var paginationQueryParams = map[string]bool{
	"page":   true,
	"paged":  true, // WordPress
	"p":      true, // WordPress, but also generic "product id" — context-aware check below
	"_page":  true,
	"pg":     true,
	"start":  true, // offset-based
	"offset": true,
	"from":   true,
	"skip":   true,
}

// paginationPathPattern matches path-based pagination like /page/2, /p/3, /section/page/4.
// Matches "/page/<N>" or "/p/<N>" where N > 1.
var paginationPathPattern = regexp.MustCompile(`(?i)/(page|p|pagina|strana|seite)/([2-9]|[1-9][0-9]+)(/|$)`)

// IsPaginatedURL returns true if the URL looks like a paginated variant of
// another page (e.g. ?page=3, /page/2). Page 1 (or missing page param) is the
// canonical page and is NOT considered paginated.
func IsPaginatedURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Path-based pagination
	if paginationPathPattern.MatchString(u.Path) {
		return true
	}

	// Query-based pagination
	for key, values := range u.Query() {
		lowerKey := strings.ToLower(key)
		if !paginationQueryParams[lowerKey] {
			continue
		}
		// "p" is ambiguous (WordPress post ID vs page number).
		// Only treat as pagination if the value is a small integer > 1.
		for _, v := range values {
			if v == "" || v == "1" || v == "0" {
				continue
			}
			// Must be a pure integer (pagination values are always integers)
			isInt := true
			for _, c := range v {
				if c < '0' || c > '9' {
					isInt = false
					break
				}
			}
			if !isInt {
				continue
			}
			// Small positive integer → almost certainly pagination
			// (large integers are more likely IDs; cap at reasonable max page count)
			if len(v) <= 4 { // up to 9999 pages
				return true
			}
		}
	}

	return false
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

	// Normalize empty path to "/" (e.g. https://example.com → https://example.com/)
	if u.Path == "" {
		u.Path = "/"
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

	// Phase 0: skip paginated variants (page 2, 3, ...).
	// These are not meaningful targets for internal linking suggestions —
	// the main page is the canonical entry point.
	for rawURL := range pageMeta {
		if IsPaginatedURL(rawURL) {
			skip[rawURL] = true
		}
	}

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
