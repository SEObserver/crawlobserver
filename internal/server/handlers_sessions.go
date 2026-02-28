package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/crawler"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/temoto/robotstxt"
)

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	// Paginated mode if ?limit= is present
	if r.URL.Query().Get("limit") != "" {
		limit := queryInt(r, "limit", 30)
		offset := queryInt(r, "offset", 0)
		projectID := r.URL.Query().Get("project_id")
		search := r.URL.Query().Get("search")

		// If project API key, force project filter
		auth := apikeys.FromContext(r.Context())
		if auth != nil && auth.ProjectID != nil {
			projectID = *auth.ProjectID
		}

		sessions, total, err := s.store.ListSessionsPaginated(r.Context(), limit, offset, projectID, search)
		if err != nil {
			internalError(w, r, err)
			return
		}

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
		if resp == nil {
			resp = []map[string]interface{}{}
		}
		writeJSON(w, map[string]interface{}{
			"sessions": resp,
			"total":    total,
		})
		return
	}

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

	var lastPages int64
	var lastQueue int
	var lastRunning bool
	var lastLostPages, lastLostLinks int64
	first := true

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}

		pages, queue, running := s.manager.Progress(sessionID)
		bufState := s.manager.BufferState(sessionID)

		// Only send if data changed or first message
		if !first && pages == lastPages && queue == lastQueue && running == lastRunning &&
			bufState.LostPages == lastLostPages && bufState.LostLinks == lastLostLinks {
			continue
		}
		lastPages, lastQueue, lastRunning = pages, queue, running
		lastLostPages, lastLostLinks = bufState.LostPages, bufState.LostLinks
		first = false

		data := fmt.Sprintf(`{"pages_crawled":%d,"queue_size":%d,"is_running":%t`, pages, queue, running)
		if bufState.LostPages > 0 {
			data += fmt.Sprintf(`,"lost_pages":%d`, bufState.LostPages)
		}
		if bufState.LostLinks > 0 {
			data += fmt.Sprintf(`,"lost_links":%d`, bufState.LostLinks)
		}
		data += "}"
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if !running {
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
			return
		}
	}
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	pages, queue, running := s.manager.Progress(sessionID)
	resp := map[string]interface{}{
		"pages_crawled": pages,
		"queue_size":    queue,
		"is_running":    running,
	}
	bufState := s.manager.BufferState(sessionID)
	if bufState.LostPages > 0 {
		resp["lost_pages"] = bufState.LostPages
	}
	if bufState.LostLinks > 0 {
		resp["lost_links"] = bufState.LostLinks
	}
	writeJSON(w, resp)
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

	var overrides *crawler.CrawlRequest
	statusCode := queryInt(r, "status_code", 0)
	if statusCode > 0 {
		overrides = &crawler.CrawlRequest{RetryStatusCode: statusCode}
	}

	count, err := s.manager.RetryFailed(sessionID, overrides)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Retrying %d pages (status %d) for session %s", count, statusCode, sessionID),
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

func (s *Server) handleExpiredDomains(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)

	result, err := s.store.GetExpiredDomains(r.Context(), sessionID, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handlePageResourceChecks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	filters := parseFilters(r, storage.PageResourceCheckFilters)

	checks, err := s.store.GetPageResourceChecks(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if checks == nil {
		checks = []storage.PageResourceCheck{}
	}
	writeJSON(w, checks)
}

func (s *Server) handlePageResourceChecksSummary(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	summary, err := s.store.GetPageResourceTypeSummary(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if summary == nil {
		summary = []storage.ResourceTypeSummary{}
	}
	writeJSON(w, summary)
}
