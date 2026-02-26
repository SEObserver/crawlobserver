<script>
  import { getSessions, getStats, getPages, getExternalLinks, getInternalLinks, getProgress,
    startCrawl, stopCrawl, resumeCrawl, deleteSession, subscribeProgress, getTheme, updateTheme,
    getPageHTML, getStorageStats, getPageDetail } from './lib/api.js';

  let sessions = $state([]);
  let selectedSession = $state(null);
  let stats = $state(null);
  let pages = $state([]);
  let extLinks = $state([]);
  let intLinks = $state([]);
  let tab = $state('overview');
  let loading = $state(true);
  let error = $state(null);

  // Theme
  let theme = $state({ app_name: 'SEOCrawler', logo_url: '', accent_color: '#7c3aed', mode: 'light' });
  let darkMode = $state(false);

  // Pagination
  const PAGE_SIZE = 100;
  let pagesOffset = $state(0);
  let extLinksOffset = $state(0);
  let intLinksOffset = $state(0);
  let hasMorePages = $state(false);
  let hasMoreExtLinks = $state(false);
  let hasMoreIntLinks = $state(false);

  // Internal links filters
  let intSourceFilter = $state('');
  let intTargetFilter = $state('');

  // Settings
  let showSettings = $state(false);
  let editTheme = $state({ app_name: '', logo_url: '', accent_color: '#7c3aed', mode: 'light' });
  let savingTheme = $state(false);

  // New crawl form
  let showNewCrawl = $state(false);
  let seedInput = $state('');
  let maxPages = $state(0);
  let maxDepth = $state(0);
  let workers = $state(10);
  let crawlDelay = $state('1s');
  let storeHtml = $state(false);
  let starting = $state(false);

  // Resume modal
  let showResumeModal = $state(false);
  let resumeSessionId = $state(null);
  let resumeMaxPages = $state(0);
  let resumeMaxDepth = $state(0);
  let resumeWorkers = $state(10);
  let resumeDelay = $state('1s');
  let resumeStoreHtml = $state(false);
  let resuming = $state(false);

  // HTML modal
  let showHtmlModal = $state(false);
  let htmlModalData = $state({ url: '', body_html: '' });
  let htmlModalView = $state('render'); // 'render' or 'source'
  let htmlModalLoading = $state(false);

  // Storage stats
  let storageStats = $state(null);

  // Page detail
  let pageDetail = $state(null);
  let pageDetailLoading = $state(false);

  // Live progress
  let liveProgress = $state({});
  let sseConnections = {};

  // --- Theme ---
  async function loadTheme() {
    try {
      const t = await getTheme();
      theme = t;
      darkMode = t.mode === 'dark';
      applyTheme();
    } catch {}
  }

  function applyTheme() {
    document.documentElement.setAttribute('data-theme', darkMode ? 'dark' : 'light');
    if (theme.accent_color) {
      document.documentElement.style.setProperty('--accent', theme.accent_color);
      // Generate a lighter version for backgrounds
      const hex = theme.accent_color.replace('#', '');
      const r = parseInt(hex.substr(0, 2), 16);
      const g = parseInt(hex.substr(2, 2), 16);
      const b = parseInt(hex.substr(4, 2), 16);
      if (darkMode) {
        document.documentElement.style.setProperty('--accent-light', `rgba(${r},${g},${b},0.15)`);
      } else {
        document.documentElement.style.setProperty('--accent-light', `rgba(${r},${g},${b},0.08)`);
      }
    }
  }

  function toggleDarkMode() {
    darkMode = !darkMode;
    applyTheme();
  }

  // --- Settings ---
  function openSettings() {
    editTheme = { ...theme };
    showSettings = true;
    selectedSession = null;
    showNewCrawl = false;
  }

  function previewTheme() {
    theme.accent_color = editTheme.accent_color;
    theme.app_name = editTheme.app_name;
    theme.logo_url = editTheme.logo_url;
    darkMode = editTheme.mode === 'dark';
    applyTheme();
  }

  async function saveTheme() {
    savingTheme = true;
    try {
      const saved = await updateTheme(editTheme);
      theme = saved;
      darkMode = saved.mode === 'dark';
      applyTheme();
      showSettings = false;
    } catch (e) {
      error = e.message;
    } finally {
      savingTheme = false;
    }
  }

  function cancelSettings() {
    // Revert to saved theme
    loadTheme();
    showSettings = false;
  }

  // --- URL Routing ---
  function pushURL(path) {
    if (window.location.pathname !== path) {
      history.pushState(null, '', path);
    }
  }

  function parseRoute() {
    const path = window.location.pathname;
    const urlMatch = path.match(/^\/sessions\/([^/]+)\/url\/(.+)/);
    if (urlMatch) {
      return { sessionId: urlMatch[1], tab: 'url-detail', detailUrl: decodeURIComponent(urlMatch[2]) };
    }
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
      if (!selectedSession || selectedSession.ID !== route.sessionId) {
        if (sessions.length === 0) {
          await loadSessions();
        }
        const found = sessions.find(s => s.ID === route.sessionId);
        if (found) {
          selectedSession = found;
          stats = await getStats(found.ID);
          loadStorageStats();
        }
      }
      if (route.tab === 'url-detail') {
        tab = 'url-detail';
        pageDetail = null;
        await loadPageDetail(route.sessionId, route.detailUrl);
      } else {
        tab = route.tab;
        pageDetail = null;
        pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
        await loadTabData();
      }
    } else {
      selectedSession = null;
      stats = null;
      pageDetail = null;
      await loadSessions();
    }
  }

  window.addEventListener('popstate', () => applyRoute());

  async function selectSession(session) {
    selectedSession = session;
    tab = 'overview';
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    pushURL(`/sessions/${session.ID}/overview`);
    try {
      stats = await getStats(session.ID);
      loadStorageStats();
      await loadTabData();
    } catch (e) {
      error = e.message;
    }
  }

  function goHome() {
    selectedSession = null;
    stats = null;
    showNewCrawl = false;
    showSettings = false;
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
        const result = await getInternalLinks(id, PAGE_SIZE, intLinksOffset, intSourceFilter, intTargetFilter);
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
    intSourceFilter = ''; intTargetFilter = '';
    if (selectedSession) {
      pushURL(`/sessions/${selectedSession.ID}/${newTab}`);
    }
    loadTabData();
  }

  function applyIntLinksFilter() {
    intLinksOffset = 0;
    loadTabData();
  }
  function clearIntLinksFilter() {
    intSourceFilter = '';
    intTargetFilter = '';
    intLinksOffset = 0;
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
      setTimeout(async () => {
        await loadSessions();
        const sess = sessions.find(s => s.ID === id);
        if (sess && selectedSession?.ID !== id) {
          selectSession(sess);
        }
      }, 1000);
    } catch (e) { error = e.message; }
  }

  function openResumeModal(id) {
    const sess = sessions.find(s => s.ID === id);
    let cfg = {};
    if (sess?.Config) {
      try { cfg = typeof sess.Config === 'string' ? JSON.parse(sess.Config) : sess.Config; } catch {}
    }
    resumeSessionId = id;
    resumeMaxPages = cfg.max_pages || 0;
    resumeMaxDepth = cfg.max_depth || 0;
    resumeWorkers = cfg.workers || 10;
    resumeDelay = cfg.delay || '1s';
    resumeStoreHtml = cfg.store_html || false;
    showResumeModal = true;
  }

  function closeResumeModal() {
    showResumeModal = false;
    resumeSessionId = null;
  }

  async function handleResume() {
    resuming = true;
    error = null;
    try {
      await resumeCrawl(resumeSessionId, {
        max_pages: resumeMaxPages,
        max_depth: resumeMaxDepth,
        workers: resumeWorkers,
        delay: resumeDelay,
        store_html: resumeStoreHtml,
      });
      closeResumeModal();
      await loadSessions();
      const sess = sessions.find(s => s.ID === resumeSessionId);
      if (sess) {
        await selectSession(sess);
      }
    } catch (e) { error = e.message; }
    finally { resuming = false; }
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
  function timeAgo(date) {
    const d = new Date(date);
    const now = new Date();
    const diff = Math.floor((now - d) / 1000);
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.floor(diff/60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff/3600)}h ago`;
    return d.toLocaleDateString();
  }

  async function openHtmlModal(url) {
    htmlModalLoading = true;
    showHtmlModal = true;
    htmlModalView = 'render';
    try {
      htmlModalData = await getPageHTML(selectedSession.ID, url);
    } catch (e) {
      htmlModalData = { url, body_html: '' };
      error = e.message;
    } finally {
      htmlModalLoading = false;
    }
  }

  function closeHtmlModal() {
    showHtmlModal = false;
    htmlModalData = { url: '', body_html: '' };
  }

  async function loadPageDetail(sessionId, url) {
    pageDetailLoading = true;
    try {
      pageDetail = await getPageDetail(sessionId, url);
    } catch (e) {
      error = e.message;
    } finally {
      pageDetailLoading = false;
    }
  }

  function urlDetailHref(url) {
    if (!selectedSession) return '#';
    return `/sessions/${selectedSession.ID}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    navigateTo(urlDetailHref(url));
  }

  async function loadStorageStats() {
    try {
      storageStats = await getStorageStats();
    } catch {}
  }

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

  // Boot
  loadTheme();
  applyRoute();
