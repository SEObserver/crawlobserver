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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/backup"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/crawler"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/SEObserver/crawlobserver/internal/telemetry"
	"github.com/SEObserver/crawlobserver/internal/updater"
	"github.com/posthog/posthog-go"
	"github.com/spf13/viper"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// gscFetchStatus tracks the progress of a background GSC fetch.
type gscFetchStatus struct {
	Fetching  bool   `json:"fetching"`
	RowsSoFar int    `json:"rows_so_far"`
	Error     string `json:"error,omitempty"`
	cancel    context.CancelFunc
}

// phaseResult tracks the outcome of a single fetch phase.
type phaseResult struct {
	Rows   int    `json:"rows"`
	Error  string `json:"error,omitempty"`
	Cached bool   `json:"cached,omitempty"`
}

// providerFetchStatus tracks the progress of a background provider data fetch.
type providerFetchStatus struct {
	Fetching     bool                   `json:"fetching"`
	Phase        string                 `json:"phase"`
	RowsSoFar    int                    `json:"rows_so_far"`
	Error        string                 `json:"error,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	PhaseResults map[string]phaseResult `json:"phase_results,omitempty"`
	cancel       context.CancelFunc
}

// SetupProgress tracks the download/startup progress exposed to the frontend.
type SetupProgress struct {
	Percent         int   `json:"percent"`
	BytesDownloaded int64 `json:"bytes_downloaded"`
	TotalBytes      int64 `json:"total_bytes"`
}

// Server serves the web GUI and REST API.
type Server struct {
	cfg             *config.Config
	store           StorageService
	keyStore        *apikeys.Store
	manager         CrawlService
	server          *http.Server
	IsDesktop       bool // true when running as .app desktop bundle
	UpdateStatus    *updater.UpdateStatus
	BackupOpts      *backup.BackupOptions
	StopClickHouse  func()       // stops managed CH (nil if external)
	StartClickHouse func() error // restarts managed CH (nil if external)

	rateLimiter *rateLimitMiddleware

	gscFetchMu     sync.Mutex
	gscFetchStatus map[string]*gscFetchStatus // projectID -> status

	providerFetchMu     sync.Mutex
	providerFetchStatus map[string]*providerFetchStatus // "projectID:provider" -> status

	// Setup mode fields (desktop onboarding)
	SetupMode        bool
	downloadProgress atomic.Value // *SetupProgress
	readyCh          chan struct{}
}

// New creates a new Server.
func New(cfg *config.Config, store *storage.Store, keyStore *apikeys.Store) *Server {
	return &Server{
		cfg:      cfg,
		store:    store,
		keyStore: keyStore,
		manager:  crawler.NewManager(cfg, store, keyStore),
	}
}

// NewSetupServer creates a Server in setup mode (no store/keyStore yet).
// The server can serve the frontend and setup endpoints while ClickHouse downloads.
func NewSetupServer(cfg *config.Config) *Server {
	s := &Server{
		cfg:       cfg,
		SetupMode: true,
		readyCh:   make(chan struct{}),
	}
	s.downloadProgress.Store(&SetupProgress{})
	return s
}

// TransitionToReady wires the store and keyStore, creates the crawl manager,
// and marks the server as fully operational.
func (s *Server) TransitionToReady(store *storage.Store, keyStore *apikeys.Store) {
	s.store = store
	s.keyStore = keyStore
	s.manager = crawler.NewManager(s.cfg, store, keyStore)
	s.SetupMode = false
	close(s.readyCh)
}

// SetDownloadProgress updates the download progress visible to the frontend.
func (s *Server) SetDownloadProgress(p SetupProgress) {
	s.downloadProgress.Store(&p)
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

// Handler builds and returns the HTTP handler (useful for testing).
func (s *Server) Handler() (http.Handler, error) {
	return s.buildHandler()
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
	mux.HandleFunc("GET /api/sessions/{id}/status-timeline", s.handleStatusTimeline)
	mux.HandleFunc("GET /api/sessions/{id}/status-timeline-recent", s.handleStatusTimelineRecent)
	mux.HandleFunc("GET /api/sessions/{id}/pagerank-distribution", s.handlePageRankDistribution)
	mux.HandleFunc("GET /api/sessions/{id}/pagerank-treemap", s.handlePageRankTreemap)
	mux.HandleFunc("GET /api/sessions/{id}/pagerank-top", s.handlePageRankTop)
	mux.HandleFunc("GET /api/sessions/{id}/pagerank-weighted-top", s.handleWeightedPageRankTop)
	mux.HandleFunc("GET /api/sessions/{id}/robots", s.handleRobotsHosts)
	mux.HandleFunc("GET /api/sessions/{id}/robots-content", s.handleRobotsContent)
	mux.HandleFunc("GET /api/sessions/{id}/sitemaps", s.handleSitemaps)
	mux.HandleFunc("GET /api/sessions/{id}/sitemap-urls", s.handleSitemapURLs)
	mux.HandleFunc("GET /api/sessions/{id}/sitemap-coverage-urls", s.handleSitemapCoverageURLs)
	mux.HandleFunc("GET /api/sessions/{id}/external-checks", s.handleExternalLinkChecks)
	mux.HandleFunc("GET /api/sessions/{id}/external-checks/domains", s.handleExternalLinkCheckDomains)
	mux.HandleFunc("GET /api/sessions/{id}/external-checks/expired-domains", s.handleExpiredDomains)
	mux.HandleFunc("GET /api/sessions/{id}/resource-checks", s.handlePageResourceChecks)
	mux.HandleFunc("GET /api/sessions/{id}/resource-checks/summary", s.handlePageResourceChecksSummary)
	mux.HandleFunc("POST /api/sessions/{id}/reparse-resources", s.handleReparseResources)
	mux.HandleFunc("GET /api/sessions/{id}/near-duplicates", s.handleNearDuplicates)
	mux.HandleFunc("GET /api/sessions/{id}/redirect-pages", s.handleRedirectPages)
	mux.HandleFunc("GET /api/sessions/{id}/url-patterns", s.handleURLPatterns)
	mux.HandleFunc("GET /api/sessions/{id}/url-params", s.handleURLParams)
	mux.HandleFunc("GET /api/sessions/{id}/url-directories", s.handleURLDirectories)
	mux.HandleFunc("GET /api/sessions/{id}/url-hosts", s.handleURLHosts)
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
	mux.HandleFunc("POST /api/check-ip", s.handleCheckIP)
	mux.HandleFunc("POST /api/crawl", s.handleStartCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.handleStopCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/resume", s.handleResumeCrawl)
	mux.HandleFunc("POST /api/sessions/{id}/recompute-depths", s.handleRecomputeDepths)
	mux.HandleFunc("POST /api/sessions/{id}/compute-pagerank", s.handleComputePageRank)
	mux.HandleFunc("POST /api/sessions/{id}/compute-near-duplicates", s.handleComputeNearDuplicates)

	// Interlinking
	mux.HandleFunc("POST /api/sessions/{id}/compute-interlinking", s.handleComputeInterlinking)
	mux.HandleFunc("GET /api/sessions/{id}/interlinking-opportunities", s.handleInterlinkingOpportunities)
	mux.HandleFunc("POST /api/sessions/{id}/simulate-interlinking", s.handleSimulateInterlinking)
	mux.HandleFunc("GET /api/sessions/{id}/interlinking-simulations", s.handleListSimulations)
	mux.HandleFunc("GET /api/sessions/{id}/interlinking-simulations/{simId}", s.handleGetSimulationResults)
	mux.HandleFunc("POST /api/sessions/{id}/import-virtual-links", s.handleImportVirtualLinks)
	mux.HandleFunc("POST /api/sessions/{id}/recompute-content-hashes", s.handleRecomputeContentHashes)
	mux.HandleFunc("POST /api/sessions/{id}/retry-failed", s.handleRetryFailed)
	mux.HandleFunc("POST /api/sessions/{id}/robots-test", s.handleRobotsTest)
	mux.HandleFunc("POST /api/sessions/{id}/robots-simulate", s.handleRobotsSimulate)
	mux.HandleFunc("GET /api/sessions/{id}/export", s.handleExportSession)
	mux.HandleFunc("POST /api/sessions/import", s.handleImportSession)
	mux.HandleFunc("POST /api/sessions/import/csv", s.handleImportCSVSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("DELETE /api/sessions-unassigned", s.handleDeleteUnassignedSessions)

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
	mux.HandleFunc("DELETE /api/projects/{id}/with-sessions", s.handleDeleteProjectWithSessions)
	mux.HandleFunc("POST /api/projects/{pid}/sessions/{sid}", s.handleAssociateSession)
	mux.HandleFunc("DELETE /api/projects/{pid}/sessions/{sid}", s.handleDisassociateSession)
	mux.HandleFunc("PUT /api/sessions/{sid}/label", s.handleRenameSession)
	mux.HandleFunc("POST /api/projects/{pid}/sessions/batch", s.handleBatchAssignSessions)
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
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/top-pages", s.handleProviderTopPages)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/api-calls", s.handleProviderAPICalls)
	mux.HandleFunc("GET /api/projects/{id}/providers/{provider}/data/{dataType}", s.handleProviderData)

	// Backlinks top (provider backlinks accessed by project_id)
	mux.HandleFunc("GET /api/backlinks/top", s.handleBacklinksTop)

	// Authority (crawl pages enriched with provider data)
	mux.HandleFunc("GET /api/sessions/{id}/authority", s.handleSessionAuthority)

	// Setup & Telemetry routes (accessible in setup mode)
	mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /api/setup/complete", s.handleSetupComplete)
	mux.HandleFunc("GET /api/telemetry", s.handleGetTelemetry)
	mux.HandleFunc("PUT /api/telemetry", s.handleUpdateTelemetry)
	mux.HandleFunc("PUT /api/telemetry/session-recording", s.handleUpdateSessionRecording)

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

	// Extraction routes
	mux.HandleFunc("GET /api/extractor-sets", s.handleListExtractorSets)
	mux.HandleFunc("POST /api/extractor-sets", s.handleCreateExtractorSet)
	mux.HandleFunc("GET /api/extractor-sets/{id}", s.handleGetExtractorSet)
	mux.HandleFunc("PUT /api/extractor-sets/{id}", s.handleUpdateExtractorSet)
	mux.HandleFunc("DELETE /api/extractor-sets/{id}", s.handleDeleteExtractorSet)
	mux.HandleFunc("GET /api/sessions/{id}/extractions", s.handleGetExtractions)
	mux.HandleFunc("POST /api/sessions/{id}/run-extractions", s.handleRunExtractions)

	// Static frontend files with SPA fallback
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return nil, fmt.Errorf("frontend filesystem: %w", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
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
			if _, err := w.Write(f); err != nil {
				applog.Errorf("server", "write static %s: %v", path, err)
			}
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// Wrap with auth middleware
	var handler http.Handler = mux
	switch {
	case s.keyStore != nil && s.cfg.Server.Username != "" && s.cfg.Server.Password != "":
		handler = apikeys.Authenticate(s.keyStore, s.cfg.Server.Username, s.cfg.Server.Password)(mux)
		applog.Infof("server", "Authentication enabled (API keys + basic auth) — user: %s, password in config.yaml", s.cfg.Server.Username)
	case s.cfg.Server.Username != "" && s.cfg.Server.Password != "":
		handler = basicAuth(mux, s.cfg.Server.Username, s.cfg.Server.Password)
		applog.Infof("server", "Basic authentication enabled — user: %s, password in config.yaml", s.cfg.Server.Username)
	default:
		if s.cfg.Server.Host == "0.0.0.0" || (s.cfg.Server.Host != "127.0.0.1" && s.cfg.Server.Host != "localhost") {
			applog.Errorf("server", "WARNING: No authentication configured and server is listening on %s. The API is publicly accessible! Set server.username and server.password in config.", s.cfg.Server.Host)
		} else {
			applog.Warn("server", "No authentication configured. Set server.username and server.password in config.")
		}
	}

	if s.cfg.Server.RateLimit.Enabled {
		s.rateLimiter = newRateLimitMiddleware(s.cfg.Server.RateLimit)
		handler = s.rateLimiter.Handler(handler)
	}

	handler = s.requireReady(handler)
	handler = s.securityHeaders(handler)
	return handler, nil
}

const banner = `
                       +############-
  :====.        .:######################*-..
  .-==:      .=*############+==#############+=
           .*#######*:.. .--------  ..*########+
 .:==-           +...------------------..-*######=.
 .====.  .-===-.  :----------:    .:------:.=######=
 ..--:  :=======-. ---------:      :--------- -#####+
     .  :========: .---------:.   .:----------.:#####*.
    :#- .-=====-.  ----------:.:-:-:..:---------.+####+
    +##=  .:::.  :...--------:.:-------:..------:.*####=
   =####+ .. .-------..:----:.:----------:    ..-:.#####-
  .=####=:-------------:.       ---------:      :-.#####-
  -####* :---------:----.       :::. .::::.   .:--..####-.
  -####* :--------------.       -----:::---:.-----. ####*.
  -####* :--------------.       ------::--:.------. ####=.
  :*###*.:--:::-------. :-:-:::. :-------:.-------.+####-
   =####+.---:-::--. .---::------:.:-----.:------:.#####-
   =####*-:--.     .----:::--------     ..-------.*####=
    +####+::-       ---------------      :------.+####*:
     *####*:.:    :----------------.    .------.=####*:
      %#####..------:-------:----------------.:######
       :######:.---:-----------------------..-#####+
        .+#####*=..---------------------:.-+#######+
          .*########...:------------:...#############+
             *##########-::::::::::=###################=.
               :=*########################+=.=###########+
                   ::#################-:.     .-###########+
 ▞▀▖          ▜ ▞▀▖▌                             =###########+.
 ▌  ▙▀▖▝▀▖▌  ▌▐ ▌ ▌▛▀▖▞▀▘▞▀▖▙▀▖▌ ▌▞▀▖▙▀▖           =###########:
 ▌ ▖▌  ▞▀▌▐▐▐ ▐ ▌ ▌▌ ▌▝▀▖▛▀ ▌  ▐▐ ▛▀ ▌              .-########%:
 ▝▀ ▘  ▝▀▘ ▘▘  ▘▝▀ ▀▀ ▀▀ ▝▀▘▘   ▘ ▝▀▘▘                 =#####=
`

// Start starts the HTTP server.
func (s *Server) Start() error {
	if s.store != nil {
		applog.Init(s.store)
	}

	fmt.Print(banner)

	if s.cfg.Server.PasswordGenerated {
		applog.Info("server", "Generated random password, saved to config.yaml (server.password)")
	}
	if s.cfg.Server.WeakPassword {
		fmt.Fprintf(os.Stderr, "\n  *** WARNING: server is listening on 0.0.0.0 with a weak password! ***\n  *** Set a strong password (>= 8 chars) in server.password before exposing to the internet. ***\n\n")
	}
	if s.cfg.Telemetry.SessionRecording {
		fmt.Fprintf(os.Stderr, "\n  *** WARNING: Session recording is ENABLED. ***\n  *** Full browser sessions (URLs, page content, clicks) are sent to PostHog. ***\n  *** Disable with: telemetry.session_recording: false in config.yaml ***\n\n")
	}

	source := "cli"
	if s.IsDesktop {
		source = "ui"
	}
	telemetry.Track("serve_started", posthog.NewProperties().
		Set("port", s.cfg.Server.Port).
		Set("source", source))

	if s.manager != nil {
		s.manager.RecoverOrphanedSessions(context.Background())
	}

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
	if s.cfg.Server.Host == "127.0.0.1" || s.cfg.Server.Host == "localhost" {
		applog.Infof("server", "Listening on %s (localhost only). Set server.host to 0.0.0.0 in config.yaml to allow external access.", s.cfg.Server.Host)
	} else if s.cfg.Server.Host == "0.0.0.0" {
		applog.Info("server", "Listening on 0.0.0.0 (all interfaces, accessible from the network)")
	}

	s.writeAPIDiscoveryFile()

	return s.server.ListenAndServe()
}


// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.manager != nil {
		s.manager.Shutdown(30 * time.Second)
	}
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}
	s.removeAPIDiscoveryFile()
	applog.Close()
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

const apiDiscoveryFileName = ".crawlobserver-api.json"

func apiDiscoveryFilePath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, apiDiscoveryFileName)
	}
	return apiDiscoveryFileName
}

func (s *Server) writeAPIDiscoveryFile() {
	data, err := json.MarshalIndent(s.serverInfoPayload(), "", "  ")
	if err != nil {
		applog.Warnf("server", "Could not marshal API discovery file: %v", err)
		return
	}
	path := apiDiscoveryFilePath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		applog.Warnf("server", "Could not write %s: %v", path, err)
		return
	}
	applog.Infof("server", "API discovery file written to %s", path)
}

func (s *Server) removeAPIDiscoveryFile() {
	path := apiDiscoveryFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		applog.Warnf("server", "Could not remove %s: %v", path, err)
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
	return info
}

func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		config.ThemeConfig
		Language string `json:"language"`
	}{
		ThemeConfig: s.cfg.Theme,
		Language:    viper.GetString("language"),
	}
	writeJSON(w, resp)
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
			internalError(w, r, err)
			return
		}
	}

	s.cfg.Theme = t
	writeJSON(w, s.cfg.Theme)
}

// --- Setup & Telemetry endpoints ---

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	progress := s.downloadProgress.Load()
	var dp *SetupProgress
	if progress != nil {
		dp = progress.(*SetupProgress)
	} else {
		dp = &SetupProgress{}
	}

	chReady := true // default true for non-setup servers (CLI mode)
	if s.readyCh != nil {
		chReady = false
		select {
		case <-s.readyCh:
			chReady = true
		default:
		}
	}

	writeJSON(w, map[string]interface{}{
		"setup_complete":    s.cfg.SetupComplete,
		"download_progress": dp,
		"clickhouse_ready":  chReady,
		"telemetry_asked":   s.cfg.Telemetry.AskedAt != "",
		"os":                runtime.GOOS,
	})
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Language         string `json:"language"`
		CrawlerDelay     string `json:"crawler_delay"`
		CrawlerWorkers   int    `json:"crawler_workers"`
		TelemetryEnabled bool   `json:"telemetry_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Language != "" {
		viper.Set("language", req.Language)
	}
	if req.CrawlerDelay != "" {
		viper.Set("crawler.delay", req.CrawlerDelay)
	}
	if req.CrawlerWorkers > 0 {
		viper.Set("crawler.workers", req.CrawlerWorkers)
	}
	viper.Set("telemetry.enabled", req.TelemetryEnabled)
	viper.Set("telemetry.asked_at", time.Now().UTC().Format(time.RFC3339))
	viper.Set("setup_complete", true)

	if err := viperWriteConfig(); err != nil {
		internalError(w, r, err)
		return
	}

	s.cfg.SetupComplete = true
	s.cfg.Telemetry.Enabled = req.TelemetryEnabled
	s.cfg.Telemetry.AskedAt = viper.GetString("telemetry.asked_at")
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleGetTelemetry(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"enabled":           s.cfg.Telemetry.Enabled,
		"instance_id":       s.cfg.Telemetry.InstanceID,
		"session_recording": s.cfg.Telemetry.SessionRecording,
	})
}

