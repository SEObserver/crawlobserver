package crawler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/fetcher"
	"github.com/SEObserver/seocrawler/internal/frontier"
	"github.com/SEObserver/seocrawler/internal/normalizer"
	"github.com/SEObserver/seocrawler/internal/parser"
	"github.com/SEObserver/seocrawler/internal/storage"
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

	ctx    context.Context
	cancel context.CancelFunc
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
	e.buffer = storage.NewBuffer(e.store, e.cfg.Storage.BatchSize, e.cfg.Storage.FlushInterval, e.session.ID)

	// Save session to ClickHouse
	if err := e.store.InsertSession(e.ctx, e.session.ToStorageRow()); err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}

	log.Printf("Starting crawl session %s with %d seed(s), %d workers",
		e.session.ID, len(seeds), e.cfg.Crawler.Workers)

	// Seed the frontier
	for i, seed := range seeds {
		normalized, err := normalizer.Normalize(seed)
		if err != nil {
			log.Printf("WARNING: skipping invalid seed %q: %v", seed, err)
			continue
		}
		e.front.Add(frontier.CrawlURL{
			URL:      normalized,
			Priority: i,
			Depth:    0,
		})
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

	// Dispatcher: feeds URLs from frontier to fetch workers
	e.dispatcher(fetchCh)

	// Wait for fetch workers to finish
	wg.Wait()
	close(parseCh)

	// Wait for parse workers to finish
	parseWg.Wait()

	// Final flush
	e.buffer.Close()

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
			log.Printf("WARNING: failed to persist robots.txt data: %v", err)
		} else {
			log.Printf("Persisted robots.txt for %d hosts", len(robotsRows))
		}
	}

	// Recompute depths via BFS
	if err := e.store.RecomputeDepths(context.Background(), e.session.ID, e.session.SeedURLs); err != nil {
		log.Printf("WARNING: depth recomputation failed: %v", err)
	}

	// Compute internal PageRank
	if err := e.store.ComputePageRank(context.Background(), e.session.ID); err != nil {
		log.Printf("WARNING: PageRank computation failed: %v", err)
	}

	// Update session status
	e.session.Status = "completed"
	e.session.Pages = uint64(e.pagesCrawled.Load())
	row := e.session.ToStorageRow()
	row.FinishedAt = time.Now()
	if err := e.store.InsertSession(context.Background(), row); err != nil {
		log.Printf("ERROR updating session: %v", err)
	}

	log.Printf("Crawl complete: %d pages crawled, session %s",
		e.pagesCrawled.Load(), e.session.ID)

	return nil
}

// Stop gracefully stops the engine.
func (e *Engine) Stop() {
	log.Println("Stopping crawl engine...")
	e.cancel()
	e.front.Close()
}

func (e *Engine) dispatcher(fetchCh chan<- *frontier.CrawlURL) {
	defer close(fetchCh)

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
			log.Printf("Reached max pages limit (%d)", e.maxPages)
			return
		}

		next := e.front.Next()
		if next == nil {
			emptyCount++
			// If frontier is empty and all pages have been dispatched, we're done
			if e.front.Len() == 0 && emptyCount > 50 {
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
			log.Printf("Blocked by robots.txt: %s", crawlURL.URL)
			continue
		}

		result := e.fetch.Fetch(crawlURL.URL, crawlURL.Depth, crawlURL.FoundOn)
		e.pagesCrawled.Add(1)

		count := e.pagesCrawled.Load()
		if count%100 == 0 {
			log.Printf("Progress: %d pages crawled, %d in queue", count, e.front.Len())
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
				log.Printf("Parse error for %s: %v", result.URL, err)
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
						newDepth := result.Depth + 1
						if e.cfg.Crawler.MaxDepth == 0 || newDepth <= e.cfg.Crawler.MaxDepth {
							e.front.Add(frontier.CrawlURL{
								URL:      link.TargetURL,
								Priority: newDepth,
								Depth:    newDepth,
								FoundOn:  result.URL,
							})
						}
					} else {
						externalOut++
					}
				}
				pageRow.InternalLinksOut = internalOut
				pageRow.ExternalLinksOut = externalOut

				if len(linkRows) > 0 {
					e.buffer.AddLinks(linkRows)
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
