package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/fetcher"
	"github.com/SEObserver/crawlobserver/internal/frontier"
	"github.com/SEObserver/crawlobserver/internal/normalizer"
	"github.com/SEObserver/crawlobserver/internal/parser"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"golang.org/x/net/publicsuffix"
)

// Engine orchestrates the crawling pipeline.
type Engine struct {
	cfg     *config.Config
	store   *storage.Store
	buffer  *storage.Buffer
	front   *frontier.Frontier
	fetch   *fetcher.Fetcher
	robots  *fetcher.RobotsCache
	session *Session

	pagesCrawled atomic.Int64
	maxPages     int64

	allowedHosts   map[string]bool
	allowedDomains map[string]bool

	retryQueue     *RetryQueue
	hostHealth     *HostHealth
	retryPolicy    *RetryPolicy
	pendingRetries atomic.Int64

	sitemapOnly      bool
	checkExternal    bool
	externalWorkers  int
	externalCh       chan string
	externalChecked  sync.Map
	externalCheckBuf []storage.ExternalLinkCheck
	externalCheckMu  sync.Mutex

	checkResources   bool
	resourceWorkers  int
	resourceCh       chan resourceCheckItem
	resourceChecked  sync.Map
	resourceCheckBuf []storage.PageResourceCheck
	resourceCheckMu  sync.Mutex
	resourceRefBuf   []storage.PageResourceRef
	resourceRefMu    sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// resourceCheckItem wraps a resource URL with its metadata for the check worker.
type resourceCheckItem struct {
	URL          string
	ResourceType string
	IsInternal   bool
}

// NewEngine creates a new crawl engine.
func NewEngine(cfg *config.Config, store *storage.Store) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		cfg:    cfg,
		store:  store,
		front:  frontier.New(cfg.Crawler.Delay),
		fetch:  fetcher.New(cfg.Crawler.UserAgent, cfg.Crawler.Timeout, cfg.Crawler.MaxBodySize),
		robots: fetcher.NewRobotsCache(cfg.Crawler.UserAgent, cfg.Crawler.Timeout),
		retryQueue: NewRetryQueue(),
		hostHealth: NewHostHealth(),
		retryPolicy: &RetryPolicy{
			MaxRetries: cfg.Crawler.Retry.MaxRetries,
			BaseDelay:  cfg.Crawler.Retry.BaseDelay,
			MaxDelay:   cfg.Crawler.Retry.MaxDelay,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SessionID creates the session and returns its ID without starting the crawl.
func (e *Engine) SessionID(seeds []string) string {
	e.session = NewSession(seeds, e.cfg)
	return e.session.ID
}

// SetSessionID sets a pre-existing session ID (for resume).
func (e *Engine) SetSessionID(id string) {
	if e.session != nil {
		e.session.ID = id
	}
}

// ResumeSession prepares the engine to resume an existing session.
func (e *Engine) ResumeSession(id string, originalSeeds []string) {
	e.session = NewSession(originalSeeds, e.cfg)
	e.session.ID = id
}

// PagesCrawled returns the current number of pages crawled.
func (e *Engine) PagesCrawled() int64 {
	return e.pagesCrawled.Load()
}

// QueueLen returns the current frontier queue length.
func (e *Engine) QueueLen() int {
	return e.front.Len()
}

// PreSeedDedup adds URLs to the dedup database without adding them to the queue.
// Used when resuming a session to avoid re-crawling already visited URLs.
func (e *Engine) PreSeedDedup(urls []string) {
	for _, u := range urls {
		e.front.MarkSeen(u)
	}
}

// buildScope extracts allowed hostnames/domains from the session's original seed URLs.
func (e *Engine) buildScope() {
	e.allowedHosts = make(map[string]bool)
	e.allowedDomains = make(map[string]bool)

	seedURLs := e.session.SeedURLs
	for _, seed := range seedURLs {
		u, err := url.Parse(seed)
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		e.allowedHosts[host] = true
		domain, err := publicsuffix.EffectiveTLDPlusOne(host)
		if err == nil {
			e.allowedDomains[strings.ToLower(domain)] = true
		}
	}
}

// isInScope checks if a URL falls within the configured crawl scope.
func (e *Engine) isInScope(targetURL string) bool {
	u, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())

	switch e.cfg.Crawler.CrawlScope {
	case "domain":
		domain, err := publicsuffix.EffectiveTLDPlusOne(host)
		if err != nil {
			return e.allowedHosts[host]
		}
		return e.allowedDomains[strings.ToLower(domain)]
	default: // "host"
		return e.allowedHosts[host]
	}
}

// Run starts the crawl with the given seed URLs.
func (e *Engine) Run(seeds []string) error {
	if e.session == nil {
		e.session = NewSession(seeds, e.cfg)
	} else {
		// Don't overwrite SeedURLs — on resume/retry, seeds param contains
		// uncrawled/failed URLs, not the original seed URLs. Keep the originals
		// so RecomputeDepths assigns depth 0 only to the true seeds.
		e.session.Status = "running"
	}
	e.maxPages = int64(e.cfg.Crawler.MaxPages)
	e.buildScope()
	e.buffer = storage.NewBuffer(e.store, e.cfg.Storage.BatchSize, e.cfg.Storage.FlushInterval, e.session.ID)
	e.buffer.SetOnDataLost(func(lostPages, lostLinks int64) {
		applog.Errorf("crawler", "[%s] DATA LOSS: %d pages, %d links dropped — stopping crawl (disk full?)", e.session.ID, lostPages, lostLinks)
		e.Stop()
	})

	// Save session to ClickHouse
	if err := e.store.InsertSession(e.ctx, e.session.ToStorageRow()); err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}

	applog.Infof("crawler", "Starting crawl session %s with %d seed(s), %d workers",
		e.session.ID, len(seeds), e.cfg.Crawler.Workers)

	// Seed the frontier
	for i, seed := range seeds {
		normalized, err := normalizer.Normalize(seed)
		if err != nil {
			applog.Warnf("crawler", "skipping invalid seed %q: %v", seed, err)
			continue
		}
		e.front.Add(frontier.CrawlURL{
			URL:      normalized,
			Priority: i,
			Depth:    0,
		})
	}

	// Pre-fetch robots.txt for all seed hosts so we have sitemap directives
	for _, seed := range e.session.SeedURLs {
		e.robots.IsAllowed(seed) // triggers fetch + cache
	}

	// Discover and persist sitemaps (before workers start)
	sitemapURLs := e.robots.SitemapURLs()
	if len(sitemapURLs) > 0 {
		now := time.Now()
		sitemapEntries := fetcher.DiscoverSitemaps(e.fetch.Client(), e.cfg.Crawler.UserAgent, sitemapURLs)

		parentMap := make(map[string]string)
		for _, entry := range sitemapEntries {
			if entry.Type == "index" {
				for _, child := range entry.Sitemaps {
					parentMap[child] = entry.URL
				}
			}
		}

		var sitemapRows []storage.SitemapRow
		var sitemapURLRows []storage.SitemapURLRow

		for _, entry := range sitemapEntries {
			sitemapRows = append(sitemapRows, storage.SitemapRow{
				CrawlSessionID: e.session.ID,
				URL:            entry.URL,
				Type:           entry.Type,
				URLCount:       uint32(len(entry.URLs)),
				ParentURL:      parentMap[entry.URL],
				StatusCode:     uint16(entry.StatusCode),
				FetchedAt:      now,
			})
			for _, u := range entry.URLs {
				sitemapURLRows = append(sitemapURLRows, storage.SitemapURLRow{
					CrawlSessionID: e.session.ID,
					SitemapURL:     entry.URL,
					Loc:            u.Loc,
					LastMod:        u.LastMod,
					ChangeFreq:     u.ChangeFreq,
					Priority:       u.Priority,
				})
			}
		}

		if err := e.store.InsertSitemaps(context.Background(), sitemapRows); err != nil {
			applog.Warnf("crawler", "failed to persist sitemaps: %v", err)
		}
		if err := e.store.InsertSitemapURLs(context.Background(), sitemapURLRows); err != nil {
			applog.Warnf("crawler", "failed to persist sitemap URLs: %v", err)
		}
		if len(sitemapRows) > 0 {
			applog.Infof("crawler", "Persisted %d sitemaps (%d URLs total)", len(sitemapRows), len(sitemapURLRows))
		}

		if e.sitemapOnly && len(sitemapURLRows) > 0 {
			added := 0
			for _, su := range sitemapURLRows {
				norm, err := normalizer.Normalize(su.Loc)
				if err != nil {
					continue
				}
				if e.isInScope(norm) {
					e.front.Add(frontier.CrawlURL{URL: norm, Priority: 1, Depth: 1})
					added++
				}
			}
			applog.Infof("crawler", "Sitemap-only mode: enqueued %d URLs from sitemaps", added)
		}
	}

	// Channels
	fetchCh := make(chan *frontier.CrawlURL, e.cfg.Crawler.Workers)
	parseCh := make(chan *fetcher.FetchResult, e.cfg.Crawler.Workers)

	var wg sync.WaitGroup

	// Fetch workers
	numFetchWorkers := e.cfg.Crawler.Workers
	for i := 0; i < numFetchWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			e.fetchWorker(id, fetchCh, parseCh)
		}(i)
	}

	// Parse workers
	numParseWorkers := max(1, numFetchWorkers/2)
	var parseWg sync.WaitGroup
	for i := 0; i < numParseWorkers; i++ {
		parseWg.Add(1)
		go func(id int) {
			defer parseWg.Done()
			e.parseWorker(id, parseCh)
		}(i)
	}

	// Retry dispatcher: polls retryQueue and sends ready items to fetchCh
	var retryWg sync.WaitGroup
	retryCtx, retryCancel := context.WithCancel(context.Background())
	retryWg.Add(1)
	go func() {
		defer retryWg.Done()
		e.retryDispatcher(retryCtx, fetchCh)
	}()

	// External link check workers
	var extWg sync.WaitGroup
	if e.checkExternal {
		e.externalCh = make(chan string, 1000)
		numExtWorkers := e.externalWorkers
		if numExtWorkers <= 0 {
			numExtWorkers = 3
		}
		for i := 0; i < numExtWorkers; i++ {
			extWg.Add(1)
			go func() {
				defer extWg.Done()
				e.externalCheckWorker()
			}()
		}
		applog.Infof("crawler", "Started %d external link check workers", numExtWorkers)
	}

	// Page resource check workers
	var resWg sync.WaitGroup
	if e.checkResources {
		e.resourceCh = make(chan resourceCheckItem, 1000)
		numResWorkers := e.resourceWorkers
		if numResWorkers <= 0 {
			numResWorkers = 3
		}
		for i := 0; i < numResWorkers; i++ {
			resWg.Add(1)
			go func() {
				defer resWg.Done()
				e.resourceCheckWorker()
			}()
		}
		applog.Infof("crawler", "Started %d resource check workers", numResWorkers)
	}

	// Dispatcher: feeds URLs from frontier to fetch workers
	e.dispatcher(fetchCh)

	// Dispatcher returned — cancel retry dispatcher, wait for it, then close fetchCh
	retryCancel()
	retryWg.Wait()
	close(fetchCh)

	// Wait for fetch workers to finish
	wg.Wait()
	close(parseCh)

	// Wait for parse workers to finish
	parseWg.Wait()

	// Shutdown external check workers
	if e.checkExternal && e.externalCh != nil {
		close(e.externalCh)
		extWg.Wait()
		e.flushExternalChecks()
	}

	// Shutdown resource check workers
	if e.checkResources && e.resourceCh != nil {
		close(e.resourceCh)
		resWg.Wait()
		e.flushResourceChecks()
		e.flushResourceRefs()
	}

	// Final flush
	e.buffer.Close()

	// Check for data loss
	bufState := e.buffer.ErrorState()
	if bufState.LostPages > 0 || bufState.LostLinks > 0 {
		applog.Warnf("crawler", "[%s] Crawl ended with data loss: %d pages, %d links dropped",
			e.session.ID, bufState.LostPages, bufState.LostLinks)
	}

	// Persist robots.txt data
	entries := e.robots.Entries()
	if len(entries) > 0 {
		var robotsRows []storage.RobotsRow
		for host, entry := range entries {
			robotsRows = append(robotsRows, storage.RobotsRow{
				CrawlSessionID: e.session.ID,
				Host:           host,
				StatusCode:     uint16(entry.StatusCode),
				Content:        entry.Content,
				FetchedAt:      entry.FetchedAt,
			})
		}
		if err := e.store.InsertRobotsData(context.Background(), robotsRows); err != nil {
			applog.Warnf("crawler", "failed to persist robots.txt data: %v", err)
		} else {
			applog.Infof("crawler", "Persisted robots.txt for %d hosts", len(robotsRows))
		}
	}

	// Recompute depths via BFS
	if err := e.store.RecomputeDepths(context.Background(), e.session.ID, e.session.SeedURLs); err != nil {
		applog.Warnf("crawler", "depth recomputation failed: %v", err)
	}

	// Compute internal PageRank
	if err := e.store.ComputePageRank(context.Background(), e.session.ID); err != nil {
		applog.Warnf("crawler", "PageRank computation failed: %v", err)
	}

	// Update session status with actual page count from storage
	if bufState.LostPages > 0 || bufState.LostLinks > 0 {
		e.session.Status = "completed_with_errors"
	} else {
		e.session.Status = "completed"
	}
	if total, err := e.store.CountPages(context.Background(), e.session.ID); err == nil {
		e.session.Pages = total
	} else {
		e.session.Pages = uint64(e.pagesCrawled.Load())
	}
	row := e.session.ToStorageRow()
	row.FinishedAt = time.Now()
	if err := e.store.InsertSession(context.Background(), row); err != nil {
		applog.Errorf("crawler", "updating session: %v", err)
	}

	applog.Infof("crawler", "Crawl complete: %d pages crawled, session %s",
		e.pagesCrawled.Load(), e.session.ID)

	return nil
}

