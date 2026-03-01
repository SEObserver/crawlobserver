package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/crawler"
	"github.com/SEObserver/crawlobserver/internal/customtests"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// ---------------------------------------------------------------------------
// mockStore implements StorageService
// ---------------------------------------------------------------------------

type mockStore struct {
	sessions             []storage.CrawlSession
	pages                []storage.PageRow
	links                []storage.LinkRow
	stats                *storage.SessionStats
	pageHTML             string
	page                 *storage.PageRow
	pageLinks            *storage.PageLinksResult
	storageStats         *storage.StorageStatsResult
	sessionStorageStats  map[string]uint64
	globalSessions       []storage.GlobalSessionStats
	pagerankDist         *storage.PageRankDistributionResult
	pagerankTreemap      []storage.PageRankTreemapEntry
	pagerankTop          *storage.PageRankTopResult
	robotsHosts          []storage.RobotsRow
	robotsContent        *storage.RobotsRow
	sitemaps             []storage.SitemapRow
	sitemapURLs          []storage.SitemapURLRow
	urlsByHost           map[string][]string // host prefix -> URLs
	compareStatsResult   *storage.CompareStatsResult
	comparePagesResult   *storage.PageDiffResult
	compareLinksResult   *storage.LinkDiffResult
	auditResult          *storage.AuditResult
	expiredDomainsResult *storage.ExpiredDomainsResult
	err                  error
	deleteCalls          []string
	updateProjectCalls   []updateProjectCall
	getSessionByID       map[string]*storage.CrawlSession
	listPagesCalls       []listPagesCall
	deleteProviderCalls  []deleteProviderCall
}

type listPagesCall struct {
	SessionID string
	Limit     int
	Offset    int
	Filters   []storage.ParsedFilter
}

type deleteProviderCall struct {
	ProjectID string
	Provider  string
}

type updateProjectCall struct {
	SessionID string
	ProjectID *string
}

func (m *mockStore) ListSessions(_ context.Context, projectID ...string) ([]storage.CrawlSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(projectID) > 0 && projectID[0] != "" {
		var filtered []storage.CrawlSession
		for _, s := range m.sessions {
			if s.ProjectID != nil && *s.ProjectID == projectID[0] {
				filtered = append(filtered, s)
			}
		}
		return filtered, nil
	}
	return m.sessions, nil
}

func (m *mockStore) ListSessionsPaginated(_ context.Context, limit, offset int, projectID, search string) ([]storage.CrawlSession, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.sessions, len(m.sessions), nil
}

