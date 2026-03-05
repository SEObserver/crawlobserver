package fetcher

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/temoto/robotstxt"
)

// ---------------------------------------------------------------------------
// Fetch — cover non-HTML early return with headers
// ---------------------------------------------------------------------------

func TestFetchNonHTMLCollectsHeadersOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "value")
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, `{"key":"value"}`)
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 2, "https://parent.com")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("status = %d, want 200", result.StatusCode)
	}
	// Body should be empty for non-HTML
	if len(result.Body) != 0 {
		t.Errorf("expected empty body for JSON, got %d bytes", len(result.Body))
	}
	if result.BodySize != 0 {
		t.Errorf("expected bodySize 0, got %d", result.BodySize)
	}
	// But headers should still be captured
	if result.Headers == nil {
		t.Fatal("headers should be captured even for non-HTML")
	}
	if result.Headers["X-Custom"] != "value" {
		t.Errorf("X-Custom = %q, want value", result.Headers["X-Custom"])
	}
	if result.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want application/json", result.ContentType)
	}
	// Depth and FoundOn should be preserved
	if result.Depth != 2 {
		t.Errorf("Depth = %d, want 2", result.Depth)
	}
	if result.FoundOn != "https://parent.com" {
		t.Errorf("FoundOn = %q, want https://parent.com", result.FoundOn)
	}
	// Duration should be positive
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

// ---------------------------------------------------------------------------
// Fetch — cover FinalURL and empty content type (downloads body)
// ---------------------------------------------------------------------------

func TestFetchEmptyContentTypeDownloadsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally no Content-Type
		w.WriteHeader(200)
		fmt.Fprint(w, "<html>data</html>")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// Empty content type should be treated as potential HTML, body should be downloaded
	if len(result.Body) == 0 {
		t.Error("expected body to be downloaded when Content-Type is empty")
	}
}

// ---------------------------------------------------------------------------
// Fetch — server error status codes
// ---------------------------------------------------------------------------

func TestFetch500ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(500)
		fmt.Fprint(w, "<html>Internal Server Error</html>")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.StatusCode != 500 {
		t.Errorf("status = %d, want 500", result.StatusCode)
	}
	// Error field should be empty (HTTP errors are not fetch errors)
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	// Body should still be downloaded for HTML
	if len(result.Body) == 0 {
		t.Error("expected body for 500 HTML response")
	}
}

// ---------------------------------------------------------------------------
// Fetch — 301/302 status codes with final redirect landing page
// ---------------------------------------------------------------------------

