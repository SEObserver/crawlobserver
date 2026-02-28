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
		if _, err := client.GetDomainMetrics(r.Context(), body.Domain); err != nil {
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
		body.DataTypes = []string{"metrics", "backlinks", "refdomains", "rankings", "visibility"}
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

func (s *Server) runProviderFetch(ctx context.Context, cancel context.CancelFunc, projectID, provider string, conn *providers.ProviderConnection, dataTypes []string, key string) {
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			applog.Errorf("provider", "fetch PANIC: %v", r)
			s.providerFetchMu.Lock()
			s.providerFetchStatus[key] = &providerFetchStatus{Fetching: false, Error: fmt.Sprintf("panic: %v", r)}
			s.providerFetchMu.Unlock()
		}
	}()

	var client *seobserver.Client
	switch provider {
	case "seobserver":
		client = seobserver.NewClient(conn.APIKey)
	default:
		s.providerFetchMu.Lock()
		s.providerFetchStatus[key] = &providerFetchStatus{Fetching: false, Error: "unsupported provider"}
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
		if err := ctx.Err(); err != nil {
			return
		}
		metrics, err := client.GetDomainMetrics(ctx, domain)
		if err != nil {
			applog.Errorf("provider", "metrics error: %v", err)
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
		}
	}

	// Backlinks
	if wantType("backlinks") {
		setPhase("backlinks")
		if err := ctx.Err(); err != nil {
			return
		}
		backlinks, err := client.FetchBacklinks(ctx, domain, 1000)
		if err != nil {
			applog.Errorf("provider", "backlinks error: %v", err)
		} else if len(backlinks) > 0 {
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
		}
	}

	// RefDomains
	if wantType("refdomains") {
		setPhase("refdomains")
		if err := ctx.Err(); err != nil {
			return
		}
		refdoms, err := client.FetchRefDomains(ctx, domain, 1000)
		if err != nil {
			applog.Errorf("provider", "refdomains error: %v", err)
		} else if len(refdoms) > 0 {
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
		}
	}

	// Rankings
	if wantType("rankings") {
		setPhase("rankings")
		if err := ctx.Err(); err != nil {
			return
		}
		rankings, err := client.FetchRankings(ctx, domain, "fr", 1000, 0)
		if err != nil {
			applog.Errorf("provider", "rankings error: %v", err)
		} else if len(rankings) > 0 {
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
		}
	}

	// Visibility
	if wantType("visibility") {
		setPhase("visibility")
		if err := ctx.Err(); err != nil {
			return
		}
		vis, err := client.FetchVisibilityHistory(ctx, domain, "fr")
		if err != nil {
			applog.Errorf("provider", "visibility error: %v", err)
		} else if len(vis) > 0 {
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
		}
	}

	s.providerFetchMu.Lock()
	applog.Infof("provider", "fetch completed for %s/%s: %d total rows", projectID, provider, totalRows)
	delete(s.providerFetchStatus, key)
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
