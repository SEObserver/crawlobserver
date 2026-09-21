package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/fetcher"
	"github.com/SEObserver/crawlobserver/internal/frontier"
	"github.com/SEObserver/crawlobserver/internal/parser"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

func newHreflangTestEngine(t *testing.T, follow bool, scope string, maxDepth int) *Engine {
	t.Helper()
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: scope,
			MaxDepth:   maxDepth,
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://shop.example.com/"}, cfg)
	engine.buildScope()
	engine.followHreflang = follow
	return engine
}

var shopAlternates = []parser.HreflangEntry{
	{Lang: "fr", URL: "https://shop.example.com/products/chair"},       // self
	{Lang: "en", URL: "https://shop.example.com/en/products/chair"},    // in scope
	{Lang: "de", URL: "/de/products/chair"},                            // relative, in scope
	{Lang: "en-us", URL: "https://us.shop.example.com/products/chair"}, // other host
	{Lang: "x-default", URL: "https://shop.example.com/en/products/chair"},
}

func drainFrontier(f *frontier.Frontier) map[string]frontier.CrawlURL {
	queued := map[string]frontier.CrawlURL{}
	for f.Len() > 0 {
		item := f.Next()
		if item == nil {
			break
		}
		queued[item.URL] = *item
	}
	return queued
}

func TestEnqueueHreflangAlternates_QueuesInScopeAlternates(t *testing.T) {
	engine := newHreflangTestEngine(t, true, "host", 0)
	result := &fetcher.FetchResult{
		URL:      "https://shop.example.com/products/chair",
		FinalURL: "https://shop.example.com/products/chair",
		Depth:    2,
	}

	added := engine.enqueueHreflangAlternates(result, shopAlternates)

	if added != 2 {
		t.Fatalf("expected the en and de alternates to be queued once each, got %d", added)
	}
	queued := drainFrontier(engine.front)
	for _, want := range []string{"https://shop.example.com/en/products/chair", "https://shop.example.com/de/products/chair"} {
		item, ok := queued[want]
		if !ok {
			t.Errorf("%s should have been queued, queued = %v", want, queued)
			continue
		}
		if item.Depth != 3 {
			t.Errorf("%s should sit one level below its page, depth = %d", want, item.Depth)
		}
		if item.FoundOn != result.URL {
			t.Errorf("%s should be attributed to %s, got %s", want, result.URL, item.FoundOn)
		}
	}
	if len(queued) != 2 {
		t.Errorf("self, duplicate and off-host alternates must not be queued, got %v", queued)
	}
}

func TestEnqueueHreflangAlternates_DisabledByDefault(t *testing.T) {
	engine := newHreflangTestEngine(t, false, "host", 0)
	result := &fetcher.FetchResult{URL: "https://shop.example.com/products/chair", Depth: 1}

	if added := engine.enqueueHreflangAlternates(result, shopAlternates); added != 0 {
		t.Fatalf("nothing should be queued when the setting is off, got %d", added)
	}
	if engine.front.Len() != 0 {
		t.Fatal("frontier should stay empty when the setting is off")
	}
}

func TestEnqueueHreflangAlternates_RedirectedPageResolvesAgainstFinalURL(t *testing.T) {
	engine := newHreflangTestEngine(t, true, "host", 0)
	// /fr redirected to /fr/ with tracking noise; the page declares itself
	// with the raw final URL and its sibling with a relative href.
	result := &fetcher.FetchResult{
		URL:      "https://shop.example.com/fr",
		FinalURL: "https://Shop.Example.com/fr/?utm_source=newsletter",
		Depth:    1,
	}
	alternates := []parser.HreflangEntry{
		{Lang: "fr", URL: "https://Shop.Example.com/fr/?utm_source=newsletter"},
		{Lang: "en", URL: "../en/"},
	}

	added := engine.enqueueHreflangAlternates(result, alternates)

	queued := drainFrontier(engine.front)
	if added != 1 || len(queued) != 1 {
		t.Fatalf("only the en alternate should be queued, added = %d, queued = %v", added, queued)
	}
	if _, ok := queued["https://shop.example.com/en/"]; !ok {
		t.Errorf("relative alternate should resolve against the final URL, queued = %v", queued)
	}
}

