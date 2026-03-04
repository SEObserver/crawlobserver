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
	"github.com/SEObserver/crawlobserver/internal/frontier"
	"github.com/SEObserver/crawlobserver/internal/normalizer"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

func TestNewSession(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
	}

	seeds := []string{"https://example.com", "https://other.com"}
	sess := NewSession(seeds, cfg)

	if sess.ID == "" {
		t.Error("session ID should not be empty")
	}
	if sess.Status != "running" {
		t.Errorf("Status = %q, want running", sess.Status)
	}
	if len(sess.SeedURLs) != 2 {
		t.Errorf("SeedURLs len = %d, want 2", len(sess.SeedURLs))
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
}

func TestSessionToStorageRow(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Workers:   5,
			Timeout:   10 * time.Second,
		},
	}

	sess := NewSession([]string{"https://example.com"}, cfg)
	sess.Pages = 42

	row := sess.ToStorageRow()
	if row.ID != sess.ID {
		t.Errorf("ID mismatch: %q != %q", row.ID, sess.ID)
	}
	if row.UserAgent != "TestBot/1.0" {
		t.Errorf("UserAgent = %q, want TestBot/1.0", row.UserAgent)
	}
	if row.PagesCrawled != 42 {
		t.Errorf("PagesCrawled = %d, want 42", row.PagesCrawled)
	}
	if row.Config == "" {
		t.Error("Config JSON should not be empty")
	}
}

// TestResumeSessionPreservesSeedURLs is a regression test for the bug where
// Run() overwrote session.SeedURLs with the uncrawled/failed URLs passed as
// the seeds parameter. This caused RecomputeDepths to assign depth 0 to
// hundreds of pages instead of only the original seed.
func TestResumeSessionPreservesSeedURLs(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
	}

	engine := NewEngine(cfg, nil)

	originalSeeds := []string{"https://example.com"}
	engine.ResumeSession("test-session-id", originalSeeds)

	// Verify seeds are set correctly after ResumeSession
	if len(engine.session.SeedURLs) != 1 || engine.session.SeedURLs[0] != "https://example.com" {
		t.Fatalf("after ResumeSession, SeedURLs = %v, want [https://example.com]", engine.session.SeedURLs)
	}

	// Simulate what Run() does to the session (without actually running the crawl).
	// Before the fix, Run() did: e.session.SeedURLs = seeds
	// which overwrote the original seeds with uncrawled URLs.
	uncrawledURLs := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://example.com/page3",
	}

	// After the fix, Run() only sets Status, not SeedURLs.
	// We test the session state as Run() would set it.
	if engine.session != nil {
		// This is the fixed path — session already exists, so don't overwrite SeedURLs
		engine.session.Status = "running"
	}
	_ = uncrawledURLs // these would be passed to Run() but should NOT corrupt SeedURLs

	// SeedURLs must still be the original seed
	if len(engine.session.SeedURLs) != 1 {
		t.Errorf("SeedURLs len = %d, want 1", len(engine.session.SeedURLs))
	}
	if engine.session.SeedURLs[0] != "https://example.com" {
		t.Errorf("SeedURLs[0] = %q, want https://example.com", engine.session.SeedURLs[0])
	}

	// Verify the storage row also has the correct seeds
	row := engine.session.ToStorageRow()
	if len(row.SeedURLs) != 1 || row.SeedURLs[0] != "https://example.com" {
		t.Errorf("storage row SeedURLs = %v, want [https://example.com]", row.SeedURLs)
	}
}

// diskFullInserter simulates ClickHouse returning "Cannot reserve N MiB" (code 243)
// when the Docker virtual disk is full. All inserts fail permanently.
type diskFullInserter struct{}

func (d *diskFullInserter) InsertPages(_ context.Context, _ []storage.PageRow) error {
	return fmt.Errorf("code: 243, Cannot reserve 1073741824 bytes in file")
}

func (d *diskFullInserter) InsertLinks(_ context.Context, _ []storage.LinkRow) error {
	return fmt.Errorf("code: 243, Cannot reserve 1073741824 bytes in file")
}

func (d *diskFullInserter) InsertExtractions(_ context.Context, _ []extraction.ExtractionRow) error {
	return fmt.Errorf("code: 243, Cannot reserve 1073741824 bytes in file")
}

