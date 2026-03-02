//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/server"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

const (
	testUser = "admin"
	testPass = "testpass"
)

// testEnv holds the servers and IDs created by setup.
type testEnv struct {
	apiURL  string // base URL of the API server
	siteURL string // base URL of the test site
}

// setup creates a ClickHouse store, API server, and test site server.
func setup(t *testing.T) *testEnv {
	t.Helper()

	chHost := envOr("CH_HOST", "localhost")
	chPort := envOrInt("CH_PORT", 19000)

	store, err := storage.NewStore(chHost, chPort, "crawlobserver", "default", "")
	if err != nil {
		t.Skipf("ClickHouse unavailable (%s:%d): %v", chHost, chPort, err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	keyStore, err := apikeys.NewStore(":memory:")
	if err != nil {
		t.Fatalf("apikeys store: %v", err)
	}
	t.Cleanup(func() { keyStore.Close() })

	site := httptest.NewServer(testSiteHandler())
	t.Cleanup(site.Close)

	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			Workers:         4,
			MaxPages:        100,
			Delay:           0,
			Timeout:         10 * time.Second,
			UserAgent:       "CrawlObserverE2ETest/1.0",
			MaxBodySize:     10 * 1024 * 1024,
			RespectRobots:   true,
			AllowPrivateIPs: true,
			CrawlScope:      "host",
			Retry: config.RetryConfig{
				MaxRetries:          0,
				MaxConsecutiveFails: 10,
				MaxGlobalErrorRate:  0.8,
			},
		},
		ClickHouse: config.ClickHouseConfig{
			Host:     chHost,
			Port:     chPort,
			Database: "crawlobserver",
			Username: "default",
		},
		Storage: config.StorageConfig{
			BatchSize:     50,
			FlushInterval: 1 * time.Second,
		},
		Server: config.ServerConfig{
			Host:     "127.0.0.1",
			Port:     0,
			Username: testUser,
			Password: testPass,
		},
	}

	srv := server.New(cfg, store, keyStore)
	handler, err := srv.Handler()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	apiServer := httptest.NewServer(handler)
	t.Cleanup(apiServer.Close)

	return &testEnv{
		apiURL:  apiServer.URL,
		siteURL: site.URL,
	}
}

// --- HTTP helpers ---

func apiGET(t *testing.T, baseURL, path string) []byte {
	t.Helper()
	req, _ := http.NewRequest("GET", baseURL+path, nil)
	req.SetBasicAuth(testUser, testPass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("GET %s: status %d: %s", path, resp.StatusCode, body)
	}
	return body
}

func apiPOST(t *testing.T, baseURL, path, body string) []byte {
	t.Helper()
	req, _ := http.NewRequest("POST", baseURL+path, strings.NewReader(body))
	req.SetBasicAuth(testUser, testPass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("POST %s: status %d: %s", path, resp.StatusCode, respBody)
	}
	return respBody
}

func apiDELETE(t *testing.T, baseURL, path string) []byte {
	t.Helper()
	req, _ := http.NewRequest("DELETE", baseURL+path, nil)
	req.SetBasicAuth(testUser, testPass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("DELETE %s: status %d: %s", path, resp.StatusCode, body)
	}
	return body
}

// waitForCrawl polls progress until is_running=false, then waits for ClickHouse mutations.
func waitForCrawl(t *testing.T, baseURL, sessionID string, timeout time.Duration) {
	t.Helper()
	// Small initial delay to let the engine initialize (avoids race on buffer field)
	time.Sleep(500 * time.Millisecond)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body := apiGET(t, baseURL, "/api/sessions/"+sessionID+"/progress")
		var progress map[string]interface{}
		if err := json.Unmarshal(body, &progress); err != nil {
			t.Fatalf("parse progress: %v", err)
		}
		running, _ := progress["is_running"].(bool)
		if !running {
			// Wait for ClickHouse to finalize async mutations
			time.Sleep(2 * time.Second)
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("crawl %s did not finish within %v", sessionID, timeout)
}

// startCrawl starts a crawl on the test site and returns the session ID.
func startCrawl(t *testing.T, env *testEnv) string {
	t.Helper()
	checkExt := false
	checkRes := false
	body := apiPOST(t, env.apiURL, "/api/crawl", fmt.Sprintf(`{
		"seeds": ["%s"],
		"max_pages": 50,
		"workers": 4,
		"delay": "0s",
		"user_agent": "CrawlObserverE2ETest/1.0",
		"check_external_links": %v,
		"check_page_resources": %v
	}`, env.siteURL+"/", checkExt, checkRes))

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse crawl response: %v", err)
	}
	sid := result["session_id"]
	if sid == "" {
		t.Fatalf("empty session_id in response: %s", body)
	}
	return sid
}