func TestEnqueueHreflangAlternates_DomainScopeAdmitsSubdomains(t *testing.T) {
	engine := newHreflangTestEngine(t, true, "domain", 0)
	result := &fetcher.FetchResult{URL: "https://shop.example.com/products/chair", Depth: 1}

	if added := engine.enqueueHreflangAlternates(result, shopAlternates); added != 3 {
		t.Fatalf("domain scope should admit the us. subdomain too, got %d", added)
	}
	if _, ok := drainFrontier(engine.front)["https://us.shop.example.com/products/chair"]; !ok {
		t.Error("the subdomain alternate should be queued under domain scope")
	}
}

func TestEnqueueHreflangAlternates_RespectsExcludesAndDepth(t *testing.T) {
	engine := newHreflangTestEngine(t, true, "host", 2)
	engine.excludePatterns = []string{"/de/"}
	result := &fetcher.FetchResult{URL: "https://shop.example.com/products/chair", Depth: 1}

	if added := engine.enqueueHreflangAlternates(result, shopAlternates); added != 1 {
		t.Fatalf("only the en alternate should pass the exclude pattern, got %d", added)
	}
	if item := engine.front.Next(); item == nil || item.URL != "https://shop.example.com/en/products/chair" {
		t.Fatalf("unexpected queued item %+v", item)
	}

	deep := &fetcher.FetchResult{URL: "https://shop.example.com/collections/all", Depth: 2}
	if added := engine.enqueueHreflangAlternates(deep, shopAlternates); added != 0 {
		t.Fatalf("alternates beyond max depth must not be queued, got %d", added)
	}
}

func TestEnqueueHreflangAlternates_SitemapOnlyIgnoresThem(t *testing.T) {
	engine := newHreflangTestEngine(t, true, "host", 0)
	engine.sitemapOnly = true
	result := &fetcher.FetchResult{URL: "https://shop.example.com/products/chair", Depth: 1}

	if added := engine.enqueueHreflangAlternates(result, shopAlternates); added != 0 {
		t.Fatalf("sitemap-only crawls must not follow alternates, got %d", added)
	}
}

func TestCrawlRequest_FollowHreflangField(t *testing.T) {
	var req CrawlRequest
	if err := json.Unmarshal([]byte(`{"seeds":["https://shop.example.com/"],"follow_hreflang":true}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.FollowHreflang {
		t.Fatal("follow_hreflang should decode into CrawlRequest.FollowHreflang")
	}
}

// runParseWorkerWithHreflang parses one page whose alternates live on the test
// server itself, and returns how many URLs the parse worker queued.
func runParseWorkerWithHreflang(t *testing.T, follow bool) int {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
			<link rel="alternate" hreflang="en" href="%s/en">
			<link rel="alternate" hreflang="fr" href="%s/fr">
		</head><body><h1>Test</h1></body></html>`, server.URL, server.URL)
	}))
	defer server.Close()
	host, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:    1,
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     1000,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:            cfg,
		ctx:            ctx,
		cancel:         cancel,
		front:          frontier.New(0, 10000),
		retryQueue:     NewRetryQueue(),
		hostHealth:     NewHostHealth(),
		retryPolicy:    &RetryPolicy{MaxRetries: 0},
		session:        &Session{ID: "test-follow-hreflang"},
		allowedHosts:   map[string]bool{host.Hostname(): true},
		followHreflang: follow,
	}
	e.buffer = storage.NewBuffer(&e2eInserter{}, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)
	e.parseWorker(0, parseCh)
	e.buffer.Flush()

	return e.front.Len()
}

func TestParseWorker_FollowsHreflangAlternatesOnRequest(t *testing.T) {
	if got := runParseWorkerWithHreflang(t, true); got != 2 {
		t.Errorf("parse worker should queue the two alternates when asked, queued %d", got)
	}
	if got := runParseWorkerWithHreflang(t, false); got != 0 {
		t.Errorf("parse worker must not queue alternates by default, queued %d", got)
	}
}
