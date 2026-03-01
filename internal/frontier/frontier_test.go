package frontier

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestFrontierAddAndNext(t *testing.T) {
	f := New(0, 0) // no delay

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
	f := New(0, 0)

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
	f := New(100*time.Millisecond, 0)

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
	f := New(0, 0)
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

func TestFrontierMaxSize(t *testing.T) {
	f := New(0, 2) // max 2 items in queue

	if !f.Add(CrawlURL{URL: "https://example.com/a", Priority: 1}) {
		t.Error("first add should succeed")
	}
	if !f.Add(CrawlURL{URL: "https://example.com/b", Priority: 2}) {
		t.Error("second add should succeed")
	}
	if f.Add(CrawlURL{URL: "https://example.com/c", Priority: 3}) {
		t.Error("third add should fail (maxSize=2)")
	}
	if f.Len() != 2 {
		t.Errorf("expected Len() = 2, got %d", f.Len())
	}

	// After dequeuing one, should be able to add again
	next := f.Next()
	if next == nil {
		t.Fatal("expected a URL")
	}
	if !f.Add(CrawlURL{URL: "https://example.com/d", Priority: 4}) {
		t.Error("add after dequeue should succeed")
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

func TestFrontierMaxSizeZeroUnlimited(t *testing.T) {
	f := New(0, 0) // no delay, no limit
	for i := 0; i < 500; i++ {
		ok := f.Add(CrawlURL{URL: fmt.Sprintf("http://example.com/%d", i)})
		if !ok {
			t.Fatalf("Add failed at i=%d, expected unlimited capacity", i)
		}
	}
	if f.Len() != 500 {
		t.Fatalf("expected Len()=500, got %d", f.Len())
	}
}

func TestFrontierMaxSizeWithDedup(t *testing.T) {
	f := New(0, 3)
	f.Add(CrawlURL{URL: "http://a.com/1"})
	f.Add(CrawlURL{URL: "http://a.com/2"})
	// duplicate — should not count toward maxSize
	ok := f.Add(CrawlURL{URL: "http://a.com/1"})
	if ok {
		t.Fatal("expected duplicate to be rejected")
	}
	// should still be able to add one more (slot 3/3)
	ok = f.Add(CrawlURL{URL: "http://a.com/3"})
	if !ok {
		t.Fatal("expected 3rd unique URL to succeed (capacity=3)")
	}
	if f.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", f.Len())
	}
	// now at capacity — next add should fail
	ok = f.Add(CrawlURL{URL: "http://a.com/4"})
	if ok {
		t.Fatal("expected add to fail at capacity")
	}
}

func TestFrontierMaxSizeDrainAndRefill(t *testing.T) {
	f := New(0, 3)
	// fill
	for i := 0; i < 3; i++ {
		f.Add(CrawlURL{URL: fmt.Sprintf("http://a.com/%d", i)})
	}
	if f.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", f.Len())
	}
	// drain
	for f.Len() > 0 {
		next := f.Next()
		if next == nil {
			t.Fatal("Next() returned nil before queue empty")
		}
	}
	// refill with new URLs (old ones are deduped, so use new)
	for i := 10; i < 13; i++ {
		ok := f.Add(CrawlURL{URL: fmt.Sprintf("http://a.com/%d", i)})
		if !ok {
			t.Fatalf("expected refill to succeed at i=%d", i)
		}
	}
	if f.Len() != 3 {
		t.Fatalf("expected Len()=3 after refill, got %d", f.Len())
	}
}

func TestFrontierConcurrentAccess(t *testing.T) {
	f := New(0, 0)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			f.Add(CrawlURL{URL: fmt.Sprintf("http://host%d.com/page", n)})
			f.Len()
			f.Next()
			f.SeenCount()
		}(i)
	}
	wg.Wait()
}
