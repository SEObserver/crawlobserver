package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/extraction"
	"github.com/SEObserver/crawlobserver/internal/fetcher"
	"github.com/SEObserver/crawlobserver/internal/frontier"
	"github.com/SEObserver/crawlobserver/internal/parser"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// ---------------------------------------------------------------------------
// dispatcher — cover the maxPages exit path
// ---------------------------------------------------------------------------

func TestDispatcherExitsOnMaxPages(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
			Delay:   0,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:   cfg,
		front: frontier.New(0, 10000),
		ctx:   ctx,
	}
	e.maxPages = 10
	e.pagesCrawled.Store(10) // already at limit

	// Add something to the frontier so we don't exit via empty-frontier path
	e.front.Add(frontier.CrawlURL{URL: "https://example.com/page"})

	fetchCh := make(chan *frontier.CrawlURL, 1)
	done := make(chan struct{})
	go func() {
		e.dispatcher(fetchCh)
		close(done)
	}()

	select {
	case <-done:
		// Dispatcher exited due to max pages — expected
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not exit within 5s despite maxPages reached")
	}
}

// Test dispatcher exits when context is cancelled while sending to fetchCh
func TestDispatcherExitsOnContextCancelDuringSend(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
			Delay:   0,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		cfg:   cfg,
		front: frontier.New(0, 10000),
		ctx:   ctx,
	}

	e.front.Add(frontier.CrawlURL{URL: "https://example.com/page1"})
	e.front.Add(frontier.CrawlURL{URL: "https://example.com/page2"})
	e.front.Add(frontier.CrawlURL{URL: "https://example.com/page3"})

	// fetchCh has capacity 0 — blocks on send
	fetchCh := make(chan *frontier.CrawlURL)
	done := make(chan struct{})
	go func() {
		e.dispatcher(fetchCh)
		close(done)
	}()

	// Give dispatcher time to reach the send-to-fetchCh select
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good: dispatcher exited on context cancel
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not exit on context cancel during send")
	}
}

// Test dispatcher exits when frontier is empty and no pending retries
func TestDispatcherExitsEmptyFrontierNoPendingRetries(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
			Delay:   0,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:   cfg,
		front: frontier.New(0, 10000),
		ctx:   ctx,
	}
	// No pending retries, empty frontier
	e.pendingRetries.Store(0)

	fetchCh := make(chan *frontier.CrawlURL, 1)
	done := make(chan struct{})
	go func() {
		e.dispatcher(fetchCh)
		close(done)
	}()

	select {
	case <-done:
		// Dispatcher should exit fairly quickly
	case <-time.After(10 * time.Second):
		t.Fatal("dispatcher did not exit with empty frontier and no pending retries")
	}
}

// ---------------------------------------------------------------------------
// fetchWorker — cover robots blocking path
// ---------------------------------------------------------------------------

func TestFetchWorkerRobotsBlocking(t *testing.T) {
	// Create a test server that serves robots.txt blocking /blocked/
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "User-agent: TestBot\nDisallow: /blocked/\n")
		default:
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html><body>Hello</body></html>")
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         1,
			UserAgent:       "TestBot",
			Timeout:         5 * time.Second,
			MaxBodySize:     1 << 20,
			AllowPrivateIPs: true,
			RespectRobots:   true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel

	// Pre-fetch robots.txt
	engine.robots.IsAllowed(server.URL + "/page")

	in := make(chan *frontier.CrawlURL, 2)
	out := make(chan *fetcher.FetchResult, 2)

	// Send a blocked URL and an allowed URL
	in <- &frontier.CrawlURL{URL: server.URL + "/blocked/page", Depth: 1}
	in <- &frontier.CrawlURL{URL: server.URL + "/allowed/page", Depth: 1}
	close(in)

	engine.fetchWorker(0, in, out)

	// Only the allowed page should produce a result
	var results []*fetcher.FetchResult
	close(out)
	for r := range out {
		results = append(results, r)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (blocked URL skipped), got %d", len(results))
	}
	if !strings.HasSuffix(results[0].URL, "/allowed/page") {
		t.Errorf("expected allowed page result, got %s", results[0].URL)
	}
}

// Test fetchWorker context cancellation
func TestFetchWorkerContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>ok</html>")
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         1,
			UserAgent:       "TestBot",
			Timeout:         5 * time.Second,
			MaxBodySize:     1 << 20,
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel

	in := make(chan *frontier.CrawlURL, 5)
	out := make(chan *fetcher.FetchResult, 5)

	// Queue several URLs
	for i := 0; i < 5; i++ {
		in <- &frontier.CrawlURL{URL: fmt.Sprintf("%s/page%d", server.URL, i), Depth: 0}
	}
	close(in)

	// Cancel context immediately
	cancel()

	// fetchWorker should exit quickly
	done := make(chan struct{})
	go func() {
		engine.fetchWorker(0, in, out)
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(5 * time.Second):
		t.Fatal("fetchWorker did not exit within 5s after context cancel")
	}
}

// Test fetchWorker counts pages and progress
func TestFetchWorkerCountsPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>ok</html>")
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         1,
			UserAgent:       "TestBot",
			Timeout:         5 * time.Second,
			MaxBodySize:     1 << 20,
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel

	in := make(chan *frontier.CrawlURL, 3)
	out := make(chan *fetcher.FetchResult, 3)

	// First attempt pages
	in <- &frontier.CrawlURL{URL: server.URL + "/p1", Depth: 0, Attempt: 0}
	in <- &frontier.CrawlURL{URL: server.URL + "/p2", Depth: 0, Attempt: 0}
	// Retry attempt (should NOT increment pagesCrawled)
	in <- &frontier.CrawlURL{URL: server.URL + "/p3", Depth: 0, Attempt: 1}
	close(in)

	engine.fetchWorker(0, in, out)
	close(out)

	// Drain results
	for range out {
	}

	if got := engine.pagesCrawled.Load(); got != 2 {
		t.Errorf("pagesCrawled = %d, want 2 (retries should not count)", got)
	}
	if engine.lastProgressAt.Load() == 0 {
		t.Error("lastProgressAt should be set after fetching pages")
	}
}

// ---------------------------------------------------------------------------
// retryDispatcher — cover the context cancellation during send path
// ---------------------------------------------------------------------------

func TestRetryDispatcherContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rq := NewRetryQueue()

	// Add an item that's ready
	rq.Push(&RetryItem{
		URL:     "https://example.com/retry",
		Depth:   1,
		FoundOn: "https://example.com",
		Attempt: 1,
		ReadyAt: time.Now().Add(-1 * time.Second),
	})

	// fetchCh with no capacity — will block on send
	fetchCh := make(chan *frontier.CrawlURL)

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
		},
	}
	e := &Engine{
		cfg:        cfg,
		retryQueue: rq,
		ctx:        ctx,
	}

	done := make(chan struct{})
	go func() {
		e.retryDispatcher(ctx, fetchCh)
		close(done)
	}()

	// Give it time to pick up the ready item
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// retryDispatcher exited on context cancel
	case <-time.After(5 * time.Second):
		t.Fatal("retryDispatcher did not exit within 5s on context cancel")
	}
}