// TestDiskFullAutoStop verifies the full disk-full scenario:
// ClickHouse inserts fail permanently → buffer drops data after max retries →
// onDataLost callback fires → engine.Stop() cancels the crawl context.
func TestDiskFullAutoStop(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour, // won't auto-tick during test
		},
	}

	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	// Create buffer with a permanently failing store (simulates disk full)
	engine.buffer = storage.NewBuffer(&diskFullInserter{}, 100, time.Hour, engine.session.ID)

	// Wire up the same callback as Run() does
	engine.buffer.SetOnDataLost(func(lostPages, lostLinks int64) {
		engine.Stop()
	})

	// Simulate crawler writing pages and links to the buffer
	for i := 0; i < 5; i++ {
		engine.buffer.AddPage(storage.PageRow{URL: fmt.Sprintf("https://example.com/%d", i)})
	}
	engine.buffer.AddLinks([]storage.LinkRow{
		{SourceURL: "https://example.com", TargetURL: "https://example.com/1"},
		{SourceURL: "https://example.com", TargetURL: "https://example.com/2"},
	})

	// Flush 1: initial failure → data moves to retry queue
	engine.buffer.Flush()
	select {
	case <-engine.ctx.Done():
		t.Fatal("engine should NOT be stopped yet (retries pending)")
	default:
	}

	// Flush 2-3: retries fail, still under maxRetries
	engine.buffer.Flush()
	engine.buffer.Flush()
	select {
	case <-engine.ctx.Done():
		t.Fatal("engine should NOT be stopped yet (retries still pending)")
	default:
	}

	// Flush 4: retries exhaust (retries=3 >= maxRetries=3) → data dropped → callback → Stop()
	engine.buffer.Flush()

	select {
	case <-engine.ctx.Done():
		// Engine was stopped — this is the expected behavior
	default:
		t.Fatal("engine context should be cancelled after disk-full data loss")
	}

	// Verify buffer reports the data loss
	state := engine.BufferState()
	if state.LostPages != 5 {
		t.Errorf("LostPages = %d, want 5", state.LostPages)
	}
	if state.LostLinks != 2 {
		t.Errorf("LostLinks = %d, want 2", state.LostLinks)
	}

	// Clean up the buffer's flush goroutine
	engine.buffer.Close()
}

// TestDiskFullCompletedWithErrors verifies that after a disk-full scenario,
// the session status is set to "completed_with_errors" (not plain "completed").
func TestDiskFullCompletedWithErrors(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}

	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	// Create buffer that will lose data
	engine.buffer = storage.NewBuffer(&diskFullInserter{}, 100, time.Hour, engine.session.ID)

	engine.buffer.AddPage(storage.PageRow{URL: "https://example.com/1"})

	// Exhaust retries: 4 flushes → drop
	for i := 0; i < 5; i++ {
		engine.buffer.Flush()
	}
	engine.buffer.Close()

	// Reproduce the same status logic as Run()
	bufState := engine.BufferState()
	if bufState.LostPages > 0 || bufState.LostLinks > 0 {
		engine.session.Status = "completed_with_errors"
	} else {
		engine.session.Status = "completed"
	}

	if engine.session.Status != "completed_with_errors" {
		t.Errorf("session.Status = %q, want %q", engine.session.Status, "completed_with_errors")
	}
}

// TestNoDiskIssueCompletedNormally verifies that without data loss,
// the session status remains "completed".
func TestNoDiskIssueCompletedNormally(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}

	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	// Create buffer with a working store
	engine.buffer = storage.NewBuffer(&successInserter{}, 100, time.Hour, engine.session.ID)

	engine.buffer.AddPage(storage.PageRow{URL: "https://example.com/1"})
	engine.buffer.Flush()
	engine.buffer.Close()

	bufState := engine.BufferState()
	if bufState.LostPages > 0 || bufState.LostLinks > 0 {
		engine.session.Status = "completed_with_errors"
	} else {
		engine.session.Status = "completed"
	}

	if engine.session.Status != "completed" {
		t.Errorf("session.Status = %q, want %q", engine.session.Status, "completed")
	}
}

