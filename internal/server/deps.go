package server

import (
	"context"

	"github.com/SEObserver/seocrawler/internal/crawler"
	"github.com/SEObserver/seocrawler/internal/storage"
)

// StorageService is the subset of storage.Store used by the HTTP server.
type StorageService interface {
	ListSessions(ctx context.Context, projectID ...string) ([]storage.CrawlSession, error)
	GetSession(ctx context.Context, sessionID string) (*storage.CrawlSession, error)
	DeleteSession(ctx context.Context, sessionID string) error
	UpdateSessionProject(ctx context.Context, sessionID string, projectID *string) error
	ListPages(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter) ([]storage.PageRow, error)
	ExternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter) ([]storage.LinkRow, error)
	InternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int, filters []storage.ParsedFilter) ([]storage.LinkRow, error)
	SessionStats(ctx context.Context, sessionID string) (*storage.SessionStats, error)
	GetPageHTML(ctx context.Context, sessionID, url string) (string, error)
	GetPage(ctx context.Context, sessionID, url string) (*storage.PageRow, error)
	GetPageLinks(ctx context.Context, sessionID, url string, inLimit, inOffset int) (*storage.PageLinksResult, error)
	StorageStats(ctx context.Context) (*storage.StorageStatsResult, error)
	SessionStorageStats(ctx context.Context) (map[string]uint64, error)
	GlobalStats(ctx context.Context) ([]storage.GlobalSessionStats, *storage.StorageStatsResult, error)
	RecomputeDepths(ctx context.Context, sessionID string, seedURLs []string) error
	ComputePageRank(ctx context.Context, sessionID string) error
	PageRankDistribution(ctx context.Context, sessionID string, buckets int) (*storage.PageRankDistributionResult, error)
	PageRankTreemap(ctx context.Context, sessionID string, depth, minPages int) ([]storage.PageRankTreemapEntry, error)
	PageRankTop(ctx context.Context, sessionID string, limit, offset int, directory string) (*storage.PageRankTopResult, error)
	GetRobotsHosts(ctx context.Context, sessionID string) ([]storage.RobotsRow, error)
	GetRobotsContent(ctx context.Context, sessionID, host string) (*storage.RobotsRow, error)
	GetURLsByHost(ctx context.Context, sessionID, host string) ([]string, error)
	GetSitemaps(ctx context.Context, sessionID string) ([]storage.SitemapRow, error)
	GetSitemapURLs(ctx context.Context, sessionID, sitemapURL string, limit, offset int) ([]storage.SitemapURLRow, error)
}

// CrawlService is the subset of crawler.Manager used by the HTTP server.
type CrawlService interface {
	IsRunning(sessionID string) bool
	Progress(sessionID string) (int64, int, bool)
	StartCrawl(req crawler.CrawlRequest) (string, error)
	StopCrawl(sessionID string) error
	ResumeCrawl(sessionID string, overrides *crawler.CrawlRequest) (string, error)
	RetryFailed(sessionID string, overrides *crawler.CrawlRequest) (int, error)
}
