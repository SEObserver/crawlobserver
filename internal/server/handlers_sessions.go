package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"crypto/tls"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/crawler"
	"github.com/SEObserver/crawlobserver/internal/fetcher"
	"github.com/SEObserver/crawlobserver/internal/parser"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/temoto/robotstxt"
)

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	// Paginated mode if ?limit= is present
	if r.URL.Query().Get("limit") != "" {
		limit, offset := clampPagination(queryInt(r, "limit", 30), queryInt(r, "offset", 0))
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
			pagesCrawled := sess.PagesCrawled
			if pages, _, running := s.manager.Progress(sess.ID); running {
				pagesCrawled = uint64(pages)
			}
			resp = append(resp, map[string]interface{}{
				"ID":           sess.ID,
				"StartedAt":    sess.StartedAt,
				"FinishedAt":   sess.FinishedAt,
				"Status":       sess.Status,
				"SeedURLs":     sess.SeedURLs,
				"Config":       sess.Config,
				"PagesCrawled": pagesCrawled,
				"UserAgent":    sess.UserAgent,
				"ProjectID":    sess.ProjectID,
				"is_running":   s.manager.IsRunning(sess.ID),
				"is_queued":    s.manager.IsQueued(sess.ID),
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

	// Enrich with running/queued status
	var resp []map[string]interface{}
	for _, sess := range sessions {
		pagesCrawled := sess.PagesCrawled
		if pages, _, running := s.manager.Progress(sess.ID); running {
			pagesCrawled = uint64(pages)
		}
		resp = append(resp, map[string]interface{}{
			"ID":           sess.ID,
			"StartedAt":    sess.StartedAt,
			"FinishedAt":   sess.FinishedAt,
			"Status":       sess.Status,
			"SeedURLs":     sess.SeedURLs,
			"Config":       sess.Config,
			"PagesCrawled": pagesCrawled,
			"UserAgent":    sess.UserAgent,
			"ProjectID":    sess.ProjectID,
			"is_running":   s.manager.IsRunning(sess.ID),
			"is_queued":    s.manager.IsQueued(sess.ID),
		})
	}
	writeJSON(w, resp)
}

func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.PageFilters)
	sort := parseSort(r, storage.PageSortColumns)

	pages, err := s.store.ListPages(r.Context(), sessionID, limit, offset, filters, sort)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, pages)
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.LinkFilters)
	sort := parseSort(r, storage.LinkSortColumns)

	links, err := s.store.ExternalLinksPaginated(r.Context(), sessionID, limit, offset, filters, sort)
	if err != nil {
		internalError(w, r, err)
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
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.LinkFilters)
	sort := parseSort(r, storage.LinkSortColumns)

	links, err := s.store.InternalLinksPaginated(r.Context(), sessionID, limit, offset, filters, sort)
	if err != nil {
		internalError(w, r, err)
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
	var lastRunning, lastQueued bool
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
		queued := s.manager.IsQueued(sessionID)
		bufState := s.manager.BufferState(sessionID)

		// Only send if data changed or first message
		if !first && pages == lastPages && queue == lastQueue && running == lastRunning &&
			queued == lastQueued &&
			bufState.LostPages == lastLostPages && bufState.LostLinks == lastLostLinks {
			continue
		}
		lastPages, lastQueue, lastRunning, lastQueued = pages, queue, running, queued
		lastLostPages, lastLostLinks = bufState.LostPages, bufState.LostLinks
		first = false

		data := fmt.Sprintf(`{"pages_crawled":%d,"queue_size":%d,"is_running":%t,"is_queued":%t`, pages, queue, running, queued)
		if bufState.LostPages > 0 {
			data += fmt.Sprintf(`,"lost_pages":%d`, bufState.LostPages)
		}
		if bufState.LostLinks > 0 {
			data += fmt.Sprintf(`,"lost_links":%d`, bufState.LostLinks)
		}
		data += "}"
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Don't close if session is queued — wait for it to start
		if queued {
			continue
		}

		if !running {
			errMsg := s.manager.LastError(sessionID)
			if errMsg != "" {
				fmt.Fprintf(w, "event: done\ndata: {\"error\":%q}\n\n", errMsg)
			} else {
				fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			}
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
	if errMsg := s.manager.LastError(sessionID); errMsg != "" {
		resp["error"] = errMsg
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

	status := "started"
	if s.manager.IsQueued(sessionID) {
		status = "queued"
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{
		"session_id": sessionID,
		"status":     status,
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
	outLimit, outOffset := clampPagination(queryInt(r, "out_limit", 100), queryInt(r, "out_offset", 0))
	inLimit, inOffset := clampPagination(queryInt(r, "in_limit", 100), queryInt(r, "in_offset", 0))

	page, err := s.store.GetPage(r.Context(), sessionID, url)
	if err != nil {
		internalError(w, r, err)
		return
	}
	links, err := s.store.GetPageLinks(r.Context(), sessionID, url, outLimit, outOffset, inLimit, inOffset)
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

	// Limit upload to 50 GB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<30)

	// Stream directly from the multipart body — no temp file on disk.
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart request")
		return
	}

	var sess *storage.CrawlSession
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "error reading multipart data")
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		sess, err = s.store.ImportSession(r.Context(), part)
		part.Close()
		if err != nil {
			applog.Errorf("server", "import session: %v", err)
			writeError(w, http.StatusBadRequest, "import failed")
			return
		}
		break
	}

	if sess == nil {
		writeError(w, http.StatusBadRequest, "missing file field in upload")
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

	go func() {
		if err := s.store.RecomputeDepths(context.Background(), sessionID, sess.SeedURLs); err != nil {
			applog.Errorf("server", "RecomputeDepths %s: %v", sessionID, err)
		}
	}()

	writeJSON(w, map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("Depth recomputation started for session %s", sessionID),
	})
}

func (s *Server) handleComputePageRank(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	go func() {
		if err := s.store.ComputePageRank(context.Background(), sessionID); err != nil {
			applog.Errorf("server", "ComputePageRank %s: %v", sessionID, err)
		}
	}()

	writeJSON(w, map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("PageRank computation started for session %s", sessionID),
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
	limit, offset := clampPagination(queryInt(r, "limit", 50), queryInt(r, "offset", 0))
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
		internalError(w, r, err)
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
		writeError(w, http.StatusBadRequest, "failed to parse current robots.txt")
		return
	}
	newRobots, err := robotstxt.FromBytes([]byte(req.NewContent))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse new robots.txt")
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
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))

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

