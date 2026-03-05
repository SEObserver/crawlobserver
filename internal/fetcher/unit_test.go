package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
	"github.com/temoto/robotstxt"
)

// ---------------------------------------------------------------------------
// clientHelloID
// ---------------------------------------------------------------------------

func TestClientHelloID(t *testing.T) {
	tests := []struct {
		profile TLSProfile
		wantID  utls.ClientHelloID
		wantErr bool
	}{
		{TLSChrome, utls.HelloChrome_Auto, false},
		{TLSEdge, utls.HelloChrome_Auto, false},
		{TLSFirefox, utls.HelloFirefox_Auto, false},
		{"safari", utls.ClientHelloID{}, true},
		{"", utls.ClientHelloID{}, true},
		{"CHROME", utls.ClientHelloID{}, true}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.profile), func(t *testing.T) {
			got, err := clientHelloID(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("clientHelloID(%q) error = %v, wantErr %v", tt.profile, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantID {
				t.Errorf("clientHelloID(%q) = %v, want %v", tt.profile, got, tt.wantID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// hasPort
// ---------------------------------------------------------------------------

func TestHasPort(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"example.com:443", true},
		{"example.com:80", true},
		{"example.com:8080", true},
		{"127.0.0.1:80", true},
		{"[::1]:443", true},
		{"example.com", false},
		{"example.com:", true},                     // SplitHostPort accepts empty port
		{"[::1]", false},                           // IPv6 without port
		{"", false},                                // empty string
		{"192.168.1.1", false},                     // bare IPv4
		{"sub.domain.example.com:9090", true},      // deep subdomain with port
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := hasPort(tt.host)
			if got != tt.want {
				t.Errorf("hasPort(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsHTML (method on FetchResult)
// ---------------------------------------------------------------------------

func TestFetchResultIsHTML(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"TEXT/HTML", true},
		{"application/xhtml+xml", true},
		{"APPLICATION/XHTML+XML; charset=utf-8", true},
		{"", false}, // IsHTML differs from isHTMLContentType: empty => false
		{"image/png", false},
		{"application/json", false},
		{"text/plain", false},
	}

	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			r := &FetchResult{ContentType: tt.ct}
			got := r.IsHTML()
			if got != tt.want {
				t.Errorf("FetchResult{ContentType: %q}.IsHTML() = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SitemapURLs on RobotsCache
// ---------------------------------------------------------------------------

func TestSitemapURLs_Empty(t *testing.T) {
	rc := &RobotsCache{
		cache: make(map[string]*RobotsCacheEntry),
	}
	urls := rc.SitemapURLs()
	if len(urls) != 0 {
		t.Errorf("expected 0 URLs from empty cache, got %d", len(urls))
	}
}

func TestSitemapURLs_FallbackPaths(t *testing.T) {
	// Populate cache with entries that have no sitemaps declared in robots.txt
	rc := &RobotsCache{
		cache: map[string]*RobotsCacheEntry{
			"https://example.com": {
				StatusCode: 200,
				parsed:     &robotstxt.RobotsData{},
			},
		},
	}

	urls := rc.SitemapURLs()

	// Should get the two fallback paths
	sort.Strings(urls)
	expected := []string{
		"https://example.com/sitemap.xml",
		"https://example.com/sitemap_index.xml",
	}
	sort.Strings(expected)

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
	}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("url[%d] = %q, want %q", i, u, expected[i])
		}
	}
}

func TestSitemapURLs_DeclaredAndFallback(t *testing.T) {
	// Create a real robots.txt with sitemaps declared
	robotsBody := []byte("User-agent: *\nDisallow: /private/\nSitemap: https://example.com/custom-sitemap.xml\n")
	parsed, err := robotstxt.FromBytes(robotsBody)
	if err != nil {
		t.Fatalf("failed to parse robots.txt: %v", err)
	}

	rc := &RobotsCache{
		cache: map[string]*RobotsCacheEntry{
			"https://example.com": {
				StatusCode: 200,
				Content:    string(robotsBody),
				parsed:     parsed,
			},
		},
	}

	urls := rc.SitemapURLs()
	sort.Strings(urls)

	// Should get the declared sitemap + fallbacks
	expected := []string{
		"https://example.com/custom-sitemap.xml",
		"https://example.com/sitemap.xml",
		"https://example.com/sitemap_index.xml",
	}
	sort.Strings(expected)

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
	}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("url[%d] = %q, want %q", i, u, expected[i])
		}
	}
}

func TestSitemapURLs_MultipleHosts(t *testing.T) {
	rc := &RobotsCache{
		cache: map[string]*RobotsCacheEntry{
			"https://a.com": {
				StatusCode: 200,
				parsed:     &robotstxt.RobotsData{},
			},
			"https://b.com": {
				StatusCode: 200,
				parsed:     &robotstxt.RobotsData{},
			},
		},
	}

	urls := rc.SitemapURLs()

	// Each host produces 2 fallback paths
	if len(urls) != 4 {
		t.Fatalf("expected 4 URLs, got %d: %v", len(urls), urls)
	}

	sort.Strings(urls)
	expected := []string{
		"https://a.com/sitemap.xml",
		"https://a.com/sitemap_index.xml",
		"https://b.com/sitemap.xml",
		"https://b.com/sitemap_index.xml",
	}
	sort.Strings(expected)

	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("url[%d] = %q, want %q", i, u, expected[i])
		}
	}
}

func TestSitemapURLs_Deduplication(t *testing.T) {
	// Declared sitemap overlaps with fallback
	robotsBody := []byte("User-agent: *\nSitemap: https://example.com/sitemap.xml\n")
	parsed, err := robotstxt.FromBytes(robotsBody)
	if err != nil {
		t.Fatalf("failed to parse robots.txt: %v", err)
	}

	rc := &RobotsCache{
		cache: map[string]*RobotsCacheEntry{
			"https://example.com": {
				StatusCode: 200,
				Content:    string(robotsBody),
				parsed:     parsed,
			},
		},
	}

	urls := rc.SitemapURLs()

	// sitemap.xml is declared AND a fallback -- should appear only once
	count := 0
	for _, u := range urls {
		if u == "https://example.com/sitemap.xml" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected sitemap.xml to appear once, appeared %d times in %v", count, urls)
	}
}

// ---------------------------------------------------------------------------
// FetchSitemap with httptest (urlset parsing)
// ---------------------------------------------------------------------------

func TestFetchSitemap_URLSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
    <lastmod>2026-01-01</lastmod>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
  </url>
</urlset>`)
	}))
	defer server.Close()

	entry := FetchSitemap(context.Background(), server.Client(), server.URL+"/sitemap.xml", "TestBot")

	if entry.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", entry.StatusCode)
	}
	if entry.Type != "urlset" {
		t.Errorf("expected type 'urlset', got %q", entry.Type)
	}
	if len(entry.URLs) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(entry.URLs))
	}
	if entry.URLs[0].Loc != "https://example.com/page1" {
		t.Errorf("url[0].Loc = %q", entry.URLs[0].Loc)
	}
	if entry.URLs[0].LastMod != "2026-01-01" {
		t.Errorf("url[0].LastMod = %q", entry.URLs[0].LastMod)
	}
	if entry.URLs[0].ChangeFreq != "weekly" {
		t.Errorf("url[0].ChangeFreq = %q", entry.URLs[0].ChangeFreq)
	}
	if entry.URLs[0].Priority != "0.8" {
		t.Errorf("url[0].Priority = %q", entry.URLs[0].Priority)
	}
	if entry.URLs[1].Loc != "https://example.com/page2" {
		t.Errorf("url[1].Loc = %q", entry.URLs[1].Loc)
	}
}

func TestFetchSitemap_Index(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://example.com/sitemap1.xml</loc></sitemap>
  <sitemap><loc>https://example.com/sitemap2.xml</loc></sitemap>
</sitemapindex>`)
	}))
	defer server.Close()

	entry := FetchSitemap(context.Background(), server.Client(), server.URL+"/sitemap_index.xml", "TestBot")

	if entry.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", entry.StatusCode)
	}
	if entry.Type != "index" {
		t.Errorf("expected type 'index', got %q", entry.Type)
	}
	if len(entry.Sitemaps) != 2 {
		t.Fatalf("expected 2 child sitemaps, got %d", len(entry.Sitemaps))
	}
	if entry.Sitemaps[0] != "https://example.com/sitemap1.xml" {
		t.Errorf("sitemaps[0] = %q", entry.Sitemaps[0])
	}
}

