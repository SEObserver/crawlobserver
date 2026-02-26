package storage

// DDL statements for ClickHouse tables.

const CreateDatabase = `CREATE DATABASE IF NOT EXISTS seocrawler`

const CreateCrawlSessions = `
CREATE TABLE IF NOT EXISTS seocrawler.crawl_sessions (
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
CREATE TABLE IF NOT EXISTS seocrawler.pages (
    crawl_session_id UUID,
    url String,
    final_url String,
    status_code UInt16,
    content_type String,
    title String,
    canonical String,
    meta_robots String,
    meta_description String,
    h1 Array(String),
    h2 Array(String),
    h3 Array(String),
    h4 Array(String),
    h5 Array(String),
    h6 Array(String),
    headers Map(String, String),
    redirect_chain Array(Tuple(url String, status_code UInt16)),
    body_size UInt64,
    fetch_duration_ms UInt64,
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
CREATE TABLE IF NOT EXISTS seocrawler.links (
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

// Migrations is the ordered list of DDL statements.
var Migrations = []string{
	CreateDatabase,
	CreateCrawlSessions,
	CreatePages,
	CreateLinks,
}
