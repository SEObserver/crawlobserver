<p align="center">
  <h1 align="center">CrawlObserver</h1>
  <p align="center">
    Free, open-source SEO crawler built by <a href="https://www.seobserver.com">SEObserver</a>.<br>
    Extract 45+ SEO signals per page. Query millions of pages in milliseconds.
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
  <a href="https://github.com/SEObserver/crawlobserver/actions"><img src="https://github.com/SEObserver/crawlobserver/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/SEObserver/crawlobserver/releases/latest"><img src="https://img.shields.io/github/v/release/SEObserver/crawlobserver" alt="Latest Release"></a>
  <a href="https://github.com/SEObserver/crawlobserver/releases"><img src="https://img.shields.io/github/downloads/SEObserver/crawlobserver/total" alt="Downloads"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue.svg" alt="AGPL-3.0 License"></a>
  <a href="https://goreportcard.com/report/github.com/SEObserver/crawlobserver"><img src="https://goreportcard.com/badge/github.com/SEObserver/crawlobserver" alt="Go Report Card"></a>
</p>

<p align="center">
  <img src="screenshot.png" alt="CrawlObserver Web UI" width="900">
</p>

---

## Why CrawlObserver?

At [SEObserver](https://www.seobserver.com), we crawl billions of pages. We built CrawlObserver because every SEO deserves a proper crawler — not a spreadsheet with 10,000 rows, not a SaaS with monthly limits. A real tool that runs on your machine, stores data in a columnar database, and lets you query millions of pages in milliseconds.

**We're giving it to the community for free.** Use it, break it, improve it.

### What it does

- Crawls websites following internal links from seed URLs
- Extracts **45+ SEO signals** per page (title, canonical, meta tags, headings, hreflang, Open Graph, schema.org, images, links, indexability...)
- Respects `robots.txt` and per-host crawl delays
- Tracks redirect chains, response times, and body sizes
- Stores everything in a columnar database for instant analytical queries
- Computes **PageRank** and **crawl depth** per session
- Comes with a **web UI**, a **REST API**, and a **native desktop app**

---

## Quick Start

```bash
curl -fsSL https://raw.githubusercontent.com/SEObserver/crawlobserver/main/install.sh | sh
crawlobserver
```

That's it. Open `http://127.0.0.1:8899` — the setup wizard guides you through the rest. No Docker, no dependencies, no manual setup. CrawlObserver downloads and manages its own database on first run.

> **macOS desktop app:** download the DMG from the [latest release](https://github.com/SEObserver/crawlobserver/releases/latest).

<details>
<summary><strong>Build from source</strong></summary>

Requires Go 1.25+:

```bash
git clone https://github.com/SEObserver/crawlobserver.git
cd crawlobserver
make build
./crawlobserver
```

</details>

> **Advanced:** You can also point CrawlObserver at an existing database instance (Docker, remote server...). See the [Configuration](#configuration) section for `clickhouse.*` settings.

---

## Web UI

Start the web interface with `./crawlobserver serve` and open `http://127.0.0.1:8899`.

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
crawlobserver [command]
```

| Command | Description |
|---------|-------------|
| `crawl` | Start a crawl session |
| `serve` | Start the web server and browser UI |
| `migrate` | Create or update database tables |
| `sessions` | List all crawl sessions |
| `report external-links` | Export external links (table or CSV) |
| `update` | Check for updates and self-update |
| `install-clickhouse` | Download database binary for offline use |
| `version` | Print version |

### Crawl examples

```bash
# Single seed URL
crawlobserver crawl --seed https://example.com

# Multiple seeds from file (one URL per line)
crawlobserver crawl --seeds-file urls.txt

# Fine-tune the crawl
crawlobserver crawl --seed https://example.com \
  --workers 20 \
  --delay 500ms \
  --max-pages 50000 \
  --max-depth 10 \
  --store-html
```

### Reports

```bash
# External links as a table
crawlobserver report external-links --format table

# Export to CSV
crawlobserver report external-links --format csv > external-links.csv

# Filter by session
crawlobserver report external-links --session <session-id> --format csv
```

---

## Configuration

Copy `config.example.yaml` to `config.yaml`:

```bash
cp config.example.yaml config.yaml
```

All settings can be overridden via **environment variables** with the `CRAWLOBSERVER_` prefix (e.g. `CRAWLOBSERVER_CRAWLER_WORKERS=20`) or via **CLI flags**.

### Key settings

| Setting | Default | Description |
|---------|---------|-------------|
| `crawler.workers` | `10` | Concurrent fetch workers |
| `crawler.delay` | `1s` | Per-host request delay |
| `crawler.max_pages` | `0` | Max pages to crawl (0 = unlimited) |
| `crawler.max_depth` | `0` | Max crawl depth (0 = unlimited) |
| `crawler.timeout` | `30s` | HTTP request timeout |
| `crawler.user_agent` | `CrawlObserver/1.0` | User-Agent string |
| `crawler.respect_robots` | `true` | Obey robots.txt |
| `crawler.store_html` | `false` | Store raw HTML (ZSTD compressed) |
| `crawler.crawl_scope` | `host` | `host`, `domain` (eTLD+1), or `subdirectory` |
| `clickhouse.host` | `localhost` | Database host |
| `clickhouse.port` | `19000` | Database native protocol port |
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
Columnar DB  (partitioned by crawl session, managed automatically)
    |
    |---> Web UI  (Svelte 5, embedded in binary)
    |---> REST API  (40+ endpoints)
    |---> CLI reports
```

<details>
<summary><strong>Why a columnar database?</strong></summary>

A crawl is a link graph, so why not a graph database? Because **a crawler is an analytics pipeline, not a graph explorer.** The questions you ask are analytical — "show me all pages with a missing H1 and a 301 canonical", "give me PageRank percentiles by subdirectory" — and columnar databases answer these instantly, even over millions of rows.

When we need graph algorithms (PageRank, crawl depth), we compute them in-memory in Go and write the results back. A million-page link graph fits in ~200MB of RAM and computes in seconds — no need for a graph database.

Under the hood, CrawlObserver uses ClickHouse in managed mode: it downloads a static binary and runs it as a subprocess. You see one program; it gets concurrent read/write access, columnar compression (~10:1), and instant session deletion.

</details>

### Tech stack

| Layer | Technology |
|-------|-----------|
| Crawler engine | Go, `net/http`, goroutine pool, HTTP/2 (via `utls` ALPN negotiation) |
| TLS fingerprinting | `refraction-networking/utls` (Chrome/Firefox/Edge profiles) |
| HTML parsing | `goquery` (CSS selectors) |
| URL normalization | `purell` + custom rules |
| robots.txt | `temoto/robotstxt` |
| Storage | ClickHouse (via `clickhouse-go/v2`) |
| API keys / sessions | SQLite (`modernc.org/sqlite`) |
| Web UI | Svelte 5, Vite (zero runtime dependencies) |
| Desktop app | webview (macOS) |
| CLI | Cobra + Viper |

---

## API

The REST API is available when running `crawlobserver serve`. All endpoints are under `/api/`.

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

## Acknowledgments

Thanks to the people who helped shape CrawlObserver with their feedback, testing, and ideas:

- **Fabien Raquidel** &mdash; [referenceur-web.pro](https://www.referenceur-web.pro/) · [@fabienr34](https://x.com/fabienr34)
- **Jean-Benoît Moingt** &mdash; [watussi.fr](https://www.watussi.fr/) · [@jeanbenoit](https://x.com/jeanbenoit)

---

## License

AGPL-3.0 &mdash; see [LICENSE](LICENSE).

Built by [SEObserver](https://www.seobserver.com).