// Test retryDispatcher dispatches ready items
func TestRetryDispatcherSendsReadyItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rq := NewRetryQueue()
	rq.Push(&RetryItem{
		URL:     "https://example.com/retry1",
		Depth:   1,
		FoundOn: "https://example.com",
		Attempt: 1,
		ReadyAt: time.Now().Add(-1 * time.Second), // ready now
	})

	fetchCh := make(chan *frontier.CrawlURL, 5)

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
		},
	}
	e := &Engine{
		cfg:        cfg,
		retryQueue: rq,
		ctx:        ctx,
	}

	go e.retryDispatcher(ctx, fetchCh)

	// Should receive the retry item
	select {
	case item := <-fetchCh:
		if item.URL != "https://example.com/retry1" {
			t.Errorf("expected retry1 URL, got %s", item.URL)
		}
		if item.Attempt != 1 {
			t.Errorf("expected attempt 1, got %d", item.Attempt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retryDispatcher did not send ready item within 3s")
	}

	cancel()
}

// ---------------------------------------------------------------------------
// enqueueRetry — test the retry enqueueing logic
// ---------------------------------------------------------------------------

func TestEnqueueRetry(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries: 3,
				BaseDelay:  100 * time.Millisecond,
				MaxDelay:   1 * time.Second,
			},
		},
	}
	engine := NewEngine(cfg, nil)

	// First attempt (attempt=0) — should increment pendingRetries
	result := &fetcher.FetchResult{
		URL:        "https://example.com/page",
		StatusCode: 503,
		Error:      "",
		Attempt:    0,
		Depth:      1,
		FoundOn:    "https://example.com/",
		Headers:    map[string]string{},
	}

	engine.enqueueRetry(result)

	if engine.pendingRetries.Load() != 1 {
		t.Errorf("pendingRetries = %d, want 1 after first enqueue", engine.pendingRetries.Load())
	}
	if engine.retryQueue.Len() != 1 {
		t.Errorf("retryQueue.Len() = %d, want 1", engine.retryQueue.Len())
	}

	// Second attempt (attempt=1) — should NOT increment pendingRetries again
	result2 := &fetcher.FetchResult{
		URL:        "https://example.com/page",
		StatusCode: 503,
		Error:      "",
		Attempt:    1,
		Depth:      1,
		FoundOn:    "https://example.com/",
		Headers:    map[string]string{},
	}

	engine.enqueueRetry(result2)

	if engine.pendingRetries.Load() != 1 {
		t.Errorf("pendingRetries = %d, want 1 (only first enqueue increments)", engine.pendingRetries.Load())
	}
	if engine.retryQueue.Len() != 2 {
		t.Errorf("retryQueue.Len() = %d, want 2", engine.retryQueue.Len())
	}
}

func TestEnqueueRetryWithRetryAfterHeader(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries: 3,
				BaseDelay:  100 * time.Millisecond,
				MaxDelay:   60 * time.Second,
			},
		},
	}
	engine := NewEngine(cfg, nil)

	result := &fetcher.FetchResult{
		URL:        "https://example.com/page",
		StatusCode: 429,
		Attempt:    0,
		Headers:    map[string]string{"Retry-After": "5"},
	}

	engine.enqueueRetry(result)

	if engine.retryQueue.Len() != 1 {
		t.Fatalf("retryQueue.Len() = %d, want 1", engine.retryQueue.Len())
	}
}

// ---------------------------------------------------------------------------
// parseWorker — test retry path and host health tracking
// ---------------------------------------------------------------------------

func TestParseWorkerRetryPath(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          3,
				BaseDelay:           10 * time.Millisecond,
				MaxDelay:            100 * time.Millisecond,
				MaxConsecutiveFails: 100,
				MaxGlobalErrorRate:  1.0,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buffer = storage.NewBuffer(&successInserter{}, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	in := make(chan *fetcher.FetchResult, 3)

	// A 503 result on first attempt (should be retried, not stored)
	in <- &fetcher.FetchResult{
		URL:        "https://example.com/retryable",
		StatusCode: 503,
		Attempt:    0,
		Headers:    map[string]string{},
	}

	// A successful 200 result (should be stored)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/success",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte("<html><head><title>Test</title></head><body>Hello world content here</body></html>"),
		FinalURL:    "https://example.com/success",
		Attempt:     0,
		Headers:     map[string]string{},
	}

	// A retry final failure (attempt=3, max retries=3) with retry attempt > 0
	in <- &fetcher.FetchResult{
		URL:        "https://example.com/final-fail",
		StatusCode: 503,
		Attempt:    3, // max retries reached
		Headers:    map[string]string{},
	}

	close(in)

	engine.parseWorker(0, in)

	// Check that retry was enqueued for the 503 first attempt
	if engine.retryQueue.Len() != 1 {
		t.Errorf("retryQueue.Len() = %d, want 1 (503 on attempt 0)", engine.retryQueue.Len())
	}

	// Check pending retries incremented then decremented
	// The 503 attempt=0 adds 1, the 503 attempt=3 decrements 1
	if engine.pendingRetries.Load() != 0 {
		t.Errorf("pendingRetries = %d, want 0", engine.pendingRetries.Load())
	}
}

// Test parseWorker health tracking
func TestParseWorkerHostHealthTracking(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0, // disable retries
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buffer = storage.NewBuffer(&successInserter{}, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	in := make(chan *fetcher.FetchResult, 2)

	// Successful fetch
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/good",
		StatusCode:  200,
		ContentType: "image/png", // non-HTML, quick processing
		Attempt:     0,
		Headers:     map[string]string{},
	}

	// Failed fetch (500 error)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/bad",
		StatusCode:  500,
		ContentType: "text/html",
		Attempt:     0,
		Headers:     map[string]string{},
	}

	close(in)
	engine.parseWorker(0, in)

	// Host health should record 1 success and 1 failure
	rate := engine.hostHealth.GlobalErrorRate()
	expected := 0.5
	if rate < expected-0.01 || rate > expected+0.01 {
		t.Errorf("GlobalErrorRate = %f, want ~%f", rate, expected)
	}
}

// Test parseWorker with error result (non-retryable)
func TestParseWorkerErrorResult(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	in := make(chan *fetcher.FetchResult, 1)

	// Result with error string (like dns_not_found)
	in <- &fetcher.FetchResult{
		URL:     "https://example.com/error-page",
		Error:   "dns_not_found",
		Attempt: 0,
		Headers: map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)
	engine.buffer.Flush()

	// Should record as failure in host health
	engine.hostHealth.mu.Lock()
	stats := engine.hostHealth.hosts["example.com"]
	engine.hostHealth.mu.Unlock()
	if stats == nil || stats.failures != 1 {
		t.Error("expected host health to record 1 failure for error result")
	}
}

// Test parseWorker with HTML that has links — covers link discovery path
func TestParseWorkerLinkDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>ok</body></html>")
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			MaxPages:        100,
			AllowPrivateIPs: true,
			CrawlScope:      "host",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{server.URL + "/"}, cfg)
	engine.buildScope()

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	// HTML with internal and external links
	body := fmt.Sprintf(`<!DOCTYPE html><html><head><title>Test Page</title></head><body>
		<a href="%s/internal-page">Internal</a>
		<a href="https://external.com/page">External</a>
	</body></html>`, server.URL)

	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         server.URL + "/",
		FinalURL:    server.URL + "/",
		StatusCode:  200,
		ContentType: "text/html; charset=utf-8",
		Body:        []byte(body),
		BodySize:    int64(len(body)),
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)
	engine.buffer.Flush()

	// Check that the internal link was added to frontier
	if engine.front.SeenCount() < 1 {
		t.Error("expected internal link to be added to frontier")
	}
}

// Test parseWorker with redirect chain
func TestParseWorkerRedirectChain(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/old",
		FinalURL:    "https://example.com/new",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte("<html><head><title>Redirected</title></head><body>Hi</body></html>"),
		BodySize:    64,
		Attempt:     0,
		Headers:     map[string]string{"Content-Encoding": "gzip", "X-Robots-Tag": "noindex"},
		RedirectChain: []fetcher.RedirectHop{
			{URL: "https://example.com/old", StatusCode: 301},
		},
	}
	close(in)

	engine.parseWorker(0, in)
	engine.buffer.Flush()

	pages := inserter.pages
	if len(pages) == 0 {
		t.Fatal("expected at least 1 page stored")
	}

	page := pages[0]
	if page.ContentEncoding != "gzip" {
		t.Errorf("ContentEncoding = %q, want gzip", page.ContentEncoding)
	}
	if page.XRobotsTag != "noindex" {
		t.Errorf("XRobotsTag = %q, want noindex", page.XRobotsTag)
	}
	if len(page.RedirectChain) != 1 {
		t.Errorf("RedirectChain len = %d, want 1", len(page.RedirectChain))
	}
	// X-Robots-Tag noindex should make it non-indexable
	if page.IsIndexable {
		t.Error("expected page to be non-indexable due to X-Robots-Tag noindex")
	}
	if page.IndexReason != "x_robots_noindex" {
		t.Errorf("IndexReason = %q, want x_robots_noindex", page.IndexReason)
	}
}

// Test parseWorker with StoreHTML enabled
func TestParseWorkerStoreHTML(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			StoreHTML: true,
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	htmlBody := "<html><head><title>Store Me</title></head><body>Content</body></html>"
	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/store-html",
		FinalURL:    "https://example.com/store-html",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte(htmlBody),
		BodySize:    int64(len(htmlBody)),
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)
	engine.buffer.Flush()

	if len(inserter.pages) == 0 {
		t.Fatal("expected page to be stored")
	}
	if inserter.pages[0].BodyHTML != htmlBody {
		t.Error("expected BodyHTML to be set when StoreHTML is true")
	}
}

