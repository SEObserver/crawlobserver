package frontier

import (
	"container/heap"
	"net/url"
	"sync"
	"time"
)

// CrawlURL represents a URL to be crawled with priority and metadata.
type CrawlURL struct {
	URL      string
	Priority int // lower = higher priority
	Depth    int
	FoundOn  string
	index    int // heap index
}

// Frontier manages the URL queue with priority, dedup, and per-host politeness.
type Frontier struct {
	mu        sync.Mutex
	pq        priorityQueue
	urldb     *URLDb
	hostQueue *HostQueue
	closed    bool
}

// New creates a new Frontier.
func New(delay time.Duration) *Frontier {
	f := &Frontier{
		urldb:     NewURLDb(),
		hostQueue: NewHostQueue(delay),
	}
	heap.Init(&f.pq)
	return f
}

// Add adds a URL to the frontier if it hasn't been seen before.
// Returns true if the URL was added.
func (f *Frontier) Add(crawlURL CrawlURL) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return false
	}

	if !f.urldb.Add(crawlURL.URL) {
		return false
	}

	heap.Push(&f.pq, &crawlURL)
	return true
}

// Next returns the next URL that is ready to be fetched (respecting per-host delay).
// Returns nil if no URL is ready or the frontier is empty.
func (f *Frontier) Next() *CrawlURL {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.pq.Len() == 0 {
		return nil
	}

	// Try to find a URL whose host is ready
	// Look through top items for one that can be fetched
	for i := 0; i < f.pq.Len(); i++ {
		item := f.pq[i]
		host := extractHost(item.URL)
		if f.hostQueue.CanFetch(host) {
			heap.Remove(&f.pq, i)
			f.hostQueue.RecordFetch(host)
			return item
		}
	}

	return nil
}

// Len returns the number of URLs in the queue.
func (f *Frontier) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pq.Len()
}

// SeenCount returns the total number of unique URLs seen.
func (f *Frontier) SeenCount() int {
	return f.urldb.Len()
}

// Close closes the frontier, preventing new URLs from being added.
func (f *Frontier) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

// priorityQueue implements heap.Interface for CrawlURL.
type priorityQueue []*CrawlURL

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].Priority < pq[j].Priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*CrawlURL)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[:n-1]
	return item
}
