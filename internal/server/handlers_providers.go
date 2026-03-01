package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/providers"
	"github.com/SEObserver/crawlobserver/internal/seobserver"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

func (s *Server) handleListProviderConnections(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	conns, err := s.keyStore.ListProviderConnections(projectID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, conns)
}

func (s *Server) handleProviderConnect(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")

	var body struct {
		APIKey string `json:"api_key"`
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	// If no API key provided, reuse existing one (update scenario)
	apiKey := body.APIKey
	if apiKey == "" {
		existing, err := s.keyStore.GetProviderConnection(projectID, provider)
		if err != nil || existing.APIKey == "" {
			writeError(w, http.StatusBadRequest, "api_key is required for new connections")
			return
		}
		apiKey = existing.APIKey
	}

	// Validate key by calling the provider API
	switch provider {
	case "seobserver":
		client := seobserver.NewClient(apiKey)
		if _, _, err := client.GetDomainMetrics(r.Context(), body.Domain); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("SEObserver API validation failed: %v", err))
			return
		}
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported provider: %s", provider))
		return
	}

	conn := &providers.ProviderConnection{
		ProjectID: projectID,
		Provider:  provider,
		Domain:    body.Domain,
		APIKey:    apiKey,
	}
	if err := s.keyStore.SaveProviderConnection(conn); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "connected"})
}

func (s *Server) handleProviderDisconnect(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	s.keyStore.DeleteProviderConnection(projectID, provider)
	s.store.DeleteProviderData(r.Context(), projectID, provider)
	writeJSON(w, map[string]string{"status": "disconnected"})
}

func (s *Server) handleProviderStatus(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")

	conn, err := s.keyStore.GetProviderConnection(projectID, provider)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"connected": false,
		})
		return
	}

	result := map[string]interface{}{
		"connected":  true,
		"domain":     conn.Domain,
		"provider":   conn.Provider,
		"created_at": conn.CreatedAt,
	}

	key := projectID + ":" + provider
	s.providerFetchMu.Lock()
	if fs, ok := s.providerFetchStatus[key]; ok {
		result["fetch_status"] = fs
	}
	s.providerFetchMu.Unlock()

	writeJSON(w, result)
}

func (s *Server) providerFetchKey(projectID, provider string) string {
	return projectID + ":" + provider
}

