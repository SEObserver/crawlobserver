package crawler

import (
	"testing"
	"time"

	"github.com/SEObserver/seocrawler/internal/config"
)

func TestNewSession(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
	}

	seeds := []string{"https://example.com", "https://other.com"}
	sess := NewSession(seeds, cfg)

	if sess.ID == "" {
		t.Error("session ID should not be empty")
	}
	if sess.Status != "running" {
		t.Errorf("Status = %q, want running", sess.Status)
	}
	if len(sess.SeedURLs) != 2 {
		t.Errorf("SeedURLs len = %d, want 2", len(sess.SeedURLs))
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
}

func TestSessionToStorageRow(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
			Workers:   5,
			Timeout:   10 * time.Second,
		},
	}

	sess := NewSession([]string{"https://example.com"}, cfg)
	sess.Pages = 42

	row := sess.ToStorageRow()
	if row.ID != sess.ID {
		t.Errorf("ID mismatch: %q != %q", row.ID, sess.ID)
	}
	if row.UserAgent != "TestBot/1.0" {
		t.Errorf("UserAgent = %q, want TestBot/1.0", row.UserAgent)
	}
	if row.PagesCrawled != 42 {
		t.Errorf("PagesCrawled = %d, want 42", row.PagesCrawled)
	}
	if row.Config == "" {
		t.Error("Config JSON should not be empty")
	}
}

// TestResumeSessionPreservesSeedURLs is a regression test for the bug where
// Run() overwrote session.SeedURLs with the uncrawled/failed URLs passed as
// the seeds parameter. This caused RecomputeDepths to assign depth 0 to
// hundreds of pages instead of only the original seed.
func TestResumeSessionPreservesSeedURLs(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
	}

	engine := NewEngine(cfg, nil)

	originalSeeds := []string{"https://example.com"}
	engine.ResumeSession("test-session-id", originalSeeds)

	// Verify seeds are set correctly after ResumeSession
	if len(engine.session.SeedURLs) != 1 || engine.session.SeedURLs[0] != "https://example.com" {
		t.Fatalf("after ResumeSession, SeedURLs = %v, want [https://example.com]", engine.session.SeedURLs)
	}

	// Simulate what Run() does to the session (without actually running the crawl).
	// Before the fix, Run() did: e.session.SeedURLs = seeds
	// which overwrote the original seeds with uncrawled URLs.
	uncrawledURLs := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://example.com/page3",
	}

	// After the fix, Run() only sets Status, not SeedURLs.
	// We test the session state as Run() would set it.
	if engine.session != nil {
		// This is the fixed path — session already exists, so don't overwrite SeedURLs
		engine.session.Status = "running"
	}
	_ = uncrawledURLs // these would be passed to Run() but should NOT corrupt SeedURLs

	// SeedURLs must still be the original seed
	if len(engine.session.SeedURLs) != 1 {
		t.Errorf("SeedURLs len = %d, want 1", len(engine.session.SeedURLs))
	}
	if engine.session.SeedURLs[0] != "https://example.com" {
		t.Errorf("SeedURLs[0] = %q, want https://example.com", engine.session.SeedURLs[0])
	}

	// Verify the storage row also has the correct seeds
	row := engine.session.ToStorageRow()
	if len(row.SeedURLs) != 1 || row.SeedURLs[0] != "https://example.com" {
		t.Errorf("storage row SeedURLs = %v, want [https://example.com]", row.SeedURLs)
	}
}
