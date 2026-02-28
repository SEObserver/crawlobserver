package server

import (
	"net/http"
)

func (s *Server) handleCompareStats(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		writeError(w, http.StatusBadRequest, "both 'a' and 'b' session IDs are required")
		return
	}
	if !s.requireSessionAccess(w, r, a) || !s.requireSessionAccess(w, r, b) {
		return
	}
	result, err := s.store.CompareStats(r.Context(), a, b)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleComparePages(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		writeError(w, http.StatusBadRequest, "both 'a' and 'b' session IDs are required")
		return
	}
	if !s.requireSessionAccess(w, r, a) || !s.requireSessionAccess(w, r, b) {
		return
	}
	diffType := r.URL.Query().Get("type")
	switch diffType {
	case "added", "removed", "changed":
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: added, removed, changed")
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	result, err := s.store.ComparePages(r.Context(), a, b, diffType, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleCompareLinks(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		writeError(w, http.StatusBadRequest, "both 'a' and 'b' session IDs are required")
		return
	}
	if !s.requireSessionAccess(w, r, a) || !s.requireSessionAccess(w, r, b) {
		return
	}
	diffType := r.URL.Query().Get("type")
	switch diffType {
	case "added", "removed":
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: added, removed")
		return
	}
	limit, offset := clampPagination(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	result, err := s.store.CompareLinks(r.Context(), a, b, diffType, limit, offset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}
