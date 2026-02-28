package storage

import (
	"context"
	"fmt"
	"log"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Migration represents a schema migration step.
// Either DDL (a SQL string) or Fn (a function) must be set, not both.
type Migration struct {
	Name string
	DDL  string
	Fn   func(ctx context.Context, conn driver.Conn) error
}

// DDL statements for ClickHouse tables.

const CreateDatabase = `CREATE DATABASE IF NOT EXISTS crawlobserver`

const CreateCrawlSessions = `
CREATE TABLE IF NOT EXISTS crawlobserver.crawl_sessions (
    id UUID,
    started_at DateTime64(3),
    finished_at DateTime64(3),
    status String,
    seed_urls Array(String),
    config String,
    pages_crawled UInt64,
    user_agent String
) ENGINE = ReplacingMergeTree()
ORDER BY (id)
`

const CreatePages = `
CREATE TABLE IF NOT EXISTS crawlobserver.pages (
    crawl_session_id UUID,
    url String,
    final_url String,
    status_code UInt16,
    content_type String,
    title String,
    title_length UInt16,
    canonical String,
    canonical_is_self Bool,
    is_indexable Bool,
    index_reason String,
    meta_robots String,
    meta_description String,
    meta_desc_length UInt16,
    meta_keywords String,
    h1 Array(String),
    h2 Array(String),
    h3 Array(String),
    h4 Array(String),
    h5 Array(String),
    h6 Array(String),
    word_count UInt32,
    internal_links_out UInt32,
    external_links_out UInt32,
    images_count UInt16,
    images_no_alt UInt16,
    hreflang Array(Tuple(lang String, url String)),
    lang String,
    og_title String,
    og_description String,
    og_image String,
    schema_types Array(String),
    headers Map(String, String),
    redirect_chain Array(Tuple(url String, status_code UInt16)),
    body_size UInt64,
    fetch_duration_ms UInt64,
    content_encoding String,
    x_robots_tag String,
    error String,
    depth UInt16,
    found_on String,
    body_html String CODEC(ZSTD(3)),
    crawled_at DateTime64(3)
) ENGINE = ReplacingMergeTree(crawled_at)
PARTITION BY toYYYYMM(crawled_at)
ORDER BY (crawl_session_id, url)
`

const CreateLinks = `
CREATE TABLE IF NOT EXISTS crawlobserver.links (
    crawl_session_id UUID,
    source_url String,
    target_url String,
    anchor_text String,
    rel String,
    is_internal Bool,
    tag String,
    crawled_at DateTime64(3)
) ENGINE = MergeTree()
ORDER BY (crawl_session_id, source_url, target_url)
`

// AlterPagesV2 adds new columns to existing pages table.
const AlterPagesV2 = `
ALTER TABLE crawlobserver.pages
    ADD COLUMN IF NOT EXISTS title_length UInt16 AFTER title,
    ADD COLUMN IF NOT EXISTS canonical_is_self Bool AFTER canonical,
    ADD COLUMN IF NOT EXISTS is_indexable Bool AFTER canonical_is_self,
    ADD COLUMN IF NOT EXISTS index_reason String AFTER is_indexable,
    ADD COLUMN IF NOT EXISTS meta_desc_length UInt16 AFTER meta_description,
    ADD COLUMN IF NOT EXISTS meta_keywords String AFTER meta_desc_length,
    ADD COLUMN IF NOT EXISTS word_count UInt32 AFTER h6,
    ADD COLUMN IF NOT EXISTS internal_links_out UInt32 AFTER word_count,
    ADD COLUMN IF NOT EXISTS external_links_out UInt32 AFTER internal_links_out,
    ADD COLUMN IF NOT EXISTS images_count UInt16 AFTER external_links_out,
    ADD COLUMN IF NOT EXISTS images_no_alt UInt16 AFTER images_count,
    ADD COLUMN IF NOT EXISTS hreflang Array(Tuple(lang String, url String)) AFTER images_no_alt,
    ADD COLUMN IF NOT EXISTS lang String AFTER hreflang,
    ADD COLUMN IF NOT EXISTS og_title String AFTER lang,
    ADD COLUMN IF NOT EXISTS og_description String AFTER og_title,
    ADD COLUMN IF NOT EXISTS og_image String AFTER og_description,
    ADD COLUMN IF NOT EXISTS schema_types Array(String) AFTER og_image,
    ADD COLUMN IF NOT EXISTS content_encoding String AFTER fetch_duration_ms,
    ADD COLUMN IF NOT EXISTS x_robots_tag String AFTER content_encoding
`

const AlterPagesV3 = `
ALTER TABLE crawlobserver.pages
    ADD COLUMN IF NOT EXISTS pagerank Float64 DEFAULT 0 AFTER found_on
`

const AlterPagesV4 = `
ALTER TABLE crawlobserver.pages
    ADD COLUMN IF NOT EXISTS body_truncated Bool DEFAULT false AFTER body_html
`

const CreateRobotsTxt = `
CREATE TABLE IF NOT EXISTS crawlobserver.robots_txt (
    crawl_session_id UUID,
    host String,
    status_code UInt16,
    content String CODEC(ZSTD(3)),
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
ORDER BY (crawl_session_id, host)
`

const AlterSessionsV2 = `
ALTER TABLE crawlobserver.crawl_sessions
    ADD COLUMN IF NOT EXISTS project_id Nullable(String) DEFAULT NULL
`

const CreateSitemaps = `
CREATE TABLE IF NOT EXISTS crawlobserver.sitemaps (
    crawl_session_id UUID,
    url String,
    type String,
    url_count UInt32,
    parent_url String,
    status_code UInt16,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
ORDER BY (crawl_session_id, url)
`

const CreateSitemapURLs = `
CREATE TABLE IF NOT EXISTS crawlobserver.sitemap_urls (
    crawl_session_id UUID,
    sitemap_url String,
    loc String,
    lastmod String,
    changefreq String,
    priority String
) ENGINE = ReplacingMergeTree()
ORDER BY (crawl_session_id, sitemap_url, loc)
`

// DDL for v2 tables partitioned by crawl_session_id.
const CreatePagesV2 = `
CREATE TABLE IF NOT EXISTS crawlobserver.pages_v2 (
    crawl_session_id UUID,
    url String,
    final_url String,
    status_code UInt16,
    content_type String,
    title String,
    title_length UInt16,
    canonical String,
    canonical_is_self Bool,
    is_indexable Bool,
    index_reason String,
    meta_robots String,
    meta_description String,
    meta_desc_length UInt16,
    meta_keywords String,
    h1 Array(String),
    h2 Array(String),
    h3 Array(String),
    h4 Array(String),
    h5 Array(String),
    h6 Array(String),
    word_count UInt32,
    internal_links_out UInt32,
    external_links_out UInt32,
    images_count UInt16,
    images_no_alt UInt16,
    hreflang Array(Tuple(lang String, url String)),
    lang String,
    og_title String,
    og_description String,
    og_image String,
    schema_types Array(String),
    headers Map(String, String),
    redirect_chain Array(Tuple(url String, status_code UInt16)),
    body_size UInt64,
    fetch_duration_ms UInt64,
    content_encoding String,
    x_robots_tag String,
    error String,
    depth UInt16,
    found_on String,
    pagerank Float64 DEFAULT 0,
    body_html String CODEC(ZSTD(3)),
    body_truncated Bool DEFAULT false,
    crawled_at DateTime64(3)
) ENGINE = ReplacingMergeTree(crawled_at)
PARTITION BY crawl_session_id
ORDER BY (crawl_session_id, url)
`

const CreateLinksV2 = `
CREATE TABLE IF NOT EXISTS crawlobserver.links_v2 (
    crawl_session_id UUID,
    source_url String,
    target_url String,
    anchor_text String,
    rel String,
    is_internal Bool,
    tag String,
    crawled_at DateTime64(3)
) ENGINE = MergeTree()
PARTITION BY crawl_session_id
ORDER BY (crawl_session_id, source_url, target_url)
`

const CreateRobotsTxtV2 = `
CREATE TABLE IF NOT EXISTS crawlobserver.robots_txt_v2 (
    crawl_session_id UUID,
    host String,
    status_code UInt16,
    content String CODEC(ZSTD(3)),
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY crawl_session_id
ORDER BY (crawl_session_id, host)
`

const CreateSitemapsV2 = `
CREATE TABLE IF NOT EXISTS crawlobserver.sitemaps_v2 (
    crawl_session_id UUID,
    url String,
    type String,
    url_count UInt32,
    parent_url String,
    status_code UInt16,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY crawl_session_id
ORDER BY (crawl_session_id, url)
`

const CreateSitemapURLsV2 = `
CREATE TABLE IF NOT EXISTS crawlobserver.sitemap_urls_v2 (
    crawl_session_id UUID,
    sitemap_url String,
    loc String,
    lastmod String,
    changefreq String,
    priority String
) ENGINE = ReplacingMergeTree()
PARTITION BY crawl_session_id
ORDER BY (crawl_session_id, sitemap_url, loc)
`

const CreateGSCAnalytics = `
CREATE TABLE IF NOT EXISTS crawlobserver.gsc_analytics (
    project_id String,
    date Date,
    query String,
    page String,
    country String,
    device String,
    clicks UInt32,
    impressions UInt32,
    ctr Float32,
    position Float32,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY project_id
ORDER BY (project_id, date, query, page, country, device)
`

const CreateGSCInspection = `
CREATE TABLE IF NOT EXISTS crawlobserver.gsc_inspection (
    project_id String,
    url String,
    verdict String,
    coverage_state String,
    indexing_state String,
    robots_txt_state String,
    last_crawl_time DateTime64(3),
    crawled_as String,
    canonical_url String,
    is_google_canonical Bool,
    mobile_usability String,
    rich_results_items UInt16,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY project_id
ORDER BY (project_id, url)
`

const CreateExternalLinkChecks = `
CREATE TABLE IF NOT EXISTS crawlobserver.external_link_checks (
    crawl_session_id UUID,
    url String,
    status_code UInt16,
    error String,
    content_type String,
    redirect_url String,
    response_time_ms UInt32,
    checked_at DateTime64(3)
) ENGINE = ReplacingMergeTree(checked_at)
PARTITION BY crawl_session_id
ORDER BY (crawl_session_id, url)
`

const CreateProviderDomainMetrics = `
CREATE TABLE IF NOT EXISTS crawlobserver.provider_domain_metrics (
    project_id String,
    provider String,
    domain String,
    backlinks_total Int64,
    refdomains_total Int64,
    domain_rank Float64,
    organic_keywords Int64,
    organic_traffic Int64,
    organic_cost Float64,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY project_id
ORDER BY (project_id, provider, domain)
`

const CreateProviderBacklinks = `
CREATE TABLE IF NOT EXISTS crawlobserver.provider_backlinks (
    project_id String,
    provider String,
    domain String,
    source_url String,
    target_url String,
    anchor_text String,
    source_domain String,
    link_type String,
    domain_rank Float64,
    page_rank Float64,
    nofollow Bool,
    first_seen Date,
    last_seen Date,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY project_id
ORDER BY (project_id, provider, domain, source_url, target_url)
`

const CreateProviderRefDomains = `
CREATE TABLE IF NOT EXISTS crawlobserver.provider_refdomains (
    project_id String,
    provider String,
    domain String,
    ref_domain String,
    backlink_count Int64,
    domain_rank Float64,
    first_seen Date,
    last_seen Date,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY project_id
ORDER BY (project_id, provider, domain, ref_domain)
`

const CreateProviderRankings = `
CREATE TABLE IF NOT EXISTS crawlobserver.provider_rankings (
    project_id String,
    provider String,
    domain String,
    keyword String,
    url String,
    search_base String,
    position UInt16,
    search_volume Int64,
    cpc Float64,
    traffic Float64,
    traffic_pct Float64,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY project_id
ORDER BY (project_id, provider, domain, search_base, keyword)
`

const CreateProviderVisibility = `
CREATE TABLE IF NOT EXISTS crawlobserver.provider_visibility (
    project_id String,
    provider String,
    domain String,
    search_base String,
    date Date,
    visibility Float64,
    keywords_count Int64,
    fetched_at DateTime64(3)
) ENGINE = ReplacingMergeTree(fetched_at)
PARTITION BY project_id
ORDER BY (project_id, provider, domain, search_base, date)
`

const CreateApplicationLogs = `
CREATE TABLE IF NOT EXISTS crawlobserver.application_logs (
    timestamp DateTime64(3),
    level LowCardinality(String),
    component LowCardinality(String),
    message String,
    context String DEFAULT ''
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (timestamp, level, component)
TTL timestamp + INTERVAL 7 DAY DELETE
`

// repartitionTable migrates a table to use PARTITION BY crawl_session_id.
// It checks the current partition key first and skips if already correct.
func repartitionTable(ctx context.Context, conn driver.Conn, table, createV2DDL string) error {
	// Check current partition expression
	var partitionKey string
	err := conn.QueryRow(ctx,
		`SELECT partition_key FROM system.tables WHERE database = 'crawlobserver' AND name = ?`, table,
	).Scan(&partitionKey)
	if err != nil {
		// Table might not exist yet — skip
		return nil
	}
	if partitionKey == "crawl_session_id" {
		log.Printf("  %s: already partitioned by crawl_session_id, skipping", table)
		return nil
	}

	log.Printf("  %s: repartitioning (current: %q) → crawl_session_id", table, partitionKey)

	// Create v2 table
	if err := conn.Exec(ctx, createV2DDL); err != nil {
		return fmt.Errorf("creating %s_v2: %w", table, err)
	}

	// Copy data
	copySQL := fmt.Sprintf("INSERT INTO crawlobserver.%s_v2 SELECT * FROM crawlobserver.%s", table, table)
	if err := conn.Exec(ctx, copySQL); err != nil {
		// Clean up v2 on failure
		conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS crawlobserver.%s_v2", table))
		return fmt.Errorf("copying data to %s_v2: %w", table, err)
	}

	// Atomic swap
	renameSQL := fmt.Sprintf(
		"RENAME TABLE crawlobserver.%s TO crawlobserver.%s_old, crawlobserver.%s_v2 TO crawlobserver.%s",
		table, table, table, table,
	)
	if err := conn.Exec(ctx, renameSQL); err != nil {
		conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS crawlobserver.%s_v2", table))
		return fmt.Errorf("swapping %s: %w", table, err)
	}

	// Drop old table
	if err := conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS crawlobserver.%s_old", table)); err != nil {
		log.Printf("  warning: failed to drop %s_old: %v", table, err)
	}

	log.Printf("  %s: repartitioned successfully", table)
	return nil
}

