package server

import (
	"context"
	"io"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/crawler"
	"github.com/SEObserver/crawlobserver/internal/customtests"
	"github.com/SEObserver/crawlobserver/internal/extraction"
	"github.com/SEObserver/crawlobserver/internal/schema"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// SessionStore handles crawl session lifecycle and global stats.
type SessionStore interface {
	ListSessions(ctx context.Context, projectID ...string) ([]storage.CrawlSession, error)
	ListSessionsPaginated(ctx context.Context, limit, offset int, projectID, search string) ([]storage.CrawlSession, int, error)
	GetSession(ctx context.Context, sessionID string) (*storage.CrawlSession, error)
	DeleteSession(ctx context.Context, sessionID string) error
	UpdateSessionProject(ctx context.Context, sessionID string, projectID *string) error
	UpdateSessionLabel(ctx context.Context, sessionID, label string) error
	StorageStats(ctx context.Context) (*storage.StorageStatsResult, error)
	SessionStorageStats(ctx context.Context) (map[string]uint64, error)
	GlobalStats(ctx context.Context) ([]storage.GlobalSessionStats, *storage.StorageStatsResult, error)
	SessionStats(ctx context.Context, sessionID string) (*storage.SessionStats, error)
}

// PageStore handles reading and exploring crawled pages.
type PageStore interface {
	ListPages(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.PageRow, error)
	GetPage(ctx context.Context, sessionID, url string) (*storage.PageRow, error)
	GetPageHTML(ctx context.Context, sessionID, url string) (string, error)
	GetPageLinks(ctx context.Context, sessionID, url string, outLimit, outOffset, inLimit, inOffset int) (*storage.PageLinksResult, error)
	StatusTimeline(ctx context.Context, sessionID string) ([]storage.StatusTimelineBucket, error)
	StatusTimelineRecent(ctx context.Context, sessionID string) ([]storage.StatusTimelineBucket, error)
	ListRedirectPages(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.RedirectPageRow, error)
}

// LinkStore handles internal/external link browsing and expired domains.
type LinkStore interface {
	ExternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.LinkRow, error)
	InternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.LinkRow, error)
	GetExternalLinkChecks(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.ExternalLinkCheckWithSource, error)
	GetExternalLinkCheckDomains(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter, havingFilters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.ExternalDomainCheck, error)
	GetExpiredDomains(ctx context.Context, sessionID string, limit, offset int, nsOnly bool) (*storage.ExpiredDomainsResult, error)
}

// PageRankStore handles PageRank computation, distribution, and weighted PR.
type PageRankStore interface {
	ComputePageRank(ctx context.Context, sessionID string) error
	RecomputeDepths(ctx context.Context, sessionID string, seedURLs []string) error
	PageRankDistribution(ctx context.Context, sessionID string, buckets int) (*storage.PageRankDistributionResult, error)
	PageRankTreemap(ctx context.Context, sessionID string, depth, minPages int) ([]storage.PageRankTreemapEntry, error)
	PageRankTop(ctx context.Context, sessionID string, limit, offset int, directory string) (*storage.PageRankTopResult, error)
	WeightedPageRankTop(ctx context.Context, sessionID, projectID string, limit, offset int, directory, sort, order string) (*storage.WeightedPageRankResult, error)
	PagesWithAuthority(ctx context.Context, sessionID, projectID string, limit, offset int) ([]storage.PageWithAuthority, int, error)
}

// SitemapRobotsStore handles sitemaps and robots.txt data.
type SitemapRobotsStore interface {
	GetRobotsHosts(ctx context.Context, sessionID string) ([]storage.RobotsRow, error)
	GetRobotsContent(ctx context.Context, sessionID, host string) (*storage.RobotsRow, error)
	GetURLsByHost(ctx context.Context, sessionID, host string) ([]string, error)
	GetSitemaps(ctx context.Context, sessionID string) ([]storage.SitemapRow, error)
	GetSitemapURLs(ctx context.Context, sessionID, sitemapURL string, limit, offset int) ([]storage.SitemapURLRow, error)
	GetSitemapCoverageURLs(ctx context.Context, sessionID, filter string, limit, offset int) ([]storage.SitemapURLRow, error)
}