func (s *Server) handleProviderFetch(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")

	var body struct {
		DataTypes []string `json:"data_types"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if len(body.DataTypes) == 0 {
		body.DataTypes = []string{"metrics", "backlinks", "refdomains", "rankings", "visibility", "top_pages"}
	}

	conn, err := s.keyStore.GetProviderConnection(projectID, provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no provider connection for this project")
		return
	}

	key := s.providerFetchKey(projectID, provider)

	s.providerFetchMu.Lock()
	if s.providerFetchStatus == nil {
		s.providerFetchStatus = make(map[string]*providerFetchStatus)
	}
	if existing := s.providerFetchStatus[key]; existing != nil && existing.cancel != nil {
		existing.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.providerFetchStatus[key] = &providerFetchStatus{Fetching: true, Phase: "starting", cancel: cancel}
	s.providerFetchMu.Unlock()

	go s.runProviderFetch(ctx, cancel, projectID, provider, conn, body.DataTypes, key)

	writeJSON(w, map[string]string{"status": "fetching"})
}

func (s *Server) logAPICall(ctx context.Context, meta *seobserver.APICallMeta, projectID, provider string, rowsReturned int, callErr error) {
	if meta == nil {
		return
	}
	errStr := ""
	if callErr != nil {
		errStr = callErr.Error()
	}
	row := storage.ProviderAPICallRow{
		ProjectID:    projectID,
		Provider:     provider,
		Endpoint:     meta.Endpoint,
		Method:       meta.Method,
		StatusCode:   meta.StatusCode,
		DurationMs:   meta.DurationMs,
		RowsReturned: uint32(rowsReturned),
		ResponseBody: meta.ResponseBody,
		Error:        errStr,
		CalledAt:     time.Now(),
	}
	if err := s.store.InsertProviderAPICalls(ctx, []storage.ProviderAPICallRow{row}); err != nil {
		applog.Errorf("provider", "insert api call log error: %v", err)
	}
}

func (s *Server) runProviderFetch(ctx context.Context, cancel context.CancelFunc, projectID, provider string, conn *providers.ProviderConnection, dataTypes []string, key string) {
	defer cancel()

	results := make(map[string]phaseResult)

	defer func() {
		if r := recover(); r != nil {
			applog.Errorf("provider", "fetch PANIC: %v", r)
			now := time.Now()
			s.providerFetchMu.Lock()
			s.providerFetchStatus[key] = &providerFetchStatus{
				Fetching: false, Error: fmt.Sprintf("panic: %v", r),
				CompletedAt: &now, PhaseResults: results,
			}
			s.providerFetchMu.Unlock()
		}
	}()

	var client *seobserver.Client
	switch provider {
	case "seobserver":
		client = seobserver.NewClient(conn.APIKey)
	default:
		now := time.Now()
		s.providerFetchMu.Lock()
		s.providerFetchStatus[key] = &providerFetchStatus{Fetching: false, Error: "unsupported provider", CompletedAt: &now}
		s.providerFetchMu.Unlock()
		return
	}

	domain := conn.Domain
	totalRows := 0

	setPhase := func(phase string) {
		s.providerFetchMu.Lock()
		s.providerFetchStatus[key] = &providerFetchStatus{Fetching: true, Phase: phase, RowsSoFar: totalRows, cancel: cancel}
		s.providerFetchMu.Unlock()
	}

	wantType := func(t string) bool {
		for _, dt := range dataTypes {
			if dt == t {
				return true
			}
		}
		return false
	}

	// Metrics
	if wantType("metrics") {
		setPhase("metrics")
		if ctx.Err() != nil {
			return
		}
		metrics, meta, err := client.GetDomainMetrics(ctx, domain)
		s.logAPICall(ctx, meta, projectID, provider, 1, err)
		if err != nil {
			applog.Errorf("provider", "metrics error: %v", err)
			results["metrics"] = phaseResult{Error: err.Error()}
		} else {
			row := storage.ProviderDomainMetricsRow{
				Provider: provider, Domain: domain,
				BacklinksTotal: metrics.BacklinksTotal, RefDomainsTotal: metrics.RefDomainsTotal,
				DomainRank: metrics.DomainRank, OrganicKeywords: metrics.OrganicKeywords,
				OrganicTraffic: metrics.OrganicTraffic, OrganicCost: metrics.OrganicCost,
			}
			if err := s.store.InsertProviderDomainMetrics(ctx, projectID, []storage.ProviderDomainMetricsRow{row}); err != nil {
				applog.Errorf("provider", "insert metrics error: %v", err)
			}
			totalRows++
			results["metrics"] = phaseResult{Rows: 1}
		}
	}

	// Backlinks
	if wantType("backlinks") {
		setPhase("backlinks")
		if ctx.Err() != nil {
			return
		}
		backlinks, meta, err := client.FetchBacklinks(ctx, domain, 1000)
		s.logAPICall(ctx, meta, projectID, provider, len(backlinks), err)
		if err != nil {
			applog.Errorf("provider", "backlinks error: %v", err)
			results["backlinks"] = phaseResult{Error: err.Error()}
		} else {
			insertRows := make([]storage.ProviderBacklinkRow, len(backlinks))
			for i, b := range backlinks {
				insertRows[i] = storage.ProviderBacklinkRow{
					Provider: provider, Domain: domain,
					SourceURL: b.SourceURL, TargetURL: b.TargetURL, AnchorText: b.AnchorText,
					SourceDomain: b.SourceDomain, LinkType: b.LinkType,
					DomainRank: b.DomainRank, PageRank: b.PageRank, Nofollow: b.Nofollow,
					FirstSeen: parseDate(b.FirstSeen), LastSeen: parseDate(b.LastSeen),
				}
			}
			if err := s.store.InsertProviderBacklinks(ctx, projectID, insertRows); err != nil {
				applog.Errorf("provider", "insert backlinks error: %v", err)
			}
			totalRows += len(insertRows)
			results["backlinks"] = phaseResult{Rows: len(insertRows)}
		}
	}

	// RefDomains
	if wantType("refdomains") {
		setPhase("refdomains")
		if ctx.Err() != nil {
			return
		}
		refdoms, meta, err := client.FetchRefDomains(ctx, domain, 1000)
		s.logAPICall(ctx, meta, projectID, provider, len(refdoms), err)
		if err != nil {
			applog.Errorf("provider", "refdomains error: %v", err)
			results["refdomains"] = phaseResult{Error: err.Error()}
		} else {
			insertRows := make([]storage.ProviderRefDomainRow, len(refdoms))
			for i, rd := range refdoms {
				insertRows[i] = storage.ProviderRefDomainRow{
					Provider: provider, Domain: domain,
					RefDomain: rd.Domain, BacklinkCount: rd.BacklinkCount, DomainRank: rd.DomainRank,
					FirstSeen: parseDate(rd.FirstSeen), LastSeen: parseDate(rd.LastSeen),
				}
			}
			if err := s.store.InsertProviderRefDomains(ctx, projectID, insertRows); err != nil {
				applog.Errorf("provider", "insert refdomains error: %v", err)
			}
			totalRows += len(insertRows)
			results["refdomains"] = phaseResult{Rows: len(insertRows)}
		}
	}

	// Rankings
	if wantType("rankings") {
		setPhase("rankings")
		if ctx.Err() != nil {
			return
		}
		rankings, meta, err := client.FetchRankings(ctx, domain, "fr", 1000, 0)
		s.logAPICall(ctx, meta, projectID, provider, len(rankings), err)
		if err != nil {
			applog.Errorf("provider", "rankings error: %v", err)
			results["rankings"] = phaseResult{Error: err.Error()}
		} else {
			insertRows := make([]storage.ProviderRankingRow, len(rankings))
			for i, rk := range rankings {
				insertRows[i] = storage.ProviderRankingRow{
					Provider: provider, Domain: domain,
					Keyword: rk.Keyword, URL: rk.URL, SearchBase: "fr",
					Position: rk.Position, SearchVolume: rk.SearchVolume,
					CPC: rk.CPC, Traffic: rk.Traffic, TrafficPct: rk.TrafficPct,
				}
			}
			if err := s.store.InsertProviderRankings(ctx, projectID, insertRows); err != nil {
				applog.Errorf("provider", "insert rankings error: %v", err)
			}
			totalRows += len(insertRows)
			results["rankings"] = phaseResult{Rows: len(insertRows)}
		}
	}

	// Visibility
	if wantType("visibility") {
		setPhase("visibility")
		if ctx.Err() != nil {
			return
		}
		vis, meta, err := client.FetchVisibilityHistory(ctx, domain, "fr")
		s.logAPICall(ctx, meta, projectID, provider, len(vis), err)
		if err != nil {
			applog.Errorf("provider", "visibility error: %v", err)
			results["visibility"] = phaseResult{Error: err.Error()}
		} else {
			insertRows := make([]storage.ProviderVisibilityRow, len(vis))
			for i, v := range vis {
				insertRows[i] = storage.ProviderVisibilityRow{
					Provider: provider, Domain: domain, SearchBase: "fr",
					Date: parseDate(v.Date), Visibility: v.Visibility, KeywordsCount: v.KeywordsCount,
				}
			}
			if err := s.store.InsertProviderVisibility(ctx, projectID, insertRows); err != nil {
				applog.Errorf("provider", "insert visibility error: %v", err)
			}
			totalRows += len(insertRows)
			results["visibility"] = phaseResult{Rows: len(insertRows)}
		}
	}

	// Top Pages
	if wantType("top_pages") {
		setPhase("top_pages")
		if ctx.Err() != nil {
			return
		}
		pages, meta, err := client.FetchTopPages(ctx, domain, 1000)
		s.logAPICall(ctx, meta, projectID, provider, len(pages), err)
		if err != nil {
			applog.Errorf("provider", "top_pages error: %v", err)
			results["top_pages"] = phaseResult{Error: err.Error()}
		} else {
			insertRows := make([]storage.ProviderTopPageRow, len(pages))
			for i, p := range pages {
				ttf := make([]storage.TopicalTF, len(p.TopicalTrustFlow))
				for j, t := range p.TopicalTrustFlow {
					ttf[j] = storage.TopicalTF{Topic: t.Topic, Value: t.Value}
				}
				insertRows[i] = storage.ProviderTopPageRow{
					Provider: provider, Domain: domain,
					URL: p.URL, Title: p.Title,
					TrustFlow: p.TrustFlow, CitationFlow: p.CitationFlow,
					ExtBackLinks: p.ExtBackLinks, RefDomains: p.RefDomains,
					TopicalTrustFlow: ttf, Language: p.Language,
				}
			}
			if err := s.store.InsertProviderTopPages(ctx, projectID, insertRows); err != nil {
				applog.Errorf("provider", "insert top_pages error: %v", err)
			}
			totalRows += len(insertRows)
			results["top_pages"] = phaseResult{Rows: len(insertRows)}
		}
	}

	now := time.Now()
	s.providerFetchMu.Lock()
	applog.Infof("provider", "fetch completed for %s/%s: %d total rows", projectID, provider, totalRows)
	s.providerFetchStatus[key] = &providerFetchStatus{
		Fetching:     false,
		Phase:        "done",
		RowsSoFar:    totalRows,
		CompletedAt:  &now,
		PhaseResults: results,
	}
	s.providerFetchMu.Unlock()
}

func parseDate(s string) time.Time {
	layouts := []string{"2006-01-02", "2006-01-02T15:04:05", time.RFC3339}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (s *Server) handleProviderStopFetch(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	key := s.providerFetchKey(projectID, provider)
	s.providerFetchMu.Lock()
	if fs, ok := s.providerFetchStatus[key]; ok && fs.cancel != nil {
		fs.cancel()
	}
	delete(s.providerFetchStatus, key)
	s.providerFetchMu.Unlock()
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleProviderMetrics(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	metrics, err := s.store.ProviderDomainMetrics(r.Context(), projectID, provider)
	if err != nil {
		writeJSON(w, map[string]interface{}{})
		return
	}
	writeJSON(w, metrics)
}

func (s *Server) handleProviderBacklinks(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	limit, offset = clampPagination(limit, offset)
	rows, total, err := s.store.ProviderBacklinks(r.Context(), projectID, provider, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

func (s *Server) handleProviderRefDomains(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	limit, offset = clampPagination(limit, offset)
	rows, total, err := s.store.ProviderRefDomains(r.Context(), projectID, provider, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

func (s *Server) handleProviderRankings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	limit, offset = clampPagination(limit, offset)
	rows, total, err := s.store.ProviderRankings(r.Context(), projectID, provider, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

func (s *Server) handleProviderVisibility(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	rows, err := s.store.ProviderVisibilityHistory(r.Context(), projectID, provider)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleProviderTopPages(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	limit, offset = clampPagination(limit, offset)
	rows, total, err := s.store.ProviderTopPages(r.Context(), projectID, provider, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

func (s *Server) handleProviderAPICalls(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := r.PathValue("provider")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	limit, offset = clampPagination(limit, offset)
	rows, total, err := s.store.ProviderAPICalls(r.Context(), projectID, provider, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}

func (s *Server) handleSessionAuthority(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	limit, offset = clampPagination(limit, offset)
	rows, total, err := s.store.PagesWithAuthority(r.Context(), sessionID, projectID, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows, "total": total})
}
