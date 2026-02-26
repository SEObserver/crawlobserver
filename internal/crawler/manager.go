package crawler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/storage"
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
	Seeds     []string `json:"seeds"`
	MaxPages  int      `json:"max_pages"`
	MaxDepth  int      `json:"max_depth"`
	Workers   int      `json:"workers"`
	Delay     string   `json:"delay"`
	StoreHTML bool     `json:"store_html"`
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
	cfg.Crawler = crawlerCfg

	engine := NewEngine(&cfg, m.store)
	sessionID := engine.SessionID(req.Seeds)

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
	uncrawled, err := m.store.UncrawledURLs(sessionID)
	if err != nil {
		return "", fmt.Errorf("fetching uncrawled URLs: %w", err)
	}
	if len(uncrawled) == 0 {
		return "", fmt.Errorf("no uncrawled URLs found for session %s", sessionID)
	}

	// Get already crawled URLs to pre-seed dedup
	crawled, err := m.store.CrawledURLs(sessionID)
	if err != nil {
		return "", fmt.Errorf("fetching crawled URLs: %w", err)
	}

	// Get original session info to preserve seed URLs
	originalSession, err := m.store.GetSession(sessionID)
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
		cfg.Crawler = crawlerCfg
	}
	engine := NewEngine(&cfg, m.store)

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
