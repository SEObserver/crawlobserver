package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SEObserver/seocrawler/internal/applog"
)

// PageLinkInserter is the subset of Store used by Buffer for flushing data.
type PageLinkInserter interface {
	InsertPages(ctx context.Context, pages []PageRow) error
	InsertLinks(ctx context.Context, links []LinkRow) error
}

type retryBatch[T any] struct {
	data    []T
	retries int
}

// BufferErrorState exposes retry/loss counters for monitoring.
type BufferErrorState struct {
	LostPages    int64 `json:"lost_pages"`
	LostLinks    int64 `json:"lost_links"`
	PendingPages int   `json:"pending_retry_pages"`
	PendingLinks int   `json:"pending_retry_links"`
	LastError    error `json:"last_error,omitempty"`
}

// Buffer accumulates rows and flushes them in batches.
type Buffer struct {
	store         PageLinkInserter
	batchSize     int
	flushInterval time.Duration
	sessionID     string
	maxRetries    int

	mu          sync.Mutex
	pages       []PageRow
	links       []LinkRow
	failedPages []retryBatch[PageRow]
	failedLinks []retryBatch[LinkRow]
	lostPages   int64
	lostLinks   int64
	lastError   error

	done chan struct{}
	wg   sync.WaitGroup

	// Atomic counter for lost data (safe to read without lock)
	lostPagesAtomic atomic.Int64
	lostLinksAtomic atomic.Int64
}

// NewBuffer creates a new write buffer.
func NewBuffer(store PageLinkInserter, batchSize int, flushInterval time.Duration, sessionID string) *Buffer {
	b := &Buffer{
		store:         store,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		sessionID:     sessionID,
		maxRetries:    3,
		done:          make(chan struct{}),
	}
	b.wg.Add(1)
	go b.flushLoop()
	return b
}

// AddPage adds a page row to the buffer.
func (b *Buffer) AddPage(page PageRow) {
	b.mu.Lock()
	b.pages = append(b.pages, page)
	shouldFlush := len(b.pages) >= b.batchSize
	b.mu.Unlock()

	if shouldFlush {
		b.Flush()
	}
}

// AddLinks adds link rows to the buffer.
func (b *Buffer) AddLinks(links []LinkRow) {
	b.mu.Lock()
	b.links = append(b.links, links...)
	shouldFlush := len(b.links) >= b.batchSize
	b.mu.Unlock()

	if shouldFlush {
		b.Flush()
	}
}

// Flush writes all buffered data to ClickHouse, retrying previously failed batches first.
func (b *Buffer) Flush() {
	b.mu.Lock()
	pages := b.pages
	links := b.links
	b.pages = nil
	b.links = nil

	// Grab failed batches to retry
	failedPages := b.failedPages
	failedLinks := b.failedLinks
	b.failedPages = nil
	b.failedLinks = nil
	b.mu.Unlock()

	ctx := context.Background()

	// Retry previously failed page batches
	for _, batch := range failedPages {
		if err := b.store.InsertPages(ctx, batch.data); err != nil {
			batch.retries++
			if batch.retries >= b.maxRetries {
				lost := int64(len(batch.data))
				applog.Errorf("storage", "[%s] dropping %d pages after %d retries: %v", b.sessionID, len(batch.data), batch.retries, err)
				b.mu.Lock()
				b.lostPages += lost
				b.lastError = err
				b.mu.Unlock()
				b.lostPagesAtomic.Add(lost)
			} else {
				b.mu.Lock()
				b.failedPages = append(b.failedPages, batch)
				b.lastError = err
				b.mu.Unlock()
			}
		}
	}

	// Retry previously failed link batches
	for _, batch := range failedLinks {
		if err := b.store.InsertLinks(ctx, batch.data); err != nil {
			batch.retries++
			if batch.retries >= b.maxRetries {
				lost := int64(len(batch.data))
				applog.Errorf("storage", "[%s] dropping %d links after %d retries: %v", b.sessionID, len(batch.data), batch.retries, err)
				b.mu.Lock()
				b.lostLinks += lost
				b.lastError = err
				b.mu.Unlock()
				b.lostLinksAtomic.Add(lost)
			} else {
				b.mu.Lock()
				b.failedLinks = append(b.failedLinks, batch)
				b.lastError = err
				b.mu.Unlock()
			}
		}
	}

	// Flush current pages
	if len(pages) > 0 {
		if err := b.store.InsertPages(ctx, pages); err != nil {
			applog.Errorf("storage", "[%s] flushing %d pages (will retry): %v", b.sessionID, len(pages), err)
			b.mu.Lock()
			b.failedPages = append(b.failedPages, retryBatch[PageRow]{data: pages, retries: 0})
			b.lastError = err
			b.mu.Unlock()
		}
	}

	// Flush current links
	if len(links) > 0 {
		if err := b.store.InsertLinks(ctx, links); err != nil {
			applog.Errorf("storage", "[%s] flushing %d links (will retry): %v", b.sessionID, len(links), err)
			b.mu.Lock()
			b.failedLinks = append(b.failedLinks, retryBatch[LinkRow]{data: links, retries: 0})
			b.lastError = err
			b.mu.Unlock()
		}
	}
}

// Close flushes remaining data and stops the flush loop.
func (b *Buffer) Close() {
	close(b.done)
	b.wg.Wait()
	b.Flush() // final flush (includes retry of failed batches)
}

func (b *Buffer) flushLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.Flush()
		case <-b.done:
			return
		}
	}
}

// PageCount returns the number of buffered pages.
func (b *Buffer) PageCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pages)
}

// ErrorState returns the current error state of the buffer for monitoring.
func (b *Buffer) ErrorState() BufferErrorState {
	b.mu.Lock()
	defer b.mu.Unlock()

	pendingPages := 0
	for _, batch := range b.failedPages {
		pendingPages += len(batch.data)
	}
	pendingLinks := 0
	for _, batch := range b.failedLinks {
		pendingLinks += len(batch.data)
	}

	return BufferErrorState{
		LostPages:    b.lostPages,
		LostLinks:    b.lostLinks,
		PendingPages: pendingPages,
		PendingLinks: pendingLinks,
		LastError:    b.lastError,
	}
}
