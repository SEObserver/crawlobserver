package server

import (
	"encoding/json"
	"net/http"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/spf13/viper"
)

// handleAnnouncements returns the currently cached announcement (if any)
// along with the user's enabled/disabled preference. If the feature is
// disabled or no message is cached yet, message is null.
func (s *Server) handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	s.announcerMu.RLock()
	enabled := s.cfg.Announcements.Enabled
	fetcher := s.announcer
	s.announcerMu.RUnlock()

	payload := map[string]interface{}{
		"enabled": enabled,
		"message": nil,
	}

	if enabled && fetcher != nil {
		if msg, _ := fetcher.Snapshot(); msg != nil {
			payload["message"] = msg
		}
	}

	writeJSON(w, payload)
}

// handleUpdateAnnouncementsSettings lets the user opt out (or back in) of the
// announcements banner. The setting is persisted to config.yaml and the
// background fetcher is started or stopped accordingly.
func (s *Server) handleUpdateAnnouncementsSettings(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.announcerMu.Lock()
	s.cfg.Announcements.Enabled = req.Enabled
	s.announcerMu.Unlock()

	viper.Set("announcements.enabled", req.Enabled)
	if err := viperWriteConfig(); err != nil {
		internalError(w, r, err)
		return
	}

	if req.Enabled {
		s.startAnnouncer()
	} else {
		s.stopAnnouncer()
		applog.Info("server", "Announcements fetcher stopped (disabled by user)")
	}

	writeJSON(w, map[string]interface{}{
		"enabled": req.Enabled,
	})
}
