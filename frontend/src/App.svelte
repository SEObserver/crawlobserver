<script>
  import { getSessions, getStats, getPages, getLinks } from './lib/api.js';

  let sessions = $state([]);
  let selectedSession = $state(null);
  let stats = $state(null);
  let pages = $state([]);
  let links = $state([]);
  let view = $state('pages');
  let loading = $state(true);
  let error = $state(null);

  async function loadSessions() {
    try {
      loading = true;
      sessions = await getSessions() || [];
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
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
</nav>

<div class="container" style="padding-top: 24px; padding-bottom: 48px;">
  {#if error}
    <div class="card" style="border-color: var(--error); margin-bottom: 16px;">
      <p style="color: var(--error);">{error}</p>
      <button class="btn" style="margin-top: 8px;" onclick={() => { error = null; loadSessions(); }}>
        Retry
      </button>
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
        <p>Run <code>seocrawler crawl --seed https://example.com</code> to start.</p>
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
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each sessions as session}
              <tr>
                <td>{session.SeedURLs?.[0] || '-'}</td>
                <td>
                  <span class="badge" class:badge-success={session.Status === 'completed'}
                    class:badge-info={session.Status === 'running'}
                    class:badge-error={session.Status === 'failed'}>
                    {session.Status}
                  </span>
                </td>
                <td>{session.PagesCrawled}</td>
                <td>{new Date(session.StartedAt).toLocaleString()}</td>
                <td>
                  <button class="btn" onclick={() => selectSession(session)}>View</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {:else}
    <!-- Session Detail -->
    {#if stats}
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">{stats.total_pages}</div>
          <div class="stat-label">Pages crawled</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{stats.internal_links}</div>
          <div class="stat-label">Internal links</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{stats.external_links}</div>
          <div class="stat-label">External links</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{formatDuration(Math.round(stats.avg_fetch_ms))}</div>
          <div class="stat-label">Avg fetch time</div>
        </div>
        <div class="stat-card">
          <div class="stat-value" style="color: var(--error)">{stats.error_count}</div>
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
