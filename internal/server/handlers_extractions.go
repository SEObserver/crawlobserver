package server

import (
	"encoding/json"
	"net/http"

	"github.com/SEObserver/crawlobserver/internal/extraction"
)

func (s *Server) handleListExtractorSets(w http.ResponseWriter, r *http.Request) {
	sets, err := s.keyStore.ListExtractorSets()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, sets)
}

func (s *Server) handleCreateExtractorSet(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Name       string                 `json:"name"`
		Extractors []extraction.Extractor `json:"extractors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	set, err := s.keyStore.CreateExtractorSet(req.Name, req.Extractors)
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, set)
}

func (s *Server) handleGetExtractorSet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	set, err := s.keyStore.GetExtractorSet(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "extractor set not found")
		return
	}
	writeJSON(w, set)
}

func (s *Server) handleUpdateExtractorSet(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Name       string                 `json:"name"`
		Extractors []extraction.Extractor `json:"extractors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.keyStore.UpdateExtractorSet(id, req.Name, req.Extractors); err != nil {
		writeError(w, http.StatusNotFound, "extractor set not found")
		return
	}
	set, err := s.keyStore.GetExtractorSet(id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, set)
}

func (s *Server) handleDeleteExtractorSet(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.keyStore.DeleteExtractorSet(id); err != nil {
		writeError(w, http.StatusNotFound, "extractor set not found")
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleGetExtractions(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	limit, offset = clampPagination(limit, offset)

	result, err := s.store.GetExtractions(r.Context(), sessionID, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleRunExtractions(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	var req struct {
		ExtractorSetID string `json:"extractor_set_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ExtractorSetID == "" {
		writeError(w, http.StatusBadRequest, "extractor_set_id is required")
		return
	}

	set, err := s.keyStore.GetExtractorSet(req.ExtractorSetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "extractor set not found")
		return
	}

	hasHTML, err := s.store.HasStoredHTML(r.Context(), sessionID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !hasHTML {
		writeError(w, http.StatusBadRequest, "no stored HTML for this session (enable store_html)")
		return
	}

	result, err := s.store.RunExtractionsPostCrawl(r.Context(), sessionID, set.Extractors)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}