func (s *Server) handleUpdateTelemetry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.cfg.Telemetry.Enabled = req.Enabled
	viper.Set("telemetry.enabled", req.Enabled)
	if err := viperWriteConfig(); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"enabled":           s.cfg.Telemetry.Enabled,
		"instance_id":       s.cfg.Telemetry.InstanceID,
		"session_recording": s.cfg.Telemetry.SessionRecording,
	})
}

func (s *Server) handleUpdateSessionRecording(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.cfg.Telemetry.SessionRecording = req.Enabled
	viper.Set("telemetry.session_recording", req.Enabled)
	if err := viperWriteConfig(); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"session_recording": s.cfg.Telemetry.SessionRecording,
	})
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

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		scriptSrc := "'self'"
		connectSrc := "'self'"
		workerSrc := "'self'"
		if s.cfg.Telemetry.Enabled {
			scriptSrc = "'self' https://*.posthog.com https://*.i.posthog.com"
			connectSrc = "'self' https://*.posthog.com https://*.i.posthog.com"
			workerSrc = "'self' blob:"
		}
		csp := fmt.Sprintf("default-src 'self'; script-src %s; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; frame-src 'self' blob:; connect-src %s; worker-src %s; base-uri 'self'; form-action 'self'; object-src 'none'", scriptSrc, connectSrc, workerSrc)
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// requireReady returns 503 for endpoints that need ClickHouse to be ready.
func (s *Server) requireReady(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.SetupMode {
			path := r.URL.Path
			// Allow setup, telemetry, health, theme, and static frontend routes
			if strings.HasPrefix(path, "/api/setup/") ||
				strings.HasPrefix(path, "/api/telemetry") ||
				path == "/api/health" ||
				path == "/api/theme" ||
				!strings.HasPrefix(path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusServiceUnavailable, "server is starting up")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// viperWriteConfig writes the current viper config to disk, creating it if needed.
func viperWriteConfig() error {
	if err := viper.WriteConfig(); err != nil {
		return viper.SafeWriteConfig()
	}
	return nil
}

// --- Utilities ---

// clampPagination enforces sane bounds on limit and offset values.
func clampPagination(limit, offset int) (int, int) {
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
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

// parseSort extracts sort/order query params and validates against a whitelist.
func parseSort(r *http.Request, whitelist map[string]string) *storage.SortParam {
	return storage.ParseSort(r.URL.Query().Get("sort"), r.URL.Query().Get("order"), whitelist)
}

// parseFilters extracts filter parameters from the request query string.
func parseFilters(r *http.Request, whitelist map[string]storage.FilterDef) []storage.ParsedFilter {
	var filters []storage.ParsedFilter
	for key, values := range r.URL.Query() {
		if key == "limit" || key == "offset" || key == "sort" || key == "order" {
			continue
		}
		def, ok := whitelist[key]
		if !ok || len(values) == 0 || values[0] == "" || len(values[0]) > 500 {
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
	if err := json.NewEncoder(w).Encode(data); err != nil {
		applog.Errorf("server", "writeJSON: %v", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		applog.Errorf("server", "writeError: %v", err)
	}
}

// internalError logs the real error server-side and returns a user-friendly message.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	applog.Errorf("server", "%s %s: %v", r.Method, r.URL.Path, err)
	writeError(w, http.StatusInternalServerError, classifyInternalError(err))
}

// classifyInternalError returns a user-friendly message based on the error type.
func classifyInternalError(err error) string {
	if err == nil {
		return "internal server error"
	}
	msg := err.Error()

	// ClickHouse connection errors
	switch {
	case strings.Contains(msg, "connection refused"):
		return "ClickHouse is not reachable (connection refused). Is the database running?"
	case strings.Contains(msg, "connect: connection reset"):
		return "ClickHouse connection was reset. The database may be restarting."
	case strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "dial tcp"):
		return "ClickHouse connection timed out. Is the database running?"
	case strings.Contains(msg, "EOF") && strings.Contains(msg, "clickhouse"):
		return "ClickHouse connection closed unexpectedly. The database may have crashed."
	case strings.Contains(msg, "broken pipe"):
		return "ClickHouse connection lost (broken pipe). The database may have restarted."
	case strings.Contains(msg, "database disk image is malformed"):
		return "SQLite database is corrupted. Try restarting the application."
	case strings.Contains(msg, "disk full") || strings.Contains(msg, "no space left"):
		return "Disk is full. Free up space and try again."
	default:
		return "internal server error"
	}
}