// ContentAnalysisStore handles near-duplicates, content hashes, resources, audit, and HTML streaming.
type ContentAnalysisStore interface {
	SessionAudit(ctx context.Context, sessionID string) (*storage.AuditResult, error)
	ComputeNearDuplicates(ctx context.Context, sessionID string) error
	NearDuplicates(ctx context.Context, sessionID string, threshold int, limit, offset int) (*storage.NearDuplicatesResult, error)
	UpdateContentHashes(ctx context.Context, sessionID string, hashes map[string]uint64) error
	StreamPagesHTML(ctx context.Context, sessionID string) (<-chan storage.PageHTMLRow, error)
	HasStoredHTML(ctx context.Context, sessionID string) (bool, error)
	GetPageBodies(ctx context.Context, sessionID string, limit, offset int) ([]storage.PageBody, error)
	GetPageResourceChecks(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter) ([]storage.PageResourceCheck, error)
	GetPageResourceTypeSummary(ctx context.Context, sessionID string) ([]storage.ResourceTypeSummary, error)
	InsertStructuredData(ctx context.Context, items []schema.StructuredDataItem) error
	GetStructuredData(ctx context.Context, sessionID, url string) ([]schema.StructuredDataItem, error)
	ComputeHreflangValidation(ctx context.Context, sessionID string) error
	HreflangValidation(ctx context.Context, sessionID string, issueType string, pageURL string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) (*storage.HreflangValidationResult, error)
}

// URLPatternStore handles URL pattern analysis (grouping, params, directories, hosts).
type URLPatternStore interface {
	URLPatterns(ctx context.Context, sessionID string, depth int) ([]storage.URLPattern, error)
	URLParams(ctx context.Context, sessionID string, limit int) ([]storage.URLParam, error)
	URLDirectories(ctx context.Context, sessionID string, depth, minPages int) ([]storage.URLDirectory, error)
	URLHosts(ctx context.Context, sessionID string) ([]storage.URLHost, error)
}

// CompareStore handles side-by-side session comparison.
type CompareStore interface {
	CompareStats(ctx context.Context, sessionA, sessionB string) (*storage.CompareStatsResult, error)
	ComparePages(ctx context.Context, sessionA, sessionB, diffType string, limit, offset int) (*storage.PageDiffResult, error)
	CompareLinks(ctx context.Context, sessionA, sessionB, diffType string, limit, offset int) (*storage.LinkDiffResult, error)
}

// ImportExportStore handles bulk import and export of sessions.
type ImportExportStore interface {
	ExportSession(ctx context.Context, sessionID string, w io.Writer, includeHTML bool) error
	ImportSession(ctx context.Context, r io.Reader) (*storage.CrawlSession, error)
	ImportCSVSession(ctx context.Context, r io.Reader, projectID string) (*storage.CSVImportResult, error)
}

// ExtractionStore handles custom extractions and SQL-based tests.
type ExtractionStore interface {
	InsertExtractions(ctx context.Context, rows []extraction.ExtractionRow) error
	GetExtractions(ctx context.Context, sessionID string, limit, offset int) (*extraction.ExtractionResult, error)
	DeleteExtractions(ctx context.Context, sessionID string) error
	RunExtractionsPostCrawl(ctx context.Context, sessionID string, extractors []extraction.Extractor) (*extraction.ExtractionResult, error)
	RunCustomTestsSQL(ctx context.Context, sessionID string, rules []customtests.TestRule) (map[string]map[string]string, error)
}

// InterlinkingStore handles interlinking opportunities and PageRank simulations.
type InterlinkingStore interface {
	DeleteInterlinkingOpportunities(ctx context.Context, sessionID string) error
	InsertInterlinkingOpportunities(ctx context.Context, sessionID string, opps []storage.InterlinkingOpportunity) error
	ListInterlinkingOpportunities(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.InterlinkingOpportunity, int, error)
	LoadInternalLinkSet(ctx context.Context, sessionID string) (map[[2]string]struct{}, error)
	LoadPageMetadata(ctx context.Context, sessionID string) (map[string]storage.PageMetadata, error)
	LoadPageRankGraph(ctx context.Context, sessionID string) (*storage.PageRankGraph, error)
	InsertSimulation(ctx context.Context, sessionID string, simID string, virtualLinks []storage.VirtualLink, results []storage.SimulationResultRow, meta storage.SimulationMeta) error
	ListSimulations(ctx context.Context, sessionID string) ([]storage.SimulationMeta, error)
	GetSimulation(ctx context.Context, sessionID, simID string) (*storage.SimulationMeta, error)
	ListSimulationResults(ctx context.Context, sessionID, simID string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.SimulationResultRow, int, error)
}

// CrawlStore handles core crawl data: sessions, pages, links, stats, and analysis.
// It composes domain-specific sub-interfaces for clearer API boundaries.
type CrawlStore interface {
	SessionStore
	PageStore
	LinkStore
	PageRankStore
	SitemapRobotsStore
	ContentAnalysisStore
	URLPatternStore
	CompareStore
	ImportExportStore
	ExtractionStore
	InterlinkingStore
	// Write-side methods used by resource check handlers.
	InsertPageResourceRefs(ctx context.Context, refs []storage.PageResourceRef) error
	InsertPageResourceChecks(ctx context.Context, checks []storage.PageResourceCheck) error
}

