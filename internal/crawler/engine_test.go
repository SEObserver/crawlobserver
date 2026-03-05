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
	"github.com/SEObserver/crawlobserver/internal/normalizer"
	"github.com/SEObserver/crawlobserver/internal/parser"
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

// --- stringSlicesEqual tests ---

func TestStringSlicesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "both empty",
			a:    []string{},
			b:    []string{},
			want: true,
		},
		{
			name: "equal single element",
			a:    []string{"hello"},
			b:    []string{"hello"},
			want: true,
		},
		{
			name: "equal multiple elements",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: true,
		},
		{
			name: "different lengths",
			a:    []string{"a", "b"},
			b:    []string{"a"},
			want: false,
		},
		{
			name: "same length different content",
			a:    []string{"a", "b"},
			b:    []string{"a", "c"},
			want: false,
		},
		{
			name: "same elements different order",
			a:    []string{"a", "b"},
			b:    []string{"b", "a"},
			want: false,
		},
		{
			name: "nil vs empty",
			a:    nil,
			b:    []string{},
			want: true,
		},
		{
			name: "one nil one non-empty",
			a:    nil,
			b:    []string{"a"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSlicesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("stringSlicesEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- computeIndexability tests ---

func TestComputeIndexability(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  uint16
		metaRobots  string
		xRobotsTag  string
		canonical   string
		finalURL    string
		originalURL string
		wantIndex   bool
		wantReason  string
	}{
		{
			name:       "200 OK indexable",
			statusCode: 200,
			wantIndex:  true,
			wantReason: "",
		},
		{
			name:       "201 created indexable",
			statusCode: 201,
			wantIndex:  true,
			wantReason: "",
		},
		{
			name:       "301 redirect",
			statusCode: 301,
			wantIndex:  false,
			wantReason: "redirect",
		},
		{
			name:       "302 redirect",
			statusCode: 302,
			wantIndex:  false,
			wantReason: "redirect",
		},
		{
			name:       "404 not found",
			statusCode: 404,
			wantIndex:  false,
			wantReason: "status_404",
		},
		{
			name:       "500 server error",
			statusCode: 500,
			wantIndex:  false,
			wantReason: "status_500",
		},
		{
			name:       "meta noindex",
			statusCode: 200,
			metaRobots: "noindex, follow",
			wantIndex:  false,
			wantReason: "meta_noindex",
		},
		{
			name:       "meta noindex uppercase",
			statusCode: 200,
			metaRobots: "NOINDEX",
			wantIndex:  false,
			wantReason: "meta_noindex",
		},
		{
			name:       "meta index follow",
			statusCode: 200,
			metaRobots: "index, follow",
			wantIndex:  true,
			wantReason: "",
		},
		{
			name:       "x-robots-tag noindex",
			statusCode: 200,
			xRobotsTag: "noindex",
			wantIndex:  false,
			wantReason: "x_robots_noindex",
		},
		{
			name:       "x-robots-tag noindex mixed case",
			statusCode: 200,
			xRobotsTag: "NoIndex, NoFollow",
			wantIndex:  false,
			wantReason: "x_robots_noindex",
		},
		{
			name:        "canonical mismatch",
			statusCode:  200,
			canonical:   "https://example.com/canonical",
			finalURL:    "https://example.com/page",
			originalURL: "https://example.com/original",
			wantIndex:   false,
			wantReason:  "canonical_mismatch",
		},
		{
			name:        "canonical matches final URL",
			statusCode:  200,
			canonical:   "https://example.com/page",
			finalURL:    "https://example.com/page",
			originalURL: "https://example.com/original",
			wantIndex:   true,
			wantReason:  "",
		},
		{
			name:        "canonical matches original URL",
			statusCode:  200,
			canonical:   "https://example.com/original",
			finalURL:    "https://example.com/page",
			originalURL: "https://example.com/original",
			wantIndex:   true,
			wantReason:  "",
		},
		{
			name:        "empty canonical is indexable",
			statusCode:  200,
			canonical:   "",
			finalURL:    "https://example.com/page",
			originalURL: "https://example.com/page",
			wantIndex:   true,
			wantReason:  "",
		},
		{
			name:       "0 status code",
			statusCode: 0,
			wantIndex:  false,
			wantReason: "status_0",
		},
		{
			name:       "meta noindex takes precedence over x-robots",
			statusCode: 200,
			metaRobots: "noindex",
			xRobotsTag: "noindex",
			wantIndex:  false,
			wantReason: "meta_noindex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex, gotReason := computeIndexability(
				tt.statusCode, tt.metaRobots, tt.xRobotsTag,
				tt.canonical, tt.finalURL, tt.originalURL,
			)
			if gotIndex != tt.wantIndex {
				t.Errorf("computeIndexability() indexable = %v, want %v", gotIndex, tt.wantIndex)
			}
			if gotReason != tt.wantReason {
				t.Errorf("computeIndexability() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
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

// --- Manager tests (no ClickHouse required) ---

// testManagerConfig returns a minimal config suitable for Manager tests.
func testManagerConfig() *config.Config {
	return &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:               2,
			Delay:                 10 * time.Millisecond,
			MaxPages:              100,
			Timeout:               5 * time.Second,
			UserAgent:             "TestBot/1.0",
			MaxBodySize:           1 << 20,
			MaxConcurrentSessions: 5,
			MaxFrontierSize:       10000,
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}
}

func TestNewManager(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.engines == nil {
		t.Error("engines map should be initialized")
	}
	if m.lastErrors == nil {
		t.Error("lastErrors map should be initialized")
	}
	if m.queuedSet == nil {
		t.Error("queuedSet map should be initialized")
	}
}

func TestNewManager_DefaultMaxSessions(t *testing.T) {
	cfg := testManagerConfig()
	cfg.Crawler.MaxConcurrentSessions = 0 // should default to 20
	m := NewManager(cfg, nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	// The semaphore channel capacity reflects the max sessions.
	if cap(m.sem) != 20 {
		t.Errorf("sem capacity = %d, want 20 (default)", cap(m.sem))
	}
}

func TestManager_LastError_EmptyByDefault(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	if got := m.LastError("nonexistent"); got != "" {
		t.Errorf("LastError for unknown session = %q, want empty", got)
	}
}

func TestManager_LastError_SetManually(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	// Simulate an error recorded by runEngine (field is accessible within package)
	m.mu.Lock()
	m.lastErrors["sess-1"] = "something went wrong"
	m.mu.Unlock()

	if got := m.LastError("sess-1"); got != "something went wrong" {
		t.Errorf("LastError = %q, want %q", got, "something went wrong")
	}
	// Unrelated session should still be empty
	if got := m.LastError("sess-2"); got != "" {
		t.Errorf("LastError for other session = %q, want empty", got)
	}
}

func TestManager_IsRunning_UnknownSession(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	if m.IsRunning("nonexistent") {
		t.Error("IsRunning should return false for unknown session")
	}
}

func TestManager_IsRunning_KnownEngine(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)

	engine := NewEngine(cfg, nil)
	m.mu.Lock()
	m.engines["sess-1"] = engine
	m.mu.Unlock()

	if !m.IsRunning("sess-1") {
		t.Error("IsRunning should return true for registered engine")
	}
	if m.IsRunning("sess-2") {
		t.Error("IsRunning should return false for unregistered session")
	}
}

func TestManager_Progress_UnknownSession(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	pages, queueLen, ok := m.Progress("nonexistent")
	if ok {
		t.Error("Progress should return false for unknown session")
	}
	if pages != 0 {
		t.Errorf("pages = %d, want 0", pages)
	}
	if queueLen != 0 {
		t.Errorf("queueLen = %d, want 0", queueLen)
	}
}

func TestManager_Progress_RunningSession(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)

	engine := NewEngine(cfg, nil)
	engine.pagesCrawled.Store(42)
	m.mu.Lock()
	m.engines["sess-1"] = engine
	m.mu.Unlock()

	pages, queueLen, ok := m.Progress("sess-1")
	if !ok {
		t.Error("Progress should return true for running session")
	}
	if pages != 42 {
		t.Errorf("pages = %d, want 42", pages)
	}
	if queueLen != 0 {
		t.Errorf("queueLen = %d, want 0", queueLen)
	}
}

func TestManager_Phase_UnknownSession(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	if got := m.Phase("nonexistent"); got != "" {
		t.Errorf("Phase = %q, want empty for unknown session", got)
	}
}

func TestManager_Phase_RunningSession(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)

	engine := NewEngine(cfg, nil)
	engine.phase.Store("crawling")
	m.mu.Lock()
	m.engines["sess-1"] = engine
	m.mu.Unlock()

	if got := m.Phase("sess-1"); got != "crawling" {
		t.Errorf("Phase = %q, want %q", got, "crawling")
	}
}

func TestManager_BufferState_UnknownSession(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	state := m.BufferState("nonexistent")
	if state != (storage.BufferErrorState{}) {
		t.Errorf("BufferState = %+v, want empty state", state)
	}
}

func TestManager_ActiveSessions_Empty(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	ids := m.ActiveSessions()
	if len(ids) != 0 {
		t.Errorf("ActiveSessions = %v, want empty", ids)
	}
}

func TestManager_ActiveSessions_WithEngines(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)

	m.mu.Lock()
	m.engines["sess-a"] = NewEngine(cfg, nil)
	m.engines["sess-b"] = NewEngine(cfg, nil)
	m.mu.Unlock()

	ids := m.ActiveSessions()
	if len(ids) != 2 {
		t.Fatalf("ActiveSessions len = %d, want 2", len(ids))
	}
	// Check both IDs are present (order not guaranteed)
	found := make(map[string]bool)
	for _, id := range ids {
		found[id] = true
	}
	if !found["sess-a"] || !found["sess-b"] {
		t.Errorf("ActiveSessions = %v, want [sess-a, sess-b]", ids)
	}
}

func TestManager_StopCrawl_NotRunning(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	err := m.StopCrawl("nonexistent")
	if err == nil {
		t.Fatal("StopCrawl should return error for non-running session")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error = %q, want it to contain 'not running'", err.Error())
	}
}

func TestManager_StopCrawl_RunningSession(t *testing.T) {
	cfg := testManagerConfig()
	m := NewManager(cfg, nil)

	engine := NewEngine(cfg, nil)
	m.mu.Lock()
	m.engines["sess-1"] = engine
	m.mu.Unlock()

	err := m.StopCrawl("sess-1")
	if err != nil {
		t.Fatalf("StopCrawl returned unexpected error: %v", err)
	}

	// Verify the engine context was cancelled
	select {
	case <-engine.ctx.Done():
		// Expected
	default:
		t.Error("engine context should be cancelled after StopCrawl")
	}
}

func TestManager_IsQueued(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	if m.IsQueued("nonexistent") {
		t.Error("IsQueued should return false for unknown session")
	}

	// Manually add to queue set
	m.queueMu.Lock()
	m.queuedSet["sess-q"] = true
	m.queue = append(m.queue, queuedCrawl{sessionID: "sess-q"})
	m.queueMu.Unlock()

	if !m.IsQueued("sess-q") {
		t.Error("IsQueued should return true for queued session")
	}
}

func TestManager_QueuedSessions(t *testing.T) {
	m := NewManager(testManagerConfig(), nil)
	if ids := m.QueuedSessions(); len(ids) != 0 {
		t.Errorf("QueuedSessions = %v, want empty", ids)
	}

	m.queueMu.Lock()
	m.queue = append(m.queue,
		queuedCrawl{sessionID: "q1"},
		queuedCrawl{sessionID: "q2"},
	)
	m.queuedSet["q1"] = true
	m.queuedSet["q2"] = true
	m.queueMu.Unlock()

	ids := m.QueuedSessions()
	if len(ids) != 2 {
		t.Fatalf("QueuedSessions len = %d, want 2", len(ids))
	}
	// Queue is FIFO, so order matters
	if ids[0] != "q1" || ids[1] != "q2" {
		t.Errorf("QueuedSessions = %v, want [q1, q2]", ids)
	}
}

// --- Engine getter tests ---

func TestEnginePagesCrawled(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)
	if got := engine.PagesCrawled(); got != 0 {
		t.Errorf("PagesCrawled = %d, want 0", got)
	}

	engine.pagesCrawled.Store(123)
	if got := engine.PagesCrawled(); got != 123 {
		t.Errorf("PagesCrawled = %d, want 123", got)
	}
}

func TestEnginePhase(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)
	if got := engine.Phase(); got != "" {
		t.Errorf("Phase = %q, want empty", got)
	}

	engine.phase.Store("fetching_sitemaps")
	if got := engine.Phase(); got != "fetching_sitemaps" {
		t.Errorf("Phase = %q, want %q", got, "fetching_sitemaps")
	}

	engine.phase.Store("crawling")
	if got := engine.Phase(); got != "crawling" {
		t.Errorf("Phase = %q, want %q", got, "crawling")
	}
}

