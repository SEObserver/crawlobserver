<script>
  import { onDestroy } from 'svelte';
  import { getSessions, getStats, getPages, getExternalLinks, getInternalLinks, getProgress,
    stopCrawl, deleteSession,
    subscribeProgress, getTheme,
    getStorageStats, getGlobalStats, getSessionStorage, getSystemStats,
    getProjects,
    getUpdateStatus, applyUpdate,
    createBackup } from './lib/api.js';
  import { statusBadge, fmt, fmtSize, fmtN, trunc, timeAgo, a11yKeydown } from './lib/utils.js';
  import { PAGE_SIZE, TAB_FILTERS, TABS } from './lib/tabColumns.js';
  import HtmlModal from './lib/components/HtmlModal.svelte';
  import ResumeModal from './lib/components/ResumeModal.svelte';
  import NewCrawlForm from './lib/components/NewCrawlForm.svelte';
  import PageRankTab from './lib/components/PageRankTab.svelte';
  import RobotsTab from './lib/components/RobotsTab.svelte';
  import SitemapsTab from './lib/components/SitemapsTab.svelte';
  import GSCTab from './lib/components/GSCTab.svelte';
  import ProvidersTab from './lib/components/ProvidersTab.svelte';
  import GlobalStatsPage from './lib/components/GlobalStatsPage.svelte';
  import SettingsPage from './lib/components/SettingsPage.svelte';
  import APIManagementPage from './lib/components/APIManagementPage.svelte';
  import SessionsList from './lib/components/SessionsList.svelte';
  import Sidebar from './lib/components/Sidebar.svelte';
  import SessionActionBar from './lib/components/SessionActionBar.svelte';
  import ReportsHub from './lib/components/ReportsHub.svelte';
  import ComparePage from './lib/components/ComparePage.svelte';
  import CustomTestsTab from './lib/components/CustomTestsTab.svelte';
  import ExternalChecksTab from './lib/components/ExternalChecksTab.svelte';
  import LogsPage from './lib/components/LogsPage.svelte';
  import UrlDetailView from './lib/components/UrlDetailView.svelte';
  import DataTable from './lib/components/DataTable.svelte';

  let sessions = $state([]);
  let selectedSession = $state(null);
  let stats = $state(null);
  let sessionStorageMap = $state({});
  let pages = $state([]);
  let extLinks = $state([]);
  let intLinks = $state([]);
  let tab = $state('overview');
  let detailUrl = $state('');
  let loading = $state(true);
  let error = $state(null);

  // Theme
  let theme = $state({ app_name: 'SEOCrawler', logo_url: '', accent_color: '#7c3aed', mode: 'light' });
  let darkMode = $state(false);

  // Pagination
  let pagesOffset = $state(0);
  let extLinksOffset = $state(0);
  let intLinksOffset = $state(0);
  let hasMorePages = $state(false);
  let hasMoreExtLinks = $state(false);
  let hasMoreIntLinks = $state(false);

  // Global filters
  let filters = $state({});


  // Settings
  let showSettings = $state(false);

  // Update
  let updateInfo = $state(null);
  let updateDismissed = $state(false);
  let updatingApp = $state(false);
  let updateMessage = $state('');


  // New crawl form
  let showNewCrawl = $state(false);

  // Resume modal
  let showResumeModal = $state(false);
  let resumeSessionId = $state(null);

  // HTML modal
  let showHtmlModal = $state(false);
  let htmlModalUrl = $state('');

  // Storage stats
  let storageStats = $state(null);

  // System stats (CPU/memory monitoring)
  let systemStats = $state(null);
  let systemStatsInterval = null;


  // PageRank tab
  let prSubView = $state('top');

  // Reports tab
  let reportsSubView = $state('overview');

  // External checks tab
  let extChecksSubView = $state('domains');

  // Live progress
  let liveProgress = $state({});
  let sseConnections = {};

  // Global Stats
  let showGlobalStats = $state(false);
  let globalStats = $state(null);

  // API page
  let showAPI = $state(false);
  let projects = $state([]);

  // Compare
  let showCompare = $state(false);
  let showLogs = $state(false);
  let compareSessionA = $state('');
  let compareSessionB = $state('');

  // Project view
  let selectedProject = $state(null);
  let projectTab = $state('sessions');
  let gscSubView = $state('overview');
  let providerSubView = $state('overview');

  // --- Project view ---
  function selectProject(proj) {
    selectedProject = proj;
    selectedSession = null;
    projectTab = 'sessions';
    showSettings = false;
    showGlobalStats = false;
    showAPI = false;
    showNewCrawl = false;
    showCompare = false;
    showLogs = false;
    pushURL(`/projects/${proj.id}`);
  }

  function switchProjectTab(t) {
    projectTab = t;
    if (selectedProject) pushURL(`/projects/${selectedProject.id}/${t}`);
  }

  // --- Global Stats ---
  function openGlobalStats() {
    showGlobalStats = true;
    showSettings = false;
    showAPI = false;
    showNewCrawl = false;
    showCompare = false;
    showLogs = false;
    selectedSession = null;
    selectedProject = null;
    pushURL('/stats');
  }

  // --- API page ---
  function openAPI() {
    showAPI = true;
    showSettings = false;
    showGlobalStats = false;
    showNewCrawl = false;
    showCompare = false;
    showLogs = false;
    selectedSession = null;
    selectedProject = null;
    pushURL('/api');
  }

  function openLogs() {
    showLogs = true;
    showSettings = false;
    showGlobalStats = false;
    showAPI = false;
    showNewCrawl = false;
    showCompare = false;
    selectedSession = null;
    selectedProject = null;
    pushURL('/logs');
  }


  // --- Theme ---
  async function loadTheme() {
    try {
      const t = await getTheme();
      theme = t;
      const saved = localStorage.getItem('darkMode');
      darkMode = saved !== null ? saved === 'true' : t.mode === 'dark';
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
    localStorage.setItem('darkMode', darkMode ? 'true' : 'false');
    applyTheme();
  }

  // --- Settings ---
  function openSettings() {
    showSettings = true;
    selectedSession = null;
    selectedProject = null;
    showNewCrawl = false;
    showAPI = false;
    showGlobalStats = false;
    showCompare = false;
    showLogs = false;
    pushURL('/settings');
  }

  function handleSettingsSave(saved, isPreview) {
    if (isPreview) {
      theme.accent_color = saved.accent_color;
      theme.app_name = saved.app_name;
      theme.logo_url = saved.logo_url;
      darkMode = saved.mode === 'dark';
      applyTheme();
    } else {
      theme = saved;
      darkMode = saved.mode === 'dark';
      applyTheme();
      showSettings = false;
    }
  }

  function handleSettingsCancel() {
    loadTheme();
    showSettings = false;
  }

  // --- Filter helpers ---
  function applyFilters() {
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (selectedSession) {
      pushURL(`/sessions/${selectedSession.ID}/${tab}`, filters);
    }
    loadTabData();
  }

  function clearFilters() {
    filters = {};
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (selectedSession) {
      pushURL(`/sessions/${selectedSession.ID}/${tab}`);
    }
    loadTabData();
  }

  function setFilter(key, val) {
    filters[key] = val;
    filters = { ...filters };
  }

  function hasActiveFilters() {
    return Object.values(filters).some(v => v && v !== '');
  }

  // --- URL Routing ---
  function pushURL(path, queryFilters = {}, offset = 0) {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(queryFilters)) {
      if (v !== '' && v != null) params.set(k, v);
    }
    if (offset > 0) params.set('offset', String(offset));
    const qs = params.toString();
    const full = qs ? `${path}?${qs}` : path;
    if (window.location.pathname + window.location.search !== full) {
      history.pushState(null, '', full);
    }
  }

  function parseRoute() {
    const path = window.location.pathname;
    const search = window.location.search;

    // Top-level pages
    if (path === '/new-crawl') return { page: 'new-crawl' };
    if (path === '/settings') return { page: 'settings' };
    if (path === '/stats') return { page: 'stats' };
    if (path === '/api') return { page: 'api' };
    if (path === '/logs') return { page: 'logs' };
    if (path === '/compare') {
      const sp = new URLSearchParams(search);
      return { page: 'compare', sessionA: sp.get('a') || '', sessionB: sp.get('b') || '' };
    }

    // Project view
    const projMatch = path.match(/^\/projects\/([^/]+)(?:\/([^/]+)(?:\/([^/]+))?)?/);
    if (projMatch) {
      return { page: 'project', projectId: projMatch[1], projectTab: projMatch[2] || 'sessions', projectSubView: projMatch[3] || null };
    }

    // URL detail
    const urlMatch = path.match(/^\/sessions\/([^/]+)\/url\/(.+)/);
    if (urlMatch) {
      return { sessionId: urlMatch[1], tab: 'url-detail', detailUrl: decodeURIComponent(urlMatch[2]), filters: {}, offset: 0 };
    }
    // Session detail
    const m = path.match(/^\/sessions\/([^/]+)(?:\/([^/]+)(?:\/([^/]+))?)?/);
    if (m) {
      const sp = new URLSearchParams(search);
      const routeFilters = {};
      let routeOffset = 0;
      for (const [k, v] of sp.entries()) {
        if (k === 'offset') {
          routeOffset = parseInt(v, 10) || 0;
        } else if (k !== 'limit') {
          routeFilters[k] = v;
        }
      }
      return { sessionId: m[1], tab: m[2] || 'overview', subView: m[3] || null, filters: routeFilters, offset: routeOffset };
    }
    // Home
    return { page: 'home' };
  }

  async function navigateTo(path, queryFilters = {}) {
    pushURL(path, queryFilters);
    await applyRoute();
  }

  async function applyRoute() {
    const route = parseRoute();

    // Top-level pages (home, new-crawl, settings, stats, api, project)
    if (route.page) {
      selectedSession = null;
      stats = null;
      filters = {};
      loading = false;
      showSettings = route.page === 'settings';
      showGlobalStats = route.page === 'stats';
      showAPI = route.page === 'api';
      showNewCrawl = route.page === 'new-crawl';
      showCompare = route.page === 'compare';
      showLogs = route.page === 'logs';

      if (route.page === 'project') {
        selectedProject = projects.find(p => p.id === route.projectId) || null;
        projectTab = route.projectTab || 'sessions';
        gscSubView = route.projectTab === 'gsc' ? (route.projectSubView || 'overview') : 'overview';
        providerSubView = route.projectTab === 'providers' ? (route.projectSubView || 'overview') : 'overview';
        showSettings = false; showGlobalStats = false; showAPI = false; showNewCrawl = false; showCompare = false; showLogs = false;
        if (!selectedProject && projects.length === 0) {
          getProjects().then(p => { projects = p; selectedProject = p.find(pr => pr.id === route.projectId) || null; }).catch(() => {});
        }
        if (sessions.length === 0) loadSessions();
        return;
      }

      selectedProject = null;
      if (route.page === 'compare') {
        compareSessionA = route.sessionA || '';
        compareSessionB = route.sessionB || '';
      }

      if (sessions.length === 0) loadSessions();
      if (route.page === 'new-crawl') getProjects().then(p => projects = p).catch(() => {});
      return;
    }

    // Session detail routes
    selectedProject = null;
    showSettings = false;
    showGlobalStats = false;
    showAPI = false;
    showNewCrawl = false;
    showCompare = false;
    showLogs = false;

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
      detailUrl = route.detailUrl;
      filters = {};
    } else {
      tab = route.tab;
      filters = route.filters || {};
      const off = route.offset || 0;
      if (['internal'].includes(tab)) { intLinksOffset = off; }
      else if (['external'].includes(tab)) { extLinksOffset = off; }
      else { pagesOffset = off; }
      if (tab === 'pagerank') {
        if (route.subView && ['top', 'directory', 'distribution', 'table'].includes(route.subView)) {
          prSubView = route.subView;
        }
      } else if (tab === 'reports') {
        if (route.subView && ['overview', 'content', 'technical', 'links', 'structure', 'sitemaps', 'international'].includes(route.subView)) {
          reportsSubView = route.subView;
        } else {
          reportsSubView = 'overview';
        }
      } else if (tab === 'ext-checks') {
        if (route.subView && ['domains', 'urls'].includes(route.subView)) {
          extChecksSubView = route.subView;
        } else {
          extChecksSubView = 'domains';
        }
      } else if (tab !== 'robots' && tab !== 'sitemaps') {
        await loadTabData();
      }
    }
  }

  window.addEventListener('popstate', () => applyRoute());

  async function selectSession(session) {
    selectedSession = session;
    selectedProject = null;
    showSettings = false;
    showGlobalStats = false;
    showAPI = false;
    showNewCrawl = false;
    showCompare = false;
    showLogs = false;
    tab = 'overview';
    filters = {};
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
    selectedProject = null;
    stats = null;
    showNewCrawl = false;
    showSettings = false;
    showAPI = false;
    showGlobalStats = false;
    showCompare = false;
    showLogs = false;
    pushURL('/');
  }

  async function loadSessions() {
    try {
      loading = true;
      const [sessionsData, storageData] = await Promise.all([
        getSessions(),
        getSessionStorage().catch(() => ({})),
      ]);
      sessions = sessionsData || [];
      sessionStorageMap = storageData || {};
      for (const s of sessions) {
        if (s.is_running && !sseConnections[s.ID]) {
          sseConnections[s.ID] = subscribeProgress(s.ID,
            (data) => { liveProgress[s.ID] = data; liveProgress = { ...liveProgress }; scheduleStatsRefresh(s.ID); },
            () => { delete sseConnections[s.ID]; if (selectedSession?.ID === s.ID) { getStats(s.ID).then(st => stats = st).catch(() => {}); } loadSessions(); }
          );
        }
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  // --- Live stats refresh (throttled) ---
  let statsRefreshTimer = null;
  function scheduleStatsRefresh(sessionId) {
    if (statsRefreshTimer) return; // already scheduled
    statsRefreshTimer = setTimeout(async () => {
      statsRefreshTimer = null;
      if (selectedSession?.ID === sessionId) {
        try {
          stats = await getStats(sessionId);
        } catch (_) {}
      }
    }, 5000);
  }

  // --- Update check polling ---
  let updatePollTimer = null;
  function startUpdatePoll() {
    let attempts = 0;
    const maxAttempts = 6; // 6 * 10s = 60s
    updatePollTimer = setInterval(async () => {
      attempts++;
      try {
        const status = await getUpdateStatus();
        if (status.available || status.checked_at || attempts >= maxAttempts) {
          clearInterval(updatePollTimer);
          updatePollTimer = null;
          if (status.available) {
            updateInfo = status;
          }
        }
      } catch {
        if (attempts >= maxAttempts) {
          clearInterval(updatePollTimer);
          updatePollTimer = null;
        }
      }
    }, 10000);
  }
  startUpdatePoll();

  async function doBackupAndUpdate() {
    updatingApp = true;
    updateMessage = '';
    try {
      updateMessage = 'Creating backup...';
      await createBackup();
      updateMessage = 'Downloading and installing update...';
      const result = await applyUpdate();
      updateMessage = result.message || 'Update installed. Restart to apply.';
      updateInfo = null;
    } catch (e) {
      updateMessage = 'Update failed: ' + e.message;
    } finally {
      updatingApp = false;
    }
  }


  async function loadTabData() {
    if (!selectedSession) return;
    const id = selectedSession.ID;
    try {
      if (['overview','titles','meta','headings','images','indexability','response'].includes(tab)) {
        const result = await getPages(id, PAGE_SIZE, pagesOffset, filters);
        pages = result || [];
        hasMorePages = pages.length === PAGE_SIZE;
      } else if (tab === 'internal') {
        const result = await getInternalLinks(id, PAGE_SIZE, intLinksOffset, filters);
        intLinks = result || [];
        hasMoreIntLinks = intLinks.length === PAGE_SIZE;
      } else if (tab === 'external') {
        const result = await getExternalLinks(id, PAGE_SIZE, extLinksOffset, filters);
        extLinks = result || [];
        hasMoreExtLinks = extLinks.length === PAGE_SIZE;
      }
    } catch (e) {
      error = e.message;
    }
  }

  function switchTab(newTab) {
    tab = newTab;
    filters = {};
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (selectedSession) {
      const path = newTab === 'pagerank' ? `${newTab}/${prSubView}` : newTab === 'reports' ? `${newTab}/${reportsSubView}` : newTab;
      pushURL(`/sessions/${selectedSession.ID}/${path}`);
    }
    if (newTab !== 'pagerank' && newTab !== 'robots' && newTab !== 'sitemaps' && newTab !== 'reports' && newTab !== 'tests' && newTab !== 'ext-checks') {
      loadTabData();
    }
  }

  async function nextPage() {
    if (tab === 'internal') { intLinksOffset += PAGE_SIZE; }
    else if (tab === 'external') { extLinksOffset += PAGE_SIZE; }
    else { pagesOffset += PAGE_SIZE; }
    if (selectedSession) pushURL(`/sessions/${selectedSession.ID}/${tab}`, filters, currentOffset());
    await loadTabData();
  }
  async function prevPage() {
    if (tab === 'internal') { intLinksOffset = Math.max(0, intLinksOffset - PAGE_SIZE); }
    else if (tab === 'external') { extLinksOffset = Math.max(0, extLinksOffset - PAGE_SIZE); }
    else { pagesOffset = Math.max(0, pagesOffset - PAGE_SIZE); }
    if (selectedSession) pushURL(`/sessions/${selectedSession.ID}/${tab}`, filters, currentOffset());
    await loadTabData();
  }

  function currentOffset() {
    if (tab === 'internal') return intLinksOffset;
    if (tab === 'external') return extLinksOffset;
    return pagesOffset;
  }

  function onCrawlStarted() {
    showNewCrawl = false;
    pushURL('/');
    setTimeout(() => loadSessions(), 500);
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
    resumeSessionId = id;
    showResumeModal = true;
  }

  function closeResumeModal() {
    showResumeModal = false;
    resumeSessionId = null;
  }

  async function onResumeComplete() {
    closeResumeModal();
    await loadSessions();
    const sess = sessions.find(s => s.ID === resumeSessionId);
    if (sess) {
      await selectSession(sess);
    }
  }

  async function handleDelete(id) {
    const sizeBytes = sessionStorageMap[id];
    const sizeText = sizeBytes ? ` and free ${fmtSize(sizeBytes)}` : '';
    if (!confirm(`Delete this session${sizeText}?`)) return;
    try {
      await deleteSession(id);
      if (selectedSession?.ID === id) { selectedSession = null; pushURL('/'); }
      loadSessions();
    } catch (e) { error = e.message; }
  }


  function openHtmlModal(url) {
    htmlModalUrl = url;
    showHtmlModal = true;
  }

  function closeHtmlModal() {
    showHtmlModal = false;
    htmlModalUrl = '';
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

  async function loadSystemStats() {
    try {
      systemStats = await getSystemStats();
    } catch {}
  }

  function startSystemStatsPolling() {
    if (systemStatsInterval) return;
    loadSystemStats();
    systemStatsInterval = setInterval(loadSystemStats, 3000);
  }


  // Boot
  loadTheme();
  applyRoute();
  startSystemStatsPolling();
  getProjects().then(p => projects = p).catch(() => {});
  if (!globalStats) getGlobalStats().then(gs => globalStats = gs).catch(() => {});

  // Cleanup on destroy
  onDestroy(() => {
    if (systemStatsInterval) clearInterval(systemStatsInterval);
    if (updatePollTimer) clearInterval(updatePollTimer);
    for (const id of Object.keys(sseConnections)) {
      sseConnections[id].close();
      delete sseConnections[id];
    }
  });
</script>

<div class="layout">
  <div class="drag-bar"><span class="drag-bar-title">{theme.app_name}</span></div>
  <Sidebar {theme} {darkMode} {sessions} {projects} {globalStats} {systemStats}
    {selectedSession} {selectedProject} {showNewCrawl} {showSettings} {showGlobalStats} {showAPI} {showCompare} {showLogs} {liveProgress}
    ontoggledarkmmode={toggleDarkMode} onselectsession={selectSession} onselectproject={selectProject}
    onnavigate={navigateTo} onopensettings={openSettings}
    onopenstats={openGlobalStats} onopenapi={openAPI} onopenlogs={openLogs} ongohome={goHome} />

  <!-- Main Content -->
  <main class="main">
    <div class="main-content">
      {#if error}
        <div class="alert alert-error">
          <span>{error}</span>
          <button class="btn btn-sm btn-ghost" onclick={() => error = null}>Dismiss</button>
        </div>
      {/if}

      {#if updateInfo && !updateDismissed}
        <div class="alert alert-info">
          <span>Update available: <strong>v{updateInfo.latest_version}</strong></span>
          <div style="display:flex;gap:8px;align-items:center">
            {#if updatingApp}
              <span style="font-size:13px">{updateMessage}</span>
            {:else}
              {#if updateMessage}
                <span style="font-size:13px">{updateMessage}</span>
              {/if}
              <button class="btn btn-sm btn-primary" onclick={doBackupAndUpdate} disabled={updatingApp}>Backup & Update</button>
              <button class="btn btn-sm btn-ghost" onclick={() => updateDismissed = true}>Dismiss</button>
            {/if}
          </div>
        </div>
      {/if}

      {#if showSettings}
        <SettingsPage initialTheme={theme} onerror={(msg) => error = msg} onsave={handleSettingsSave} oncancel={handleSettingsCancel} />

      {:else if showGlobalStats}
        <GlobalStatsPage onerror={(msg) => error = msg} />

      {:else if showCompare}
        <ComparePage {sessions} initialA={compareSessionA} initialB={compareSessionB}
          onerror={(msg) => error = msg} onnavigate={navigateTo} />

      {:else if showLogs}
        <LogsPage onerror={(msg) => error = msg} />

      {:else if showAPI && !selectedSession}
        <APIManagementPage onerror={(msg) => error = msg} onprojectschanged={(p) => projects = p} />

      {:else if showNewCrawl && !selectedSession}
        <NewCrawlForm {projects} onstart={onCrawlStarted} oncancel={() => navigateTo('/')} onerror={(msg) => error = msg} />

      {:else if selectedProject}
        <!-- Project View -->
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Dashboard</a>
          <span>/</span>
          <span style="color: var(--text);">{selectedProject.name}</span>
        </div>

        <div class="tab-bar">
          <button class="tab" class:tab-active={projectTab === 'sessions'} onclick={() => switchProjectTab('sessions')}>Sessions</button>
          <button class="tab" class:tab-active={projectTab === 'gsc'} onclick={() => switchProjectTab('gsc')}>Search Console</button>
          <button class="tab" class:tab-active={projectTab === 'providers'} onclick={() => switchProjectTab('providers')}>SEO Data</button>
        </div>

        <div class="card card-flush" style="border-top-left-radius: 0; border-top-right-radius: 0; border-top: none;">
          {#if projectTab === 'sessions'}
            {@const projSessions = sessions.filter(s => s.ProjectID === selectedProject.id)}
            {#if projSessions.length > 0}
              <table>
                <thead>
                  <tr>
                    <th>Seed URL</th>
                    <th>Status</th>
                    <th>Pages</th>
                    <th>Started</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {#each projSessions as s}
                    <tr>
                      <td class="cell-url">
                        <a href={`/sessions/${s.ID}/overview`} onclick={(e) => { e.preventDefault(); selectSession(s); }}>
                          {s.SeedURLs?.[0] || s.ID}
                        </a>
                      </td>
                      <td>
                        {#if s.is_running}
                          <span class="badge badge-info">Running</span>
                        {:else if s.Status === 'completed'}
                          <span class="badge badge-success">Completed</span>
                        {:else}
                          <span class="badge">{s.Status || 'Unknown'}</span>
                        {/if}
                      </td>
                      <td>{fmtN(s.PagesCrawled || 0)}</td>
                      <td style="white-space: nowrap; color: var(--text-muted); font-size: 13px;">{s.StartedAt ? timeAgo(s.StartedAt) : '-'}</td>
                      <td>
                        <button class="btn btn-sm" onclick={() => selectSession(s)}>View</button>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {:else}
              <p style="color: var(--text-muted); padding: 40px 0; text-align: center;">No crawl sessions in this project yet.</p>
            {/if}
          {:else if projectTab === 'gsc'}
            <GSCTab projectId={selectedProject.id} initialSubView={gscSubView} onerror={(msg) => error = msg} onpushurl={(u) => pushURL(u)} />
          {:else if projectTab === 'providers'}
            <ProvidersTab projectId={selectedProject.id} initialSubView={providerSubView} onerror={(msg) => error = msg} onpushurl={(u) => pushURL(u)} />
          {/if}
        </div>

      {:else if !selectedSession}
        <SessionsList {sessions} {projects} {liveProgress} {sessionStorageMap} {loading}
          onselectsession={selectSession} onstop={handleStop} onresume={openResumeModal}
          ondelete={handleDelete} onnewcrawl={() => navigateTo('/new-crawl')} onrefresh={loadSessions} />

      {:else if tab === 'url-detail' && selectedSession}
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
          <span>/</span>
          <a href={`/sessions/${selectedSession.ID}/overview`} onclick={(e) => { e.preventDefault(); navigateTo(`/sessions/${selectedSession.ID}/overview`); }}>{selectedSession.SeedURLs?.[0] || selectedSession.ID}</a>
          <span>/</span>
          <span style="color: var(--text);">URL Detail</span>
        </div>
        {#key detailUrl}
          <UrlDetailView sessionId={selectedSession.ID} url={detailUrl}
            onerror={(msg) => error = msg} onnavigate={navigateTo}
            onopenhtml={openHtmlModal} />
        {/key}

      {:else}
        <!-- Session Detail -->
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
          <span>/</span>
          <span style="color: var(--text);">{selectedSession.SeedURLs?.[0] || selectedSession.ID}</span>
        </div>

        <SessionActionBar session={selectedSession} {stats} {liveProgress}
          onerror={(msg) => error = msg} onstop={handleStop} onresume={openResumeModal}
          ondelete={handleDelete} onrefresh={() => selectSession(selectedSession)}
          oncompare={(id) => navigateTo(`/compare?a=${id}`)} />

        {#if stats}
          {@const non200 = stats.status_codes ? Object.entries(stats.status_codes).filter(([k]) => k !== '200').reduce((a, [, v]) => a + v, 0) : stats.error_count}
          {@const maxDepth = stats.depth_distribution ? Math.max(...Object.keys(stats.depth_distribution).map(Number)) : 0}
          <div class="stats-grid">
            <div class="stat-card"><div class="stat-value">{fmtN(stats.total_pages)}</div><div class="stat-label">Pages</div></div>
            <div class="stat-card"><div class="stat-value" style={non200 > 0 ? 'color: var(--error)' : ''}>{fmtN(non200)}</div><div class="stat-label">Non-200</div></div>
            <div class="stat-card"><div class="stat-value">{fmtN(stats.internal_links)}</div><div class="stat-label">Internal links</div></div>
            <div class="stat-card"><div class="stat-value">{fmtN(stats.external_links)}</div><div class="stat-label">External links</div></div>
            <div class="stat-card"><div class="stat-value">{maxDepth}</div><div class="stat-label">Max depth</div></div>
            <div class="stat-card"><div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">Avg response</div></div>
          </div>
          {#if stats.status_codes && Object.keys(stats.status_codes).length > 1}
            <div class="stats-mini" style="margin-top: 8px;">
              {#each Object.entries(stats.status_codes).sort((a, b) => Number(a[0]) - Number(b[0])) as [code, count]}
                <span class="stats-mini-item"><span class="badge {statusBadge(Number(code))}">{code}</span> {fmtN(count)}</span>
              {/each}
            </div>
          {/if}
          <div class="stats-secondary" style="margin-top: 10px;">
            {#if stats.pages_per_second > 0}<span>{stats.pages_per_second.toFixed(1)} pages/sec</span>{/if}
            {#if stats.crawl_duration_sec > 0}<span>{stats.crawl_duration_sec < 60 ? stats.crawl_duration_sec.toFixed(0) + 's' : (stats.crawl_duration_sec / 60).toFixed(1) + 'min'}</span>{/if}
            {#if sessionStorageMap[selectedSession.ID]}<span>{fmtSize(sessionStorageMap[selectedSession.ID])} storage</span>{/if}
          </div>
        {/if}

        <div class="tab-bar">
          {#each TABS as t}
            <button class="tab" class:tab-active={tab === t.id} onclick={() => switchTab(t.id)}>{t.label}</button>
          {/each}
        </div>

        <div class="card card-flush" style="border-top-left-radius: 0; border-top-right-radius: 0; border-top: none;">

          {#if tab === 'overview'}
            <DataTable columns={[{label:'URL'},{label:'Status'},{label:'Title'},{label:'Words'},{label:'Int Out'},{label:'Ext Out'},{label:'Size'},{label:'Time'},{label:'Depth'},{label:'PR'},{label:''}]}
              filterKeys={TAB_FILTERS.overview} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
              hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(p)}
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
                  <td style="color: var(--accent); font-weight: 500;">{p.PageRank > 0 ? p.PageRank.toFixed(1) : '-'}</td>
                  <td>{#if p.BodySize > 0}<button class="btn-html" title="View HTML" onclick={() => openHtmlModal(p.URL)}><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg></button>{/if}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'titles'}
            <DataTable columns={[{label:'URL'},{label:'Title'},{label:'Length'},{label:'H1'}]}
              filterKeys={TAB_FILTERS.titles} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
              hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(p)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                  <td class="cell-title" class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.Title || '-'}</td>
                  <td class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.TitleLength}</td>
                  <td class="cell-title">{p.H1?.[0] || '-'}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'meta'}
            <DataTable columns={[{label:'URL'},{label:'Meta Description'},{label:'Length'},{label:'Meta Keywords'},{label:'OG Title'}]}
              filterKeys={TAB_FILTERS.meta} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
              hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(p)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                  <td class="cell-title" class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{trunc(p.MetaDescription, 80)}</td>
                  <td class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{p.MetaDescLength}</td>
                  <td class="cell-title">{trunc(p.MetaKeywords, 60)}</td>
                  <td class="cell-title">{trunc(p.OGTitle, 60)}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'headings'}
            <DataTable columns={[{label:'URL'},{label:'H1'},{label:'H1 Count'},{label:'H2'},{label:'H2 Count'}]}
              filterKeys={TAB_FILTERS.headings} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
              hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(p)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                  <td class="cell-title" class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.[0] || '-'}</td>
                  <td class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.length || 0}</td>
                  <td class="cell-title">{p.H2?.[0] || '-'}</td>
                  <td>{p.H2?.length || 0}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'images'}
            <DataTable columns={[{label:'URL'},{label:'Images'},{label:'Without Alt'},{label:'Title'},{label:'Words'}]}
              filterKeys={TAB_FILTERS.images} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
              hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(p)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                  <td>{p.ImagesCount}</td>
                  <td class:cell-warn={p.ImagesNoAlt > 0}>{p.ImagesNoAlt}</td>
                  <td class="cell-title">{trunc(p.Title, 50)}</td>
                  <td>{fmtN(p.WordCount)}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'indexability'}
            <DataTable columns={[{label:'URL'},{label:'Indexable'},{label:'Reason'},{label:'Meta Robots'},{label:'Canonical'},{label:'Self'}]}
              filterKeys={TAB_FILTERS.indexability} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
              hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(p)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                  <td><span class="badge" class:badge-success={p.IsIndexable} class:badge-error={!p.IsIndexable}>{p.IsIndexable ? 'Yes' : 'No'}</span></td>
                  <td>{p.IndexReason || '-'}</td>
                  <td>{p.MetaRobots || '-'}</td>
                  <td class="cell-url">{trunc(p.Canonical, 60)}</td>
                  <td>{p.CanonicalIsSelf ? 'Yes' : '-'}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'response'}
            <DataTable columns={[{label:'URL'},{label:'Status'},{label:'Content Type'},{label:'Encoding'},{label:'Size'},{label:'Time'},{label:'Redirects'}]}
              filterKeys={TAB_FILTERS.response} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
              hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(p)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                  <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
                  <td>{p.ContentType || '-'}</td>
                  <td>{p.ContentEncoding || '-'}</td>
                  <td>{fmtSize(p.BodySize)}</td>
                  <td>{fmt(p.FetchDurationMs)}</td>
                  <td>{p.FinalURL !== p.URL ? p.FinalURL : '-'}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'internal'}
            <DataTable columns={[{label:'Source'},{label:'Target'},{label:'Anchor Text'},{label:'Tag'}]}
              filterKeys={TAB_FILTERS.internal} {filters} data={intLinks} offset={intLinksOffset} pageSize={PAGE_SIZE}
              hasMore={hasMoreIntLinks} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(l)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                  <td class="cell-url"><a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a></td>
                  <td class="cell-title">{l.AnchorText || '-'}</td>
                  <td>{l.Tag}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'external'}
            <DataTable columns={[{label:'Source'},{label:'Target'},{label:'Anchor Text'},{label:'Rel'}]}
              filterKeys={TAB_FILTERS.external} {filters} data={extLinks} offset={extLinksOffset} pageSize={PAGE_SIZE}
              hasMore={hasMoreExtLinks} hasActiveFilters={hasActiveFilters()}
              onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
              {#snippet row(l)}
                <tr>
                  <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                  <td class="cell-url"><a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a></td>
                  <td class="cell-title">{l.AnchorText || '-'}</td>
                  <td>{l.Rel || '-'}</td>
                </tr>
              {/snippet}
            </DataTable>

          {:else if tab === 'ext-checks'}
            <ExternalChecksTab sessionId={selectedSession.ID} initialSubView={extChecksSubView} initialFilters={filters}
              onpushurl={(u) => pushURL(u)}
              onnavigate={(t, f) => navigateTo(`/sessions/${selectedSession.ID}/${t}`, f)}
              onerror={(msg) => error = msg} />

          {:else if tab === 'pagerank'}
            <PageRankTab sessionId={selectedSession.ID} initialSubView={prSubView}
              onnavigate={(url) => goToUrlDetail({preventDefault:()=>{}}, url)}
              onpushurl={(u) => pushURL(u)}
              onerror={(msg) => error = msg} />

          {:else if tab === 'robots'}
            <RobotsTab sessionId={selectedSession.ID} onerror={(msg) => error = msg} />

          {:else if tab === 'sitemaps'}
            <SitemapsTab sessionId={selectedSession.ID} onerror={(msg) => error = msg} />
          {:else if tab === 'reports'}
            <ReportsHub sessionId={selectedSession.ID} {stats} initialSubView={reportsSubView}
              onnavigate={(url, f) => navigateTo(url, f)}
              onpushurl={(u) => pushURL(u)}
              onerror={(msg) => error = msg} />
          {:else if tab === 'tests'}
            <CustomTestsTab sessionId={selectedSession.ID} onerror={(msg) => error = msg} />
          {/if}
        </div>
      {/if}
    </div>
  </main>
</div>

{#if showResumeModal}
  <ResumeModal sessionId={resumeSessionId} {sessions} onresume={onResumeComplete} onclose={closeResumeModal} onerror={(msg) => error = msg} />
{/if}

{#if showHtmlModal}
  <HtmlModal sessionId={selectedSession.ID} url={htmlModalUrl} onclose={closeHtmlModal} onerror={(msg) => error = msg} />
{/if}