// GSCStore handles Google Search Console data.
type GSCStore interface {
	InsertGSCAnalytics(ctx context.Context, projectID string, rows []storage.GSCAnalyticsInsertRow) error
	InsertGSCInspection(ctx context.Context, projectID string, rows []storage.GSCInspectionInsertRow) error
	GSCOverview(ctx context.Context, projectID string) (*storage.GSCOverviewStats, error)
	GSCTopQueries(ctx context.Context, projectID string, limit, offset int) ([]storage.GSCQueryRow, int, error)
	GSCTopPages(ctx context.Context, projectID string, limit, offset int) ([]storage.GSCPageRow, int, error)
	GSCByCountry(ctx context.Context, projectID string) ([]storage.GSCCountryRow, error)
	GSCByDevice(ctx context.Context, projectID string) ([]storage.GSCDeviceRow, error)
	GSCTimeline(ctx context.Context, projectID string) ([]storage.GSCTimelineRow, error)
	GSCInspectionResults(ctx context.Context, projectID string, limit, offset int) ([]storage.GSCInspectionRow, int, error)
	DeleteGSCData(ctx context.Context, projectID string) error
}

// ProviderStore handles third-party provider data (SEObserver, etc.).
type ProviderStore interface {
	InsertProviderDomainMetrics(ctx context.Context, projectID string, rows []storage.ProviderDomainMetricsRow) error
	InsertProviderBacklinks(ctx context.Context, projectID string, rows []storage.ProviderBacklinkRow) error
	InsertProviderRefDomains(ctx context.Context, projectID string, rows []storage.ProviderRefDomainRow) error
	InsertProviderRankings(ctx context.Context, projectID string, rows []storage.ProviderRankingRow) error
	InsertProviderVisibility(ctx context.Context, projectID string, rows []storage.ProviderVisibilityRow) error
	ProviderDomainMetrics(ctx context.Context, projectID, provider string) (*storage.ProviderDomainMetricsRow, error)
	ProviderBacklinks(ctx context.Context, projectID, provider string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.ProviderBacklinkRow, int, error)
	ProviderRefDomains(ctx context.Context, projectID, provider string, limit, offset int) ([]storage.ProviderRefDomainRow, int, error)
	ProviderRankings(ctx context.Context, projectID, provider string, limit, offset int) ([]storage.ProviderRankingRow, int, error)
	ProviderVisibilityHistory(ctx context.Context, projectID, provider string) ([]storage.ProviderVisibilityRow, error)
	InsertProviderTopPages(ctx context.Context, projectID string, rows []storage.ProviderTopPageRow) error
	ProviderTopPages(ctx context.Context, projectID, provider string, limit, offset int) ([]storage.ProviderTopPageRow, int, error)
	InsertProviderAPICalls(ctx context.Context, rows []storage.ProviderAPICallRow) error
	ProviderAPICalls(ctx context.Context, projectID, provider string, limit, offset int) ([]storage.ProviderAPICallRow, int, error)
	DeleteProviderData(ctx context.Context, projectID, provider string) error
	InsertProviderData(ctx context.Context, projectID string, rows []storage.ProviderDataRow) error
	ProviderData(ctx context.Context, projectID, provider, dataType string, limit, offset int, filters []storage.ParsedFilter, sort *storage.SortParam) ([]storage.ProviderDataRow, int, error)
	ProviderDataAge(ctx context.Context, projectID, provider, dataType string) (time.Time, error)
}

// LogStore handles application logs.
type LogStore interface {
	InsertLogs(ctx context.Context, logs []applog.LogRow) error
	ListLogs(ctx context.Context, limit, offset int, level, component, search string) ([]applog.LogRow, int, error)
	ExportLogs(ctx context.Context) ([]applog.LogRow, error)
}

// StorageService is the full storage interface used by the HTTP server.
// It composes domain-specific interfaces for clearer API boundaries.
type StorageService interface {
	CrawlStore
	GSCStore
	ProviderStore
	LogStore
}

// CrawlService is the subset of crawler.Manager used by the HTTP server.
type CrawlService interface {
	IsRunning(sessionID string) bool
	IsQueued(sessionID string) bool
	Progress(sessionID string) (int64, int, bool)
	Phase(sessionID string) string
	BufferState(sessionID string) storage.BufferErrorState
	LastError(sessionID string) string
	StartCrawl(req crawler.CrawlRequest) (string, error)
	StopCrawl(sessionID string) error
	ResumeCrawl(sessionID string, overrides *crawler.CrawlRequest) (string, error)
	RetryFailed(sessionID string, overrides *crawler.CrawlRequest) (int, error)
	QueuedSessions() []string
	Shutdown(timeout time.Duration)
	RecoverOrphanedSessions(ctx context.Context)
}
