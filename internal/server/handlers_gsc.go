package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/gsc"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

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