func TestFetch301Redirect(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>New page</html>")
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/old", 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("status = %d, want 200", result.StatusCode)
	}
	if !strings.HasSuffix(result.FinalURL, "/new") {
		t.Errorf("FinalURL = %q, want /new suffix", result.FinalURL)
	}
	if len(result.RedirectChain) != 1 {
		t.Errorf("redirect chain len = %d, want 1", len(result.RedirectChain))
	}
	if result.RedirectChain[0].StatusCode != 301 {
		t.Errorf("redirect status = %d, want 301", result.RedirectChain[0].StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Fetch — connection refused (server not listening)
// ---------------------------------------------------------------------------

func TestFetchConnectionRefused(t *testing.T) {
	// Use a port that nobody is listening on
	f := New("TestBot/1.0", 2*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch("http://127.0.0.1:1", 0, "")

	if result.Error == "" {
		t.Fatal("expected error for connection refused")
	}
	if result.StatusCode != 0 {
		t.Errorf("status = %d, want 0 for connection error", result.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Fetch — 10+ redirect stops with error
// ---------------------------------------------------------------------------

func TestFetchTooManyRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always redirect to self
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/loop", 0, "")

	if result.Error == "" {
		t.Fatal("expected error for redirect loop")
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — cover net.Error timeout (not context.DeadlineExceeded)
// ---------------------------------------------------------------------------

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestCategorizeError_NetErrorTimeout(t *testing.T) {
	err := &net.OpError{
		Op:  "read",
		Err: &timeoutError{},
	}
	got := CategorizeError(err)
	if got != "timeout" {
		t.Errorf("CategorizeError(net timeout) = %q, want timeout", got)
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — wrapped errors
// ---------------------------------------------------------------------------

func TestCategorizeError_WrappedDNSError(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "example.invalid", IsNotFound: true}
	wrappedErr := fmt.Errorf("request failed: %w", dnsErr)
	got := CategorizeError(wrappedErr)
	if got != "dns_not_found" {
		t.Errorf("CategorizeError(wrapped DNS) = %q, want dns_not_found", got)
	}
}

func TestCategorizeError_WrappedContextDeadline(t *testing.T) {
	wrappedErr := fmt.Errorf("fetching page: %w", context.DeadlineExceeded)
	got := CategorizeError(wrappedErr)
	if got != "timeout" {
		t.Errorf("CategorizeError(wrapped deadline) = %q, want timeout", got)
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — OpError with non-dial op
// ---------------------------------------------------------------------------

func TestCategorizeError_OpErrorNonDial(t *testing.T) {
	err := &net.OpError{
		Op:  "read",
		Err: fmt.Errorf("connection reset by peer"),
	}
	got := CategorizeError(err)
	// Should fall through to generic error (or timeout if net.Error)
	if !strings.HasPrefix(got, "fetch_error") && got != "timeout" {
		t.Errorf("CategorizeError(OpError read) = %q, expected fetch_error or timeout", got)
	}
}

// ---------------------------------------------------------------------------
// isHTMLContentType — additional edge cases
// ---------------------------------------------------------------------------

func TestIsHTMLContentType_Variants(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/html; charset=ISO-8859-1", true},
		{"TEXT/HTML;CHARSET=UTF-8", true},
		{"application/xhtml+xml", true},
		{"text/css", false},
		{"text/javascript", false},
		{"multipart/form-data", false},
		{"application/octet-stream", false},
	}
	for _, tt := range tests {
		got := isHTMLContentType(tt.ct)
		if got != tt.want {
			t.Errorf("isHTMLContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FetchResult.IsHTML — additional edge cases
// ---------------------------------------------------------------------------

func TestFetchResultIsHTML_MixedCase(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"Text/Html; charset=utf-8", true},
		{"APPLICATION/XHTML+XML", true},
		{"text/plain", false},
		{"application/xml", false},
	}
	for _, tt := range tests {
		r := &FetchResult{ContentType: tt.ct}
		if got := r.IsHTML(); got != tt.want {
			t.Errorf("IsHTML(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — cover invalid URL parse error paths
// ---------------------------------------------------------------------------

func TestRobotsCacheIsAllowedInvalidURL(t *testing.T) {
	rc := &RobotsCache{
		cache:     make(map[string]*RobotsCacheEntry),
		userAgent: "TestBot",
		client:    &http.Client{Timeout: 2 * time.Second},
	}
	// Invalid URL should return true (allow on parse error)
	if !rc.IsAllowed("://invalid-url") {
		t.Error("expected IsAllowed to return true for invalid URL")
	}
}

func TestRobotsCacheCrawlDelayInvalidURL(t *testing.T) {
	rc := &RobotsCache{
		cache:     make(map[string]*RobotsCacheEntry),
		userAgent: "TestBot",
		client:    &http.Client{Timeout: 2 * time.Second},
	}
	// Invalid URL should return 0 delay
	delay := rc.CrawlDelay("://invalid-url")
	if delay != 0 {
		t.Errorf("expected 0 delay for invalid URL, got %v", delay)
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — cover fetch error (server unreachable)
// ---------------------------------------------------------------------------

func TestRobotsCacheFetchError(t *testing.T) {
	rc := NewRobotsCache("TestBot", 1*time.Second, DialOptions{AllowPrivateIPs: true}, "")
	// Use unreachable URL — should still allow everything
	allowed := rc.IsAllowed("http://127.0.0.1:1/some-page")
	if !allowed {
		t.Error("expected all URLs to be allowed when robots.txt fetch fails")
	}

	// Verify the entry was cached (with empty parsed data)
	entries := rc.Entries()
	if len(entries) != 1 {
		t.Errorf("expected 1 cached entry after failed fetch, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — cover double-check-after-lock path in fetch()
// ---------------------------------------------------------------------------

func TestRobotsCacheFetchDoubleCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nAllow: /\n")
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, DialOptions{AllowPrivateIPs: true}, "")

	// First call triggers fetch and cache
	rc.IsAllowed(server.URL + "/page1")

	// Second call should hit the cache (read path), not re-fetch
	rc.IsAllowed(server.URL + "/page2")

	entries := rc.Entries()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (same host), got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — cover fetch with malformed robots.txt (parse error)
// ---------------------------------------------------------------------------

func TestRobotsCacheMalformedRobotsTxt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Write something that will parse but is unusual
		fmt.Fprint(w, "This is not valid robots.txt format at all \x00\x01\x02")
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, DialOptions{AllowPrivateIPs: true}, "")

	// Should not panic; should allow everything
	allowed := rc.IsAllowed(server.URL + "/page")
	if !allowed {
		t.Error("expected all URLs to be allowed with malformed robots.txt")
	}

	entries := rc.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	for _, entry := range entries {
		if entry.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", entry.StatusCode)
		}
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — cover CrawlDelay with actual delay
// ---------------------------------------------------------------------------

func TestRobotsCacheCrawlDelayZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No crawl-delay directive
		fmt.Fprint(w, "User-agent: *\nAllow: /\n")
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, DialOptions{AllowPrivateIPs: true}, "")
	delay := rc.CrawlDelay(server.URL + "/page")
	if delay != 0 {
		t.Errorf("expected 0 delay, got %v", delay)
	}
}

// ---------------------------------------------------------------------------
// NewRobotsCache — with TLS profile
// ---------------------------------------------------------------------------

func TestNewRobotsCacheWithTLSProfile(t *testing.T) {
	rc := NewRobotsCache("TestBot", 5*time.Second, DialOptions{AllowPrivateIPs: true}, TLSChrome)
	if rc == nil {
		t.Fatal("NewRobotsCache with TLS profile returned nil")
	}
	if rc.client == nil {
		t.Fatal("client is nil")
	}
	// The transport should be wrapped for TLS profile
	if _, ok := rc.client.Transport.(*alpnSwitchTransport); !ok {
		t.Errorf("expected *alpnSwitchTransport with TLS profile, got %T", rc.client.Transport)
	}
}

func TestNewRobotsCacheWithoutTLSProfile(t *testing.T) {
	rc := NewRobotsCache("TestBot", 5*time.Second, DialOptions{}, "")
	if rc == nil {
		t.Fatal("NewRobotsCache without TLS profile returned nil")
	}
	if _, ok := rc.client.Transport.(*http.Transport); !ok {
		t.Errorf("expected *http.Transport without TLS profile, got %T", rc.client.Transport)
	}
}

// ---------------------------------------------------------------------------
// Fetch — cover the body read error path using a server that closes early
// ---------------------------------------------------------------------------

func TestFetchBodyReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", "1000000") // lie about content length
		w.WriteHeader(200)
		// Write some data then close connection abruptly
		fmt.Fprint(w, "<html>")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Hijack and close to force a read error
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			if conn != nil {
				conn.Close()
			}
		}
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	// This may result in either a body read error or a partial read
	// Either way, it should not panic
	_ = result.Error
}

// ---------------------------------------------------------------------------
// Fetch — exact body size (no truncation)
// ---------------------------------------------------------------------------

func TestFetchExactBodySizeNoTruncation(t *testing.T) {
	// Body size exactly equals maxBodySize
	const maxSize = 100
	body := strings.Repeat("x", maxSize)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, int64(maxSize), DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.BodyTruncated {
		t.Error("expected BodyTruncated=false when body exactly matches limit")
	}
	if result.BodySize != maxSize {
		t.Errorf("BodySize = %d, want %d", result.BodySize, maxSize)
	}
}

// ---------------------------------------------------------------------------
// Fetch — body 1 byte over limit triggers truncation
// ---------------------------------------------------------------------------

func TestFetchBodyOneBytOverLimit(t *testing.T) {
	const maxSize = 100
	body := strings.Repeat("x", maxSize+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, int64(maxSize), DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !result.BodyTruncated {
		t.Error("expected BodyTruncated=true when body is 1 byte over limit")
	}
	if result.BodySize != maxSize {
		t.Errorf("BodySize = %d, want %d", result.BodySize, maxSize)
	}
}

// ---------------------------------------------------------------------------
// Fetch — multi-value response headers are joined with comma
// ---------------------------------------------------------------------------

func TestFetchMultiValueHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>ok</html>")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// Multi-value headers should be joined with ", "
	cookies := result.Headers["Set-Cookie"]
	if !strings.Contains(cookies, "a=1") || !strings.Contains(cookies, "b=2") {
		t.Errorf("expected joined Set-Cookie header, got %q", cookies)
	}
}

// ---------------------------------------------------------------------------
// SafeDialContextWithOpts — with various options
// ---------------------------------------------------------------------------

func TestSafeDialContextWithOpts_ForceIPv4(t *testing.T) {
	dial := SafeDialContextWithOpts(DialOptions{ForceIPv4: true, AllowPrivateIPs: true})
	if dial == nil {
		t.Fatal("SafeDialContextWithOpts returned nil")
	}
}

func TestSafeDialContextWithOpts_SourceIP(t *testing.T) {
	dial := SafeDialContextWithOpts(DialOptions{SourceIP: "0.0.0.0", AllowPrivateIPs: true})
	if dial == nil {
		t.Fatal("SafeDialContextWithOpts returned nil")
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — SitemapURLs with parsed data having sitemaps
// ---------------------------------------------------------------------------

func TestSitemapURLs_DeclaredSitemapOnlyNoDupes(t *testing.T) {
	robotsBody := []byte("User-agent: *\nSitemap: https://example.com/custom.xml\nSitemap: https://example.com/custom2.xml\n")
	parsed, _ := robotstxt.FromBytes(robotsBody)

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
	// Should have: custom.xml, custom2.xml, sitemap.xml (fallback), sitemap_index.xml (fallback)
	if len(urls) != 4 {
		t.Errorf("expected 4 URLs, got %d: %v", len(urls), urls)
	}
}

// ---------------------------------------------------------------------------
// Fetch with SSRF protection — redirect to private IP
// ---------------------------------------------------------------------------

func TestFetchSSRFBlockRedirectToPrivateIP(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to a private IP
		http.Redirect(w, r, "http://127.0.0.1:1/evil", http.StatusFound)
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	// AllowPrivateIPs=false — should block the redirect
	f := New("TestBot/1.0", 5*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: false}, "")
	result := f.Fetch(server.URL+"/redir", 0, "")

	if result.Error == "" {
		t.Error("expected error for SSRF redirect to private IP")
	}
}

// ---------------------------------------------------------------------------
// Fetch — request creation error (invalid URL scheme)
// ---------------------------------------------------------------------------

func TestFetchRequestCreationError(t *testing.T) {
	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")

	// Various invalid URLs
	tests := []string{
		"://missing-scheme",
		"",
	}
	for _, u := range tests {
		result := f.Fetch(u, 0, "")
		if result.Error == "" {
			t.Errorf("expected error for invalid URL %q", u)
		}
		if result.Duration <= 0 {
			t.Errorf("expected positive duration even for error, URL=%q", u)
		}
	}
}

// ---------------------------------------------------------------------------
// Fetch — large body read with io.LimitReader
// ---------------------------------------------------------------------------

func TestFetchLargeBodyLimited(t *testing.T) {
	// Server sends 5MB but limit is 1KB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.Copy(w, io.LimitReader(neverEndingReader{}, 5*1024*1024))
	}))
	defer server.Close()

	f := New("TestBot/1.0", 30*time.Second, 1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !result.BodyTruncated {
		t.Error("expected body to be truncated")
	}
	if result.BodySize != 1024 {
		t.Errorf("body size = %d, want 1024", result.BodySize)
	}
}

// neverEndingReader provides an infinite stream of 'x' bytes.
type neverEndingReader struct{}

func (neverEndingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// ---------------------------------------------------------------------------
// RobotsCache.Entries — verify thread-safety
// ---------------------------------------------------------------------------

func TestRobotsCacheEntriesConcurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nAllow: /\n")
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, DialOptions{AllowPrivateIPs: true}, "")
	rc.IsAllowed(server.URL + "/page")

	// Concurrent calls to Entries should not race
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			entries := rc.Entries()
			done <- len(entries) >= 1
		}()
	}

	for i := 0; i < 20; i++ {
		if !<-done {
			t.Error("expected at least 1 entry from concurrent Entries() call")
		}
	}
}

// ---------------------------------------------------------------------------
// NewFetcher — verify User-Agent and Accept headers are sent
// ---------------------------------------------------------------------------

func TestFetchSendsCorrectHeaders(t *testing.T) {
	var receivedUA, receivedAccept, receivedAcceptLang string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		receivedAccept = r.Header.Get("Accept")
		receivedAcceptLang = r.Header.Get("Accept-Language")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>ok</html>")
	}))
	defer server.Close()

	f := New("CustomBot/2.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if receivedUA != "CustomBot/2.0" {
		t.Errorf("User-Agent = %q, want CustomBot/2.0", receivedUA)
	}
	if receivedAccept == "" {
		t.Error("expected Accept header to be sent")
	}
	if receivedAcceptLang == "" {
		t.Error("expected Accept-Language header to be sent")
	}
}

// ---------------------------------------------------------------------------
// SafeDialContextWithOpts — cover ForceIPv4 DNS filtering
// ---------------------------------------------------------------------------

func TestSafeDialContextWithOpts_ForceIPv4DNS(t *testing.T) {
	// ForceIPv4 but allow private IPs — this triggers the inner function path
	dial := SafeDialContextWithOpts(DialOptions{ForceIPv4: true, AllowPrivateIPs: true})
	if dial == nil {
		t.Fatal("SafeDialContextWithOpts returned nil")
	}
	// Try dialing localhost (IPv4)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create a quick test server to actually connect to
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	// Extract host:port
	addr := server.Listener.Addr().String()
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	conn.Close()
}

// ---------------------------------------------------------------------------
// SafeDialContextWithOpts — cover private IP blocking via DNS
// ---------------------------------------------------------------------------

func TestSafeDialContextWithOpts_PrivateIPBlocking(t *testing.T) {
	dial := SafeDialContextWithOpts(DialOptions{AllowPrivateIPs: false})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to dial a private IP — should be blocked
	_, err := dial(ctx, "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected error for private IP dial")
	}
	if !strings.Contains(err.Error(), "private") && !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error = %q, expected private/blocked mention", err)
	}
}

// ---------------------------------------------------------------------------
// SafeDialContextWithOpts — cover ForceIPv4 IP literal rejection
// ---------------------------------------------------------------------------

func TestSafeDialContextWithOpts_ForceIPv4RejectsIPv6Literal(t *testing.T) {
	dial := SafeDialContextWithOpts(DialOptions{ForceIPv4: true, AllowPrivateIPs: true})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to dial an IPv6 literal
	_, err := dial(ctx, "tcp", "[::1]:80")
	if err == nil {
		t.Fatal("expected error for IPv6 literal with ForceIPv4")
	}
	if !strings.Contains(err.Error(), "IPv6") {
		t.Errorf("error = %q, expected IPv6 mention", err)
	}
}

// ---------------------------------------------------------------------------
// SafeDialContextWithOpts — cover AllowPrivate + ForceIPv4 combination
// ---------------------------------------------------------------------------

func TestSafeDialContextWithOpts_AllowPrivateForceIPv4(t *testing.T) {
	dial := SafeDialContextWithOpts(DialOptions{ForceIPv4: true, AllowPrivateIPs: false})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to dial a private IPv4 — should be blocked
	_, err := dial(ctx, "tcp", "192.168.1.1:80")
	if err == nil {
		t.Fatal("expected error for private IP with AllowPrivateIPs=false")
	}
}

// ---------------------------------------------------------------------------
// SafeDialContextWithOpts — cover DNS resolution with private IP blocking
// ---------------------------------------------------------------------------

func TestSafeDialContextWithOpts_DNSResolutionPrivateBlock(t *testing.T) {
	dial := SafeDialContextWithOpts(DialOptions{AllowPrivateIPs: false})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// "localhost" resolves to 127.0.0.1, which is private
	_, err := dial(ctx, "tcp", "localhost:80")
	if err == nil {
		t.Fatal("expected error: localhost resolves to private IP")
	}
}

// ---------------------------------------------------------------------------
// SafeDialContextWithOpts — cover AllowPrivate=true, ForceIPv4=false (fast path)
// ---------------------------------------------------------------------------

func TestSafeDialContextWithOpts_AllowPrivateNoForceIPv4(t *testing.T) {
	dial := SafeDialContextWithOpts(DialOptions{AllowPrivateIPs: true, ForceIPv4: false})
	if dial == nil {
		t.Fatal("SafeDialContextWithOpts returned nil")
	}
	// This should return dialer.DialContext directly (fast path)
}

// ---------------------------------------------------------------------------
// utlsTransport — cover unknown profile fallback
// ---------------------------------------------------------------------------

func TestUtlsTransport_UnknownProfileFallback(t *testing.T) {
	base := &http.Transport{}
	rt := utlsTransport(TLSProfile("invalid"), nil, base)
	// Should fall back to the base transport
	if rt != base {
		t.Error("expected fallback to base transport for unknown profile")
	}
}

// ---------------------------------------------------------------------------
// utlsTransport — cover valid chrome profile returns alpnSwitchTransport
// ---------------------------------------------------------------------------

func TestUtlsTransport_ChromeProfile(t *testing.T) {
	safeDial := SafeDialContextWithOpts(DialOptions{AllowPrivateIPs: true})
	base := &http.Transport{DialContext: safeDial}
	rt := utlsTransport(TLSChrome, safeDial, base)
	if _, ok := rt.(*alpnSwitchTransport); !ok {
		t.Errorf("expected *alpnSwitchTransport, got %T", rt)
	}
}

func TestUtlsTransport_FirefoxProfile(t *testing.T) {
	safeDial := SafeDialContextWithOpts(DialOptions{AllowPrivateIPs: true})
	base := &http.Transport{DialContext: safeDial}
	rt := utlsTransport(TLSFirefox, safeDial, base)
	if _, ok := rt.(*alpnSwitchTransport); !ok {
		t.Errorf("expected *alpnSwitchTransport, got %T", rt)
	}
}

// ---------------------------------------------------------------------------
// clientHelloID — cover all branches
// ---------------------------------------------------------------------------

func TestClientHelloID_All(t *testing.T) {
	tests := []struct {
		profile TLSProfile
		wantErr bool
	}{
		{TLSChrome, false},
		{TLSFirefox, false},
		{TLSEdge, false},
		{"unknown", true},
		{"", true},
	}
	for _, tt := range tests {
		_, err := clientHelloID(tt.profile)
		if (err != nil) != tt.wantErr {
			t.Errorf("clientHelloID(%q) error = %v, wantErr = %v", tt.profile, err, tt.wantErr)
		}
	}
}

// ---------------------------------------------------------------------------
// hasPort — cover both paths
// ---------------------------------------------------------------------------

func TestHasPort_AdditionalCases(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"example.com:443", true},
		{"example.com", false},
		{"[::1]:80", true},
		{"::1", false},
	}
	for _, tt := range tests {
		if got := hasPort(tt.host); got != tt.want {
			t.Errorf("hasPort(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — cover TLS certificate error
// ---------------------------------------------------------------------------

func TestCategorizeError_TLSError(t *testing.T) {
	tlsErr := &tls.CertificateVerificationError{
		Err: fmt.Errorf("x509: certificate signed by unknown authority"),
	}
	got := CategorizeError(tlsErr)
	if got != "tls_error" {
		t.Errorf("CategorizeError(TLS) = %q, want tls_error", got)
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — cover SSRF blocked error
// ---------------------------------------------------------------------------

func TestCategorizeError_SSRFBlocked(t *testing.T) {
	err := fmt.Errorf("failed: %w", ErrPrivateIP)
	got := CategorizeError(err)
	if got != "ssrf_blocked" {
		t.Errorf("CategorizeError(SSRF) = %q, want ssrf_blocked", got)
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — cover nil error
// ---------------------------------------------------------------------------

func TestCategorizeError_Nil(t *testing.T) {
	got := CategorizeError(nil)
	if got != "" {
		t.Errorf("CategorizeError(nil) = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — cover DNS timeout
// ---------------------------------------------------------------------------

func TestCategorizeError_DNSTimeout(t *testing.T) {
	err := &net.DNSError{
		Err:       "i/o timeout",
		Name:      "example.com",
		IsTimeout: true,
	}
	got := CategorizeError(err)
	if got != "dns_timeout" {
		t.Errorf("CategorizeError(DNS timeout) = %q, want dns_timeout", got)
	}
}

// ---------------------------------------------------------------------------
// CategorizeError — cover context.Canceled
// ---------------------------------------------------------------------------

func TestCategorizeError_ContextCanceled(t *testing.T) {
	got := CategorizeError(context.Canceled)
	if !strings.HasPrefix(got, "fetch_error") {
		t.Errorf("CategorizeError(context.Canceled) = %q, want prefix fetch_error", got)
	}
}

// ---------------------------------------------------------------------------
// Fetch — cover redirect tracker context path
// ---------------------------------------------------------------------------

func TestFetchRedirectTrackerContextPath(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/step1", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/step1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/step2", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/step2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>final</html>")
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/start", 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(result.RedirectChain) != 2 {
		t.Errorf("redirect chain len = %d, want 2", len(result.RedirectChain))
	}
	if result.RedirectChain[0].StatusCode != 307 {
		t.Errorf("first redirect status = %d, want 307", result.RedirectChain[0].StatusCode)
	}
	if result.RedirectChain[1].StatusCode != 301 {
		t.Errorf("second redirect status = %d, want 301", result.RedirectChain[1].StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Fetch — cover SSRF block on direct private IP
// ---------------------------------------------------------------------------

func TestFetchSSRFDirectPrivateIP(t *testing.T) {
	f := New("TestBot/1.0", 2*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: false}, "")
	result := f.Fetch("http://192.168.1.1/page", 0, "")

	if result.Error == "" {
		t.Fatal("expected error for direct private IP")
	}
}

// ---------------------------------------------------------------------------
// IsPrivateIP — cover various ranges
// ---------------------------------------------------------------------------

func TestIsPrivateIP_Various(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.0.1", true},
		{"0.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"2001:db8::1", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("could not parse IP %q", tt.ip)
		}
		if got := IsPrivateIP(ip); got != tt.want {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — cover SitemapURLs with no sitemaps declared
// ---------------------------------------------------------------------------

func TestSitemapURLs_NoSitemapsDeclared(t *testing.T) {
	robotsBody := []byte("User-agent: *\nDisallow: /admin\n")
	parsed, _ := robotstxt.FromBytes(robotsBody)

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
	// Should still have the default fallback sitemaps
	if len(urls) < 2 {
		t.Errorf("expected at least 2 fallback sitemap URLs, got %d: %v", len(urls), urls)
	}
}

// ---------------------------------------------------------------------------
// RobotsCache — cover CrawlDelay with actual delay directive
// ---------------------------------------------------------------------------

func TestRobotsCacheCrawlDelayActual(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: TestBot\nCrawl-delay: 5\n")
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, DialOptions{AllowPrivateIPs: true}, "")
	delay := rc.CrawlDelay(server.URL + "/page")
	if delay != 5*time.Second {
		t.Errorf("CrawlDelay = %v, want 5s", delay)
	}
}

// ---------------------------------------------------------------------------
// Fetch — cover non-HTML content types that should not download body
// ---------------------------------------------------------------------------

func TestFetch_VariousNonHTMLContentTypes(t *testing.T) {
	tests := []struct {
		contentType string
	}{
		{"application/pdf"},
		{"image/jpeg"},
		{"application/javascript"},
		{"text/css"},
		{"font/woff2"},
	}
	for _, tt := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", tt.contentType)
			fmt.Fprint(w, "binary data here")
		}))

		f := New("TestBot/1.0", 5*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
		result := f.Fetch(server.URL, 0, "")

		if result.Error != "" {
			t.Errorf("CT=%s: unexpected error: %s", tt.contentType, result.Error)
		}
		if len(result.Body) != 0 {
			t.Errorf("CT=%s: expected empty body, got %d bytes", tt.contentType, len(result.Body))
		}
		if result.ContentType != tt.contentType {
			t.Errorf("CT=%s: ContentType = %q", tt.contentType, result.ContentType)
		}
		server.Close()
	}
}

// ---------------------------------------------------------------------------
// New — cover fetcher creation with TLS profile
// ---------------------------------------------------------------------------

func TestNewFetcher_WithTLSProfile_ClientAccess(t *testing.T) {
	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, TLSChrome)
	if f == nil {
		t.Fatal("New returned nil")
	}
	client := f.Client()
	if client == nil {
		t.Fatal("Client() returned nil")
	}
	if client.Timeout != 10*time.Second {
		t.Errorf("client Timeout = %v, want 10s", client.Timeout)
	}
}

// ---------------------------------------------------------------------------
// Fetch — cover the CheckRedirect SSRF path (redirect to private IP literal)
// ---------------------------------------------------------------------------

func TestFetchCheckRedirectSSRFPrivateIPLiteral(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to a private IP literal
		http.Redirect(w, r, "http://10.0.0.1/evil", http.StatusFound)
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	f := New("TestBot/1.0", 5*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: false}, "")
	result := f.Fetch(server.URL+"/start", 0, "")

	if result.Error == "" {
		t.Fatal("expected error for redirect to private IP literal")
	}
	if result.Error != "ssrf_blocked" {
		t.Errorf("Error = %q, want ssrf_blocked", result.Error)
	}
}
