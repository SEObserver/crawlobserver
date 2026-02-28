package server

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"sync"

	"github.com/SEObserver/seocrawler/internal/apikeys"
	"github.com/SEObserver/seocrawler/internal/applog"
	"github.com/SEObserver/seocrawler/internal/backup"
	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/crawler"
	"github.com/SEObserver/seocrawler/internal/customtests"
	"github.com/SEObserver/seocrawler/internal/gsc"
	// "github.com/SEObserver/seocrawler/internal/providers"
	// "github.com/SEObserver/seocrawler/internal/seobserver"
	"github.com/SEObserver/seocrawler/internal/storage"
	"github.com/SEObserver/seocrawler/internal/updater"
	"github.com/spf13/viper"
	"github.com/temoto/robotstxt"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// gscFetchStatus tracks the progress of a background GSC fetch.
type gscFetchStatus struct {
	Fetching  bool           `json:"fetching"`
	RowsSoFar int            `json:"rows_so_far"`
	Error     string         `json:"error,omitempty"`
	cancel    context.CancelFunc
}

// providerFetchStatus tracks the progress of a background provider data fetch.
type providerFetchStatus struct {
	Fetching  bool   `json:"fetching"`
	Phase     string `json:"phase"`
	RowsSoFar int    `json:"rows_so_far"`
	Error     string `json:"error,omitempty"`
	cancel    context.CancelFunc
}

// Server serves the web GUI and REST API.
type Server struct {
	cfg             *config.Config
	store           StorageService
	keyStore        *apikeys.Store
	manager         CrawlService
	server          *http.Server
	NoBrowserOpen   bool // skip auto-opening browser (e.g. in GUI/Wails mode)
	IsDesktop       bool // true when running as .app desktop bundle
	UpdateStatus    *updater.UpdateStatus
	BackupOpts      *backup.BackupOptions
	StopClickHouse  func()           // stops managed CH (nil if external)
	StartClickHouse func() error     // restarts managed CH (nil if external)

	gscFetchMu      sync.Mutex
	gscFetchStatus  map[string]*gscFetchStatus // projectID -> status

	providerFetchMu     sync.Mutex
	providerFetchStatus map[string]*providerFetchStatus // "projectID:provider" -> status
}

// New creates a new Server.
func New(cfg *config.Config, store *storage.Store, keyStore *apikeys.Store) *Server {
	return &Server{
		cfg:      cfg,
		store:    store,
		keyStore: keyStore,
		manager:  crawler.NewManager(cfg, store),
	}
}

// NewWithDeps creates a new Server with explicit dependencies (for testing).
func NewWithDeps(cfg *config.Config, store StorageService, keyStore *apikeys.Store, manager CrawlService) *Server {
	return &Server{
		cfg:      cfg,
		store:    store,
		keyStore: keyStore,
		manager:  manager,
	}
}