func TestFetchSitemap_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	entry := FetchSitemap(context.Background(), server.Client(), server.URL+"/sitemap.xml", "TestBot")

	if entry.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", entry.StatusCode)
	}
	if entry.Type != "" {
		t.Errorf("expected empty type, got %q", entry.Type)
	}
}

func TestFetchSitemap_InvalidURL(t *testing.T) {
	entry := FetchSitemap(context.Background(), http.DefaultClient, "://bad-url", "TestBot")

	if entry.StatusCode != 0 {
		t.Errorf("expected status 0, got %d", entry.StatusCode)
	}
	if entry.URL != "://bad-url" {
		t.Errorf("expected URL preserved, got %q", entry.URL)
	}
}

// ---------------------------------------------------------------------------
// DiscoverSitemaps with httptest
// ---------------------------------------------------------------------------

func TestDiscoverSitemaps_RecursesIntoIndex(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server

	mux.HandleFunc("/sitemap_index.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/sitemap1.xml</loc></sitemap>
  <sitemap><loc>%s/sitemap2.xml</loc></sitemap>
</sitemapindex>`, server.URL, server.URL)
	})

	mux.HandleFunc("/sitemap1.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc></url>
</urlset>`)
	})

	mux.HandleFunc("/sitemap2.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/b</loc></url>
</urlset>`)
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	entries := DiscoverSitemaps(
		context.Background(),
		server.Client(),
		"TestBot",
		[]string{server.URL + "/sitemap_index.xml"},
	)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (1 index + 2 urlsets), got %d", len(entries))
	}

	totalURLs := 0
	for _, e := range entries {
		totalURLs += len(e.URLs)
	}
	if totalURLs != 2 {
		t.Errorf("expected 2 total URLs, got %d", totalURLs)
	}
}

func TestDiscoverSitemaps_DedupesInput(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc></url>
</urlset>`)
	}))
	defer server.Close()

	entries := DiscoverSitemaps(
		context.Background(),
		server.Client(),
		"TestBot",
		[]string{
			server.URL + "/sitemap.xml",
			server.URL + "/sitemap.xml", // duplicate
			server.URL + "/sitemap.xml", // duplicate
		},
	)

	if len(entries) != 1 {
		t.Errorf("expected 1 entry (deduped), got %d", len(entries))
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (deduped), got %d", callCount)
	}
}

