package server

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/backup"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/crawler"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/SEObserver/crawlobserver/internal/updater"
	"github.com/spf13/viper"
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
	mux.HandleFunc("GET /api/sessions/{id}/external-checks/expired-domains", s.handleExpiredDomains)
	mux.HandleFunc("GET /api/sessions/{id}/resource-checks", s.handlePageResourceChecks)
	mux.HandleFunc("GET /api/sessions/{id}/resource-checks/summary", s.handlePageResourceChecksSummary)
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

	// Provider (SEObserver, etc.) routes
	mux.HandleFunc("GET /api/projects/{id}/providers", s.handleListProviderConnections)
	mux.HandleFunc("POST /api/projects/{id}/providers/{provider}/connect", s.handleProviderConnect)
	mux.HandleFunc("DELETE /api/projects/{id}/providers/{provider}/disconnect", s.handleProviderDisconnect)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/status", s.handleProviderStatus)
	mux.HandleFunc("POST /api/projects/{id}/providers/{provider}/fetch", s.handleProviderFetch)
	mux.HandleFunc("POST /api/projects/{id}/providers/{provider}/stop", s.handleProviderStopFetch)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/metrics", s.handleProviderMetrics)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/backlinks", s.handleProviderBacklinks)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/refdomains", s.handleProviderRefDomains)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/rankings", s.handleProviderRankings)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/visibility", s.handleProviderVisibility)

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

	s.writeAPIDiscoveryFile()

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

const apiDiscoveryFile = ".crawlobserver-api.json"

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

// --- Simple handlers ---

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

// --- Auth helpers ---

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

func basicAuth(next http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="CrawlObserver"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Middleware ---

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

// --- Utilities ---

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
