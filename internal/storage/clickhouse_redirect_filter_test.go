//go:build integration

package storage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// insertTestPages batch-inserts pages for redirect filter tests.
func insertTestPages(t *testing.T, s *Store, sessionID string, pages []PageRow) {
	t.Helper()
	ctx := context.Background()
	if err := s.InsertPages(ctx, pages); err != nil {
		t.Fatalf("inserting test pages: %v", err)
	}
	// Wait for ClickHouse to process
	time.Sleep(500 * time.Millisecond)
}

// cleanupRedirectTestSession removes all test data for redirect filter tests.
func cleanupRedirectTestSession(t *testing.T, s *Store, sessionID string) {
	t.Helper()
	ctx := context.Background()
	tables := []string{"pages", "links", "sitemap_urls", "sitemaps", "crawl_sessions"}
	for _, tbl := range tables {
		if err := s.conn.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE crawlobserver.%s DELETE WHERE crawl_session_id = ?", tbl,
		), sessionID); err != nil {
			t.Logf("cleanup %s: %v", tbl, err)
		}
	}
	time.Sleep(500 * time.Millisecond)
}

// redirectTestSessionID is a fixed UUID for redirect filter tests.
const redirectTestSessionID = "11111111-2222-3333-4444-555555555555"

// setupRedirectTestData creates the standard test dataset:
// - 3 normal pages (status 200, final_url = '' or = url, pagerank > 0)
// - 2 followed redirects (status 200, final_url != url, pagerank > 0)
// - 1 true 301 redirect (status 301)
func setupRedirectTestData(t *testing.T, s *Store) {
	t.Helper()
	now := time.Now()

	pages := []PageRow{
		// Normal pages
		{CrawlSessionID: redirectTestSessionID, URL: "https://example.com/page1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Page 1", PageRank: 10.0, Depth: 1, WordCount: 500, CrawledAt: now, Lang: "en"},
		{CrawlSessionID: redirectTestSessionID, URL: "https://example.com/page2", FinalURL: "https://example.com/page2", StatusCode: 200, ContentType: "text/html", Title: "Page 2", PageRank: 8.0, Depth: 1, WordCount: 300, CrawledAt: now, Lang: "fr"},
		{CrawlSessionID: redirectTestSessionID, URL: "https://example.com/page3", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Page 3", PageRank: 5.0, Depth: 2, WordCount: 200, CrawledAt: now, Lang: "en"},
		// Followed redirects (status 200, final_url != url)
		{CrawlSessionID: redirectTestSessionID, URL: "https://example.com/old1", FinalURL: "https://example.com/page1", StatusCode: 200, ContentType: "text/html", Title: "Page 1", PageRank: 3.0, Depth: 1, WordCount: 500, CrawledAt: now, Lang: "en"},
		{CrawlSessionID: redirectTestSessionID, URL: "https://example.com/old2", FinalURL: "https://example.com/page2", StatusCode: 200, ContentType: "text/html", Title: "Page 2", PageRank: 2.0, Depth: 1, WordCount: 300, CrawledAt: now, Lang: "fr"},
		// True 301 redirect
		{CrawlSessionID: redirectTestSessionID, URL: "https://example.com/moved", FinalURL: "https://example.com/page3", StatusCode: 301, ContentType: "", Title: "", PageRank: 0, Depth: 1, CrawledAt: now},
	}
	insertTestPages(t, s, redirectTestSessionID, pages)
}

// ===========================================================================
// Basic functional tests — smoke-test each query surface against the standard
// dataset (3 normal + 2 followed redirects + 1 true 301). Every filtered
// query must see exactly 3 pages.
// ===========================================================================

func TestCountPages_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)
	setupRedirectTestData(t, s)

	count, err := s.CountPages(ctx, redirectTestSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 3 {
		t.Errorf("CountPages: expected 3, got %d", count)
	}
}

func TestListPages_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)
	setupRedirectTestData(t, s)

	pages, err := s.ListPages(ctx, redirectTestSessionID, 100, 0, nil, nil)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != 3 {
		t.Errorf("ListPages: expected 3 pages, got %d", len(pages))
		for _, p := range pages {
			t.Logf("  url=%s final_url=%s status=%d", p.URL, p.FinalURL, p.StatusCode)
		}
	}
	for _, p := range pages {
		if p.FinalURL != "" && p.FinalURL != p.URL {
			t.Errorf("ListPages returned followed redirect: url=%s final_url=%s", p.URL, p.FinalURL)
		}
	}
}

func TestPageRankTop_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)
	setupRedirectTestData(t, s)

	result, err := s.PageRankTop(ctx, redirectTestSessionID, 50, 0, "")
	if err != nil {
		t.Fatalf("PageRankTop: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("PageRankTop total: expected 3, got %d", result.Total)
	}
	if len(result.Pages) != 3 {
		t.Errorf("PageRankTop pages: expected 3, got %d", len(result.Pages))
	}
	for _, p := range result.Pages {
		if p.URL == "https://example.com/old1" || p.URL == "https://example.com/old2" {
			t.Errorf("PageRankTop returned followed redirect: %s", p.URL)
		}
	}
}

func TestPageRankDistribution_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)
	setupRedirectTestData(t, s)

	result, err := s.PageRankDistribution(ctx, redirectTestSessionID, 20)
	if err != nil {
		t.Fatalf("PageRankDistribution: %v", err)
	}
	if result.TotalWithPR != 3 {
		t.Errorf("PageRankDistribution TotalWithPR: expected 3, got %d", result.TotalWithPR)
	}
}