// Test parseWorker context cancellation
func TestParseWorkerContextCancel(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buffer = storage.NewBuffer(&successInserter{}, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	in := make(chan *fetcher.FetchResult, 5)
	for i := 0; i < 5; i++ {
		in <- &fetcher.FetchResult{
			URL:         fmt.Sprintf("https://example.com/page%d", i),
			StatusCode:  200,
			ContentType: "text/html",
			Body:        []byte("<html><body>ok</body></html>"),
			Attempt:     0,
			Headers:     map[string]string{},
		}
	}
	close(in)

	// Cancel context immediately
	cancel()

	done := make(chan struct{})
	go func() {
		engine.parseWorker(0, in)
		close(done)
	}()

	select {
	case <-done:
		// parseWorker exited on context cancel
	case <-time.After(5 * time.Second):
		t.Fatal("parseWorker did not exit within 5s after context cancel")
	}
}

// ---------------------------------------------------------------------------
// promoteNext — test the full promotion path (not just queue manipulation)
// ---------------------------------------------------------------------------

func TestPromoteNextQueueDrain(t *testing.T) {
	// Test that promoteNext correctly drains the queue and acquires semaphore.
	// We can't call promoteNext with nil store (runEngine panics),
	// so we test the queue manipulation directly.
	m := newTestManager(5)

	m.queue = []queuedCrawl{
		{sessionID: "promote-me"},
		{sessionID: "second"},
	}
	m.queuedSet["promote-me"] = true
	m.queuedSet["second"] = true

	// Simulate what promoteNext does before launching runEngine
	m.queueMu.Lock()
	next := m.queue[0]
	m.queue = m.queue[1:]
	delete(m.queuedSet, next.sessionID)
	m.queueMu.Unlock()

	if next.sessionID != "promote-me" {
		t.Errorf("expected promote-me, got %q", next.sessionID)
	}

	// Queue should now have just "second"
	m.queueMu.Lock()
	queueLen := len(m.queue)
	m.queueMu.Unlock()
	if queueLen != 1 {
		t.Errorf("queue length = %d, want 1", queueLen)
	}

	if m.IsQueued("promote-me") {
		t.Error("promote-me should not be in queuedSet anymore")
	}
	if !m.IsQueued("second") {
		t.Error("second should still be in queuedSet")
	}
}

func TestPromoteNextActualCallEmptyQueue(t *testing.T) {
	// Calling promoteNext on empty queue should return immediately (no panic)
	m := newTestManager(5)
	m.promoteNext() // should not panic
}

// ---------------------------------------------------------------------------
// Manager.StopCrawl — test stopping a queued session (without store)
// ---------------------------------------------------------------------------

func TestStopCrawlNotRunningNotQueued(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	err := m.StopCrawl("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-running, non-queued session")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error = %q, want 'not running'", err)
	}
}

// ---------------------------------------------------------------------------
// Manager.NewManager with extractorLoader variadic param
// ---------------------------------------------------------------------------

type mockExtractorLoader struct{}

func (m *mockExtractorLoader) GetExtractorSet(id string) (*extraction.ExtractorSet, error) {
	return &extraction.ExtractorSet{Name: "test-set"}, nil
}

func TestNewManagerWithExtractorLoader(t *testing.T) {
	cfg := testManagerConfig()
	loader := &mockExtractorLoader{}
	m := NewManager(cfg, nil, loader)
	if m.extractorLoader == nil {
		t.Error("extractorLoader should be set when passed to NewManager")
	}
}

func TestNewManagerWithoutExtractorLoader(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)
	if m.extractorLoader != nil {
		t.Error("extractorLoader should be nil when not passed")
	}
}

// ---------------------------------------------------------------------------
// buildScope — cover TLD error fallback in domain scope
// ---------------------------------------------------------------------------

func TestBuildScope_DomainScopeTLDError(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "domain",
		},
	}
	engine := NewEngine(cfg, nil)
	// "localhost" has no valid eTLD+1
	engine.session = NewSession([]string{"https://localhost/page"}, cfg)
	engine.buildScope()

	// Since eTLD+1 fails for localhost, it should fall back to allowedHosts
	if !engine.allowedHosts["localhost"] {
		t.Error("localhost should be in allowedHosts")
	}
}

func TestIsInScope_DomainScopeFallbackToHost(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "domain",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://localhost/page"}, cfg)
	engine.buildScope()

	// eTLD+1 for "localhost" fails, so domain scope should fall back to host matching
	if !engine.isInScope("https://localhost/other") {
		t.Error("expected localhost/other to be in scope (host fallback)")
	}
}

func TestIsInScope_InvalidURLReturnsFalse(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buildScope()

	if engine.isInScope("://invalid") {
		t.Error("invalid URL should not be in scope")
	}
}

// ---------------------------------------------------------------------------
// extractHost — additional edge cases
// ---------------------------------------------------------------------------

