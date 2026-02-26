# SEOCrawler

Open-source SEO crawler written in Go. Extracts SEO signals (title, canonical, meta tags, headings, links) and stores results in ClickHouse for analysis.

## Quick Start

### Prerequisites

- Go 1.21+
- Docker (for ClickHouse)

### Setup

```bash
# Start ClickHouse
docker compose up -d

# Build
make build

# Create tables
./seocrawler migrate

# Crawl a site
./seocrawler crawl --seed https://example.com --max-pages 100
```

### Commands

```bash
# Crawl from a single seed URL
seocrawler crawl --seed https://example.com

# Crawl from a file (one URL per line)
seocrawler crawl --seeds-file urls.txt

# Crawl with options
seocrawler crawl --seed https://example.com --delay 2s --max-pages 5000 --workers 20

# Run database migrations
seocrawler migrate

# List crawl sessions
seocrawler sessions

# External links report
seocrawler report external-links --format table
seocrawler report external-links --format csv > links.csv
```

## Configuration

Copy `config.example.yaml` to `config.yaml` and edit. All settings can be overridden via environment variables with prefix `SEOCRAWLER_` or via CLI flags.

| Setting | Default | Description |
|---------|---------|-------------|
| `crawler.workers` | 10 | Concurrent fetch workers |
| `crawler.delay` | 1s | Per-host request delay |
| `crawler.max_pages` | 0 | Max pages (0 = unlimited) |
| `crawler.max_depth` | 0 | Max crawl depth (0 = unlimited) |
| `crawler.timeout` | 30s | HTTP request timeout |
| `crawler.user_agent` | SEOCrawler/1.0 | User-Agent string |
| `crawler.respect_robots` | true | Obey robots.txt |
| `clickhouse.host` | localhost | ClickHouse host |
| `clickhouse.port` | 9000 | ClickHouse native port |

## Architecture

```
Seed URL(s) → Frontier (priority queue + per-host delay)
                  ↓
            Fetch Workers (net/http, N goroutines)
              ↕ robots.txt cache
                  ↓
            Parse Workers (goquery, M goroutines)
                  ↓
            Storage Buffer → batch INSERT → ClickHouse
                  ↓
            Discovered URLs → Normalizer → Dedup → Frontier
```

## License

MIT