func TestSessionStats_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)

	sess := &CrawlSession{
		ID:        redirectTestSessionID,
		StartedAt: time.Now().Add(-1 * time.Hour),
		Status:    "finished",
	}
	if err := s.InsertSession(ctx, sess); err != nil {
		t.Fatalf("inserting session: %v", err)
	}
	setupRedirectTestData(t, s)

	stats, err := s.SessionStats(ctx, redirectTestSessionID)
	if err != nil {
		t.Fatalf("SessionStats: %v", err)
	}
	if stats.TotalPages != 3 {
		t.Errorf("SessionStats TotalPages: expected 3, got %d", stats.TotalPages)
	}
}

func TestSessionAudit_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)
	setupRedirectTestData(t, s)

	audit, err := s.SessionAudit(ctx, redirectTestSessionID)
	if err != nil {
		t.Fatalf("SessionAudit: %v", err)
	}
	if audit.Content.Total != 3 {
		t.Errorf("SessionAudit content.Total: expected 3, got %d", audit.Content.Total)
	}
	if audit.Content.HTMLPages != 3 {
		t.Errorf("SessionAudit content.HTMLPages: expected 3, got %d", audit.Content.HTMLPages)
	}
}

func TestComparePages_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sessionA := "22222222-3333-4444-5555-666666666666"
	sessionB := "33333333-4444-5555-6666-777777777777"
	t.Cleanup(func() {
		cleanupRedirectTestSession(t, s, sessionA)
		cleanupRedirectTestSession(t, s, sessionB)
	})
	cleanupRedirectTestSession(t, s, sessionA)
	cleanupRedirectTestSession(t, s, sessionB)

	now := time.Now()

	pagesA := []PageRow{
		{CrawlSessionID: sessionA, URL: "https://example.com/page1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "A Page 1", PageRank: 10.0, CrawledAt: now},
		{CrawlSessionID: sessionA, URL: "https://example.com/page2", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "A Page 2", PageRank: 8.0, CrawledAt: now},
		{CrawlSessionID: sessionA, URL: "https://example.com/old-a", FinalURL: "https://example.com/page1", StatusCode: 200, ContentType: "text/html", Title: "A Page 1", PageRank: 3.0, CrawledAt: now},
	}
	insertTestPages(t, s, sessionA, pagesA)

	pagesB := []PageRow{
		{CrawlSessionID: sessionB, URL: "https://example.com/page1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "B Page 1 Updated", PageRank: 12.0, CrawledAt: now},
		{CrawlSessionID: sessionB, URL: "https://example.com/page3", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "B Page 3", PageRank: 5.0, CrawledAt: now},
		{CrawlSessionID: sessionB, URL: "https://example.com/old-b", FinalURL: "https://example.com/page3", StatusCode: 200, ContentType: "text/html", Title: "B Page 3", PageRank: 2.0, CrawledAt: now},
	}
	insertTestPages(t, s, sessionB, pagesB)

	result, err := s.ComparePages(ctx, sessionA, sessionB, "added", 100, 0)
	if err != nil {
		t.Fatalf("ComparePages added: %v", err)
	}
	if result.TotalAdded != 1 {
		t.Errorf("ComparePages TotalAdded: expected 1, got %d", result.TotalAdded)
	}
	for _, p := range result.Pages {
		if p.URL == "https://example.com/old-b" || p.URL == "https://example.com/old-a" {
			t.Errorf("ComparePages returned followed redirect in added: %s", p.URL)
		}
	}

	resultR, err := s.ComparePages(ctx, sessionA, sessionB, "removed", 100, 0)
	if err != nil {
		t.Fatalf("ComparePages removed: %v", err)
	}
	if resultR.TotalRemoved != 1 {
		t.Errorf("ComparePages TotalRemoved: expected 1, got %d", resultR.TotalRemoved)
	}
	for _, p := range resultR.Pages {
		if p.URL == "https://example.com/old-a" {
			t.Errorf("ComparePages returned followed redirect in removed: %s", p.URL)
		}
	}
}

func TestSitemapCoverage_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)
	setupRedirectTestData(t, s)

	sitemapURLs := []SitemapURLRow{
		{CrawlSessionID: redirectTestSessionID, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/page1"},
		{CrawlSessionID: redirectTestSessionID, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/page2"},
		{CrawlSessionID: redirectTestSessionID, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/old1"},
		{CrawlSessionID: redirectTestSessionID, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/not-crawled"},
	}
	if err := s.InsertSitemapURLs(ctx, sitemapURLs); err != nil {
		t.Fatalf("inserting sitemap URLs: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	inBoth, err := s.GetSitemapCoverageURLs(ctx, redirectTestSessionID, "in_both", 100, 0)
	if err != nil {
		t.Fatalf("GetSitemapCoverageURLs in_both: %v", err)
	}
	if len(inBoth) != 2 {
		t.Errorf("in_both: expected 2 URLs, got %d", len(inBoth))
		for _, u := range inBoth {
			t.Logf("  loc=%s", u.Loc)
		}
	}
	for _, u := range inBoth {
		if u.Loc == "https://example.com/old1" {
			t.Errorf("in_both returned followed redirect URL: %s", u.Loc)
		}
	}

	sitemapOnly, err := s.GetSitemapCoverageURLs(ctx, redirectTestSessionID, "sitemap_only", 100, 0)
	if err != nil {
		t.Fatalf("GetSitemapCoverageURLs sitemap_only: %v", err)
	}
	if len(sitemapOnly) != 2 {
		t.Errorf("sitemap_only: expected 2 URLs, got %d", len(sitemapOnly))
		for _, u := range sitemapOnly {
			t.Logf("  loc=%s", u.Loc)
		}
	}
}

// ===========================================================================
// Edge case tests — each test injects pages that mimic a specific redirect
// pattern observed in production crawls. Comments note real-world likelihood:
//   - no tag     = common redirect pattern
//   - [unlikely] = theoretically possible but normalizer prevents it
//   - [impossible] = cannot happen with our normalizer pipeline
// ===========================================================================

const edgeCaseSessionID = "44444444-5555-6666-7777-888888888888"

// Trailing slash add/remove: Apache mod_dir, Nginx try_files, Next.js — very common.
func TestEdgeCase_TrailingSlashIsExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// Apache mod_dir adds trailing slash → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/products", FinalURL: "https://example.com/products/", StatusCode: 200, ContentType: "text/html", Title: "Products", PageRank: 5.0, CrawledAt: now},
		// Next.js strips trailing slash → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/about/", FinalURL: "https://example.com/about", StatusCode: 200, ContentType: "text/html", Title: "About", PageRank: 3.0, CrawledAt: now},
		// Normal page, trailing slash, exact match → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/blog/", FinalURL: "https://example.com/blog/", StatusCode: 200, ContentType: "text/html", Title: "Blog", PageRank: 7.0, CrawledAt: now},
		// Normal page, no redirect → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/contact", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Contact", PageRank: 2.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	// Only /blog/ and /contact should be included
	if count != 2 {
		t.Errorf("trailing slash edge case: expected 2 pages, got %d", count)
	}

	listed, err := s.ListPages(ctx, edgeCaseSessionID, 100, 0, nil, nil)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	for _, p := range listed {
		if p.URL == "https://example.com/products" || p.URL == "https://example.com/about/" {
			t.Errorf("trailing slash redirect should be excluded: url=%s final_url=%s", p.URL, p.FinalURL)
		}
	}
}

// http→https upgrade: near-universal since 2018. Seeds may start as http://.
func TestEdgeCase_HTTPToHTTPSIsExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// http→https: near-universal since HSTS adoption → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "http://example.com/page", FinalURL: "https://example.com/page", StatusCode: 200, ContentType: "text/html", Title: "Page", PageRank: 5.0, CrawledAt: now},
		// [unlikely] https→http downgrade: blocked by most clients → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://secure.example.com/api", FinalURL: "http://secure.example.com/api", StatusCode: 200, ContentType: "text/html", Title: "API", PageRank: 3.0, CrawledAt: now},
		// Normal https page → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/home", FinalURL: "https://example.com/home", StatusCode: 200, ContentType: "text/html", Title: "Home", PageRank: 10.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 1 {
		t.Errorf("protocol redirect edge case: expected 1 page, got %d", count)
	}
}

// WWW canonicalization: daily SEO audit finding, both directions (add/remove www).
func TestEdgeCase_WWWCanonicalizationIsExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// non-www → www: daily audit finding → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/page", FinalURL: "https://www.example.com/page", StatusCode: 200, ContentType: "text/html", Title: "Page", PageRank: 5.0, CrawledAt: now},
		// www → non-www: reverse canonicalization → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://www.example.com/other", FinalURL: "https://example.com/other", StatusCode: 200, ContentType: "text/html", Title: "Other", PageRank: 3.0, CrawledAt: now},
		// Normal www page, exact match → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://www.example.com/ok", FinalURL: "https://www.example.com/ok", StatusCode: 200, ContentType: "text/html", Title: "OK", PageRank: 8.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 1 {
		t.Errorf("www canonicalization edge case: expected 1 page, got %d", count)
	}
}