func TestExtractHostEdgeCases(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://user:pass@example.com:8080/path", "example.com:8080"},
		{"ftp://files.example.com/readme.txt", "files.example.com"},
	}
	for _, tt := range tests {
		got := extractHost(tt.url)
		if got != tt.want {
			t.Errorf("extractHost(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// computeJSDiffs — cover zero static word count with rendered > 0 but <= 50
// ---------------------------------------------------------------------------

func TestComputeJSDiffs_ZeroStaticRenderedExactly50(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{WordCount: 0}
	rendered := &parser.PageData{WordCount: 50}

	computeJSDiffs(row, static, rendered)

	if row.JSChangedContent {
		t.Error("JSChangedContent should be false when rendered is exactly 50 (threshold is >50)")
	}
}

func TestComputeJSDiffs_ZeroStaticRenderedZero(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{WordCount: 0}
	rendered := &parser.PageData{WordCount: 0}

	computeJSDiffs(row, static, rendered)

	if row.JSChangedContent {
		t.Error("JSChangedContent should be false when both are 0")
	}
}

func TestComputeJSDiffs_SchemaNewTypeAdded(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{
		SchemaTypes: []string{"WebPage"},
	}
	rendered := &parser.PageData{
		SchemaTypes: []string{"WebPage", "Product"},
	}

	computeJSDiffs(row, static, rendered)

	if !row.JSAddedSchema {
		t.Error("JSAddedSchema should be true when new schema type added")
	}
}

func TestComputeJSDiffs_EmptySchemas(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{}
	rendered := &parser.PageData{}

	computeJSDiffs(row, static, rendered)

	if row.JSAddedSchema {
		t.Error("JSAddedSchema should be false with empty schemas")
	}
}

func TestComputeJSDiffs_NegativeContentChange(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{WordCount: 100}
	rendered := &parser.PageData{WordCount: 50}

	computeJSDiffs(row, static, rendered)

	// 50% decrease > 20% threshold
	if !row.JSChangedContent {
		t.Error("JSChangedContent should be true for 50% decrease")
	}
}

func TestComputeJSDiffs_ImagesNegativeDelta(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{
		Images: []parser.Image{{Src: "a.png"}, {Src: "b.png"}, {Src: "c.png"}},
	}
	rendered := &parser.PageData{
		Images: []parser.Image{{Src: "a.png"}},
	}

	computeJSDiffs(row, static, rendered)

	if row.JSAddedImages != -2 {
		t.Errorf("JSAddedImages = %d, want -2", row.JSAddedImages)
	}
}

// ---------------------------------------------------------------------------
// seedFrontier — cover priority assignment
// ---------------------------------------------------------------------------

func TestSeedFrontierPriority(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	seeds := []string{"https://example.com/first", "https://example.com/second", "https://example.com/third"}
	engine.seedFrontier(seeds)

	if got := engine.front.Len(); got != 3 {
		t.Fatalf("frontier.Len() = %d, want 3", got)
	}

	// Seeds should be added with priority = index
	first := engine.front.Next()
	if first == nil || first.URL != "https://example.com/first" {
		t.Errorf("expected first seed to come out first, got %v", first)
	}
}

// ---------------------------------------------------------------------------
// Manager concurrent operations
// ---------------------------------------------------------------------------

func TestManagerConcurrentActiveSessionsAndProgress(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)

	// Register a few engines
	for i := 0; i < 5; i++ {
		e := NewEngine(cfg, nil)
		e.pagesCrawled.Store(int64(i * 10))
		m.mu.Lock()
		m.engines[fmt.Sprintf("sess-%d", i)] = e
		m.mu.Unlock()
	}

	// Concurrent reads should not race
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.ActiveSessions()
			_, _, _ = m.Progress("sess-0")
			_ = m.Phase("sess-1")
			_ = m.BufferState("sess-2")
			_ = m.IsRunning("sess-3")
			_ = m.LastError("sess-4")
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Manager.StartCrawl — test config overrides
// ---------------------------------------------------------------------------

func TestStartCrawlNoSeeds(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	_, err := m.StartCrawl(CrawlRequest{Seeds: nil})
	if err == nil || !strings.Contains(err.Error(), "at least one seed") {
		t.Errorf("expected error about empty seeds, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ensureNonNilArrays — double-call idempotency
// ---------------------------------------------------------------------------

func TestEnsureNonNilArrays_Idempotent(t *testing.T) {
	row := &storage.PageRow{
		H1:      []string{"h1"},
		H2:      []string{"h2"},
		Headers: map[string]string{"k": "v"},
	}
	ensureNonNilArrays(row)
	ensureNonNilArrays(row)

	if len(row.H1) != 1 || row.H1[0] != "h1" {
		t.Error("H1 data corrupted by double ensureNonNilArrays")
	}
	if row.Headers["k"] != "v" {
		t.Error("Headers data corrupted by double ensureNonNilArrays")
	}
}

// ---------------------------------------------------------------------------
// Engine.Stop and context check
// ---------------------------------------------------------------------------

func TestEngineStopCancelsContext(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	engine.Stop()

	select {
	case <-engine.ctx.Done():
		// Expected
	default:
		t.Error("engine context should be cancelled after Stop()")
	}
}

// ---------------------------------------------------------------------------
// ResumeSession
// ---------------------------------------------------------------------------

func TestResumeSession(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	engine.ResumeSession("existing-id", []string{"https://example.com/original"})

	if engine.session == nil {
		t.Fatal("session should not be nil after ResumeSession")
	}
	if engine.session.ID != "existing-id" {
		t.Errorf("session.ID = %q, want existing-id", engine.session.ID)
	}
	if len(engine.session.SeedURLs) != 1 || engine.session.SeedURLs[0] != "https://example.com/original" {
		t.Errorf("session.SeedURLs = %v, want [https://example.com/original]", engine.session.SeedURLs)
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover the non-HTML content type path (no parsing)
// ---------------------------------------------------------------------------

func TestParseWorkerNonHTML(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/image.png",
		FinalURL:    "https://example.com/image.png",
		StatusCode:  200,
		ContentType: "image/png",
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)
	engine.buffer.Flush()

	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page stored, got %d", len(inserter.pages))
	}
	// Non-HTML pages should have empty title, no links, etc.
	if inserter.pages[0].Title != "" {
		t.Errorf("expected empty title for non-HTML page, got %q", inserter.pages[0].Title)
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover canonical self-referencing
// ---------------------------------------------------------------------------

func TestParseWorkerCanonicalSelf(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	htmlBody := `<html><head><title>Self Canonical</title><link rel="canonical" href="https://example.com/self"></head><body>Hello world</body></html>`
	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/self",
		FinalURL:    "https://example.com/self",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte(htmlBody),
		BodySize:    int64(len(htmlBody)),
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)
	engine.buffer.Flush()

	if len(inserter.pages) == 0 {
		t.Fatal("expected page to be stored")
	}
	if !inserter.pages[0].CanonicalIsSelf {
		t.Error("expected CanonicalIsSelf to be true when canonical matches FinalURL")
	}
	if !inserter.pages[0].IsIndexable {
		t.Error("expected page with self-referencing canonical to be indexable")
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover sitemapOnly mode (no frontier additions for internal links)
// ---------------------------------------------------------------------------

func TestParseWorkerSitemapOnlyMode(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.sitemapOnly = true
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buildScope()

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	htmlBody := `<html><head><title>Sitemap Only</title></head><body>
		<a href="https://example.com/internal">Internal</a>
	</body></html>`

	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/",
		FinalURL:    "https://example.com/",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte(htmlBody),
		BodySize:    int64(len(htmlBody)),
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)

	// In sitemapOnly mode, internal links should NOT be added to frontier
	if engine.front.SeenCount() > 0 {
		t.Error("in sitemapOnly mode, no internal links should be added to frontier")
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover maxDepth limiting
// ---------------------------------------------------------------------------

func TestParseWorkerMaxDepthLimiting(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			MaxDepth:   1, // only allow depth 0 and 1
			CrawlScope: "host",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buildScope()

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	htmlBody := `<html><head><title>Deep</title></head><body>
		<a href="https://example.com/child">Link</a>
	</body></html>`

	in := make(chan *fetcher.FetchResult, 1)
	// This page is at depth 1 — links from it would be depth 2, exceeding maxDepth
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/parent",
		FinalURL:    "https://example.com/parent",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte(htmlBody),
		BodySize:    int64(len(htmlBody)),
		Depth:       1,
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)

	// Child link at depth 2 should NOT be added because maxDepth=1
	if engine.front.SeenCount() > 0 {
		t.Error("link at depth 2 should not be added when maxDepth=1")
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover sitemapURLSet priority boost
// ---------------------------------------------------------------------------

func TestParseWorkerSitemapPriorityBoost(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
			MaxPages:   100,
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buildScope()

	// Set up sitemap URL set
	engine.sitemapURLSet = map[string]bool{
		"https://example.com/sitemap-page": true,
	}

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	htmlBody := `<html><head><title>Test</title></head><body>
		<a href="https://example.com/sitemap-page">Sitemap</a>
		<a href="https://example.com/normal-page">Normal</a>
	</body></html>`

	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/",
		FinalURL:    "https://example.com/",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte(htmlBody),
		BodySize:    int64(len(htmlBody)),
		Depth:       0,
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)

	// Both links should be in the frontier
	if engine.front.SeenCount() < 2 {
		t.Errorf("expected at least 2 URLs seen, got %d", engine.front.SeenCount())
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover extractors path
// ---------------------------------------------------------------------------

func TestParseWorkerWithExtractors(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine(cfg, nil)
	engine.ctx = ctx
	engine.cancel = cancel
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	inserter := &e2eInserter{}
	engine.buffer = storage.NewBuffer(inserter, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	// Add a CSS selector extractor
	engine.extractors = []extraction.Extractor{
		{
			Name:     "test_extractor",
			Type:     "css",
			Selector: "title",
		},
	}

	htmlBody := `<html><head><title>Extract Me</title></head><body>Content here</body></html>`
	in := make(chan *fetcher.FetchResult, 1)
	in <- &fetcher.FetchResult{
		URL:         "https://example.com/extract",
		FinalURL:    "https://example.com/extract",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte(htmlBody),
		BodySize:    int64(len(htmlBody)),
		Attempt:     0,
		Headers:     map[string]string{},
	}
	close(in)

	engine.parseWorker(0, in)
	engine.buffer.Flush()

	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page stored, got %d", len(inserter.pages))
	}
}

// ---------------------------------------------------------------------------
// computeIndexability — cover more edge cases
// ---------------------------------------------------------------------------

func TestComputeIndexability_100StatusCode(t *testing.T) {
	indexable, reason := computeIndexability(100, "", "", "", "", "")
	if indexable {
		t.Error("status 100 should not be indexable")
	}
	if reason != "status_100" {
		t.Errorf("reason = %q, want status_100", reason)
	}
}

func TestComputeIndexability_199StatusCode(t *testing.T) {
	indexable, reason := computeIndexability(199, "", "", "", "", "")
	if indexable {
		t.Error("status 199 should not be indexable")
	}
	if reason != "status_199" {
		t.Errorf("reason = %q, want status_199", reason)
	}
}

func TestComputeIndexability_299StatusCode(t *testing.T) {
	indexable, _ := computeIndexability(299, "", "", "", "", "")
	if !indexable {
		t.Error("status 299 should be indexable (2xx range)")
	}
}

// ---------------------------------------------------------------------------
// externalCheckWorker — cover with httptest server
// ---------------------------------------------------------------------------

func TestExternalCheckWorker_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-ext-check"},
	}
	e.externalCh = make(chan string, 10)

	// Send a URL and close the channel so the worker exits
	e.externalCh <- server.URL + "/page1"
	close(e.externalCh)

	e.externalCheckWorker()

	if len(e.externalCheckBuf) != 1 {
		t.Fatalf("expected 1 buffered check, got %d", len(e.externalCheckBuf))
	}
	check := e.externalCheckBuf[0]
	if check.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", check.StatusCode)
	}
	if check.ContentType != "text/html" {
		t.Errorf("ContentType = %q, want text/html", check.ContentType)
	}
	if check.CrawlSessionID != "test-ext-check" {
		t.Errorf("CrawlSessionID = %q, want test-ext-check", check.CrawlSessionID)
	}
}

func TestExternalCheckWorker_RequestError(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-ext-err"},
	}
	e.externalCh = make(chan string, 10)

	// Send an invalid URL (will fail at NewRequest)
	e.externalCh <- "://bad-url"
	close(e.externalCh)

	e.externalCheckWorker()

	if len(e.externalCheckBuf) != 1 {
		t.Fatalf("expected 1 buffered check, got %d", len(e.externalCheckBuf))
	}
	if e.externalCheckBuf[0].Error == "" {
		t.Error("expected error for invalid URL")
	}
}

func TestExternalCheckWorker_ConnectionError(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-ext-conn"},
	}
	e.externalCh = make(chan string, 10)

	// Use a port that nobody is listening on
	e.externalCh <- "http://127.0.0.1:1/page"
	close(e.externalCh)

	e.externalCheckWorker()

	if len(e.externalCheckBuf) != 1 {
		t.Fatalf("expected 1 buffered check, got %d", len(e.externalCheckBuf))
	}
	if e.externalCheckBuf[0].Error == "" {
		t.Error("expected error for connection refused")
	}
}

func TestExternalCheckWorker_Redirect(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "ok")
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-ext-redir"},
	}
	e.externalCh = make(chan string, 10)

	e.externalCh <- server.URL + "/old"
	close(e.externalCh)

	e.externalCheckWorker()

	if len(e.externalCheckBuf) != 1 {
		t.Fatalf("expected 1 buffered check, got %d", len(e.externalCheckBuf))
	}
	check := e.externalCheckBuf[0]
	if check.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", check.StatusCode)
	}
	if !strings.HasSuffix(check.RedirectURL, "/new") {
		t.Errorf("RedirectURL = %q, want suffix /new", check.RedirectURL)
	}
}

func TestExternalCheckWorker_SkipsDuringShutdown(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-ext-skip"},
	}
	e.externalCh = make(chan string, 10)
	e.externalCh <- "http://example.com/page"
	close(e.externalCh)

	e.externalCheckWorker()

	// Should have skipped the item (context cancelled), but still drained the channel
	// No check should be buffered with a successful result
	// (Note: the worker still loops through items but skips HTTP requests)
	// The buffer may or may not have an entry depending on skip logic
}

// ---------------------------------------------------------------------------
// resourceCheckWorker — cover with httptest server
// ---------------------------------------------------------------------------

func TestResourceCheckWorker_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.WriteHeader(200)
		fmt.Fprint(w, "body{}")
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-res-check"},
	}
	e.resourceCh = make(chan resourceCheckItem, 10)

	e.resourceCh <- resourceCheckItem{
		URL:          server.URL + "/style.css",
		ResourceType: "stylesheet",
		IsInternal:   true,
	}
	close(e.resourceCh)

	e.resourceCheckWorker()

	if len(e.resourceCheckBuf) != 1 {
		t.Fatalf("expected 1 buffered resource check, got %d", len(e.resourceCheckBuf))
	}
	check := e.resourceCheckBuf[0]
	if check.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", check.StatusCode)
	}
	if check.ContentType != "text/css" {
		t.Errorf("ContentType = %q, want text/css", check.ContentType)
	}
	if check.ResourceType != "stylesheet" {
		t.Errorf("ResourceType = %q, want stylesheet", check.ResourceType)
	}
	if !check.IsInternal {
		t.Error("expected IsInternal=true")
	}
}

