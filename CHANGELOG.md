# Changelog

All notable changes to CrawlObserver are documented here.

## Unreleased

This is the initial open-source release of CrawlObserver by [SEObserver](https://www.seobserver.com).

### Crawler Engine
- Concurrent crawl workers with per-host delay and robots.txt compliance
- 45+ SEO signals extracted per page (title, canonical, meta tags, headings, hreflang, Open Graph, schema.org, images, links, indexability)
- Redirect chain tracking with full hop-by-hop detail
- Sitemap-only crawl mode (`--sitemap-only`) to skip link following
- Configurable crawl scope: `host` (exact match) or `domain` (eTLD+1)
- Per-crawl User-Agent override with browser presets
- TLS fingerprinting via utls to match User-Agent identity
- SSRF protection: private IP blocking, DNS rebinding defense
- Per-status-code retry policy with configurable backoff
- Disk-full resilience: auto-stop on data loss, unlimited resume

### Storage
- ClickHouse backend with columnar storage, partitioned by month
- Managed mode: auto-download and run ClickHouse without Docker
- Batch insert buffer with configurable flush interval
- ZSTD-compressed HTML storage (opt-in)

### Web UI
- Svelte 5 frontend embedded in the Go binary
- Session management: start, stop, resume, delete, compare
- Page explorer with filtering by status code, content type, depth, word count
- Tabs: overview, titles, meta, headings, images, indexability, response codes, internal/external links
- PageRank: distribution histogram, treemap by path, top-N pages
- robots.txt tester and sitemap viewer
- Google Search Console integration
- Real-time crawl progress via Server-Sent Events
- Custom accent color, dark mode, SEObserver branding
- API key management with project-scoped access

### CLI
- `crawl` — start a crawl with seed URLs or seeds file
- `serve` — start the web server and REST API
- `migrate` — create or update ClickHouse tables
- `sessions` — list crawl sessions
- `report external-links` — export external links (table or CSV)
- `update` — self-update from GitHub releases
- `install-clickhouse` — download ClickHouse binary for managed mode

### API
- 40+ REST endpoints for sessions, pages, links, analytics, robots.txt, sitemaps
- Basic Auth and API key authentication (`X-API-Key` header)
- Paginated responses with filtering and sorting

### Security
- Parameterized SQL queries throughout
- Constant-time API key comparison
- SHA256-hashed API key storage with salt
- Content Security Policy headers
- Input validation on all user-facing endpoints
