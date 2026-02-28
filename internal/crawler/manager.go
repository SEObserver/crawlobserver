package crawler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// Manager manages running crawl engines.
type Manager struct {
	mu      sync.RWMutex
	engines map[string]*Engine // sessionID -> engine
	cfg     *config.Config
	store   *storage.Store
}

// NewManager creates a new crawl manager.
func NewManager(cfg *config.Config, store *storage.Store) *Manager {
	return &Manager{
		engines: make(map[string]*Engine),
		cfg:     cfg,
		store:   store,
	}
}

// CrawlRequest holds parameters for starting a new crawl.
type CrawlRequest struct {
	Seeds               []string `json:"seeds"`
	MaxPages            int      `json:"max_pages"`
	MaxDepth            int      `json:"max_depth"`
	Workers             int      `json:"workers"`
	Delay               string   `json:"delay"`
	StoreHTML           bool     `json:"store_html"`
	CrawlScope          string   `json:"crawl_scope"`
	ProjectID           *string  `json:"project_id"`
	CheckExternalLinks  *bool    `json:"check_external_links"`
	ExternalLinkWorkers int      `json:"external_link_workers"`
	RetryStatusCode     int      `json:"retry_status_code"`
	UserAgent           string   `json:"user_agent"`
	CrawlSitemapOnly    bool     `json:"crawl_sitemap_only"`
}

// StartCrawl launches a new crawl session in background. Returns the session ID.
func (m *Manager) StartCrawl(req CrawlRequest) (string, error) {
	if len(req.Seeds) == 0 {
		return "", fmt.Errorf("at least one seed URL is required")
	}

	// Build config overrides
	cfg := *m.cfg
	crawlerCfg := cfg.Crawler
	if req.MaxPages > 0 {
		crawlerCfg.MaxPages = req.MaxPages
	}
	if req.MaxDepth > 0 {
		crawlerCfg.MaxDepth = req.MaxDepth
	}
	if req.Workers > 0 {
		crawlerCfg.Workers = req.Workers
	}
	if req.Delay != "" {
		if d, err := parseDuration(req.Delay); err == nil {
			crawlerCfg.Delay = d
		}
	}
	crawlerCfg.StoreHTML = req.StoreHTML
	if req.CrawlScope != "" {
		crawlerCfg.CrawlScope = req.CrawlScope
	}
	cfg.Crawler = crawlerCfg

	if req.UserAgent != "" {
		cfg.Crawler.UserAgent = req.UserAgent
	}

	engine := NewEngine(&cfg, m.store)
	sessionID := engine.SessionID(req.Seeds)
	engine.session.ProjectID = req.ProjectID
	engine.sitemapOnly = req.CrawlSitemapOnly

	// External link checking: default true
	engine.checkExternal = req.CheckExternalLinks == nil || *req.CheckExternalLinks
	engine.externalWorkers = req.ExternalLinkWorkers
	if engine.externalWorkers <= 0 {
		engine.externalWorkers = 3
	}

	m.mu.Lock()
	m.engines[sessionID] = engine
	m.mu.Unlock()

	// Run in background
	go func() {
		if err := engine.Run(req.Seeds); err != nil {
			log.Printf("Crawl %s failed: %v", sessionID, err)
		}
		m.mu.Lock()
		delete(m.engines, sessionID)
		m.mu.Unlock()
	}()

	return sessionID, nil
}

// StopCrawl stops a running crawl session.
func (m *Manager) StopCrawl(sessionID string) error {
	m.mu.RLock()
	engine, ok := m.engines[sessionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session %s is not running", sessionID)
	}

	engine.Stop()
	return nil
}

// IsRunning checks if a session is currently running.
func (m *Manager) IsRunning(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.engines[sessionID]
	return ok
}

// Progress returns current crawl progress for a running session.
func (m *Manager) Progress(sessionID string) (int64, int, bool) {
	m.mu.RLock()
	engine, ok := m.engines[sessionID]
	m.mu.RUnlock()
	if !ok {
		return 0, 0, false
	}
	return engine.PagesCrawled(), engine.QueueLen(), true
}

// BufferState returns the buffer error state for a running session.
func (m *Manager) BufferState(sessionID string) storage.BufferErrorState {
	m.mu.RLock()
	engine, ok := m.engines[sessionID]
	m.mu.RUnlock()
	if !ok {
		return storage.BufferErrorState{}
	}
	return engine.BufferState()
}

// ActiveSessions returns IDs of currently running sessions.
func (m *Manager) ActiveSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.engines))
	for id := range m.engines {
		ids = append(ids, id)
	}
	return ids
}