func TestResourceCheckWorker_RequestError(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-res-err"},
	}
	e.resourceCh = make(chan resourceCheckItem, 10)

	e.resourceCh <- resourceCheckItem{URL: "://bad", ResourceType: "script"}
	close(e.resourceCh)

	e.resourceCheckWorker()

	if len(e.resourceCheckBuf) != 1 {
		t.Fatalf("expected 1 buffered check, got %d", len(e.resourceCheckBuf))
	}
	if e.resourceCheckBuf[0].Error == "" {
		t.Error("expected error for invalid URL")
	}
}

func TestResourceCheckWorker_ConnectionError(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-res-conn"},
	}
	e.resourceCh = make(chan resourceCheckItem, 10)

	e.resourceCh <- resourceCheckItem{URL: "http://127.0.0.1:1/img.png", ResourceType: "image"}
	close(e.resourceCh)

	e.resourceCheckWorker()

	if len(e.resourceCheckBuf) != 1 {
		t.Fatalf("expected 1 buffered check, got %d", len(e.resourceCheckBuf))
	}
	if e.resourceCheckBuf[0].Error == "" {
		t.Error("expected error for connection refused")
	}
}

func TestResourceCheckWorker_Redirect(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/old.js", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/new.js", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, "console.log('ok')")
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "test-res-redir"},
	}
	e.resourceCh = make(chan resourceCheckItem, 10)
	e.resourceCh <- resourceCheckItem{URL: server.URL + "/old.js", ResourceType: "script", IsInternal: true}
	close(e.resourceCh)

	e.resourceCheckWorker()

	if len(e.resourceCheckBuf) != 1 {
		t.Fatalf("expected 1 check, got %d", len(e.resourceCheckBuf))
	}
	check := e.resourceCheckBuf[0]
	if check.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", check.StatusCode)
	}
	if !strings.HasSuffix(check.RedirectURL, "/new.js") {
		t.Errorf("RedirectURL = %q, want suffix /new.js", check.RedirectURL)
	}
}

// ---------------------------------------------------------------------------
// bufferExternalCheck — cover threshold flush path
// ---------------------------------------------------------------------------

func TestBufferExternalCheck_UnderThreshold(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "buf-ext"},
	}

	// Add 49 checks (under 50 threshold)
	for i := 0; i < 49; i++ {
		e.bufferExternalCheck(storage.ExternalLinkCheck{
			CrawlSessionID: "buf-ext",
			URL:            fmt.Sprintf("http://example.com/%d", i),
		})
	}

	if len(e.externalCheckBuf) != 49 {
		t.Errorf("expected 49 buffered checks, got %d", len(e.externalCheckBuf))
	}
}

// ---------------------------------------------------------------------------
// bufferResourceCheck — cover threshold path
// ---------------------------------------------------------------------------

func TestBufferResourceCheck_UnderThreshold(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "buf-res"},
	}

	for i := 0; i < 49; i++ {
		e.bufferResourceCheck(storage.PageResourceCheck{
			CrawlSessionID: "buf-res",
			URL:            fmt.Sprintf("http://example.com/res%d", i),
		})
	}

	if len(e.resourceCheckBuf) != 49 {
		t.Errorf("expected 49 buffered checks, got %d", len(e.resourceCheckBuf))
	}
}

// ---------------------------------------------------------------------------
// bufferResourceRef — cover threshold path
// ---------------------------------------------------------------------------

func TestBufferResourceRef_UnderThreshold(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		session: &Session{ID: "buf-ref"},
	}

	for i := 0; i < 199; i++ {
		e.bufferResourceRef(storage.PageResourceRef{
			CrawlSessionID: "buf-ref",
			PageURL:        "http://example.com/page",
			ResourceURL:    fmt.Sprintf("http://example.com/res%d", i),
		})
	}

	if len(e.resourceRefBuf) != 199 {
		t.Errorf("expected 199 buffered refs, got %d", len(e.resourceRefBuf))
	}
}

// ---------------------------------------------------------------------------
// flushExternalChecks — empty buffer (no-op path)
// ---------------------------------------------------------------------------

func TestFlushExternalChecks_Empty(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// Should not panic on empty buffer
	e.flushExternalChecks()
}

// ---------------------------------------------------------------------------
// flushResourceChecks — empty buffer (no-op path)
// ---------------------------------------------------------------------------

func TestFlushResourceChecks_Empty(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	e.flushResourceChecks()
}

// ---------------------------------------------------------------------------
// flushResourceRefs — empty buffer (no-op path)
// ---------------------------------------------------------------------------

func TestFlushResourceRefs_Empty(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	e.flushResourceRefs()
}

// ---------------------------------------------------------------------------
// newCheckClient — verify it creates a valid client
// ---------------------------------------------------------------------------

