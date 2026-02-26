package crawler

import (
	"context"
	"fmt"
	"log"
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
		e.session.SeedURLs = seeds
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

		// Check robots.txt
		if e.cfg.Crawler.RespectRobots && !e.robots.IsAllowed(crawlURL.URL) {
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
			FetchDurationMs: uint64(result.Duration.Milliseconds()),
			Error:           result.Error,
			Depth:           uint16(result.Depth),
			FoundOn:         result.FoundOn,
			CrawledAt:       now,
			Headers:         result.Headers,
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
				pageRow.Canonical = pageData.Canonical
				pageRow.MetaRobots = pageData.MetaRobots
				pageRow.MetaDescription = pageData.MetaDescription
				pageRow.H1 = pageData.H1
				pageRow.H2 = pageData.H2
				pageRow.H3 = pageData.H3
				pageRow.H4 = pageData.H4
				pageRow.H5 = pageData.H5
				pageRow.H6 = pageData.H6

				// Process links
				var linkRows []storage.LinkRow
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

					// Add internal links to frontier
					if link.IsInternal {
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
				}

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

		e.buffer.AddPage(pageRow)
	}
}