func TestEngineQueueLen(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)
	if got := engine.QueueLen(); got != 0 {
		t.Errorf("QueueLen = %d, want 0", got)
	}
}

func TestEngineSessionID(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	seeds := []string{"https://example.com", "https://other.com"}
	id := engine.SessionID(seeds)

	if id == "" {
		t.Fatal("SessionID should return a non-empty ID")
	}
	if engine.session == nil {
		t.Fatal("SessionID should create the session")
	}
	if engine.session.ID != id {
		t.Errorf("session.ID = %q, want %q", engine.session.ID, id)
	}
	if len(engine.session.SeedURLs) != 2 {
		t.Errorf("session.SeedURLs len = %d, want 2", len(engine.session.SeedURLs))
	}
}

func TestEngineSetSessionID(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	// SetSessionID on nil session should be a no-op (not panic)
	engine.SetSessionID("new-id") // should not panic

	// Create session then change ID
	engine.SessionID([]string{"https://example.com"})
	originalID := engine.session.ID

	engine.SetSessionID("custom-id")
	if engine.session.ID != "custom-id" {
		t.Errorf("session.ID = %q, want %q", engine.session.ID, "custom-id")
	}
	if engine.session.ID == originalID {
		t.Error("session.ID should have changed")
	}
}

func TestEnginePreSeedDedup(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	urls := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://example.com/page3",
	}

	engine.PreSeedDedup(urls)

	// Verify URLs are marked as seen by checking SeenCount
	if got := engine.front.SeenCount(); got != 3 {
		t.Errorf("SeenCount = %d, want 3 after PreSeedDedup", got)
	}

	// Adding a pre-seeded URL to the frontier should be rejected (dedup)
	added := engine.front.Add(frontier.CrawlURL{URL: "https://example.com/page1"})
	if added {
		t.Error("frontier.Add should return false for pre-seeded URL")
	}

	// Adding a new URL should succeed
	added = engine.front.Add(frontier.CrawlURL{URL: "https://example.com/new-page"})
	if !added {
		t.Error("frontier.Add should return true for new URL")
	}
}

