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
	"strconv"
	"strings"
	"time"

	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/crawler"
	"github.com/SEObserver/seocrawler/internal/storage"
	"github.com/spf13/viper"
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
	mux.HandleFunc("GET /api/storage-stats", s.handleStorageStats)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/theme", s.handleTheme)

	// API routes - write
	mux.HandleFunc("PUT /api/theme", s.handleUpdateTheme)
	mux.HandleFunc("POST /api/crawl", s.handleStartCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.handleStopCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/resume", s.handleResumeCrawl)
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
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
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

	pages, err := s.store.ListPages(r.Context(), sessionID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, pages)
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	links, err := s.store.ExternalLinksPaginated(r.Context(), sessionID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	source := r.URL.Query().Get("source")
	target := r.URL.Query().Get("target")
	links, err := s.store.InternalLinksPaginated(r.Context(), sessionID, limit, offset, source, target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	_, err := s.manager.ResumeCrawl(sessionID)
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