// buildHandler builds the HTTP handler with all routes, auth, and security headers.
func (s *Server) buildHandler() (http.Handler, error) {
	mux := http.NewServeMux()

	// API routes - read
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}/pages", s.handlePages)
	mux.HandleFunc("GET /api/sessions/{id}/links", s.handleLinks)
	mux.HandleFunc("GET /api/sessions/{id}/internal-links", s.handleInternalLinks)
	mux.HandleFunc("GET /api/sessions/{id}/stats", s.handleStats)
	mux.HandleFunc("GET /api/sessions/{id}/audit", s.handleAudit)
	mux.HandleFunc("GET /api/sessions/{id}/progress", s.handleProgress)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.handleSSE)
	mux.HandleFunc("GET /api/sessions/{id}/page-html", s.handlePageHTML)
	mux.HandleFunc("GET /api/sessions/{id}/page-detail", s.handlePageDetail)
	mux.HandleFunc("GET /api/sessions/{id}/pagerank-distribution", s.handlePageRankDistribution)
	mux.HandleFunc("GET /api/sessions/{id}/pagerank-treemap", s.handlePageRankTreemap)
	mux.HandleFunc("GET /api/sessions/{id}/pagerank-top", s.handlePageRankTop)
	mux.HandleFunc("GET /api/sessions/{id}/robots", s.handleRobotsHosts)
	mux.HandleFunc("GET /api/sessions/{id}/robots-content", s.handleRobotsContent)
	mux.HandleFunc("GET /api/sessions/{id}/sitemaps", s.handleSitemaps)
	mux.HandleFunc("GET /api/sessions/{id}/sitemap-urls", s.handleSitemapURLs)
	mux.HandleFunc("GET /api/sessions/{id}/external-checks", s.handleExternalLinkChecks)
	mux.HandleFunc("GET /api/sessions/{id}/external-checks/domains", s.handleExternalLinkCheckDomains)
	mux.HandleFunc("GET /api/storage-stats", s.handleStorageStats)
	mux.HandleFunc("GET /api/session-storage", s.handleSessionStorage)
	mux.HandleFunc("GET /api/global-stats", s.handleGlobalStats)
	mux.HandleFunc("GET /api/system-stats", s.handleSystemStats)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/server-info", s.handleServerInfo)
	mux.HandleFunc("GET /api/theme", s.handleTheme)
	mux.HandleFunc("GET /api/compare/stats", s.handleCompareStats)
	mux.HandleFunc("GET /api/compare/pages", s.handleComparePages)
	mux.HandleFunc("GET /api/compare/links", s.handleCompareLinks)

	// API routes - write
	mux.HandleFunc("PUT /api/theme", s.handleUpdateTheme)
	mux.HandleFunc("POST /api/crawl", s.handleStartCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.handleStopCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/resume", s.handleResumeCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/recompute-depths", s.handleRecomputeDepths)
	mux.HandleFunc("POST /api/sessions/{id}/compute-pagerank", s.handleComputePageRank)
	mux.HandleFunc("POST /api/sessions/{id}/retry-failed", s.handleRetryFailed)
	mux.HandleFunc("POST /api/sessions/{id}/robots-test", s.handleRobotsTest)
	mux.HandleFunc("POST /api/sessions/{id}/robots-simulate", s.handleRobotsSimulate)
	mux.HandleFunc("GET /api/sessions/{id}/export", s.handleExportSession)
	mux.HandleFunc("POST /api/sessions/import", s.handleImportSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)

	// Update & backup routes (desktop mode)
	mux.HandleFunc("GET /api/update/status", s.handleUpdateStatus)
	mux.HandleFunc("POST /api/update/apply", s.handleUpdateApply)
	mux.HandleFunc("GET /api/backups", s.handleListBackups)
	mux.HandleFunc("POST /api/backups", s.handleCreateBackup)
	mux.HandleFunc("POST /api/backups/restore", s.handleRestoreBackup)
	mux.HandleFunc("DELETE /api/backups/{name}", s.handleDeleteBackup)

	// Projects & API keys routes
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("PUT /api/projects/{id}", s.handleRenameProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	mux.HandleFunc("POST /api/projects/{pid}/sessions/{sid}", s.handleAssociateSession)
	mux.HandleFunc("DELETE /api/projects/{pid}/sessions/{sid}", s.handleDisassociateSession)
	mux.HandleFunc("GET /api/api-keys", s.handleListAPIKeys)
	mux.HandleFunc("POST /api/api-keys", s.handleCreateAPIKey)
	mux.HandleFunc("DELETE /api/api-keys/{id}", s.handleDeleteAPIKey)

	// GSC (Google Search Console) routes
	mux.HandleFunc("GET /api/gsc/authorize", s.handleGSCAuthorize)
	mux.HandleFunc("GET /api/gsc/callback", s.handleGSCCallback)
	mux.HandleFunc("GET /api/projects/{id}/gsc/status", s.handleGSCStatus)
	mux.HandleFunc("POST /api/projects/{id}/gsc/fetch", s.handleGSCFetch)
	mux.HandleFunc("POST /api/projects/{id}/gsc/stop", s.handleGSCStopFetch)
	mux.HandleFunc("DELETE /api/projects/{id}/gsc/disconnect", s.handleGSCDisconnect)
	mux.HandleFunc("GET /api/projects/{id}/gsc/overview", s.handleGSCOverview)
	mux.HandleFunc("GET /api/projects/{id}/gsc/queries", s.handleGSCQueries)
	mux.HandleFunc("GET /api/projects/{id}/gsc/pages", s.handleGSCPages)
	mux.HandleFunc("GET /api/projects/{id}/gsc/countries", s.handleGSCCountries)
	mux.HandleFunc("GET /api/projects/{id}/gsc/devices", s.handleGSCDevices)
	mux.HandleFunc("GET /api/projects/{id}/gsc/timeline", s.handleGSCTimeline)
	mux.HandleFunc("GET /api/projects/{id}/gsc/inspection", s.handleGSCInspection)

	// Provider (SEObserver, etc.) routes — TODO: handlers not yet implemented
	// mux.HandleFunc("GET /api/projects/{id}/providers", s.handleListProviderConnections)
	// mux.HandleFunc("POST /api/projects/{id}/providers/{provider}/connect", s.handleProviderConnect)
	// mux.HandleFunc("DELETE /api/projects/{id}/providers/{provider}/disconnect", s.handleProviderDisconnect)
	// mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/status", s.handleProviderStatus)
	// mux.HandleFunc("POST /api/projects/{id}/providers/{provider}/fetch", s.handleProviderFetch)
	// mux.HandleFunc("POST /api/projects/{id}/providers/{provider}/stop", s.handleProviderStopFetch)
	// mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/metrics", s.handleProviderMetrics)
	// mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/backlinks", s.handleProviderBacklinks)
	// mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/refdomains", s.handleProviderRefDomains)
	// mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/rankings", s.handleProviderRankings)
	// mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/visibility", s.handleProviderVisibility)

	// Application Logs routes
	mux.HandleFunc("GET /api/logs", s.handleListLogs)
	mux.HandleFunc("GET /api/logs/export", s.handleExportLogs)

	// Custom Tests / Rulesets routes
	mux.HandleFunc("GET /api/rulesets", s.handleListRulesets)
	mux.HandleFunc("POST /api/rulesets", s.handleCreateRuleset)
	mux.HandleFunc("GET /api/rulesets/{id}", s.handleGetRuleset)
	mux.HandleFunc("PUT /api/rulesets/{id}", s.handleUpdateRuleset)
	mux.HandleFunc("DELETE /api/rulesets/{id}", s.handleDeleteRuleset)
	mux.HandleFunc("POST /api/sessions/{id}/run-tests", s.handleRunTests)

	// Static frontend files with SPA fallback
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return nil, fmt.Errorf("frontend filesystem: %w", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		if f, err := distFS.(fs.ReadFileFS).ReadFile(path[1:]); err == nil {
			switch {
			case strings.HasSuffix(path, ".js"):
				w.Header().Set("Content-Type", "application/javascript")
			case strings.HasSuffix(path, ".css"):
				w.Header().Set("Content-Type", "text/css")
			case strings.HasSuffix(path, ".svg"):
				w.Header().Set("Content-Type", "image/svg+xml")
			case strings.HasSuffix(path, ".png"):
				w.Header().Set("Content-Type", "image/png")
			case strings.HasSuffix(path, ".ico"):
				w.Header().Set("Content-Type", "image/x-icon")
			}
			w.Write(f)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// Wrap with auth middleware
	var handler http.Handler = mux
	if s.keyStore != nil {
		handler = apikeys.Authenticate(s.keyStore, s.cfg.Server.Username, s.cfg.Server.Password)(mux)
		applog.Info("server", "Authentication enabled (API keys + basic auth)")
	} else if s.cfg.Server.Username != "" && s.cfg.Server.Password != "" {
		handler = basicAuth(mux, s.cfg.Server.Username, s.cfg.Server.Password)
		applog.Info("server", "Basic authentication enabled")
	} else {
		applog.Warn("server", "No authentication configured. Set server.username and server.password in config.")
	}

	handler = securityHeaders(handler)
	return handler, nil
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	applog.Init(s.store)

	handler, err := s.buildHandler()
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	url := fmt.Sprintf("http://%s", addr)
	applog.Infof("server", "Web UI available at %s", url)

	// s.writeAPIDiscoveryFile() // TODO: not yet implemented

	if !s.NoBrowserOpen {
		go func() {
			time.Sleep(500 * time.Millisecond)
			openBrowser(url)
		}()
	}

	return s.server.ListenAndServe()
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		applog.Warnf("server", "Could not open browser: %v", err)
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.removeAPIDiscoveryFile()
	applog.Close()
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

const apiDiscoveryFile = ".seocrawler-api.json"

func (s *Server) writeAPIDiscoveryFile() {
	data, err := json.MarshalIndent(s.serverInfoPayload(), "", "  ")
	if err != nil {
		applog.Warnf("server", "Could not marshal API discovery file: %v", err)
		return
	}
	if err := os.WriteFile(apiDiscoveryFile, data, 0600); err != nil {
		applog.Warnf("server", "Could not write %s: %v", apiDiscoveryFile, err)
		return
	}
	applog.Infof("server", "API discovery file written to %s", apiDiscoveryFile)
}

func (s *Server) removeAPIDiscoveryFile() {
	if err := os.Remove(apiDiscoveryFile); err != nil && !os.IsNotExist(err) {
		applog.Warnf("server", "Could not remove %s: %v", apiDiscoveryFile, err)
	}
}

func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	writeJSON(w, map[string]interface{}{
		"mem_alloc":      m.Alloc,
		"mem_sys":        m.Sys,
		"mem_heap_inuse": m.HeapInuse,
		"num_goroutines": runtime.NumGoroutine(),
		"num_gc":         m.NumGC,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.serverInfoPayload())
}

func (s *Server) serverInfoPayload() map[string]interface{} {
	addr := fmt.Sprintf("http://%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	info := map[string]interface{}{
		"api_url":  addr + "/api",
		"host":     s.cfg.Server.Host,
		"port":     s.cfg.Server.Port,
		"has_auth": s.cfg.Server.Username != "" && s.cfg.Server.Password != "",
	}
	if s.cfg.Server.Username != "" && s.cfg.Server.Password != "" {
		info["username"] = s.cfg.Server.Username
	}
	return info
}

func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.cfg.Theme)
}

func (s *Server) handleUpdateTheme(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var t config.ThemeConfig
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	viper.Set("theme.app_name", t.AppName)
	viper.Set("theme.logo_url", t.LogoURL)
	viper.Set("theme.accent_color", t.AccentColor)
	viper.Set("theme.mode", t.Mode)

	if err := viper.WriteConfig(); err != nil {
		// Config file doesn't exist yet — create it
		if err := viper.SafeWriteConfig(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
			return
		}
	}

	s.cfg.Theme = t
	writeJSON(w, s.cfg.Theme)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	var sessions []storage.CrawlSession
	var err error

	// If project API key, filter by project
	auth := apikeys.FromContext(r.Context())
	if auth != nil && auth.ProjectID != nil {
		sessions, err = s.store.ListSessions(r.Context(), *auth.ProjectID)
	} else {
		sessions, err = s.store.ListSessions(r.Context())
	}
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Enrich with running status
	var resp []map[string]interface{}
	for _, sess := range sessions {
		resp = append(resp, map[string]interface{}{
			"ID":           sess.ID,
			"StartedAt":    sess.StartedAt,
			"FinishedAt":   sess.FinishedAt,
			"Status":       sess.Status,
			"SeedURLs":     sess.SeedURLs,
			"Config":       sess.Config,
			"PagesCrawled": sess.PagesCrawled,
			"UserAgent":    sess.UserAgent,
			"ProjectID":    sess.ProjectID,
			"is_running":   s.manager.IsRunning(sess.ID),
		})
	}
	writeJSON(w, resp)
}

func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	filters := parseFilters(r, storage.PageFilters)

	pages, err := s.store.ListPages(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, pages)
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	filters := parseFilters(r, storage.LinkFilters)

	links, err := s.store.ExternalLinksPaginated(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, links)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	stats, err := s.store.SessionStats(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	audit, err := s.store.SessionAudit(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, audit)
}

func (s *Server) handleInternalLinks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	filters := parseFilters(r, storage.LinkFilters)

	links, err := s.store.InternalLinksPaginated(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, links)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		pages, queue, running := s.manager.Progress(sessionID)
		data := fmt.Sprintf(`{"pages_crawled":%d,"queue_size":%d,"is_running":%t}`, pages, queue, running)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if !running {
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
			return
		}

		time.Sleep(1 * time.Second)
	}
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	pages, queue, running := s.manager.Progress(sessionID)
	writeJSON(w, map[string]interface{}{
		"pages_crawled": pages,
		"queue_size":    queue,
		"is_running":    running,
	})
}

