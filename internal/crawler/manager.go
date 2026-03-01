package crawler

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

const (
	defaultExternalWorkers = 3
	defaultResourceWorkers = 3
	maxLastErrors          = 1000
	defaultMaxPages        = 100000
)

// queuedCrawl holds a crawl waiting for a semaphore slot.
type queuedCrawl struct {
	sessionID string
	engine    *Engine
	seeds     []string
}

// Manager manages running crawl engines.
type Manager struct {
	mu         sync.RWMutex
	engines    map[string]*Engine // sessionID -> engine
	lastErrors map[string]string  // sessionID -> error message (persists after engine cleanup)
	cfg        *config.Config
	store      *storage.Store

	sem       chan struct{}    // semaphore limiting concurrent sessions
	queueMu   sync.Mutex
	queue      []queuedCrawl   // FIFO of crawls waiting for a slot
	queuedSet  map[string]bool // sessionID -> true for fast lookup
}

// NewManager creates a new crawl manager.
func NewManager(cfg *config.Config, store *storage.Store) *Manager {
	maxSessions := cfg.Crawler.MaxConcurrentSessions
	if maxSessions <= 0 {
		maxSessions = 20
	}
	return &Manager{
		engines:    make(map[string]*Engine),
		lastErrors: make(map[string]string),
		cfg:        cfg,
		store:      store,
		sem:        make(chan struct{}, maxSessions),
		queuedSet:  make(map[string]bool),
	}
}

// LastError returns the error message from the last run of a session, if any.
func (m *Manager) LastError(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastErrors[sessionID]
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
	CheckPageResources  *bool    `json:"check_page_resources"`
	ResourceWorkers     int      `json:"resource_workers"`
	TLSProfile          string   `json:"tls_profile"`
	JSRenderMode        string   `json:"js_render_mode"`
	JSRenderMaxPages    int      `json:"js_render_max_pages"`
	JSRenderTimeout     string   `json:"js_render_timeout"`
	FollowJSLinks       bool     `json:"follow_js_links"`
}

// StartCrawl launches a new crawl session in background. Returns the session ID.
// If all semaphore slots are taken, the crawl is queued and starts automatically
// when a slot becomes available.
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
		if d, err := time.ParseDuration(req.Delay); err == nil {
			crawlerCfg.Delay = d
		}
	}
	crawlerCfg.StoreHTML = req.StoreHTML
	if req.CrawlScope != "" {
		crawlerCfg.CrawlScope = req.CrawlScope
	}

	// Guardrails: cap workers to MaxWorkers
	maxWorkers := m.cfg.Crawler.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 100
	}
	if crawlerCfg.Workers > maxWorkers {
		crawlerCfg.Workers = maxWorkers
	}

	// Guardrails: default max_pages to avoid infinite crawl
	if crawlerCfg.MaxPages <= 0 {
		crawlerCfg.MaxPages = defaultMaxPages
	}

	cfg.Crawler = crawlerCfg

	if req.UserAgent != "" {
		cfg.Crawler.UserAgent = req.UserAgent
	}
	if req.TLSProfile != "" {
		cfg.Crawler.TLSProfile = req.TLSProfile
	}

	// JS rendering overrides
	if req.JSRenderMode != "" {
		cfg.Crawler.JSRender.Mode = req.JSRenderMode
	}
	if req.JSRenderMaxPages > 0 {
		cfg.Crawler.JSRender.MaxPages = req.JSRenderMaxPages
	}
	if req.JSRenderTimeout != "" {
		if d, err := time.ParseDuration(req.JSRenderTimeout); err == nil {
			cfg.Crawler.JSRender.PageTimeout = d
		}
	}

	engine := NewEngine(&cfg, m.store)
	sessionID := engine.SessionID(req.Seeds)
	engine.session.ProjectID = req.ProjectID
	engine.sitemapOnly = req.CrawlSitemapOnly

	// External link checking: default true
	engine.checkExternal = req.CheckExternalLinks == nil || *req.CheckExternalLinks
	engine.externalWorkers = req.ExternalLinkWorkers
	if engine.externalWorkers <= 0 {
		engine.externalWorkers = defaultExternalWorkers
	}

	// Page resource checking: default true
	engine.checkResources = req.CheckPageResources == nil || *req.CheckPageResources
	engine.resourceWorkers = req.ResourceWorkers
	if engine.resourceWorkers <= 0 {
		engine.resourceWorkers = defaultResourceWorkers
	}

	// JS rendering
	engine.followJSLinks = req.FollowJSLinks

	// Try to acquire a semaphore slot (non-blocking)
	select {
	case m.sem <- struct{}{}:
		// Got a slot — start immediately
		m.mu.Lock()
		m.engines[sessionID] = engine
		m.mu.Unlock()
		go m.runEngine(sessionID, engine, req.Seeds)
	default:
		// All slots taken — enqueue
		m.enqueue(sessionID, engine, req.Seeds)
	}

	return sessionID, nil
}

