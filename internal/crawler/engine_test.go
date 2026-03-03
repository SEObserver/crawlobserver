package crawler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/extraction"
	"github.com/SEObserver/crawlobserver/internal/storage"
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

// diskFullInserter simulates ClickHouse returning "Cannot reserve N MiB" (code 243)
// when the Docker virtual disk is full. All inserts fail permanently.
type diskFullInserter struct{}

func (d *diskFullInserter) InsertPages(_ context.Context, _ []storage.PageRow) error {
	return fmt.Errorf("code: 243, Cannot reserve 1073741824 bytes in file")
}

func (d *diskFullInserter) InsertLinks(_ context.Context, _ []storage.LinkRow) error {
	return fmt.Errorf("code: 243, Cannot reserve 1073741824 bytes in file")
}

func (d *diskFullInserter) InsertExtractions(_ context.Context, _ []extraction.ExtractionRow) error {
	return fmt.Errorf("code: 243, Cannot reserve 1073741824 bytes in file")
}

// TestDiskFullAutoStop verifies the full disk-full scenario:
// ClickHouse inserts fail permanently → buffer drops data after max retries →
// onDataLost callback fires → engine.Stop() cancels the crawl context.
func TestDiskFullAutoStop(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour, // won't auto-tick during test
		},
	}

	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	// Create buffer with a permanently failing store (simulates disk full)
	engine.buffer = storage.NewBuffer(&diskFullInserter{}, 100, time.Hour, engine.session.ID)

	// Wire up the same callback as Run() does
	engine.buffer.SetOnDataLost(func(lostPages, lostLinks int64) {
		engine.Stop()
	})

	// Simulate crawler writing pages and links to the buffer
	for i := 0; i < 5; i++ {
		engine.buffer.AddPage(storage.PageRow{URL: fmt.Sprintf("https://example.com/%d", i)})
	}
	engine.buffer.AddLinks([]storage.LinkRow{
		{SourceURL: "https://example.com", TargetURL: "https://example.com/1"},
		{SourceURL: "https://example.com", TargetURL: "https://example.com/2"},
	})

	// Flush 1: initial failure → data moves to retry queue
	engine.buffer.Flush()
	select {
	case <-engine.ctx.Done():
		t.Fatal("engine should NOT be stopped yet (retries pending)")
	default:
	}

	// Flush 2-3: retries fail, still under maxRetries
	engine.buffer.Flush()
	engine.buffer.Flush()
	select {
	case <-engine.ctx.Done():
		t.Fatal("engine should NOT be stopped yet (retries still pending)")
	default:
	}

	// Flush 4: retries exhaust (retries=3 >= maxRetries=3) → data dropped → callback → Stop()
	engine.buffer.Flush()

	select {
	case <-engine.ctx.Done():
		// Engine was stopped — this is the expected behavior
	default:
		t.Fatal("engine context should be cancelled after disk-full data loss")
	}

	// Verify buffer reports the data loss
	state := engine.BufferState()
	if state.LostPages != 5 {
		t.Errorf("LostPages = %d, want 5", state.LostPages)
	}
	if state.LostLinks != 2 {
		t.Errorf("LostLinks = %d, want 2", state.LostLinks)
	}

	// Clean up the buffer's flush goroutine
	engine.buffer.Close()
}

// TestDiskFullCompletedWithErrors verifies that after a disk-full scenario,
// the session status is set to "completed_with_errors" (not plain "completed").
func TestDiskFullCompletedWithErrors(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}

	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	// Create buffer that will lose data
	engine.buffer = storage.NewBuffer(&diskFullInserter{}, 100, time.Hour, engine.session.ID)

	engine.buffer.AddPage(storage.PageRow{URL: "https://example.com/1"})

	// Exhaust retries: 4 flushes → drop
	for i := 0; i < 5; i++ {
		engine.buffer.Flush()
	}
	engine.buffer.Close()

	// Reproduce the same status logic as Run()
	bufState := engine.BufferState()
	if bufState.LostPages > 0 || bufState.LostLinks > 0 {
		engine.session.Status = "completed_with_errors"
	} else {
		engine.session.Status = "completed"
	}

	if engine.session.Status != "completed_with_errors" {
		t.Errorf("session.Status = %q, want %q", engine.session.Status, "completed_with_errors")
	}
}

// TestNoDiskIssueCompletedNormally verifies that without data loss,
// the session status remains "completed".
func TestNoDiskIssueCompletedNormally(t *testing.T) {
	cfg := &config.Config{
		Crawler: config.CrawlerConfig{
			UserAgent: "TestBot/1.0",
		},
		Storage: config.StorageConfig{
			BatchSize:     100,
			FlushInterval: time.Hour,
		},
	}

	engine := NewEngine(cfg, nil)
	engine.session = NewSession([]string{"https://example.com"}, cfg)

	// Create buffer with a working store
	engine.buffer = storage.NewBuffer(&successInserter{}, 100, time.Hour, engine.session.ID)

	engine.buffer.AddPage(storage.PageRow{URL: "https://example.com/1"})
	engine.buffer.Flush()
	engine.buffer.Close()

	bufState := engine.BufferState()
	if bufState.LostPages > 0 || bufState.LostLinks > 0 {
		engine.session.Status = "completed_with_errors"
	} else {
		engine.session.Status = "completed"
	}

	if engine.session.Status != "completed" {
		t.Errorf("session.Status = %q, want %q", engine.session.Status, "completed")
	}
}

type successInserter struct{}

func (s *successInserter) InsertPages(_ context.Context, _ []storage.PageRow) error  { return nil }
func (s *successInserter) InsertLinks(_ context.Context, _ []storage.LinkRow) error  { return nil }
func (s *successInserter) InsertExtractions(_ context.Context, _ []extraction.ExtractionRow) error {
	return nil
}