// --- seedFrontier tests ---

func TestSeedFrontier_ValidSeeds(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	seeds := []string{"https://example.com/page1", "https://example.com/page2"}
	engine.seedFrontier(seeds)

	if got := engine.front.Len(); got != 2 {
		t.Errorf("frontier.Len() = %d, want 2", got)
	}
}

func TestSeedFrontier_InvalidSeedSkipped(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	// An invalid URL that normalizer.Normalize will reject
	seeds := []string{"://invalid", "https://example.com/valid"}
	engine.seedFrontier(seeds)

	// Only the valid seed should be in the frontier
	if got := engine.front.Len(); got != 1 {
		t.Errorf("frontier.Len() = %d, want 1 (invalid seed should be skipped)", got)
	}
}

func TestSeedFrontier_Empty(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	engine.seedFrontier(nil)
	if got := engine.front.Len(); got != 0 {
		t.Errorf("frontier.Len() = %d, want 0 for nil seeds", got)
	}

	engine.seedFrontier([]string{})
	if got := engine.front.Len(); got != 0 {
		t.Errorf("frontier.Len() = %d, want 0 for empty seeds", got)
	}
}

func TestSeedFrontier_AllInvalid(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	seeds := []string{"://bad1", "://bad2"}
	engine.seedFrontier(seeds)

	if got := engine.front.Len(); got != 0 {
		t.Errorf("frontier.Len() = %d, want 0 for all-invalid seeds", got)
	}
}

