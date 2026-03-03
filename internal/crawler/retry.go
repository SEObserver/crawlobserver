package crawler

import (
	"container/heap"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RetryPolicy decides whether a failed request should be retried and computes delays.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// retryableCodes is the set of HTTP status codes eligible for retry.
var retryableCodes = map[int]bool{
	429: true,
	500: true,
	502: true,
	503: true,
	504: true,
}

// ShouldRetry returns true if the request should be retried based on status code,
// error string, and current attempt number.
func (p *RetryPolicy) ShouldRetry(statusCode int, errString string, attempt int) bool {
	if p.MaxRetries <= 0 || attempt >= p.MaxRetries {
		return false
	}

	// HTTP status code based retry
	if retryableCodes[statusCode] {
		return true
	}

	// Network error based retry (status_code=0)
	if errString != "" {
		lower := strings.ToLower(errString)
		// Non-retryable errors
		if strings.Contains(lower, "dns_not_found") || strings.Contains(lower, "tls_error") {
			return false
		}
		// DNS timeout: max 1 retry
		if strings.Contains(lower, "dns_timeout") {
			return attempt < 1
		}
		// Retryable network errors
		if strings.Contains(lower, "timeout") ||
			strings.Contains(lower, "connection_refused") ||
			strings.Contains(lower, "connection refused") ||
			strings.Contains(lower, "connection reset") ||
			strings.Contains(lower, "eof") {
			return true
		}
	}

	return false
}

// ComputeDelay calculates the delay before the next retry attempt.
// If a Retry-After header is present, it takes priority.
func (p *RetryPolicy) ComputeDelay(attempt int, retryAfterHeader string) time.Duration {
	// Check Retry-After header first
	if retryAfterHeader != "" {
		if d := parseRetryAfter(retryAfterHeader); d > 0 {
			if d > p.MaxDelay {
				return p.MaxDelay
			}
			return d
		}
	}

	// Exponential backoff: base * 2^(attempt-1)
	exp := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(p.BaseDelay) * exp)

	// Cap at max delay
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}

	// Add jitter: 0 to delay/2
	jitter := time.Duration(rand.Int63n(int64(delay/2) + 1))
	delay += jitter

	// Re-cap after jitter
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}

	return delay
}

// parseRetryAfter parses a Retry-After header value as either seconds or HTTP-date.
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)

	// Try as seconds
	if secs, err := strconv.Atoi(value); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}

	// Try as HTTP-date (RFC 7231)
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}

	return 0
}

// RetryItem represents a URL waiting to be retried.
type RetryItem struct {
	URL      string
	Host     string
	Depth    int
	FoundOn  string
	Attempt  int
	ReadyAt  time.Time
	LastCode int
	LastErr  string
	index    int // heap index
}

// retryHeap implements heap.Interface ordered by ReadyAt (earliest first).
type retryHeap []*RetryItem

func (h retryHeap) Len() int           { return len(h) }
func (h retryHeap) Less(i, j int) bool { return h[i].ReadyAt.Before(h[j].ReadyAt) }
func (h retryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *retryHeap) Push(x interface{}) {
	item := x.(*RetryItem)
	item.index = len(*h)
	*h = append(*h, item)
}
func (h *retryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[:n-1]
	return item
}

// RetryQueue is a thread-safe min-heap of RetryItems ordered by ReadyAt.
type RetryQueue struct {
	mu   sync.Mutex
	heap retryHeap
}

// NewRetryQueue creates a new empty RetryQueue.
func NewRetryQueue() *RetryQueue {
	rq := &RetryQueue{}
	heap.Init(&rq.heap)
	return rq
}

// Push adds an item to the retry queue.
func (rq *RetryQueue) Push(item *RetryItem) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	heap.Push(&rq.heap, item)
}

// PopReady returns the next item whose ReadyAt is in the past, or nil if none are ready.
func (rq *RetryQueue) PopReady() *RetryItem {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if rq.heap.Len() == 0 {
		return nil
	}

	// Peek at the earliest item
	if rq.heap[0].ReadyAt.After(time.Now()) {
		return nil
	}

	return heap.Pop(&rq.heap).(*RetryItem)
}

// Len returns the number of items in the queue.
func (rq *RetryQueue) Len() int {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	return rq.heap.Len()
}

// HostHealth tracks success/failure rates per host.
type HostHealth struct {
	mu    sync.Mutex
	hosts map[string]*hostStats
}

type hostStats struct {
	successes           int64
	failures            int64
	consecutiveFailures int
}

// NewHostHealth creates a new HostHealth tracker.
func NewHostHealth() *HostHealth {
	return &HostHealth{
		hosts: make(map[string]*hostStats),
	}
}

func (hh *HostHealth) getOrCreate(host string) *hostStats {
	s, ok := hh.hosts[host]
	if !ok {
		s = &hostStats{}
		hh.hosts[host] = s
	}
	return s
}

// RecordSuccess records a successful fetch for a host.
func (hh *HostHealth) RecordSuccess(host string) {
	hh.mu.Lock()
	defer hh.mu.Unlock()
	s := hh.getOrCreate(host)
	s.successes++
	s.consecutiveFailures = 0
}

// RecordFailure records a failed fetch for a host.
func (hh *HostHealth) RecordFailure(host string) {
	hh.mu.Lock()
	defer hh.mu.Unlock()
	s := hh.getOrCreate(host)
	s.failures++
	s.consecutiveFailures++
}

// ShouldRetry returns false if the host has exceeded the consecutive failure threshold.
func (hh *HostHealth) ShouldRetry(host string, maxConsecutiveFails int) bool {
	hh.mu.Lock()
	defer hh.mu.Unlock()
	s, ok := hh.hosts[host]
	if !ok {
		return true
	}
	return s.consecutiveFailures < maxConsecutiveFails
}

// GlobalErrorRate returns the global error rate across all hosts.
func (hh *HostHealth) GlobalErrorRate() float64 {
	hh.mu.Lock()
	defer hh.mu.Unlock()

	var totalSuccess, totalFailure int64
	for _, s := range hh.hosts {
		totalSuccess += s.successes
		totalFailure += s.failures
	}

	total := totalSuccess + totalFailure
	if total == 0 {
		return 0
	}
	return float64(totalFailure) / float64(total)
}