func (m *mockStore) GetSession(_ context.Context, sessionID string) (*storage.CrawlSession, error) {
	if m.getSessionByID != nil {
		if s, ok := m.getSessionByID[sessionID]; ok {
			return s, nil
		}
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	if len(m.sessions) > 0 {
		for _, s := range m.sessions {
			if s.ID == sessionID {
				return &s, nil
			}
		}
	}
	return nil, fmt.Errorf("session %s not found", sessionID)
}

func (m *mockStore) DeleteSession(_ context.Context, sessionID string) error {
	m.deleteCalls = append(m.deleteCalls, sessionID)
	return m.err
}

func (m *mockStore) UpdateSessionProject(_ context.Context, sessionID string, projectID *string) error {
	m.updateProjectCalls = append(m.updateProjectCalls, updateProjectCall{sessionID, projectID})
	return m.err
}

func (m *mockStore) ListPages(_ context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter) ([]storage.PageRow, error) {
	m.listPagesCalls = append(m.listPagesCalls, listPagesCall{sessionID, limit, offset, filters})
	return m.pages, m.err
}

func (m *mockStore) ExternalLinksPaginated(_ context.Context, _ string, _, _ int, _ []storage.ParsedFilter) ([]storage.LinkRow, error) {
	return m.links, m.err
}

func (m *mockStore) InternalLinksPaginated(_ context.Context, _ string, _, _ int, _ []storage.ParsedFilter) ([]storage.LinkRow, error) {
	return m.links, m.err
}

func (m *mockStore) SessionStats(_ context.Context, _ string) (*storage.SessionStats, error) {
	return m.stats, m.err
}

func (m *mockStore) SessionAudit(_ context.Context, _ string) (*storage.AuditResult, error) {
	if m.auditResult != nil {
		return m.auditResult, m.err
	}
	return &storage.AuditResult{}, m.err
}

func (m *mockStore) ExportSession(_ context.Context, _ string, _ io.Writer, _ bool) error {
	return m.err
}

func (m *mockStore) ImportSession(_ context.Context, _ io.Reader) (*storage.CrawlSession, error) {
	return &storage.CrawlSession{}, m.err
}

func (m *mockStore) GetPageHTML(_ context.Context, _, _ string) (string, error) {
	return m.pageHTML, m.err
}

func (m *mockStore) GetPage(_ context.Context, _, _ string) (*storage.PageRow, error) {
	return m.page, m.err
}

func (m *mockStore) GetPageLinks(_ context.Context, _, _ string, _, _, _, _ int) (*storage.PageLinksResult, error) {
	return m.pageLinks, m.err
}

func (m *mockStore) StorageStats(_ context.Context) (*storage.StorageStatsResult, error) {
	if m.storageStats != nil {
		return m.storageStats, m.err
	}
	return &storage.StorageStatsResult{}, m.err
}

func (m *mockStore) SessionStorageStats(_ context.Context) (map[string]uint64, error) {
	if m.sessionStorageStats != nil {
		return m.sessionStorageStats, m.err
	}
	return map[string]uint64{}, m.err
}

func (m *mockStore) GlobalStats(_ context.Context) ([]storage.GlobalSessionStats, *storage.StorageStatsResult, error) {
	ss := m.globalSessions
	if ss == nil {
		ss = []storage.GlobalSessionStats{}
	}
	sr := m.storageStats
	if sr == nil {
		sr = &storage.StorageStatsResult{}
	}
	return ss, sr, m.err
}

func (m *mockStore) RecomputeDepths(_ context.Context, _ string, _ []string) error {
	return m.err
}

func (m *mockStore) ComputePageRank(_ context.Context, _ string) error {
	return m.err
}

func (m *mockStore) PageRankDistribution(_ context.Context, _ string, _ int) (*storage.PageRankDistributionResult, error) {
	return m.pagerankDist, m.err
}

func (m *mockStore) PageRankTreemap(_ context.Context, _ string, _, _ int) ([]storage.PageRankTreemapEntry, error) {
	return m.pagerankTreemap, m.err
}

func (m *mockStore) PageRankTop(_ context.Context, _ string, _, _ int, _ string) (*storage.PageRankTopResult, error) {
	return m.pagerankTop, m.err
}

func (m *mockStore) GetRobotsHosts(_ context.Context, _ string) ([]storage.RobotsRow, error) {
	return m.robotsHosts, m.err
}

func (m *mockStore) GetRobotsContent(_ context.Context, _, _ string) (*storage.RobotsRow, error) {
	return m.robotsContent, m.err
}

func (m *mockStore) GetSitemaps(_ context.Context, _ string) ([]storage.SitemapRow, error) {
	return m.sitemaps, m.err
}

func (m *mockStore) GetSitemapURLs(_ context.Context, _, _ string, _, _ int) ([]storage.SitemapURLRow, error) {
	return m.sitemapURLs, m.err
}

func (m *mockStore) GetURLsByHost(_ context.Context, _ string, host string) ([]string, error) {
	if m.urlsByHost != nil {
		return m.urlsByHost[host], m.err
	}
	return nil, m.err
}

// Compare mock methods
func (m *mockStore) CompareStats(_ context.Context, _, _ string) (*storage.CompareStatsResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.compareStatsResult, nil
}
func (m *mockStore) ComparePages(_ context.Context, _, _, _ string, _, _ int) (*storage.PageDiffResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.comparePagesResult, nil
}
func (m *mockStore) CompareLinks(_ context.Context, _, _, _ string, _, _ int) (*storage.LinkDiffResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.compareLinksResult, nil
}

// GSC mock methods
func (m *mockStore) InsertGSCAnalytics(_ context.Context, _ string, _ []storage.GSCAnalyticsInsertRow) error {
	return m.err
}
func (m *mockStore) InsertGSCInspection(_ context.Context, _ string, _ []storage.GSCInspectionInsertRow) error {
	return m.err
}
func (m *mockStore) GSCOverview(_ context.Context, _ string) (*storage.GSCOverviewStats, error) {
	return &storage.GSCOverviewStats{}, m.err
}
func (m *mockStore) GSCTopQueries(_ context.Context, _ string, _, _ int) ([]storage.GSCQueryRow, int, error) {
	return []storage.GSCQueryRow{}, 0, m.err
}
func (m *mockStore) GSCTopPages(_ context.Context, _ string, _, _ int) ([]storage.GSCPageRow, int, error) {
	return []storage.GSCPageRow{}, 0, m.err
}
func (m *mockStore) GSCByCountry(_ context.Context, _ string) ([]storage.GSCCountryRow, error) {
	return []storage.GSCCountryRow{}, m.err
}
func (m *mockStore) GSCByDevice(_ context.Context, _ string) ([]storage.GSCDeviceRow, error) {
	return []storage.GSCDeviceRow{}, m.err
}
func (m *mockStore) GSCTimeline(_ context.Context, _ string) ([]storage.GSCTimelineRow, error) {
	return []storage.GSCTimelineRow{}, m.err
}
func (m *mockStore) GSCInspectionResults(_ context.Context, _ string, _, _ int) ([]storage.GSCInspectionRow, int, error) {
	return []storage.GSCInspectionRow{}, 0, m.err
}
func (m *mockStore) DeleteGSCData(_ context.Context, _ string) error {
	return m.err
}

// Custom Tests mock methods
func (m *mockStore) RunCustomTestsSQL(_ context.Context, _ string, rules []customtests.TestRule) (map[string]map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make(map[string]map[string]string)
	result["https://example.com/"] = make(map[string]string)
	for _, r := range rules {
		result["https://example.com/"][r.ID] = "pass"
	}
	return result, nil
}
func (m *mockStore) StreamPagesHTML(_ context.Context, _ string) (<-chan storage.PageHTMLRow, error) {
	ch := make(chan storage.PageHTMLRow)
	go func() {
		defer close(ch)
		ch <- storage.PageHTMLRow{URL: "https://example.com/", HTML: "<html><head><title>Test</title></head><body><h1>Hello</h1></body></html>"}
	}()
	return ch, m.err
}

// External Link Check mock methods
func (m *mockStore) GetExternalLinkChecks(_ context.Context, _ string, _, _ int, _ []storage.ParsedFilter) ([]storage.ExternalLinkCheck, error) {
	return []storage.ExternalLinkCheck{}, m.err
}
func (m *mockStore) GetExternalLinkCheckDomains(_ context.Context, _ string, _, _ int, _ []storage.ParsedFilter) ([]storage.ExternalDomainCheck, error) {
	return []storage.ExternalDomainCheck{}, m.err
}
func (m *mockStore) GetExpiredDomains(_ context.Context, _ string, _, _ int) (*storage.ExpiredDomainsResult, error) {
	if m.expiredDomainsResult != nil {
		return m.expiredDomainsResult, m.err
	}
	return &storage.ExpiredDomainsResult{Domains: []storage.ExpiredDomain{}, Total: 0}, m.err
}

// Page Resource Checks mock methods
func (m *mockStore) GetPageResourceChecks(_ context.Context, _ string, _, _ int, _ []storage.ParsedFilter) ([]storage.PageResourceCheck, error) {
	return []storage.PageResourceCheck{}, m.err
}
func (m *mockStore) GetPageResourceTypeSummary(_ context.Context, _ string) ([]storage.ResourceTypeSummary, error) {
	return []storage.ResourceTypeSummary{}, m.err
}

// Application Logs mock methods
func (m *mockStore) InsertLogs(_ context.Context, _ []applog.LogRow) error {
	return m.err
}
func (m *mockStore) ListLogs(_ context.Context, _, _ int, _, _, _ string) ([]applog.LogRow, int, error) {
	return []applog.LogRow{}, 0, m.err
}
func (m *mockStore) ExportLogs(_ context.Context) ([]applog.LogRow, error) {
	return []applog.LogRow{}, m.err
}

// Provider mock methods
func (m *mockStore) InsertProviderDomainMetrics(_ context.Context, _ string, _ []storage.ProviderDomainMetricsRow) error {
	return m.err
}
func (m *mockStore) InsertProviderBacklinks(_ context.Context, _ string, _ []storage.ProviderBacklinkRow) error {
	return m.err
}
func (m *mockStore) InsertProviderRefDomains(_ context.Context, _ string, _ []storage.ProviderRefDomainRow) error {
	return m.err
}
func (m *mockStore) InsertProviderRankings(_ context.Context, _ string, _ []storage.ProviderRankingRow) error {
	return m.err
}
func (m *mockStore) InsertProviderVisibility(_ context.Context, _ string, _ []storage.ProviderVisibilityRow) error {
	return m.err
}
func (m *mockStore) ProviderDomainMetrics(_ context.Context, _, _ string) (*storage.ProviderDomainMetricsRow, error) {
	return &storage.ProviderDomainMetricsRow{}, m.err
}
func (m *mockStore) ProviderBacklinks(_ context.Context, _, _ string, _, _ int) ([]storage.ProviderBacklinkRow, int, error) {
	return []storage.ProviderBacklinkRow{}, 0, m.err
}
func (m *mockStore) ProviderRefDomains(_ context.Context, _, _ string, _, _ int) ([]storage.ProviderRefDomainRow, int, error) {
	return []storage.ProviderRefDomainRow{}, 0, m.err
}
func (m *mockStore) ProviderRankings(_ context.Context, _, _ string, _, _ int) ([]storage.ProviderRankingRow, int, error) {
	return []storage.ProviderRankingRow{}, 0, m.err
}
func (m *mockStore) ProviderVisibilityHistory(_ context.Context, _, _ string) ([]storage.ProviderVisibilityRow, error) {
	return []storage.ProviderVisibilityRow{}, m.err
}
func (m *mockStore) DeleteProviderData(_ context.Context, projectID, provider string) error {
	m.deleteProviderCalls = append(m.deleteProviderCalls, deleteProviderCall{projectID, provider})
	return m.err
}

// ---------------------------------------------------------------------------
// mockManager implements CrawlService
// ---------------------------------------------------------------------------

type mockManager struct {
	running      map[string]bool
	startResult  string
	startErr     error
	stopErr      error
	resumeErr    error
	retryCount   int
	retryErr     error
	progress     map[string][2]int64 // sessionID -> [pages, queue]
	resumeCalls  []resumeCall
	startCalls   []crawler.CrawlRequest
	stopCalls    []string
}

type resumeCall struct {
	SessionID string
	Overrides *crawler.CrawlRequest
}

func newMockManager() *mockManager {
	return &mockManager{
		running:  make(map[string]bool),
		progress: make(map[string][2]int64),
	}
}

func (m *mockManager) IsRunning(sessionID string) bool {
	return m.running[sessionID]
}

func (m *mockManager) Progress(sessionID string) (int64, int, bool) {
	if p, ok := m.progress[sessionID]; ok {
		return p[0], int(p[1]), m.running[sessionID]
	}
	return 0, 0, m.running[sessionID]
}

func (m *mockManager) StartCrawl(req crawler.CrawlRequest) (string, error) {
	m.startCalls = append(m.startCalls, req)
	if m.startErr != nil {
		return "", m.startErr
	}
	return m.startResult, nil
}

func (m *mockManager) StopCrawl(sessionID string) error {
	m.stopCalls = append(m.stopCalls, sessionID)
	return m.stopErr
}

func (m *mockManager) ResumeCrawl(sessionID string, overrides *crawler.CrawlRequest) (string, error) {
	m.resumeCalls = append(m.resumeCalls, resumeCall{SessionID: sessionID, Overrides: overrides})
	return sessionID, m.resumeErr
}

func (m *mockManager) RetryFailed(sessionID string, overrides *crawler.CrawlRequest) (int, error) {
	return m.retryCount, m.retryErr
}

func (m *mockManager) BufferState(sessionID string) storage.BufferErrorState {
	return storage.BufferErrorState{}
}

func (m *mockManager) LastError(sessionID string) string {
	return ""
}

func (m *mockManager) Shutdown(timeout time.Duration) {}

func (m *mockManager) RecoverOrphanedSessions(ctx context.Context) {}

// ---------------------------------------------------------------------------
// newTestServer helper
// ---------------------------------------------------------------------------

func newTestServer(t *testing.T) (*Server, http.Handler, *apikeys.Store) {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Username: "admin",
			Password: "secret",
		},
		Theme: config.ThemeConfig{
			AppName:     "Test",
			AccentColor: "#000000",
			Mode:        "light",
		},
	}

	keyStore, err := apikeys.NewStore(":memory:")
	if err != nil {
		t.Fatalf("creating key store: %v", err)
	}

	ms := &mockStore{}
	mm := newMockManager()

	srv := NewWithDeps(cfg, ms, keyStore, mm)
	handler, err := srv.buildHandler()
	if err != nil {
		t.Fatalf("building handler: %v", err)
	}

	return srv, handler, keyStore
}