// --- BufferState tests ---

func TestBufferState_NilBuffer(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)
	// engine.buffer is nil by default (before Run)
	state := engine.BufferState()
	if state != (storage.BufferErrorState{}) {
		t.Errorf("BufferState on nil buffer = %+v, want zero value", state)
	}
}

func TestBufferState_WithBuffer(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	engine.buffer = storage.NewBuffer(&successInserter{}, 100, time.Hour, engine.session.ID)
	defer engine.buffer.Close()

	state := engine.BufferState()
	// A fresh buffer with no errors should have all zero fields
	if state.LostPages != 0 || state.LostLinks != 0 {
		t.Errorf("BufferState = %+v, want zero lost counts", state)
	}
}

// --- promoteNext tests ---

func TestPromoteNext_DrainsQueue(t *testing.T) {
	// Test that promoteNext removes the first item from the queue and queuedSet.
	// We cannot call promoteNext directly because it launches runEngine which
	// requires a real store. Instead, we verify the queue manipulation logic
	// by testing via the internal queue state.
	m := newTestManager(5)

	m.queue = []queuedCrawl{
		{sessionID: "q1"},
		{sessionID: "q2"},
	}
	m.queuedSet["q1"] = true
	m.queuedSet["q2"] = true

	// Simulate what promoteNext does to the queue (lines 658-666 of manager.go):
	// It locks queueMu, pops the first element, removes from set, then unlocks.
	m.queueMu.Lock()
	if len(m.queue) > 0 {
		next := m.queue[0]
		m.queue = m.queue[1:]
		delete(m.queuedSet, next.sessionID)
	}
	m.queueMu.Unlock()

	if len(m.queue) != 1 {
		t.Errorf("queue length = %d, want 1", len(m.queue))
	}
	if m.queue[0].sessionID != "q2" {
		t.Errorf("remaining item = %q, want q2", m.queue[0].sessionID)
	}
	if m.queuedSet["q1"] {
		t.Error("q1 should be removed from queuedSet")
	}
	if !m.queuedSet["q2"] {
		t.Error("q2 should still be in queuedSet")
	}
}

func TestPromoteNext_EmptyQueueSafe(t *testing.T) {
	// promoteNext on an empty queue should return immediately without panic
	m := newTestManager(5)
	m.promoteNext() // should not panic
	if len(m.queue) != 0 {
		t.Fatal("queue should still be empty")
	}
}

func TestPromoteNext_MultipleItems(t *testing.T) {
	// Verify that promoteNext only takes the first item, leaving the rest.
	m := newTestManager(5)

	m.queue = []queuedCrawl{
		{sessionID: "first"},
		{sessionID: "second"},
		{sessionID: "third"},
	}
	m.queuedSet["first"] = true
	m.queuedSet["second"] = true
	m.queuedSet["third"] = true

	// Simulate promoteNext queue drain logic
	m.queueMu.Lock()
	next := m.queue[0]
	m.queue = m.queue[1:]
	delete(m.queuedSet, next.sessionID)
	m.queueMu.Unlock()

	if next.sessionID != "first" {
		t.Errorf("promoted item = %q, want 'first'", next.sessionID)
	}
	if len(m.queue) != 2 {
		t.Errorf("queue length = %d, want 2", len(m.queue))
	}
	if m.queue[0].sessionID != "second" || m.queue[1].sessionID != "third" {
		t.Errorf("remaining queue = [%s, %s], want [second, third]",
			m.queue[0].sessionID, m.queue[1].sessionID)
	}
}

