package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/extraction"
	"github.com/SEObserver/crawlobserver/internal/schema"
)

// PageLinkInserter is the subset of Store used by Buffer for flushing data.
type PageLinkInserter interface {
	InsertPages(ctx context.Context, pages []PageRow) error
	InsertLinks(ctx context.Context, links []LinkRow) error
	InsertExtractions(ctx context.Context, rows []extraction.ExtractionRow) error
	InsertStructuredData(ctx context.Context, items []schema.StructuredDataItem) error
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

	mu              sync.Mutex
	pages           []PageRow
	links           []LinkRow
	extractions     []extraction.ExtractionRow
	structuredData       []schema.StructuredDataItem
	failedPages          []retryBatch[PageRow]
	failedLinks          []retryBatch[LinkRow]
	failedStructuredData []retryBatch[schema.StructuredDataItem]
	lostPages   int64
	lostLinks   int64
	lastError   error

	done chan struct{}
	wg   sync.WaitGroup

	// Atomic counter for lost data (safe to read without lock)
	lostPagesAtomic atomic.Int64
	lostLinksAtomic atomic.Int64

	onDataLost func(lostPages, lostLinks int64)
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

// AddExtractions adds extraction rows to the buffer.
func (b *Buffer) AddExtractions(rows []extraction.ExtractionRow) {
	if len(rows) == 0 {
		return
	}
	b.mu.Lock()
	b.extractions = append(b.extractions, rows...)
	shouldFlush := len(b.extractions) >= b.batchSize
	b.mu.Unlock()

	if shouldFlush {
		b.Flush()
	}
}

// AddStructuredData adds structured data items to the buffer.
func (b *Buffer) AddStructuredData(items []schema.StructuredDataItem) {
	if len(items) == 0 {
		return
	}
	b.mu.Lock()
	b.structuredData = append(b.structuredData, items...)
	shouldFlush := len(b.structuredData) >= b.batchSize
	b.mu.Unlock()

	if shouldFlush {
		b.Flush()
	}
}

// SetOnDataLost registers a callback invoked (outside the lock) whenever data is dropped.
func (b *Buffer) SetOnDataLost(fn func(lostPages, lostLinks int64)) {
	b.mu.Lock()
	b.onDataLost = fn
	b.mu.Unlock()
}

// Flush writes all buffered data to ClickHouse, retrying previously failed batches first.
func (b *Buffer) Flush() {
	b.mu.Lock()
	pages := b.pages
	links := b.links
	extractions := b.extractions
	structuredData := b.structuredData
	b.pages = nil
	b.links = nil
	b.extractions = nil
	b.structuredData = nil

	// Grab failed batches to retry
	failedPages := b.failedPages
	failedLinks := b.failedLinks
	failedSD := b.failedStructuredData
	b.failedPages = nil
	b.failedLinks = nil
	b.failedStructuredData = nil
	b.mu.Unlock()

	ctx := context.Background()

	// Retry previously failed page batches
	dataLost := false
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
				dataLost = true
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
				dataLost = true
			} else {
				b.mu.Lock()
				b.failedLinks = append(b.failedLinks, batch)
				b.lastError = err
				b.mu.Unlock()
			}
		}
	}

	// Retry previously failed structured data batches
	for _, batch := range failedSD {
		if err := b.store.InsertStructuredData(ctx, batch.data); err != nil {
			batch.retries++
			if batch.retries >= b.maxRetries {
				applog.Errorf("storage", "[%s] dropping %d structured data items after %d retries: %v", b.sessionID, len(batch.data), batch.retries, err)
				b.mu.Lock()
				b.lastError = err
				b.mu.Unlock()
			} else {
				b.mu.Lock()
				b.failedStructuredData = append(b.failedStructuredData, batch)
				b.lastError = err
				b.mu.Unlock()
			}
		}
	}

	// Notify data loss callback (outside lock)
	if dataLost {
		b.mu.Lock()
		cb := b.onDataLost
		b.mu.Unlock()
		if cb != nil {
			cb(b.lostPagesAtomic.Load(), b.lostLinksAtomic.Load())
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

	// Flush current extractions
	if len(extractions) > 0 {
		if err := b.store.InsertExtractions(ctx, extractions); err != nil {
			applog.Errorf("storage", "[%s] flushing %d extractions: %v", b.sessionID, len(extractions), err)
			b.mu.Lock()
			b.lastError = err
			b.mu.Unlock()
		}
	}

	// Flush current structured data
	if len(structuredData) > 0 {
		if err := b.store.InsertStructuredData(ctx, structuredData); err != nil {
			applog.Errorf("storage", "[%s] flushing %d structured data items (will retry): %v", b.sessionID, len(structuredData), err)
			b.mu.Lock()
			b.failedStructuredData = append(b.failedStructuredData, retryBatch[schema.StructuredDataItem]{data: structuredData, retries: 0})
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