// ResumeCrawl resumes a stopped/completed session by re-crawling undiscovered links.
// If overrides is non-nil, its non-zero fields override the default config.
func (m *Manager) ResumeCrawl(sessionID string, overrides *CrawlRequest) (string, error) {
	m.mu.RLock()
	_, running := m.engines[sessionID]
	m.mu.RUnlock()
	if running {
		return "", fmt.Errorf("session %s is already running", sessionID)
	}

	// Get uncrawled URLs from storage
	uncrawled, err := m.store.UncrawledURLs(context.Background(), sessionID)
	if err != nil {
		return "", fmt.Errorf("fetching uncrawled URLs: %w", err)
	}
	if len(uncrawled) == 0 {
		return "", fmt.Errorf("no uncrawled URLs found for session %s", sessionID)
	}

	// Get already crawled URLs to pre-seed dedup
	crawled, err := m.store.CrawledURLs(context.Background(), sessionID)
	if err != nil {
		return "", fmt.Errorf("fetching crawled URLs: %w", err)
	}

	// Get original session info to preserve seed URLs
	originalSession, err := m.store.GetSession(context.Background(), sessionID)
	if err != nil {
		return "", fmt.Errorf("fetching original session: %w", err)
	}

	log.Printf("Resuming session %s with %d uncrawled URLs (%d already crawled)",
		sessionID, len(uncrawled), len(crawled))

	cfg := *m.cfg
	if overrides != nil {
		crawlerCfg := cfg.Crawler
		if overrides.MaxPages > 0 {
			crawlerCfg.MaxPages = overrides.MaxPages
		}
		if overrides.MaxDepth > 0 {
			crawlerCfg.MaxDepth = overrides.MaxDepth
		}
		if overrides.Workers > 0 {
			crawlerCfg.Workers = overrides.Workers
		}
		if overrides.Delay != "" {
			if d, err := parseDuration(overrides.Delay); err == nil {
				crawlerCfg.Delay = d
			}
		}
		crawlerCfg.StoreHTML = overrides.StoreHTML
		if overrides.CrawlScope != "" {
			crawlerCfg.CrawlScope = overrides.CrawlScope
		}
		if overrides.UserAgent != "" {
			crawlerCfg.UserAgent = overrides.UserAgent
		}
		cfg.Crawler = crawlerCfg
	}
	engine := NewEngine(&cfg, m.store)
	engine.sitemapOnly = overrides != nil && overrides.CrawlSitemapOnly

	// Restore the original session with its seed URLs, not the uncrawled URLs
	engine.ResumeSession(sessionID, originalSession.SeedURLs)

	// Pre-seed dedup with already crawled URLs
	engine.PreSeedDedup(crawled)

	m.mu.Lock()
	m.engines[sessionID] = engine
	m.mu.Unlock()

	go func() {
		if err := engine.Run(uncrawled); err != nil {
			log.Printf("Resumed crawl %s failed: %v", sessionID, err)
		}
		m.mu.Lock()
		delete(m.engines, sessionID)
		m.mu.Unlock()
	}()

	return sessionID, nil
}

// RetryFailed retries pages with status_code = 0 (fetch errors) or a specific status code.
// Deletes the failed rows, then runs a mini-crawl with those URLs.
func (m *Manager) RetryFailed(sessionID string, overrides *CrawlRequest) (int, error) {
	statusCode := 0
	if overrides != nil && overrides.RetryStatusCode > 0 {
		statusCode = overrides.RetryStatusCode
	}

	m.mu.RLock()
	_, running := m.engines[sessionID]
	m.mu.RUnlock()
	if running {
		return 0, fmt.Errorf("session %s is already running", sessionID)
	}

	var failedURLs []string
	var deleted int
	var err error

	if statusCode == 0 {
		// Original behavior: retry status_code=0
		failedURLs, err = m.store.FailedURLs(context.Background(), sessionID)
		if err != nil {
			return 0, fmt.Errorf("fetching failed URLs: %w", err)
		}
		if len(failedURLs) == 0 {
			return 0, fmt.Errorf("no failed pages (status 0) found for session %s", sessionID)
		}
		deleted, err = m.store.DeleteFailedPages(context.Background(), sessionID)
		if err != nil {
			return 0, fmt.Errorf("deleting failed pages: %w", err)
		}
	} else {
		// Retry pages with specific status code
		failedURLs, err = m.store.URLsByStatus(context.Background(), sessionID, statusCode)
		if err != nil {
			return 0, fmt.Errorf("fetching URLs with status %d: %w", statusCode, err)
		}
		if len(failedURLs) == 0 {
			return 0, fmt.Errorf("no pages with status %d found for session %s", statusCode, sessionID)
		}
		deleted, err = m.store.DeletePagesByStatus(context.Background(), sessionID, statusCode)
		if err != nil {
			return 0, fmt.Errorf("deleting pages with status %d: %w", statusCode, err)
		}
	}

	// Get already crawled URLs (minus the deleted ones) for dedup
	crawled, err := m.store.CrawledURLs(context.Background(), sessionID)
	if err != nil {
		return 0, fmt.Errorf("fetching crawled URLs: %w", err)
	}

	// Get original session
	originalSession, err := m.store.GetSession(context.Background(), sessionID)
	if err != nil {
		return 0, fmt.Errorf("fetching original session: %w", err)
	}

	log.Printf("Retrying %d failed URLs for session %s", len(failedURLs), sessionID)

	cfg := *m.cfg
	if overrides != nil {
		crawlerCfg := cfg.Crawler
		if overrides.Workers > 0 {
			crawlerCfg.Workers = overrides.Workers
		}
		if overrides.Delay != "" {
			if d, err := parseDuration(overrides.Delay); err == nil {
				crawlerCfg.Delay = d
			}
		}
		cfg.Crawler = crawlerCfg
	}
	cfg.Crawler.MaxPages = len(failedURLs)

	engine := NewEngine(&cfg, m.store)
	engine.ResumeSession(sessionID, originalSession.SeedURLs)
	engine.PreSeedDedup(crawled)

	m.mu.Lock()
	m.engines[sessionID] = engine
	m.mu.Unlock()

	go func() {
		if err := engine.Run(failedURLs); err != nil {
			log.Printf("Retry crawl %s failed: %v", sessionID, err)
		}
		m.mu.Lock()
		delete(m.engines, sessionID)
		m.mu.Unlock()
	}()

	return deleted, nil
}
