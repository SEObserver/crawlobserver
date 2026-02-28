//go:build integration

package storage

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// testStore creates a Store connected to a local ClickHouse, runs migrations,
// and returns it. Skips the test if CH is not reachable.
func testStore(t *testing.T) *Store {
	t.Helper()

	host := os.Getenv("CH_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 19000 // default mapped port for crawlobserver-clickhouse
	if p := os.Getenv("CH_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	s, err := NewStore(host, port, "default", "default", "")
	if err != nil {
		t.Skipf("ClickHouse not available: %v", err)
	}

	ctx := context.Background()

	// Create test database
	if err := s.conn.Exec(ctx, "CREATE DATABASE IF NOT EXISTS crawlobserver"); err != nil {
		t.Fatalf("creating test database: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	return s
}

// cleanupSession removes all test data for a given session ID.
func cleanupSession(t *testing.T, s *Store, sessionID string) {
	t.Helper()
	ctx := context.Background()
	tables := []string{"external_link_checks", "links"}
	for _, tbl := range tables {
		if err := s.conn.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE crawlobserver.%s DELETE WHERE crawl_session_id = ?", tbl,
		), sessionID); err != nil {
			t.Logf("cleanup %s: %v", tbl, err)
		}
	}
}

func TestGetExpiredDomains_NoData(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	result, err := s.GetExpiredDomains(ctx, "00000000-0000-0000-0000-000000000000", 100, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
	if len(result.Domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(result.Domains))
	}
}

func TestGetExpiredDomains_FullScenario(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	t.Cleanup(func() { cleanupSession(t, s, sessionID) })
	// Clean before in case previous run left data
	cleanupSession(t, s, sessionID)

	now := time.Now()

	// Insert external link checks:
	// - expired.com: 3 URLs, all dns_not_found → expired
	// - gone.org: 1 URL with old-style "no such host" error → expired
	// - alive.com: 1 URL with dns_not_found + 1 URL with status 200 → NOT expired
	// - down.net: 1 URL with connection_refused → NOT expired (DNS works)
	checks := []ExternalLinkCheck{
		{CrawlSessionID: sessionID, URL: "https://www.expired.com/page1", Error: "dns_not_found", CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://expired.com/page2", Error: "dns_not_found", CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://cdn.expired.com/asset", Error: "dns_not_found", CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://gone.org/old", Error: `Get "https://gone.org/old": dial tcp: lookup gone.org: no such host`, CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://alive.com/page", Error: "dns_not_found", CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://alive.com/ok", StatusCode: 200, CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://down.net/x", Error: "connection_refused", CheckedAt: now},
	}
	if err := s.InsertExternalLinkChecks(ctx, checks); err != nil {
		t.Fatalf("inserting checks: %v", err)
	}

	// Insert links (source pages → target URLs)
	links := []LinkRow{
		{CrawlSessionID: sessionID, SourceURL: "https://site-a.com/page1", TargetURL: "https://www.expired.com/page1", IsInternal: false, CrawledAt: now},
		{CrawlSessionID: sessionID, SourceURL: "https://site-b.com/links", TargetURL: "https://expired.com/page2", IsInternal: false, CrawledAt: now},
		{CrawlSessionID: sessionID, SourceURL: "https://site-a.com/page1", TargetURL: "https://gone.org/old", IsInternal: false, CrawledAt: now},
		{CrawlSessionID: sessionID, SourceURL: "https://site-a.com/page1", TargetURL: "https://alive.com/page", IsInternal: false, CrawledAt: now},
		// Internal link (should NOT appear in sources)
		{CrawlSessionID: sessionID, SourceURL: "https://site-a.com/page1", TargetURL: "https://site-a.com/page2", IsInternal: true, CrawledAt: now},
	}
	if err := s.InsertLinks(ctx, links); err != nil {
		t.Fatalf("inserting links: %v", err)
	}

	// Wait for ClickHouse to process mutations
	time.Sleep(500 * time.Millisecond)

	// Test: get all expired domains
	result, err := s.GetExpiredDomains(ctx, sessionID, 100, 0)
	if err != nil {
		t.Fatalf("GetExpiredDomains: %v", err)
	}

	if result.Total != 2 {
		t.Fatalf("expected total 2 expired domains, got %d", result.Total)
	}
	if len(result.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(result.Domains))
	}

	// expired.com should be first (3 dead URLs > 1)
	domainMap := make(map[string]ExpiredDomain)
	for _, d := range result.Domains {
		domainMap[d.RegistrableDomain] = d
	}

	// Verify expired.com
	exp, ok := domainMap["expired.com"]
	if !ok {
		t.Fatal("expired.com not found in results")
	}
	if exp.DeadURLsChecked != 3 {
		t.Errorf("expired.com: expected 3 dead URLs, got %d", exp.DeadURLsChecked)
	}
	if len(exp.Sources) < 1 {
		t.Errorf("expired.com: expected sources, got %d", len(exp.Sources))
	}

	// Verify gone.org (old-style error format)
	gone, ok := domainMap["gone.org"]
	if !ok {
		t.Fatal("gone.org not found in results — old-style error not matched by hybrid condition")
	}
	if gone.DeadURLsChecked != 1 {
		t.Errorf("gone.org: expected 1 dead URL, got %d", gone.DeadURLsChecked)
	}

	// Verify alive.com and down.net are NOT in results
	if _, ok := domainMap["alive.com"]; ok {
		t.Error("alive.com should NOT be expired (has a 200 response)")
	}
	if _, ok := domainMap["down.net"]; ok {
		t.Error("down.net should NOT be expired (connection_refused, not DNS)")
	}
}

func TestGetExpiredDomains_Pagination(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sessionID := "aaaaaaaa-bbbb-cccc-dddd-ffffffffffff"
	t.Cleanup(func() { cleanupSession(t, s, sessionID) })
	cleanupSession(t, s, sessionID)

	now := time.Now()

	// Insert 3 expired domains with different URL counts for deterministic ordering
	checks := []ExternalLinkCheck{
		// domain-a.com: 3 URLs
		{CrawlSessionID: sessionID, URL: "https://domain-a.com/1", Error: "dns_not_found", CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://domain-a.com/2", Error: "dns_not_found", CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://domain-a.com/3", Error: "dns_not_found", CheckedAt: now},
		// domain-b.com: 2 URLs
		{CrawlSessionID: sessionID, URL: "https://domain-b.com/1", Error: "dns_not_found", CheckedAt: now},
		{CrawlSessionID: sessionID, URL: "https://domain-b.com/2", Error: "dns_not_found", CheckedAt: now},
		// domain-c.com: 1 URL
		{CrawlSessionID: sessionID, URL: "https://domain-c.com/1", Error: "dns_not_found", CheckedAt: now},
	}
	if err := s.InsertExternalLinkChecks(ctx, checks); err != nil {
		t.Fatalf("inserting checks: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Page 1: limit=2, offset=0 → should get domain-a.com (3) and domain-b.com (2)
	page1, err := s.GetExpiredDomains(ctx, sessionID, 2, 0)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if page1.Total != 3 {
		t.Errorf("expected total 3, got %d", page1.Total)
	}
	if len(page1.Domains) != 2 {
		t.Fatalf("expected 2 domains on page 1, got %d", len(page1.Domains))
	}
	if page1.Domains[0].RegistrableDomain != "domain-a.com" {
		t.Errorf("expected first domain domain-a.com, got %s", page1.Domains[0].RegistrableDomain)
	}

	// Page 2: limit=2, offset=2 → should get domain-c.com (1)
	page2, err := s.GetExpiredDomains(ctx, sessionID, 2, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if page2.Total != 3 {
		t.Errorf("expected total 3, got %d", page2.Total)
	}
	if len(page2.Domains) != 1 {
		t.Fatalf("expected 1 domain on page 2, got %d", len(page2.Domains))
	}
	if page2.Domains[0].RegistrableDomain != "domain-c.com" {
		t.Errorf("expected domain-c.com, got %s", page2.Domains[0].RegistrableDomain)
	}
}
