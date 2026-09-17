package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/fetcher"
)

// These tests exercise the engine's own fetcher, robots cache and sitemap
// fetch rather than the fetcher package directly. Without them the wiring in
// NewEngine could be removed entirely and every test in this package would
// still pass — which is how it stood when this was first written.

func engineTestHeaders() map[string]string {
	return map[string]string{
		"Signature-Agent": `"https://example.com/.well-known/http-message-signatures-directory"`,
		"Signature":       `sig1=:dGVzdA==:`,
	}
}

// headerRecordingServer records the headers of every request it answers.
func headerRecordingServer(t *testing.T, body string) (*httptest.Server, func(string) http.Header) {
	t.Helper()
	received := make(map[string]http.Header)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received[r.URL.Path] = r.Header.Clone()
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, func(path string) http.Header { return received[path] }
}

func engineWithHeaders(headers map[string]string) *Engine {
	cfg := &config.Config{}
	cfg.Crawler.UserAgent = "TestBot/1.0"
	cfg.Crawler.Timeout = 5 * time.Second
	cfg.Crawler.MaxBodySize = 1 << 20
	cfg.Crawler.Delay = 0
	cfg.Crawler.AllowPrivateIPs = true
	cfg.Crawler.Headers = headers
	return NewEngine(cfg, nil)
}

func assertCarriesHeaders(t *testing.T, got http.Header, where string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: no request reached the server", where)
	}
	for name, want := range engineTestHeaders() {
		if g := got.Get(name); g != want {
			t.Errorf("%s: header %s = %q, want %q", where, name, g, want)
		}
	}
}

func TestEngine_PageFetchCarriesConfiguredHeaders(t *testing.T) {
	srv, received := headerRecordingServer(t, "<html><body>hi</body></html>")
	e := engineWithHeaders(engineTestHeaders())

	if res := e.fetch.Fetch(srv.URL+"/page", 0, ""); res.StatusCode != http.StatusOK {
		t.Fatalf("Fetch status = %d (%s), want 200", res.StatusCode, res.Error)
	}
	assertCarriesHeaders(t, received("/page"), "page fetch through the engine")
}

// robots.txt is fetched before anything else. A site that gates on a header
// answers 403 to an unsigned request, which reads as a disallow and stops the
// crawl before it fetches a single page.
func TestEngine_RobotsFetchCarriesConfiguredHeaders(t *testing.T) {
	srv, received := headerRecordingServer(t, "User-agent: *\nAllow: /\n")
	e := engineWithHeaders(engineTestHeaders())

	e.robots.IsAllowed(srv.URL + "/page")
	assertCarriesHeaders(t, received("/robots.txt"), "robots.txt through the engine")
}

func TestEngine_SitemapFetchCarriesConfiguredHeaders(t *testing.T) {
	srv, received := headerRecordingServer(t,
		`<?xml version="1.0"?><urlset><url><loc>https://example.com/</loc></url></urlset>`)
	e := engineWithHeaders(engineTestHeaders())

	e.ctx = t.Context()
	e.retrieveSitemaps([]string{srv.URL + "/sitemap.xml"})
	assertCarriesHeaders(t, received("/sitemap.xml"), "sitemap through the engine")
}

// An engine built without headers must send none, so that the tests above are
// measuring the headers rather than something the server sends anyway.
func TestEngine_NoHeadersConfiguredSendsNone(t *testing.T) {
	srv, received := headerRecordingServer(t, "ok")
	e := engineWithHeaders(nil)

	e.fetch.Fetch(srv.URL+"/page", 0, "")
	if got := received("/page").Get("Signature"); got != "" {
		t.Errorf("Signature = %q, want it absent", got)
	}
}

// The renderer receives the headers filtered by the same rule the HTTP paths
// use, since the DevTools protocol applies none of net/http's protections.
func TestEngine_RendererHeadersAreSanitized(t *testing.T) {
	headers := map[string]string{
		"Signature-Agent": `"https://example.com/"`,
		"User-Agent":      "Impostor/9.9",
		"Host":            "example.com",
	}
	got := fetcher.SanitizeExtraHeaders(headers)

	if _, ok := got["Signature-Agent"]; !ok {
		t.Error("the renderer would not receive Signature-Agent")
	}
	for _, refused := range []string{"User-Agent", "Host"} {
		if _, ok := got[refused]; ok {
			t.Errorf("the renderer would receive %s, which the HTTP paths refuse", refused)
		}
	}
}
