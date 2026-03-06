# Contributing to CrawlObserver

CrawlObserver is maintained by [SEObserver](https://www.seobserver.com) and open to community contributions. This document explains how to contribute effectively and what we expect from pull requests.

## Ground Rules

1. **Open an issue first.** Before writing code, open an issue describing what you want to do. This avoids wasted effort on things we won't merge. Bug fixes for obvious issues can skip this.

2. **One PR = one thing.** Don't bundle a bug fix with a refactor with a new feature. Keep PRs focused and reviewable. If you spot something unrelated while working, open a separate issue.

3. **Don't refactor code you didn't change.** If your PR adds a feature to `server.go`, don't also rename variables in `crawler.go`. We review diffs, and noise makes that harder.

4. **Write tests.** New code needs tests. Bug fixes need a test that reproduces the bug. We won't merge code that lowers coverage without good reason.

5. **Run CI locally.** Before pushing:
   ```bash
   make test          # Tests pass
   make lint          # No lint errors
   cd frontend && npm run build  # Frontend builds without errors
   ```

6. **Keep commits clean.** Squash fixup commits. Write meaningful commit messages. The first line should be imperative, under 72 characters: `Fix redirect chain tracking for 307 responses`, not `fixed stuff`.

## Development Setup

### Prerequisites

- Go 1.21+
- Node.js 18+ (for the frontend)
- Docker (for ClickHouse)
- [golangci-lint](https://golangci-lint.run/usage/install/) (for linting)

### Getting started

```bash
git clone https://github.com/SEObserver/crawlobserver.git
cd crawlobserver

# Start ClickHouse
docker compose up -d

# Build everything
make build

# Run migrations
./crawlobserver migrate

# Run the web UI (with hot-reload on the backend)
./crawlobserver serve
```

### Frontend development

```bash
cd frontend
npm install
npm run dev    # Vite dev server with HMR
```

The frontend is a Svelte 5 app (`frontend/src/App.svelte`) bundled with Vite. On production builds, it's embedded into the Go binary via `//go:embed`.

**Build cycle:**
```bash
cd frontend && npm run build && cd ..
rm -rf internal/server/frontend/dist
cp -r frontend/dist internal/server/frontend/dist
go build -o crawlobserver ./cmd/crawlobserver
```

Or just `make build`.

## Project Structure

```
cmd/crawlobserver/         Entry point
internal/
  cli/                  Cobra commands (crawl, serve, gui, migrate...)
  config/               Viper config loading + validation
  crawler/              Crawl orchestrator
  fetcher/              HTTP fetcher with redirect tracking
  parser/               HTML parser (45+ SEO signals)
  frontier/             URL priority queue with per-host delay
  normalizer/           URL normalization
  storage/              ClickHouse read/write + buffer
  server/               HTTP API + embedded frontend
  apikeys/              API key management (SQLite)
  clickhouse/           Managed ClickHouse subprocess
  report/               CLI report generation
  updater/              Self-update
frontend/
  src/App.svelte        Main UI component (Svelte 5)
  src/lib/api.js        API client
  src/app.css           Styles (CSS variables)
```

## Code Guidelines

### Go

- **Standard library first.** Don't add a dependency when `net/http` or `strings` works fine.
- **Error handling.** Always handle errors. No `_ = something()` unless there's a comment explaining why.
- **Naming.** Follow Go conventions. Unexported by default. Short variable names in small scopes, descriptive names in larger ones.
- **No global state.** Pass dependencies explicitly. The `Store`, `Server`, and `Config` structs exist for a reason.
- **SQL safety.** Never concatenate user input into queries. Use parameterized queries or validated values only. See `storage/clickhouse.go` for the temp-table pattern we use for batch updates.
- **Logging.** Use `log.Printf` with context. Include the function name or operation for grep-ability: `log.Printf("ComputePageRank: computed for %d pages in session %s", n, sessionID)`.

### Frontend (Svelte)

- **Svelte 5 runes.** Use `$state()` for reactive state. No Svelte 4 stores.
- **Accessibility.** Interactive elements need `role`, `tabindex`, and keyboard handlers. No `svelte-ignore a11y_*` comments.
- **API calls.** All API calls go through `frontend/src/lib/api.js`. Don't `fetch()` directly from components.
- **CSS.** Use CSS variables from `app.css`. No inline colors or hardcoded values.

## What We Accept

- Bug fixes with a reproducing test
- Performance improvements with benchmarks
- New SEO signals in the parser (with tests)
- UI improvements that don't break existing workflows
- Documentation improvements
- ClickHouse query optimizations

## What We Don't Accept

- **Feature creep.** CrawlObserver crawls websites and extracts SEO signals. It's not a keyword tracker, a rank checker, or a backlink monitor. Stay focused.
- **Massive refactors without prior discussion.** If you want to restructure the project, open an issue and make the case first.
- **Dependency bloat.** Think twice before adding a dependency. If it saves 10 lines of code, it's not worth the supply chain risk.
- **Breaking changes to the API.** The REST API is used by other tools. Breaking changes need a deprecation path and a very good reason.
- **AI-generated PRs without review.** If you use AI tools, review and understand every line before submitting. We can tell when someone submits code they don't understand.

## Pull Request Process

1. Fork the repo and create a branch from `main`.
2. Make your changes. Write tests. Run `make test && make lint`.
3. Push and open a PR against `main`.
4. Fill in the PR template: what changed, why, and how to test it.
5. A maintainer will review. Expect feedback — we review thoroughly.
6. Once approved, a maintainer will merge.

**PR title format:** Start with a verb. `Add`, `Fix`, `Update`, `Remove`, `Refactor`.

**Examples:**
- `Fix redirect chain losing cookies on 307`
- `Add hreflang cluster validation to parser`
- `Update ClickHouse batch size for large crawls`

## Reporting Bugs

Open an issue with:

1. **What you expected** vs. **what happened**
2. **Steps to reproduce** (seed URL, config, command)
3. **Version** (`crawlobserver version`)
4. **Logs** (relevant output, not the entire terminal)

## Security

If you find a security vulnerability, **do not open a public issue**. Email security@crawlobserver.com instead.

---

Thanks for contributing. We appreciate your time.
