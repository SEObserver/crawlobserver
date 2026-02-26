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
	index    int    // heap index
	host     string // cached host, set by Frontier.Add
}

// Frontier manages the URL queue with priority, dedup, and per-host politeness.
type Frontier struct {
	mu        sync.Mutex
	pq        priorityQueue
	urldb     *URLDb
	hostQueue *HostQueue
	hostCount map[string]int // number of URLs per host in the queue
	minDepth  map[uint64]int // minimum depth seen per URL (keyed by hash)
	bestFound map[uint64]string // best foundOn per URL (the one with min depth)
	closed    bool
}

// New creates a new Frontier.
func New(delay time.Duration) *Frontier {
	f := &Frontier{
		urldb:     NewURLDb(),
		hostQueue: NewHostQueue(delay),
		hostCount: make(map[string]int),
		minDepth:  make(map[uint64]int),
		bestFound: make(map[uint64]string),
	}
	heap.Init(&f.pq)
	return f
}

// Add adds a URL to the frontier if it hasn't been seen before.
// Returns true if the URL was added. Even if already seen, updates the
// minimum depth tracking so that dequeued URLs get their true shortest-path depth.
func (f *Frontier) Add(crawlURL CrawlURL) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return false
	}

	h := hash(crawlURL.URL)

	if !f.urldb.Add(crawlURL.URL) {
		// URL already seen — still update min depth if this path is shorter
		if d, ok := f.minDepth[h]; ok && crawlURL.Depth < d {
			f.minDepth[h] = crawlURL.Depth
			f.bestFound[h] = crawlURL.FoundOn
		}
		return false
	}

	// New URL — record depth and add to queue
	f.minDepth[h] = crawlURL.Depth
	f.bestFound[h] = crawlURL.FoundOn
	crawlURL.host = extractHost(crawlURL.URL)
	f.hostCount[crawlURL.host]++
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

	// Check which hosts in the queue are ready — O(distinct hosts) instead of O(queue size)
	readySet := make(map[string]bool, len(f.hostCount))
	for host := range f.hostCount {
		if f.hostQueue.CanFetch(host) {
			readySet[host] = true
		}
	}
	if len(readySet) == 0 {
		return nil
	}

	// Find the highest-priority URL from a ready host
	for i := 0; i < f.pq.Len(); i++ {
		if readySet[f.pq[i].host] {
			item := f.pq[i]
			heap.Remove(&f.pq, i)
			f.hostQueue.RecordFetch(item.host)
			f.hostCount[item.host]--
			if f.hostCount[item.host] == 0 {
				delete(f.hostCount, item.host)
			}
			// Override depth with the minimum seen across all discovery paths
			h := hash(item.URL)
			if d, ok := f.minDepth[h]; ok && d < item.Depth {
				item.Depth = d
			}
			if fo, ok := f.bestFound[h]; ok && fo != "" {
				item.FoundOn = fo
			}
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

// MarkSeen adds a URL to the dedup database without adding it to the queue.
func (f *Frontier) MarkSeen(url string) {
	f.urldb.Add(url)
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