// migrateRepartitionBySession repartitions all data tables by crawl_session_id.
func migrateRepartitionBySession(ctx context.Context, conn driver.Conn) error {
	log.Println("Running repartition migration...")

	tables := []struct {
		name      string
		createDDL string
	}{
		{"pages", CreatePagesV2},
		{"links", CreateLinksV2},
		{"robots_txt", CreateRobotsTxtV2},
		{"sitemaps", CreateSitemapsV2},
		{"sitemap_urls", CreateSitemapURLsV2},
	}

	for _, t := range tables {
		if err := repartitionTable(ctx, conn, t.name, t.createDDL); err != nil {
			return fmt.Errorf("repartitioning %s: %w", t.name, err)
		}
	}

	log.Println("Repartition migration complete.")
	return nil
}

// Migrations is the ordered list of migrations.
var Migrations = []Migration{
	{Name: "create database", DDL: CreateDatabase},
	{Name: "create crawl_sessions", DDL: CreateCrawlSessions},
	{Name: "create pages", DDL: CreatePages},
	{Name: "create links", DDL: CreateLinks},
	{Name: "alter pages v2", DDL: AlterPagesV2},
	{Name: "alter pages v3", DDL: AlterPagesV3},
	{Name: "alter pages v4", DDL: AlterPagesV4},
	{Name: "create robots_txt", DDL: CreateRobotsTxt},
	{Name: "alter sessions v2", DDL: AlterSessionsV2},
	{Name: "create sitemaps", DDL: CreateSitemaps},
	{Name: "create sitemap_urls", DDL: CreateSitemapURLs},
	{Name: "repartition by session_id", Fn: migrateRepartitionBySession},
	{Name: "create gsc_analytics", DDL: CreateGSCAnalytics},
	{Name: "create gsc_inspection", DDL: CreateGSCInspection},
	{Name: "create external_link_checks", DDL: CreateExternalLinkChecks},
	{Name: "create application_logs", DDL: CreateApplicationLogs},
	{Name: "create provider_domain_metrics", DDL: CreateProviderDomainMetrics},
	{Name: "create provider_backlinks", DDL: CreateProviderBacklinks},
	{Name: "create provider_refdomains", DDL: CreateProviderRefDomains},
	{Name: "create provider_rankings", DDL: CreateProviderRankings},
	{Name: "create provider_visibility", DDL: CreateProviderVisibility},
}
