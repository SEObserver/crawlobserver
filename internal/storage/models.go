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
	ProjectID    *string
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
	BodyTruncated   bool
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

// RobotsRow represents a robots.txt entry for storage.
type RobotsRow struct {
	CrawlSessionID string
	Host           string
	StatusCode     uint16
	Content        string
	FetchedAt      time.Time
}

// SitemapRow represents a discovered sitemap for storage.
type SitemapRow struct {
	CrawlSessionID string
	URL            string
	Type           string // "index" | "urlset"
	URLCount       uint32
	ParentURL      string // empty if top-level
	StatusCode     uint16
	FetchedAt      time.Time
}

// SitemapURLRow represents a URL entry within a sitemap.
type SitemapURLRow struct {
	CrawlSessionID string
	SitemapURL     string
	Loc            string
	LastMod        string
	ChangeFreq     string
	Priority       string
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

// CompareStatsResult holds side-by-side stats for two sessions.
type CompareStatsResult struct {
	SessionA string        `json:"session_a"`
	SessionB string        `json:"session_b"`
	StatsA   *SessionStats `json:"stats_a"`
	StatsB   *SessionStats `json:"stats_b"`
}

// PageDiffRow represents a single page difference between two crawls.
type PageDiffRow struct {
	URL              string  `json:"url"`
	DiffType         string  `json:"diff_type"`
	StatusCodeA      uint16  `json:"status_code_a"`
	TitleA           string  `json:"title_a"`
	CanonicalA       string  `json:"canonical_a"`
	IsIndexableA     bool    `json:"is_indexable_a"`
	WordCountA       uint32  `json:"word_count_a"`
	DepthA           uint16  `json:"depth_a"`
	PageRankA        float64 `json:"pagerank_a"`
	MetaDescriptionA string  `json:"meta_description_a"`
	H1A              string  `json:"h1_a"`
	StatusCodeB      uint16  `json:"status_code_b"`
	TitleB           string  `json:"title_b"`
	CanonicalB       string  `json:"canonical_b"`
	IsIndexableB     bool    `json:"is_indexable_b"`
	WordCountB       uint32  `json:"word_count_b"`
	DepthB           uint16  `json:"depth_b"`
	PageRankB        float64 `json:"pagerank_b"`
	MetaDescriptionB string  `json:"meta_description_b"`
	H1B              string  `json:"h1_b"`
}

// PageDiffResult wraps paginated page diff results.
type PageDiffResult struct {
	Pages        []PageDiffRow `json:"pages"`
	TotalAdded   uint64        `json:"total_added"`
	TotalRemoved uint64        `json:"total_removed"`
	TotalChanged uint64        `json:"total_changed"`
}

// LinkDiffRow represents a single internal link difference.
type LinkDiffRow struct {
	SourceURL  string `json:"source_url"`
	TargetURL  string `json:"target_url"`
	AnchorText string `json:"anchor_text"`
	DiffType   string `json:"diff_type"`
}

// LinkDiffResult wraps paginated link diff results.
type LinkDiffResult struct {
	Links        []LinkDiffRow `json:"links"`
	TotalAdded   uint64        `json:"total_added"`
	TotalRemoved uint64        `json:"total_removed"`
}