// Query param changes: server adds lang/session param, strips UTM, or reorders.
// [unlikely] UTM strip and reorder — normalizer already handles both before crawl.
func TestEdgeCase_QueryParamChangesExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// Server adds lang/session param on redirect → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/search", FinalURL: "https://example.com/search?lang=en", StatusCode: 200, ContentType: "text/html", Title: "Search", PageRank: 5.0, CrawledAt: now},
		// [unlikely] UTM stripped by server — normalizer already removes them → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/page?utm_source=google&utm_medium=cpc", FinalURL: "https://example.com/page", StatusCode: 200, ContentType: "text/html", Title: "Page", PageRank: 3.0, CrawledAt: now},
		// [unlikely] Query reordered — normalizer sorts params (FlagSortQuery) → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/api?a=1&b=2", FinalURL: "https://example.com/api?b=2&a=1", StatusCode: 200, ContentType: "text/html", Title: "API", PageRank: 2.0, CrawledAt: now},
		// Exact match with query params → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/results?q=test", FinalURL: "https://example.com/results?q=test", StatusCode: 200, ContentType: "text/html", Title: "Results", PageRank: 7.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 1 {
		t.Errorf("query param edge case: expected 1 page, got %d", count)
	}
}

// Case sensitivity: path case normalized by server (IIS, some CMS).
// [unlikely] Domain case — normalizer lowercases host, so url never has uppercase domain.
func TestEdgeCase_CaseSensitivityExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// [unlikely] Domain case — normalizer lowercases host → excluded (defence in depth)
		{CrawlSessionID: edgeCaseSessionID, URL: "https://Example.COM/page", FinalURL: "https://example.com/page", StatusCode: 200, ContentType: "text/html", Title: "Page", PageRank: 5.0, CrawledAt: now},
		// Path case normalized by server (IIS, some CMS) — path not lowercased → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/Page", FinalURL: "https://example.com/page", StatusCode: 200, ContentType: "text/html", Title: "Page Lower", PageRank: 3.0, CrawledAt: now},
		// Consistent case → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/normal", FinalURL: "https://example.com/normal", StatusCode: 200, ContentType: "text/html", Title: "Normal", PageRank: 8.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	// ClickHouse string comparison is case-sensitive, so case differences are excluded
	if count != 1 {
		t.Errorf("case sensitivity edge case: expected 1 page, got %d", count)
	}
}