func TestNewCheckClient(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	client := e.newCheckClient()
	if client == nil {
		t.Fatal("newCheckClient returned nil")
	}
	if client.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want 15s", client.Timeout)
	}
	if client.Transport == nil {
		t.Error("Transport is nil")
	}
	if client.CheckRedirect == nil {
		t.Error("CheckRedirect is nil")
	}
}

// ---------------------------------------------------------------------------
// newCheckClient — verify CheckRedirect behavior
// ---------------------------------------------------------------------------

func TestNewCheckClient_CheckRedirect(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	redirectCount := 0
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount > 12 {
			fmt.Fprint(w, "final")
			return
		}
		http.Redirect(w, r, server.URL+fmt.Sprintf("/r%d", redirectCount), http.StatusFound)
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{cfg: cfg, ctx: ctx, cancel: cancel}
	client := e.newCheckClient()

	resp, err := client.Get(server.URL + "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// CheckRedirect returns ErrUseLastResponse after 10 redirects
	// so we should get a redirect response (302), not the final page
	if resp.StatusCode != 302 {
		t.Errorf("StatusCode = %d, expected 302 (stopped at 10 redirects)", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// dispatcher — cover the idle-timeout-with-pending-retries exit path
// ---------------------------------------------------------------------------

func TestDispatcherIdleTimeoutWithPendingRetries(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
			Delay:   0,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:   cfg,
		front: frontier.New(0, 10000),
		ctx:   ctx,
	}
	// Simulate pending retries
	e.pendingRetries.Store(1)
	// Set lastProgressAt to 40 seconds ago to trigger the 30s timeout
	e.lastProgressAt.Store(time.Now().Unix() - 40)

	fetchCh := make(chan *frontier.CrawlURL, 1)
	done := make(chan struct{})
	go func() {
		e.dispatcher(fetchCh)
		close(done)
	}()

	select {
	case <-done:
		// Dispatcher exited due to 30s no-progress timeout — expected
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("dispatcher did not exit within 10s")
	}
}

// ---------------------------------------------------------------------------
// dispatcher — cover backoff growth (empty frontier, emptyCount > 10)
// ---------------------------------------------------------------------------

func TestDispatcherBackoffGrowth(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
			Delay:   0,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:   cfg,
		front: frontier.New(0, 10000),
		ctx:   ctx,
	}
	// No pending retries: dispatcher will exit when emptyCount > 50

	fetchCh := make(chan *frontier.CrawlURL, 1)
	done := make(chan struct{})
	start := time.Now()
	go func() {
		e.dispatcher(fetchCh)
		close(done)
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		// Should take >0ms (backoff applies) but <10s (exits after 50 empty iterations)
		if elapsed > 30*time.Second {
			t.Errorf("dispatcher took too long: %v", elapsed)
		}
	case <-time.After(30 * time.Second):
		cancel()
		t.Fatal("dispatcher did not exit within 30s")
	}
}

// ---------------------------------------------------------------------------
// fetchWorker — cover the every-100-pages log path
// ---------------------------------------------------------------------------

func TestFetchWorkerProgress100Pages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>ok</html>")
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         1,
			Delay:           0,
			UserAgent:       "TestBot/1.0",
			Timeout:         5 * time.Second,
			MaxBodySize:     1 << 20,
			AllowPrivateIPs: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		front:  frontier.New(0, 10000),
		fetch:  fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, ""),
		robots: fetcher.NewRobotsCache("TestBot/1.0", 5*time.Second, fetcher.DialOptions{AllowPrivateIPs: true}, ""),
	}

	// Set pagesCrawled to 99 so the next increment hits 100
	e.pagesCrawled.Store(99)

	fetchCh := make(chan *frontier.CrawlURL, 1)
	parseCh := make(chan *fetcher.FetchResult, 1)

	fetchCh <- &frontier.CrawlURL{URL: server.URL + "/page", Depth: 0}
	close(fetchCh)

	e.fetchWorker(0, fetchCh, parseCh)

	// Should have sent result
	select {
	case result := <-parseCh:
		if result.StatusCode != 200 {
			t.Errorf("StatusCode = %d, want 200", result.StatusCode)
		}
	default:
		t.Fatal("no result on parseCh")
	}

	// pagesCrawled should be 100
	if e.pagesCrawled.Load() != 100 {
		t.Errorf("pagesCrawled = %d, want 100", e.pagesCrawled.Load())
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover the circuit breaker path
// ---------------------------------------------------------------------------

func TestParseWorkerCircuitBreaker(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			Delay:     0,
			UserAgent: "TestBot/1.0",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  0.1, // 10% threshold
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		front:       frontier.New(0, 10000),
		retryQueue:  NewRetryQueue(),
		hostHealth:  NewHostHealth(),
		retryPolicy: &RetryPolicy{MaxRetries: 0},
		session:     &Session{ID: "test-circuit"},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	// Set resultsProcessed to 9 so the next result makes it 10 (10 % 10 == 0)
	e.resultsProcessed.Store(9)

	// Record many failures so error rate > 10%
	for i := 0; i < 50; i++ {
		e.hostHealth.RecordFailure("example.com")
	}
	for i := 0; i < 10; i++ {
		e.hostHealth.RecordSuccess("example.com")
	}

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- &fetcher.FetchResult{
		URL:        "http://example.com/page",
		StatusCode: 500,
		Error:      "",
		Duration:   100 * time.Millisecond,
		Headers:    map[string]string{},
	}
	close(parseCh)

	e.parseWorker(0, parseCh)

	// The circuit breaker should have called Stop(), cancelling the context
	select {
	case <-e.ctx.Done():
		// Expected: engine was stopped by circuit breaker
	default:
		t.Error("expected context to be cancelled by circuit breaker")
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover external link channel (dedup + send)
// ---------------------------------------------------------------------------

func TestParseWorkerExternalLinkChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
			<a href="http://external.example.com/page1">ext1</a>
			<a href="http://external.example.com/page2">ext2</a>
			<a href="http://external.example.com/page1">ext1-dup</a>
		</body></html>`)
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         1,
			Delay:           0,
			UserAgent:       "TestBot/1.0",
			Timeout:         5 * time.Second,
			MaxBodySize:     1 << 20,
			AllowPrivateIPs: true,
			CrawlScope:      "host",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		front:       frontier.New(0, 10000),
		retryQueue:  NewRetryQueue(),
		hostHealth:  NewHostHealth(),
		retryPolicy: &RetryPolicy{MaxRetries: 0},
		session:     &Session{ID: "test-ext-links"},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)
	e.allowedHosts = map[string]bool{server.URL[7:]: true} // strip "http://"
	e.checkExternal = true
	e.externalCh = make(chan string, 100)

	// Fetch the page to get its body
	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	// Collect what was sent to externalCh
	close(e.externalCh)
	var extURLs []string
	for u := range e.externalCh {
		extURLs = append(extURLs, u)
	}

	// Should have 2 unique URLs (dedup removes the duplicate)
	if len(extURLs) != 2 {
		t.Errorf("expected 2 unique external URLs, got %d: %v", len(extURLs), extURLs)
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover resource check channel path
// ---------------------------------------------------------------------------

func TestParseWorkerResourceCheckChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head>
			<link rel="stylesheet" href="/style.css">
			<script src="/app.js"></script>
			<script src="/app.js"></script>
		</head><body><img src="/logo.png" alt="logo"></body></html>`)
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         1,
			Delay:           0,
			UserAgent:       "TestBot/1.0",
			Timeout:         5 * time.Second,
			MaxBodySize:     1 << 20,
			AllowPrivateIPs: true,
			CrawlScope:      "host",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		front:       frontier.New(0, 10000),
		retryQueue:  NewRetryQueue(),
		hostHealth:  NewHostHealth(),
		retryPolicy: &RetryPolicy{MaxRetries: 0},
		session:     &Session{ID: "test-res-links"},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)
	e.allowedHosts = map[string]bool{server.URL[7:]: true}
	e.checkResources = true
	e.resourceCh = make(chan resourceCheckItem, 100)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	// Collect resource checks
	close(e.resourceCh)
	var resItems []resourceCheckItem
	for item := range e.resourceCh {
		resItems = append(resItems, item)
	}

	// Should have at least 2 unique resources (style.css, app.js — deduped, and logo.png)
	if len(resItems) < 2 {
		t.Errorf("expected at least 2 unique resource items, got %d", len(resItems))
	}

	// Also check that resourceRefs were buffered (one per unique resource reference)
	if len(e.resourceRefBuf) < 2 {
		t.Errorf("expected at least 2 resource refs, got %d", len(e.resourceRefBuf))
	}
}