func (s *Server) handleStartCrawl(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req crawler.CrawlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sessionID, err := s.manager.StartCrawl(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{
		"session_id": sessionID,
		"status":     "started",
	})
}

func (s *Server) handleStopCrawl(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")
	if err := s.manager.StopCrawl(sessionID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleResumeCrawl(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	// Decode optional overrides from body
	var overrides *crawler.CrawlRequest
	if r.Body != nil && r.ContentLength != 0 {
		var req crawler.CrawlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		overrides = &req
	}

	_, err := s.manager.ResumeCrawl(sessionID, overrides)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "resumed"})
}

func (s *Server) handlePageHTML(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	url := r.URL.Query().Get("url")
	if url == "" {
		writeError(w, http.StatusBadRequest, "missing url parameter")
		return
	}
	html, err := s.store.GetPageHTML(r.Context(), sessionID, url)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"url": url, "body_html": html})
}

func (s *Server) handlePageDetail(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	url := r.URL.Query().Get("url")
	if url == "" {
		writeError(w, http.StatusBadRequest, "missing url parameter")
		return
	}
	inLimit := queryInt(r, "in_limit", 100)
	inOffset := queryInt(r, "in_offset", 0)

	page, err := s.store.GetPage(r.Context(), sessionID, url)
	if err != nil {
		internalError(w, r, err)
		return
	}
	links, err := s.store.GetPageLinks(r.Context(), sessionID, url, inLimit, inOffset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"page":  page,
		"links": links,
	})
}