// [unlikely] Percent-encoding mismatch: both Go net/http and normalizer encode
// consistently. Possible only with exotic servers returning decoded UTF-8 in Location.
func TestEdgeCase_URLEncodingDifferencesExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// [unlikely] %20 vs space — both sides encode consistently. Exotic servers only → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/my%20page", FinalURL: "https://example.com/my page", StatusCode: 200, ContentType: "text/html", Title: "My Page", PageRank: 5.0, CrawledAt: now},
		// [unlikely] UTF-8 encoding mismatch — same issue → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/caf%C3%A9", FinalURL: "https://example.com/café", StatusCode: 200, ContentType: "text/html", Title: "Café", PageRank: 3.0, CrawledAt: now},
		// No redirect → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/simple", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Simple", PageRank: 8.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 1 {
		t.Errorf("URL encoding edge case: expected 1 page, got %d", count)
	}
}

// [impossible] Path normalization: double slash, dot segments, default port.
// Normalizer applies FlagRemoveDuplicateSlashes, FlagRemoveDefaultPort;
// Go url.Parse resolves ./ and ../. These URLs can never reach the DB.
func TestEdgeCase_PathNormalizationExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// [impossible] Double slash — FlagRemoveDuplicateSlashes → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com//page", FinalURL: "https://example.com/page", StatusCode: 200, ContentType: "text/html", Title: "Page", PageRank: 5.0, CrawledAt: now},
		// [impossible] Dot segment ./ — resolved by url.Parse → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/a/./b", FinalURL: "https://example.com/a/b", StatusCode: 200, ContentType: "text/html", Title: "AB", PageRank: 3.0, CrawledAt: now},
		// [impossible] Dot-dot segment ../ — resolved by url.Parse → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/a/x/../b", FinalURL: "https://example.com/a/b", StatusCode: 200, ContentType: "text/html", Title: "AB2", PageRank: 2.0, CrawledAt: now},
		// [impossible] Explicit :443 — FlagRemoveDefaultPort; Go omits it too → excluded
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/page2", FinalURL: "https://example.com:443/page2", StatusCode: 200, ContentType: "text/html", Title: "Page2", PageRank: 1.0, CrawledAt: now},
		// Normal page → included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/clean", FinalURL: "https://example.com/clean", StatusCode: 200, ContentType: "text/html", Title: "Clean", PageRank: 8.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 1 {
		t.Errorf("path normalization edge case: expected 1 page, got %d", count)
	}
}

// Cross-domain and subdomain redirects: domain migration, subdomain consolidation.
// Common in enterprise SEO (brand mergers, blog.example.com → example.com/blog).
func TestEdgeCase_CrossDomainRedirectExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// Cross-domain redirect
		{CrawlSessionID: edgeCaseSessionID, URL: "https://old-domain.com/page", FinalURL: "https://new-domain.com/page", StatusCode: 200, ContentType: "text/html", Title: "Page", PageRank: 5.0, CrawledAt: now},
		// Subdomain redirect
		{CrawlSessionID: edgeCaseSessionID, URL: "https://blog.example.com/post", FinalURL: "https://example.com/blog/post", StatusCode: 200, ContentType: "text/html", Title: "Post", PageRank: 3.0, CrawledAt: now},
		// Normal page
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/home", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Home", PageRank: 8.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 1 {
		t.Errorf("cross-domain redirect edge case: expected 1 page, got %d", count)
	}
}

// Combined multi-hop: the most realistic production scenario.
// http + no-www + no-slash → https + www + slash. Stored as first→final.
func TestEdgeCase_CombinedRedirectExcluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// Triple combo: http + non-www + no trailing slash -> https + www + trailing slash
		{CrawlSessionID: edgeCaseSessionID, URL: "http://example.com/products", FinalURL: "https://www.example.com/products/", StatusCode: 200, ContentType: "text/html", Title: "Products", PageRank: 5.0, CrawledAt: now},
		// Chain redirect stored as first->final
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/step1", FinalURL: "https://example.com/step3", StatusCode: 200, ContentType: "text/html", Title: "Step 3", PageRank: 3.0, CrawledAt: now},
		// Normal page
		{CrawlSessionID: edgeCaseSessionID, URL: "https://www.example.com/ok", FinalURL: "https://www.example.com/ok", StatusCode: 200, ContentType: "text/html", Title: "OK", PageRank: 8.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 1 {
		t.Errorf("combined redirect edge case: expected 1 page, got %d", count)
	}
}

// ===========================================================================
// Status code interaction — the filter is status-agnostic: it only checks
// final_url vs url. 301/302 with final_url set are excluded; 404/500/0 with
// empty final_url are included (useful for error auditing).
// ===========================================================================