// --- Tests ---

func TestE2E_FullCrawl(t *testing.T) {
	env := setup(t)

	sid := startCrawl(t, env)
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+sid)
	})

	waitForCrawl(t, env.apiURL, sid, 60*time.Second)

	// Check session in list
	sessBody := apiGET(t, env.apiURL, "/api/sessions")
	var sessions []map[string]interface{}
	mustUnmarshal(t, sessBody, &sessions)

	var found map[string]interface{}
	for _, s := range sessions {
		if s["ID"] == sid {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatalf("session %s not found in sessions list", sid)
	}

	status, _ := found["Status"].(string)
	if status != "completed" {
		t.Errorf("expected status=completed, got %s", status)
	}

	pagesCrawled, _ := found["PagesCrawled"].(float64)
	if pagesCrawled < 8 {
		t.Errorf("expected pages_crawled >= 8, got %.0f", pagesCrawled)
	}

	// Check pages (PascalCase field names — no JSON tags on PageRow)
	pagesBody := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/pages?limit=100")
	var pages []map[string]interface{}
	mustUnmarshal(t, pagesBody, &pages)

	if len(pages) < 8 {
		t.Errorf("expected at least 8 pages, got %d", len(pages))
	}

	// Verify /private/secret is NOT crawled (robots.txt)
	for _, p := range pages {
		url, _ := p["URL"].(string)
		if strings.Contains(url, "/private/") {
			t.Errorf("robots.txt blocked URL was crawled: %s", url)
		}
	}

	// Verify /products exists (redirect destination)
	foundProducts := false
	for _, p := range pages {
		url, _ := p["URL"].(string)
		if strings.HasSuffix(url, "/products") {
			foundProducts = true
			break
		}
	}
	if !foundProducts {
		t.Error("/products not found in crawled pages (redirect destination)")
	}

	// Check depths are coherent (homepage = 0)
	for _, p := range pages {
		url, _ := p["URL"].(string)
		depth, _ := p["Depth"].(float64)
		if strings.HasSuffix(url, "/") && !strings.Contains(url, "/products") && !strings.Contains(url, "/blog") {
			if depth != 0 {
				t.Errorf("expected homepage depth=0, got %.0f for %s", depth, url)
			}
		}
	}

	// Check stats have status codes
	statsBody := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/stats")
	var stats map[string]interface{}
	mustUnmarshal(t, statsBody, &stats)

	statusCodes, _ := stats["status_codes"].(map[string]interface{})
	if statusCodes == nil {
		t.Fatal("status_codes missing from stats")
	}

	count2xx, _ := statusCodes["200"].(float64)
	if count2xx == 0 {
		t.Error("expected status_codes[200] > 0")
	}

	count404, _ := statusCodes["404"].(float64)
	if count404 == 0 {
		t.Error("expected status_codes[404] > 0 (the /gone page)")
	}
}

func TestE2E_Robots(t *testing.T) {
	env := setup(t)

	sid := startCrawl(t, env)
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+sid)
	})
	waitForCrawl(t, env.apiURL, sid, 60*time.Second)

	// Check robots hosts (PascalCase — no JSON tags on RobotsRow)
	robotsBody := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/robots")
	var robots []map[string]interface{}
	mustUnmarshal(t, robotsBody, &robots)

	if len(robots) == 0 {
		t.Fatal("expected at least one robots.txt host entry")
	}

	host, _ := robots[0]["Host"].(string)
	if host == "" {
		t.Error("robots host is empty")
	}

	statusCode, _ := robots[0]["StatusCode"].(float64)
	if statusCode != 200 {
		t.Errorf("expected robots.txt status_code=200, got %.0f", statusCode)
	}

	// Check robots content
	robotsContentBody := apiGET(t, env.apiURL, fmt.Sprintf("/api/sessions/%s/robots-content?host=%s", sid, host))
	var robotsContent map[string]interface{}
	mustUnmarshal(t, robotsContentBody, &robotsContent)

	content, _ := robotsContent["Content"].(string)
	if !strings.Contains(content, "Disallow: /private/") {
		t.Errorf("robots.txt content doesn't contain expected rule, got: %s", content)
	}
}