</script>

<div class="layout">
  <!-- Sidebar -->
  <aside class="sidebar">
    <div class="sidebar-header">
      {#if theme.logo_url}
        <img class="sidebar-logo" src={theme.logo_url} alt="Logo" />
      {:else}
        <div class="sidebar-logo-placeholder">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        </div>
      {/if}
      <span class="sidebar-app-name">{theme.app_name}</span>
    </div>

    <div class="sidebar-section">
      <div class="sidebar-section-title">Main Menu</div>
      <nav class="sidebar-nav">
        <button class="sidebar-link" class:active={!selectedSession && !showNewCrawl} onclick={() => goHome()}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
          Dashboard
        </button>
        <button class="sidebar-link" class:active={showNewCrawl && !selectedSession} onclick={() => { goHome(); showNewCrawl = true; }}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="8" y1="12" x2="16" y2="12"/></svg>
          New Crawl
        </button>
      </nav>
    </div>

    {#if sessions.length > 0}
      <div class="sidebar-section">
        <div class="sidebar-section-title">Recent Sessions</div>
        <nav class="sidebar-nav">
          {#each sessions.slice(0, 8) as s}
            <button class="sidebar-link" class:active={selectedSession?.ID === s.ID} onclick={() => selectSession(s)}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
              <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                {#if s.is_running}
                  <span style="color: var(--info);">{new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}</span>
                {:else}
                  {new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}
                {/if}
              </span>
            </button>
          {/each}
        </nav>
      </div>
    {/if}

    <div class="sidebar-section" style="margin-top: auto;">
      <div class="sidebar-section-title">General</div>
      <nav class="sidebar-nav">
        <button class="sidebar-link" class:active={showSettings} onclick={openSettings}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          Settings
        </button>
      </nav>
    </div>

    <div class="sidebar-footer">
      <button class="theme-toggle" onclick={toggleDarkMode}>
        {#if darkMode}
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
          Light mode
        {:else}
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
          Dark mode
        {/if}
      </button>
    </div>
  </aside>

  <!-- Main Content -->
  <main class="main">
    <div class="main-content">
      {#if error}
        <div class="alert alert-error">
          <span>{error}</span>
          <button class="btn btn-sm btn-ghost" onclick={() => error = null}>Dismiss</button>
        </div>
      {/if}

      {#if showSettings}
        <!-- Settings -->
        <div class="page-header">
          <h1>Settings</h1>
        </div>
        <div class="card">
          <div class="form-grid">
            <div class="form-group">
              <label for="set-appname">App Name</label>
              <input id="set-appname" type="text" bind:value={editTheme.app_name} oninput={previewTheme} />
            </div>
            <div class="form-group">
              <label for="set-logo">Logo URL</label>
              <input id="set-logo" type="text" bind:value={editTheme.logo_url} oninput={previewTheme} placeholder="https://example.com/logo.png" />
            </div>
            {#if editTheme.logo_url}
              <div class="form-group" style="grid-column: 1 / -1;">
                <span style="font-weight: 500; font-size: 0.85rem; color: var(--text-secondary);">Logo Preview</span>
                <img src={editTheme.logo_url} alt="Logo preview" style="max-height: 48px; border-radius: 6px; background: var(--bg-card);" />
              </div>
            {/if}
            <div class="form-group">
              <label for="set-accent">Accent Color</label>
              <div style="display: flex; align-items: center; gap: 10px;">
                <input id="set-accent" type="color" value={editTheme.accent_color} oninput={(e) => { editTheme.accent_color = e.target.value; previewTheme(); }} style="width: 48px; height: 36px; border: 1px solid var(--border); border-radius: 6px; cursor: pointer; padding: 2px;" />
                <span style="font-family: monospace; color: var(--text-secondary);">{editTheme.accent_color}</span>
              </div>
            </div>
            <div class="form-group">
              <span style="font-weight: 500; font-size: 0.85rem; color: var(--text-secondary); display: block; margin-bottom: 4px;">Mode</span>
              <div style="display: flex; gap: 8px;">
                <button class="btn btn-sm" class:btn-primary={editTheme.mode === 'light'} onclick={() => { editTheme.mode = 'light'; previewTheme(); }}>Light</button>
                <button class="btn btn-sm" class:btn-primary={editTheme.mode === 'dark'} onclick={() => { editTheme.mode = 'dark'; previewTheme(); }}>Dark</button>
              </div>
            </div>
          </div>
          <div style="display: flex; gap: 8px; margin-top: 20px;">
            <button class="btn btn-primary" onclick={saveTheme} disabled={savingTheme}>
              {savingTheme ? 'Saving...' : 'Save'}
            </button>
            <button class="btn" onclick={cancelSettings}>Cancel</button>
          </div>
        </div>

      {:else if showNewCrawl && !selectedSession}
        <!-- New Crawl Form -->
        <div class="page-header">
          <h1>New Crawl</h1>
        </div>
        <div class="card">
          <div class="form-grid">
            <div class="form-group" style="grid-column: 1 / -1;">
              <label for="seeds">Seed URLs (one per line)</label>
              <textarea id="seeds" bind:value={seedInput} rows="3" placeholder="https://example.com"></textarea>
            </div>
            <div class="form-group"><label for="workers">Workers</label><input id="workers" type="number" bind:value={workers} min="1" max="100" /></div>
            <div class="form-group"><label for="delay">Delay</label><input id="delay" type="text" bind:value={crawlDelay} placeholder="1s" /></div>
            <div class="form-group"><label for="maxpages">Max pages (0 = unlimited)</label><input id="maxpages" type="number" bind:value={maxPages} min="0" /></div>
            <div class="form-group"><label for="maxdepth">Max depth (0 = unlimited)</label><input id="maxdepth" type="number" bind:value={maxDepth} min="0" /></div>
            <div class="form-group" style="display: flex; flex-direction: row; align-items: center; gap: 8px; padding-top: 24px;">
              <input id="storehtml" type="checkbox" bind:checked={storeHtml} /><label for="storehtml" style="margin: 0;">Store raw HTML</label>
            </div>
          </div>
          <div style="display: flex; gap: 8px; margin-top: 20px;">
            <button class="btn btn-primary" onclick={handleStartCrawl} disabled={starting || !seedInput.trim()}>
              {starting ? 'Starting...' : 'Start Crawl'}
            </button>
            <button class="btn" onclick={() => showNewCrawl = false}>Cancel</button>
          </div>
        </div>

      {:else if !selectedSession}
        <!-- Sessions List -->
        <div class="page-header">
          <h1>Crawl Sessions</h1>
          <button class="btn btn-primary" onclick={() => showNewCrawl = true}>
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            New Crawl
          </button>
        </div>

        {#if loading}
          <p style="color: var(--text-muted); padding: 40px 0;">Loading...</p>
        {:else if sessions.length === 0}
          <div class="empty-state">
            <h2>No crawl sessions yet</h2>
            <p>Start your first crawl to begin analyzing your site.</p>
            <button class="btn btn-primary" style="margin-top: 16px;" onclick={() => showNewCrawl = true}>Start a Crawl</button>
          </div>
        {:else}
          <div class="card card-flush">
            {#each sessions as s}
              <div class="session-row">
                <div class="session-info">
                  <div class="session-seed">{s.SeedURLs?.[0] || 'Unknown'}</div>
                  <div class="session-meta">
                    {#if s.is_running}
                      <span class="badge badge-info">
                        Running
                        {#if liveProgress[s.ID]}
                          &middot; {fmtN(liveProgress[s.ID].pages_crawled)} pages &middot; {fmtN(liveProgress[s.ID].queue_size)} queued
                        {/if}
                      </span>
                    {:else}
                      <span class="badge" class:badge-success={s.Status==='completed'} class:badge-error={s.Status==='failed'} class:badge-warning={s.Status==='stopped'}>{s.Status}</span>
                    {/if}
                    <span>{fmtN(s.PagesCrawled)} pages</span>
                    <span>{timeAgo(s.StartedAt)}</span>
                  </div>
                </div>
                <div class="session-actions">
                  <button class="btn btn-sm" onclick={() => selectSession(s)}>View</button>
                  {#if s.is_running}
                    <button class="btn btn-sm btn-danger" onclick={() => handleStop(s.ID)}>Stop</button>
                  {:else}
                    <button class="btn btn-sm" onclick={() => openResumeModal(s.ID)}>Resume</button>
                    <button class="btn btn-sm btn-danger" onclick={() => handleDelete(s.ID)}>Delete</button>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}

      {:else if tab === 'url-detail' && selectedSession}
        <!-- URL Detail View -->
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
          <span>/</span>
          <a href={`/sessions/${selectedSession.ID}/overview`} onclick={(e) => { e.preventDefault(); navigateTo(`/sessions/${selectedSession.ID}/overview`); }}>{selectedSession.SeedURLs?.[0] || selectedSession.ID}</a>
          <span>/</span>
          <span style="color: var(--text);">URL Detail</span>
        </div>

        {#if pageDetailLoading}
          <p style="color: var(--text-muted); padding: 40px 0;">Loading...</p>
        {:else if pageDetail?.page}
          {@const pg = pageDetail.page}
          {@const outLinks = pageDetail.links?.filter(l => l.SourceURL === pg.URL) || []}
          {@const inLinks = pageDetail.links?.filter(l => l.TargetURL === pg.URL) || []}

          <!-- Header -->
          <div class="page-header" style="gap: 12px; flex-wrap: wrap;">
            <div style="display: flex; align-items: center; gap: 8px; min-width: 0; flex: 1;">
              <button class="btn btn-sm" onclick={() => navigateTo(`/sessions/${selectedSession.ID}/overview`)} title="Back">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
              </button>
              <h1 style="font-size: 1rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title={pg.URL}>{pg.URL}</h1>
              <span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span>
            </div>
            <div style="display: flex; gap: 6px;">
              <a class="btn btn-sm" href={pg.URL} target="_blank" rel="noopener">Open URL</a>
              {#if pg.BodySize > 0}
                <button class="btn btn-sm" onclick={() => openHtmlModal(pg.URL)}>View HTML</button>
              {/if}
            </div>
          </div>

          <!-- Summary Cards -->
          <div class="stats-grid">
            <div class="stat-card"><div class="stat-value"><span class="badge {statusBadge(pg.StatusCode)}" style="font-size: 1.2rem;">{pg.StatusCode}</span></div><div class="stat-label">Status Code</div></div>
            <div class="stat-card"><div class="stat-value" style="font-size: 0.95rem;">{pg.ContentType || '-'}</div><div class="stat-label">Content-Type</div></div>
            <div class="stat-card"><div class="stat-value">{fmtSize(pg.BodySize)}</div><div class="stat-label">Size</div></div>
            <div class="stat-card"><div class="stat-value">{fmt(pg.FetchDurationMs)}</div><div class="stat-label">Response Time</div></div>
            <div class="stat-card"><div class="stat-value">{pg.Depth}</div><div class="stat-label">Depth</div></div>
            {#if pg.FoundOn}
              <div class="stat-card"><div class="stat-value" style="font-size: 0.8rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"><a href={urlDetailHref(pg.FoundOn)} onclick={(e) => goToUrlDetail(e, pg.FoundOn)} style="color: var(--accent);">{pg.FoundOn}</a></div><div class="stat-label">Found On</div></div>
            {/if}
            <div class="stat-card"><div class="stat-value" style="font-size: 0.8rem;">{new Date(pg.CrawledAt).toLocaleString()}</div><div class="stat-label">Crawled At</div></div>
          </div>

          {#if pg.Error}
            <div class="alert alert-error" style="margin-bottom: 16px;">
              <strong>Error:</strong> {pg.Error}
            </div>
          {/if}

          <!-- Response Headers -->
          {#if pg.Headers && Object.keys(pg.Headers).length > 0}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Response Headers</h3>
              <table>
                <thead><tr><th>Header</th><th>Value</th></tr></thead>
                <tbody>
                  {#each Object.entries(pg.Headers).sort((a,b) => a[0].localeCompare(b[0])) as [key, val]}
                    <tr><td style="font-weight: 500; white-space: nowrap;">{key}</td><td style="word-break: break-all;">{val}</td></tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- SEO -->
          <div class="card" style="margin-bottom: 16px;">
            <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">SEO</h3>
            <table>
              <tbody>
                <tr><td style="font-weight: 500; width: 160px;">Title</td><td>{pg.Title || '-'} <span style="color: var(--text-muted);">({pg.TitleLength} chars)</span></td></tr>
                <tr><td style="font-weight: 500;">Meta Description</td><td>{pg.MetaDescription || '-'} <span style="color: var(--text-muted);">({pg.MetaDescLength} chars)</span></td></tr>
                {#if pg.MetaKeywords}<tr><td style="font-weight: 500;">Meta Keywords</td><td>{pg.MetaKeywords}</td></tr>{/if}
                <tr><td style="font-weight: 500;">Meta Robots</td><td>{pg.MetaRobots || '-'}</td></tr>
                {#if pg.XRobotsTag}<tr><td style="font-weight: 500;">X-Robots-Tag</td><td>{pg.XRobotsTag}</td></tr>{/if}
                <tr><td style="font-weight: 500;">Canonical</td><td>{pg.Canonical || '-'} {#if pg.CanonicalIsSelf}<span class="badge badge-success" style="font-size: 0.7rem;">self</span>{/if}</td></tr>
                <tr><td style="font-weight: 500;">Indexable</td><td><span class="badge" class:badge-success={pg.IsIndexable} class:badge-error={!pg.IsIndexable}>{pg.IsIndexable ? 'Yes' : 'No'}</span> {#if pg.IndexReason}<span style="color: var(--text-muted);">({pg.IndexReason})</span>{/if}</td></tr>
                {#if pg.Lang}<tr><td style="font-weight: 500;">Language</td><td>{pg.Lang}</td></tr>{/if}
              </tbody>
            </table>
          </div>

          <!-- Open Graph -->
          {#if pg.OGTitle || pg.OGDescription || pg.OGImage}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Open Graph</h3>
              <table>
                <tbody>
                  {#if pg.OGTitle}<tr><td style="font-weight: 500; width: 160px;">OG Title</td><td>{pg.OGTitle}</td></tr>{/if}
                  {#if pg.OGDescription}<tr><td style="font-weight: 500;">OG Description</td><td>{pg.OGDescription}</td></tr>{/if}
                  {#if pg.OGImage}
                    <tr><td style="font-weight: 500;">OG Image</td><td><a href={pg.OGImage} target="_blank" rel="noopener">{pg.OGImage}</a></td></tr>
                    <tr><td></td><td><img src={pg.OGImage} alt="OG preview" style="max-width: 300px; max-height: 200px; border-radius: 6px; border: 1px solid var(--border); margin-top: 4px;" /></td></tr>
                  {/if}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Headings -->
          {#if pg.H1?.length || pg.H2?.length || pg.H3?.length || pg.H4?.length || pg.H5?.length || pg.H6?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Headings</h3>
              {#each [['H1', pg.H1], ['H2', pg.H2], ['H3', pg.H3], ['H4', pg.H4], ['H5', pg.H5], ['H6', pg.H6]] as [label, items]}
                {#if items?.length}
                  <div style="margin-bottom: 8px;">
                    <strong style="font-size: 0.85rem;">{label}</strong> <span style="color: var(--text-muted);">({items.length})</span>
                    <ul style="margin: 4px 0 0 20px; padding: 0;">
                      {#each items as h}<li style="font-size: 0.85rem; color: var(--text-secondary);">{h}</li>{/each}
                    </ul>
                  </div>
                {/if}
              {/each}
            </div>
          {/if}

          <!-- Redirect Chain -->
          {#if pg.RedirectChain?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Redirect Chain</h3>
              <table>
                <thead><tr><th>#</th><th>URL</th><th>Status</th></tr></thead>
                <tbody>
                  {#each pg.RedirectChain as hop, i}
                    <tr>
                      <td>{i + 1}</td>
                      <td class="cell-url">{hop.URL}</td>
                      <td><span class="badge {statusBadge(hop.StatusCode)}">{hop.StatusCode}</span></td>
                    </tr>
                  {/each}
                  <tr>
                    <td>{pg.RedirectChain.length + 1}</td>
                    <td class="cell-url" style="font-weight: 500;">{pg.FinalURL || pg.URL}</td>
                    <td><span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span></td>
                  </tr>
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Hreflang -->
          {#if pg.Hreflang?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Hreflang</h3>
              <table>
                <thead><tr><th>Language</th><th>URL</th></tr></thead>
                <tbody>
                  {#each pg.Hreflang as h}
                    <tr><td style="font-weight: 500;">{h.Lang}</td><td class="cell-url">{h.URL}</td></tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Schema.org -->
          {#if pg.SchemaTypes?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Schema.org</h3>
              <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                {#each pg.SchemaTypes as t}
                  <span class="badge badge-info">{t}</span>
                {/each}
              </div>
            </div>
          {/if}

          <!-- Content -->
          <div class="card" style="margin-bottom: 16px;">
            <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Content</h3>
            <div class="stats-grid">
              <div class="stat-card"><div class="stat-value">{fmtN(pg.WordCount)}</div><div class="stat-label">Words</div></div>
              <div class="stat-card"><div class="stat-value">{pg.ImagesCount}</div><div class="stat-label">Images</div></div>
              <div class="stat-card"><div class="stat-value" style={pg.ImagesNoAlt > 0 ? 'color: var(--warning)' : ''}>{pg.ImagesNoAlt}</div><div class="stat-label">Images without alt</div></div>
              <div class="stat-card"><div class="stat-value">{fmtN(pg.InternalLinksOut)}</div><div class="stat-label">Internal Links Out</div></div>
              <div class="stat-card"><div class="stat-value">{fmtN(pg.ExternalLinksOut)}</div><div class="stat-label">External Links Out</div></div>
            </div>
          </div>

          <!-- Outbound Links -->
          {#if outLinks.length > 0}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Outbound Links <span style="color: var(--text-muted);">({outLinks.length})</span></h3>
              <table>
                <thead><tr><th>Target URL</th><th>Anchor</th><th>Type</th><th>Tag</th><th>Rel</th></tr></thead>
                <tbody>
                  {#each outLinks as l}
                    <tr>
                      <td class="cell-url">
                        {#if l.IsInternal}
                          <a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a>
                        {:else}
                          <a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a>
                        {/if}
                      </td>
                      <td class="cell-title">{l.AnchorText || '-'}</td>
                      <td><span class="badge" class:badge-success={l.IsInternal} class:badge-warning={!l.IsInternal}>{l.IsInternal ? 'Internal' : 'External'}</span></td>
                      <td>{l.Tag || '-'}</td>
                      <td>{l.Rel || '-'}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Inbound Links -->
          {#if inLinks.length > 0}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Inbound Links <span style="color: var(--text-muted);">({inLinks.length})</span></h3>
              <table>
                <thead><tr><th>Source URL</th><th>Anchor</th><th>Tag</th><th>Rel</th></tr></thead>
                <tbody>
                  {#each inLinks as l}
                    <tr>
                      <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                      <td class="cell-title">{l.AnchorText || '-'}</td>
                      <td>{l.Tag || '-'}</td>
                      <td>{l.Rel || '-'}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        {:else}
          <p style="color: var(--text-muted); padding: 40px 0;">Page not found.</p>
        {/if}

      {:else}
        <!-- Session Detail -->
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
          <span>/</span>
          <span style="color: var(--text);">{selectedSession.SeedURLs?.[0] || selectedSession.ID}</span>
        </div>

        <div class="action-bar">
          {#if selectedSession.is_running}
            <span class="badge badge-info">Running
              {#if liveProgress[selectedSession.ID]}
                &middot; {fmtN(liveProgress[selectedSession.ID].pages_crawled)} pages
              {/if}
            </span>
            <button class="btn btn-sm btn-danger" onclick={() => handleStop(selectedSession.ID)}>Stop</button>
          {:else}
            <span class="badge" class:badge-success={selectedSession.Status==='completed'} class:badge-error={selectedSession.Status==='failed'} class:badge-warning={selectedSession.Status==='stopped'}>{selectedSession.Status}</span>
            <button class="btn btn-sm" onclick={() => openResumeModal(selectedSession.ID)}>Resume</button>
            <button class="btn btn-sm btn-danger" onclick={() => handleDelete(selectedSession.ID)}>Delete</button>
          {/if}
          <button class="btn btn-sm" onclick={() => selectSession(selectedSession)}>
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
            Refresh
          </button>
        </div>

        {#if stats}
          <div class="stats-grid">
            <div class="stat-card"><div class="stat-value">{fmtN(stats.total_pages)}</div><div class="stat-label">Pages crawled</div></div>
            <div class="stat-card"><div class="stat-value">{fmtN(stats.internal_links)}</div><div class="stat-label">Internal links</div></div>
            <div class="stat-card"><div class="stat-value">{fmtN(stats.external_links)}</div><div class="stat-label">External links</div></div>
            <div class="stat-card"><div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">Avg response</div></div>
            <div class="stat-card"><div class="stat-value" style="color: var(--error)">{fmtN(stats.error_count)}</div><div class="stat-label">Errors</div></div>
            {#if storageStats?.tables?.length}
              <div class="stat-card">
                <div class="stat-value">{fmtSize(storageStats.tables.reduce((a, t) => a + t.bytes_on_disk, 0))}</div>
                <div class="stat-label">Storage total</div>
              </div>
              {#each storageStats.tables as t}
                <div class="stat-card">
                  <div class="stat-value">{fmtSize(t.bytes_on_disk)}</div>
                  <div class="stat-label">{t.name} ({fmtN(t.rows)} rows)</div>
                </div>
              {/each}
            {/if}
          </div>
        {/if}

        <div class="tab-bar">
          {#each TABS as t}
            <button class="tab" class:tab-active={tab === t.id} onclick={() => switchTab(t.id)}>{t.label}</button>
          {/each}
        </div>

        <div class="card card-flush" style="border-top-left-radius: 0; border-top-right-radius: 0; border-top: none;">

          {#if tab === 'overview'}
            <table>
              <thead><tr><th>URL</th><th>Status</th><th>Title</th><th>Words</th><th>Int Out</th><th>Ext Out</th><th>Size</th><th>Time</th><th>Depth</th><th></th></tr></thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
                    <td class="cell-title">{trunc(p.Title, 60)}</td>
                    <td>{fmtN(p.WordCount)}</td>
                    <td>{fmtN(p.InternalLinksOut)}</td>
                    <td>{fmtN(p.ExternalLinksOut)}</td>
                    <td>{fmtSize(p.BodySize)}</td>
                    <td>{fmt(p.FetchDurationMs)}</td>
                    <td>{p.Depth}</td>
                    <td>
                      {#if p.BodySize > 0}
                        <button class="btn-html" title="View HTML" onclick={() => openHtmlModal(p.URL)}>
                          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
                        </button>
                      {/if}
                    </td>
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
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
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
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
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
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
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
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
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
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
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
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
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
            <div class="filter-bar">
              <input type="text" placeholder="Filter source URL…" bind:value={intSourceFilter} onkeydown={(e) => e.key === 'Enter' && applyIntLinksFilter()} />
              <input type="text" placeholder="Filter target URL…" bind:value={intTargetFilter} onkeydown={(e) => e.key === 'Enter' && applyIntLinksFilter()} />
              <button class="btn btn-sm" onclick={applyIntLinksFilter}>Filter</button>
              <button class="btn btn-sm btn-ghost" onclick={clearIntLinksFilter}>Clear</button>
            </div>
            <table>
              <thead><tr><th>Source</th><th>Target</th><th>Anchor Text</th><th>Tag</th></tr></thead>
              <tbody>
                {#each intLinks as l}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                    <td class="cell-url"><a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a></td>
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
                    <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                    <td class="cell-url"><a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a></td>
                    <td class="cell-title">{l.AnchorText || '-'}</td>
                    <td>{l.Rel || '-'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}

          {#if currentData().length > 0}
            <div class="pagination">
              <button class="btn btn-sm" onclick={prevPage} disabled={currentOffset() === 0}>Previous</button>
              <span class="pagination-info">{currentOffset() + 1} - {currentOffset() + currentData().length}</span>
              <button class="btn btn-sm" onclick={nextPage} disabled={!hasMore()}>Next</button>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </main>
</div>

{#if showResumeModal}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="html-modal-overlay" onclick={closeResumeModal}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="html-modal" onclick={(e) => e.stopPropagation()} style="max-width: 480px; height: auto;">
      <div class="html-modal-header">
        <div class="html-modal-url">Resume Crawl</div>
        <div class="html-modal-actions">
          <button class="btn btn-sm" title="Close" onclick={closeResumeModal}>
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
      </div>
      <div style="padding: 20px;">
        <div class="form-grid">
          <div class="form-group"><label for="r-maxpages">Max pages (0 = unlimited)</label><input id="r-maxpages" type="number" bind:value={resumeMaxPages} min="0" /></div>
          <div class="form-group"><label for="r-maxdepth">Max depth (0 = unlimited)</label><input id="r-maxdepth" type="number" bind:value={resumeMaxDepth} min="0" /></div>
          <div class="form-group"><label for="r-workers">Workers</label><input id="r-workers" type="number" bind:value={resumeWorkers} min="1" max="100" /></div>
          <div class="form-group"><label for="r-delay">Delay</label><input id="r-delay" type="text" bind:value={resumeDelay} placeholder="1s" /></div>
          <div class="form-group" style="display: flex; flex-direction: row; align-items: center; gap: 8px; padding-top: 24px;">
            <input id="r-storehtml" type="checkbox" bind:checked={resumeStoreHtml} /><label for="r-storehtml" style="margin: 0;">Store raw HTML</label>
          </div>
        </div>
        <div style="display: flex; gap: 8px; margin-top: 20px;">
          <button class="btn btn-primary" onclick={handleResume} disabled={resuming}>
            {resuming ? 'Resuming...' : 'Resume'}
          </button>
          <button class="btn" onclick={closeResumeModal}>Cancel</button>
        </div>
      </div>
    </div>
  </div>
{/if}

{#if showHtmlModal}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="html-modal-overlay" onclick={closeHtmlModal}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="html-modal" onclick={(e) => e.stopPropagation()}>
      <div class="html-modal-header">
        <div class="html-modal-url" title={htmlModalData.url}>{htmlModalData.url}</div>
        <div class="html-modal-actions">
          <button class="btn btn-sm" class:btn-primary={htmlModalView === 'render'} onclick={() => htmlModalView = 'render'}>Render</button>
          <button class="btn btn-sm" class:btn-primary={htmlModalView === 'source'} onclick={() => htmlModalView = 'source'}>Source</button>
          <button class="btn btn-sm" title="Close" onclick={closeHtmlModal}>
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
      </div>
      <div class="html-modal-body">
        {#if htmlModalLoading}
          <p style="padding: 40px; color: var(--text-muted); text-align: center;">Loading...</p>
        {:else if !htmlModalData.body_html}
          <p style="padding: 40px; color: var(--text-muted); text-align: center;">No HTML stored for this page.</p>
        {:else if htmlModalView === 'render'}
          <iframe srcdoc={htmlModalData.body_html} title="Page render" class="html-modal-iframe"></iframe>
        {:else}
          <pre class="html-modal-source"><code>{htmlModalData.body_html}</code></pre>
        {/if}
      </div>
    </div>
  </div>
{/if}