// BufferState returns the current buffer error state for monitoring.
func (e *Engine) BufferState() storage.BufferErrorState {
	if e.buffer == nil {
		return storage.BufferErrorState{}
	}
	return e.buffer.ErrorState()
}

// Stop gracefully stops the engine.
func (e *Engine) Stop() {
	applog.Info("crawler", "Stopping crawl engine...")
	e.cancel()
	e.front.Close()
}

func (e *Engine) dispatcher(fetchCh chan<- *frontier.CrawlURL) {
	backoff := 10 * time.Millisecond
	maxBackoff := 500 * time.Millisecond
	emptyCount := 0

	for {
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		// Check max pages limit
		if e.maxPages > 0 && e.pagesCrawled.Load() >= e.maxPages {
			applog.Infof("crawler", "Reached max pages limit (%d)", e.maxPages)
			return
		}

		next := e.front.Next()
		if next == nil {
			emptyCount++
			// If frontier is empty, no pending retries, and idle long enough, we're done
			if e.front.Len() == 0 && e.pendingRetries.Load() == 0 && emptyCount > 50 {
				return
			}
			// Backoff when nothing is ready
			wait := backoff * time.Duration(min(emptyCount, 10))
			if wait > maxBackoff {
				wait = maxBackoff
			}
			time.Sleep(wait)
			continue
		}

		emptyCount = 0

		select {
		case fetchCh <- next:
		case <-e.ctx.Done():
			return
		}
	}
}

