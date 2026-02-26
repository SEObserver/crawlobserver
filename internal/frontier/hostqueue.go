package frontier

import (
	"sync"
	"time"
)

// HostQueue manages per-host politeness delays.
type HostQueue struct {
	mu        sync.Mutex
	lastFetch map[string]time.Time
	delay     time.Duration
}

// NewHostQueue creates a new HostQueue with the given default delay.
func NewHostQueue(delay time.Duration) *HostQueue {
	return &HostQueue{
		lastFetch: make(map[string]time.Time),
		delay:     delay,
	}
}

// CanFetch returns true if enough time has passed since the last fetch to this host.
func (hq *HostQueue) CanFetch(host string) bool {
	hq.mu.Lock()
	defer hq.mu.Unlock()
	last, ok := hq.lastFetch[host]
	if !ok {
		return true
	}
	return time.Since(last) >= hq.delay
}

// RecordFetch records that a fetch was made to this host.
func (hq *HostQueue) RecordFetch(host string) {
	hq.mu.Lock()
	defer hq.mu.Unlock()
	hq.lastFetch[host] = time.Now()
}

// TimeUntilReady returns how long to wait before the host can be fetched again.
func (hq *HostQueue) TimeUntilReady(host string) time.Duration {
	hq.mu.Lock()
	defer hq.mu.Unlock()
	last, ok := hq.lastFetch[host]
	if !ok {
		return 0
	}
	remaining := hq.delay - time.Since(last)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// SetDelay updates the delay for a specific host (e.g., from robots.txt crawl-delay).
func (hq *HostQueue) SetDelay(delay time.Duration) {
	hq.mu.Lock()
	defer hq.mu.Unlock()
	hq.delay = delay
}