// StopCrawl stops a running crawl session or removes it from the queue.
func (m *Manager) StopCrawl(sessionID string) error {
	// Check if queued first
	if m.dequeue(sessionID) {
		applog.Infof("crawler", "Removed queued session %s", sessionID)
		// Mark as stopped in ClickHouse
		ctx := context.Background()
		sess, err := m.store.GetSession(ctx, sessionID)
		if err == nil {
			sess.Status = "stopped"
			sess.FinishedAt = time.Now()
			_ = m.store.InsertSession(ctx, sess)
		}
		return nil
	}

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

	applog.Infof("crawler", "Resuming session %s with %d uncrawled URLs (%d already crawled)",
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
			if d, err := time.ParseDuration(overrides.Delay); err == nil {
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
		if overrides.TLSProfile != "" {
			crawlerCfg.TLSProfile = overrides.TLSProfile
		}
		cfg.Crawler = crawlerCfg
	}
	engine := NewEngine(&cfg, m.store)
	engine.sitemapOnly = overrides != nil && overrides.CrawlSitemapOnly

	// Restore the original session with its seed URLs, not the uncrawled URLs
	engine.ResumeSession(sessionID, originalSession.SeedURLs)
	engine.session.ProjectID = originalSession.ProjectID

	// Pre-seed dedup with already crawled URLs
	engine.PreSeedDedup(crawled)

	// Try to acquire a semaphore slot (non-blocking)
	select {
	case m.sem <- struct{}{}:
		m.mu.Lock()
		m.engines[sessionID] = engine
		m.mu.Unlock()
		go m.runEngine(sessionID, engine, uncrawled)
	default:
		m.enqueue(sessionID, engine, uncrawled)
	}

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

	applog.Infof("crawler", "Retrying %d failed URLs for session %s", len(failedURLs), sessionID)

	cfg := *m.cfg
	if overrides != nil {
		crawlerCfg := cfg.Crawler
		if overrides.Workers > 0 {
			crawlerCfg.Workers = overrides.Workers
		}
		if overrides.Delay != "" {
			if d, err := time.ParseDuration(overrides.Delay); err == nil {
				crawlerCfg.Delay = d
			}
		}
		cfg.Crawler = crawlerCfg
	}
	cfg.Crawler.MaxPages = len(failedURLs)

	engine := NewEngine(&cfg, m.store)
	engine.ResumeSession(sessionID, originalSession.SeedURLs)
	engine.session.ProjectID = originalSession.ProjectID
	engine.PreSeedDedup(crawled)

	// Try to acquire a semaphore slot (non-blocking)
	select {
	case m.sem <- struct{}{}:
		m.mu.Lock()
		m.engines[sessionID] = engine
		m.mu.Unlock()
		go m.runEngine(sessionID, engine, failedURLs)
	default:
		m.enqueue(sessionID, engine, failedURLs)
	}

	return deleted, nil
}

// markSessionCrashed updates a session's status to "crashed" in ClickHouse.
func (m *Manager) markSessionCrashed(sessionID string, engine *Engine, reason string) {
	ctx := context.Background()
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		applog.Errorf("crawler", "markSessionCrashed: could not load session %s: %v", sessionID, err)
		return
	}
	sess.Status = "crashed"
	sess.FinishedAt = time.Now()
	if engine != nil {
		sess.PagesCrawled = uint64(engine.PagesCrawled())
	}
	if err := m.store.InsertSession(ctx, sess); err != nil {
		applog.Errorf("crawler", "markSessionCrashed: could not update session %s: %v", sessionID, err)
	}
	applog.Warnf("crawler", "Session %s marked as crashed: %s", sessionID, reason)
}

// Shutdown gracefully stops all running engines within the given timeout.
// Engines still running after the timeout are marked as "crashed".
// Queued sessions are marked as "stopped".
func (m *Manager) Shutdown(timeout time.Duration) {
	// Drain the queue first
	m.queueMu.Lock()
	queued := m.queue
	m.queue = nil
	m.queuedSet = make(map[string]bool)
	m.queueMu.Unlock()
	for _, qc := range queued {
		ctx := context.Background()
		sess, err := m.store.GetSession(ctx, qc.sessionID)
		if err == nil {
			sess.Status = "stopped"
			sess.FinishedAt = time.Now()
			_ = m.store.InsertSession(ctx, sess)
		}
		applog.Infof("crawler", "Shutdown: queued session %s marked stopped", qc.sessionID)
	}

	m.mu.RLock()
	snapshot := make(map[string]*Engine, len(m.engines))
	for id, e := range m.engines {
		snapshot[id] = e
	}
	m.mu.RUnlock()

	if len(snapshot) == 0 {
		return
	}

	applog.Infof("crawler", "Shutdown: stopping %d running engine(s)...", len(snapshot))
	for _, e := range snapshot {
		e.Stop()
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			m.mu.Lock()
			remaining := make(map[string]*Engine, len(m.engines))
			for id, e := range m.engines {
				remaining[id] = e
			}
			m.mu.Unlock()
			for id, e := range remaining {
				applog.Errorf("crawler", "Shutdown timeout: engine %s still running, marking crashed", id)
				m.markSessionCrashed(id, e, "shutdown timeout")
			}
			return
		case <-ticker.C:
			m.mu.RLock()
			n := len(m.engines)
			m.mu.RUnlock()
			if n == 0 {
				applog.Info("crawler", "Shutdown: all engines stopped cleanly")
				return
			}
		}
	}
}