func (e *Engine) fetchWorker(id int, in <-chan *frontier.CrawlURL, out chan<- *fetcher.FetchResult) {
	for crawlURL := range in {
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		// Always fetch robots.txt for storage; only block if configured
		allowed := e.robots.IsAllowed(crawlURL.URL)
		if e.cfg.Crawler.RespectRobots && !allowed {
			applog.Infof("crawler", "Blocked by robots.txt: %s", crawlURL.URL)
			continue
		}

		result := e.fetch.Fetch(crawlURL.URL, crawlURL.Depth, crawlURL.FoundOn)
		result.Attempt = crawlURL.Attempt

		// Only count first attempts for progress
		if crawlURL.Attempt == 0 {
			e.pagesCrawled.Add(1)
		}

		count := e.pagesCrawled.Load()
		if count%100 == 0 {
			applog.Infof("crawler", "Progress: %d pages crawled, %d in queue, %d pending retries",
				count, e.front.Len(), e.pendingRetries.Load())
		}

		select {
		case out <- result:
		case <-e.ctx.Done():
			return
		}
	}
}

func (e *Engine) parseWorker(id int, in <-chan *fetcher.FetchResult) {
	for result := range in {
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		// Retry decision
		if e.shouldRetryResult(result) {
			e.enqueueRetry(result)
			continue // skip storage
		}
		// Decrement pending retries if this was a retry attempt (success or final failure)
		if result.Attempt > 0 {
			e.pendingRetries.Add(-1)
		}

		// Host health tracking
		host := extractHost(result.URL)
		if result.Error != "" || result.StatusCode >= 500 {
			e.hostHealth.RecordFailure(host)
		} else {
			e.hostHealth.RecordSuccess(host)
		}

		// Circuit breaker: check every 100 pages
		if pages := e.pagesCrawled.Load(); pages > 0 && pages%100 == 0 {
			if rate := e.hostHealth.GlobalErrorRate(); rate > e.cfg.Crawler.Retry.MaxGlobalErrorRate {
				applog.Errorf("crawler", "STOPPING: global error rate %.2f exceeds threshold %.2f",
					rate, e.cfg.Crawler.Retry.MaxGlobalErrorRate)
				e.Stop()
			}
		}

		now := time.Now()

		// Build page row
		pageRow := storage.PageRow{
			CrawlSessionID:  e.session.ID,
			URL:             result.URL,
			FinalURL:        result.FinalURL,
			StatusCode:      uint16(result.StatusCode),
			ContentType:     result.ContentType,
			BodySize:        uint64(result.BodySize),
			BodyTruncated:   result.BodyTruncated,
			FetchDurationMs: uint64(result.Duration.Milliseconds()),
			Error:           result.Error,
			Depth:           uint16(result.Depth),
			FoundOn:         result.FoundOn,
			CrawledAt:       now,
			Headers:         result.Headers,
		}

		// Extract response headers info
		if enc, ok := result.Headers["Content-Encoding"]; ok {
			pageRow.ContentEncoding = enc
		}
		if xrt, ok := result.Headers["X-Robots-Tag"]; ok {
			pageRow.XRobotsTag = xrt
		}

		// Convert redirect chain
		for _, hop := range result.RedirectChain {
			pageRow.RedirectChain = append(pageRow.RedirectChain, storage.RedirectHopRow{
				URL:        hop.URL,
				StatusCode: uint16(hop.StatusCode),
			})
		}

		// Store raw HTML if enabled
		if e.cfg.Crawler.StoreHTML && result.IsHTML() && len(result.Body) > 0 {
			pageRow.BodyHTML = string(result.Body)
		}

		// Parse HTML if applicable
		if result.IsHTML() && len(result.Body) > 0 && result.Error == "" {
			pageData, err := parser.Parse(result.Body, result.FinalURL)
			if err != nil {
				applog.Warnf("crawler", "Parse error for %s: %v", result.URL, err)
			} else {
				pageRow.Title = pageData.Title
				pageRow.TitleLength = uint16(len(pageData.Title))
				pageRow.Canonical = pageData.Canonical
				pageRow.MetaRobots = pageData.MetaRobots
				pageRow.MetaDescription = pageData.MetaDescription
				pageRow.MetaDescLength = uint16(len(pageData.MetaDescription))
				pageRow.MetaKeywords = pageData.MetaKeywords
				pageRow.H1 = pageData.H1
				pageRow.H2 = pageData.H2
				pageRow.H3 = pageData.H3
				pageRow.H4 = pageData.H4
				pageRow.H5 = pageData.H5
				pageRow.H6 = pageData.H6
				pageRow.WordCount = uint32(pageData.WordCount)
				pageRow.Lang = pageData.Lang
				pageRow.OGTitle = pageData.OGTitle
				pageRow.OGDescription = pageData.OGDescription
				pageRow.OGImage = pageData.OGImage
				pageRow.SchemaTypes = pageData.SchemaTypes

				// Images
				pageRow.ImagesCount = uint16(len(pageData.Images))
				noAlt := 0
				for _, img := range pageData.Images {
					if img.Alt == "" {
						noAlt++
					}
				}
				pageRow.ImagesNoAlt = uint16(noAlt)

				// Hreflang
				for _, h := range pageData.Hreflang {
					pageRow.Hreflang = append(pageRow.Hreflang, storage.HreflangRow{
						Lang: h.Lang,
						URL:  h.URL,
					})
				}

				// Canonical self-referencing check
				if pageData.Canonical != "" {
					pageRow.CanonicalIsSelf = (pageData.Canonical == result.FinalURL || pageData.Canonical == result.URL)
				}

				// Indexability
				pageRow.IsIndexable, pageRow.IndexReason = computeIndexability(
					uint16(result.StatusCode), pageData.MetaRobots, pageRow.XRobotsTag,
					pageData.Canonical, result.FinalURL, result.URL,
				)

				// Process links
				var linkRows []storage.LinkRow
				var internalOut, externalOut uint32
				for _, link := range pageData.Links {
					linkRows = append(linkRows, storage.LinkRow{
						CrawlSessionID: e.session.ID,
						SourceURL:      result.URL,
						TargetURL:      link.TargetURL,
						AnchorText:     link.AnchorText,
						Rel:            link.Rel,
						IsInternal:     link.IsInternal,
						Tag:            link.Tag,
						CrawledAt:      now,
					})

					if link.IsInternal {
						internalOut++
						// Check crawl scope before adding to frontier
						if !e.sitemapOnly && e.isInScope(link.TargetURL) {
							newDepth := result.Depth + 1
							if e.cfg.Crawler.MaxDepth == 0 || newDepth <= e.cfg.Crawler.MaxDepth {
								e.front.Add(frontier.CrawlURL{
									URL:      link.TargetURL,
									Priority: newDepth,
									Depth:    newDepth,
									FoundOn:  result.URL,
								})
							}
						}
					} else {
						externalOut++
						if e.checkExternal && e.externalCh != nil {
							if _, loaded := e.externalChecked.LoadOrStore(link.TargetURL, struct{}{}); !loaded {
								select {
								case e.externalCh <- link.TargetURL:
								default:
								}
							}
						}
					}
				}
				pageRow.InternalLinksOut = internalOut
				pageRow.ExternalLinksOut = externalOut

				if len(linkRows) > 0 {
					e.buffer.AddLinks(linkRows)
				}

				// Process page resources
				if e.checkResources && e.resourceCh != nil {
					for _, res := range pageData.Resources {
						// Always record the ref (page -> resource)
						e.bufferResourceRef(storage.PageResourceRef{
							CrawlSessionID: e.session.ID,
							PageURL:        result.URL,
							ResourceURL:    res.URL,
							ResourceType:   res.ResourceType,
							IsInternal:     res.IsInternal,
						})
						// Check each unique resource URL only once
						if _, loaded := e.resourceChecked.LoadOrStore(res.URL, struct{}{}); !loaded {
							select {
							case e.resourceCh <- resourceCheckItem{
								URL:          res.URL,
								ResourceType: res.ResourceType,
								IsInternal:   res.IsInternal,
							}:
							default:
							}
						}
					}
				}
			}
		}

		// Ensure arrays are not nil for ClickHouse
		if pageRow.H1 == nil {
			pageRow.H1 = []string{}
		}
		if pageRow.H2 == nil {
			pageRow.H2 = []string{}
		}
		if pageRow.H3 == nil {
			pageRow.H3 = []string{}
		}
		if pageRow.H4 == nil {
			pageRow.H4 = []string{}
		}
		if pageRow.H5 == nil {
			pageRow.H5 = []string{}
		}
		if pageRow.H6 == nil {
			pageRow.H6 = []string{}
		}
		if pageRow.Headers == nil {
			pageRow.Headers = map[string]string{}
		}
		if pageRow.RedirectChain == nil {
			pageRow.RedirectChain = []storage.RedirectHopRow{}
		}
		if pageRow.Hreflang == nil {
			pageRow.Hreflang = []storage.HreflangRow{}
		}
		if pageRow.SchemaTypes == nil {
			pageRow.SchemaTypes = []string{}
		}

		e.buffer.AddPage(pageRow)
	}
}