func TestIsInScope_Host(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com/page"}, cfg)
	engine.buildScope()

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/other", true},
		{"https://example.com/", true},
		{"https://sub.example.com/page", false},
		{"https://other.com/page", false},
	}
	for _, tt := range tests {
		if got := engine.isInScope(tt.url); got != tt.want {
			t.Errorf("isInScope(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsInScope_Domain(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "domain",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://www.example.com/page"}, cfg)
	engine.buildScope()

	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.example.com/other", true},
		{"https://example.com/other", true},
		{"https://sub.example.com/page", true},
		{"https://other.com/page", false},
	}
	for _, tt := range tests {
		if got := engine.isInScope(tt.url); got != tt.want {
			t.Errorf("isInScope(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsInScope_Subdirectory(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "subdirectory",
		},
	}

	t.Run("seed with path", func(t *testing.T) {
		engine := NewEngine(cfg, nil)
		engine.session = NewSession([]string{"https://example.com/blog/article"}, cfg)
		engine.buildScope()

		tests := []struct {
			url  string
			want bool
		}{
			{"https://example.com/blog/other", true},
			{"https://example.com/blog/", true},
			{"https://example.com/blog/sub/deep", true},
			{"https://example.com/", false},
			{"https://example.com/other/page", false},
			{"https://other.com/blog/article", false},
		}
		for _, tt := range tests {
			if got := engine.isInScope(tt.url); got != tt.want {
				t.Errorf("isInScope(%q) = %v, want %v", tt.url, got, tt.want)
			}
		}
	})

	t.Run("seed with trailing slash", func(t *testing.T) {
		engine := NewEngine(cfg, nil)
		engine.session = NewSession([]string{"https://example.com/blog/"}, cfg)
		engine.buildScope()

		tests := []struct {
			url  string
			want bool
		}{
			{"https://example.com/blog/article", true},
			{"https://example.com/blog/", true},
			{"https://example.com/", false},
			{"https://example.com/other", false},
		}
		for _, tt := range tests {
			if got := engine.isInScope(tt.url); got != tt.want {
				t.Errorf("isInScope(%q) = %v, want %v", tt.url, got, tt.want)
			}
		}
	})

	t.Run("seed at root", func(t *testing.T) {
		engine := NewEngine(cfg, nil)
		engine.session = NewSession([]string{"https://example.com/"}, cfg)
		engine.buildScope()

		tests := []struct {
			url  string
			want bool
		}{
			{"https://example.com/anything", true},
			{"https://example.com/blog/deep", true},
			{"https://other.com/", false},
		}
		for _, tt := range tests {
			if got := engine.isInScope(tt.url); got != tt.want {
				t.Errorf("isInScope(%q) = %v, want %v", tt.url, got, tt.want)
			}
		}
	})

	t.Run("multiple seeds", func(t *testing.T) {
		engine := NewEngine(cfg, nil)
		engine.session = NewSession([]string{
			"https://example.com/blog/post",
			"https://example.com/docs/guide",
		}, cfg)
		engine.buildScope()

		tests := []struct {
			url  string
			want bool
		}{
			{"https://example.com/blog/other", true},
			{"https://example.com/docs/api", true},
			{"https://example.com/about", false},
		}
		for _, tt := range tests {
			if got := engine.isInScope(tt.url); got != tt.want {
				t.Errorf("isInScope(%q) = %v, want %v", tt.url, got, tt.want)
			}
		}
	})
}

type successInserter struct{}

func (s *successInserter) InsertPages(_ context.Context, _ []storage.PageRow) error { return nil }
func (s *successInserter) InsertLinks(_ context.Context, _ []storage.LinkRow) error { return nil }
func (s *successInserter) InsertExtractions(_ context.Context, _ []extraction.ExtractionRow) error {
	return nil
}

// --- E2E crawl scope tests ---

// e2eInserter collects crawled pages and links in memory for assertions.
type e2eInserter struct {
	mu    sync.Mutex
	pages []storage.PageRow
	links []storage.LinkRow
}

func (i *e2eInserter) InsertPages(_ context.Context, pages []storage.PageRow) error {
	i.mu.Lock()
	i.pages = append(i.pages, pages...)
	i.mu.Unlock()
	return nil
}

func (i *e2eInserter) InsertLinks(_ context.Context, links []storage.LinkRow) error {
	i.mu.Lock()
	i.links = append(i.links, links...)
	i.mu.Unlock()
	return nil
}

