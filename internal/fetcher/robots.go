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

// RobotsCache caches robots.txt data per host.
type RobotsCache struct {
	mu        sync.RWMutex
	cache     map[string]*robotstxt.RobotsData
	client    *http.Client
	userAgent string
}

// NewRobotsCache creates a new RobotsCache.
func NewRobotsCache(userAgent string, timeout time.Duration) *RobotsCache {
	return &RobotsCache{
		cache:     make(map[string]*robotstxt.RobotsData),
		userAgent: userAgent,
		client: &http.Client{
			Timeout: timeout,
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
	robots := rc.get(host)
	if robots == nil {
		robots = rc.fetch(host)
	}

	group := robots.FindGroup(rc.userAgent)
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
	robots := rc.get(host)
	if robots == nil {
		robots = rc.fetch(host)
	}

	group := robots.FindGroup(rc.userAgent)
	return group.CrawlDelay
}

func (rc *RobotsCache) get(host string) *robotstxt.RobotsData {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.cache[host]
}

func (rc *RobotsCache) fetch(host string) *robotstxt.RobotsData {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Double-check after acquiring write lock
	if robots, ok := rc.cache[host]; ok {
		return robots
	}

	robotsURL := fmt.Sprintf("%s/robots.txt", host)
	req, err := http.NewRequest("GET", robotsURL, nil)
	if err != nil {
		robots := &robotstxt.RobotsData{}
		rc.cache[host] = robots
		return robots
	}
	req.Header.Set("User-Agent", rc.userAgent)

	resp, err := rc.client.Do(req)
	if err != nil {
		// On error, allow everything
		robots := &robotstxt.RobotsData{}
		rc.cache[host] = robots
		return robots
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB limit
	if err != nil || resp.StatusCode != 200 {
		robots := &robotstxt.RobotsData{}
		rc.cache[host] = robots
		return robots
	}

	robots, err := robotstxt.FromBytes(body)
	if err != nil {
		robots = &robotstxt.RobotsData{}
	}

	rc.cache[host] = robots
	return robots
}
