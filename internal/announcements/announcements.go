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
// Content is localized via the Translations map; the frontend picks the
// translation matching the user's current locale, falling back to
// DefaultLocale or the first available entry.
//
// Legacy flat fields (title/body/cta_label) are accepted for backward
// compatibility with feeds that predate the translations map. They are
// normalized into a synthetic single-locale entry in Translations after
// unmarshal; see normalize().
type Message struct {
	ID            string                        `json:"id"`
	PublishedAt   string                        `json:"published_at"`
	ShowUntil     string                        `json:"show_until,omitempty"`
	CTAURL        string                        `json:"cta_url,omitempty"`
	DefaultLocale string                        `json:"default_locale,omitempty"`
	Translations  map[string]MessageTranslation `json:"translations,omitempty"`

	// Legacy flat fields (backward compat). Cleared after normalize().
	LegacyTitle    string `json:"title,omitempty"`
	LegacyBody     string `json:"body,omitempty"`
	LegacyCTALabel string `json:"cta_label,omitempty"`
}

// MessageTranslation holds the localized content for a single language.
type MessageTranslation struct {
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	CTALabel string `json:"cta_label,omitempty"`
	CTAURL   string `json:"cta_url,omitempty"`
}

// normalize promotes legacy flat fields into the Translations map when the
// latter is absent. Produces a single-entry translation map keyed by
// DefaultLocale (or "en" if unset) so downstream code can rely on the
// translations structure uniformly.
func (m *Message) normalize() {
	if len(m.Translations) == 0 && m.LegacyTitle != "" {
		locale := m.DefaultLocale
		if locale == "" {
			locale = "en"
		}
		m.Translations = map[string]MessageTranslation{
			locale: {
				Title:    m.LegacyTitle,
				Body:     m.LegacyBody,
				CTALabel: m.LegacyCTALabel,
			},
		}
		if m.DefaultLocale == "" {
			m.DefaultLocale = locale
		}
	}
	m.LegacyTitle = ""
	m.LegacyBody = ""
	m.LegacyCTALabel = ""
}

// hasValidTranslation reports whether the message has at least one
// translation with a non-empty title.
func (m *Message) hasValidTranslation() bool {
	for _, tr := range m.Translations {
		if tr.Title != "" {
			return true
		}
	}
	return false
}

// isActive reports whether the message is currently within its display
// window: published_at <= now AND (show_until absent or show_until > now).
// Invalid dates cause the message to be considered inactive.
func (m *Message) isActive(now time.Time) bool {
	published, err := time.Parse(time.RFC3339, m.PublishedAt)
	if err != nil {
		return false
	}
	if published.After(now) {
		return false
	}
	if m.ShowUntil != "" {
		until, err := time.Parse(time.RFC3339, m.ShowUntil)
		if err == nil && !until.After(now) {
			return false
		}
	}
	return true
}

// Feed is the top-level feed payload.
type Feed struct {
	Messages []Message `json:"messages"`
}

// Fetcher periodically pulls a remote JSON feed and caches the latest
// active message. "Active" means within the display window defined by
// published_at and show_until — pre-scheduled or expired messages are
// skipped, so a future message never shadows an active one lower in the
// feed.
type Fetcher struct {
	feedURL  string
	interval time.Duration
	client   *http.Client
	now      func() time.Time // injectable for tests

	mu        sync.RWMutex
	latest    *Message
	fetchedAt time.Time
	lastErr   string
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
		now:      func() time.Time { return time.Now().UTC() },
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

	latest := pickActive(feed.Messages, f.now())

	f.mu.Lock()
	f.latest = latest
	f.fetchedAt = time.Now()
	f.lastErr = ""
	f.mu.Unlock()
}

// pickActive returns the first message that is well-formed and currently
// active (published_at <= now, not expired). Legacy flat messages are
// normalized to translations on the fly.
func pickActive(messages []Message, now time.Time) *Message {
	for i := range messages {
		m := &messages[i]
		m.normalize()
		if m.ID == "" || !m.hasValidTranslation() {
			continue
		}
		if !m.isActive(now) {
			continue
		}
		return m
	}
	return nil
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
