<script>
  import { getSessions, getStats, getPages, getLinks, getProgress, startCrawl, stopCrawl, resumeCrawl, deleteSession } from './lib/api.js';

  let sessions = $state([]);
  let selectedSession = $state(null);
  let stats = $state(null);
  let pages = $state([]);
  let links = $state([]);
  let view = $state('pages');
  let loading = $state(true);
  let error = $state(null);

  // New crawl form
  let showNewCrawl = $state(false);
  let seedInput = $state('');
  let maxPages = $state(0);
  let maxDepth = $state(0);
  let workers = $state(10);
  let crawlDelay = $state('1s');
  let storeHtml = $state(false);
  let starting = $state(false);

  // Progress polling
  let progressInterval = $state(null);
  let liveProgress = $state({});

  async function loadSessions() {
    try {
      loading = true;
      sessions = await getSessions() || [];
      // Start polling progress for running sessions
      pollRunning();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function pollRunning() {
    if (progressInterval) clearInterval(progressInterval);
    const hasRunning = sessions.some(s => s.is_running);
    if (!hasRunning) return;

    progressInterval = setInterval(async () => {
      const running = sessions.filter(s => s.is_running);
      if (running.length === 0) {
        clearInterval(progressInterval);
        progressInterval = null;
        loadSessions();
        return;
      }
      for (const s of running) {
        try {
          const p = await getProgress(s.ID);
          liveProgress[s.ID] = p;
          if (!p.is_running) {
            loadSessions();
          }
        } catch {}
      }
      // Force reactivity
      liveProgress = { ...liveProgress };
    }, 2000);
  }

  async function selectSession(session) {
    selectedSession = session;
    view = 'pages';
    try {
      [stats, pages, links] = await Promise.all([
        getStats(session.ID),
        getPages(session.ID),
        getLinks(session.ID)
      ]);
      pages = pages || [];
      links = links || [];
    } catch (e) {
      error = e.message;
    }
  }

  async function handleStartCrawl() {
    const seeds = seedInput.split('\n').map(s => s.trim()).filter(Boolean);
    if (seeds.length === 0) return;
    starting = true;
    error = null;
    try {
      await startCrawl(seeds, {
        max_pages: maxPages,
        max_depth: maxDepth,
        workers: workers,
        delay: crawlDelay,
        store_html: storeHtml,
      });
      showNewCrawl = false;
      seedInput = '';
      maxPages = 0;
      maxDepth = 0;
      // Reload sessions after a brief delay to let engine start
      setTimeout(() => loadSessions(), 500);
    } catch (e) {
      error = e.message;
    } finally {
      starting = false;
    }
  }

  async function handleStop(id) {
    try {
      await stopCrawl(id);
      setTimeout(() => loadSessions(), 1000);
    } catch (e) {
      error = e.message;
    }
  }

  async function handleResume(id) {
    try {
      await resumeCrawl(id);
      setTimeout(() => loadSessions(), 500);
    } catch (e) {
      error = e.message;
    }
  }

  async function handleDelete(id) {
    if (!confirm('Delete this session and all its data?')) return;
    try {
      await deleteSession(id);
      if (selectedSession && selectedSession.ID === id) {
        selectedSession = null;
      }
      loadSessions();
    } catch (e) {
      error = e.message;
    }
  }

  function statusBadge(code) {
    if (code >= 200 && code < 300) return 'badge-success';
    if (code >= 300 && code < 400) return 'badge-info';
    if (code >= 400 && code < 500) return 'badge-warning';
    return 'badge-error';
  }

  function formatDuration(ms) {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  }

  function formatSize(bytes) {
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)}KB`;
    return `${(bytes / 1048576).toFixed(1)}MB`;
  }

  function formatNumber(n) {
    return new Intl.NumberFormat().format(n);
  }

  loadSessions();
</script>

<nav>
  <span class="logo">SEOCrawler</span>
  {#if selectedSession}
    <a href="#" onclick={(e) => { e.preventDefault(); selectedSession = null; }}>Sessions</a>
    <span style="color: var(--text-muted)">/</span>
    <span style="color: var(--text)">{selectedSession.SeedURLs?.[0] || selectedSession.ID}</span>
  {:else}
    <span style="color: var(--text)">Sessions</span>
  {/if}
  <div style="margin-left: auto;">
    {#if !selectedSession}
      <button class="btn btn-primary" onclick={() => showNewCrawl = !showNewCrawl}>
        + New Crawl
      </button>
    {/if}
  </div>
</nav>

<div class="container" style="padding-top: 24px; padding-bottom: 48px;">
  {#if error}
    <div class="card" style="border-color: var(--error); margin-bottom: 16px;">
      <p style="color: var(--error);">{error}</p>
      <button class="btn" style="margin-top: 8px;" onclick={() => { error = null; }}>
        Dismiss
      </button>
    </div>
  {/if}

  {#if showNewCrawl && !selectedSession}
    <!-- New Crawl Form -->
    <div class="card" style="margin-bottom: 24px;">
      <h2 style="font-size: 18px; margin-bottom: 16px;">New Crawl</h2>
      <div class="form-grid">
        <div class="form-group" style="grid-column: 1 / -1;">
          <label for="seeds">Seed URLs (one per line)</label>
          <textarea id="seeds" bind:value={seedInput} rows="3"
            placeholder="https://example.com"></textarea>
        </div>
        <div class="form-group">
          <label for="workers">Workers</label>
          <input id="workers" type="number" bind:value={workers} min="1" max="100" />
        </div>
        <div class="form-group">
          <label for="delay">Delay</label>
          <input id="delay" type="text" bind:value={crawlDelay} placeholder="1s" />
        </div>
        <div class="form-group">
          <label for="maxpages">Max pages (0 = unlimited)</label>
          <input id="maxpages" type="number" bind:value={maxPages} min="0" />
        </div>
        <div class="form-group">
          <label for="maxdepth">Max depth (0 = unlimited)</label>
          <input id="maxdepth" type="number" bind:value={maxDepth} min="0" />
        </div>
        <div class="form-group" style="display: flex; align-items: center; gap: 8px; padding-top: 24px;">
          <input id="storehtml" type="checkbox" bind:checked={storeHtml} />
          <label for="storehtml" style="margin: 0;">Store raw HTML</label>
        </div>
      </div>
      <div style="display: flex; gap: 8px; margin-top: 16px;">
        <button class="btn btn-primary" onclick={handleStartCrawl} disabled={starting || !seedInput.trim()}>
          {starting ? 'Starting...' : 'Start Crawl'}
        </button>
        <button class="btn" onclick={() => showNewCrawl = false}>Cancel</button>
      </div>
    </div>
  {/if}

  {#if !selectedSession}
    <!-- Sessions List -->
    <h1 style="font-size: 24px; margin-bottom: 20px;">Crawl Sessions</h1>

    {#if loading}
      <p style="color: var(--text-muted)">Loading...</p>
    {:else if sessions.length === 0}
      <div class="empty-state">
        <h2>No crawl sessions yet</h2>
        <p>Click <strong>+ New Crawl</strong> above to start your first crawl.</p>
      </div>
    {:else}
      <div class="card">
        <table>
          <thead>
            <tr>
              <th>Seed</th>
              <th>Status</th>
              <th>Pages</th>
              <th>Started</th>
              <th style="text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each sessions as session}
              <tr>
                <td style="max-width: 350px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                  {session.SeedURLs?.[0] || '-'}
                </td>
                <td>
                  {#if session.is_running}
                    <span class="badge badge-info">
                      running
                      {#if liveProgress[session.ID]}
                        ({formatNumber(liveProgress[session.ID].pages_crawled)} pages,
                        {formatNumber(liveProgress[session.ID].queue_size)} in queue)
                      {/if}
                    </span>
                  {:else}
                    <span class="badge" class:badge-success={session.Status === 'completed'}
                      class:badge-error={session.Status === 'failed'}
                      class:badge-warning={session.Status === 'stopped'}>
                      {session.Status}
                    </span>
                  {/if}
                </td>
                <td>{formatNumber(session.PagesCrawled)}</td>
                <td>{new Date(session.StartedAt).toLocaleString()}</td>
                <td style="text-align: right; white-space: nowrap;">
                  <div style="display: flex; gap: 4px; justify-content: flex-end;">
                    <button class="btn btn-sm" onclick={() => selectSession(session)}>View</button>
                    {#if session.is_running}
                      <button class="btn btn-sm btn-danger" onclick={() => handleStop(session.ID)}>Stop</button>
                    {:else}
                      <button class="btn btn-sm" onclick={() => handleResume(session.ID)}>Resume</button>
                      <button class="btn btn-sm btn-danger" onclick={() => handleDelete(session.ID)}>Delete</button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {:else}
    <!-- Session Detail -->
    <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 20px;">
      {#if selectedSession.is_running}
        <button class="btn btn-danger" onclick={() => handleStop(selectedSession.ID)}>Stop Crawl</button>
      {:else}
        <button class="btn" onclick={() => handleResume(selectedSession.ID)}>Resume Crawl</button>
        <button class="btn btn-danger" onclick={() => handleDelete(selectedSession.ID)}>Delete Session</button>
      {/if}
      <button class="btn" onclick={() => { selectSession(selectedSession); }}>Refresh</button>
    </div>

    {#if stats}
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">{formatNumber(stats.total_pages)}</div>
          <div class="stat-label">Pages crawled</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{formatNumber(stats.internal_links)}</div>
          <div class="stat-label">Internal links</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{formatNumber(stats.external_links)}</div>
          <div class="stat-label">External links</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{formatDuration(Math.round(stats.avg_fetch_ms))}</div>
          <div class="stat-label">Avg fetch time</div>
        </div>
        <div class="stat-card">
          <div class="stat-value" style="color: var(--error)">{formatNumber(stats.error_count)}</div>
          <div class="stat-label">Errors</div>
        </div>
      </div>
    {/if}

    <!-- Tabs -->
    <div style="display: flex; gap: 8px; margin-bottom: 16px;">
      <button class="btn" class:btn-primary={view === 'pages'} onclick={() => view = 'pages'}>
        Pages
      </button>
      <button class="btn" class:btn-primary={view === 'links'} onclick={() => view = 'links'}>
        External Links
      </button>
    </div>

    {#if view === 'pages'}
      <div class="card">
        <table>
          <thead>
            <tr>
              <th>URL</th>
              <th>Status</th>
              <th>Title</th>
              <th>Size</th>
              <th>Time</th>
              <th>Depth</th>
            </tr>
          </thead>
          <tbody>
            {#each pages as page}
              <tr>
                <td style="max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                  <a href={page.URL} target="_blank" rel="noopener">{page.URL}</a>
                </td>
                <td><span class="badge {statusBadge(page.StatusCode)}">{page.StatusCode}</span></td>
                <td style="max-width: 250px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                  {page.Title || '-'}
                </td>
                <td>{formatSize(page.BodySize)}</td>
                <td>{formatDuration(page.FetchDurationMs)}</td>
                <td>{page.Depth}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <div class="card">
        <table>
          <thead>
            <tr>
              <th>Source</th>
              <th>Target</th>
              <th>Anchor</th>
              <th>Rel</th>
            </tr>
          </thead>
          <tbody>
            {#each links as link}
              <tr>
                <td style="max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                  {link.SourceURL}
                </td>
                <td style="max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                  <a href={link.TargetURL} target="_blank" rel="noopener">{link.TargetURL}</a>
                </td>
                <td style="max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                  {link.AnchorText || '-'}
                </td>
                <td>{link.Rel || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}
</div>
