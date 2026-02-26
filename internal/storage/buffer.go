package storage

import (
	"context"
	"log"
	"sync"
	"time"
)

// Buffer accumulates rows and flushes them in batches.
type Buffer struct {
	store         *Store
	batchSize     int
	flushInterval time.Duration
	sessionID     string

	mu    sync.Mutex
	pages []PageRow
	links []LinkRow

	done chan struct{}
	wg   sync.WaitGroup
}

// NewBuffer creates a new write buffer.
func NewBuffer(store *Store, batchSize int, flushInterval time.Duration, sessionID string) *Buffer {
	b := &Buffer{
		store:         store,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		sessionID:     sessionID,
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

// Flush writes all buffered data to ClickHouse.
func (b *Buffer) Flush() {
	b.mu.Lock()
	pages := b.pages
	links := b.links
	b.pages = nil
	b.links = nil
	b.mu.Unlock()

	ctx := context.Background()

	if len(pages) > 0 {
		if err := b.store.InsertPages(ctx, pages); err != nil {
			log.Printf("ERROR flushing pages: %v", err)
		}
	}

	if len(links) > 0 {
		if err := b.store.InsertLinks(ctx, links); err != nil {
			log.Printf("ERROR flushing links: %v", err)
		}
	}
}

// Close flushes remaining data and stops the flush loop.
func (b *Buffer) Close() {
	close(b.done)
	b.wg.Wait()
	b.Flush() // final flush
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