func TestEdgeCase_StatusCodeInteraction(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// True 301 redirect with final_url set — excluded by filter (final_url != url)
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/old-301", FinalURL: "https://example.com/new", StatusCode: 301, ContentType: "", Title: "", PageRank: 0, CrawledAt: now},
		// True 302 temporary redirect
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/temp-302", FinalURL: "https://example.com/new", StatusCode: 302, ContentType: "", Title: "", PageRank: 0, CrawledAt: now},
		// 404 page with empty final_url — included by filter (final_url = '')
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/not-found", FinalURL: "", StatusCode: 404, ContentType: "text/html", Title: "Not Found", PageRank: 0, CrawledAt: now},
		// 500 error page with empty final_url — included by filter
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/error", FinalURL: "", StatusCode: 500, ContentType: "text/html", Title: "Error", PageRank: 0, CrawledAt: now},
		// Followed redirect with 200 status (the main case)
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/redirect-200", FinalURL: "https://example.com/target", StatusCode: 200, ContentType: "text/html", Title: "Target", PageRank: 5.0, CrawledAt: now},
		// Normal 200 page
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/normal", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Normal", PageRank: 10.0, CrawledAt: now},
		// Status 0 (fetch error) with empty final_url — included
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/timeout", FinalURL: "", StatusCode: 0, ContentType: "", Title: "", PageRank: 0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	// Included: /not-found (404, empty final), /error (500, empty final),
	//           /normal (200, empty final), /timeout (0, empty final)
	// Excluded: /old-301 (final != url), /temp-302 (final != url), /redirect-200 (final != url)
	if count != 4 {
		t.Errorf("status code interaction: expected 4 pages, got %d", count)
		listed, _ := s.ListPages(ctx, edgeCaseSessionID, 100, 0, nil, nil)
		for _, p := range listed {
			t.Logf("  url=%s final_url=%q status=%d", p.URL, p.FinalURL, p.StatusCode)
		}
	}
}

// ===========================================================================
// Self-redirect / identity — server 302→same URL (e.g. cookie check).
// final_url == url → same content, must be kept.
// ===========================================================================

func TestEdgeCase_SelfRedirectIncluded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// Self-redirect: final_url == url (browser followed redirect back to same URL)
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/self", FinalURL: "https://example.com/self", StatusCode: 200, ContentType: "text/html", Title: "Self", PageRank: 5.0, CrawledAt: now},
		// Empty final_url
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/empty", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Empty", PageRank: 3.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	// Both should be included
	if count != 2 {
		t.Errorf("self-redirect edge case: expected 2 pages, got %d", count)
	}
}

// ===========================================================================
// PageRankTreemap — directory-level PR aggregation must not inflate page counts
// with followed redirects (they share content with their target).
// ===========================================================================

func TestPageRankTreemap_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, redirectTestSessionID) })
	cleanupRedirectTestSession(t, s, redirectTestSessionID)
	setupRedirectTestData(t, s)

	entries, err := s.PageRankTreemap(ctx, redirectTestSessionID, 2, 1)
	if err != nil {
		t.Fatalf("PageRankTreemap: %v", err)
	}

	// Sum page counts across all entries — should be 3 (only normal pages)
	var totalPages uint64
	for _, e := range entries {
		totalPages += e.PageCount
	}
	if totalPages != 3 {
		t.Errorf("PageRankTreemap total pages: expected 3, got %d", totalPages)
	}
}

// ===========================================================================
// SessionAudit deep checks — a followed redirect duplicating "Title A" would
// inflate title duplicate counts, skew lang/schema distributions, and corrupt
// content type stats. Each sub-assertion targets a specific audit section.
// ===========================================================================

func TestSessionAudit_DeepExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sid := "55555555-6666-7777-8888-999999999999"
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, sid) })
	cleanupRedirectTestSession(t, s, sid)

	now := time.Now()
	pages := []PageRow{
		// 2 normal HTML pages
		{CrawlSessionID: sid, URL: "https://example.com/p1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Title A", PageRank: 10.0, Depth: 1, WordCount: 500, InternalLinksOut: 5, ExternalLinksOut: 2, IsIndexable: true, Lang: "en", SchemaTypes: []string{"Article"}, CrawledAt: now},
		{CrawlSessionID: sid, URL: "https://example.com/p2", FinalURL: "https://example.com/p2", StatusCode: 200, ContentType: "text/html", Title: "Title B", PageRank: 8.0, Depth: 2, WordCount: 300, InternalLinksOut: 3, ExternalLinksOut: 0, IsIndexable: true, Lang: "fr", SchemaTypes: []string{"WebPage"}, CrawledAt: now},
		// 1 followed redirect — duplicates Title A, same lang
		{CrawlSessionID: sid, URL: "https://example.com/old", FinalURL: "https://example.com/p1", StatusCode: 200, ContentType: "text/html", Title: "Title A", PageRank: 2.0, Depth: 1, WordCount: 500, InternalLinksOut: 5, ExternalLinksOut: 2, IsIndexable: true, Lang: "en", SchemaTypes: []string{"Article"}, CrawledAt: now},
	}
	insertTestPages(t, s, sid, pages)

	audit, err := s.SessionAudit(ctx, sid)
	if err != nil {
		t.Fatalf("SessionAudit: %v", err)
	}

	// Content: total should be 2, not 3
	if audit.Content.Total != 2 {
		t.Errorf("content.Total: expected 2, got %d", audit.Content.Total)
	}

	// Title duplicates: "Title A" appears once (not duplicated without the redirect)
	if audit.Content.TitleDuplicates != 0 {
		t.Errorf("content.TitleDuplicates: expected 0 (no dups among 2 normal pages), got %d", audit.Content.TitleDuplicates)
	}

	// Technical: indexable count
	if audit.Technical.Indexable != 2 {
		t.Errorf("technical.Indexable: expected 2, got %d", audit.Technical.Indexable)
	}

	// Content types: only 2 pages with text/html
	var htmlCount uint64
	for _, ct := range audit.Technical.ContentTypes {
		if ct.ContentType == "text/html" {
			htmlCount = ct.Count
		}
	}
	if htmlCount != 2 {
		t.Errorf("content type text/html count: expected 2, got %d", htmlCount)
	}

	// Link distribution: 2 pages, not 3
	// PagesNoExternal should be 1 (p2 has 0 external)
	if audit.Links.PagesNoExternal != 1 {
		t.Errorf("links.PagesNoExternal: expected 1, got %d", audit.Links.PagesNoExternal)
	}

	// Directories: total pages across all dirs should be 2
	var dirTotal uint64
	for _, d := range audit.Structure.Directories {
		dirTotal += d.Count
	}
	if dirTotal != 2 {
		t.Errorf("structure directories total: expected 2, got %d", dirTotal)
	}

	// International: pages with lang should be 2
	if audit.International.PagesWithLang != 2 {
		t.Errorf("international.PagesWithLang: expected 2, got %d", audit.International.PagesWithLang)
	}

	// Lang distribution: en=1, fr=1 (not en=2 which would happen with the redirect)
	langMap := make(map[string]uint64)
	for _, lc := range audit.International.LangDistribution {
		langMap[lc.Lang] = lc.Count
	}
	if langMap["en"] != 1 {
		t.Errorf("lang distribution en: expected 1, got %d", langMap["en"])
	}
	if langMap["fr"] != 1 {
		t.Errorf("lang distribution fr: expected 1, got %d", langMap["fr"])
	}

	// Schema distribution: Article=1, WebPage=1
	schemaMap := make(map[string]uint64)
	for _, sc := range audit.International.SchemaDistribution {
		schemaMap[sc.SchemaType] = sc.Count
	}
	if schemaMap["Article"] != 1 {
		t.Errorf("schema distribution Article: expected 1, got %d", schemaMap["Article"])
	}
	if schemaMap["WebPage"] != 1 {
		t.Errorf("schema distribution WebPage: expected 1, got %d", schemaMap["WebPage"])
	}
}

