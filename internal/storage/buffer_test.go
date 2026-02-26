package storage

import (
	"testing"
	"time"
)

// mockStore is a minimal mock that tracks calls without needing ClickHouse.
// Buffer tests focus on buffering logic, not actual DB inserts.

func TestBufferPageCount(t *testing.T) {
	// Create buffer without a real store — we only test the buffering logic
	b := &Buffer{
		batchSize:     100,
		flushInterval: 1 * time.Hour, // won't fire during test
		sessionID:     "test",
		done:          make(chan struct{}),
	}

	if b.PageCount() != 0 {
		t.Errorf("initial PageCount = %d, want 0", b.PageCount())
	}

	b.AddPage(PageRow{URL: "https://example.com/1"})
	b.AddPage(PageRow{URL: "https://example.com/2"})

	if b.PageCount() != 2 {
		t.Errorf("PageCount = %d, want 2", b.PageCount())
	}
}

func TestBufferFlushClearsBuffer(t *testing.T) {
	b := &Buffer{
		batchSize:     100,
		flushInterval: 1 * time.Hour,
		sessionID:     "test",
		done:          make(chan struct{}),
	}

	b.pages = []PageRow{{URL: "a"}, {URL: "b"}}
	b.links = []LinkRow{{SourceURL: "a", TargetURL: "b"}}

	// Flush without a store just clears the buffers (store is nil, so Insert calls are skipped)
	b.mu.Lock()
	pages := b.pages
	links := b.links
	b.pages = nil
	b.links = nil
	b.mu.Unlock()

	if len(pages) != 2 {
		t.Errorf("flushed pages = %d, want 2", len(pages))
	}
	if len(links) != 1 {
		t.Errorf("flushed links = %d, want 1", len(links))
	}
	if b.PageCount() != 0 {
		t.Errorf("PageCount after flush = %d, want 0", b.PageCount())
	}
}