func (s *Server) handleSitemapCoverageURLs(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	filter := r.URL.Query().Get("filter")
	if filter != "sitemap_only" && filter != "in_both" {
		writeError(w, http.StatusBadRequest, "filter must be sitemap_only or in_both")
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))

	urls, err := s.store.GetSitemapCoverageURLs(r.Context(), sessionID, filter, limit, offset)
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
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.ExternalCheckFilters)

	checks, err := s.store.GetExternalLinkChecks(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		internalError(w, r, err)
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
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.ExternalDomainCheckFilters)

	domains, err := s.store.GetExternalLinkCheckDomains(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		internalError(w, r, err)
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
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))

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
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.PageResourceCheckFilters)

	checks, err := s.store.GetPageResourceChecks(r.Context(), sessionID, limit, offset, filters)
	if err != nil {
		internalError(w, r, err)
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

func (s *Server) handleCheckIP(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		SourceIP  string `json:"source_ip"`
		ForceIPv4 bool   `json:"force_ipv4"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	dialOpts := fetcher.DialOptions{
		SourceIP:        req.SourceIP,
		ForceIPv4:       req.ForceIPv4,
		AllowPrivateIPs: true,
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: fetcher.SafeDialContextWithOpts(dialOpts),
		},
	}
	httpReq, err := http.NewRequestWithContext(r.Context(), "GET", "https://ifconfig.me", nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	httpReq.Header.Set("User-Agent", "curl/8.0")
	resp, err := client.Do(httpReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("check failed: %v", err))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	ip := strings.TrimSpace(string(body))
	writeJSON(w, map[string]string{"ip": ip})
}

func (s *Server) handleNearDuplicates(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	threshold := queryInt(r, "threshold", 3)
	result, err := s.store.NearDuplicates(r.Context(), sessionID, threshold, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleRedirectPages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.RedirectFilters)
	sort := parseSort(r, storage.RedirectSortColumns)

	pages, err := s.store.ListRedirectPages(r.Context(), sessionID, limit, offset, filters, sort)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if pages == nil {
		pages = []storage.RedirectPageRow{}
	}
	writeJSON(w, pages)
}

// handleReparseResources re-extracts page resources from stored body_html
// and HTTP-checks each unique resource.
func (s *Server) handleReparseResources(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	const batchSize = 500
	var allRefs []storage.PageResourceRef
	uniqueResources := make(map[string]storage.PageResourceRef) // dedupe by URL+type
	offset := 0
	pagesProcessed := 0

	for {
		pages, err := s.store.GetPageBodies(r.Context(), sessionID, batchSize, offset)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if len(pages) == 0 {
			break
		}

		for _, page := range pages {
			pageURL, err := url.Parse(page.URL)
			if err != nil {
				continue
			}
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(page.BodyHTML))
			if err != nil {
				continue
			}
			resources := parser.ExtractResources(doc, pageURL)
			for _, res := range resources {
				ref := storage.PageResourceRef{
					CrawlSessionID: sessionID,
					PageURL:        page.URL,
					ResourceURL:    res.URL,
					ResourceType:   res.ResourceType,
					IsInternal:     res.IsInternal,
				}
				allRefs = append(allRefs, ref)
				key := res.URL + "|" + res.ResourceType
				if _, exists := uniqueResources[key]; !exists {
					uniqueResources[key] = ref
				}
			}
		}

		pagesProcessed += len(pages)

		// Flush refs in batches
		if len(allRefs) >= 5000 {
			if err := s.store.InsertPageResourceRefs(r.Context(), allRefs); err != nil {
				internalError(w, r, err)
				return
			}
			allRefs = allRefs[:0]
		}

		if len(pages) < batchSize {
			break
		}
		offset += batchSize
	}

	// Flush remaining refs
	if len(allRefs) > 0 {
		if err := s.store.InsertPageResourceRefs(r.Context(), allRefs); err != nil {
			internalError(w, r, err)
			return
		}
	}

	// HTTP HEAD check unique resources (concurrently)
	var checks []storage.PageResourceCheck
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // 10 concurrent checks

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for _, ref := range uniqueResources {
		wg.Add(1)
		go func(ref storage.PageResourceRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			check := storage.PageResourceCheck{
				CrawlSessionID: sessionID,
				URL:            ref.ResourceURL,
				ResourceType:   ref.ResourceType,
				IsInternal:     ref.IsInternal,
				CheckedAt:      time.Now(),
			}

			start := time.Now()
			resp, err := client.Head(ref.ResourceURL)
			check.ResponseTimeMs = uint32(time.Since(start).Milliseconds())

			if err != nil {
				check.Error = err.Error()
			} else {
				check.StatusCode = uint16(resp.StatusCode)
				check.ContentType = resp.Header.Get("Content-Type")
				if loc := resp.Header.Get("Location"); loc != "" {
					check.RedirectURL = loc
				}
				resp.Body.Close()
			}

			mu.Lock()
			checks = append(checks, check)
			mu.Unlock()
		}(ref)
	}
	wg.Wait()

	if err := s.store.InsertPageResourceChecks(r.Context(), checks); err != nil {
		internalError(w, r, err)
		return
	}

	applog.Info("reparse-resources session=%s pages=%d resources=%d checked=%d", sessionID, pagesProcessed, len(uniqueResources), len(checks))
	writeJSON(w, map[string]any{
		"pages_processed":   pagesProcessed,
		"resources_found":   len(uniqueResources),
		"resources_checked": len(checks),
	})
}
