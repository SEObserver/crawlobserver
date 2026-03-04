package fetcher

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestTLSProfileChromeFetch verifies that the Chrome TLS profile can fetch
// real HTTPS pages without errors.
func TestTLSProfileChromeFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	profiles := []TLSProfile{TLSChrome, TLSFirefox}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			f := New("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
				15*time.Second, 10*1024*1024, DialOptions{}, profile)

			result := f.Fetch("https://www.google.com/", 0, "")
			if result.Error != "" {
				t.Fatalf("fetch failed with TLS profile %s: %s", profile, result.Error)
			}
			if result.StatusCode != 200 {
				t.Errorf("expected status 200, got %d", result.StatusCode)
			}
			if !result.IsHTML() {
				t.Errorf("expected HTML response, got %s", result.ContentType)
			}
		})
	}
}

// TestTLSProfileSitemapFetch verifies that sitemap fetching works correctly
// with a TLS profile applied to the HTTP client.
func TestTLSProfileSitemapFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	profiles := []struct {
		name    string
		profile TLSProfile
	}{
		{"default", ""},
		{"chrome", TLSChrome},
	}

	sitemapURL := "https://www.google.com/sitemap.xml"

	for _, tc := range profiles {
		t.Run(tc.name, func(t *testing.T) {
			ua := "Mozilla/5.0 (compatible; TestBot/1.0)"
			if tc.profile != "" {
				ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
			}

			f := New(ua, 15*time.Second, 10*1024*1024, DialOptions{}, tc.profile)
			entry := FetchSitemap(context.Background(), f.Client(),sitemapURL, ua)

			if entry.StatusCode == 0 {
				t.Fatalf("sitemap fetch failed (status 0) with profile %s — likely TLS/network error", tc.name)
			}
			t.Logf("profile=%s status=%d type=%s urls=%d sitemaps=%d",
				tc.name, entry.StatusCode, entry.Type, len(entry.URLs), len(entry.Sitemaps))
		})
	}
}

// TestTLSProfileCloudflare tests fetching through Cloudflare with TLS profiles.
// melty.fr is behind Cloudflare.
func TestTLSProfileCloudflare(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testURL := "https://www.melty.fr/sitemap_index.xml"

	profiles := []struct {
		name    string
		profile TLSProfile
		ua      string
	}{
		{"default-ua", "", "CrawlObserver/1.0"},
		{"chrome-ua-no-tls", "", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"},
		{"chrome-ua-chrome-tls", TLSChrome, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"},
	}

	for _, tc := range profiles {
		t.Run(tc.name, func(t *testing.T) {
			f := New(tc.ua, 15*time.Second, 10*1024*1024, DialOptions{}, tc.profile)
			entry := FetchSitemap(context.Background(), f.Client(),testURL, tc.ua)

			if entry.StatusCode == 0 {
				t.Errorf("sitemap fetch failed (status 0) with %s — likely TLS/network error", tc.name)
			} else if entry.StatusCode != 200 {
				t.Errorf("expected status 200, got %d with %s", entry.StatusCode, tc.name)
			}

			if entry.Type != "index" {
				t.Errorf("expected sitemap index, got type=%q with %s", entry.Type, tc.name)
			}

			if len(entry.Sitemaps) == 0 {
				t.Errorf("expected child sitemaps in index, got 0 with %s", tc.name)
			}

			t.Logf("%s: status=%d type=%s children=%d", tc.name, entry.StatusCode, entry.Type, len(entry.Sitemaps))
		})
	}
}

// TestTLSProfileHTTPClientReuse verifies that the HTTP client with TLS profile
// can handle multiple sequential requests (connection reuse).
func TestTLSProfileHTTPClientReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	f := New("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		15*time.Second, 10*1024*1024, DialOptions{}, TLSChrome)

	for i := 0; i < 3; i++ {
		req, err := http.NewRequest("GET", "https://www.google.com/robots.txt", nil)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		req.Header.Set("User-Agent", f.userAgent)

		resp, err := f.Client().Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
}

// TestTLSProfileDiscoverSitemaps tests the full sitemap discovery flow
// with TLS profile — this reproduces the production bug where Chrome TLS
// profile discovered 0 sitemap URLs while default discovered 45k+.
func TestTLSProfileDiscoverSitemaps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name    string
		profile TLSProfile
		ua      string
	}{
		{"default", "", "CrawlObserver/1.0"},
		{"chrome", TLSChrome, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := New(tc.ua, 15*time.Second, 10*1024*1024, DialOptions{}, tc.profile)

			entries := DiscoverSitemaps(context.Background(), f.Client(),tc.ua, []string{
				"https://www.melty.fr/sitemap.xml",
				"https://www.melty.fr/sitemap_index.xml",
			})

			totalURLs := 0
			for _, e := range entries {
				totalURLs += len(e.URLs)
				if e.StatusCode == 0 {
					t.Errorf("sitemap %s returned status 0 (fetch error)", e.URL)
				}
			}

			t.Logf("%s: discovered %d sitemaps, %d total URLs", tc.name, len(entries), totalURLs)

			if len(entries) < 3 {
				t.Errorf("expected at least 3 sitemaps (index + children), got %d", len(entries))
			}
		})
	}
}