func TestDiscoverSitemaps_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc></url>
</urlset>`)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	entries := DiscoverSitemaps(ctx, server.Client(), "TestBot",
		[]string{server.URL + "/sitemap.xml"})

	// With a cancelled context, should return no entries or at most partial
	if len(entries) > 1 {
		t.Errorf("expected at most 1 entry with cancelled context, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// New constructor
// ---------------------------------------------------------------------------

func TestNewFetcher(t *testing.T) {
	f := New("TestBot/1.0", 5*time.Second, 1024, DialOptions{AllowPrivateIPs: true}, "")

	if f.userAgent != "TestBot/1.0" {
		t.Errorf("userAgent = %q, want %q", f.userAgent, "TestBot/1.0")
	}
	if f.maxBodySize != 1024 {
		t.Errorf("maxBodySize = %d, want 1024", f.maxBodySize)
	}
	if f.client == nil {
		t.Fatal("client is nil")
	}
	if f.Client() != f.client {
		t.Error("Client() should return the internal client")
	}
}

func TestNewFetcher_WithTLSProfile(t *testing.T) {
	f := New("TestBot/1.0", 5*time.Second, 1024, DialOptions{AllowPrivateIPs: true}, TLSChrome)

	if f.client == nil {
		t.Fatal("client is nil")
	}
	// The transport should be wrapped (alpnSwitchTransport)
	if _, ok := f.client.Transport.(*alpnSwitchTransport); !ok {
		t.Errorf("expected *alpnSwitchTransport, got %T", f.client.Transport)
	}
}

func TestNewFetcher_UnknownTLSProfileFallsBack(t *testing.T) {
	f := New("TestBot/1.0", 5*time.Second, 1024, DialOptions{AllowPrivateIPs: true}, "safari")

	if f.client == nil {
		t.Fatal("client is nil")
	}
	// utlsTransport should fall back to base transport for unknown profile
	if _, ok := f.client.Transport.(*http.Transport); !ok {
		t.Errorf("expected *http.Transport fallback, got %T", f.client.Transport)
	}
}

// ---------------------------------------------------------------------------
// CategorizeError with SSRF error
// ---------------------------------------------------------------------------

func TestCategorizeError_SSRF(t *testing.T) {
	err := fmt.Errorf("%w: redirect to 10.0.0.1", ErrPrivateIP)
	got := CategorizeError(err)
	if got != "ssrf_blocked" {
		t.Errorf("CategorizeError(ErrPrivateIP) = %q, want %q", got, "ssrf_blocked")
	}
}