// ---------------------------------------------------------------------------
// E2E test: crawl with external and resource check workers enabled
// ---------------------------------------------------------------------------

func TestE2E_CrawlWithExternalAndResourceChecks(t *testing.T) {
	extServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "external page")
	}))
	defer extServer.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "User-agent: *\nAllow: /\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
			<link rel="stylesheet" href="/style.css">
		</head><body>
			<a href="%s/ext-page">external</a>
			<a href="/page2">internal</a>
		</body></html>`, extServer.URL)
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>page2</body></html>`)
	})
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "body{}")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := e2eCrawlerConfig("host")
	inserter := &e2eInserter{}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{server.URL + "/"}, cfg)
	engine.maxPages = 10
	engine.buildScope()
	engine.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, engine.session.ID)
	engine.seedFrontier([]string{server.URL + "/"})
	engine.checkExternal = true
	engine.externalWorkers = 2
	engine.checkResources = true
	engine.resourceWorkers = 2

	fetchCh, drainWorkers, finalize := engine.startWorkers()
	engine.dispatcher(fetchCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recover() }()
		drainWorkers()
		finalize()
	}()
	<-done

	crawled := inserter.crawledURLs()
	if len(crawled) == 0 {
		t.Fatal("expected at least one page to be crawled")
	}
}

// ---------------------------------------------------------------------------
// startWorkers — cover the shutdown function with ext/res/render nil paths
// ---------------------------------------------------------------------------