// retryDispatcher polls the retry queue and sends ready items to fetchCh.
func (e *Engine) retryDispatcher(ctx context.Context, fetchCh chan<- *frontier.CrawlURL) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				item := e.retryQueue.PopReady()
				if item == nil {
					break
				}
				crawlURL := &frontier.CrawlURL{
					URL:     item.URL,
					Depth:   item.Depth,
					FoundOn: item.FoundOn,
					Attempt: item.Attempt,
				}
				select {
				case fetchCh <- crawlURL:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// shouldRetryResult checks if a failed result should be retried.
func (e *Engine) shouldRetryResult(result *fetcher.FetchResult) bool {
	if !e.retryPolicy.ShouldRetry(result.StatusCode, result.Error, result.Attempt) {
		return false
	}
	host := extractHost(result.URL)
	return e.hostHealth.ShouldRetry(host, e.cfg.Crawler.Retry.MaxConsecutiveFails)
}

// enqueueRetry adds a failed result to the retry queue with computed delay.
func (e *Engine) enqueueRetry(result *fetcher.FetchResult) {
	nextAttempt := result.Attempt + 1
	retryAfter := result.Headers["Retry-After"]
	delay := e.retryPolicy.ComputeDelay(result.Attempt, retryAfter)

	host := extractHost(result.URL)
	applog.Infof("crawler", "Retry #%d for %s (status=%d, err=%q) in %v",
		nextAttempt, result.URL, result.StatusCode, result.Error, delay)

	// Track pending retries (first enqueue only)
	if result.Attempt == 0 {
		e.pendingRetries.Add(1)
	}

	e.retryQueue.Push(&RetryItem{
		URL:      result.URL,
		Host:     host,
		Depth:    result.Depth,
		FoundOn:  result.FoundOn,
		Attempt:  nextAttempt,
		ReadyAt:  time.Now().Add(delay),
		LastCode: result.StatusCode,
		LastErr:  result.Error,
	})
}

// extractHost returns the host portion of a URL.
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

// computeIndexability determines if a page is indexable and why not.
func computeIndexability(statusCode uint16, metaRobots, xRobotsTag, canonical, finalURL, originalURL string) (bool, string) {
	// Non-2xx status codes are not indexable
	if statusCode < 200 || statusCode >= 300 {
		if statusCode >= 300 && statusCode < 400 {
			return false, "redirect"
		}
		return false, fmt.Sprintf("status_%d", statusCode)
	}

	// Check meta robots
	lower := strings.ToLower(metaRobots)
	if strings.Contains(lower, "noindex") {
		return false, "meta_noindex"
	}

	// Check X-Robots-Tag header
	if strings.Contains(strings.ToLower(xRobotsTag), "noindex") {
		return false, "x_robots_noindex"
	}

	// Check canonical pointing elsewhere
	if canonical != "" && canonical != finalURL && canonical != originalURL {
		return false, "canonical_mismatch"
	}

	return true, ""
}

// externalCheckWorker checks external URLs and buffers the results.
func (e *Engine) externalCheckWorker() {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	for rawURL := range e.externalCh {
		start := time.Now()
		check := storage.ExternalLinkCheck{
			CrawlSessionID: e.session.ID,
			URL:            rawURL,
			CheckedAt:      time.Now(),
		}
		req, err := http.NewRequestWithContext(e.ctx, "GET", rawURL, nil)
		if err != nil {
			check.Error = fetcher.CategorizeError(err)
			check.ResponseTimeMs = uint32(time.Since(start).Milliseconds())
			e.bufferExternalCheck(check)
			continue
		}
		req.Header.Set("User-Agent", e.cfg.Crawler.UserAgent)
		resp, err := client.Do(req)
		check.ResponseTimeMs = uint32(time.Since(start).Milliseconds())
		if err != nil {
			check.Error = fetcher.CategorizeError(err)
		} else {
			resp.Body.Close()
			check.StatusCode = uint16(resp.StatusCode)
			check.ContentType = resp.Header.Get("Content-Type")
			if resp.Request.URL.String() != rawURL {
				check.RedirectURL = resp.Request.URL.String()
			}
		}
		e.bufferExternalCheck(check)
	}
}

func (e *Engine) bufferExternalCheck(check storage.ExternalLinkCheck) {
	e.externalCheckMu.Lock()
	e.externalCheckBuf = append(e.externalCheckBuf, check)
	if len(e.externalCheckBuf) >= 50 {
		batch := e.externalCheckBuf
		e.externalCheckBuf = nil
		e.externalCheckMu.Unlock()
		if err := e.store.InsertExternalLinkChecks(context.Background(), batch); err != nil {
			applog.Warnf("crawler", "failed to insert external link checks: %v", err)
		}
		return
	}
	e.externalCheckMu.Unlock()
}

func (e *Engine) flushExternalChecks() {
	e.externalCheckMu.Lock()
	batch := e.externalCheckBuf
	e.externalCheckBuf = nil
	e.externalCheckMu.Unlock()
	if len(batch) > 0 {
		if err := e.store.InsertExternalLinkChecks(context.Background(), batch); err != nil {
			applog.Warnf("crawler", "failed to flush external link checks: %v", err)
		} else {
			applog.Infof("crawler", "Flushed %d external link checks", len(batch))
		}
	}
}

// resourceCheckWorker checks resource URLs and buffers the results.
func (e *Engine) resourceCheckWorker() {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	for item := range e.resourceCh {
		start := time.Now()
		check := storage.PageResourceCheck{
			CrawlSessionID: e.session.ID,
			URL:            item.URL,
			ResourceType:   item.ResourceType,
			IsInternal:     item.IsInternal,
			CheckedAt:      time.Now(),
		}
		req, err := http.NewRequestWithContext(e.ctx, "GET", item.URL, nil)
		if err != nil {
			check.Error = err.Error()
			check.ResponseTimeMs = uint32(time.Since(start).Milliseconds())
			e.bufferResourceCheck(check)
			continue
		}
		req.Header.Set("User-Agent", e.cfg.Crawler.UserAgent)
		resp, err := client.Do(req)
		check.ResponseTimeMs = uint32(time.Since(start).Milliseconds())
		if err != nil {
			check.Error = err.Error()
		} else {
			resp.Body.Close()
			check.StatusCode = uint16(resp.StatusCode)
			check.ContentType = resp.Header.Get("Content-Type")
			if resp.Request.URL.String() != item.URL {
				check.RedirectURL = resp.Request.URL.String()
			}
		}
		e.bufferResourceCheck(check)
	}
}

func (e *Engine) bufferResourceCheck(check storage.PageResourceCheck) {
	e.resourceCheckMu.Lock()
	e.resourceCheckBuf = append(e.resourceCheckBuf, check)
	if len(e.resourceCheckBuf) >= 50 {
		batch := e.resourceCheckBuf
		e.resourceCheckBuf = nil
		e.resourceCheckMu.Unlock()
		if err := e.store.InsertPageResourceChecks(context.Background(), batch); err != nil {
			applog.Warnf("crawler", "failed to insert page resource checks: %v", err)
		}
		return
	}
	e.resourceCheckMu.Unlock()
}

func (e *Engine) flushResourceChecks() {
	e.resourceCheckMu.Lock()
	batch := e.resourceCheckBuf
	e.resourceCheckBuf = nil
	e.resourceCheckMu.Unlock()
	if len(batch) > 0 {
		if err := e.store.InsertPageResourceChecks(context.Background(), batch); err != nil {
			applog.Warnf("crawler", "failed to flush page resource checks: %v", err)
		} else {
			applog.Infof("crawler", "Flushed %d page resource checks", len(batch))
		}
	}
}

func (e *Engine) bufferResourceRef(ref storage.PageResourceRef) {
	e.resourceRefMu.Lock()
	e.resourceRefBuf = append(e.resourceRefBuf, ref)
	if len(e.resourceRefBuf) >= 200 {
		batch := e.resourceRefBuf
		e.resourceRefBuf = nil
		e.resourceRefMu.Unlock()
		if err := e.store.InsertPageResourceRefs(context.Background(), batch); err != nil {
			applog.Warnf("crawler", "failed to insert page resource refs: %v", err)
		}
		return
	}
	e.resourceRefMu.Unlock()
}

func (e *Engine) flushResourceRefs() {
	e.resourceRefMu.Lock()
	batch := e.resourceRefBuf
	e.resourceRefBuf = nil
	e.resourceRefMu.Unlock()
	if len(batch) > 0 {
		if err := e.store.InsertPageResourceRefs(context.Background(), batch); err != nil {
			applog.Warnf("crawler", "failed to flush page resource refs: %v", err)
		} else {
			applog.Infof("crawler", "Flushed %d page resource refs", len(batch))
		}
	}
}
