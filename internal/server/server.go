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
	cfg     *config.Config
	store   *storage.Store
	manager *crawler.Manager
	server  *http.Server
}

// New creates a new Server.
func New(cfg *config.Config, store *storage.Store) *Server {
	return &Server{
		cfg:     cfg,
		store:   store,
		manager: crawler.NewManager(cfg, store),
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
	mux.HandleFunc("GET /api/storage-stats", s.handleStorageStats)
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

	// Wrap with basic auth if credentials are configured
	var handler http.Handler = mux
	if s.cfg.Server.Username != "" && s.cfg.Server.Password != "" {
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

	log.Printf("Web UI available at http://%s", addr)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
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
	sessions, err := s.store.ListSessions(r.Context())
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
			"is_running":   s.manager.IsRunning(sess.ID),
		})
	}
	writeJSON(w, resp)
}

func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
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
	stats, err := s.store.SessionStats(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleInternalLinks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
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
	pages, queue, running := s.manager.Progress(sessionID)
	writeJSON(w, map[string]interface{}{
		"pages_crawled": pages,
		"queue_size":    queue,
		"is_running":    running,
	})
}

func (s *Server) handleStartCrawl(w http.ResponseWriter, r *http.Request) {
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
	sessionID := r.PathValue("id")
	if err := s.manager.StopCrawl(sessionID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleResumeCrawl(w http.ResponseWriter, r *http.Request) {
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
	stats, err := s.store.StorageStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
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
