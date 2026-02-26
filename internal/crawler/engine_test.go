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
