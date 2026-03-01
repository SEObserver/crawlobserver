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
	ContentHash     uint64
	BodyHTML        string
	BodyTruncated   bool
	CrawledAt       time.Time

	// JS Rendering
	JSRendered         bool
	JSRenderDurationMs uint64
	JSRenderError      string

	// Rendered data
	RenderedTitle           string
	RenderedMetaDescription string
	RenderedH1              []string
	RenderedWordCount       uint32
	RenderedLinksCount      uint32
	RenderedImagesCount     uint16
	RenderedCanonical       string
	RenderedMetaRobots      string
	RenderedSchemaTypes     []string
	RenderedBodyHTML        string

	// Diff flags (static vs rendered)
	JSChangedTitle       bool
	JSChangedDescription bool
	JSChangedH1          bool
	JSChangedCanonical   bool
	JSChangedContent     bool  // word count changed >20%
	JSAddedLinks         int32 // delta links
	JSAddedImages        int32 // delta images
	JSAddedSchema        bool  // new schema types appeared
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

// ExternalLinkCheck represents a single external URL check result.
type ExternalLinkCheck struct {
	CrawlSessionID string    `json:"crawl_session_id"`
	URL            string    `json:"url"`
	StatusCode     uint16    `json:"status_code"`
	Error          string    `json:"error"`
	ContentType    string    `json:"content_type"`
	RedirectURL    string    `json:"redirect_url"`
	ResponseTimeMs uint32   `json:"response_time_ms"`
	CheckedAt      time.Time `json:"checked_at"`
}

// ExternalDomainCheck represents aggregated external check stats per domain.
type ExternalDomainCheck struct {
	Domain        string `json:"domain"`
	TotalURLs     uint64 `json:"total_urls"`
	OK            uint64 `json:"ok"`
	Redirects     uint64 `json:"redirects"`
	ClientErrors  uint64 `json:"client_errors"`
	ServerErrors  uint64 `json:"server_errors"`
	Unreachable   uint64 `json:"unreachable"`
	AvgResponseMs uint32 `json:"avg_response_ms"`
}

// ExpiredDomain represents a registrable domain where all checked URLs had DNS failures.
type ExpiredDomain struct {
	RegistrableDomain string                `json:"registrable_domain"`
	DeadURLsChecked   uint64                `json:"dead_urls_checked"`
	Sources           []ExpiredDomainSource `json:"sources"`
}

// ExpiredDomainSource represents a source page linking to an expired domain.
type ExpiredDomainSource struct {
	SourceURL string `json:"source_url"`
	TargetURL string `json:"target_url"`
}

// ExpiredDomainsResult wraps paginated expired domain results.
type ExpiredDomainsResult struct {
	Domains []ExpiredDomain `json:"domains"`
	Total   uint64          `json:"total"`
}

// --- Provider Data Models ---

type ProviderDomainMetricsRow struct {
	Provider        string  `json:"provider"`
	Domain          string  `json:"domain"`
	BacklinksTotal  int64   `json:"backlinks_total"`
	RefDomainsTotal int64   `json:"refdomains_total"`
	DomainRank      float64 `json:"domain_rank"`
	OrganicKeywords int64   `json:"organic_keywords"`
	OrganicTraffic  int64   `json:"organic_traffic"`
	OrganicCost     float64 `json:"organic_cost"`
	FetchedAt       time.Time `json:"fetched_at"`
}

type ProviderBacklinkRow struct {
	Provider     string  `json:"provider"`
	Domain       string  `json:"domain"`
	SourceURL    string  `json:"source_url"`
	TargetURL    string  `json:"target_url"`
	AnchorText   string  `json:"anchor_text"`
	SourceDomain string  `json:"source_domain"`
	LinkType     string  `json:"link_type"`
	DomainRank   float64 `json:"domain_rank"`
	PageRank     float64 `json:"page_rank"`
	Nofollow     bool    `json:"nofollow"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	FetchedAt    time.Time `json:"fetched_at"`
}

type ProviderRefDomainRow struct {
	Provider      string  `json:"provider"`
	Domain        string  `json:"domain"`
	RefDomain     string  `json:"ref_domain"`
	BacklinkCount int64   `json:"backlink_count"`
	DomainRank    float64 `json:"domain_rank"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	FetchedAt     time.Time `json:"fetched_at"`
}

type ProviderRankingRow struct {
	Provider     string  `json:"provider"`
	Domain       string  `json:"domain"`
	Keyword      string  `json:"keyword"`
	URL          string  `json:"url"`
	SearchBase   string  `json:"search_base"`
	Position     uint16  `json:"position"`
	SearchVolume int64   `json:"search_volume"`
	CPC          float64 `json:"cpc"`
	Traffic      float64 `json:"traffic"`
	TrafficPct   float64 `json:"traffic_pct"`
	FetchedAt    time.Time `json:"fetched_at"`
}

type ProviderVisibilityRow struct {
	Provider      string  `json:"provider"`
	Domain        string  `json:"domain"`
	SearchBase    string  `json:"search_base"`
	Date          time.Time `json:"date"`
	Visibility    float64 `json:"visibility"`
	KeywordsCount int64   `json:"keywords_count"`
	FetchedAt     time.Time `json:"fetched_at"`
}

// PageResourceCheck represents a single page resource check result.
type PageResourceCheck struct {
	CrawlSessionID string    `json:"crawl_session_id"`
	URL            string    `json:"url"`
	ResourceType   string    `json:"resource_type"`
	IsInternal     bool      `json:"is_internal"`
	StatusCode     uint16    `json:"status_code"`
	Error          string    `json:"error"`
	ContentType    string    `json:"content_type"`
	RedirectURL    string    `json:"redirect_url"`
	ResponseTimeMs uint32    `json:"response_time_ms"`
	CheckedAt      time.Time `json:"checked_at"`
	PageCount      uint64    `json:"page_count,omitempty"`
}

// PageResourceRef links a page to a resource it uses.
type PageResourceRef struct {
	CrawlSessionID string `json:"crawl_session_id"`
	PageURL        string `json:"page_url"`
	ResourceURL    string `json:"resource_url"`
	ResourceType   string `json:"resource_type"`
	IsInternal     bool   `json:"is_internal"`
}

// NearDuplicatePair represents two pages with near-identical content.
type NearDuplicatePair struct {
	URLa      string `json:"url_a"`
	URLb      string `json:"url_b"`
	TitleA    string `json:"title_a"`
	TitleB    string `json:"title_b"`
	WordCountA uint32 `json:"word_count_a"`
	WordCountB uint32 `json:"word_count_b"`
	Similarity float64 `json:"similarity"` // 0–1, 1 = exact duplicate
}

// NearDuplicatesResult wraps paginated near-duplicate results.
type NearDuplicatesResult struct {
	Pairs []NearDuplicatePair `json:"pairs"`
	Total uint64              `json:"total"`
}

// ResourceTypeSummary holds aggregated stats for one resource type.
type ResourceTypeSummary struct {
	ResourceType string `json:"resource_type"`
	Total        uint64 `json:"total"`
	Internal     uint64 `json:"internal"`
	External     uint64 `json:"external"`
	OK           uint64 `json:"ok"`
	Errors       uint64 `json:"errors"`
}