func (s *Server) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	stats, err := s.store.StorageStats(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleSessionStorage(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	stats, err := s.store.SessionStorageStats(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleGlobalStats(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}

	sessionStats, storageResult, err := s.store.GlobalStats(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Get exact per-session storage from system.parts
	sessionStorage, err := s.store.SessionStorageStats(r.Context())
	if err != nil {
		// Non-fatal: fall back to proportional estimation
		applog.Warnf("server", "session storage stats unavailable: %v", err)
		sessionStorage = map[string]uint64{}
	}

	// Load sessions
	sessions, err := s.store.ListSessions(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Load projects
	var projectMap map[string]string // id -> name
	if s.keyStore != nil {
		projects, _ := s.keyStore.ListProjects()
		projectMap = make(map[string]string, len(projects))
		for _, p := range projects {
			projectMap[p.ID] = p.Name
		}
	}

	// Auto-assign orphan sessions: create a project per hostname and associate
	if s.keyStore != nil {
		// Build reverse map: project name -> id
		nameToID := make(map[string]string, len(projectMap))
		for id, name := range projectMap {
			nameToID[name] = id
		}

		for i, sess := range sessions {
			if sess.ProjectID != nil {
				continue
			}
			// Extract hostname from first seed URL
			hostname := "unknown"
			if len(sess.SeedURLs) > 0 {
				if u, err := url.Parse(sess.SeedURLs[0]); err == nil && u.Hostname() != "" {
					hostname = u.Hostname()
				}
			}
			// Find or create project for this hostname
			pid, exists := nameToID[hostname]
			if !exists {
				p, err := s.keyStore.CreateProject(hostname)
				if err != nil {
					applog.Warnf("server", "auto-assign: failed to create project %q: %v", hostname, err)
					continue
				}
				pid = p.ID
				nameToID[hostname] = pid
				projectMap[pid] = hostname
			}
			// Associate session to project
			if err := s.store.UpdateSessionProject(r.Context(), sess.ID, &pid); err != nil {
				applog.Warnf("server", "auto-assign: failed to associate session %s: %v", sess.ID, err)
				continue
			}
			sessions[i].ProjectID = &pid
		}
	}

	// Build session-to-project mapping
	type sessionInfo struct {
		ProjectID *string
		SeedURLs  []string
	}
	sessionMap := map[string]sessionInfo{}
	for _, sess := range sessions {
		sessionMap[sess.ID] = sessionInfo{ProjectID: sess.ProjectID, SeedURLs: sess.SeedURLs}
	}

	// Aggregate by project
	type projectStats struct {
		ProjectID    *string `json:"project_id"`
		ProjectName  string  `json:"project_name"`
		Sessions     int     `json:"sessions"`
		TotalPages   uint64  `json:"total_pages"`
		TotalLinks   uint64  `json:"total_links"`
		ErrorCount   uint64  `json:"error_count"`
		AvgFetchMs   float64 `json:"avg_fetch_ms"`
		StorageBytes uint64  `json:"storage_bytes"`
	}

	type fetchAcc struct {
		sum   float64
		count uint64
	}

	projectAgg := map[string]*projectStats{}
	projectFetch := map[string]*fetchAcc{}
	var globalPages, globalLinks, globalErrors uint64
	var globalFetchSum float64
	var globalFetchCount uint64

	for _, gs := range sessionStats {
		info := sessionMap[gs.SessionID]
		key := ""
		if info.ProjectID != nil {
			key = *info.ProjectID
		}
		ps, ok := projectAgg[key]
		if !ok {
			ps = &projectStats{ProjectID: info.ProjectID}
			if info.ProjectID != nil && projectMap != nil {
				ps.ProjectName = projectMap[*info.ProjectID]
			} else {
				ps.ProjectName = "(No project)"
			}
			projectAgg[key] = ps
			projectFetch[key] = &fetchAcc{}
		}
		ps.Sessions++
		ps.TotalPages += gs.TotalPages
		ps.TotalLinks += gs.TotalLinks
		ps.ErrorCount += gs.ErrorCount

		fa := projectFetch[key]
		fa.sum += gs.AvgFetchMs * float64(gs.TotalPages)
		fa.count += gs.TotalPages

		globalFetchSum += gs.AvgFetchMs * float64(gs.TotalPages)
		globalFetchCount += gs.TotalPages

		// Exact storage from system.parts partitions
		ps.StorageBytes += sessionStorage[gs.SessionID]

		globalPages += gs.TotalPages
		globalLinks += gs.TotalLinks
		globalErrors += gs.ErrorCount
	}

	// Compute weighted avg fetch per project
	for key, ps := range projectAgg {
		if fa := projectFetch[key]; fa.count > 0 {
			ps.AvgFetchMs = fa.sum / float64(fa.count)
		}
	}

	projectList := make([]projectStats, 0, len(projectAgg))
	for _, ps := range projectAgg {
		projectList = append(projectList, *ps)
	}

	var globalAvgFetch float64
	if globalFetchCount > 0 {
		globalAvgFetch = globalFetchSum / float64(globalFetchCount)
	}

	var totalStorage uint64
	for _, t := range storageResult.Tables {
		totalStorage += t.BytesOnDisk
	}

	writeJSON(w, map[string]interface{}{
		"total_pages":    globalPages,
		"total_links":    globalLinks,
		"total_errors":   globalErrors,
		"avg_fetch_ms":   globalAvgFetch,
		"total_storage":  totalStorage,
		"total_sessions": len(sessions),
		"projects":       projectList,
		"storage_tables": storageResult.Tables,
	})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	// Don't allow deleting running sessions
	if s.manager.IsRunning(sessionID) {
		writeError(w, http.StatusConflict, "cannot delete a running session, stop it first")
		return
	}

	if err := s.store.DeleteSession(r.Context(), sessionID); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleExportSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	includeHTML := r.URL.Query().Get("include_html") == "true"

	sess, err := s.store.GetSession(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}

	filename := fmt.Sprintf("crawl-%s.jsonl.gz", sess.ID[:8])
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if err := s.store.ExportSession(r.Context(), sessionID, w, includeHTML); err != nil {
		applog.Errorf("server", "export session %s: %v", sessionID, err)
	}
}

func (s *Server) handleImportSession(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}

	// Limit upload to 2 GB
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30)

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field: "+err.Error())
		return
	}
	defer file.Close()

	sess, err := s.store.ImportSession(r.Context(), file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "import failed: "+err.Error())
		return
	}

	writeJSON(w, sess)
}

func (s *Server) handleRecomputeDepths(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	sess, err := s.store.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	if err := s.store.RecomputeDepths(r.Context(), sessionID, sess.SeedURLs); err != nil {
		internalError(w, r, err)
		return
	}

	writeJSON(w, map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("Depths recomputed for session %s", sessionID),
	})
}