// authRequest adds basic auth credentials to a request.
func authRequest(req *http.Request) *http.Request {
	req.SetBasicAuth("admin", "secret")
	return req
}

// jsonBody encodes v as JSON and returns a *bytes.Reader.
func jsonBody(t *testing.T, v interface{}) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling json: %v", err)
	}
	return bytes.NewReader(b)
}

// decodeJSON decodes the response body into v.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decoding json: %v", err)
	}
}

// =========================================================================
// 1. Auth middleware tests
// =========================================================================

func TestAuth_NoCredentials(t *testing.T) {
	_, handler, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_ValidBasicAuth(t *testing.T) {
	_, handler, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuth_WrongBasicAuth(t *testing.T) {
	_, handler, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.SetBasicAuth("admin", "wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_ValidGeneralAPIKey(t *testing.T) {
	_, handler, ks := newTestServer(t)

	result, err := ks.CreateAPIKey("test-key", "general", nil)
	if err != nil {
		t.Fatalf("creating API key: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuth_InvalidAPIKey(t *testing.T) {
	_, handler, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", "sk_boguskey1234567890")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_SecurityHeaders(t *testing.T) {
	_, handler, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headers := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Referrer-Policy",
		"Content-Security-Policy",
	}
	for _, h := range headers {
		if rec.Header().Get(h) == "" {
			t.Errorf("expected security header %s to be set", h)
		}
	}
}

func TestAuth_HealthNoAuth(t *testing.T) {
	// Health endpoint is behind the same auth middleware in the current code,
	// but we test that it responds correctly when authorized.
	_, handler, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

// =========================================================================
// 2. Authorization tests
// =========================================================================

func TestAuthz_ProjectKeyBlockedOnPost(t *testing.T) {
	_, handler, ks := newTestServer(t)

	proj, err := ks.CreateProject("test-proj")
	if err != nil {
		t.Fatalf("creating project: %v", err)
	}
	result, err := ks.CreateAPIKey("proj-key", "project", &proj.ID)
	if err != nil {
		t.Fatalf("creating API key: %v", err)
	}

	body := jsonBody(t, map[string]interface{}{"seeds": []string{"https://example.com"}})
	req := httptest.NewRequest("POST", "/api/crawl", body)
	req.Header.Set("X-API-Key", result.FullKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestAuthz_ProjectKeyBlockedOnDelete(t *testing.T) {
	_, handler, ks := newTestServer(t)

	proj, err := ks.CreateProject("test-proj")
	if err != nil {
		t.Fatalf("creating project: %v", err)
	}
	result, err := ks.CreateAPIKey("proj-key", "project", &proj.ID)
	if err != nil {
		t.Fatalf("creating API key: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/sessions/sess-123", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestAuthz_ProjectKeyCanReadOwnSessions(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("my-proj")
	result, _ := ks.CreateAPIKey("proj-key", "project", &proj.ID)

	ms := srv.store.(*mockStore)
	ms.sessions = []storage.CrawlSession{
		{ID: "sess-1", ProjectID: &proj.ID, Status: "completed"},
		{ID: "sess-2", Status: "completed"}, // no project
	}

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var sessions []map[string]interface{}
	decodeJSON(t, rec, &sessions)

	// Should only see sess-1 (filtered by project)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0]["ID"] != "sess-1" {
		t.Errorf("expected sess-1, got %v", sessions[0]["ID"])
	}
}

func TestAuthz_ProjectKeyCannotAccessOtherSession(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("my-proj")
	result, _ := ks.CreateAPIKey("proj-key", "project", &proj.ID)

	otherProj := "other-proj-id"
	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-other": {ID: "sess-other", ProjectID: &otherProj, Status: "completed"},
	}

	req := httptest.NewRequest("GET", "/api/sessions/sess-other/pages", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestAuthz_GeneralKeyFullAccess(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	result, _ := ks.CreateAPIKey("general-key", "general", nil)

	mm := srv.manager.(*mockManager)
	mm.startResult = "new-session-id"

	body := jsonBody(t, map[string]interface{}{"seeds": []string{"https://example.com"}})
	req := httptest.NewRequest("POST", "/api/crawl", body)
	req.Header.Set("X-API-Key", result.FullKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthz_BasicAuthFullWriteAccess(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)
	mm.startResult = "new-session-id"

	body := jsonBody(t, map[string]interface{}{"seeds": []string{"https://example.com"}})
	req := httptest.NewRequest("POST", "/api/crawl", body)
	req.SetBasicAuth("admin", "secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// =========================================================================
// 3. CRUD tests
// =========================================================================

func TestCRUD_GetSessions(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.sessions = []storage.CrawlSession{
		{ID: "s1", Status: "completed", SeedURLs: []string{"https://a.com"}, StartedAt: time.Now()},
		{ID: "s2", Status: "running", SeedURLs: []string{"https://b.com"}, StartedAt: time.Now()},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var sessions []map[string]interface{}
	decodeJSON(t, rec, &sessions)
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestCRUD_DeleteRunningSession(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)
	mm.running["sess-running"] = true

	req := authRequest(httptest.NewRequest("DELETE", "/api/sessions/sess-running", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestCRUD_DeleteNonRunningSession(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("DELETE", "/api/sessions/sess-done", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["status"] != "deleted" {
		t.Errorf("expected status deleted, got %q", body["status"])
	}
}

func TestCRUD_StartCrawl(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)
	mm.startResult = "new-sess-123"

	body := jsonBody(t, map[string]interface{}{
		"seeds":     []string{"https://example.com"},
		"max_pages": 100,
		"workers":   5,
	})
	req := authRequest(httptest.NewRequest("POST", "/api/crawl", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["session_id"] != "new-sess-123" {
		t.Errorf("expected session_id new-sess-123, got %q", resp["session_id"])
	}
}

func TestCRUD_StopCrawl(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("POST", "/api/sessions/sess-1/stop", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["status"] != "stopped" {
		t.Errorf("expected status stopped, got %q", resp["status"])
	}
}

func TestCRUD_Progress(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "running"},
	}

	mm := srv.manager.(*mockManager)
	mm.running["sess-1"] = true
	mm.progress["sess-1"] = [2]int64{42, 10}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/progress", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	decodeJSON(t, rec, &resp)
	if resp["pages_crawled"] != float64(42) {
		t.Errorf("expected pages_crawled 42, got %v", resp["pages_crawled"])
	}
	if resp["is_running"] != true {
		t.Errorf("expected is_running true, got %v", resp["is_running"])
	}
}

func TestCRUD_GetTheme(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("GET", "/api/theme", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	decodeJSON(t, rec, &resp)
	if resp["app_name"] != "Test" {
		t.Errorf("expected app_name Test, got %v", resp["app_name"])
	}
}

// =========================================================================
// 4. Validation tests
// =========================================================================

func TestValidation_InvalidJSON(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("POST", "/api/crawl", bytes.NewReader([]byte("{bad json"))))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestValidation_EmptySeeds(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)
	mm.startErr = fmt.Errorf("at least one seed URL is required")

	body := jsonBody(t, map[string]interface{}{"seeds": []string{}})
	req := authRequest(httptest.NewRequest("POST", "/api/crawl", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestValidation_PagesDefaultLimit(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}
	ms.pages = []storage.PageRow{
		{CrawlSessionID: "sess-1", URL: "https://example.com/page1"},
	}

	// No limit param -- should use default (100) and succeed
	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/pages", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestValidation_QueryIntNegative(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	// Negative limit should fall back to default
	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/pages?limit=-5", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (using default limit), got %d", rec.Code)
	}
}

func TestValidation_PageHTMLMissingURL(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/page-html", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestValidation_RobotsContentMissingHost(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/robots-content", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// =========================================================================
// 5. Projects & API keys lifecycle tests
// =========================================================================

func TestProjects_Lifecycle(t *testing.T) {
	_, handler, _ := newTestServer(t)

	// Create project
	body := jsonBody(t, map[string]interface{}{"name": "My Project"})
	req := authRequest(httptest.NewRequest("POST", "/api/projects", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var created apikeys.Project
	decodeJSON(t, rec, &created)
	if created.Name != "My Project" {
		t.Errorf("expected name 'My Project', got %q", created.Name)
	}
	projectID := created.ID

	// List projects
	req = authRequest(httptest.NewRequest("GET", "/api/projects", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list projects: expected 200, got %d", rec.Code)
	}

	var projects []apikeys.Project
	decodeJSON(t, rec, &projects)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	// Rename project
	body = jsonBody(t, map[string]interface{}{"name": "Renamed Project"})
	req = authRequest(httptest.NewRequest("PUT", "/api/projects/"+projectID, body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("rename project: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Delete project
	req = authRequest(httptest.NewRequest("DELETE", "/api/projects/"+projectID, nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete project: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify deletion: list should return 0
	req = authRequest(httptest.NewRequest("GET", "/api/projects", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var afterDelete []apikeys.Project
	decodeJSON(t, rec, &afterDelete)
	if len(afterDelete) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(afterDelete))
	}
}

func TestAPIKeys_Lifecycle(t *testing.T) {
	_, handler, _ := newTestServer(t)

	// Create general API key
	body := jsonBody(t, map[string]interface{}{
		"name": "My Key",
		"type": "general",
	})
	req := authRequest(httptest.NewRequest("POST", "/api/api-keys", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createdKey map[string]interface{}
	decodeJSON(t, rec, &createdKey)
	keyID, _ := createdKey["id"].(string)
	if keyID == "" {
		t.Fatal("expected non-empty key ID")
	}

	// List keys
	req = authRequest(httptest.NewRequest("GET", "/api/api-keys", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list keys: expected 200, got %d", rec.Code)
	}

	var keys []map[string]interface{}
	decodeJSON(t, rec, &keys)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	// Delete key
	req = authRequest(httptest.NewRequest("DELETE", "/api/api-keys/"+keyID, nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete key: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted
	req = authRequest(httptest.NewRequest("GET", "/api/api-keys", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var afterDelete []map[string]interface{}
	decodeJSON(t, rec, &afterDelete)
	if len(afterDelete) != 0 {
		t.Errorf("expected 0 keys after delete, got %d", len(afterDelete))
	}
}

func TestAPIKeys_ProjectKeyReadOnly(t *testing.T) {
	_, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("read-only-proj")
	result, _ := ks.CreateAPIKey("rk", "project", &proj.ID)

	// Project key should be able to read sessions (GET)
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("project key read sessions: expected 200, got %d", rec.Code)
	}

	// But blocked from write endpoints like POST /api/crawl
	body := jsonBody(t, map[string]interface{}{"seeds": []string{"https://example.com"}})
	req = httptest.NewRequest("POST", "/api/crawl", body)
	req.Header.Set("X-API-Key", result.FullKey)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("project key write: expected 403, got %d", rec.Code)
	}

	// Blocked from listing API keys (requireFullAccess)
	req = httptest.NewRequest("GET", "/api/api-keys", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("project key list api-keys: expected 403, got %d", rec.Code)
	}
}

func TestProjects_AssociateDisassociateSession(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("assoc-proj")

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	// Associate
	req := authRequest(httptest.NewRequest("POST", "/api/projects/"+proj.ID+"/sessions/sess-1", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("associate: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if len(ms.updateProjectCalls) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(ms.updateProjectCalls))
	}
	if ms.updateProjectCalls[0].SessionID != "sess-1" {
		t.Errorf("expected session sess-1, got %s", ms.updateProjectCalls[0].SessionID)
	}
	if ms.updateProjectCalls[0].ProjectID == nil || *ms.updateProjectCalls[0].ProjectID != proj.ID {
		t.Errorf("expected project %s, got %v", proj.ID, ms.updateProjectCalls[0].ProjectID)
	}

	// Disassociate
	req = authRequest(httptest.NewRequest("DELETE", "/api/projects/"+proj.ID+"/sessions/sess-1", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("disassociate: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if len(ms.updateProjectCalls) != 2 {
		t.Fatalf("expected 2 update calls, got %d", len(ms.updateProjectCalls))
	}
	if ms.updateProjectCalls[1].ProjectID != nil {
		t.Errorf("expected nil project on disassociate, got %v", ms.updateProjectCalls[1].ProjectID)
	}
}

// =========================================================================
// 6. Compare tests
// =========================================================================

func TestCompareStats_MissingParams(t *testing.T) {
	_, handler, _ := newTestServer(t)

	// Missing both
	req := authRequest(httptest.NewRequest("GET", "/api/compare/stats", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing params, got %d", rec.Code)
	}

	// Missing b
	req = authRequest(httptest.NewRequest("GET", "/api/compare/stats?a=sess-1", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing b, got %d", rec.Code)
	}
}

func TestCompareStats_Success(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.compareStatsResult = &storage.CompareStatsResult{
		SessionA: "sess-a",
		SessionB: "sess-b",
		StatsA: &storage.SessionStats{
			TotalPages:        100,
			StatusCodes:       map[uint16]uint64{200: 100},
			DepthDistribution: map[uint16]uint64{0: 10, 1: 90},
		},
		StatsB: &storage.SessionStats{
			TotalPages:        120,
			StatusCodes:       map[uint16]uint64{200: 115, 404: 5},
			DepthDistribution: map[uint16]uint64{0: 10, 1: 110},
		},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/compare/stats?a=sess-a&b=sess-b", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var result storage.CompareStatsResult
	decodeJSON(t, rec, &result)
	if result.StatsA.TotalPages != 100 {
		t.Errorf("expected stats_a total_pages 100, got %d", result.StatsA.TotalPages)
	}
	if result.StatsB.TotalPages != 120 {
		t.Errorf("expected stats_b total_pages 120, got %d", result.StatsB.TotalPages)
	}
}

func TestComparePages_InvalidType(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("GET", "/api/compare/pages?a=sess-a&b=sess-b&type=invalid", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid type, got %d", rec.Code)
	}
}

func TestComparePages_Success(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.comparePagesResult = &storage.PageDiffResult{
		Pages: []storage.PageDiffRow{
			{URL: "https://example.com/new", DiffType: "added", StatusCodeB: 200, TitleB: "New Page"},
		},
		TotalAdded:   1,
		TotalRemoved: 0,
		TotalChanged: 0,
	}

	req := authRequest(httptest.NewRequest("GET", "/api/compare/pages?a=sess-a&b=sess-b&type=added", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var result storage.PageDiffResult
	decodeJSON(t, rec, &result)
	if len(result.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(result.Pages))
	}
	if result.Pages[0].URL != "https://example.com/new" {
		t.Errorf("expected URL https://example.com/new, got %s", result.Pages[0].URL)
	}
}

func TestCompareLinks_Success(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.compareLinksResult = &storage.LinkDiffResult{
		Links: []storage.LinkDiffRow{
			{SourceURL: "https://example.com/a", TargetURL: "https://example.com/b", AnchorText: "link", DiffType: "added"},
		},
		TotalAdded:   1,
		TotalRemoved: 0,
	}

	req := authRequest(httptest.NewRequest("GET", "/api/compare/links?a=sess-a&b=sess-b&type=added", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var result storage.LinkDiffResult
	decodeJSON(t, rec, &result)
	if len(result.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(result.Links))
	}
}

func TestCompareLinks_InvalidType(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("GET", "/api/compare/links?a=sess-a&b=sess-b&type=changed", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid link type, got %d", rec.Code)
	}
}

func TestCompare_CrossProjectForbidden(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("my-proj")
	result, _ := ks.CreateAPIKey("proj-key", "project", &proj.ID)

	otherProj := "other-proj-id"
	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-mine":  {ID: "sess-mine", ProjectID: &proj.ID, Status: "completed"},
		"sess-other": {ID: "sess-other", ProjectID: &otherProj, Status: "completed"},
	}

	req := httptest.NewRequest("GET", "/api/compare/stats?a=sess-mine&b=sess-other", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for cross-project compare, got %d", rec.Code)
	}
}

func TestAudit_Success(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.sessions = []storage.CrawlSession{{ID: "sess-1", Status: "completed"}}
	ms.auditResult = &storage.AuditResult{
		Content: &storage.AuditContent{
			Total:        100,
			HTMLPages:    95,
			TitleMissing: 5,
			TitleTooLong: 10,
		},
		Technical: &storage.AuditTechnical{
			Indexable:    80,
			NonIndexable: 20,
		},
		Links: &storage.AuditLinks{
			TotalInternal: 500,
			TotalExternal: 50,
		},
		Structure: &storage.AuditStructure{
			OrphanPages: 3,
		},
		Sitemaps:      &storage.AuditSitemaps{InBoth: 80, CrawledOnly: 15, SitemapOnly: 5},
		International: &storage.AuditInternational{PagesWithLang: 90},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/audit", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result storage.AuditResult
	decodeJSON(t, rec, &result)
	if result.Content.Total != 100 {
		t.Errorf("expected total 100, got %d", result.Content.Total)
	}
	if result.Content.TitleMissing != 5 {
		t.Errorf("expected title_missing 5, got %d", result.Content.TitleMissing)
	}
	if result.Technical.Indexable != 80 {
		t.Errorf("expected indexable 80, got %d", result.Technical.Indexable)
	}
	if result.Links.TotalInternal != 500 {
		t.Errorf("expected total_internal 500, got %d", result.Links.TotalInternal)
	}
}

// =========================================================================
// Custom Tests tests
// =========================================================================

func TestListRulesets_Success(t *testing.T) {
	_, handler, ks := newTestServer(t)

	_, err := ks.CreateRuleset("My Ruleset", []customtests.TestRule{
		{Type: customtests.StringContains, Name: "Has GTM", Value: "GTM-XXXX"},
	})
	if err != nil {
		t.Fatalf("creating ruleset: %v", err)
	}

	req := authRequest(httptest.NewRequest("GET", "/api/rulesets", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var rulesets []customtests.Ruleset
	decodeJSON(t, rec, &rulesets)
	if len(rulesets) != 1 {
		t.Fatalf("expected 1 ruleset, got %d", len(rulesets))
	}
	if rulesets[0].Name != "My Ruleset" {
		t.Errorf("expected name 'My Ruleset', got %q", rulesets[0].Name)
	}
}

func TestCreateRuleset_Success(t *testing.T) {
	_, handler, _ := newTestServer(t)

	body := jsonBody(t, map[string]interface{}{
		"name": "SEO Checks",
		"rules": []map[string]interface{}{
			{"type": "string_contains", "name": "Has GTM", "value": "GTM-XXXX"},
			{"type": "header_exists", "name": "Has X-Frame", "value": "X-Frame-Options"},
		},
	})
	req := authRequest(httptest.NewRequest("POST", "/api/rulesets", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var ruleset customtests.Ruleset
	decodeJSON(t, rec, &ruleset)
	if ruleset.Name != "SEO Checks" {
		t.Errorf("expected name 'SEO Checks', got %q", ruleset.Name)
	}
	if len(ruleset.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(ruleset.Rules))
	}
}

func TestRunTests_Success(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.sessions = []storage.CrawlSession{{ID: "sess-1"}}

	ruleset, err := ks.CreateRuleset("Test", []customtests.TestRule{
		{Type: customtests.StringContains, Name: "Has GTM", Value: "GTM-XXXX"},
	})
	if err != nil {
		t.Fatalf("creating ruleset: %v", err)
	}

	body := jsonBody(t, map[string]string{"ruleset_id": ruleset.ID})
	req := authRequest(httptest.NewRequest("POST", "/api/sessions/sess-1/run-tests", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result customtests.TestRunResult
	decodeJSON(t, rec, &result)
	if result.RulesetName != "Test" {
		t.Errorf("expected ruleset name 'Test', got %q", result.RulesetName)
	}
	if len(result.Pages) == 0 {
		t.Error("expected at least one page in results")
	}
}

func TestRunTests_MissingRuleset(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.sessions = []storage.CrawlSession{{ID: "sess-1"}}

	body := jsonBody(t, map[string]string{"ruleset_id": ""})
	req := authRequest(httptest.NewRequest("POST", "/api/sessions/sess-1/run-tests", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// =========================================================================
// 8. Integration: API key workflow (multi-step)
// =========================================================================

func TestIntegration_GeneralAPIKeyWorkflow(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)
	mm.startResult = "crawl-sess-1"

	// Step 1: Create project via basic auth
	body := jsonBody(t, map[string]interface{}{"name": "Integration Proj"})
	req := authRequest(httptest.NewRequest("POST", "/api/projects", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d", rec.Code)
	}

	// Step 2: Create a general API key
	body = jsonBody(t, map[string]interface{}{"name": "general-key", "type": "general"})
	req = authRequest(httptest.NewRequest("POST", "/api/api-keys", body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: expected 201, got %d", rec.Code)
	}
	var keyResp map[string]interface{}
	decodeJSON(t, rec, &keyResp)
	fullKey := keyResp["key"].(string)

	// Step 3: Use that key to start a crawl
	body = jsonBody(t, map[string]interface{}{"seeds": []string{"https://example.com"}})
	req = httptest.NewRequest("POST", "/api/crawl", body)
	req.Header.Set("X-API-Key", fullKey)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start crawl with general key: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var crawlResp map[string]string
	decodeJSON(t, rec, &crawlResp)
	if crawlResp["session_id"] != "crawl-sess-1" {
		t.Errorf("expected session_id crawl-sess-1, got %q", crawlResp["session_id"])
	}
}

func TestIntegration_ProjectScopedAPIKeyAccess(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	// Create two projects
	proj1, _ := ks.CreateProject("proj-alpha")
	proj2, _ := ks.CreateProject("proj-beta")

	// Create project-scoped key for proj1
	result, _ := ks.CreateAPIKey("proj1-key", "project", &proj1.ID)

	ms := srv.store.(*mockStore)
	ms.sessions = []storage.CrawlSession{
		{ID: "sess-alpha", ProjectID: &proj1.ID, Status: "completed"},
		{ID: "sess-beta", ProjectID: &proj2.ID, Status: "completed"},
	}
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-alpha": {ID: "sess-alpha", ProjectID: &proj1.ID, Status: "completed"},
		"sess-beta":  {ID: "sess-beta", ProjectID: &proj2.ID, Status: "completed"},
	}

	// Can list sessions — only sees proj1 sessions
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions: expected 200, got %d", rec.Code)
	}
	var sessions []map[string]interface{}
	decodeJSON(t, rec, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Can read pages for own session
	req = httptest.NewRequest("GET", "/api/sessions/sess-alpha/pages", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("read own session pages: expected 200, got %d", rec.Code)
	}

	// Cannot read pages for other project's session
	req = httptest.NewRequest("GET", "/api/sessions/sess-beta/pages", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("read other session pages: expected 403, got %d", rec.Code)
	}

	// Cannot start a crawl (write operation)
	body := jsonBody(t, map[string]interface{}{"seeds": []string{"https://example.com"}})
	req = httptest.NewRequest("POST", "/api/crawl", body)
	req.Header.Set("X-API-Key", result.FullKey)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("start crawl with project key: expected 403, got %d", rec.Code)
	}

	// Cannot delete a session
	req = httptest.NewRequest("DELETE", "/api/sessions/sess-alpha", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("delete with project key: expected 403, got %d", rec.Code)
	}
}

// =========================================================================
// 9. Integration: Token expired / deleted
// =========================================================================

func TestIntegration_DeletedAPIKeyReturns401(t *testing.T) {
	_, handler, ks := newTestServer(t)

	// Create and then delete an API key
	result, err := ks.CreateAPIKey("temp-key", "general", nil)
	if err != nil {
		t.Fatalf("creating key: %v", err)
	}

	// Key works before deletion
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("before delete: expected 200, got %d", rec.Code)
	}

	// Delete the key
	err = ks.DeleteAPIKey(result.ID)
	if err != nil {
		t.Fatalf("deleting key: %v", err)
	}

	// Key should no longer work
	req = httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("after delete: expected 401, got %d", rec.Code)
	}
}

// =========================================================================
// 10. Integration: Session lifecycle
// =========================================================================

func TestIntegration_SessionLifecycle(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)
	mm.startResult = "lifecycle-sess"

	// Start crawl → 201
	body := jsonBody(t, map[string]interface{}{"seeds": []string{"https://example.com"}, "max_pages": 50})
	req := authRequest(httptest.NewRequest("POST", "/api/crawl", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start: expected 201, got %d", rec.Code)
	}

	// Simulate running
	mm.running["lifecycle-sess"] = true
	mm.progress["lifecycle-sess"] = [2]int64{25, 5}

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"lifecycle-sess": {ID: "lifecycle-sess", Status: "running"},
	}

	// Progress → 200
	req = authRequest(httptest.NewRequest("GET", "/api/sessions/lifecycle-sess/progress", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("progress: expected 200, got %d", rec.Code)
	}
	var prog map[string]interface{}
	decodeJSON(t, rec, &prog)
	if prog["pages_crawled"] != float64(25) {
		t.Errorf("expected 25 pages, got %v", prog["pages_crawled"])
	}
	if prog["is_running"] != true {
		t.Errorf("expected is_running true")
	}

	// Stop → 200
	req = authRequest(httptest.NewRequest("POST", "/api/sessions/lifecycle-sess/stop", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: expected 200, got %d", rec.Code)
	}

	mm.running["lifecycle-sess"] = false

	// Delete → 200
	req = authRequest(httptest.NewRequest("DELETE", "/api/sessions/lifecycle-sess", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", rec.Code)
	}
}

func TestIntegration_CannotDeleteRunningThenStopAndDelete(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)
	mm.running["sess-active"] = true

	// Attempt delete running → 409
	req := authRequest(httptest.NewRequest("DELETE", "/api/sessions/sess-active", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete running: expected 409, got %d", rec.Code)
	}

	// Stop it
	req = authRequest(httptest.NewRequest("POST", "/api/sessions/sess-active/stop", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: expected 200, got %d", rec.Code)
	}
	mm.running["sess-active"] = false

	// Now delete → 200
	req = authRequest(httptest.NewRequest("DELETE", "/api/sessions/sess-active", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete after stop: expected 200, got %d", rec.Code)
	}
}

// =========================================================================
// 11. Integration: Pagination parameters
// =========================================================================

func TestIntegration_PaginationPassedToStore(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/pages?limit=5&offset=10", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if len(ms.listPagesCalls) != 1 {
		t.Fatalf("expected 1 ListPages call, got %d", len(ms.listPagesCalls))
	}
	call := ms.listPagesCalls[0]
	if call.Limit != 5 {
		t.Errorf("expected limit 5, got %d", call.Limit)
	}
	if call.Offset != 10 {
		t.Errorf("expected offset 10, got %d", call.Offset)
	}
}

func TestIntegration_InvalidFiltersIgnored(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	// "bogus_filter" is not in PageFilters whitelist — should be silently ignored
	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/pages?limit=10&bogus_filter=bad", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if len(ms.listPagesCalls) != 1 {
		t.Fatalf("expected 1 ListPages call, got %d", len(ms.listPagesCalls))
	}
	// Unknown filters should not appear in the parsed filters
	for _, f := range ms.listPagesCalls[0].Filters {
		if f.Value == "bad" {
			t.Errorf("bogus filter should not have been passed to store")
		}
	}
}

// =========================================================================
// 12. Integration: Storage error propagation
// =========================================================================

func TestIntegration_StorageErrorSessions(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.err = fmt.Errorf("clickhouse connection failed")

	req := authRequest(httptest.NewRequest("GET", "/api/sessions", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	var body map[string]string
	decodeJSON(t, rec, &body)
	// Should return generic message, not the actual error
	if body["error"] != "internal server error" {
		t.Errorf("expected generic error message, got %q", body["error"])
	}
}

func TestIntegration_StorageErrorStats(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}
	ms.err = fmt.Errorf("disk full")

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/stats", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["error"] != "internal server error" {
		t.Errorf("expected generic error, got %q", body["error"])
	}
}

func TestIntegration_StorageErrorPages(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}
	ms.err = fmt.Errorf("timeout")

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/pages", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Pages handler returns 500 for store errors (sanitized)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// =========================================================================
// 13. Logs endpoints
// =========================================================================

func TestLogs_List(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("GET", "/api/logs?limit=50&level=error", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	decodeJSON(t, rec, &resp)
	if resp["total"] != float64(0) {
		t.Errorf("expected total 0, got %v", resp["total"])
	}
	if resp["logs"] == nil {
		t.Error("expected logs field to be present")
	}
}

func TestLogs_Export(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("GET", "/api/logs/export", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/x-ndjson" {
		t.Errorf("expected Content-Type application/x-ndjson, got %q", ct)
	}

	cd := rec.Header().Get("Content-Disposition")
	if cd != "attachment; filename=application_logs.jsonl" {
		t.Errorf("expected Content-Disposition attachment, got %q", cd)
	}
}

// =========================================================================
// 14. Backup handlers (no ClickHouse configured)
// =========================================================================

func TestBackups_ListWithoutConfig(t *testing.T) {
	_, handler, _ := newTestServer(t)

	// BackupOpts is nil by default in newTestServer
	req := authRequest(httptest.NewRequest("GET", "/api/backups", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var backups []interface{}
	decodeJSON(t, rec, &backups)
	if len(backups) != 0 {
		t.Errorf("expected empty list, got %d items", len(backups))
	}
}

func TestBackups_CreateWithoutConfig(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("POST", "/api/backups", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["error"] != "backup not configured" {
		t.Errorf("expected 'backup not configured', got %q", body["error"])
	}
}

// =========================================================================
// 15. Provider status/disconnect
// =========================================================================

func TestProvider_StatusNotConnected(t *testing.T) {
	_, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("prov-proj")

	req := authRequest(httptest.NewRequest("GET", "/api/projects/"+proj.ID+"/providers/seobserver/status", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	decodeJSON(t, rec, &resp)
	if resp["connected"] != false {
		t.Errorf("expected connected=false, got %v", resp["connected"])
	}
}

func TestProvider_Disconnect(t *testing.T) {
	srv, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("disc-proj")

	req := authRequest(httptest.NewRequest("DELETE", "/api/projects/"+proj.ID+"/providers/seobserver/disconnect", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["status"] != "disconnected" {
		t.Errorf("expected status disconnected, got %q", resp["status"])
	}

	// Verify DeleteProviderData was called on the store
	ms := srv.store.(*mockStore)
	if len(ms.deleteProviderCalls) != 1 {
		t.Fatalf("expected 1 DeleteProviderData call, got %d", len(ms.deleteProviderCalls))
	}
	if ms.deleteProviderCalls[0].ProjectID != proj.ID {
		t.Errorf("expected project %s, got %s", proj.ID, ms.deleteProviderCalls[0].ProjectID)
	}
	if ms.deleteProviderCalls[0].Provider != "seobserver" {
		t.Errorf("expected provider seobserver, got %s", ms.deleteProviderCalls[0].Provider)
	}
}

// =========================================================================
// 16. Resume crawl with overrides
// =========================================================================

func TestResume_WithOverrides(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)

	body := jsonBody(t, map[string]interface{}{
		"max_pages": 200,
		"workers":   10,
	})
	req := authRequest(httptest.NewRequest("POST", "/api/sessions/old-sess/resume", body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["status"] != "resumed" {
		t.Errorf("expected status resumed, got %q", resp["status"])
	}

	// Verify overrides were passed
	if len(mm.resumeCalls) != 1 {
		t.Fatalf("expected 1 resume call, got %d", len(mm.resumeCalls))
	}
	rc := mm.resumeCalls[0]
	if rc.SessionID != "old-sess" {
		t.Errorf("expected session old-sess, got %s", rc.SessionID)
	}
	if rc.Overrides == nil {
		t.Fatal("expected overrides to be non-nil")
	}
	if rc.Overrides.MaxPages != 200 {
		t.Errorf("expected max_pages 200, got %d", rc.Overrides.MaxPages)
	}
	if rc.Overrides.Workers != 10 {
		t.Errorf("expected workers 10, got %d", rc.Overrides.Workers)
	}
}

func TestResume_WithoutBody(t *testing.T) {
	srv, handler, _ := newTestServer(t)

	mm := srv.manager.(*mockManager)

	req := authRequest(httptest.NewRequest("POST", "/api/sessions/old-sess/resume", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if len(mm.resumeCalls) != 1 {
		t.Fatalf("expected 1 resume call, got %d", len(mm.resumeCalls))
	}
	if mm.resumeCalls[0].Overrides != nil {
		t.Errorf("expected nil overrides for empty body, got %+v", mm.resumeCalls[0].Overrides)
	}
}

func TestResume_ProjectKeyBlocked(t *testing.T) {
	_, handler, ks := newTestServer(t)

	proj, _ := ks.CreateProject("res-proj")
	result, _ := ks.CreateAPIKey("rk", "project", &proj.ID)

	req := httptest.NewRequest("POST", "/api/sessions/some-sess/resume", nil)
	req.Header.Set("X-API-Key", result.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for project key resume, got %d", rec.Code)
	}
}

// =========================================================================
// Expired Domains endpoint tests
// =========================================================================

func TestExpiredDomains_EmptyResult(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/external-checks/expired-domains", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var result storage.ExpiredDomainsResult
	decodeJSON(t, rec, &result)
	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
	if len(result.Domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(result.Domains))
	}
}

func TestExpiredDomains_WithResults(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}
	ms.expiredDomainsResult = &storage.ExpiredDomainsResult{
		Total: 2,
		Domains: []storage.ExpiredDomain{
			{
				RegistrableDomain: "expired.com",
				DeadURLsChecked:   3,
				Sources: []storage.ExpiredDomainSource{
					{SourceURL: "https://site-a.com/page1", TargetURL: "https://www.expired.com/res"},
					{SourceURL: "https://site-b.com/links", TargetURL: "https://expired.com/page"},
				},
			},
			{
				RegistrableDomain: "gone-domain.org",
				DeadURLsChecked:   1,
				Sources: []storage.ExpiredDomainSource{
					{SourceURL: "https://site-a.com/page2", TargetURL: "https://gone-domain.org/x"},
				},
			},
		},
	}

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/external-checks/expired-domains?limit=50&offset=0", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var result storage.ExpiredDomainsResult
	decodeJSON(t, rec, &result)
	if result.Total != 2 {
		t.Errorf("expected total 2, got %d", result.Total)
	}
	if len(result.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(result.Domains))
	}
	if result.Domains[0].RegistrableDomain != "expired.com" {
		t.Errorf("expected first domain expired.com, got %s", result.Domains[0].RegistrableDomain)
	}
	if result.Domains[0].DeadURLsChecked != 3 {
		t.Errorf("expected 3 dead URLs, got %d", result.Domains[0].DeadURLsChecked)
	}
	if len(result.Domains[0].Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(result.Domains[0].Sources))
	}
}

func TestExpiredDomains_StorageError(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}
	ms.err = fmt.Errorf("clickhouse timeout")

	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/external-checks/expired-domains", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestExpiredDomains_NoAuth(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions/sess-1/external-checks/expired-domains", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestExpiredDomains_DefaultPagination(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.getSessionByID = map[string]*storage.CrawlSession{
		"sess-1": {ID: "sess-1", Status: "completed"},
	}

	// No limit/offset params — should use defaults (100, 0)
	req := authRequest(httptest.NewRequest("GET", "/api/sessions/sess-1/external-checks/expired-domains", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// Verify the mockStore and mockManager satisfy their respective interfaces
// at compile time.
var _ StorageService = (*mockStore)(nil)
var _ CrawlService = (*mockManager)(nil)
