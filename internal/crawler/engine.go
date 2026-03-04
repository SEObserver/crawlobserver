package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/extraction"
	"github.com/SEObserver/crawlobserver/internal/fetcher"
	"github.com/SEObserver/crawlobserver/internal/frontier"
	"github.com/SEObserver/crawlobserver/internal/normalizer"
	"github.com/SEObserver/crawlobserver/internal/parser"
	"github.com/SEObserver/crawlobserver/internal/renderer"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"golang.org/x/net/publicsuffix"
)

// Engine orchestrates the crawling pipeline.
type Engine struct {
	cfg      *config.Config
	store    *storage.Store
	bufferMu sync.RWMutex
	buffer   *storage.Buffer
	front    *frontier.Frontier
	fetch    *fetcher.Fetcher
	robots   *fetcher.RobotsCache
	session  *Session

	pagesCrawled   atomic.Int64
	lastProgressAt atomic.Int64
	maxPages       int64
	phase          atomic.Value // string: current phase ("fetching_sitemaps", "crawling", "")

	allowedHosts    map[string]bool
	allowedDomains  map[string]bool
	allowedPrefixes []string

	retryQueue     *RetryQueue
	hostHealth     *HostHealth
	retryPolicy    *RetryPolicy
	pendingRetries atomic.Int64

	sitemapOnly      bool
	fetchSitemaps    bool
	sitemapURLSet    map[string]bool // URLs found in sitemaps, for priority boost
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

	renderPool    *renderer.Pool
	renderMode    renderer.DetectionMode
	renderWorkers int
	renderCh      chan *renderItem
	followJSLinks bool

	extractors []extraction.Extractor

	ctx    context.Context
	cancel context.CancelFunc
}

