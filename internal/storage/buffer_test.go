package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockInserter implements PageLinkInserter with configurable failure behavior.
type mockInserter struct {
	mu            sync.Mutex
	pageInserts   int
	linkInserts   int
	pagesInserted []PageRow
	linksInserted []LinkRow
	pageErr       error // if set, InsertPages returns this error
	linkErr       error // if set, InsertLinks returns this error
	failCount     int   // number of times to fail, then succeed (0 = always use pageErr/linkErr)
	callCount     int   // internal counter for failCount logic
}

func (m *mockInserter) InsertPages(ctx context.Context, pages []PageRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.pageErr != nil {
		if m.failCount == 0 || m.callCount <= m.failCount {
			return m.pageErr
		}
	}
	m.pageInserts++
	m.pagesInserted = append(m.pagesInserted, pages...)
	return nil
}

func (m *mockInserter) InsertLinks(ctx context.Context, links []LinkRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.linkErr != nil {
		return m.linkErr
	}
	m.linkInserts++
	m.linksInserted = append(m.linksInserted, links...)
	return nil
}

func newTestBuffer(store PageLinkInserter) *Buffer {
	return &Buffer{
		store:         store,
		batchSize:     100,
		flushInterval: 1 * time.Hour,
		sessionID:     "test",
		maxRetries:    3,
		done:          make(chan struct{}),
	}
}

func TestBufferPageCount(t *testing.T) {
	b := newTestBuffer(&mockInserter{})

	if b.PageCount() != 0 {
		t.Errorf("initial PageCount = %d, want 0", b.PageCount())
	}

	b.AddPage(PageRow{URL: "https://example.com/1"})
	b.AddPage(PageRow{URL: "https://example.com/2"})

	if b.PageCount() != 2 {
		t.Errorf("PageCount = %d, want 2", b.PageCount())
	}
}

func TestBufferFlushSuccess(t *testing.T) {
	m := &mockInserter{}
	b := newTestBuffer(m)

	b.AddPage(PageRow{URL: "a"})
	b.AddPage(PageRow{URL: "b"})
	b.AddLinks([]LinkRow{{SourceURL: "a", TargetURL: "b"}})

	b.Flush()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pageInserts != 1 {
		t.Errorf("pageInserts = %d, want 1", m.pageInserts)
	}
	if len(m.pagesInserted) != 2 {
		t.Errorf("pagesInserted = %d, want 2", len(m.pagesInserted))
	}
	if m.linkInserts != 1 {
		t.Errorf("linkInserts = %d, want 1", m.linkInserts)
	}

	if b.PageCount() != 0 {
		t.Errorf("PageCount after flush = %d, want 0", b.PageCount())
	}
}

func TestFlushRetryOnFailure(t *testing.T) {
	m := &mockInserter{
		pageErr:   fmt.Errorf("connection refused"),
		failCount: 1, // fail first call, then succeed
	}
	b := newTestBuffer(m)

	b.AddPage(PageRow{URL: "a"})
	b.AddPage(PageRow{URL: "b"})

	// First flush: fails, pages go to failedPages
	b.Flush()

	state := b.ErrorState()
	if state.PendingPages != 2 {
		t.Errorf("PendingPages = %d, want 2", state.PendingPages)
	}
	if state.LostPages != 0 {
		t.Errorf("LostPages = %d, want 0", state.LostPages)
	}

	// Second flush: retry succeeds
	b.Flush()

	m.mu.Lock()
	if m.pageInserts != 1 {
		t.Errorf("pageInserts = %d, want 1", m.pageInserts)
	}
	if len(m.pagesInserted) != 2 {
		t.Errorf("pagesInserted = %d, want 2", len(m.pagesInserted))
	}
	m.mu.Unlock()

	state = b.ErrorState()
	if state.PendingPages != 0 {
		t.Errorf("PendingPages after retry = %d, want 0", state.PendingPages)
	}
	if state.LostPages != 0 {
		t.Errorf("LostPages = %d, want 0", state.LostPages)
	}
}

func TestFlushDropAfterMaxRetries(t *testing.T) {
	m := &mockInserter{
		pageErr: fmt.Errorf("permanent failure"),
	}
	b := newTestBuffer(m)
	b.maxRetries = 3

	b.AddPage(PageRow{URL: "a"})
	b.AddPage(PageRow{URL: "b"})
	b.AddPage(PageRow{URL: "c"})

	// Flush 1: initial failure -> retry batch created (retries=0)
	b.Flush()
	state := b.ErrorState()
	if state.PendingPages != 3 {
		t.Fatalf("after flush 1: PendingPages = %d, want 3", state.PendingPages)
	}

	// Flush 2: retry fails -> retries=1
	b.Flush()
	state = b.ErrorState()
	if state.PendingPages != 3 {
		t.Fatalf("after flush 2: PendingPages = %d, want 3", state.PendingPages)
	}

	// Flush 3: retry fails -> retries=2
	b.Flush()
	state = b.ErrorState()
	if state.PendingPages != 3 {
		t.Fatalf("after flush 3: PendingPages = %d, want 3", state.PendingPages)
	}

	// Flush 4: retry fails -> retries=3 >= maxRetries -> dropped
	b.Flush()
	state = b.ErrorState()
	if state.PendingPages != 0 {
		t.Errorf("after max retries: PendingPages = %d, want 0", state.PendingPages)
	}
	if state.LostPages != 3 {
		t.Errorf("LostPages = %d, want 3", state.LostPages)
	}
	if state.LastError == nil {
		t.Error("LastError should be non-nil")
	}
}

