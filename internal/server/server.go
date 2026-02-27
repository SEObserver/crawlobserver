package server

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"os/exec"

	"github.com/SEObserver/seocrawler/internal/apikeys"
	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/crawler"
	"github.com/SEObserver/seocrawler/internal/storage"
	"github.com/spf13/viper"
	"github.com/temoto/robotstxt"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// Server serves the web GUI and REST API.
type Server struct {
	cfg      *config.Config
	store    *storage.Store
	keyStore *apikeys.Store
	manager  *crawler.Manager
	server   *http.Server
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

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes - read
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}/pages", s.handlePages)
	mux.HandleFunc("GET /api/sessions/{id}/links", s.handleLinks)
	mux.HandleFunc("GET /api/sessions/{id}/internal-links", s.handleInternalLinks)
	mux.HandleFunc("GET /api/sessions/{id}/stats", s.handleStats)
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
	mux.HandleFunc("GET /api/storage-stats", s.handleStorageStats)
	mux.HandleFunc("GET /api/session-storage", s.handleSessionStorage)
	mux.HandleFunc("GET /api/global-stats", s.handleGlobalStats)
	mux.HandleFunc("GET /api/system-stats", s.handleSystemStats)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/theme", s.handleTheme)

	// API routes - write
	mux.HandleFunc("PUT /api/theme", s.handleUpdateTheme)
	mux.HandleFunc("POST /api/crawl", s.handleStartCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.handleStopCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/resume", s.handleResumeCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/recompute-depths", s.handleRecomputeDepths)
	mux.HandleFunc("POST /api/sessions/{id}/compute-pagerank", s.handleComputePageRank)
	mux.HandleFunc("POST /api/sessions/{id}/retry-failed", s.handleRetryFailed)
	mux.HandleFunc("POST /api/sessions/{id}/robots-test", s.handleRobotsTest)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)

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

	// Static frontend files with SPA fallback
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return fmt.Errorf("frontend filesystem: %w", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve static file first
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Check if static file exists (assets, etc.)
		if f, err := distFS.(fs.ReadFileFS).ReadFile(path[1:]); err == nil {
			// Detect content type from extension
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
		// SPA fallback: serve index.html for all other routes
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// Wrap with auth middleware (API key + basic auth)
	var handler http.Handler = mux
	if s.keyStore != nil {
		handler = apikeys.Authenticate(s.keyStore, s.cfg.Server.Username, s.cfg.Server.Password)(mux)
		log.Println("Authentication enabled (API keys + basic auth)")
	} else if s.cfg.Server.Username != "" && s.cfg.Server.Password != "" {
		handler = basicAuth(mux, s.cfg.Server.Username, s.cfg.Server.Password)
		log.Println("Basic authentication enabled")
	} else {
		log.Println("WARNING: No authentication configured. Set server.username and server.password in config.")
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	url := fmt.Sprintf("http://%s", addr)
	log.Printf("Web UI available at %s", url)

	// Auto-open browser
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

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
		log.Printf("Could not open browser: %v", err)
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stats)
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	links, err := s.store.GetPageLinks(r.Context(), sessionID, url, inLimit, inOffset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get exact per-session storage from system.parts
	sessionStorage, err := s.store.SessionStorageStats(r.Context())
	if err != nil {
		// Non-fatal: fall back to proportional estimation
		log.Printf("warning: session storage stats unavailable: %v", err)
		sessionStorage = map[string]uint64{}
	}

	// Load sessions
	sessions, err := s.store.ListSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
					log.Printf("auto-assign: failed to create project %q: %v", hostname, err)
					continue
				}
				pid = p.ID
				nameToID[hostname] = pid
				projectMap[pid] = hostname
			}
			// Associate session to project
			if err := s.store.UpdateSessionProject(r.Context(), sess.ID, &pid); err != nil {
				log.Printf("auto-assign: failed to associate session %s: %v", sess.ID, err)
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleRecomputeDepths(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}

	if err := s.store.RecomputeDepths(r.Context(), sessionID, sess.SeedURLs); err != nil {
		writeError(w, http.StatusInternalServerError, "recompute failed: "+err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

func (s *Server) handleSitemaps(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	sitemaps, err := s.store.GetSitemaps(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if urls == nil {
		urls = []storage.SitemapURLRow{}
	}
	writeJSON(w, urls)
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
	sess, err := s.store.GetSession(sessionID)
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

// --- Project handlers ---

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.keyStore.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
