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

	"github.com/SEObserver/seocrawler/internal/apikeys"
	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/crawler"
	"github.com/SEObserver/seocrawler/internal/customtests"
	"github.com/SEObserver/seocrawler/internal/storage"
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
	err                  error
	deleteCalls          []string
	updateProjectCalls   []updateProjectCall
	getSessionByID       map[string]*storage.CrawlSession
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

func (m *mockStore) ListPages(_ context.Context, _ string, _, _ int, _ []storage.ParsedFilter) ([]storage.PageRow, error) {
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

func (m *mockStore) GetPageLinks(_ context.Context, _, _ string, _, _ int) (*storage.PageLinksResult, error) {
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

// ---------------------------------------------------------------------------
// mockManager implements CrawlService
// ---------------------------------------------------------------------------

type mockManager struct {
	running     map[string]bool
	startResult string
	startErr    error
	stopErr     error
	resumeErr   error
	retryCount  int
	retryErr    error
	progress    map[string][2]int64 // sessionID -> [pages, queue]
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
	if m.startErr != nil {
		return "", m.startErr
	}
	return m.startResult, nil
}

func (m *mockManager) StopCrawl(sessionID string) error {
	return m.stopErr
}

func (m *mockManager) ResumeCrawl(sessionID string, overrides *crawler.CrawlRequest) (string, error) {
	return sessionID, m.resumeErr
}

func (m *mockManager) RetryFailed(sessionID string, overrides *crawler.CrawlRequest) (int, error) {
	return m.retryCount, m.retryErr
}

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

// Verify the mockStore and mockManager satisfy their respective interfaces
// at compile time.
var _ StorageService = (*mockStore)(nil)
var _ CrawlService = (*mockManager)(nil)