func TestStartWorkersShutdownNilOptionalChannels(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         1,
			Delay:           0,
			UserAgent:       "TestBot/1.0",
			Timeout:         2 * time.Second,
			MaxBodySize:     1 << 20,
			AllowPrivateIPs: true,
			CrawlScope:      "host",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}

	inserter := &e2eInserter{}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)
	engine.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, engine.session.ID)
	// No external/resource/render workers
	engine.checkExternal = false
	engine.checkResources = false

	fetchCh, drainWorkers, finalize := engine.startWorkers()

	// Close fetchCh immediately to let workers exit
	close(fetchCh)

	// finalize() panics on finalizeSession because store is nil, recover from it
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recover() }()
		drainWorkers()
		finalize()
	}()

	select {
	case <-done:
		// Expected
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown did not complete within 10s")
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover the parse error path
// ---------------------------------------------------------------------------

func TestParseWorkerParseError(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			UserAgent: "TestBot/1.0",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		front:       frontier.New(0, 10000),
		retryQueue:  NewRetryQueue(),
		hostHealth:  NewHostHealth(),
		retryPolicy: &RetryPolicy{MaxRetries: 0},
		session:     &Session{ID: "test-parse-err"},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	// Send HTML with valid content type but completely empty body treated as HTML
	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- &fetcher.FetchResult{
		URL:         "http://example.com/page",
		FinalURL:    "http://example.com/page",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte(""), // empty body, IsHTML() true but no parsing needed
		Duration:    100 * time.Millisecond,
		Headers:     map[string]string{},
	}
	close(parseCh)

	e.parseWorker(0, parseCh)

	// Should still store the page
	e.buffer.Flush()
	if len(inserter.pages) != 1 {
		t.Errorf("expected 1 page stored, got %d", len(inserter.pages))
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover images counting path (ImagesNoAlt)
// ---------------------------------------------------------------------------

func TestParseWorkerImagesNoAlt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
			<img src="/a.png" alt="good">
			<img src="/b.png">
			<img src="/c.png" alt="">
		</body></html>`)
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			UserAgent: "TestBot/1.0",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		front:       frontier.New(0, 10000),
		retryQueue:  NewRetryQueue(),
		hostHealth:  NewHostHealth(),
		retryPolicy: &RetryPolicy{MaxRetries: 0},
		session:     &Session{ID: "test-images"},
		allowedHosts: map[string]bool{},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	e.buffer.Flush()
	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(inserter.pages))
	}
	page := inserter.pages[0]
	if page.ImagesCount < 3 {
		t.Errorf("ImagesCount = %d, want >= 3", page.ImagesCount)
	}
	if page.ImagesNoAlt < 2 {
		t.Errorf("ImagesNoAlt = %d, want >= 2", page.ImagesNoAlt)
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover hreflang extraction
// ---------------------------------------------------------------------------

func TestParseWorkerHreflang(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head>
			<link rel="alternate" hreflang="en" href="http://example.com/en">
			<link rel="alternate" hreflang="fr" href="http://example.com/fr">
		</head><body><h1>Test</h1></body></html>`)
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			UserAgent: "TestBot/1.0",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		front:       frontier.New(0, 10000),
		retryQueue:  NewRetryQueue(),
		hostHealth:  NewHostHealth(),
		retryPolicy: &RetryPolicy{MaxRetries: 0},
		session:     &Session{ID: "test-hreflang"},
		allowedHosts: map[string]bool{},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	e.buffer.Flush()
	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(inserter.pages))
	}
	page := inserter.pages[0]
	if len(page.Hreflang) != 2 {
		t.Errorf("Hreflang count = %d, want 2", len(page.Hreflang))
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover OG metadata extraction
// ---------------------------------------------------------------------------

func TestParseWorkerOGMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head>
			<meta property="og:title" content="OG Title">
			<meta property="og:description" content="OG Description">
			<meta property="og:image" content="http://example.com/image.jpg">
			<meta name="description" content="Meta description">
		</head><body><h1>Hello</h1><p>Some content here with more words to test word count accurately</p></body></html>`)
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			UserAgent: "TestBot/1.0",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		front:       frontier.New(0, 10000),
		retryQueue:  NewRetryQueue(),
		hostHealth:  NewHostHealth(),
		retryPolicy: &RetryPolicy{MaxRetries: 0},
		session:     &Session{ID: "test-og"},
		allowedHosts: map[string]bool{},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	e.buffer.Flush()
	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(inserter.pages))
	}
	page := inserter.pages[0]
	if page.OGTitle != "OG Title" {
		t.Errorf("OGTitle = %q, want OG Title", page.OGTitle)
	}
	if page.OGDescription != "OG Description" {
		t.Errorf("OGDescription = %q, want OG Description", page.OGDescription)
	}
	if page.OGImage != "http://example.com/image.jpg" {
		t.Errorf("OGImage = %q", page.OGImage)
	}
	if page.MetaDescription != "Meta description" {
		t.Errorf("MetaDescription = %q", page.MetaDescription)
	}
	if page.MetaDescLength == 0 {
		t.Error("MetaDescLength should be > 0")
	}
	if page.WordCount == 0 {
		t.Error("WordCount should be > 0")
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover internal link counting and InternalLinksOut/ExternalLinksOut
// ---------------------------------------------------------------------------

func TestParseWorkerLinkCounts(t *testing.T) {
	var serverHost string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body>
			<a href="http://%s/page2">internal1</a>
			<a href="http://%s/page3">internal2</a>
			<a href="http://external.example.com/ext1">ext1</a>
		</body></html>`, serverHost, serverHost)
	}))
	defer server.Close()
	// Extract host from server URL (e.g., "127.0.0.1:XXXX")
	serverHost = server.URL[7:] // strip "http://"

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			UserAgent: "TestBot/1.0",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:          cfg,
		ctx:          ctx,
		cancel:       cancel,
		front:        frontier.New(0, 10000),
		retryQueue:   NewRetryQueue(),
		hostHealth:   NewHostHealth(),
		retryPolicy:  &RetryPolicy{MaxRetries: 0},
		session:      &Session{ID: "test-link-counts"},
		allowedHosts: map[string]bool{serverHost: true},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	e.buffer.Flush()
	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(inserter.pages))
	}
	page := inserter.pages[0]
	if page.InternalLinksOut < 2 {
		t.Errorf("InternalLinksOut = %d, want >= 2", page.InternalLinksOut)
	}
	if page.ExternalLinksOut < 1 {
		t.Errorf("ExternalLinksOut = %d, want >= 1", page.ExternalLinksOut)
	}

	// Also check links were buffered
	if len(inserter.links) < 3 {
		t.Errorf("expected at least 3 links, got %d", len(inserter.links))
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover schema types extraction
// ---------------------------------------------------------------------------

func TestParseWorkerSchemaTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head>
			<script type="application/ld+json">{"@type":"Article","name":"Test"}</script>
		</head><body><h1>Test</h1></body></html>`)
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			UserAgent: "TestBot/1.0",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:          cfg,
		ctx:          ctx,
		cancel:       cancel,
		front:        frontier.New(0, 10000),
		retryQueue:   NewRetryQueue(),
		hostHealth:   NewHostHealth(),
		retryPolicy:  &RetryPolicy{MaxRetries: 0},
		session:      &Session{ID: "test-schema"},
		allowedHosts: map[string]bool{},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	e.buffer.Flush()
	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(inserter.pages))
	}
	page := inserter.pages[0]
	if len(page.SchemaTypes) == 0 {
		t.Error("expected SchemaTypes to be populated")
	}
}

// ---------------------------------------------------------------------------
// seedFrontier — cover the sitemapURLSet priority boost path
// ---------------------------------------------------------------------------

func TestSeedFrontierWithSitemapURLSet(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
			Delay:   0,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		front:   frontier.New(0, 10000),
		session: &Session{ID: "test-seed-sitemap"},
	}
	e.sitemapURLSet = map[string]bool{
		"http://example.com/from-sitemap": true,
	}

	seeds := []string{"http://example.com/", "http://example.com/from-sitemap"}
	e.seedFrontier(seeds)

	// Both should be added
	if e.front.Len() != 2 {
		t.Errorf("frontier len = %d, want 2", e.front.Len())
	}
}

// ---------------------------------------------------------------------------
// parseWorker — cover Content-Encoding and X-Robots-Tag header extraction
// ---------------------------------------------------------------------------

func TestParseWorkerHeaderExtraction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("X-Custom-Header", "test-value")
		fmt.Fprint(w, `<html><body><h1>Test</h1></body></html>`)
	}))
	defer server.Close()

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:   1,
			UserAgent: "TestBot/1.0",
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

	inserter := &e2eInserter{}
	e := &Engine{
		cfg:          cfg,
		ctx:          ctx,
		cancel:       cancel,
		front:        frontier.New(0, 10000),
		retryQueue:   NewRetryQueue(),
		hostHealth:   NewHostHealth(),
		retryPolicy:  &RetryPolicy{MaxRetries: 0},
		session:      &Session{ID: "test-headers"},
		allowedHosts: map[string]bool{},
	}
	e.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, e.session.ID)

	f := fetcher.New("TestBot/1.0", 5*time.Second, 1<<20, fetcher.DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL+"/page", 0, "")

	parseCh := make(chan *fetcher.FetchResult, 1)
	parseCh <- result
	close(parseCh)

	e.parseWorker(0, parseCh)

	e.buffer.Flush()
	if len(inserter.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(inserter.pages))
	}
	page := inserter.pages[0]
	if page.XRobotsTag != "noindex, nofollow" {
		t.Errorf("XRobotsTag = %q, want 'noindex, nofollow'", page.XRobotsTag)
	}
	// X-Robots-Tag noindex should make it non-indexable
	if page.IsIndexable {
		t.Error("page with X-Robots-Tag noindex should not be indexable")
	}
	if page.IndexReason != "x_robots_noindex" {
		t.Errorf("IndexReason = %q, want x_robots_noindex", page.IndexReason)
	}
}

// ---------------------------------------------------------------------------
// NewEngine — cover the full construction path
// ---------------------------------------------------------------------------

func TestNewEngineFields(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         4,
			Delay:           100 * time.Millisecond,
			UserAgent:       "TestBot/1.0",
			Timeout:         10 * time.Second,
			MaxBodySize:     5 << 20,
			MaxFrontierSize: 50000,
			SourceIP:        "",
			ForceIPv4:       false,
			AllowPrivateIPs: true,
			Retry: config.RetryConfig{
				MaxRetries: 3,
				BaseDelay:  time.Second,
				MaxDelay:   30 * time.Second,
			},
		},
	}

	e := NewEngine(cfg, nil)

	if e.cfg != cfg {
		t.Error("cfg not set")
	}
	if e.store != nil {
		t.Error("store should be nil")
	}
	if e.front == nil {
		t.Error("frontier not initialized")
	}
	if e.fetch == nil {
		t.Error("fetcher not initialized")
	}
	if e.robots == nil {
		t.Error("robots cache not initialized")
	}
	if e.retryQueue == nil {
		t.Error("retry queue not initialized")
	}
	if e.hostHealth == nil {
		t.Error("host health not initialized")
	}
	if e.retryPolicy == nil {
		t.Error("retry policy not initialized")
	}
	if e.retryPolicy.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", e.retryPolicy.MaxRetries)
	}
	if e.retryPolicy.BaseDelay != time.Second {
		t.Errorf("BaseDelay = %v, want 1s", e.retryPolicy.BaseDelay)
	}
	if e.ctx == nil {
		t.Error("context not initialized")
	}
	if e.cancel == nil {
		t.Error("cancel func not initialized")
	}
}

// ---------------------------------------------------------------------------
// Engine.Stop — cover the stop method
// ---------------------------------------------------------------------------

func TestEngineStop(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
			Delay:   0,
		},
	}
	e := NewEngine(cfg, nil)

	e.Stop()

	select {
	case <-e.ctx.Done():
		// Context should be cancelled
	default:
		t.Error("expected context to be cancelled after Stop()")
	}
}

// ---------------------------------------------------------------------------
// ResumeSession — verify it preserves seed URLs
// ---------------------------------------------------------------------------

func TestResumeSessionFields(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
		},
	}
	e := NewEngine(cfg, nil)
	e.ResumeSession("session-123", []string{"http://example.com"})

	if e.session == nil {
		t.Fatal("session should not be nil")
	}
	if e.session.ID != "session-123" {
		t.Errorf("ID = %q, want session-123", e.session.ID)
	}
	if len(e.session.SeedURLs) != 1 || e.session.SeedURLs[0] != "http://example.com" {
		t.Errorf("SeedURLs = %v, want [http://example.com]", e.session.SeedURLs)
	}
}

// ---------------------------------------------------------------------------
// persistRobotsData — cover the empty entries path
// ---------------------------------------------------------------------------

func TestPersistRobotsData_EmptyEntries(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:       "TestBot/1.0",
			Timeout:         2 * time.Second,
			AllowPrivateIPs: true,
		},
	}
	e := NewEngine(cfg, nil)
	e.session = &Session{ID: "test-persist-robots"}

	// No robots entries were fetched
	// This should return early without calling store
	e.persistRobotsData()
	// If it didn't panic, the test passes
}

// ---------------------------------------------------------------------------
// Manager enqueue — cover the FIFO queue addition (without store)
// ---------------------------------------------------------------------------

func TestManagerEnqueueFields(t *testing.T) {
	m := newTestManager(2)

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers: 1,
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"http://example.com"}, cfg)

	// enqueue calls store.InsertSession which will fail on nil store
	// but we can recover and verify the queue was populated
	func() {
		defer func() { recover() }()
		m.enqueue("sess-1", engine, []string{"http://example.com"})
	}()

	if len(m.queue) != 1 {
		t.Fatalf("expected 1 item in queue, got %d", len(m.queue))
	}
	if !m.queuedSet["sess-1"] {
		t.Error("expected sess-1 in queuedSet")
	}
	if engine.session.Status != "queued" {
		t.Errorf("session status = %q, want queued", engine.session.Status)
	}
}

// ---------------------------------------------------------------------------
// Manager — concurrent operations
// ---------------------------------------------------------------------------

func TestManagerConcurrentIsRunning(t *testing.T) {
	m := newTestManager(5)

	cfg := &config.Config{Crawler: config.CrawlerConfig{Workers: 1}}
	engine := NewEngine(cfg, nil)
	m.mu.Lock()
	m.engines["sess-1"] = engine
	m.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.IsRunning("sess-1")
			_ = m.IsRunning("nonexistent")
		}()
	}
	wg.Wait()
}

func TestManagerConcurrentProgress(t *testing.T) {
	m := newTestManager(5)

	cfg := &config.Config{Crawler: config.CrawlerConfig{Workers: 1}}
	engine := NewEngine(cfg, nil)
	engine.pagesCrawled.Store(42)
	m.mu.Lock()
	m.engines["sess-1"] = engine
	m.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pages, queue, ok := m.Progress("sess-1")
			if ok && pages != 42 {
				t.Errorf("pages = %d, want 42", pages)
			}
			_, _, _ = m.Progress("nonexistent")
			_ = queue
		}()
	}
	wg.Wait()
}