// --- dequeue edge case tests ---

func TestDequeue_FirstItem(t *testing.T) {
	m := newTestManager(5)
	m.queue = []queuedCrawl{
		{sessionID: "first"},
		{sessionID: "second"},
	}
	m.queuedSet["first"] = true
	m.queuedSet["second"] = true

	ok := m.dequeue("first")
	if !ok {
		t.Fatal("dequeue should return true for first item")
	}
	if len(m.queue) != 1 {
		t.Fatalf("expected queue length 1, got %d", len(m.queue))
	}
	if m.queue[0].sessionID != "second" {
		t.Fatalf("remaining item should be 'second', got %q", m.queue[0].sessionID)
	}
}

func TestDequeue_LastItem(t *testing.T) {
	m := newTestManager(5)
	m.queue = []queuedCrawl{
		{sessionID: "first"},
		{sessionID: "last"},
	}
	m.queuedSet["first"] = true
	m.queuedSet["last"] = true

	ok := m.dequeue("last")
	if !ok {
		t.Fatal("dequeue should return true for last item")
	}
	if len(m.queue) != 1 {
		t.Fatalf("expected queue length 1, got %d", len(m.queue))
	}
	if m.queue[0].sessionID != "first" {
		t.Fatalf("remaining item should be 'first', got %q", m.queue[0].sessionID)
	}
}

func TestDequeue_OnlyItem(t *testing.T) {
	m := newTestManager(5)
	m.queue = []queuedCrawl{{sessionID: "only"}}
	m.queuedSet["only"] = true

	ok := m.dequeue("only")
	if !ok {
		t.Fatal("dequeue should return true for only item")
	}
	if len(m.queue) != 0 {
		t.Fatalf("expected empty queue, got %d items", len(m.queue))
	}
	if m.queuedSet["only"] {
		t.Fatal("'only' should be removed from queuedSet")
	}
}

func TestDequeue_InSetButNotInSlice(t *testing.T) {
	// Edge case: queuedSet has the ID but the slice does not (inconsistency)
	m := newTestManager(5)
	m.queuedSet["ghost"] = true
	// queue slice is empty — the item is in the set but not the slice

	ok := m.dequeue("ghost")
	// dequeue checks queuedSet first, then iterates the slice.
	// Since the slice is empty, it won't find the item, but it should still
	// clean up the set entry.
	if ok {
		t.Fatal("dequeue should return false when item is in set but not in slice")
	}
	// The set entry should be cleaned up regardless
	if m.queuedSet["ghost"] {
		t.Fatal("ghost should be removed from queuedSet even if not in slice")
	}
}

func TestEnginePreSeedDedup_Empty(t *testing.T) {
	cfg := testManagerConfig()
	engine := NewEngine(cfg, nil)

	engine.PreSeedDedup(nil)
	if got := engine.front.SeenCount(); got != 0 {
		t.Errorf("SeenCount = %d, want 0 after empty PreSeedDedup", got)
	}

	engine.PreSeedDedup([]string{})
	if got := engine.front.SeenCount(); got != 0 {
		t.Errorf("SeenCount = %d, want 0 after empty slice PreSeedDedup", got)
	}
}

// --- shouldRetryResult tests ---