// renderItem transports data from parseWorker to renderWorker.
type renderItem struct {
	result     *fetcher.FetchResult
	staticData *parser.PageData
	pageRow    storage.PageRow
	linkRows   []storage.LinkRow
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
	dialOpts := fetcher.DialOptions{
		SourceIP:        cfg.Crawler.SourceIP,
		ForceIPv4:       cfg.Crawler.ForceIPv4,
		AllowPrivateIPs: cfg.Crawler.AllowPrivateIPs,
	}
	return &Engine{
		cfg:        cfg,
		store:      store,
		front:      frontier.New(cfg.Crawler.Delay, cfg.Crawler.MaxFrontierSize),
		fetch:      fetcher.New(cfg.Crawler.UserAgent, cfg.Crawler.Timeout, cfg.Crawler.MaxBodySize, dialOpts, fetcher.TLSProfile(cfg.Crawler.TLSProfile)),
		robots:     fetcher.NewRobotsCache(cfg.Crawler.UserAgent, cfg.Crawler.Timeout, dialOpts, fetcher.TLSProfile(cfg.Crawler.TLSProfile)),
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

// Phase returns the current engine phase (e.g. "fetching_sitemaps", "crawling").
func (e *Engine) Phase() string {
	if v := e.phase.Load(); v != nil {
		return v.(string)
	}
	return ""
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
	e.allowedPrefixes = nil

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
		if e.cfg.Crawler.CrawlScope == "subdirectory" {
			dir := path.Dir(u.Path)
			if strings.HasSuffix(u.Path, "/") {
				dir = u.Path
			}
			if !strings.HasSuffix(dir, "/") {
				dir += "/"
			}
			prefix := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + dir
			e.allowedPrefixes = append(e.allowedPrefixes, prefix)
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
	case "subdirectory":
		targetLower := strings.ToLower(targetURL)
		for _, prefix := range e.allowedPrefixes {
			if strings.HasPrefix(targetLower, prefix) {
				return true
			}
		}
		return false
	default: // "host"
		return e.allowedHosts[host]
	}
}

// Run starts the crawl with the given seed URLs.
func (e *Engine) Run(seeds []string) error {
	if err := e.initCrawl(seeds); err != nil {
		return err
	}
	e.seedFrontier(seeds)
	e.prefetchRobots()

	fetchCh, shutdown := e.startWorkers()

	// Dispatcher blocks until frontier is drained or context is cancelled
	e.dispatcher(fetchCh)

	shutdown()
	return nil
}

// initCrawl prepares the session, buffer, and persists the session row.
func (e *Engine) initCrawl(seeds []string) error {
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
	buf := storage.NewBuffer(e.store, e.cfg.Storage.BatchSize, e.cfg.Storage.FlushInterval, e.session.ID)
	e.bufferMu.Lock()
	e.buffer = buf
	e.bufferMu.Unlock()
	e.buffer.SetOnDataLost(func(lostPages, lostLinks int64) {
		applog.Errorf("crawler", "[%s] DATA LOSS: %d pages, %d links dropped — stopping crawl (disk full?)", e.session.ID, lostPages, lostLinks)
		e.Stop()
	})

	if err := e.store.InsertSession(e.ctx, e.session.ToStorageRow()); err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}

	// Initialize JS rendering pool if enabled
	e.renderMode = renderer.ParseDetectionMode(e.cfg.Crawler.JSRender.Mode)
	if e.renderMode != renderer.ModeOff {
		poolOpts := renderer.PoolOptions{
			MaxPages:       e.cfg.Crawler.JSRender.MaxPages,
			PageTimeout:    e.cfg.Crawler.JSRender.PageTimeout,
			UserAgent:      e.cfg.Crawler.UserAgent,
			BlockResources: e.cfg.Crawler.JSRender.BlockResources,
			Headless:       true,
		}
		pool, err := renderer.NewPool(poolOpts)
		if err != nil {
			applog.Warnf("crawler", "Failed to initialize JS rendering pool: %v — rendering disabled", err)
			e.renderMode = renderer.ModeOff
		} else {
			e.renderPool = pool
			e.renderWorkers = poolOpts.MaxPages
			applog.Infof("crawler", "JS rendering enabled (mode=%s, workers=%d)", e.renderMode, e.renderWorkers)
		}
	}

	applog.Infof("crawler", "Starting crawl session %s with %d seed(s), %d workers",
		e.session.ID, len(seeds), e.cfg.Crawler.Workers)
	return nil
}

// seedFrontier normalizes seed URLs and adds them to the frontier queue.
func (e *Engine) seedFrontier(seeds []string) {
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
}

// prefetchRobots fetches robots.txt for all seed hosts and discovers sitemaps.
func (e *Engine) prefetchRobots() {
	for _, seed := range e.session.SeedURLs {
		e.robots.IsAllowed(seed) // triggers fetch + cache
	}
	if e.fetchSitemaps {
		e.phase.Store("fetching_sitemaps")
		e.discoverAndPersistSitemaps()
	}
	e.phase.Store("crawling")
}

// startWorkers launches all worker goroutines and returns the pipeline channels
// along with a shutdown function that must be called after the dispatcher returns.
func (e *Engine) startWorkers() (chan *frontier.CrawlURL, func()) {
	fetchCh := make(chan *frontier.CrawlURL, e.cfg.Crawler.Workers)
	parseCh := make(chan *fetcher.FetchResult, e.cfg.Crawler.Workers)

	var fetchWg sync.WaitGroup
	numFetchWorkers := e.cfg.Crawler.Workers
	for i := 0; i < numFetchWorkers; i++ {
		fetchWg.Add(1)
		go func(id int) {
			defer fetchWg.Done()
			defer func() {
				if r := recover(); r != nil {
					applog.Errorf("crawler", "panic in fetch worker %d: %v\n%s", id, r, debug.Stack())
				}
			}()
			e.fetchWorker(id, fetchCh, parseCh)
		}(i)
	}

	var parseWg sync.WaitGroup
	numParseWorkers := max(1, numFetchWorkers/2)
	for i := 0; i < numParseWorkers; i++ {
		parseWg.Add(1)
		go func(id int) {
			defer parseWg.Done()
			defer func() {
				if r := recover(); r != nil {
					applog.Errorf("crawler", "panic in parse worker %d: %v\n%s", id, r, debug.Stack())
				}
			}()
			e.parseWorker(id, parseCh)
		}(i)
	}

	var retryWg sync.WaitGroup
	retryCtx, retryCancel := context.WithCancel(e.ctx)
	retryWg.Add(1)
	go func() {
		defer retryWg.Done()
		defer func() {
			if r := recover(); r != nil {
				applog.Errorf("crawler", "panic in retry dispatcher: %v\n%s", r, debug.Stack())
			}
		}()
		e.retryDispatcher(retryCtx, fetchCh)
	}()

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
				defer func() {
					if r := recover(); r != nil {
						applog.Errorf("crawler", "panic in external check worker: %v\n%s", r, debug.Stack())
					}
				}()
				e.externalCheckWorker()
			}()
		}
		applog.Infof("crawler", "Started %d external link check workers", numExtWorkers)
	}

	var renderWg sync.WaitGroup
	if e.renderPool != nil {
		e.renderCh = make(chan *renderItem, e.renderWorkers*2)
		for i := 0; i < e.renderWorkers; i++ {
			renderWg.Add(1)
			go func(id int) {
				defer renderWg.Done()
				defer func() {
					if r := recover(); r != nil {
						applog.Errorf("crawler", "panic in render worker %d: %v\n%s", id, r, debug.Stack())
					}
				}()
				e.renderWorker(id, e.renderCh)
			}(i)
		}
		applog.Infof("crawler", "Started %d render workers", e.renderWorkers)
	}

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
				defer func() {
					if r := recover(); r != nil {
						applog.Errorf("crawler", "panic in resource check worker: %v\n%s", r, debug.Stack())
					}
				}()
				e.resourceCheckWorker()
			}()
		}
		applog.Infof("crawler", "Started %d resource check workers", numResWorkers)
	}

	shutdown := func() {
		// Stop retry dispatcher, then close fetch channel
		retryCancel()
		retryWg.Wait()
		close(fetchCh)

		// Wait for pipeline to drain
		fetchWg.Wait()
		close(parseCh)
		parseWg.Wait()

		// Shutdown render workers
		if e.renderCh != nil {
			close(e.renderCh)
			renderWg.Wait()
		}
		if e.renderPool != nil {
			e.renderPool.Close()
		}

		// Cancel engine context to abort in-flight external/resource check HTTP requests.
		// Pages and links are already fully processed at this point.
		e.cancel()

		// Shutdown optional workers (they'll drain quickly now that context is cancelled)
		if e.checkExternal && e.externalCh != nil {
			close(e.externalCh)
			extWg.Wait()
			e.flushExternalChecks()
		}
		if e.checkResources && e.resourceCh != nil {
			close(e.resourceCh)
			resWg.Wait()
			e.flushResourceChecks()
			e.flushResourceRefs()
		}

		// Final buffer flush
		e.buffer.Close()

		bufState := e.buffer.ErrorState()
		if bufState.LostPages > 0 || bufState.LostLinks > 0 {
			applog.Warnf("crawler", "[%s] Crawl ended with data loss: %d pages, %d links dropped",
				e.session.ID, bufState.LostPages, bufState.LostLinks)
		}

		e.persistRobotsData()
		e.finalizeSession(bufState)
	}

	return fetchCh, shutdown
}