func TestE2E_Stats(t *testing.T) {
	env := setup(t)

	sid := startCrawl(t, env)
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+sid)
	})
	waitForCrawl(t, env.apiURL, sid, 60*time.Second)

	statsBody := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/stats")
	var stats map[string]interface{}
	mustUnmarshal(t, statsBody, &stats)

	totalPages, _ := stats["total_pages"].(float64)
	if totalPages < 8 {
		t.Errorf("expected total_pages >= 8, got %.0f", totalPages)
	}

	totalLinks, _ := stats["total_links"].(float64)
	if totalLinks == 0 {
		t.Error("expected total_links > 0")
	}

	internalLinks, _ := stats["internal_links"].(float64)
	if internalLinks == 0 {
		t.Error("expected internal_links > 0")
	}

	statusCodes, _ := stats["status_codes"].(map[string]interface{})
	if statusCodes == nil {
		t.Fatal("status_codes missing from stats")
	}
	if len(statusCodes) < 2 {
		t.Errorf("expected at least 2 different status codes, got %d", len(statusCodes))
	}

	depthDist, _ := stats["depth_distribution"].(map[string]interface{})
	if depthDist == nil {
		t.Fatal("depth_distribution missing from stats")
	}
	if len(depthDist) < 2 {
		t.Errorf("expected at least 2 depth levels, got %d", len(depthDist))
	}
}

func TestE2E_Pages_Filters(t *testing.T) {
	env := setup(t)

	sid := startCrawl(t, env)
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+sid)
	})
	waitForCrawl(t, env.apiURL, sid, 60*time.Second)

	// Filter by status_code=404 (PascalCase JSON field: StatusCode)
	body404 := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/pages?status_code=404")
	var pages404 []map[string]interface{}
	mustUnmarshal(t, body404, &pages404)

	if len(pages404) == 0 {
		t.Fatal("expected at least one 404 page")
	}
	for _, p := range pages404 {
		sc, _ := p["StatusCode"].(float64)
		if sc != 404 {
			t.Errorf("filter status_code=404 returned page with StatusCode=%v", sc)
		}
	}

	// Pagination: limit=3
	bodyLimited := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/pages?limit=3&offset=0")
	var pagesLimited []map[string]interface{}
	mustUnmarshal(t, bodyLimited, &pagesLimited)

	if len(pagesLimited) > 3 {
		t.Errorf("expected at most 3 pages with limit=3, got %d", len(pagesLimited))
	}
	if len(pagesLimited) == 0 {
		t.Fatal("expected at least 1 page with limit=3")
	}

	// Verify limit=3 offset=3 returns pages (pagination works)
	bodyOffset := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/pages?limit=3&offset=3")
	var pagesOffset []map[string]interface{}
	mustUnmarshal(t, bodyOffset, &pagesOffset)

	if len(pagesOffset) == 0 {
		t.Fatal("expected pages at offset=3")
	}

	// Collect all URLs from both batches to verify they don't fully overlap
	urlSet := make(map[string]bool)
	for _, p := range pagesLimited {
		u, _ := p["URL"].(string)
		urlSet[u] = true
	}
	overlapCount := 0
	for _, p := range pagesOffset {
		u, _ := p["URL"].(string)
		if urlSet[u] {
			overlapCount++
		}
	}
	if overlapCount == len(pagesOffset) && len(pagesOffset) > 0 {
		t.Error("pagination offset=3 returned exact same pages as offset=0")
	}
}

func TestE2E_Links(t *testing.T) {
	env := setup(t)

	sid := startCrawl(t, env)
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+sid)
	})
	waitForCrawl(t, env.apiURL, sid, 60*time.Second)

	// Internal links (PascalCase JSON field names: SourceURL, TargetURL)
	linksBody := apiGET(t, env.apiURL, "/api/sessions/"+sid+"/internal-links?limit=100")
	var links []map[string]interface{}
	mustUnmarshal(t, linksBody, &links)

	if len(links) == 0 {
		t.Fatal("expected at least one internal link")
	}

	first := links[0]
	if _, ok := first["SourceURL"]; !ok {
		t.Error("link missing SourceURL field")
	}
	if _, ok := first["TargetURL"]; !ok {
		t.Error("link missing TargetURL field")
	}
}

