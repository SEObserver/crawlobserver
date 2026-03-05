package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/backup"
	"github.com/SEObserver/crawlobserver/internal/updater"
)

func (s *Server) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	stats, err := s.store.StorageStats(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleSessionStorage(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	stats, err := s.store.SessionStorageStats(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleGlobalStats(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}

	sessionStats, storageResult, err := s.store.GlobalStats(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Get exact per-session storage from system.parts
	sessionStorage, err := s.store.SessionStorageStats(r.Context())
	if err != nil {
		// Non-fatal: fall back to proportional estimation
		applog.Warnf("server", "session storage stats unavailable: %v", err)
		sessionStorage = map[string]uint64{}
	}

	// Load sessions
	sessions, err := s.store.ListSessions(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Load projects
	var projectMap map[string]string // id -> name
	if s.keyStore != nil {
		projects, _ := s.keyStore.ListProjects()
		projectMap = make(map[string]string, len(projects))
		for _, p := range projects {
			projectMap[p.ID] = p.Name
		}
	}

	// Auto-assign orphan sessions: create a project per hostname and associate
	if s.keyStore != nil {
		// Build reverse map: project name -> id
		nameToID := make(map[string]string, len(projectMap))
		for id, name := range projectMap {
			nameToID[name] = id
		}

		for i, sess := range sessions {
			if sess.ProjectID != nil {
				continue
			}
			// Extract hostname from first seed URL
			hostname := "unknown"
			if len(sess.SeedURLs) > 0 {
				if u, err := url.Parse(sess.SeedURLs[0]); err == nil && u.Hostname() != "" {
					hostname = u.Hostname()
				}
			}
			// Find or create project for this hostname
			pid, exists := nameToID[hostname]
			if !exists {
				p, err := s.keyStore.CreateProject(hostname)
				if err != nil {
					applog.Warnf("server", "auto-assign: failed to create project %q: %v", hostname, err)
					continue
				}
				pid = p.ID
				nameToID[hostname] = pid
				projectMap[pid] = hostname
			}
			// Associate session to project
			if err := s.store.UpdateSessionProject(r.Context(), sess.ID, &pid); err != nil {
				applog.Warnf("server", "auto-assign: failed to associate session %s: %v", sess.ID, err)
				continue
			}
			sessions[i].ProjectID = &pid
		}
	}

	// Build session-to-project mapping
	type sessionInfo struct {
		ProjectID *string
		SeedURLs  []string
	}
	sessionMap := map[string]sessionInfo{}
	for _, sess := range sessions {
		sessionMap[sess.ID] = sessionInfo{ProjectID: sess.ProjectID, SeedURLs: sess.SeedURLs}
	}

	// Aggregate by project
	type projectStats struct {
		ProjectID    *string `json:"project_id"`
		ProjectName  string  `json:"project_name"`
		Sessions     int     `json:"sessions"`
		TotalPages   uint64  `json:"total_pages"`
		TotalLinks   uint64  `json:"total_links"`
		ErrorCount   uint64  `json:"error_count"`
		AvgFetchMs   float64 `json:"avg_fetch_ms"`
		StorageBytes uint64  `json:"storage_bytes"`
	}

	type fetchAcc struct {
		sum   float64
		count uint64
	}

	projectAgg := map[string]*projectStats{}
	projectFetch := map[string]*fetchAcc{}
	var globalPages, globalLinks, globalErrors uint64
	var globalFetchSum float64
	var globalFetchCount uint64

	for _, gs := range sessionStats {
		info := sessionMap[gs.SessionID]
		key := ""
		if info.ProjectID != nil {
			key = *info.ProjectID
		}
		ps, ok := projectAgg[key]
		if !ok {
			ps = &projectStats{ProjectID: info.ProjectID}
			if info.ProjectID != nil && projectMap != nil {
				ps.ProjectName = projectMap[*info.ProjectID]
			} else {
				ps.ProjectName = "(No project)"
			}
			projectAgg[key] = ps
			projectFetch[key] = &fetchAcc{}
		}
		ps.Sessions++
		ps.TotalPages += gs.TotalPages
		ps.TotalLinks += gs.TotalLinks
		ps.ErrorCount += gs.ErrorCount

		fa := projectFetch[key]
		fa.sum += gs.AvgFetchMs * float64(gs.TotalPages)
		fa.count += gs.TotalPages

		globalFetchSum += gs.AvgFetchMs * float64(gs.TotalPages)
		globalFetchCount += gs.TotalPages

		// Exact storage from system.parts partitions
		ps.StorageBytes += sessionStorage[gs.SessionID]

		globalPages += gs.TotalPages
		globalLinks += gs.TotalLinks
		globalErrors += gs.ErrorCount
	}

	// Compute weighted avg fetch per project
	for key, ps := range projectAgg {
		if fa := projectFetch[key]; fa.count > 0 {
			ps.AvgFetchMs = fa.sum / float64(fa.count)
		}
	}

	projectList := make([]projectStats, 0, len(projectAgg))
	for _, ps := range projectAgg {
		projectList = append(projectList, *ps)
	}

	var globalAvgFetch float64
	if globalFetchCount > 0 {
		globalAvgFetch = globalFetchSum / float64(globalFetchCount)
	}

	var totalStorage uint64
	for _, t := range storageResult.Tables {
		totalStorage += t.BytesOnDisk
	}

	writeJSON(w, map[string]interface{}{
		"total_pages":    globalPages,
		"total_links":    globalLinks,
		"total_errors":   globalErrors,
		"avg_fetch_ms":   globalAvgFetch,
		"total_storage":  totalStorage,
		"total_sessions": len(sessions),
		"projects":       projectList,
		"storage_tables": storageResult.Tables,
	})
}

// --- Project handlers ---

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	// Paginated mode if ?limit= is present
	if r.URL.Query().Get("limit") != "" {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 30
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, offset = clampPagination(limit, offset)
		search := r.URL.Query().Get("search")

		projects, total, err := s.keyStore.ListProjectsPaginated(limit, offset, search)
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, map[string]any{
			"projects": projects,
			"total":    total,
		})
		return
	}

	projects, err := s.keyStore.ListProjects()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.keyStore.CreateProject(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to create project")
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}

func (s *Server) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.keyStore.RenameProject(id, req.Name); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, map[string]string{"status": "renamed"})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.keyStore.DeleteProject(id); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleDeleteProjectWithSessions(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")

	// List all sessions belonging to this project
	sessions, err := s.store.ListSessions(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}

	// Delete each session (skip running ones)
	for _, sess := range sessions {
		if s.manager.IsRunning(sess.ID) {
			continue
		}
		_ = s.store.DeleteSession(r.Context(), sess.ID)
	}

	// Delete the project itself
	if err := s.keyStore.DeleteProject(id); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleAssociateSession(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	pid := r.PathValue("pid")
	sid := r.PathValue("sid")

	// Verify project exists
	if _, err := s.keyStore.GetProject(pid); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err := s.store.UpdateSessionProject(r.Context(), sid, &pid); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "associated"})
}

func (s *Server) handleDisassociateSession(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sid := r.PathValue("sid")
	if err := s.store.UpdateSessionProject(r.Context(), sid, nil); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "disassociated"})
}