// ===========================================================================
// ComparePages "changed" — a redirect present in both sessions with different
// titles would appear as a false "changed" diff without the filter.
// ===========================================================================

func TestComparePages_ChangedExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sessionA := "66666666-7777-8888-9999-aaaaaaaaaaaa"
	sessionB := "77777777-8888-9999-aaaa-bbbbbbbbbbbb"
	t.Cleanup(func() {
		cleanupRedirectTestSession(t, s, sessionA)
		cleanupRedirectTestSession(t, s, sessionB)
	})
	cleanupRedirectTestSession(t, s, sessionA)
	cleanupRedirectTestSession(t, s, sessionB)

	now := time.Now()

	// Session A
	pagesA := []PageRow{
		{CrawlSessionID: sessionA, URL: "https://example.com/page1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Old Title", PageRank: 10.0, CrawledAt: now},
		// Followed redirect in A
		{CrawlSessionID: sessionA, URL: "https://example.com/redir", FinalURL: "https://example.com/page1", StatusCode: 200, ContentType: "text/html", Title: "Old Title", PageRank: 3.0, CrawledAt: now},
	}
	insertTestPages(t, s, sessionA, pagesA)

	// Session B — page1 title changed, redirect also exists with different title
	pagesB := []PageRow{
		{CrawlSessionID: sessionB, URL: "https://example.com/page1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "New Title", PageRank: 12.0, CrawledAt: now},
		// Same redirect URL exists in B too but with different data
		{CrawlSessionID: sessionB, URL: "https://example.com/redir", FinalURL: "https://example.com/page1", StatusCode: 200, ContentType: "text/html", Title: "New Title", PageRank: 4.0, CrawledAt: now},
	}
	insertTestPages(t, s, sessionB, pagesB)

	result, err := s.ComparePages(ctx, sessionA, sessionB, "changed", 100, 0)
	if err != nil {
		t.Fatalf("ComparePages changed: %v", err)
	}

	// Only page1 should appear as changed, not the redirect
	if result.TotalChanged != 1 {
		t.Errorf("ComparePages TotalChanged: expected 1, got %d", result.TotalChanged)
	}
	for _, p := range result.Pages {
		if p.URL == "https://example.com/redir" {
			t.Errorf("ComparePages changed includes followed redirect: %s", p.URL)
		}
	}
}

// ===========================================================================
// Zero-data / boundary conditions — the filter must not cause SQL errors
// when there are no pages, or when ALL pages are followed redirects.
// ===========================================================================

// Empty session: no pages → all queries return 0 without SQL errors.
func TestEdgeCase_EmptySession(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	emptySessionID := "88888888-9999-aaaa-bbbb-cccccccccccc"
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, emptySessionID) })
	cleanupRedirectTestSession(t, s, emptySessionID)

	// No pages inserted — all queries should return 0/empty without errors
	count, err := s.CountPages(ctx, emptySessionID)
	if err != nil {
		t.Fatalf("CountPages empty: %v", err)
	}
	if count != 0 {
		t.Errorf("CountPages empty: expected 0, got %d", count)
	}

	pages, err := s.ListPages(ctx, emptySessionID, 100, 0, nil, nil)
	if err != nil {
		t.Fatalf("ListPages empty: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("ListPages empty: expected 0, got %d", len(pages))
	}

	prResult, err := s.PageRankTop(ctx, emptySessionID, 50, 0, "")
	if err != nil {
		t.Fatalf("PageRankTop empty: %v", err)
	}
	if prResult.Total != 0 {
		t.Errorf("PageRankTop empty: expected 0, got %d", prResult.Total)
	}

	dist, err := s.PageRankDistribution(ctx, emptySessionID, 20)
	if err != nil {
		t.Fatalf("PageRankDistribution empty: %v", err)
	}
	if dist.TotalWithPR != 0 {
		t.Errorf("PageRankDistribution empty: expected 0, got %d", dist.TotalWithPR)
	}
}

