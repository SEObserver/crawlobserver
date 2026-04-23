// Package announcements fetches an optional JSON feed of in-app messages
// from a remote URL and caches the latest payload in memory. The backend
// exposes this cache via an HTTP endpoint so the frontend can display a
// banner. Users can disable the feature entirely at any time.
package announcements

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// Message is a single announcement entry. Fields match the JSON feed schema.
type Message struct {
	ID          string `json:"id"`
	PublishedAt string `json:"published_at"`          // required, ISO-8601 UTC
	ShowUntil   string `json:"show_until,omitempty"`  // optional, ISO-8601 UTC. Message hidden after this date.
	Title       string `json:"title"`
	Body        string `json:"body"`      // plain text, may use minimal markdown (**bold**, [link](url))
	CTALabel    string `json:"cta_label"` // optional button label
	CTAURL      string `json:"cta_url"`   // optional button URL (must start with https://)
}

// Feed is the top-level feed payload.
type Feed struct {
	Messages []Message `json:"messages"`
}

// Fetcher periodically pulls a remote JSON feed and caches the latest message.
type Fetcher struct {
	feedURL  string
	interval time.Duration
	client   *http.Client

	mu         sync.RWMutex
	latest     *Message
	fetchedAt  time.Time
	lastErr    string
}

// New creates a Fetcher. A zero or negative interval defaults to 10 minutes.
func New(feedURL string, interval time.Duration) *Fetcher {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &Fetcher{
		feedURL:  feedURL,
		interval: interval,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Run performs an initial fetch and then loops on a ticker until ctx is canceled.
// Failures are logged but do not stop the loop (the feed is best-effort).
func (f *Fetcher) Run(ctx context.Context) {
	if f.feedURL == "" {
		applog.Info("announcements", "no feed_url configured, fetcher disabled")
		return
	}
	f.fetchOnce(ctx)

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.fetchOnce(ctx)
		}
	}
}

func (f *Fetcher) fetchOnce(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.feedURL, nil)
	if err != nil {
		f.setError(fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CrawlObserver/1.0 (+announcements)")

	resp, err := f.client.Do(req)
	if err != nil {
		f.setError(fmt.Sprintf("fetch: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.setError(fmt.Sprintf("unexpected status %d", resp.StatusCode))
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		f.setError(fmt.Sprintf("read body: %v", err))
		return
	}

	var feed Feed
	if err := json.Unmarshal(body, &feed); err != nil {
		f.setError(fmt.Sprintf("parse json: %v", err))
		return
	}

	var latest *Message
	if len(feed.Messages) > 0 {
		// Feed is authored with newest first; pick the first well-formed entry.
		for i := range feed.Messages {
			if feed.Messages[i].ID != "" && feed.Messages[i].Title != "" {
				latest = &feed.Messages[i]
				break
			}
		}
	}

	f.mu.Lock()
	f.latest = latest
	f.fetchedAt = time.Now()
	f.lastErr = ""
	f.mu.Unlock()
}

func (f *Fetcher) setError(msg string) {
	applog.Warnf("announcements", "feed fetch failed: %s", msg)
	f.mu.Lock()
	f.lastErr = msg
	f.mu.Unlock()
}

// Snapshot returns the currently cached message (or nil if none) plus
// the time of the last successful fetch.
func (f *Fetcher) Snapshot() (*Message, time.Time) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.latest, f.fetchedAt
}