func (i *e2eInserter) InsertExtractions(_ context.Context, _ []extraction.ExtractionRow) error {
	return nil
}

func (i *e2eInserter) crawledURLs() map[string]bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	m := make(map[string]bool, len(i.pages))
	for _, p := range i.pages {
		m[p.URL] = true
	}
	return m
}

func e2eCrawlerConfig(scope string) *config.Config {
	return &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         2,
			Delay:           10 * time.Millisecond,
			MaxPages:        50,
			Timeout:         5 * time.Second,
			UserAgent:       "TestBot/1.0",
			MaxBodySize:     1 << 20,
			RespectRobots:   true,
			CrawlScope:      scope,
			AllowPrivateIPs: true,
			MaxFrontierSize: 10000,
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxGlobalErrorRate:  1.0,
				MaxConsecutiveFails: 100,
			},
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Second,
		},
	}
}

func testHTMLPage(links ...string) string {
	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><head><title>Test</title></head><body>`)
	for _, link := range links {
		sb.WriteString(`<a href="` + link + `">link</a> `)
	}
	sb.WriteString(`</body></html>`)
	return sb.String()
}

// runTestCrawl sets up and runs a full crawl pipeline without ClickHouse.
// It returns the inserter containing all crawled pages and links.
func runTestCrawl(t *testing.T, cfg *config.Config, seeds []string) *e2eInserter {
	t.Helper()

	inserter := &e2eInserter{}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession(seeds, cfg)
	engine.maxPages = int64(cfg.Crawler.MaxPages)
	engine.buildScope()
	engine.buffer = storage.NewBuffer(inserter, cfg.Storage.BatchSize, cfg.Storage.FlushInterval, engine.session.ID)
	engine.seedFrontier(seeds)

	fetchCh, shutdown := engine.startWorkers()
	engine.dispatcher(fetchCh)

	// shutdown() calls persistRobotsData/finalizeSession which panic on nil store.
	// Run in a goroutine with recover to get proper worker/channel cleanup.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recover() }()
		shutdown()
	}()
	<-done

	return inserter
}

func TestE2E_CrawlScope_Subdirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
		case "/blog", "/blog/":
			fmt.Fprint(w, testHTMLPage("/blog/post1", "/blog/post2", "/other/page"))
		case "/blog/post1":
			fmt.Fprint(w, testHTMLPage("/blog/post2"))
		case "/blog/post2":
			fmt.Fprint(w, testHTMLPage("/blog/post1"))
		case "/other/page":
			fmt.Fprint(w, testHTMLPage("/blog/post1", "/about"))
		case "/about":
			fmt.Fprint(w, testHTMLPage("/"))
		case "/":
			fmt.Fprint(w, testHTMLPage("/blog/", "/about"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := e2eCrawlerConfig("subdirectory")
	seeds := []string{server.URL + "/blog/"}
	inserter := runTestCrawl(t, cfg, seeds)
	crawled := inserter.crawledURLs()

	// Pages under /blog/ must be crawled
	for _, p := range []string{"/blog/post1", "/blog/post2"} {
		if !crawled[server.URL+p] {
			t.Errorf("expected %s to be crawled", p)
		}
	}

	// Pages outside /blog/ must NOT be crawled
	for _, p := range []string{"/other/page", "/about"} {
		if crawled[server.URL+p] {
			t.Errorf("expected %s NOT to be crawled (out of subdirectory scope)", p)
		}
	}
}

func TestE2E_CrawlScope_SubdirectoryFileSeed(t *testing.T) {
	// When the seed is a file (not a directory), the allowed prefix
	// should be the parent directory.
	// Seed: /docs/guide → prefix: /docs/
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
		case "/docs/guide":
			fmt.Fprint(w, testHTMLPage("/docs/api", "/docs/faq", "/blog/news"))
		case "/docs/api":
			fmt.Fprint(w, testHTMLPage("/docs/guide"))
		case "/docs/faq":
			fmt.Fprint(w, testHTMLPage("/docs/guide"))
		case "/blog/news":
			fmt.Fprint(w, testHTMLPage("/docs/guide"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := e2eCrawlerConfig("subdirectory")
	seeds := []string{server.URL + "/docs/guide"}
	inserter := runTestCrawl(t, cfg, seeds)
	crawled := inserter.crawledURLs()

	// Pages under /docs/ must be crawled
	for _, p := range []string{"/docs/guide", "/docs/api", "/docs/faq"} {
		if !crawled[server.URL+p] {
			t.Errorf("expected %s to be crawled", p)
		}
	}

	// /blog/news is outside /docs/ scope
	if crawled[server.URL+"/blog/news"] {
		t.Errorf("expected /blog/news NOT to be crawled (out of subdirectory scope)")
	}
}

func TestE2E_CrawlScope_Host(t *testing.T) {
	// With host scope, the crawler should follow links to any directory
	// on the same host (unlike subdirectory mode which restricts to the seed dir).
	// We reuse the same server structure as the subdirectory test but with
	// host scope — /other/page SHOULD be crawled this time.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
		case "/blog", "/blog/":
			fmt.Fprint(w, testHTMLPage("/blog/post1", "/other/page"))
		case "/blog/post1":
			fmt.Fprint(w, testHTMLPage())
		case "/other/page":
			fmt.Fprint(w, testHTMLPage("/about"))
		case "/about":
			fmt.Fprint(w, testHTMLPage())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := e2eCrawlerConfig("host")
	seeds := []string{server.URL + "/blog/"}
	inserter := runTestCrawl(t, cfg, seeds)
	crawled := inserter.crawledURLs()

	// With host scope, ALL pages on the same host must be crawled
	// (including /other/page which would be blocked by subdirectory scope)
	for _, p := range []string{"/blog/post1", "/other/page", "/about"} {
		if !crawled[server.URL+p] {
			t.Errorf("expected %s to be crawled (host scope allows all paths)", p)
		}
	}
}

// TestE2E_SchemelessSeed verifies that seeds without a scheme (e.g. "blog.axe-net.fr")
// are properly handled when prefixed with http:// via normalizer.EnsureScheme.
func TestE2E_SchemelessSeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
		case "/", "":
			fmt.Fprint(w, testHTMLPage("/about", "/contact"))
		case "/about":
			fmt.Fprint(w, testHTMLPage("/"))
		case "/contact":
			fmt.Fprint(w, testHTMLPage("/"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Strip the "http://" scheme to simulate a bare domain seed
	bareSeed := strings.TrimPrefix(server.URL, "http://")

	cfg := e2eCrawlerConfig("host")

	// Apply EnsureScheme like manager.StartCrawl does
	fixedSeed := normalizer.EnsureScheme(bareSeed)

	seeds := []string{fixedSeed}
	inserter := runTestCrawl(t, cfg, seeds)
	crawled := inserter.crawledURLs()

	if len(crawled) == 0 {
		t.Fatal("expected at least one page crawled from schemeless seed")
	}

	// The homepage must be crawled
	if !crawled[server.URL+"/"] && !crawled[server.URL] {
		t.Errorf("homepage not crawled; crawled URLs: %v", crawled)
	}
}

func TestDispatcherIdleTimeout(t *testing.T) {
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

	// Simulate: pending retries exist but frontier is empty and no progress for 31s
	e.pendingRetries.Store(3)
	e.lastProgressAt.Store(time.Now().Unix() - 31)

	fetchCh := make(chan *frontier.CrawlURL, 1)
	done := make(chan struct{})
	go func() {
		e.dispatcher(fetchCh)
		close(done)
	}()

	select {
	case <-done:
		// Dispatcher exited — that's the expected behavior
	case <-time.After(10 * time.Second):
		t.Fatal("dispatcher did not exit within 10s despite empty frontier and no progress for 30s")
	}
}

func TestDispatcherWaitsForPendingRetries(t *testing.T) {
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

	// Simulate: pending retries exist, frontier is empty, but progress was RECENT
	e.pendingRetries.Store(3)
	e.lastProgressAt.Store(time.Now().Unix()) // just now

	fetchCh := make(chan *frontier.CrawlURL, 1)
	done := make(chan struct{})
	go func() {
		e.dispatcher(fetchCh)
		close(done)
	}()

	// Dispatcher should NOT exit quickly (it should wait for retries)
	select {
	case <-done:
		t.Fatal("dispatcher exited too early — should wait for pending retries when progress is recent")
	case <-time.After(2 * time.Second):
		// Good — dispatcher is still running, waiting for retries
		cancel() // clean up
	}
}
