package storage

import (
	"time"
)

// CrawlSession represents a crawl session.
type CrawlSession struct {
	ID           string
	StartedAt    time.Time
	FinishedAt   time.Time
	Status       string // running, completed, failed
	SeedURLs     []string
	Config       string // JSON
	PagesCrawled uint64
	UserAgent    string
}

// PageRow represents a crawled page for storage.
type PageRow struct {
	CrawlSessionID  string
	URL             string
	FinalURL        string
	StatusCode      uint16
	ContentType     string
	Title           string
	Canonical       string
	MetaRobots      string
	MetaDescription string
	H1              []string
	H2              []string
	H3              []string
	H4              []string
	H5              []string
	H6              []string
	Headers         map[string]string
	RedirectChain   []RedirectHopRow
	BodySize        uint64
	FetchDurationMs uint64
	Error           string
	Depth           uint16
	FoundOn         string
	BodyHTML        string
	CrawledAt       time.Time
}

// RedirectHopRow represents a redirect hop for storage.
type RedirectHopRow struct {
	URL        string
	StatusCode uint16
}

// LinkRow represents a link for storage.
type LinkRow struct {
	CrawlSessionID string
	SourceURL      string
	TargetURL      string
	AnchorText     string
	Rel            string
	IsInternal     bool
	Tag            string
	CrawledAt      time.Time
}
