<script>
  import { getSessions, getStats, getPages, getExternalLinks, getInternalLinks, getProgress,
    startCrawl, stopCrawl, resumeCrawl, deleteSession, subscribeProgress } from './lib/api.js';

  let sessions = $state([]);
  let selectedSession = $state(null);
  let stats = $state(null);
  let pages = $state([]);
  let extLinks = $state([]);
  let intLinks = $state([]);
  let tab = $state('overview');
  let loading = $state(true);
  let error = $state(null);

  // Pagination
  const PAGE_SIZE = 100;
  let pagesOffset = $state(0);
  let extLinksOffset = $state(0);
  let intLinksOffset = $state(0);
  let hasMorePages = $state(false);
  let hasMoreExtLinks = $state(false);
  let hasMoreIntLinks = $state(false);

  // New crawl form
  let showNewCrawl = $state(false);
  let seedInput = $state('');
  let maxPages = $state(0);
  let maxDepth = $state(0);
  let workers = $state(10);
  let crawlDelay = $state('1s');
  let storeHtml = $state(false);
  let starting = $state(false);

  // Live progress
  let liveProgress = $state({});
  let sseConnections = {};

  // --- URL Routing ---
  function pushURL(path) {
    if (window.location.pathname !== path) {
      history.pushState(null, '', path);
    }
  }

  function parseRoute() {
    const path = window.location.pathname;
    const m = path.match(/^\/sessions\/([^/]+)(?:\/([^/]+))?/);
    if (m) {
      return { sessionId: m[1], tab: m[2] || 'overview' };
    }
    return null;
  }

  async function navigateTo(path) {
    pushURL(path);
    await applyRoute();
  }

  async function applyRoute() {
    const route = parseRoute();
    if (route) {
      // Session detail view
      tab = route.tab;
      pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
      if (!selectedSession || selectedSession.ID !== route.sessionId) {
        // Need to load session
        if (sessions.length === 0) {
          await loadSessions();
        }
        const found = sessions.find(s => s.ID === route.sessionId);
        if (found) {
          selectedSession = found;
          stats = await getStats(found.ID);
          await loadTabData();
        }
      } else {
        await loadTabData();
      }
    } else {
      // Sessions list
      selectedSession = null;
      stats = null;
      await loadSessions();
    }
  }

  // Listen for back/forward navigation
  window.addEventListener('popstate', () => applyRoute());

  async function selectSession(session) {
    selectedSession = session;
    tab = 'overview';
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    pushURL(`/sessions/${session.ID}/overview`);
    try {
      stats = await getStats(session.ID);
      await loadTabData();
    } catch (e) {
      error = e.message;
    }
  }

  function goHome() {
    selectedSession = null;
    stats = null;
    pushURL('/');
  }

  async function loadSessions() {
    try {
      loading = true;
      sessions = await getSessions() || [];
      for (const s of sessions) {
        if (s.is_running && !sseConnections[s.ID]) {
          sseConnections[s.ID] = subscribeProgress(s.ID,
            (data) => { liveProgress[s.ID] = data; liveProgress = { ...liveProgress }; },
            () => { delete sseConnections[s.ID]; loadSessions(); }
          );
        }
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function loadTabData() {
    if (!selectedSession) return;
    const id = selectedSession.ID;
    try {
      if (['overview','titles','meta','headings','images','indexability','response'].includes(tab)) {
        const result = await getPages(id, PAGE_SIZE, pagesOffset);
        pages = result || [];
        hasMorePages = pages.length === PAGE_SIZE;
      } else if (tab === 'internal') {
        const result = await getInternalLinks(id, PAGE_SIZE, intLinksOffset);
        intLinks = result || [];
        hasMoreIntLinks = intLinks.length === PAGE_SIZE;
      } else if (tab === 'external') {
        const result = await getExternalLinks(id, PAGE_SIZE, extLinksOffset);
        extLinks = result || [];
        hasMoreExtLinks = extLinks.length === PAGE_SIZE;
      }
    } catch (e) {
      error = e.message;
    }
  }

  function switchTab(newTab) {
    tab = newTab;
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (selectedSession) {
      pushURL(`/sessions/${selectedSession.ID}/${newTab}`);
    }
    loadTabData();
  }

  async function nextPage() {
    if (tab === 'internal') { intLinksOffset += PAGE_SIZE; }
    else if (tab === 'external') { extLinksOffset += PAGE_SIZE; }
    else { pagesOffset += PAGE_SIZE; }
    await loadTabData();
  }
  async function prevPage() {
    if (tab === 'internal') { intLinksOffset = Math.max(0, intLinksOffset - PAGE_SIZE); }
    else if (tab === 'external') { extLinksOffset = Math.max(0, extLinksOffset - PAGE_SIZE); }
    else { pagesOffset = Math.max(0, pagesOffset - PAGE_SIZE); }
    await loadTabData();
  }

  function currentOffset() {
    if (tab === 'internal') return intLinksOffset;
    if (tab === 'external') return extLinksOffset;
    return pagesOffset;
  }
  function currentData() {
    if (tab === 'internal') return intLinks;
    if (tab === 'external') return extLinks;
    return pages;
  }
  function hasMore() {
    if (tab === 'internal') return hasMoreIntLinks;
    if (tab === 'external') return hasMoreExtLinks;
    return hasMorePages;
  }

  async function handleStartCrawl() {
    const seeds = seedInput.split('\n').map(s => s.trim()).filter(Boolean);
    if (seeds.length === 0) return;
    starting = true;
    error = null;
    try {
      await startCrawl(seeds, { max_pages: maxPages, max_depth: maxDepth, workers, delay: crawlDelay, store_html: storeHtml });
      showNewCrawl = false;
      seedInput = '';
      maxPages = 0;
      maxDepth = 0;
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
      // Redirect to session detail after stopping
      setTimeout(async () => {
        await loadSessions();
        const sess = sessions.find(s => s.ID === id);
        if (sess && selectedSession?.ID !== id) {
          selectSession(sess);
        }
      }, 1000);
    } catch (e) { error = e.message; }
  }

  async function handleResume(id) {
    try {
      await resumeCrawl(id);
      // Reload sessions and navigate to the resumed session
      await loadSessions();
      const sess = sessions.find(s => s.ID === id);
      if (sess) {
        await selectSession(sess);
      }
    } catch (e) { error = e.message; }
  }

  async function handleDelete(id) {
    if (!confirm('Delete this session and all its data?')) return;
    try {
      await deleteSession(id);
      if (selectedSession?.ID === id) { selectedSession = null; pushURL('/'); }
      loadSessions();
    } catch (e) { error = e.message; }
  }

  function statusBadge(code) {
    if (code >= 200 && code < 300) return 'badge-success';
    if (code >= 300 && code < 400) return 'badge-info';
    if (code >= 400 && code < 500) return 'badge-warning';
    return 'badge-error';
  }

  function fmt(ms) { return ms < 1000 ? `${ms}ms` : `${(ms/1000).toFixed(1)}s`; }
  function fmtSize(b) { return b < 1024 ? `${b}B` : b < 1048576 ? `${(b/1024).toFixed(1)}KB` : `${(b/1048576).toFixed(1)}MB`; }
  function fmtN(n) { return new Intl.NumberFormat().format(n); }
  function trunc(s, n) { return s && s.length > n ? s.slice(0, n) + '...' : (s || '-'); }

  const TABS = [
    { id: 'overview', label: 'All Pages' },
    { id: 'titles', label: 'Titles' },
    { id: 'meta', label: 'Meta' },
    { id: 'headings', label: 'H1/H2' },
    { id: 'images', label: 'Images' },
    { id: 'indexability', label: 'Indexability' },
    { id: 'response', label: 'Response' },
    { id: 'internal', label: 'Internal Links' },
    { id: 'external', label: 'External Links' },
  ];

  // Boot: apply current URL route
  applyRoute();
</script>

<nav>
  <span class="logo">SEOCrawler</span>
  {#if selectedSession}
    <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
    <span style="color: var(--text-muted)">/</span>
    <span style="color: var(--text)">{selectedSession.SeedURLs?.[0] || selectedSession.ID}</span>
  {:else}
    <span style="color: var(--text)">Sessions</span>
  {/if}
  <div style="margin-left: auto;">
    {#if !selectedSession}
      <button class="btn btn-primary" onclick={() => showNewCrawl = !showNewCrawl}>+ New Crawl</button>
    {/if}
  </div>
</nav>

<div class="container" style="padding-top: 24px; padding-bottom: 48px;">
  {#if error}
    <div class="card" style="border-color: var(--error); margin-bottom: 16px;">
      <p style="color: var(--error);">{error}</p>
      <button class="btn" style="margin-top: 8px;" onclick={() => error = null}>Dismiss</button>
    </div>
  {/if}

  {#if showNewCrawl && !selectedSession}
    <div class="card" style="margin-bottom: 24px;">
      <h2 style="font-size: 18px; margin-bottom: 16px;">New Crawl</h2>
      <div class="form-grid">
        <div class="form-group" style="grid-column: 1 / -1;">
          <label for="seeds">Seed URLs (one per line)</label>
          <textarea id="seeds" bind:value={seedInput} rows="3" placeholder="https://example.com"></textarea>
        </div>
        <div class="form-group"><label for="workers">Workers</label><input id="workers" type="number" bind:value={workers} min="1" max="100" /></div>
        <div class="form-group"><label for="delay">Delay</label><input id="delay" type="text" bind:value={crawlDelay} placeholder="1s" /></div>
        <div class="form-group"><label for="maxpages">Max pages (0 = unlimited)</label><input id="maxpages" type="number" bind:value={maxPages} min="0" /></div>
        <div class="form-group"><label for="maxdepth">Max depth (0 = unlimited)</label><input id="maxdepth" type="number" bind:value={maxDepth} min="0" /></div>
        <div class="form-group" style="display: flex; align-items: center; gap: 8px; padding-top: 24px;">
          <input id="storehtml" type="checkbox" bind:checked={storeHtml} /><label for="storehtml" style="margin: 0;">Store raw HTML</label>
        </div>
      </div>
      <div style="display: flex; gap: 8px; margin-top: 16px;">
        <button class="btn btn-primary" onclick={handleStartCrawl} disabled={starting || !seedInput.trim()}>{starting ? 'Starting...' : 'Start Crawl'}</button>
        <button class="btn" onclick={() => showNewCrawl = false}>Cancel</button>
      </div>
    </div>
  {/if}

  {#if !selectedSession}
    <h1 style="font-size: 24px; margin-bottom: 20px;">Crawl Sessions</h1>
    {#if loading}
      <p style="color: var(--text-muted)">Loading...</p>
    {:else if sessions.length === 0}
      <div class="empty-state"><h2>No crawl sessions yet</h2><p>Click <strong>+ New Crawl</strong> above to start.</p></div>
    {:else}
      <div class="card">
        <table>
          <thead><tr><th>Seed</th><th>Status</th><th>Pages</th><th>Started</th><th style="text-align: right;">Actions</th></tr></thead>
          <tbody>
            {#each sessions as s}
              <tr>
                <td class="cell-url">{s.SeedURLs?.[0] || '-'}</td>
                <td>
                  {#if s.is_running}
                    <span class="badge badge-info">running {#if liveProgress[s.ID]}({fmtN(liveProgress[s.ID].pages_crawled)} pages, {fmtN(liveProgress[s.ID].queue_size)} queued){/if}</span>
                  {:else}
                    <span class="badge" class:badge-success={s.Status==='completed'} class:badge-error={s.Status==='failed'} class:badge-warning={s.Status==='stopped'}>{s.Status}</span>
                  {/if}
                </td>
                <td>{fmtN(s.PagesCrawled)}</td>
                <td>{new Date(s.StartedAt).toLocaleString()}</td>
                <td style="text-align: right; white-space: nowrap;">
                  <div style="display: flex; gap: 4px; justify-content: flex-end;">
                    <button class="btn btn-sm" onclick={() => selectSession(s)}>View</button>
                    {#if s.is_running}
                      <button class="btn btn-sm btn-danger" onclick={() => handleStop(s.ID)}>Stop</button>
                    {:else}
                      <button class="btn btn-sm" onclick={() => handleResume(s.ID)}>Resume</button>
                      <button class="btn btn-sm btn-danger" onclick={() => handleDelete(s.ID)}>Delete</button>
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
        <button class="btn btn-danger" onclick={() => handleStop(selectedSession.ID)}>Stop</button>
      {:else}
        <button class="btn" onclick={() => handleResume(selectedSession.ID)}>Resume</button>
        <button class="btn btn-danger" onclick={() => handleDelete(selectedSession.ID)}>Delete</button>
      {/if}
      <button class="btn" onclick={() => selectSession(selectedSession)}>Refresh</button>
    </div>

    {#if stats}
      <div class="stats-grid">
        <div class="stat-card"><div class="stat-value">{fmtN(stats.total_pages)}</div><div class="stat-label">Pages</div></div>
        <div class="stat-card"><div class="stat-value">{fmtN(stats.internal_links)}</div><div class="stat-label">Internal links</div></div>
        <div class="stat-card"><div class="stat-value">{fmtN(stats.external_links)}</div><div class="stat-label">External links</div></div>
        <div class="stat-card"><div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">Avg fetch</div></div>
        <div class="stat-card"><div class="stat-value" style="color: var(--error)">{fmtN(stats.error_count)}</div><div class="stat-label">Errors</div></div>
      </div>
    {/if}

    <!-- Tab bar -->
    <div class="tab-bar">
      {#each TABS as t}
        <button class="tab" class:tab-active={tab === t.id} onclick={() => switchTab(t.id)}>{t.label}</button>
      {/each}
    </div>

    <div class="card" style="border-top-left-radius: 0; border-top-right-radius: 0;">

      {#if tab === 'overview'}
        <table>
          <thead><tr><th>URL</th><th>Status</th><th>Title</th><th>Words</th><th>Int Out</th><th>Ext Out</th><th>Size</th><th>Time</th><th>Depth</th></tr></thead>
          <tbody>
            {#each pages as p}
              <tr>
                <td class="cell-url"><a href={p.URL} target="_blank" rel="noopener">{p.URL}</a></td>
                <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
                <td class="cell-title">{trunc(p.Title, 60)}</td>
                <td>{fmtN(p.WordCount)}</td>
                <td>{fmtN(p.InternalLinksOut)}</td>
                <td>{fmtN(p.ExternalLinksOut)}</td>
                <td>{fmtSize(p.BodySize)}</td>
                <td>{fmt(p.FetchDurationMs)}</td>
                <td>{p.Depth}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'titles'}
        <table>
          <thead><tr><th>URL</th><th>Title</th><th>Length</th><th>H1</th></tr></thead>
          <tbody>
            {#each pages as p}
              <tr>
                <td class="cell-url"><a href={p.URL} target="_blank" rel="noopener">{p.URL}</a></td>
                <td class="cell-title" class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.Title || '-'}</td>
                <td class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.TitleLength}</td>
                <td class="cell-title">{p.H1?.[0] || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'meta'}
        <table>
          <thead><tr><th>URL</th><th>Meta Description</th><th>Length</th><th>Meta Keywords</th><th>OG Title</th></tr></thead>
          <tbody>
            {#each pages as p}
              <tr>
                <td class="cell-url"><a href={p.URL} target="_blank" rel="noopener">{p.URL}</a></td>
                <td class="cell-title" class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{trunc(p.MetaDescription, 80)}</td>
                <td class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{p.MetaDescLength}</td>
                <td class="cell-title">{trunc(p.MetaKeywords, 60)}</td>
                <td class="cell-title">{trunc(p.OGTitle, 60)}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'headings'}
        <table>
          <thead><tr><th>URL</th><th>H1</th><th>H1 Count</th><th>H2</th><th>H2 Count</th></tr></thead>
          <tbody>
            {#each pages as p}
              <tr>
                <td class="cell-url"><a href={p.URL} target="_blank" rel="noopener">{p.URL}</a></td>
                <td class="cell-title" class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.[0] || '-'}</td>
                <td class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.length || 0}</td>
                <td class="cell-title">{p.H2?.[0] || '-'}</td>
                <td>{p.H2?.length || 0}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'images'}
        <table>
          <thead><tr><th>URL</th><th>Images</th><th>Without Alt</th><th>Title</th><th>Words</th></tr></thead>
          <tbody>
            {#each pages as p}
              <tr>
                <td class="cell-url"><a href={p.URL} target="_blank" rel="noopener">{p.URL}</a></td>
                <td>{p.ImagesCount}</td>
                <td class:cell-warn={p.ImagesNoAlt > 0}>{p.ImagesNoAlt}</td>
                <td class="cell-title">{trunc(p.Title, 50)}</td>
                <td>{fmtN(p.WordCount)}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'indexability'}
        <table>
          <thead><tr><th>URL</th><th>Indexable</th><th>Reason</th><th>Meta Robots</th><th>Canonical</th><th>Self</th></tr></thead>
          <tbody>
            {#each pages as p}
              <tr>
                <td class="cell-url"><a href={p.URL} target="_blank" rel="noopener">{p.URL}</a></td>
                <td><span class="badge" class:badge-success={p.IsIndexable} class:badge-error={!p.IsIndexable}>{p.IsIndexable ? 'Yes' : 'No'}</span></td>
                <td>{p.IndexReason || '-'}</td>
                <td>{p.MetaRobots || '-'}</td>
                <td class="cell-url">{trunc(p.Canonical, 60)}</td>
                <td>{p.CanonicalIsSelf ? 'Yes' : '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'response'}
        <table>
          <thead><tr><th>URL</th><th>Status</th><th>Content Type</th><th>Encoding</th><th>Size</th><th>Time</th><th>Redirects</th></tr></thead>
          <tbody>
            {#each pages as p}
              <tr>
                <td class="cell-url"><a href={p.URL} target="_blank" rel="noopener">{p.URL}</a></td>
                <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
                <td>{p.ContentType || '-'}</td>
                <td>{p.ContentEncoding || '-'}</td>
                <td>{fmtSize(p.BodySize)}</td>
                <td>{fmt(p.FetchDurationMs)}</td>
                <td>{p.FinalURL !== p.URL ? p.FinalURL : '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'internal'}
        <table>
          <thead><tr><th>Source</th><th>Target</th><th>Anchor Text</th><th>Tag</th></tr></thead>
          <tbody>
            {#each intLinks as l}
              <tr>
                <td class="cell-url">{l.SourceURL}</td>
                <td class="cell-url"><a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a></td>
                <td class="cell-title">{l.AnchorText || '-'}</td>
                <td>{l.Tag}</td>
              </tr>
            {/each}
          </tbody>
        </table>

      {:else if tab === 'external'}
        <table>
          <thead><tr><th>Source</th><th>Target</th><th>Anchor Text</th><th>Rel</th></tr></thead>
          <tbody>
            {#each extLinks as l}
              <tr>
                <td class="cell-url">{l.SourceURL}</td>
                <td class="cell-url"><a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a></td>
                <td class="cell-title">{l.AnchorText || '-'}</td>
                <td>{l.Rel || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}

      <!-- Pagination -->
      {#if currentData().length > 0}
        <div class="pagination">
          <button class="btn btn-sm" onclick={prevPage} disabled={currentOffset() === 0}>Previous</button>
          <span class="pagination-info">{currentOffset() + 1}–{currentOffset() + currentData().length}</span>
          <button class="btn btn-sm" onclick={nextPage} disabled={!hasMore()}>Next</button>
        </div>
      {/if}
    </div>
  {/if}
</div>