// BufferState returns the current buffer error state for monitoring.
func (e *Engine) BufferState() storage.BufferErrorState {
	e.bufferMu.RLock()
	buf := e.buffer
	e.bufferMu.RUnlock()
	if buf == nil {
		return storage.BufferErrorState{}
	}
	return buf.ErrorState()
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
			// If frontier is empty and idle long enough, we're done
			if e.front.Len() == 0 && emptyCount > 50 {
				if e.pendingRetries.Load() == 0 {
					return
				}
				// Force exit if no progress for 30 seconds
				lastProgress := e.lastProgressAt.Load()
				if lastProgress > 0 && time.Now().Unix()-lastProgress > 30 {
					applog.Infof("crawler", "No progress for 30s with empty frontier, finishing (pending retries: %d)", e.pendingRetries.Load())
					return
				}
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
			e.lastProgressAt.Store(time.Now().Unix())
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

		// Run extractors if configured
		if len(e.extractors) > 0 && result.IsHTML() && len(result.Body) > 0 {
			rows := extraction.RunExtractors(result.Body, result.URL, e.session.ID, e.extractors, now)
			if len(rows) > 0 {
				e.bufferMu.RLock()
				buf := e.buffer
				e.bufferMu.RUnlock()
				if buf != nil {
					buf.AddExtractions(rows)
				}
			}
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
				pageRow.ContentHash = pageData.ContentHash
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
								priority := newDepth
								// Boost priority for URLs found in sitemaps
								if e.sitemapURLSet[link.TargetURL] && priority > 1 {
									priority = 1
								}
								e.front.Add(frontier.CrawlURL{
									URL:      link.TargetURL,
									Priority: priority,
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

				// JS rendering: check if this page needs rendering
				if e.renderPool != nil && renderer.NeedsRendering(e.renderMode, result.Body, pageData) {
					ensureNonNilArrays(&pageRow)
					select {
					case e.renderCh <- &renderItem{
						result:     result,
						staticData: pageData,
						pageRow:    pageRow,
						linkRows:   linkRows,
					}:
						continue // renderWorker will handle buffer.AddPage
					case <-e.ctx.Done():
						// Context cancelled, fall through to store static data
					}
				}
			}
		}

		ensureNonNilArrays(&pageRow)
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
	client := e.newCheckClient()
	for rawURL := range e.externalCh {
		if e.ctx.Err() != nil {
			continue // Skip remaining items during shutdown
		}
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
		if err := e.store.InsertExternalLinkChecks(e.ctx, batch); err != nil {
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.store.InsertExternalLinkChecks(ctx, batch); err != nil {
			applog.Warnf("crawler", "failed to flush external link checks: %v", err)
		} else {
			applog.Infof("crawler", "Flushed %d external link checks", len(batch))
		}
	}
}

// resourceCheckWorker checks resource URLs and buffers the results.
func (e *Engine) resourceCheckWorker() {
	client := e.newCheckClient()
	for item := range e.resourceCh {
		if e.ctx.Err() != nil {
			continue // Skip remaining items during shutdown
		}
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
		if err := e.store.InsertPageResourceChecks(e.ctx, batch); err != nil {
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.store.InsertPageResourceChecks(ctx, batch); err != nil {
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
		if err := e.store.InsertPageResourceRefs(e.ctx, batch); err != nil {
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.store.InsertPageResourceRefs(ctx, batch); err != nil {
			applog.Warnf("crawler", "failed to flush page resource refs: %v", err)
		} else {
			applog.Infof("crawler", "Flushed %d page resource refs", len(batch))
		}
	}
}

// discoverAndPersistSitemaps fetches sitemaps from robots.txt directives and persists them.
func (e *Engine) discoverAndPersistSitemaps() {
	sitemapURLs := e.robots.SitemapURLs()
	if len(sitemapURLs) == 0 {
		return
	}

	now := time.Now()
	sitemapEntries := fetcher.DiscoverSitemaps(e.ctx, e.fetch.Client(), e.cfg.Crawler.UserAgent, sitemapURLs)

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

	if err := e.store.InsertSitemaps(e.ctx, sitemapRows); err != nil {
		applog.Warnf("crawler", "failed to persist sitemaps: %v", err)
	}
	if err := e.store.InsertSitemapURLs(e.ctx, sitemapURLRows); err != nil {
		applog.Warnf("crawler", "failed to persist sitemap URLs: %v", err)
	}
	if len(sitemapRows) > 0 {
		applog.Infof("crawler", "Persisted %d sitemaps (%d URLs total)", len(sitemapRows), len(sitemapURLRows))
	}

	// Build sitemap URL set for priority boosting (used in both modes)
	if len(sitemapURLRows) > 0 {
		e.sitemapURLSet = make(map[string]bool, len(sitemapURLRows))
		for _, su := range sitemapURLRows {
			norm, err := normalizer.Normalize(su.Loc)
			if err != nil {
				continue
			}
			e.sitemapURLSet[norm] = true
		}
		applog.Infof("crawler", "Built sitemap URL set: %d URLs for priority boosting", len(e.sitemapURLSet))
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

// persistRobotsData saves cached robots.txt data to storage.
func (e *Engine) persistRobotsData() {
	entries := e.robots.Entries()
	if len(entries) == 0 {
		return
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := e.store.InsertRobotsData(ctx, robotsRows); err != nil {
		applog.Warnf("crawler", "failed to persist robots.txt data: %v", err)
	} else {
		applog.Infof("crawler", "Persisted robots.txt for %d hosts", len(robotsRows))
	}
}

// finalizeSession recomputes depths and PageRank, then updates the session status.
// Uses a dedicated context because the crawl context (e.ctx) may already be cancelled.
func (e *Engine) finalizeSession(bufState storage.BufferErrorState) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := e.store.RecomputeDepths(ctx, e.session.ID, e.session.SeedURLs); err != nil {
		applog.Warnf("crawler", "depth recomputation failed: %v", err)
	}
	if err := e.store.ComputePageRank(ctx, e.session.ID); err != nil {
		applog.Warnf("crawler", "PageRank computation failed: %v", err)
	}

	if bufState.LostPages > 0 || bufState.LostLinks > 0 {
		e.session.Status = "completed_with_errors"
	} else {
		e.session.Status = "completed"
	}
	if total, err := e.store.CountPages(ctx, e.session.ID); err == nil {
		e.session.Pages = total
	} else {
		e.session.Pages = uint64(e.pagesCrawled.Load())
	}
	row := e.session.ToStorageRow()
	row.FinishedAt = time.Now()
	if err := e.store.InsertSession(ctx, row); err != nil {
		applog.Errorf("crawler", "updating session: %v", err)
	}

	applog.Infof("crawler", "Crawl complete: %d pages crawled, session %s",
		e.pagesCrawled.Load(), e.session.ID)
}

// newCheckClient creates an HTTP client for external/resource check workers.
func (e *Engine) newCheckClient() *http.Client {
	dialOpts := fetcher.DialOptions{
		SourceIP:        e.cfg.Crawler.SourceIP,
		ForceIPv4:       e.cfg.Crawler.ForceIPv4,
		AllowPrivateIPs: e.cfg.Crawler.AllowPrivateIPs,
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: fetcher.SafeDialContextWithOpts(dialOpts),
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// renderWorker receives pages that need JS rendering, renders them, computes diffs, and stores results.
func (e *Engine) renderWorker(id int, in <-chan *renderItem) {
	for item := range in {
		select {
		case <-e.ctx.Done():
			// Store static data on cancellation
			e.buffer.AddPage(item.pageRow)
			if len(item.linkRows) > 0 {
				e.buffer.AddLinks(item.linkRows)
			}
			continue
		default:
		}

		finalURL := item.result.FinalURL
		if finalURL == "" {
			finalURL = item.result.URL
		}

		// Create a timeout context for rendering
		renderCtx, renderCancel := context.WithTimeout(e.ctx, e.cfg.Crawler.JSRender.PageTimeout)
		renderResult := e.renderPool.Render(renderCtx, finalURL)
		renderCancel()

		item.pageRow.JSRendered = true
		item.pageRow.JSRenderDurationMs = uint64(renderResult.RenderDuration.Milliseconds())

		if renderResult.Error != nil {
			item.pageRow.JSRenderError = renderResult.Error.Error()
			applog.Warnf("crawler", "Render error for %s: %v", finalURL, renderResult.Error)
		} else {
			// Re-parse rendered HTML
			renderedData, err := parser.Parse([]byte(renderResult.RenderedHTML), finalURL)
			if err != nil {
				item.pageRow.JSRenderError = fmt.Sprintf("parse rendered: %v", err)
			} else {
				// Populate rendered fields
				item.pageRow.RenderedTitle = renderedData.Title
				item.pageRow.RenderedMetaDescription = renderedData.MetaDescription
				item.pageRow.RenderedH1 = renderedData.H1
				item.pageRow.RenderedWordCount = uint32(renderedData.WordCount)
				item.pageRow.RenderedCanonical = renderedData.Canonical
				item.pageRow.RenderedMetaRobots = renderedData.MetaRobots
				item.pageRow.RenderedSchemaTypes = renderedData.SchemaTypes

				// Count rendered links and images
				item.pageRow.RenderedLinksCount = uint32(len(renderedData.Links))
				item.pageRow.RenderedImagesCount = uint16(len(renderedData.Images))

				// Store rendered HTML if store_html is enabled
				if e.cfg.Crawler.StoreHTML {
					item.pageRow.RenderedBodyHTML = renderResult.RenderedHTML
				}

				// Compute diffs
				computeJSDiffs(&item.pageRow, item.staticData, renderedData)

				// Discover new links from rendered content
				if e.followJSLinks {
					for _, link := range renderedData.Links {
						if link.IsInternal && e.isInScope(link.TargetURL) {
							newDepth := item.result.Depth + 1
							if e.cfg.Crawler.MaxDepth == 0 || newDepth <= e.cfg.Crawler.MaxDepth {
								priority := newDepth
								if e.sitemapURLSet[link.TargetURL] && priority > 1 {
									priority = 1
								}
								e.front.Add(frontier.CrawlURL{
									URL:      link.TargetURL,
									Priority: priority,
									Depth:    newDepth,
									FoundOn:  item.result.URL,
								})
							}
						}
					}
				}
			}
		}

		ensureNonNilArrays(&item.pageRow)
		e.buffer.AddPage(item.pageRow)
		if len(item.linkRows) > 0 {
			e.buffer.AddLinks(item.linkRows)
		}
	}
}

// computeJSDiffs compares static and rendered page data and sets diff flags.
func computeJSDiffs(row *storage.PageRow, static, rendered *parser.PageData) {
	// Title
	row.JSChangedTitle = strings.TrimSpace(static.Title) != strings.TrimSpace(rendered.Title)

	// Description
	row.JSChangedDescription = strings.TrimSpace(static.MetaDescription) != strings.TrimSpace(rendered.MetaDescription)

	// H1
	row.JSChangedH1 = !stringSlicesEqual(static.H1, rendered.H1)

	// Canonical
	row.JSChangedCanonical = strings.TrimSpace(static.Canonical) != strings.TrimSpace(rendered.Canonical)

	// Content: word count changed by >20%
	if static.WordCount > 0 {
		delta := float64(rendered.WordCount-static.WordCount) / float64(static.WordCount)
		if delta < 0 {
			delta = -delta
		}
		row.JSChangedContent = delta > 0.2
	} else {
		row.JSChangedContent = rendered.WordCount > 50
	}

	// Links delta
	row.JSAddedLinks = int32(len(rendered.Links)) - int32(len(static.Links))

	// Images delta
	row.JSAddedImages = int32(len(rendered.Images)) - int32(len(static.Images))

	// Schema: check if rendered has types not in static
	staticSchemaSet := make(map[string]bool)
	for _, t := range static.SchemaTypes {
		staticSchemaSet[t] = true
	}
	for _, t := range rendered.SchemaTypes {
		if !staticSchemaSet[t] {
			row.JSAddedSchema = true
			break
		}
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ensureNonNilArrays replaces nil slices/maps with empty values for ClickHouse compatibility.
func ensureNonNilArrays(row *storage.PageRow) {
	if row.H1 == nil {
		row.H1 = []string{}
	}
	if row.H2 == nil {
		row.H2 = []string{}
	}
	if row.H3 == nil {
		row.H3 = []string{}
	}
	if row.H4 == nil {
		row.H4 = []string{}
	}
	if row.H5 == nil {
		row.H5 = []string{}
	}
	if row.H6 == nil {
		row.H6 = []string{}
	}
	if row.Headers == nil {
		row.Headers = map[string]string{}
	}
	if row.RedirectChain == nil {
		row.RedirectChain = []storage.RedirectHopRow{}
	}
	if row.Hreflang == nil {
		row.Hreflang = []storage.HreflangRow{}
	}
	if row.SchemaTypes == nil {
		row.SchemaTypes = []string{}
	}
	if row.RenderedH1 == nil {
		row.RenderedH1 = []string{}
	}
	if row.RenderedSchemaTypes == nil {
		row.RenderedSchemaTypes = []string{}
	}
}