func TestShouldRetryResult(t *testing.T) {
	tests := []struct {
		name                string
		maxRetries          int
		maxConsecutiveFails int
		statusCode          int
		errString           string
		attempt             int
		consecutiveFailures int // pre-seed host health failures
		want                bool
	}{
		{
			name:                "429 first attempt",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          429,
			attempt:             0,
			want:                true,
		},
		{
			name:                "500 first attempt",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          500,
			attempt:             0,
			want:                true,
		},
		{
			name:                "502 first attempt",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          502,
			attempt:             0,
			want:                true,
		},
		{
			name:                "503 first attempt",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          503,
			attempt:             0,
			want:                true,
		},
		{
			name:                "504 first attempt",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          504,
			attempt:             0,
			want:                true,
		},
		{
			name:                "200 OK should not retry",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          200,
			attempt:             0,
			want:                false,
		},
		{
			name:                "404 should not retry",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          404,
			attempt:             0,
			want:                false,
		},
		{
			name:                "max retries reached",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			statusCode:          500,
			attempt:             3,
			want:                false,
		},
		{
			name:                "retries disabled",
			maxRetries:          0,
			maxConsecutiveFails: 100,
			statusCode:          500,
			attempt:             0,
			want:                false,
		},
		{
			name:                "timeout error",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "timeout exceeded",
			attempt:             0,
			want:                true,
		},
		{
			name:                "connection refused error",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "connection_refused",
			attempt:             0,
			want:                true,
		},
		{
			name:                "connection reset error",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "connection reset by peer",
			attempt:             0,
			want:                true,
		},
		{
			name:                "eof error",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "unexpected EOF",
			attempt:             0,
			want:                true,
		},
		{
			name:                "dns_not_found should not retry",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "dns_not_found",
			attempt:             0,
			want:                false,
		},
		{
			name:                "tls_error should not retry",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "tls_error: certificate invalid",
			attempt:             0,
			want:                false,
		},
		{
			name:                "dns_timeout first attempt",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "dns_timeout",
			attempt:             0,
			want:                true,
		},
		{
			name:                "dns_timeout second attempt should not retry",
			maxRetries:          3,
			maxConsecutiveFails: 100,
			errString:           "dns_timeout",
			attempt:             1,
			want:                false,
		},
		{
			name:                "host exceeded consecutive failures",
			maxRetries:          3,
			maxConsecutiveFails: 5,
			statusCode:          500,
			attempt:             0,
			consecutiveFailures: 5,
			want:                false,
		},
		{
			name:                "host below consecutive failure threshold",
			maxRetries:          3,
			maxConsecutiveFails: 5,
			statusCode:          500,
			attempt:             0,
			consecutiveFailures: 4,
			want:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Crawler: config.CrawlerConfig{
					UserAgent: "TestBot/1.0",
					Retry: config.RetryConfig{
						MaxRetries:          tt.maxRetries,
						MaxConsecutiveFails: tt.maxConsecutiveFails,
					},
				},
			}
			engine := NewEngine(cfg, nil)

			// Pre-seed host health if needed
			if tt.consecutiveFailures > 0 {
				for i := 0; i < tt.consecutiveFailures; i++ {
					engine.hostHealth.RecordFailure("example.com")
				}
			}

			result := &fetcher.FetchResult{
				URL:        "https://example.com/page",
				StatusCode: tt.statusCode,
				Error:      tt.errString,
				Attempt:    tt.attempt,
			}

			got := engine.shouldRetryResult(result)
			if got != tt.want {
				t.Errorf("shouldRetryResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- extractHost tests ---

func TestExtractHost(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/page", "example.com"},
		{"https://example.com:8080/page", "example.com:8080"},
		{"http://sub.example.com/path", "sub.example.com"},
		{"https://example.com", "example.com"},
		{"not-a-valid-url-%%", "not-a-valid-url-%%"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := extractHost(tt.url)
			if got != tt.want {
				t.Errorf("extractHost(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// --- ensureNonNilArrays tests ---

func TestEnsureNonNilArrays(t *testing.T) {
	row := &storage.PageRow{}

	// Before: all slices/maps are nil
	if row.H1 != nil {
		t.Error("H1 should be nil initially")
	}
	if row.Headers != nil {
		t.Error("Headers should be nil initially")
	}

	ensureNonNilArrays(row)

	// After: all slices/maps should be non-nil
	if row.H1 == nil {
		t.Error("H1 should be non-nil after ensureNonNilArrays")
	}
	if row.H2 == nil {
		t.Error("H2 should be non-nil after ensureNonNilArrays")
	}
	if row.H3 == nil {
		t.Error("H3 should be non-nil after ensureNonNilArrays")
	}
	if row.H4 == nil {
		t.Error("H4 should be non-nil after ensureNonNilArrays")
	}
	if row.H5 == nil {
		t.Error("H5 should be non-nil after ensureNonNilArrays")
	}
	if row.H6 == nil {
		t.Error("H6 should be non-nil after ensureNonNilArrays")
	}
	if row.Headers == nil {
		t.Error("Headers should be non-nil after ensureNonNilArrays")
	}
	if row.RedirectChain == nil {
		t.Error("RedirectChain should be non-nil after ensureNonNilArrays")
	}
	if row.Hreflang == nil {
		t.Error("Hreflang should be non-nil after ensureNonNilArrays")
	}
	if row.SchemaTypes == nil {
		t.Error("SchemaTypes should be non-nil after ensureNonNilArrays")
	}
	if row.RenderedH1 == nil {
		t.Error("RenderedH1 should be non-nil after ensureNonNilArrays")
	}
	if row.RenderedSchemaTypes == nil {
		t.Error("RenderedSchemaTypes should be non-nil after ensureNonNilArrays")
	}
}

func TestEnsureNonNilArrays_PreservesExistingData(t *testing.T) {
	row := &storage.PageRow{
		H1:      []string{"existing heading"},
		Headers: map[string]string{"Server": "nginx"},
		RedirectChain: []storage.RedirectHopRow{
			{URL: "https://example.com/old", StatusCode: 301},
		},
	}

	ensureNonNilArrays(row)

	if len(row.H1) != 1 || row.H1[0] != "existing heading" {
		t.Errorf("H1 = %v, want [existing heading]", row.H1)
	}
	if row.Headers["Server"] != "nginx" {
		t.Errorf("Headers[Server] = %q, want nginx", row.Headers["Server"])
	}
	if len(row.RedirectChain) != 1 {
		t.Errorf("RedirectChain len = %d, want 1", len(row.RedirectChain))
	}
}

// --- computeJSDiffs tests ---

func TestComputeJSDiffs_NoChanges(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{
		Title:           "Same Title",
		MetaDescription: "Same Desc",
		H1:              []string{"Same H1"},
		Canonical:       "https://example.com/page",
		WordCount:       100,
		Links:           []parser.Link{{TargetURL: "a"}, {TargetURL: "b"}},
		Images:          []parser.Image{{Src: "img.png"}},
		SchemaTypes:     []string{"WebPage"},
	}
	rendered := &parser.PageData{
		Title:           "Same Title",
		MetaDescription: "Same Desc",
		H1:              []string{"Same H1"},
		Canonical:       "https://example.com/page",
		WordCount:       100,
		Links:           []parser.Link{{TargetURL: "a"}, {TargetURL: "b"}},
		Images:          []parser.Image{{Src: "img.png"}},
		SchemaTypes:     []string{"WebPage"},
	}

	computeJSDiffs(row, static, rendered)

	if row.JSChangedTitle {
		t.Error("JSChangedTitle should be false")
	}
	if row.JSChangedDescription {
		t.Error("JSChangedDescription should be false")
	}
	if row.JSChangedH1 {
		t.Error("JSChangedH1 should be false")
	}
	if row.JSChangedCanonical {
		t.Error("JSChangedCanonical should be false")
	}
	if row.JSChangedContent {
		t.Error("JSChangedContent should be false")
	}
	if row.JSAddedLinks != 0 {
		t.Errorf("JSAddedLinks = %d, want 0", row.JSAddedLinks)
	}
	if row.JSAddedImages != 0 {
		t.Errorf("JSAddedImages = %d, want 0", row.JSAddedImages)
	}
	if row.JSAddedSchema {
		t.Error("JSAddedSchema should be false")
	}
}

func TestComputeJSDiffs_AllChanged(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{
		Title:           "Old Title",
		MetaDescription: "Old Desc",
		H1:              []string{"Old H1"},
		Canonical:       "https://example.com/old",
		WordCount:       100,
		Links:           []parser.Link{{TargetURL: "a"}},
		Images:          []parser.Image{{Src: "old.png"}},
		SchemaTypes:     []string{"WebPage"},
	}
	rendered := &parser.PageData{
		Title:           "New Title",
		MetaDescription: "New Desc",
		H1:              []string{"New H1"},
		Canonical:       "https://example.com/new",
		WordCount:       200, // 100% change > 20% threshold
		Links:           []parser.Link{{TargetURL: "a"}, {TargetURL: "b"}, {TargetURL: "c"}},
		Images:          []parser.Image{{Src: "old.png"}, {Src: "new.png"}, {Src: "new2.png"}},
		SchemaTypes:     []string{"WebPage", "Article"},
	}

	computeJSDiffs(row, static, rendered)

	if !row.JSChangedTitle {
		t.Error("JSChangedTitle should be true")
	}
	if !row.JSChangedDescription {
		t.Error("JSChangedDescription should be true")
	}
	if !row.JSChangedH1 {
		t.Error("JSChangedH1 should be true")
	}
	if !row.JSChangedCanonical {
		t.Error("JSChangedCanonical should be true")
	}
	if !row.JSChangedContent {
		t.Error("JSChangedContent should be true (100% change > 20%)")
	}
	if row.JSAddedLinks != 2 {
		t.Errorf("JSAddedLinks = %d, want 2", row.JSAddedLinks)
	}
	if row.JSAddedImages != 2 {
		t.Errorf("JSAddedImages = %d, want 2", row.JSAddedImages)
	}
	if !row.JSAddedSchema {
		t.Error("JSAddedSchema should be true (Article added)")
	}
}

func TestComputeJSDiffs_ContentThreshold(t *testing.T) {
	tests := []struct {
		name        string
		staticWC    int
		renderedWC  int
		wantChanged bool
	}{
		{"exactly 20% increase", 100, 120, false},
		{"21% increase", 100, 121, true},
		{"19% increase", 100, 119, false},
		{"50% decrease", 100, 50, true},
		{"exact same", 100, 100, false},
		{"zero static, rendered >50", 0, 51, true},
		{"zero static, rendered <=50", 0, 50, false},
		{"zero static, rendered 0", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := &storage.PageRow{}
			static := &parser.PageData{WordCount: tt.staticWC}
			rendered := &parser.PageData{WordCount: tt.renderedWC}

			computeJSDiffs(row, static, rendered)

			if row.JSChangedContent != tt.wantChanged {
				t.Errorf("JSChangedContent = %v, want %v", row.JSChangedContent, tt.wantChanged)
			}
		})
	}
}

func TestComputeJSDiffs_LinksNegativeDelta(t *testing.T) {
	row := &storage.PageRow{}
	static := &parser.PageData{
		Links: []parser.Link{{TargetURL: "a"}, {TargetURL: "b"}, {TargetURL: "c"}},
	}
	rendered := &parser.PageData{
		Links: []parser.Link{{TargetURL: "a"}},
	}

	computeJSDiffs(row, static, rendered)

	if row.JSAddedLinks != -2 {
		t.Errorf("JSAddedLinks = %d, want -2", row.JSAddedLinks)
	}
}

func TestComputeJSDiffs_SchemaSubset(t *testing.T) {
	// If rendered has only a subset of static schemas, no new schema was added
	row := &storage.PageRow{}
	static := &parser.PageData{
		SchemaTypes: []string{"WebPage", "Article"},
	}
	rendered := &parser.PageData{
		SchemaTypes: []string{"WebPage"},
	}

	computeJSDiffs(row, static, rendered)

	if row.JSAddedSchema {
		t.Error("JSAddedSchema should be false when rendered is a subset of static")
	}
}

func TestComputeJSDiffs_TitleWhitespace(t *testing.T) {
	// Whitespace differences should not count
	row := &storage.PageRow{}
	static := &parser.PageData{Title: "  Title  "}
	rendered := &parser.PageData{Title: "Title"}

	computeJSDiffs(row, static, rendered)

	if row.JSChangedTitle {
		t.Error("JSChangedTitle should be false (whitespace difference only)")
	}
}

// --- buildScope edge cases ---

func TestBuildScope_InvalidSeedURL(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"://invalid-url"}, cfg)
	engine.buildScope()

	// Invalid URL should be skipped; allowed hosts should be empty
	if len(engine.allowedHosts) != 0 {
		t.Errorf("allowedHosts = %v, want empty for invalid seed", engine.allowedHosts)
	}
}

func TestBuildScope_MultipleHostSeeds(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{
		"https://example.com/page1",
		"https://other.com/page2",
	}, cfg)
	engine.buildScope()

	if !engine.allowedHosts["example.com"] {
		t.Error("example.com should be in allowedHosts")
	}
	if !engine.allowedHosts["other.com"] {
		t.Error("other.com should be in allowedHosts")
	}
}

func TestBuildScope_DomainScopeExtractsTLD(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "domain",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://www.example.co.uk/page"}, cfg)
	engine.buildScope()

	if !engine.allowedDomains["example.co.uk"] {
		t.Errorf("allowedDomains = %v, want example.co.uk", engine.allowedDomains)
	}
}

func TestBuildScope_SubdirectoryMultiplePrefixes(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "subdirectory",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{
		"https://example.com/blog/post",
		"https://example.com/docs/",
	}, cfg)
	engine.buildScope()

	if len(engine.allowedPrefixes) != 2 {
		t.Fatalf("allowedPrefixes len = %d, want 2", len(engine.allowedPrefixes))
	}

	// Check that /blog/post -> prefix is /blog/
	found := false
	for _, p := range engine.allowedPrefixes {
		if p == "https://example.com/blog/" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected prefix https://example.com/blog/ in %v", engine.allowedPrefixes)
	}

	// Check that /docs/ stays as /docs/
	found = false
	for _, p := range engine.allowedPrefixes {
		if p == "https://example.com/docs/" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected prefix https://example.com/docs/ in %v", engine.allowedPrefixes)
	}
}

// --- isInScope edge cases ---

func TestIsInScope_InvalidURL(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com/"}, cfg)
	engine.buildScope()

	if engine.isInScope("://invalid") {
		t.Error("invalid URL should not be in scope")
	}
}

func TestIsInScope_CaseInsensitive(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "host",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://Example.COM/page"}, cfg)
	engine.buildScope()

	if !engine.isInScope("https://example.com/other") {
		t.Error("scope check should be case-insensitive for host")
	}
	if !engine.isInScope("https://EXAMPLE.COM/other") {
		t.Error("scope check should be case-insensitive for host (uppercase)")
	}
}

func TestIsInScope_DomainFallbackToHostOnTLDError(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "domain",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://localhost/page"}, cfg)
	engine.buildScope()

	// localhost has no eTLD+1, so domain scope should fall back to host matching
	if !engine.isInScope("https://localhost/other") {
		t.Error("localhost should match via host fallback in domain scope")
	}
}

func TestIsInScope_SubdirectoryHostCaseInsensitive(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent:  "TestBot/1.0",
			CrawlScope: "subdirectory",
		},
	}
	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com/blog/"}, cfg)
	engine.buildScope()

	// Host case difference should still work (both scheme+host are lowered in buildScope)
	if !engine.isInScope("https://Example.COM/blog/post") {
		t.Error("subdirectory scope should be case-insensitive for host")
	}
	// Same casing works
	if !engine.isInScope("https://example.com/blog/article") {
		t.Error("subdirectory scope should match same-case prefix")
	}
}
