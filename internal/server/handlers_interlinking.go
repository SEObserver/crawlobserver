package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/interlinking"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/google/uuid"
)

// handleComputeInterlinking launches async interlinking analysis.
func (s *Server) handleComputeInterlinking(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	// Check HTML is stored
	hasHTML, err := s.store.HasStoredHTML(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !hasHTML {
		writeError(w, http.StatusBadRequest, "no stored HTML for this session (enable store_html in config)")
		return
	}

	// Parse options from body
	var opts struct {
		Method              string  `json:"method"`
		SimilarityThreshold float64 `json:"similarity_threshold"`
		MaxOpportunities    int     `json:"max_opportunities"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&opts)
	}
	if opts.Method == "" {
		opts.Method = "tfidf"
	}
	if opts.SimilarityThreshold == 0 {
		opts.SimilarityThreshold = s.cfg.Interlinking.SimilarityThreshold
	}
	if opts.MaxOpportunities == 0 {
		opts.MaxOpportunities = s.cfg.Interlinking.MaxOpportunities
	}

	go func() {
		if err := interlinking.ComputeOpportunities(context.Background(), s.store, interlinking.ComputeOpportunitiesOptions{
			SessionID:           sessionID,
			Method:              opts.Method,
			SimilarityThreshold: opts.SimilarityThreshold,
			MaxOpportunities:    opts.MaxOpportunities,
		}); err != nil {
			applog.Errorf("server", "ComputeInterlinking %s: %v", sessionID, err)
		}
	}()

	writeJSON(w, map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("Interlinking analysis started for session %s", sessionID),
	})
}

// handleInterlinkingOpportunities returns paginated interlinking opportunities.
func (s *Server) handleInterlinkingOpportunities(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.InterlinkingFilters)
	sort := parseSort(r, storage.InterlinkingSortColumns)

	opps, total, err := s.store.ListInterlinkingOpportunities(r.Context(), sessionID, limit, offset, filters, sort)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if opps == nil {
		opps = []storage.InterlinkingOpportunity{}
	}
	writeJSON(w, map[string]interface{}{
		"opportunities": opps,
		"total":         total,
	})
}

// handleSimulateInterlinking launches a PageRank simulation with virtual links.
func (s *Server) handleSimulateInterlinking(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	var body struct {
		Links []storage.VirtualLink `json:"links"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Links) == 0 {
		writeError(w, http.StatusBadRequest, "no links provided")
		return
	}

	simID := uuid.New().String()

	go func() {
		if _, err := interlinking.SimulatePageRank(context.Background(), s.store, sessionID, simID, body.Links); err != nil {
			applog.Errorf("server", "SimulateInterlinking %s: %v", sessionID, err)
		}
	}()

	writeJSON(w, map[string]string{
		"status":        "ok",
		"simulation_id": simID,
		"message":       fmt.Sprintf("PageRank simulation started for session %s", sessionID),
	})
}

// handleListSimulations returns all simulations for a session.
func (s *Server) handleListSimulations(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	sims, err := s.store.ListSimulations(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if sims == nil {
		sims = []storage.SimulationMeta{}
	}
	writeJSON(w, map[string]interface{}{
		"simulations": sims,
	})
}

// handleGetSimulationResults returns paginated results for a simulation.
func (s *Server) handleGetSimulationResults(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	simID := r.PathValue("simId")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	filters := parseFilters(r, storage.SimulationResultFilters)
	sort := parseSort(r, storage.SimulationResultSortColumns)

	// Get simulation meta
	meta, err := s.store.GetSimulation(r.Context(), sessionID, simID)
	if err != nil {
		internalError(w, r, err)
		return
	}

	results, total, err := s.store.ListSimulationResults(r.Context(), sessionID, simID, limit, offset, filters, sort)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if results == nil {
		results = []storage.SimulationResultRow{}
	}

	writeJSON(w, map[string]interface{}{
		"simulation": meta,
		"results":    results,
		"total":      total,
	})
}

// handleImportVirtualLinks allows importing a list of source/target URL pairs
// for PageRank simulation from external tools.
func (s *Server) handleImportVirtualLinks(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")

	var body struct {
		Links []storage.VirtualLink `json:"links"`
		Name  string                     `json:"name"` // optional label
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Links) == 0 {
		writeError(w, http.StatusBadRequest, "no links provided")
		return
	}

	simID := uuid.New().String()

	go func() {
		if _, err := interlinking.SimulatePageRank(context.Background(), s.store, sessionID, simID, body.Links); err != nil {
			applog.Errorf("server", "ImportVirtualLinks simulation %s: %v", sessionID, err)
		}
	}()

	writeJSON(w, map[string]string{
		"status":        "ok",
		"simulation_id": simID,
		"message":       fmt.Sprintf("Simulation with %d imported links started for session %s", len(body.Links), sessionID),
	})
}