func TestErrorStateReporting(t *testing.T) {
	m := &mockInserter{
		pageErr: fmt.Errorf("fail pages"),
		linkErr: fmt.Errorf("fail links"),
	}
	b := newTestBuffer(m)

	b.AddPage(PageRow{URL: "a"})
	b.AddLinks([]LinkRow{{SourceURL: "a", TargetURL: "b"}, {SourceURL: "a", TargetURL: "c"}})

	b.Flush()

	state := b.ErrorState()
	if state.PendingPages != 1 {
		t.Errorf("PendingPages = %d, want 1", state.PendingPages)
	}
	if state.PendingLinks != 2 {
		t.Errorf("PendingLinks = %d, want 2", state.PendingLinks)
	}
	if state.LastError == nil {
		t.Error("LastError should be set")
	}
	if state.LostPages != 0 {
		t.Errorf("LostPages = %d, want 0", state.LostPages)
	}
	if state.LostLinks != 0 {
		t.Errorf("LostLinks = %d, want 0", state.LostLinks)
	}
}

func TestOnDataLostCallbackPages(t *testing.T) {
	m := &mockInserter{
		pageErr: fmt.Errorf("disk full"),
	}
	b := newTestBuffer(m)
	b.maxRetries = 3

	var callbackPages, callbackLinks int64
	var callbackCount int
	b.SetOnDataLost(func(lostPages, lostLinks int64) {
		callbackCount++
		callbackPages = lostPages
		callbackLinks = lostLinks
	})

	b.AddPage(PageRow{URL: "a"})
	b.AddPage(PageRow{URL: "b"})

	// Flush 1-3: retries accumulate, no drop yet
	b.Flush()
	b.Flush()
	b.Flush()
	if callbackCount != 0 {
		t.Fatalf("callback called %d times during retries, want 0", callbackCount)
	}

	// Flush 4: retries=3 >= maxRetries -> drop -> callback
	b.Flush()
	if callbackCount != 1 {
		t.Fatalf("callback called %d times after drop, want 1", callbackCount)
	}
	if callbackPages != 2 {
		t.Errorf("callbackPages = %d, want 2", callbackPages)
	}
	if callbackLinks != 0 {
		t.Errorf("callbackLinks = %d, want 0", callbackLinks)
	}
}

func TestOnDataLostCallbackLinks(t *testing.T) {
	m := &mockInserter{
		linkErr: fmt.Errorf("disk full"),
	}
	b := newTestBuffer(m)
	b.maxRetries = 1 // drop after first retry

	var callbackPages, callbackLinks int64
	b.SetOnDataLost(func(lostPages, lostLinks int64) {
		callbackPages = lostPages
		callbackLinks = lostLinks
	})

	b.AddLinks([]LinkRow{{SourceURL: "a", TargetURL: "b"}, {SourceURL: "a", TargetURL: "c"}, {SourceURL: "a", TargetURL: "d"}})

	// Flush 1: initial failure -> retry batch
	b.Flush()
	// Flush 2: retries=1 >= maxRetries -> drop -> callback
	b.Flush()

	if callbackPages != 0 {
		t.Errorf("callbackPages = %d, want 0", callbackPages)
	}
	if callbackLinks != 3 {
		t.Errorf("callbackLinks = %d, want 3", callbackLinks)
	}
}

func TestOnDataLostNotCalledWithoutCallback(t *testing.T) {
	m := &mockInserter{
		pageErr: fmt.Errorf("disk full"),
	}
	b := newTestBuffer(m)
	b.maxRetries = 1

	// No SetOnDataLost called — should not panic
	b.AddPage(PageRow{URL: "a"})
	b.Flush()
	b.Flush() // drop happens here, no callback set

	state := b.ErrorState()
	if state.LostPages != 1 {
		t.Errorf("LostPages = %d, want 1", state.LostPages)
	}
}

func TestCloseRetriesFailedBatches(t *testing.T) {
	m := &mockInserter{
		pageErr:   fmt.Errorf("temporary failure"),
		failCount: 1, // fail first call, then succeed
	}
	b := newTestBuffer(m)

	b.AddPage(PageRow{URL: "a"})

	// Flush fails
	b.Flush()
	state := b.ErrorState()
	if state.PendingPages != 1 {
		t.Fatalf("PendingPages = %d, want 1", state.PendingPages)
	}

	// Close should retry failed batches
	// Note: Close calls close(done) + wg.Wait() + Flush()
	// But since we didn't start flushLoop, we just call Close pattern manually
	b.Flush() // simulate close's final flush — retry succeeds

	m.mu.Lock()
	if m.pageInserts != 1 {
		t.Errorf("pageInserts = %d, want 1 (retry should have succeeded)", m.pageInserts)
	}
	m.mu.Unlock()

	state = b.ErrorState()
	if state.PendingPages != 0 {
		t.Errorf("PendingPages after close = %d, want 0", state.PendingPages)
	}
}
