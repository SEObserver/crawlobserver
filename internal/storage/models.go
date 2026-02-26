package storage

import (
	"time"
)

// CrawlSession represents a crawl session.
type CrawlSession struct {
	ID           string
	StartedAt    time.Time
	FinishedAt   time.Time
	Status       string // running, completed, failed, stopped
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
	TitleLength     uint16
	Canonical       string
	CanonicalIsSelf bool
	IsIndexable     bool
	IndexReason     string // why not indexable
	MetaRobots      string
	MetaDescription string
	MetaDescLength  uint16
	MetaKeywords    string
	H1              []string
	H2              []string
	H3              []string
	H4              []string
	H5              []string
	H6              []string
	WordCount       uint32
	InternalLinksOut uint32
	ExternalLinksOut uint32
	ImagesCount     uint16
	ImagesNoAlt     uint16
	Hreflang        []HreflangRow
	Lang            string
	OGTitle         string
	OGDescription   string
	OGImage         string
	SchemaTypes     []string
	Headers         map[string]string
	RedirectChain   []RedirectHopRow
	BodySize        uint64
	FetchDurationMs uint64
	ContentEncoding string
	XRobotsTag      string
	Error           string
	Depth           uint16
	FoundOn         string
	PageRank        float64
	BodyHTML        string
	CrawledAt       time.Time
}

// RedirectHopRow represents a redirect hop for storage.
type RedirectHopRow struct {
	URL        string
	StatusCode uint16
}

// HreflangRow represents a hreflang entry.
type HreflangRow struct {
	Lang string
	URL  string
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
