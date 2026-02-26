package frontier

import (
	"hash/fnv"
	"sync"
)

// URLDb tracks seen URLs using FNV-1a hashes for memory efficiency.
type URLDb struct {
	mu   sync.RWMutex
	seen map[uint64]struct{}
}

// NewURLDb creates a new URL deduplication database.
func NewURLDb() *URLDb {
	return &URLDb{
		seen: make(map[uint64]struct{}),
	}
}

// Add marks a URL as seen. Returns true if the URL was new.
func (db *URLDb) Add(url string) bool {
	h := hash(url)
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, exists := db.seen[h]; exists {
		return false
	}
	db.seen[h] = struct{}{}
	return true
}

// Has checks if a URL has been seen.
func (db *URLDb) Has(url string) bool {
	h := hash(url)
	db.mu.RLock()
	defer db.mu.RUnlock()
	_, exists := db.seen[h]
	return exists
}

// Len returns the number of seen URLs.
func (db *URLDb) Len() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.seen)
}

func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