// All-redirects session: every page is a followed redirect → count = 0.
// Realistic when crawling a legacy domain fully redirected to a new one.
func TestEdgeCase_AllRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, edgeCaseSessionID) })
	cleanupRedirectTestSession(t, s, edgeCaseSessionID)

	now := time.Now()
	pages := []PageRow{
		// All pages are followed redirects — count should be 0
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/old1", FinalURL: "https://example.com/new1", StatusCode: 200, ContentType: "text/html", Title: "New 1", PageRank: 5.0, CrawledAt: now},
		{CrawlSessionID: edgeCaseSessionID, URL: "https://example.com/old2", FinalURL: "https://example.com/new2", StatusCode: 200, ContentType: "text/html", Title: "New 2", PageRank: 3.0, CrawledAt: now},
		{CrawlSessionID: edgeCaseSessionID, URL: "http://example.com/http-old", FinalURL: "https://example.com/https-new", StatusCode: 200, ContentType: "text/html", Title: "HTTPS", PageRank: 8.0, CrawledAt: now},
	}
	insertTestPages(t, s, edgeCaseSessionID, pages)

	count, err := s.CountPages(ctx, edgeCaseSessionID)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if count != 0 {
		t.Errorf("all-redirects session: expected 0 pages, got %d", count)
	}

	listed, err := s.ListPages(ctx, edgeCaseSessionID, 100, 0, nil, nil)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("all-redirects session: expected 0 listed, got %d", len(listed))
	}
}

// ===========================================================================
// Depth distribution — a redirect at depth 1 would inflate the depth-1 bucket,
// making the crawl appear shallower than it really is.
// ===========================================================================

func TestSessionStats_DepthDistribution_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sid := "99999999-aaaa-bbbb-cccc-dddddddddddd"
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, sid) })
	cleanupRedirectTestSession(t, s, sid)

	sess := &CrawlSession{
		ID:        sid,
		StartedAt: time.Now().Add(-1 * time.Hour),
		Status:    "finished",
	}
	if err := s.InsertSession(ctx, sess); err != nil {
		t.Fatalf("inserting session: %v", err)
	}

	now := time.Now()
	pages := []PageRow{
		// Depth 1: 2 normal pages
		{CrawlSessionID: sid, URL: "https://example.com/d1-a", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "D1 A", Depth: 1, PageRank: 10.0, CrawledAt: now},
		{CrawlSessionID: sid, URL: "https://example.com/d1-b", FinalURL: "https://example.com/d1-b", StatusCode: 200, ContentType: "text/html", Title: "D1 B", Depth: 1, PageRank: 8.0, CrawledAt: now},
		// Depth 1: followed redirect — should NOT count
		{CrawlSessionID: sid, URL: "https://example.com/d1-redirect", FinalURL: "https://example.com/d1-a", StatusCode: 200, ContentType: "text/html", Title: "D1 A", Depth: 1, PageRank: 3.0, CrawledAt: now},
		// Depth 2: 1 normal page
		{CrawlSessionID: sid, URL: "https://example.com/d2-a", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "D2 A", Depth: 2, PageRank: 5.0, CrawledAt: now},
	}
	insertTestPages(t, s, sid, pages)

	stats, err := s.SessionStats(ctx, sid)
	if err != nil {
		t.Fatalf("SessionStats: %v", err)
	}

	if stats.TotalPages != 3 {
		t.Errorf("TotalPages: expected 3, got %d", stats.TotalPages)
	}

	// Depth distribution: depth 1 = 2 pages, depth 2 = 1 page
	if stats.DepthDistribution[1] != 2 {
		t.Errorf("DepthDistribution[1]: expected 2, got %d", stats.DepthDistribution[1])
	}
	if stats.DepthDistribution[2] != 1 {
		t.Errorf("DepthDistribution[2]: expected 1, got %d", stats.DepthDistribution[2])
	}

	// TopPageRank: should only contain 3 entries, not 4
	if len(stats.TopPageRank) != 3 {
		t.Errorf("TopPageRank: expected 3 entries, got %d", len(stats.TopPageRank))
		for _, e := range stats.TopPageRank {
			t.Logf("  url=%s pr=%.2f", e.URL, e.PageRank)
		}
	}
	for _, e := range stats.TopPageRank {
		if e.URL == "https://example.com/d1-redirect" {
			t.Errorf("TopPageRank includes followed redirect: %s", e.URL)
		}
	}
}

// ===========================================================================
// Orphan pages — a followed redirect with no incoming links is not a real
// orphan; it's just an alias. Counting it would inflate the orphan metric.
// ===========================================================================

