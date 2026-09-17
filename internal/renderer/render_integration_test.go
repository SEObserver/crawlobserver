package renderer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests drive a real browser. They are the only place the DevTools path
// is exercised: the header rule it applies cannot be observed from Go, because
// Chromium — not net/http — makes the requests.
//
// Skipped when no browser can be launched, so that a machine without one still
// runs the rest of the suite. A launch that fails for any other reason is a
// failure, not a skip.

func newTestPool(t *testing.T, headers map[string]string) *Pool {
	t.Helper()

	opts := DefaultPoolOptions()
	opts.MaxPages = 1
	opts.PageTimeout = 20 * time.Second
	opts.ExtraHeaders = headers

	pool, err := NewPool(opts)
	if err != nil {
		t.Skipf("no browser available on this machine: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// renderTestSite serves a page that pulls a script from another host, and
// records the headers of every request it answers, whichever host was asked.
func renderTestSite(t *testing.T) (pageURL string, headersFor func(string) http.Header) {
	t.Helper()

	var mu sync.Mutex
	received := make(map[string]http.Header)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received[r.URL.Path] = r.Header.Clone()
		mu.Unlock()

		switch r.URL.Path {
		case "/third-party.js":
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, "window.__thirdParty = true;")
		default:
			// httptest listens on 127.0.0.1; naming it localhost gives a
			// genuinely different hostname on the same listener, which is what
			// makes the script a third party rather than a second path.
			other := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<!doctype html><html><head>
<script src="%s/third-party.js"></script>
</head><body><h1>rendered</h1></body></html>`, other)
		}
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/page", func(path string) http.Header {
		mu.Lock()
		defer mu.Unlock()
		return received[path]
	}
}

// The whole point of the hijack: the crawled site is told who is asking, and
// nobody else is. Chromium's own setExtraHTTPHeaders cannot make that
// distinction, which is why this path exists at all.
func TestRender_HeadersReachTheSiteAndNotThirdParties(t *testing.T) {
	headers := map[string]string{
		"Signature-Agent": `"https://example.com/.well-known/http-message-signatures-directory"`,
		"Signature":       `sig1=:dGVzdA==:`,
	}
	pool := newTestPool(t, headers)
	pageURL, headersFor := renderTestSite(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := pool.Render(ctx, pageURL)

	// First: rendering still works. Continuing a hijacked request with a
	// replaced header set is the part most likely to break everything.
	if result.Error != nil {
		t.Fatalf("Render failed: %v", result.Error)
	}
	if !strings.Contains(result.RenderedHTML, "rendered") {
		t.Fatalf("rendered HTML does not contain the page body:\n%s", result.RenderedHTML)
	}

	page := headersFor("/page")
	if page == nil {
		t.Fatal("the browser never requested the page")
	}
	for name, want := range headers {
		if got := page.Get(name); got != want {
			t.Errorf("the crawled page received %s = %q, want %q", name, got, want)
		}
	}

	// The browser must still have sent its own headers: continueRequest
	// replaces the whole set, so a bug here strips the user agent silently.
	if page.Get("User-Agent") == "" {
		t.Error("the crawled page received no User-Agent — the original headers were lost")
	}
	if page.Get("Accept") == "" {
		t.Error("the crawled page received no Accept — the original headers were lost")
	}

	third := headersFor("/third-party.js")
	if third == nil {
		t.Fatal("the browser never requested the third-party script, so this proves nothing")
	}
	for name := range headers {
		if got := third.Get(name); got != "" {
			t.Errorf("the third party received %s = %q, want it absent", name, got)
		}
	}
}

// A crawl with no headers must render exactly as it did before this feature.
func TestRender_WithoutHeadersStillRenders(t *testing.T) {
	pool := newTestPool(t, nil)
	pageURL, headersFor := renderTestSite(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := pool.Render(ctx, pageURL)
	if result.Error != nil {
		t.Fatalf("Render failed: %v", result.Error)
	}
	if !strings.Contains(result.RenderedHTML, "rendered") {
		t.Fatalf("rendered HTML does not contain the page body:\n%s", result.RenderedHTML)
	}
	if got := headersFor("/page").Get("Signature"); got != "" {
		t.Errorf("Signature = %q, want it absent", got)
	}
}