// RecoverOrphanedSessions marks any sessions still in "running" status as "crashed".
// Should be called at startup to clean up after a previous unclean shutdown.
func (m *Manager) RecoverOrphanedSessions(ctx context.Context) {
	sessions, err := m.store.ListSessions(ctx)
	if err != nil {
		applog.Errorf("crawler", "RecoverOrphanedSessions: could not list sessions: %v", err)
		return
	}
	for _, sess := range sessions {
		if sess.Status == "running" {
			sess.Status = "crashed"
			sess.FinishedAt = time.Now()
			if err := m.store.InsertSession(ctx, &sess); err != nil {
				applog.Errorf("crawler", "RecoverOrphanedSessions: could not update session %s: %v", sess.ID, err)
				continue
			}
			applog.Warnf("crawler", "Recovered orphaned session %s (was running, now crashed)", sess.ID)
		} else if sess.Status == "queued" {
			sess.Status = "stopped"
			sess.FinishedAt = time.Now()
			if err := m.store.InsertSession(ctx, &sess); err != nil {
				applog.Errorf("crawler", "RecoverOrphanedSessions: could not update session %s: %v", sess.ID, err)
				continue
			}
			applog.Warnf("crawler", "Recovered orphaned session %s (was queued, now stopped)", sess.ID)
		}
	}
}

// enqueue adds a crawl to the FIFO queue and persists a "queued" session in ClickHouse.
func (m *Manager) enqueue(sessionID string, engine *Engine, seeds []string) {
	m.queueMu.Lock()
	m.queue = append(m.queue, queuedCrawl{sessionID: sessionID, engine: engine, seeds: seeds})
	m.queuedSet[sessionID] = true
	m.queueMu.Unlock()

	// Persist queued status
	engine.session.Status = "queued"
	if err := m.store.InsertSession(context.Background(), engine.session.ToStorageRow()); err != nil {
		applog.Errorf("crawler", "enqueue: could not persist queued session %s: %v", sessionID, err)
	}
	applog.Infof("crawler", "Session %s queued (all slots busy)", sessionID)
}

// dequeue removes a session from the queue. Returns true if it was found and removed.
func (m *Manager) dequeue(sessionID string) bool {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()
	if !m.queuedSet[sessionID] {
		return false
	}
	for i, qc := range m.queue {
		if qc.sessionID == sessionID {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			delete(m.queuedSet, sessionID)
			return true
		}
	}
	delete(m.queuedSet, sessionID)
	return false
}

// promoteNext pops the next queued crawl and starts it.
// Must be called after releasing a semaphore slot.
func (m *Manager) promoteNext() {
	m.queueMu.Lock()
	if len(m.queue) == 0 {
		m.queueMu.Unlock()
		return
	}
	next := m.queue[0]
	m.queue = m.queue[1:]
	delete(m.queuedSet, next.sessionID)
	m.queueMu.Unlock()

	// Acquire the semaphore slot (should succeed immediately since caller just released one)
	m.sem <- struct{}{}

	m.mu.Lock()
	m.engines[next.sessionID] = next.engine
	m.mu.Unlock()

	applog.Infof("crawler", "Promoting queued session %s", next.sessionID)
	go m.runEngine(next.sessionID, next.engine, next.seeds)
}

// IsQueued returns true if the session is waiting in the queue.
func (m *Manager) IsQueued(sessionID string) bool {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()
	return m.queuedSet[sessionID]
}

// QueuedSessions returns the IDs of sessions waiting in the queue.
func (m *Manager) QueuedSessions() []string {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()
	ids := make([]string, 0, len(m.queue))
	for _, qc := range m.queue {
		ids = append(ids, qc.sessionID)
	}
	return ids
}

// runEngine runs the crawl engine and records the outcome.
func (m *Manager) runEngine(sessionID string, engine *Engine, seeds []string) {
	defer func() {
		// Release semaphore slot and promote next queued crawl
		<-m.sem
		m.promoteNext()
	}()
	defer func() {
		if r := recover(); r != nil {
			applog.Errorf("crawler", "panic in crawl engine %s: %v\n%s", sessionID, r, debug.Stack())
			m.markSessionCrashed(sessionID, engine, fmt.Sprintf("panic: %v", r))
			m.mu.Lock()
			delete(m.engines, sessionID)
			m.lastErrors[sessionID] = fmt.Sprintf("panic: %v", r)
			m.mu.Unlock()
		}
	}()
	err := engine.Run(seeds)
	m.mu.Lock()
	delete(m.engines, sessionID)
	if err != nil {
		if len(m.lastErrors) >= maxLastErrors {
			for k := range m.lastErrors {
				delete(m.lastErrors, k)
				break
			}
		}
		m.lastErrors[sessionID] = err.Error()
	} else {
		delete(m.lastErrors, sessionID)
	}
	m.mu.Unlock()
	if err != nil {
		applog.Errorf("crawler", "Crawl %s failed: %v", sessionID, err)
	}
}
