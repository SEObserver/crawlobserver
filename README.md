<p align="center">
  <h1 align="center">SEOCrawler</h1>
  <p align="center">
    Free, open-source SEO crawler built by <a href="https://www.seobserver.com">SEObserver</a>.<br>
    Extract 45+ SEO signals per page. Store in ClickHouse. Analyze at scale.
  </p>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#web-ui">Web UI</a> &middot;
  <a href="#cli-reference">CLI</a> &middot;
  <a href="#configuration">Config</a> &middot;
  <a href="#api">API</a> &middot;
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

<p align="center">
  <a href="https://github.com/SEObserver/seocrawler/actions"><img src="https://github.com/SEObserver/seocrawler/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT License"></a>
  <a href="https://goreportcard.com/report/github.com/SEObserver/seocrawler"><img src="https://goreportcard.com/badge/github.com/SEObserver/seocrawler" alt="Go Report Card"></a>
</p>

---

## Why SEOCrawler?

At [SEObserver](https://www.seobserver.com), we crawl billions of pages. We built SEOCrawler because every SEO deserves a proper crawler — not a spreadsheet with 10,000 rows, not a SaaS with monthly limits. A real tool that runs on your machine, stores data in a columnar database, and lets you query millions of pages in milliseconds.

**We're giving it to the community for free.** Use it, break it, improve it.

### What it does

- Crawls websites following internal links from seed URLs
- Extracts **45+ SEO signals** per page (title, canonical, meta tags, headings, hreflang, Open Graph, schema.org, images, links, indexability...)
- Respects `robots.txt` and per-host crawl delays
- Tracks redirect chains, response times, and body sizes
- Stores everything in **ClickHouse** (fast columnar queries over millions of pages)
- Computes **PageRank** and **crawl depth** per session
- Comes with a **web UI**, a **REST API**, and a **native desktop app**

---

## Quick Start

**Prerequisites:** Go 1.21+ and Docker.

```bash
# 1. Clone & build
git clone https://github.com/SEObserver/seocrawler.git
cd seocrawler
make build

# 2. Start ClickHouse
docker compose up -d

# 3. Create tables
./seocrawler migrate

# 4. Crawl a site
./seocrawler crawl --seed https://example.com --max-pages 1000

# 5. Browse results
./seocrawler serve
# Open http://127.0.0.1:8899
```

> **Managed mode:** Don't have Docker? SEOCrawler can download and run ClickHouse for you automatically. Set `clickhouse.mode: managed` in your config.

---

## Web UI

Start the web interface with `./seocrawler serve` and open `http://127.0.0.1:8899`.

The UI gives you:

- **Session management** &mdash; start, stop, resume, delete crawl sessions
- **Page explorer** &mdash; filter and browse crawled pages by status code, title, depth, word count...
- **Tabs** &mdash; overview, titles, meta, headings, images, indexability, response codes, internal links, external links
- **PageRank** &mdash; distribution histogram, treemap by path, top-N pages
- **robots.txt tester** &mdash; view robots.txt per host and test URL access
- **Sitemap viewer** &mdash; discover and browse sitemap trees
- **Real-time progress** &mdash; live crawl stats via Server-Sent Events
- **Theming** &mdash; custom accent color, logo, dark mode
- **API key management** &mdash; project-scoped keys for programmatic access

The UI is a single Go binary — no Node.js runtime needed in production.

---

## CLI Reference

```
seocrawler [command]
```

| Command | Description |
|---------|-------------|
| `crawl` | Start a crawl session |
| `serve` | Start the web UI |
| `gui` | Start the native desktop app (macOS) |
| `migrate` | Create or update ClickHouse tables |
| `sessions` | List all crawl sessions |
| `report external-links` | Export external links (table or CSV) |
| `update` | Check for updates and self-update |
| `install-clickhouse` | Download ClickHouse binary for offline use |
| `version` | Print version |

### Crawl examples

```bash
# Single seed URL
seocrawler crawl --seed https://example.com

# Multiple seeds from file (one URL per line)
seocrawler crawl --seeds-file urls.txt

# Fine-tune the crawl
seocrawler crawl --seed https://example.com \
  --workers 20 \
  --delay 500ms \
  --max-pages 50000 \
  --max-depth 10 \
  --store-html
```

### Reports

```bash
# External links as a table
seocrawler report external-links --format table

# Export to CSV
seocrawler report external-links --format csv > external-links.csv

# Filter by session
seocrawler report external-links --session <session-id> --format csv
```

---

## Configuration

Copy `config.example.yaml` to `config.yaml`:

```bash
cp config.example.yaml config.yaml
```

All settings can be overridden via **environment variables** with the `SEOCRAWLER_` prefix (e.g. `SEOCRAWLER_CRAWLER_WORKERS=20`) or via **CLI flags**.

### Key settings

| Setting | Default | Description |
|---------|---------|-------------|
| `crawler.workers` | `10` | Concurrent fetch workers |
| `crawler.delay` | `1s` | Per-host request delay |
| `crawler.max_pages` | `0` | Max pages to crawl (0 = unlimited) |
| `crawler.max_depth` | `0` | Max crawl depth (0 = unlimited) |
| `crawler.timeout` | `30s` | HTTP request timeout |
| `crawler.user_agent` | `SEOCrawler/1.0` | User-Agent string |
| `crawler.respect_robots` | `true` | Obey robots.txt |
| `crawler.store_html` | `false` | Store raw HTML (ZSTD compressed) |
| `crawler.crawl_scope` | `host` | `host` (exact) or `domain` (eTLD+1) |
| `clickhouse.host` | `localhost` | ClickHouse host |
| `clickhouse.port` | `19000` | ClickHouse native protocol port |
| `clickhouse.mode` | _(auto)_ | `managed`, `external`, or auto-detect |
| `server.port` | `8899` | Web UI port |
| `server.username` | `admin` | Basic auth username |
| `server.password` | _(generated)_ | Basic auth password (random if not set) |
| `resources.max_memory_mb` | `0` | Memory soft limit (0 = auto) |
| `resources.max_cpu` | `0` | CPU limit / GOMAXPROCS (0 = all) |

See [`config.example.yaml`](config.example.yaml) for the full reference.

---

## Architecture

```
Seed URLs
    |
    v
Frontier  (priority queue, per-host delay, dedup)
    |
    v
Fetch Workers  (N goroutines, robots.txt cache, redirect tracking)
    |
    v
Parser  (goquery: 45+ SEO signals extracted)
    |
    v
Storage Buffer  (batch insert, configurable flush)
    |
    v
ClickHouse  (columnar storage, partitioned by month)
    |
    |---> Web UI  (Svelte 5, embedded in binary)
    |---> REST API  (40+ endpoints)
    |---> CLI reports
```

### Tech stack

| Layer | Technology |
|-------|-----------|
| Crawler engine | Go, `net/http`, goroutine pool |
| HTML parsing | `goquery` (CSS selectors) |
| URL normalization | `purell` + custom rules |
| robots.txt | `temoto/robotstxt` |
| Storage | ClickHouse (via `clickhouse-go/v2`) |
| API keys / sessions | SQLite (`modernc.org/sqlite`) |
| Web UI | Svelte 5, Vite |
| Desktop app | Wails v2 (macOS) |
| CLI | Cobra + Viper |

---

## API

The REST API is available when running `seocrawler serve`. All endpoints are under `/api/`.

### Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/sessions` | List all sessions |
| `POST` | `/api/crawl` | Start a new crawl |
| `POST` | `/api/sessions/:id/stop` | Stop a running crawl |
| `POST` | `/api/sessions/:id/resume` | Resume a stopped crawl |
| `DELETE` | `/api/sessions/:id` | Delete a session and its data |

### Pages & Links

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/sessions/:id/pages` | Crawled pages (paginated, filterable) |
| `GET` | `/api/sessions/:id/links` | External links |
| `GET` | `/api/sessions/:id/internal-links` | Internal links |
| `GET` | `/api/sessions/:id/page-detail?url=` | Full detail for one URL |
| `GET` | `/api/sessions/:id/page-html?url=` | Raw HTML body |

### Analytics

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/sessions/:id/stats` | Session statistics |
| `GET` | `/api/sessions/:id/events` | Live progress (SSE) |
| `POST` | `/api/sessions/:id/compute-pagerank` | Compute internal PageRank |
| `POST` | `/api/sessions/:id/recompute-depths` | Recompute crawl depths |
| `GET` | `/api/sessions/:id/pagerank-top` | Top pages by PageRank |
| `GET` | `/api/sessions/:id/pagerank-distribution` | PageRank histogram |

### robots.txt & Sitemaps

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/sessions/:id/robots-hosts` | Hosts with robots.txt |
| `GET` | `/api/sessions/:id/robots-content` | robots.txt content |
| `POST` | `/api/sessions/:id/robots-test` | Test URLs against robots.txt |
| `GET` | `/api/sessions/:id/sitemaps` | Discovered sitemaps |

Authentication: Basic Auth or API key (`X-API-Key` header).

---

## Contributing

We welcome contributions. Please read **[CONTRIBUTING.md](CONTRIBUTING.md)** before submitting anything.

**TL;DR:**

- Open an issue before starting significant work
- One PR = one thing (don't mix features and refactors)
- Write tests for new code
- Run `make test && make lint` before pushing
- Follow existing code style — don't reorganize what you didn't change

---

## License

MIT &mdash; see [LICENSE](LICENSE).

Built by [SEObserver](https://www.seobserver.com).