func (s *Server) handleComputePageRank(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	if err := s.store.ComputePageRank(r.Context(), sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "pagerank computation failed: "+err.Error())
		return
	}

	writeJSON(w, map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("PageRank computed for session %s", sessionID),
	})
}

func (s *Server) handleRetryFailed(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	count, err := s.manager.RetryFailed(sessionID, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Retrying %d failed pages for session %s", count, sessionID),
		"count":   count,
	})
}

func (s *Server) handlePageRankDistribution(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	buckets := queryInt(r, "buckets", 20)
	result, err := s.store.PageRankDistribution(r.Context(), sessionID, buckets)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handlePageRankTreemap(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	depth := queryInt(r, "depth", 2)
	minPages := queryInt(r, "min_pages", 1)
	result, err := s.store.PageRankTreemap(r.Context(), sessionID, depth, minPages)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handlePageRankTop(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	directory := r.URL.Query().Get("directory")
	result, err := s.store.PageRankTop(r.Context(), sessionID, limit, offset, directory)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleRobotsHosts(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	hosts, err := s.store.GetRobotsHosts(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if hosts == nil {
		hosts = []storage.RobotsRow{}
	}
	writeJSON(w, hosts)
}

func (s *Server) handleRobotsContent(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		writeError(w, http.StatusBadRequest, "missing host parameter")
		return
	}
	row, err := s.store.GetRobotsContent(r.Context(), sessionID, host)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, row)
}

func (s *Server) handleRobotsTest(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	var req struct {
		Host      string   `json:"host"`
		UserAgent string   `json:"user_agent"`
		URLs      []string `json:"urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Host == "" || len(req.URLs) == 0 {
		writeError(w, http.StatusBadRequest, "host and urls are required")
		return
	}
	if req.UserAgent == "" {
		req.UserAgent = "*"
	}

	// Load robots.txt content from DB
	row, err := s.store.GetRobotsContent(r.Context(), sessionID, req.Host)
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Parse robots.txt
	robots, err := robotstxt.FromBytes([]byte(row.Content))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse robots.txt: "+err.Error())
		return
	}

	group := robots.FindGroup(req.UserAgent)

	type testResult struct {
		URL     string `json:"url"`
		Allowed bool   `json:"allowed"`
	}
	results := make([]testResult, 0, len(req.URLs))
	for _, u := range req.URLs {
		// Extract path from URL
		path := u
		if parsed, err := url.Parse(u); err == nil {
			path = parsed.Path
			if path == "" {
				path = "/"
			}
		}
		results = append(results, testResult{
			URL:     u,
			Allowed: group.Test(path),
		})
	}

	writeJSON(w, map[string]interface{}{"results": results})
}

func (s *Server) handleRobotsSimulate(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	var req struct {
		Host       string `json:"host"`
		UserAgent  string `json:"user_agent"`
		NewContent string `json:"new_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Host == "" || req.NewContent == "" {
		writeError(w, http.StatusBadRequest, "host and new_content are required")
		return
	}
	if req.UserAgent == "" {
		req.UserAgent = "*"
	}

	// Load current robots.txt
	row, err := s.store.GetRobotsContent(r.Context(), sessionID, req.Host)
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Load all URLs for this host.
	// The host field from robots_txt may already include the scheme (e.g. "https://example.com").
	// We need to match URLs in the pages table that start with this host.
	host := req.Host
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	urls, err := s.store.GetURLsByHost(r.Context(), sessionID, host)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// Also try the other scheme
	var altHost string
	if strings.HasPrefix(host, "https://") {
		altHost = "http://" + strings.TrimPrefix(host, "https://")
	} else {
		altHost = "https://" + strings.TrimPrefix(host, "http://")
	}
	altURLs, err := s.store.GetURLsByHost(r.Context(), sessionID, altHost)
	if err != nil {
		internalError(w, r, err)
		return
	}
	urls = append(urls, altURLs...)

	// Parse current and new robots.txt
	currentRobots, err := robotstxt.FromBytes([]byte(row.Content))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse current robots.txt: "+err.Error())
		return
	}
	newRobots, err := robotstxt.FromBytes([]byte(req.NewContent))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse new robots.txt: "+err.Error())
		return
	}

	currentGroup := currentRobots.FindGroup(req.UserAgent)
	newGroup := newRobots.FindGroup(req.UserAgent)

	type urlEntry struct {
		URL string `json:"url"`
	}

	var (
		currentlyAllowed int
		currentlyBlocked int
		newlyBlocked     []urlEntry
		newlyAllowed     []urlEntry
	)

	for _, u := range urls {
		path := u
		if parsed, parseErr := url.Parse(u); parseErr == nil {
			path = parsed.Path
			if path == "" {
				path = "/"
			}
		}

		currentAllowed := currentGroup.Test(path)
		newAllowed := newGroup.Test(path)

		if currentAllowed {
			currentlyAllowed++
		} else {
			currentlyBlocked++
		}

		if currentAllowed && !newAllowed {
			newlyBlocked = append(newlyBlocked, urlEntry{URL: u})
		} else if !currentAllowed && newAllowed {
			newlyAllowed = append(newlyAllowed, urlEntry{URL: u})
		}
	}

	writeJSON(w, map[string]interface{}{
		"total_urls":        len(urls),
		"currently_allowed": currentlyAllowed,
		"currently_blocked": currentlyBlocked,
		"newly_blocked":     newlyBlocked,
		"newly_allowed":     newlyAllowed,
		"summary": map[string]int{
			"will_block": len(newlyBlocked),
			"will_allow": len(newlyAllowed),
		},
	})
}