// --- API Key handlers ---

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	keys, err := s.keyStore.ListAPIKeys()
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, keys)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	var req struct {
		Name      string  `json:"name"`
		Type      string  `json:"type"`
		ProjectID *string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	result, err := s.keyStore.CreateAPIKey(req.Name, req.Type, req.ProjectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to create API key")
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, result)
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.keyStore.DeleteAPIKey(id); err != nil {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// --- Update & Backup handlers ---

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if s.UpdateStatus == nil {
		writeJSON(w, map[string]interface{}{"available": false, "current_version": updater.Version})
		return
	}
	writeJSON(w, s.UpdateStatus.Snapshot())
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.UpdateStatus == nil {
		writeError(w, http.StatusBadRequest, "update check not available")
		return
	}
	release := s.UpdateStatus.Release()
	if release == nil {
		writeError(w, http.StatusBadRequest, "no release info available, check for updates first")
		return
	}

	// Auto-backup SQLite before applying update
	if s.BackupOpts != nil && s.BackupOpts.SQLitePath != "" {
		preUpdateOpts := backup.BackupOptions{
			SQLitePath: s.BackupOpts.SQLitePath,
			BackupDir:  s.BackupOpts.BackupDir,
		}
		if info, err := backup.Create(preUpdateOpts, updater.Version); err != nil {
			applog.Warnf("server", "pre-update backup failed: %v", err)
		} else {
			applog.Infof("server", "Pre-update backup created: %s", info.Filename)
		}
	}

	if s.IsDesktop {
		newAppPath, err := updater.DownloadDesktopUpdate(release)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if err := updater.SelfUpdateDesktop(newAppPath); err != nil {
			internalError(w, r, err)
			return
		}
	} else {
		tmpPath, err := updater.DownloadUpdate(release)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if err := updater.SelfUpdate(tmpPath); err != nil {
			internalError(w, r, err)
			return
		}
	}

	writeJSON(w, map[string]string{"status": "installed", "message": "Restart the application to use the new version."})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if s.BackupOpts == nil {
		writeJSON(w, []backup.BackupInfo{})
		return
	}
	backups, err := backup.ListBackups(s.BackupOpts.BackupDir)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if backups == nil {
		backups = []backup.BackupInfo{}
	}
	writeJSON(w, backups)
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.BackupOpts == nil {
		writeError(w, http.StatusBadRequest, "backup not configured")
		return
	}

	// Stop ClickHouse for consistency
	if s.StopClickHouse != nil {
		applog.Info("server", "Stopping ClickHouse for backup...")
		s.StopClickHouse()
	}

	info, err := backup.Create(*s.BackupOpts, updater.Version)

	// Restart ClickHouse regardless of backup result
	if s.StartClickHouse != nil {
		applog.Info("server", "Restarting ClickHouse after backup...")
		if startErr := s.StartClickHouse(); startErr != nil {
			applog.Warnf("server", "failed to restart ClickHouse: %v", startErr)
		}
	}

	if err != nil {
		internalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, info)
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.BackupOpts == nil {
		writeError(w, http.StatusBadRequest, "backup not configured")
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		writeError(w, http.StatusBadRequest, "filename is required")
		return
	}

	// Sanitize filename: extract base name to prevent path traversal
	cleanName := filepath.Base(req.Filename)
	if cleanName == "." || cleanName == "/" || cleanName == "\\" {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	archivePath := filepath.Join(s.BackupOpts.BackupDir, cleanName)
	if _, err := os.Stat(archivePath); err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}

	// Stop ClickHouse for restore
	if s.StopClickHouse != nil {
		applog.Info("server", "Stopping ClickHouse for restore...")
		s.StopClickHouse()
	}

	err := backup.Restore(archivePath, *s.BackupOpts)

	// Restart ClickHouse regardless of restore result
	if s.StartClickHouse != nil {
		applog.Info("server", "Restarting ClickHouse after restore...")
		if startErr := s.StartClickHouse(); startErr != nil {
			applog.Warnf("server", "failed to restart ClickHouse: %v", startErr)
		}
	}

	if err != nil {
		internalError(w, r, err)
		return
	}

	writeJSON(w, map[string]string{"status": "restored", "message": "Restart the application to apply restored data."})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	if s.BackupOpts == nil {
		writeError(w, http.StatusBadRequest, "backup not configured")
		return
	}
	name := filepath.Base(r.PathValue("name"))
	if name == "." || name == "/" || name == "\\" {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	archivePath := filepath.Join(s.BackupOpts.BackupDir, name)
	if err := backup.DeleteBackup(archivePath); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// --- Logs handlers ---

func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, offset = clampPagination(limit, offset)
	level := r.URL.Query().Get("level")
	component := r.URL.Query().Get("component")
	search := r.URL.Query().Get("search")

	logs, total, err := s.store.ListLogs(r.Context(), limit, offset, level, component, search)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{
		"logs":  logs,
		"total": total,
	})
}

func (s *Server) handleExportLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.store.ExportLogs(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=application_logs.jsonl")
	enc := json.NewEncoder(w)
	for _, l := range logs {
		enc.Encode(l)
	}
}