// TestTLSProfileParallelFetch verifies TLS profile works with concurrent requests.
func TestTLSProfileParallelFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	f := New("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		15*time.Second, 10*1024*1024, DialOptions{}, TLSChrome)

	urls := []string{
		"https://www.google.com/",
		"https://www.google.com/robots.txt",
		"https://www.google.com/sitemap.xml",
	}

	type fetchResult struct {
		url    string
		status int
		err    string
	}

	results := make(chan fetchResult, len(urls))

	for _, u := range urls {
		go func(url string) {
			r := f.Fetch(url, 0, "")
			results <- fetchResult{url: url, status: r.StatusCode, err: r.Error}
		}(u)
	}

	for range urls {
		r := <-results
		if r.err != "" {
			t.Errorf("parallel fetch %s failed: %s", r.url, r.err)
		}
		if r.status != 200 {
			t.Errorf("parallel fetch %s: expected 200, got %d", r.url, r.status)
		}
	}
}

// TestFetchSitemapReturnsZeroOnError verifies that FetchSitemap returns
// status 0 when the HTTP request fails.
func TestFetchSitemapReturnsZeroOnError(t *testing.T) {
	f := New("TestBot", 2*time.Second, 10*1024*1024, DialOptions{}, "")

	entry := FetchSitemap(context.Background(), f.Client(),"https://this-domain-does-not-exist-12345.example.com/sitemap.xml", "TestBot")

	if entry.StatusCode != 0 {
		t.Errorf("expected status 0 for failed request, got %d", entry.StatusCode)
	}
	if entry.Type != "" {
		t.Errorf("expected empty type, got %s", entry.Type)
	}
}

// TestSitemapParsingWithXMLStylesheet tests that sitemaps with XML stylesheets
// (like melty.fr uses) parse correctly.
func TestSitemapParsingWithXMLStylesheet(t *testing.T) {
	content := []byte(`<?xml version="1.0" encoding="UTF-8"?><?xml-stylesheet type="text/xsl" href="//example.com/main-sitemap.xsl"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<sitemap>
		<loc>https://example.com/post-sitemap.xml</loc>
		<lastmod>2026-03-01T17:30:47+00:00</lastmod>
	</sitemap>
	<sitemap>
		<loc>https://example.com/post-sitemap2.xml</loc>
		<lastmod>2008-05-20T13:41:36+00:00</lastmod>
	</sitemap>
</sitemapindex>`)

	if !strings.Contains(string(content), "<sitemapindex") {
		t.Fatal("detection logic should find sitemapindex")
	}

	var idx xmlSitemapIndex
	if err := xml.Unmarshal(content, &idx); err != nil {
		t.Fatalf("failed to parse sitemap index: %v", err)
	}

	if len(idx.Sitemaps) != 2 {
		t.Errorf("expected 2 child sitemaps, got %d", len(idx.Sitemaps))
	}
	if idx.Sitemaps[0].Loc != "https://example.com/post-sitemap.xml" {
		t.Errorf("unexpected first sitemap loc: %s", idx.Sitemaps[0].Loc)
	}
}