func (s *Server) handleSitemaps(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	sitemaps, err := s.store.GetSitemaps(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if sitemaps == nil {
		sitemaps = []storage.SitemapRow{}
	}
	writeJSON(w, sitemaps)
}

func (s *Server) handleSitemapURLs(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	sitemapURL := r.URL.Query().Get("url")
	if sitemapURL == "" {
		writeError(w, http.StatusBadRequest, "missing url parameter")
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)

	urls, err := s.store.GetSitemapURLs(r.Context(), sessionID, sitemapURL, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if urls == nil {
		urls = []storage.SitemapURLRow{}
	}
	writeJSON(w, urls)
}

func (s *Server) handleExternalLinkChecks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	filters := parseFilters(r, storage.ExternalCheckFilters)

	checks, err := s.store.GetExternalLinkChecks(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if checks == nil {
		checks = []storage.ExternalLinkCheck{}
	}
	writeJSON(w, checks)
}

func (s *Server) handleExternalLinkCheckDomains(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	filters := parseFilters(r, storage.ExternalDomainCheckFilters)

	domains, err := s.store.GetExternalLinkCheckDomains(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if domains == nil {
		domains = []storage.ExternalDomainCheck{}
	}
	writeJSON(w, domains)
}

// requireFullAccess returns 403 if the caller is a project-scoped key.
func requireFullAccess(w http.ResponseWriter, r *http.Request) bool {
	auth := apikeys.FromContext(r.Context())
	if auth != nil && auth.IsReadOnly() {
		writeError(w, http.StatusForbidden, "project API keys do not have access to this endpoint")
		return false
	}
	return true
}

// requireSessionAccess checks that a project key can access the given session.
func (s *Server) requireSessionAccess(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	auth := apikeys.FromContext(r.Context())
	if auth == nil || auth.ProjectID == nil {
		return true
	}
	sess, err := s.store.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return false
	}
	if sess.ProjectID == nil || *sess.ProjectID != *auth.ProjectID {
		writeError(w, http.StatusForbidden, "session not accessible with this API key")
		return false
	}
	return true
}

// --- Compare handlers ---

func (s *Server) handleCompareStats(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		writeError(w, http.StatusBadRequest, "both 'a' and 'b' session IDs are required")
		return
	}
	if !s.requireSessionAccess(w, r, a) || !s.requireSessionAccess(w, r, b) {
		return
	}
	result, err := s.store.CompareStats(r.Context(), a, b)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleComparePages(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		writeError(w, http.StatusBadRequest, "both 'a' and 'b' session IDs are required")
		return
	}
	if !s.requireSessionAccess(w, r, a) || !s.requireSessionAccess(w, r, b) {
		return
	}
	diffType := r.URL.Query().Get("type")
	switch diffType {
	case "added", "removed", "changed":
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: added, removed, changed")
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	result, err := s.store.ComparePages(r.Context(), a, b, diffType, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleCompareLinks(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		writeError(w, http.StatusBadRequest, "both 'a' and 'b' session IDs are required")
		return
	}
	if !s.requireSessionAccess(w, r, a) || !s.requireSessionAccess(w, r, b) {
		return
	}
	diffType := r.URL.Query().Get("type")
	switch diffType {
	case "added", "removed":
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: added, removed")
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	result, err := s.store.CompareLinks(r.Context(), a, b, diffType, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

// --- Project handlers ---

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.keyStore.ListProjects()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.keyStore.CreateProject(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}

func (s *Server) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.keyStore.RenameProject(id, req.Name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "renamed"})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.keyStore.DeleteProject(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleAssociateSession(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	pid := r.PathValue("pid")
	sid := r.PathValue("sid")

	// Verify project exists
	if _, err := s.keyStore.GetProject(pid); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err := s.store.UpdateSessionProject(r.Context(), sid, &pid); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "associated"})
}

func (s *Server) handleDisassociateSession(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sid := r.PathValue("sid")
	if err := s.store.UpdateSessionProject(r.Context(), sid, nil); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "disassociated"})
}

// --- API Key handlers ---

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	keys, err := s.keyStore.ListAPIKeys()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, keys)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Name      string  `json:"name"`
		Type      string  `json:"type"`
		ProjectID *string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	result, err := s.keyStore.CreateAPIKey(req.Name, req.Type, req.ProjectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, result)
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.keyStore.DeleteAPIKey(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// --- Update & Backup handlers ---

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if s.UpdateStatus == nil {
		writeJSON(w, map[string]interface{}{"available": false, "current_version": updater.Version})
		return
	}
	writeJSON(w, s.UpdateStatus.Snapshot())
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.UpdateStatus == nil {
		writeError(w, http.StatusBadRequest, "update check not available")
		return
	}
	release := s.UpdateStatus.Release()
	if release == nil {
		writeError(w, http.StatusBadRequest, "no release info available, check for updates first")
		return
	}

	if s.IsDesktop {
		newAppPath, err := updater.DownloadDesktopUpdate(release)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "download failed: "+err.Error())
			return
		}
		if err := updater.SelfUpdateDesktop(newAppPath); err != nil {
			writeError(w, http.StatusInternalServerError, "install failed: "+err.Error())
			return
		}
	} else {
		tmpPath, err := updater.DownloadUpdate(release)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "download failed: "+err.Error())
			return
		}
		if err := updater.SelfUpdate(tmpPath); err != nil {
			writeError(w, http.StatusInternalServerError, "install failed: "+err.Error())
			return
		}
	}

	writeJSON(w, map[string]string{"status": "installed", "message": "Restart the application to use the new version."})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if s.BackupOpts == nil {
		writeJSON(w, []backup.BackupInfo{})
		return
	}
	backups, err := backup.ListBackups(s.BackupOpts.BackupDir)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if backups == nil {
		backups = []backup.BackupInfo{}
	}
	writeJSON(w, backups)
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.BackupOpts == nil {
		writeError(w, http.StatusBadRequest, "backup not configured")
		return
	}

	// Stop ClickHouse for consistency
	if s.StopClickHouse != nil {
		applog.Info("server", "Stopping ClickHouse for backup...")
		s.StopClickHouse()
	}

	info, err := backup.Create(*s.BackupOpts, updater.Version)

	// Restart ClickHouse regardless of backup result
	if s.StartClickHouse != nil {
		applog.Info("server", "Restarting ClickHouse after backup...")
		if startErr := s.StartClickHouse(); startErr != nil {
			applog.Warnf("server", "failed to restart ClickHouse: %v", startErr)
		}
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup failed: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, info)
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.BackupOpts == nil {
		writeError(w, http.StatusBadRequest, "backup not configured")
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		writeError(w, http.StatusBadRequest, "filename is required")
		return
	}

	// Sanitize filename: extract base name to prevent path traversal
	cleanName := filepath.Base(req.Filename)
	if cleanName == "." || cleanName == "/" || cleanName == "\\" {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	archivePath := filepath.Join(s.BackupOpts.BackupDir, cleanName)
	if _, err := os.Stat(archivePath); err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}

	// Stop ClickHouse for restore
	if s.StopClickHouse != nil {
		applog.Info("server", "Stopping ClickHouse for restore...")
		s.StopClickHouse()
	}

	err := backup.Restore(archivePath, *s.BackupOpts)

	// Restart ClickHouse regardless of restore result
	if s.StartClickHouse != nil {
		applog.Info("server", "Restarting ClickHouse after restore...")
		if startErr := s.StartClickHouse(); startErr != nil {
			applog.Warnf("server", "failed to restart ClickHouse: %v", startErr)
		}
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "restore failed: "+err.Error())
		return
	}

	writeJSON(w, map[string]string{"status": "restored", "message": "Restart the application to apply restored data."})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.BackupOpts == nil {
		writeError(w, http.StatusBadRequest, "backup not configured")
		return
	}
	name := filepath.Base(r.PathValue("name"))
	if name == "." || name == "/" || name == "\\" {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	archivePath := filepath.Join(s.BackupOpts.BackupDir, name)
	if err := backup.DeleteBackup(archivePath); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func basicAuth(next http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="SEOCrawler"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; frame-src 'self' blob:; base-uri 'self'; form-action 'self'; object-src 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// internalError logs the real error server-side and returns a generic message to the client.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	applog.Errorf("server", "%s %s: %v", r.Method, r.URL.Path, err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

// --- Custom Tests Handlers ---

func (s *Server) handleListRulesets(w http.ResponseWriter, r *http.Request) {
	rulesets, err := s.keyStore.ListRulesets()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, rulesets)
}

func (s *Server) handleCreateRuleset(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Name  string                 `json:"name"`
		Rules []customtests.TestRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	ruleset, err := s.keyStore.CreateRuleset(req.Name, req.Rules)
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, ruleset)
}

