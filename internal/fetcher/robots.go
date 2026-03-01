package fetcher

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

// RobotsCacheEntry holds the raw robots.txt data for a host.
type RobotsCacheEntry struct {
	Content    string
	StatusCode int
	FetchedAt  time.Time
	parsed     *robotstxt.RobotsData
}

// RobotsCache caches robots.txt data per host.
type RobotsCache struct {
	mu        sync.RWMutex
	cache     map[string]*RobotsCacheEntry
	client    *http.Client
	userAgent string
}

// NewRobotsCache creates a new RobotsCache.
func NewRobotsCache(userAgent string, timeout time.Duration, allowPrivateIPs bool, tlsProfile TLSProfile) *RobotsCache {
	transport := &http.Transport{
		DialContext: SafeDialContext(allowPrivateIPs),
	}
	var rt http.RoundTripper = transport
	if tlsProfile != "" {
		rt = utlsTransport(tlsProfile, SafeDialContext(allowPrivateIPs), transport)
	}
	return &RobotsCache{
		cache:     make(map[string]*RobotsCacheEntry),
		userAgent: userAgent,
		client: &http.Client{
			Timeout:   timeout,
			Transport: rt,
		},
	}
}

// IsAllowed checks if the given URL is allowed by robots.txt.
func (rc *RobotsCache) IsAllowed(targetURL string) bool {
	u, err := url.Parse(targetURL)
	if err != nil {
		return true // allow on parse error
	}

	host := u.Scheme + "://" + u.Host
	entry := rc.get(host)
	if entry == nil {
		entry = rc.fetch(host)
	}

	group := entry.parsed.FindGroup(rc.userAgent)
	return group.Test(u.Path)
}

// CrawlDelay returns the crawl-delay specified in robots.txt for the given URL's host.
// Returns 0 if no crawl-delay is specified.
func (rc *RobotsCache) CrawlDelay(targetURL string) time.Duration {
	u, err := url.Parse(targetURL)
	if err != nil {
		return 0
	}

	host := u.Scheme + "://" + u.Host
	entry := rc.get(host)
	if entry == nil {
		entry = rc.fetch(host)
	}

	group := entry.parsed.FindGroup(rc.userAgent)
	return group.CrawlDelay
}

// Entries returns a copy of all cached robots.txt entries.
func (rc *RobotsCache) Entries() map[string]*RobotsCacheEntry {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	result := make(map[string]*RobotsCacheEntry, len(rc.cache))
	for k, v := range rc.cache {
		result[k] = v
	}
	return result
}

func (rc *RobotsCache) get(host string) *RobotsCacheEntry {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.cache[host]
}

// SitemapURLs collects sitemap URLs from all cached robots.txt entries,
// plus common fallback paths (/sitemap.xml, /sitemap_index.xml).
func (rc *RobotsCache) SitemapURLs() []string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	seen := make(map[string]bool)
	var urls []string

	for host, entry := range rc.cache {
		// Add sitemaps declared in robots.txt
		if entry.parsed != nil {
			for _, s := range entry.parsed.Sitemaps {
				if !seen[s] {
					seen[s] = true
					urls = append(urls, s)
				}
			}
		}
		// Add common fallback paths
		for _, path := range []string{"/sitemap.xml", "/sitemap_index.xml"} {
			u := host + path
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
			}
		}
	}
	return urls
}

func (rc *RobotsCache) fetch(host string) *RobotsCacheEntry {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, ok := rc.cache[host]; ok {
		return entry
	}

	now := time.Now()
	entry := &RobotsCacheEntry{
		FetchedAt: now,
		parsed:    &robotstxt.RobotsData{},
	}

	robotsURL := fmt.Sprintf("%s/robots.txt", host)
	req, err := http.NewRequest("GET", robotsURL, nil)
	if err != nil {
		rc.cache[host] = entry
		return entry
	}
	req.Header.Set("User-Agent", rc.userAgent)

	resp, err := rc.client.Do(req)
	if err != nil {
		rc.cache[host] = entry
		return entry
	}
	defer resp.Body.Close()

	entry.StatusCode = resp.StatusCode

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB limit
	if err != nil || resp.StatusCode != 200 {
		rc.cache[host] = entry
		return entry
	}

	entry.Content = string(body)

	parsed, err := robotstxt.FromBytes(body)
	if err != nil {
		parsed = &robotstxt.RobotsData{}
	}
	entry.parsed = parsed

	rc.cache[host] = entry
	return entry
}
