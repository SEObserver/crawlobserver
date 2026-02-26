package frontier

import (
	"testing"
	"time"
)

func TestFrontierAddAndNext(t *testing.T) {
	f := New(0) // no delay

	added := f.Add(CrawlURL{URL: "https://example.com/a", Priority: 2, Depth: 0})
	if !added {
		t.Error("expected URL to be added")
	}

	f.Add(CrawlURL{URL: "https://example.com/b", Priority: 1, Depth: 0})
	f.Add(CrawlURL{URL: "https://example.com/c", Priority: 3, Depth: 0})

	if f.Len() != 3 {
		t.Errorf("expected Len() = 3, got %d", f.Len())
	}

	// Should return in priority order (lowest first)
	next := f.Next()
	if next == nil || next.URL != "https://example.com/b" {
		t.Errorf("expected /b (priority 1), got %+v", next)
	}

	next = f.Next()
	if next == nil || next.URL != "https://example.com/a" {
		t.Errorf("expected /a (priority 2), got %+v", next)
	}

	next = f.Next()
	if next == nil || next.URL != "https://example.com/c" {
		t.Errorf("expected /c (priority 3), got %+v", next)
	}

	next = f.Next()
	if next != nil {
		t.Errorf("expected nil when empty, got %+v", next)
	}
}

func TestFrontierDedup(t *testing.T) {
	f := New(0)

	if !f.Add(CrawlURL{URL: "https://example.com/page"}) {
		t.Error("first add should succeed")
	}
	if f.Add(CrawlURL{URL: "https://example.com/page"}) {
		t.Error("duplicate add should fail")
	}
	if f.Len() != 1 {
		t.Errorf("expected Len() = 1, got %d", f.Len())
	}
	if f.SeenCount() != 1 {
		t.Errorf("expected SeenCount() = 1, got %d", f.SeenCount())
	}
}

func TestFrontierPerHostDelay(t *testing.T) {
	f := New(100 * time.Millisecond)

	f.Add(CrawlURL{URL: "https://example.com/a", Priority: 1})
	f.Add(CrawlURL{URL: "https://example.com/b", Priority: 2})
	f.Add(CrawlURL{URL: "https://other.com/c", Priority: 3})

	// First fetch from example.com should work
	next := f.Next()
	if next == nil || next.URL != "https://example.com/a" {
		t.Errorf("expected example.com/a, got %+v", next)
	}

	// Second fetch from example.com should be blocked (delay not elapsed)
	// But other.com should be available
	next = f.Next()
	if next == nil || next.URL != "https://other.com/c" {
		t.Errorf("expected other.com/c (example.com is delayed), got %+v", next)
	}

	// example.com/b should still be blocked
	next = f.Next()
	if next != nil {
		t.Errorf("expected nil (all hosts delayed), got %+v", next)
	}

	// Wait for delay to pass
	time.Sleep(150 * time.Millisecond)

	next = f.Next()
	if next == nil || next.URL != "https://example.com/b" {
		t.Errorf("expected example.com/b after delay, got %+v", next)
	}
}

func TestFrontierClose(t *testing.T) {
	f := New(0)
	f.Add(CrawlURL{URL: "https://example.com/a"})
	f.Close()

	if f.Add(CrawlURL{URL: "https://example.com/b"}) {
		t.Error("should not add after close")
	}

	// Should still be able to drain existing items
	next := f.Next()
	if next == nil {
		t.Error("should still get existing items after close")
	}
}

func TestURLDb(t *testing.T) {
	db := NewURLDb()

	if db.Has("https://example.com") {
		t.Error("should not have unseen URL")
	}

	if !db.Add("https://example.com") {
		t.Error("first add should return true")
	}

	if db.Add("https://example.com") {
		t.Error("second add should return false")
	}

	if !db.Has("https://example.com") {
		t.Error("should have seen URL")
	}

	if db.Len() != 1 {
		t.Errorf("expected Len() = 1, got %d", db.Len())
	}
}