func (s *Server) handleGetRuleset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ruleset, err := s.keyStore.GetRuleset(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "ruleset not found")
		return
	}
	writeJSON(w, ruleset)
}

func (s *Server) handleUpdateRuleset(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Name  string                 `json:"name"`
		Rules []customtests.TestRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.keyStore.UpdateRuleset(id, req.Name, req.Rules); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	ruleset, err := s.keyStore.GetRuleset(id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, ruleset)
}

func (s *Server) handleDeleteRuleset(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.keyStore.DeleteRuleset(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleRunTests(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	var req struct {
		RulesetID string `json:"ruleset_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RulesetID == "" {
		writeError(w, http.StatusBadRequest, "ruleset_id is required")
		return
	}

	ruleset, err := s.keyStore.GetRuleset(req.RulesetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ruleset not found")
		return
	}

	adapter := &customTestsStorageAdapter{store: s.store}
	result, err := customtests.RunTests(r.Context(), adapter, sessionID, ruleset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

// customTestsStorageAdapter adapts StorageService to customtests.StorageInterface.
type customTestsStorageAdapter struct {
	store StorageService
}

func (a *customTestsStorageAdapter) RunCustomTestsSQL(ctx context.Context, sessionID string, rules []customtests.TestRule) (map[string]map[string]string, error) {
	return a.store.RunCustomTestsSQL(ctx, sessionID, rules)
}

func (a *customTestsStorageAdapter) StreamPagesHTML(ctx context.Context, sessionID string) (<-chan customtests.PageHTMLRow, error) {
	ch, err := a.store.StreamPagesHTML(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make(chan customtests.PageHTMLRow, 64)
	go func() {
		defer close(out)
		for row := range ch {
			out <- customtests.PageHTMLRow{URL: row.URL, HTML: row.HTML}
		}
	}()
	return out, nil
}

// --- GSC Handlers ---

func (s *Server) handleGSCAuthorize(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	if s.cfg.GSC.ClientID == "" || s.cfg.GSC.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "GSC not configured: set gsc.client_id and gsc.client_secret in config.yaml")
		return
	}
	url := gsc.AuthorizeURL(&s.cfg.GSC, projectID)
	writeJSON(w, map[string]string{"url": url})
}

func (s *Server) handleGSCCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state") // project_id
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	token, err := gsc.ExchangeCode(r.Context(), &s.cfg.GSC, code)
	if err != nil {
		applog.Errorf("gsc", "OAuth exchange error: %v", err)
		writeError(w, http.StatusBadRequest, "failed to exchange code")
		return
	}

	conn := &apikeys.GSCConnection{
		ProjectID:    state,
		PropertyURL:  "", // will be set when user selects property
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenExpiry:  token.Expiry,
	}
	if err := s.keyStore.SaveGSCConnection(conn); err != nil {
		applog.Errorf("gsc", "save connection error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save connection")
		return
	}

	// Redirect to frontend with connected status
	redirectURL := fmt.Sprintf("/?gsc_connected=%s", url.QueryEscape(state))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *Server) handleGSCStatus(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")

	conn, err := s.keyStore.GetGSCConnection(projectID)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"connected":    false,
			"property_url": "",
			"properties":   []gsc.Property{},
		})
		return
	}

	result := map[string]interface{}{
		"connected":    true,
		"property_url": conn.PropertyURL,
	}

	// Include fetch status if available
	s.gscFetchMu.Lock()
	if fs, ok := s.gscFetchStatus[projectID]; ok {
		result["fetch_status"] = fs
	}
	s.gscFetchMu.Unlock()

	// If connected but no property selected, list available properties
	if conn.PropertyURL == "" {
		client, newToken, err := gsc.NewClientFromTokens(r.Context(), &s.cfg.GSC, conn.AccessToken, conn.RefreshToken, conn.TokenExpiry)
		if err != nil {
			applog.Errorf("gsc", "client error: %v", err)
			writeJSON(w, result)
			return
		}
		// Update token if refreshed
		if newToken.AccessToken != conn.AccessToken {
			conn.AccessToken = newToken.AccessToken
			conn.TokenExpiry = newToken.Expiry
			s.keyStore.SaveGSCConnection(conn)
		}

		props, err := client.ListProperties(r.Context())
		if err != nil {
			applog.Errorf("gsc", "list properties error: %v", err)
		}
		if props == nil {
			props = []gsc.Property{}
		}
		result["properties"] = props
	}

	writeJSON(w, result)
}

func (s *Server) handleGSCFetch(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")

	// Parse optional property_url from body (for initial property selection)
	var body struct {
		PropertyURL string `json:"property_url"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	conn, err := s.keyStore.GetGSCConnection(projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no GSC connection for this project")
		return
	}

	// Update property URL if provided
	if body.PropertyURL != "" {
		conn.PropertyURL = body.PropertyURL
		if err := s.keyStore.SaveGSCConnection(conn); err != nil {
			internalError(w, r, err)
			return
		}
	}

	if conn.PropertyURL == "" {
		writeError(w, http.StatusBadRequest, "no property selected")
		return
	}

	client, newToken, err := gsc.NewClientFromTokens(r.Context(), &s.cfg.GSC, conn.AccessToken, conn.RefreshToken, conn.TokenExpiry)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "GSC authentication failed, please reconnect")
		return
	}
	// Update token if refreshed
	if newToken.AccessToken != conn.AccessToken {
		conn.AccessToken = newToken.AccessToken
		conn.TokenExpiry = newToken.Expiry
		s.keyStore.SaveGSCConnection(conn)
	}

	// Default date range: last 16 months (GSC maximum)
	endDate := body.EndDate
	startDate := body.StartDate
	if endDate == "" {
		endDate = time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	}
	if startDate == "" {
		startDate = time.Now().AddDate(-1, -4, 0).Format("2006-01-02")
	}

	// Cancel any existing fetch for this project
	s.gscFetchMu.Lock()
	if s.gscFetchStatus == nil {
		s.gscFetchStatus = make(map[string]*gscFetchStatus)
	}
	if existing := s.gscFetchStatus[projectID]; existing != nil && existing.cancel != nil {
		existing.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.gscFetchStatus[projectID] = &gscFetchStatus{Fetching: true, cancel: cancel}
	s.gscFetchMu.Unlock()

	// Fetch in background with incremental batch insertion
	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				applog.Errorf("gsc", "fetch PANIC: %v", r)
				s.gscFetchMu.Lock()
				s.gscFetchStatus[projectID] = &gscFetchStatus{Fetching: false, Error: fmt.Sprintf("panic: %v", r)}
				s.gscFetchMu.Unlock()
			}
		}()
		applog.Infof("gsc", "fetch started for project %s, property %s, range %s to %s", projectID, conn.PropertyURL, startDate, endDate)

		total, err := client.FetchSearchAnalytics(ctx, conn.PropertyURL, startDate, endDate,
			func(rows []gsc.AnalyticsRow, totalSoFar int) error {
				insertRows := make([]storage.GSCAnalyticsInsertRow, len(rows))
				for i, r := range rows {
					d, _ := time.Parse("2006-01-02", r.Date)
					insertRows[i] = storage.GSCAnalyticsInsertRow{
						Date:        d,
						Query:       r.Query,
						Page:        r.Page,
						Country:     r.Country,
						Device:      r.Device,
						Clicks:      uint32(r.Clicks),
						Impressions: uint32(r.Impressions),
						CTR:         float32(r.CTR),
						Position:    float32(r.Position),
					}
				}
				if err := s.store.InsertGSCAnalytics(ctx, projectID, insertRows); err != nil {
					return fmt.Errorf("inserting batch: %w", err)
				}
				s.gscFetchMu.Lock()
				s.gscFetchStatus[projectID] = &gscFetchStatus{Fetching: true, RowsSoFar: totalSoFar}
				s.gscFetchMu.Unlock()
				applog.Infof("gsc", "inserted %d rows (total: %d)", len(rows), totalSoFar)
				return nil
			})
		s.gscFetchMu.Lock()
		if err != nil {
			applog.Errorf("gsc", "fetch error: %v", err)
			s.gscFetchStatus[projectID] = &gscFetchStatus{Fetching: false, RowsSoFar: total, Error: err.Error()}
		} else {
			applog.Infof("gsc", "fetch completed for project %s: %d total rows", projectID, total)
			delete(s.gscFetchStatus, projectID)
		}
		s.gscFetchMu.Unlock()
	}()

	writeJSON(w, map[string]string{"status": "fetching"})
}