func TestSessionAudit_OrphanPages_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sid := "aaaaaaaa-bbbb-cccc-dddd-111111111111"
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, sid) })
	cleanupRedirectTestSession(t, s, sid)

	now := time.Now()

	// Insert pages: 1 normal (linked to), 1 normal orphan, 1 followed redirect (orphan)
	pages := []PageRow{
		{CrawlSessionID: sid, URL: "https://example.com/linked", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Linked", PageRank: 10.0, CrawledAt: now},
		{CrawlSessionID: sid, URL: "https://example.com/orphan", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Orphan", PageRank: 5.0, CrawledAt: now},
		// Followed redirect — should not count as orphan
		{CrawlSessionID: sid, URL: "https://example.com/redir-orphan", FinalURL: "https://example.com/linked", StatusCode: 200, ContentType: "text/html", Title: "Linked", PageRank: 2.0, CrawledAt: now},
	}
	insertTestPages(t, s, sid, pages)

	// Insert a link pointing to /linked
	links := []LinkRow{
		{CrawlSessionID: sid, SourceURL: "https://example.com/orphan", TargetURL: "https://example.com/linked", IsInternal: true, CrawledAt: now},
	}
	if err := s.InsertLinks(ctx, links); err != nil {
		t.Fatalf("inserting links: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	audit, err := s.SessionAudit(ctx, sid)
	if err != nil {
		t.Fatalf("SessionAudit: %v", err)
	}

	// Only /orphan should be an orphan (no internal link pointing to it, and it's not a redirect)
	// /linked has an incoming link, /redir-orphan is a redirect and excluded from count
	if audit.Structure.OrphanPages != 1 {
		t.Errorf("orphan pages: expected 1, got %d", audit.Structure.OrphanPages)
	}
}

// ===========================================================================
// Noindex reasons — a followed redirect inheriting the target's noindex meta
// tag would double-count the noindex reason, misleading the SEO audit.
// ===========================================================================

func TestSessionAudit_NoindexReasons_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sid := "bbbbbbbb-cccc-dddd-eeee-222222222222"
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, sid) })
	cleanupRedirectTestSession(t, s, sid)

	now := time.Now()
	pages := []PageRow{
		// Normal noindex page
		{CrawlSessionID: sid, URL: "https://example.com/noindex", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "NoIndex", IsIndexable: false, IndexReason: "meta_noindex", PageRank: 5.0, CrawledAt: now},
		// Followed redirect that is also noindex — should NOT appear in noindex reasons
		{CrawlSessionID: sid, URL: "https://example.com/redir-noindex", FinalURL: "https://example.com/target", StatusCode: 200, ContentType: "text/html", Title: "Target", IsIndexable: false, IndexReason: "meta_noindex", PageRank: 2.0, CrawledAt: now},
		// Normal indexable page
		{CrawlSessionID: sid, URL: "https://example.com/ok", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "OK", IsIndexable: true, PageRank: 10.0, CrawledAt: now},
	}
	insertTestPages(t, s, sid, pages)

	audit, err := s.SessionAudit(ctx, sid)
	if err != nil {
		t.Fatalf("SessionAudit: %v", err)
	}

	// Only 1 noindex reason entry, count = 1 (not 2)
	var metaNoindexCount uint64
	for _, nr := range audit.Technical.NoindexReasons {
		if nr.Reason == "meta_noindex" {
			metaNoindexCount = nr.Count
		}
	}
	if metaNoindexCount != 1 {
		t.Errorf("noindex meta_noindex count: expected 1, got %d", metaNoindexCount)
	}
}

// ===========================================================================
// Sitemap coverage — /old in sitemap is a redirect alias, not a real crawled
// page. It must not count as "in_both", otherwise coverage % is inflated.
// ===========================================================================

func TestSessionAudit_SitemapCoverage_ExcludesRedirects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sid := "cccccccc-dddd-eeee-ffff-333333333333"
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, sid) })
	cleanupRedirectTestSession(t, s, sid)

	now := time.Now()
	pages := []PageRow{
		{CrawlSessionID: sid, URL: "https://example.com/p1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "P1", PageRank: 10.0, CrawledAt: now},
		{CrawlSessionID: sid, URL: "https://example.com/p2", FinalURL: "https://example.com/p2", StatusCode: 200, ContentType: "text/html", Title: "P2", PageRank: 8.0, CrawledAt: now},
		// Followed redirect
		{CrawlSessionID: sid, URL: "https://example.com/old", FinalURL: "https://example.com/p1", StatusCode: 200, ContentType: "text/html", Title: "P1", PageRank: 2.0, CrawledAt: now},
	}
	insertTestPages(t, s, sid, pages)

	sitemapURLs := []SitemapURLRow{
		{CrawlSessionID: sid, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/p1"},
		{CrawlSessionID: sid, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/p2"},
		{CrawlSessionID: sid, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/old"},      // redirect URL in sitemap
		{CrawlSessionID: sid, SitemapURL: "https://example.com/sitemap.xml", Loc: "https://example.com/external"},  // not crawled
	}
	if err := s.InsertSitemapURLs(ctx, sitemapURLs); err != nil {
		t.Fatalf("inserting sitemap URLs: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	audit, err := s.SessionAudit(ctx, sid)
	if err != nil {
		t.Fatalf("SessionAudit: %v", err)
	}

	// InBoth: only p1 and p2 match (old is a redirect, excluded from crawl page set)
	if audit.Sitemaps.InBoth != 2 {
		t.Errorf("sitemaps.InBoth: expected 2, got %d", audit.Sitemaps.InBoth)
	}
	// CrawledOnly: total non-redirect crawled (2) - in_both (2) = 0
	if audit.Sitemaps.CrawledOnly != 0 {
		t.Errorf("sitemaps.CrawledOnly: expected 0, got %d", audit.Sitemaps.CrawledOnly)
	}
	// SitemapOnly: total sitemap URLs (4) - in_both (2) = 2
	if audit.Sitemaps.SitemapOnly != 2 {
		t.Errorf("sitemaps.SitemapOnly: expected 2, got %d", audit.Sitemaps.SitemapOnly)
	}
}
