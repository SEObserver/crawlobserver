package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/SEObserver/seocrawler/internal/customtests"
)

func (s *Server) handleListRulesets(w http.ResponseWriter, r *http.Request) {
	rulesets, err := s.keyStore.ListRulesets()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, rulesets)
}

func (s *Server) handleCreateRuleset(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Name  string                 `json:"name"`
		Rules []customtests.TestRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	ruleset, err := s.keyStore.CreateRuleset(req.Name, req.Rules)
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, ruleset)
}

func (s *Server) handleGetRuleset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ruleset, err := s.keyStore.GetRuleset(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "ruleset not found")
		return
	}
	writeJSON(w, ruleset)
}

func (s *Server) handleUpdateRuleset(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Name  string                 `json:"name"`
		Rules []customtests.TestRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.keyStore.UpdateRuleset(id, req.Name, req.Rules); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	ruleset, err := s.keyStore.GetRuleset(id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, ruleset)
}

func (s *Server) handleDeleteRuleset(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.keyStore.DeleteRuleset(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleRunTests(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.requireSessionAccess(w, r, sessionID) {
		return
	}

	var req struct {
		RulesetID string `json:"ruleset_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RulesetID == "" {
		writeError(w, http.StatusBadRequest, "ruleset_id is required")
		return
	}

	ruleset, err := s.keyStore.GetRuleset(req.RulesetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ruleset not found")
		return
	}

	adapter := &customTestsStorageAdapter{store: s.store}
	result, err := customtests.RunTests(r.Context(), adapter, sessionID, ruleset)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, result)
}

// customTestsStorageAdapter adapts StorageService to customtests.StorageInterface.
type customTestsStorageAdapter struct {
	store StorageService
}

func (a *customTestsStorageAdapter) RunCustomTestsSQL(ctx context.Context, sessionID string, rules []customtests.TestRule) (map[string]map[string]string, error) {
	return a.store.RunCustomTestsSQL(ctx, sessionID, rules)
}

func (a *customTestsStorageAdapter) StreamPagesHTML(ctx context.Context, sessionID string) (<-chan customtests.PageHTMLRow, error) {
	ch, err := a.store.StreamPagesHTML(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make(chan customtests.PageHTMLRow, 64)
	go func() {
		defer close(out)
		for row := range ch {
			out <- customtests.PageHTMLRow{URL: row.URL, HTML: row.HTML}
		}
	}()
	return out, nil
}