func (s *Server) handleGSCStopFetch(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	s.gscFetchMu.Lock()
	if fs, ok := s.gscFetchStatus[projectID]; ok && fs.cancel != nil {
		fs.cancel()
	}
	delete(s.gscFetchStatus, projectID)
	s.gscFetchMu.Unlock()
	applog.Infof("gsc", "fetch stopped for project %s", projectID)
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleGSCDisconnect(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	s.keyStore.DeleteGSCConnection(projectID)
	s.store.DeleteGSCData(r.Context(), projectID)
	writeJSON(w, map[string]string{"status": "disconnected"})
}

func (s *Server) handleGSCOverview(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	stats, err := s.store.GSCOverview(r.Context(), projectID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleGSCQueries(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	rows, total, err := s.store.GSCTopQueries(r.Context(), projectID, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

func (s *Server) handleGSCPages(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	rows, total, err := s.store.GSCTopPages(r.Context(), projectID, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

func (s *Server) handleGSCCountries(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	rows, err := s.store.GSCByCountry(r.Context(), projectID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleGSCDevices(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	rows, err := s.store.GSCByDevice(r.Context(), projectID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleGSCTimeline(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	rows, err := s.store.GSCTimeline(r.Context(), projectID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleGSCInspection(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	rows, total, err := s.store.GSCInspectionResults(r.Context(), projectID, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

// parseFilters extracts filter parameters from the request query string.
func parseFilters(r *http.Request, whitelist map[string]storage.FilterDef) []storage.ParsedFilter {
	var filters []storage.ParsedFilter
	for key, values := range r.URL.Query() {
		if key == "limit" || key == "offset" {
			continue
		}
		def, ok := whitelist[key]
		if !ok || len(values) == 0 || values[0] == "" {
			continue
		}
		filters = append(filters, storage.ParsedFilter{
			Def:   def,
			Value: values[0],
		})
	}
	return filters
}

func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	level := r.URL.Query().Get("level")
	component := r.URL.Query().Get("component")
	search := r.URL.Query().Get("search")

	logs, total, err := s.store.ListLogs(r.Context(), limit, offset, level, component, search)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{
		"logs":  logs,
		"total": total,
	})
}

func (s *Server) handleExportLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.store.ExportLogs(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=application_logs.jsonl")
	enc := json.NewEncoder(w)
	for _, l := range logs {
		enc.Encode(l)
	}
}