func TestE2E_SessionDelete(t *testing.T) {
	env := setup(t)

	sid := startCrawl(t, env)
	waitForCrawl(t, env.apiURL, sid, 60*time.Second)

	// Delete
	delBody := apiDELETE(t, env.apiURL, "/api/sessions/"+sid)
	var delResp map[string]string
	mustUnmarshal(t, delBody, &delResp)
	if delResp["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %s", delResp["status"])
	}

	// Wait for ClickHouse async delete to propagate
	time.Sleep(2 * time.Second)

	// Verify gone from list
	sessBody := apiGET(t, env.apiURL, "/api/sessions")
	var sessions []map[string]interface{}
	mustUnmarshal(t, sessBody, &sessions)

	for _, s := range sessions {
		if s["ID"] == sid {
			t.Errorf("session %s still present after deletion", sid)
		}
	}
}

func TestE2E_ExportImport(t *testing.T) {
	env := setup(t)

	// Step 1: Crawl a site
	sid := startCrawl(t, env)
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+sid)
	})
	waitForCrawl(t, env.apiURL, sid, 60*time.Second)

	// Collect original data for comparison
	var origPages []map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+sid+"/pages?limit=100"), &origPages)

	var origLinks []map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+sid+"/internal-links?limit=1000"), &origLinks)

	var origStats map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+sid+"/stats"), &origStats)

	if len(origPages) < 8 {
		t.Fatalf("expected at least 8 original pages, got %d", len(origPages))
	}
	if len(origLinks) == 0 {
		t.Fatal("expected original links > 0")
	}

	// Step 2: Export the session
	exportData := apiGETRaw(t, env.apiURL, "/api/sessions/"+sid+"/export")
	if len(exportData) == 0 {
		t.Fatal("export returned empty data")
	}

	// Step 3: Import the exported file (simulates another user receiving the file)
	importResp := apiPOSTMultipart(t, env.apiURL, "/api/sessions/import", "file", "crawl.jsonl.gz", exportData)
	var importedSession map[string]interface{}
	mustUnmarshal(t, importResp, &importedSession)

	importedID, _ := importedSession["ID"].(string)
	if importedID == "" {
		t.Fatal("imported session has empty ID")
	}
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+importedID)
	})

	// Wait for ClickHouse to settle
	time.Sleep(2 * time.Second)

	// Step 4: Verify imported session metadata
	if importedID == sid {
		t.Error("imported session should have a NEW ID, got same as original")
	}

	status, _ := importedSession["Status"].(string)
	if status != "imported" {
		t.Errorf("expected imported session status='imported', got %q", status)
	}

	// Step 5: Verify imported session appears in session list
	sessBody := apiGET(t, env.apiURL, "/api/sessions")
	var sessions []map[string]interface{}
	mustUnmarshal(t, sessBody, &sessions)

	foundImported := false
	for _, s := range sessions {
		if s["ID"] == importedID {
			foundImported = true
			break
		}
	}
	if !foundImported {
		t.Errorf("imported session %s not found in sessions list", importedID)
	}

	// Step 6: Verify pages are identical
	var importedPages []map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+importedID+"/pages?limit=100"), &importedPages)

	if len(importedPages) != len(origPages) {
		t.Errorf("page count mismatch: original=%d imported=%d", len(origPages), len(importedPages))
	}

	// Build URL sets for comparison
	origURLs := make(map[string]bool)
	for _, p := range origPages {
		u, _ := p["URL"].(string)
		origURLs[u] = true
	}
	importedURLs := make(map[string]bool)
	for _, p := range importedPages {
		u, _ := p["URL"].(string)
		importedURLs[u] = true
	}
	for u := range origURLs {
		if !importedURLs[u] {
			t.Errorf("original URL %s missing from imported session", u)
		}
	}

	// Step 7: Verify links are preserved
	var importedLinks []map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+importedID+"/internal-links?limit=1000"), &importedLinks)

	if len(importedLinks) != len(origLinks) {
		t.Errorf("link count mismatch: original=%d imported=%d", len(origLinks), len(importedLinks))
	}

	// Step 8: Verify stats match
	var importedStats map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+importedID+"/stats"), &importedStats)

	origTotal, _ := origStats["total_pages"].(float64)
	importedTotal, _ := importedStats["total_pages"].(float64)
	if origTotal != importedTotal {
		t.Errorf("total_pages mismatch: original=%.0f imported=%.0f", origTotal, importedTotal)
	}

	origIntLinks, _ := origStats["internal_links"].(float64)
	importedIntLinks, _ := importedStats["internal_links"].(float64)
	if origIntLinks != importedIntLinks {
		t.Errorf("internal_links mismatch: original=%.0f imported=%.0f", origIntLinks, importedIntLinks)
	}

	// Step 9: Verify page details are preserved (title, status_code)
	// Match pages by URL across both sets
	origByURL := make(map[string]map[string]interface{})
	for _, p := range origPages {
		u, _ := p["URL"].(string)
		origByURL[u] = p
	}
	importedByURL := make(map[string]map[string]interface{})
	for _, p := range importedPages {
		u, _ := p["URL"].(string)
		importedByURL[u] = p
	}

	for u, orig := range origByURL {
		imp, ok := importedByURL[u]
		if !ok {
			continue // already checked in step 6
		}
		origTitle, _ := orig["Title"].(string)
		impTitle, _ := imp["Title"].(string)
		if origTitle != impTitle {
			t.Errorf("title mismatch for %s: original=%q imported=%q", u, origTitle, impTitle)
		}
		origSC, _ := orig["StatusCode"].(float64)
		impSC, _ := imp["StatusCode"].(float64)
		if origSC != impSC {
			t.Errorf("status_code mismatch for %s: original=%.0f imported=%.0f", u, origSC, impSC)
		}
	}

	// Step 10: Verify robots.txt is preserved
	var origRobots []map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+sid+"/robots"), &origRobots)
	var importedRobots []map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+importedID+"/robots"), &importedRobots)

	if len(origRobots) != len(importedRobots) {
		t.Errorf("robots count mismatch: original=%d imported=%d", len(origRobots), len(importedRobots))
	}

	// Step 11: Double export-import (re-export imported session, re-import)
	reExportData := apiGETRaw(t, env.apiURL, "/api/sessions/"+importedID+"/export")
	reImportResp := apiPOSTMultipart(t, env.apiURL, "/api/sessions/import", "file", "crawl2.jsonl.gz", reExportData)
	var reImportedSession map[string]interface{}
	mustUnmarshal(t, reImportResp, &reImportedSession)

	reImportedID, _ := reImportedSession["ID"].(string)
	t.Cleanup(func() {
		apiDELETE(t, env.apiURL, "/api/sessions/"+reImportedID)
	})

	time.Sleep(2 * time.Second)

	// Verify the re-imported session also has the same page count
	var reImportedPages []map[string]interface{}
	mustUnmarshal(t, apiGET(t, env.apiURL, "/api/sessions/"+reImportedID+"/pages?limit=100"), &reImportedPages)

	if len(reImportedPages) != len(origPages) {
		t.Errorf("re-imported page count mismatch: original=%d re-imported=%d", len(origPages), len(reImportedPages))
	}

	// All three sessions should have different IDs
	if reImportedID == sid || reImportedID == importedID {
		t.Error("re-imported session should have a unique ID")
	}
}

// --- HTTP helpers ---

// apiGETRaw returns the raw response body (for binary data like gzipped exports).
func apiGETRaw(t *testing.T, baseURL, path string) []byte {
	t.Helper()
	req, _ := http.NewRequest("GET", baseURL+path, nil)
	req.SetBasicAuth(testUser, testPass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("GET %s: status %d: %s", path, resp.StatusCode, body)
	}
	return body
}

// apiPOSTMultipart sends a multipart form upload.
func apiPOSTMultipart(t *testing.T, baseURL, path, fieldName, fileName string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form data: %v", err)
	}
	w.Close()

	req, _ := http.NewRequest("POST", baseURL+path, &buf)
	req.SetBasicAuth(testUser, testPass)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("POST %s: status %d: %s", path, resp.StatusCode, respBody)
	}
	return respBody
}

// --- Helpers ---

func mustUnmarshal(t *testing.T, data []byte, v interface{}) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, data)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}
