<script>
  import { onDestroy } from 'svelte';
  import { getSessions, getStats, getPages, getExternalLinks, getInternalLinks, getProgress,
    stopCrawl, deleteSession,
    getStorageStats, getGlobalStats, getSessionStorage, getSystemStats,
    getProjects, createProject, renameProject, deleteProject,
    getUpdateStatus, applyUpdate,
    createBackup, getSessionsPaginated } from './lib/api.js';
  import { statusBadge, fmt, fmtSize, fmtN, trunc, timeAgo, a11yKeydown } from './lib/utils.js';
  import { PAGE_SIZE, TAB_FILTERS, TABS } from './lib/tabColumns.js';
  import { pushURL, parseRoute } from './lib/router.js';
  import { createSSEManager } from './lib/sse.js';
  import { applyTheme, loadThemeFromServer, saveDarkMode } from './lib/theme.js';
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
  import ResourceChecksTab from './lib/components/ResourceChecksTab.svelte';
  import LogsPage from './lib/components/LogsPage.svelte';
  import AllProjectsPage from './lib/components/AllProjectsPage.svelte';
  import UrlDetailView from './lib/components/UrlDetailView.svelte';
  import DataTable from './lib/components/DataTable.svelte';

  // --- Named constants ---
  const STATS_REFRESH_MS = 5000;
  const UPDATE_POLL_MAX = 6;
  const UPDATE_POLL_MS = 10000;
  const RELOAD_DELAY_MS = 500;
  const STOP_RELOAD_DELAY_MS = 1000;
  const PROJ_SESSIONS_LIMIT = 30;

  /** @param {HTMLElement} node */
  function focusOnMount(node) { node.focus(); }

  // --- Crawl state ---
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
  let filters = $state({});

  // --- UI state ---
  let theme = $state({ app_name: 'CrawlObserver', logo_url: '', accent_color: '#7c3aed', mode: 'light' });
  let darkMode = $state(false);
  /** @type {'home'|'settings'|'stats'|'compare'|'logs'|'all-projects'|'api'|'new-crawl'|'project'|'session'} */
  let currentView = $state('home');
  let showResumeModal = $state(false);
  let resumeSessionId = $state(null);
  let showHtmlModal = $state(false);
  let htmlModalUrl = $state('');
  let globalStats = $state(null);
  let compareSessionA = $state('');
  let compareSessionB = $state('');
  let updateInfo = $state(null);
  let updateDismissed = $state(false);
  let updatingApp = $state(false);
  let updateMessage = $state('');
  let storageStats = $state(null);
  let systemStats = $state(null);

  // --- Pagination ---
  let pagesOffset = $state(0);
  let extLinksOffset = $state(0);
  let intLinksOffset = $state(0);
  let hasMorePages = $state(false);
  let hasMoreExtLinks = $state(false);
  let hasMoreIntLinks = $state(false);

  // --- Sub-views ---
  let prSubView = $state('top');
  let reportsSubView = $state('overview');
  let extChecksSubView = $state('domains');
  let resourcesSubView = $state('summary');
  let gscSubView = $state('overview');
  let providerSubView = $state('overview');

  // --- Project state ---
  let projects = $state([]);
  let selectedProject = $state(null);
  let projectTab = $state('sessions');
  let projSessions = $state([]);
  let projSessionsTotal = $state(0);
  let projSessionsOffset = $state(0);

  // --- Live progress ---
  let liveProgress = $state({});
  const sse = createSSEManager();
  let statsRefreshTimer = null;
  let updatePollTimer = null;
  let systemStatsInterval = null;

  // --- All Projects page ---
  function openAllProjects() {
    currentView = 'all-projects';
    selectedSession = null;
    selectedProject = null;
    pushURL('/projects');
  }

  // --- Project view ---
  async function loadProjectSessions() {
    if (!selectedProject) return;
    try {
      const res = await getSessionsPaginated(PROJ_SESSIONS_LIMIT, projSessionsOffset, { projectId: selectedProject.id });
      projSessions = res.sessions || [];
      projSessionsTotal = res.total || 0;
    } catch (e) { error = e.message; }
  }

  function selectProject(proj) {
    currentView = 'project';
    selectedProject = proj;
    selectedSession = null;
    projectTab = 'sessions';
    projSessionsOffset = 0;
    loadProjectSessions();
    pushURL(`/projects/${proj.id}`);
  }

  // Project CRUD
  async function handleCreateProject(name) {
    try {
      const created = await createProject(name);
      projects = await getProjects();
      const proj = projects.find(p => p.id === created.id) || created;
      selectProject(proj);
    } catch (e) { error = e.message; }
  }

  async function handleRenameProject(id, name) {
    try {
      await renameProject(id, name);
      projects = await getProjects();
      if (selectedProject?.id === id) {
        selectedProject = projects.find(p => p.id === id) || selectedProject;
      }
    } catch (e) { error = e.message; }
  }

  async function handleDeleteProject(id) {
    if (!confirm(`Delete project "${selectedProject?.name}"? Sessions will be unassigned.`)) return;
    try {
      await deleteProject(id);
      projects = await getProjects();
      goHome();
    } catch (e) { error = e.message; }
  }

  let renamingProject = $state(false);
  let renameValue = $state('');

  function startRenameProject() {
    renamingProject = true;
    renameValue = selectedProject?.name || '';
  }

  function confirmRenameProject() {
    const name = renameValue.trim();
    if (name && name !== selectedProject?.name) {
      handleRenameProject(selectedProject.id, name);
    }
    renamingProject = false;
  }

  function cancelRenameProject() {
    renamingProject = false;
  }

  function switchProjectTab(t) {
    projectTab = t;
    if (selectedProject) pushURL(`/projects/${selectedProject.id}/${t}`);
  }

  // --- Global Stats ---
  function openGlobalStats() {
    currentView = 'stats';
    selectedSession = null;
    selectedProject = null;
    pushURL('/stats');
  }

  // --- API page ---
  function openAPI() {
    currentView = 'api';
    selectedSession = null;
    selectedProject = null;
    pushURL('/api');
  }

  function openLogs() {
    currentView = 'logs';
    selectedSession = null;
    selectedProject = null;
    pushURL('/logs');
  }


  // --- Theme ---
  async function loadTheme() {
    try {
      const result = await loadThemeFromServer();
      theme = result.theme;
      darkMode = result.darkMode;
      applyTheme(theme, darkMode);
    } catch {}
  }

  function toggleDarkMode() {
    darkMode = !darkMode;
    saveDarkMode(darkMode);
    applyTheme(theme, darkMode);
  }

  // --- Settings ---
  function openSettings() {
    currentView = 'settings';
    selectedSession = null;
    selectedProject = null;
    pushURL('/settings');
  }

  function handleSettingsSave(saved, isPreview) {
    if (isPreview) {
      theme.accent_color = saved.accent_color;
      theme.app_name = saved.app_name;
      theme.logo_url = saved.logo_url;
      darkMode = saved.mode === 'dark';
      applyTheme(theme, darkMode);
    } else {
      theme = saved;
      darkMode = saved.mode === 'dark';
      applyTheme(theme, darkMode);
      currentView = 'home';
    }
  }

  function handleSettingsCancel() {
    loadTheme();
    currentView = 'home';
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
      currentView = route.page === 'all-projects' ? 'all-projects'
        : route.page === 'new-crawl' ? 'new-crawl'
        : route.page;

      if (route.page === 'project') {
        currentView = 'project';
        selectedProject = projects.find(p => p.id === route.projectId) || null;
        projectTab = route.projectTab || 'sessions';
        gscSubView = route.projectTab === 'gsc' ? (route.projectSubView || 'overview') : 'overview';
        providerSubView = route.projectTab === 'providers' ? (route.projectSubView || 'overview') : 'overview';
        if (!selectedProject && projects.length === 0) {
          getProjects().then(p => { projects = p; selectedProject = p.find(pr => pr.id === route.projectId) || null; if (selectedProject) loadProjectSessions(); }).catch(() => {});
        } else {
          projSessionsOffset = 0;
          loadProjectSessions();
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
    currentView = 'session';
    selectedProject = null;

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
      } else if (tab === 'resources') {
        if (route.subView && ['summary', 'urls'].includes(route.subView)) {
          resourcesSubView = route.subView;
        } else {
          resourcesSubView = 'summary';
        }
      } else if (tab !== 'robots' && tab !== 'sitemaps') {
        await loadTabData();
      }
    }
  }

  window.addEventListener('popstate', () => applyRoute());

  async function selectSession(session) {
    currentView = 'session';
    selectedSession = session;
    selectedProject = null;
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
    currentView = 'home';
    selectedSession = null;
    selectedProject = null;
    stats = null;
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
        if (s.is_running && !sse.isConnected(s.ID)) {
          sse.connect(s.ID,
            (data) => { liveProgress[s.ID] = data; liveProgress = { ...liveProgress }; scheduleStatsRefresh(s.ID); },
            (id) => { if (selectedSession?.ID === id) { getStats(id).then(st => stats = st).catch(() => {}); } loadSessions(); }
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
  function scheduleStatsRefresh(sessionId) {
    if (statsRefreshTimer) return; // already scheduled
    statsRefreshTimer = setTimeout(async () => {
      statsRefreshTimer = null;
      if (selectedSession?.ID === sessionId) {
        try {
          stats = await getStats(sessionId);
        } catch (_) {}
      }
    }, STATS_REFRESH_MS);
  }

  // --- Update check polling ---
  function startUpdatePoll() {
    let attempts = 0;
    updatePollTimer = setInterval(async () => {
      attempts++;
      try {
        const status = await getUpdateStatus();
        if (status.available || status.checked_at || attempts >= UPDATE_POLL_MAX) {
          clearInterval(updatePollTimer);
          updatePollTimer = null;
          if (status.available) {
            updateInfo = status;
          }
        }
      } catch {
        if (attempts >= UPDATE_POLL_MAX) {
          clearInterval(updatePollTimer);
          updatePollTimer = null;
        }
      }
    }, UPDATE_POLL_MS);
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
    if (newTab !== 'pagerank' && newTab !== 'robots' && newTab !== 'sitemaps' && newTab !== 'reports' && newTab !== 'tests' && newTab !== 'ext-checks' && newTab !== 'resources') {
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
    currentView = 'home';
    pushURL('/');
    setTimeout(() => loadSessions(), RELOAD_DELAY_MS);
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
      }, STOP_RELOAD_DELAY_MS);
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
    sse.disconnectAll();
  });
</script>

<a class="skip-link" href="#main-content">Skip to content</a>
<div class="layout">
  <div class="drag-bar"><span class="drag-bar-title">{theme.app_name}</span></div>
  <Sidebar {theme} {darkMode} {sessions} {projects} {globalStats} {systemStats}
    {selectedSession} {selectedProject} {currentView} {liveProgress}
    ontoggledarkmode={toggleDarkMode} onselectsession={selectSession} onselectproject={selectProject}
    onnavigate={navigateTo} onopensettings={openSettings}
    onopenstats={openGlobalStats} onopenapi={openAPI} onopenlogs={openLogs} ongohome={goHome}
    oncreateproject={handleCreateProject} onviewallprojects={openAllProjects} />

  <!-- Main Content -->
  <main class="main" id="main-content">
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
          <div class="flex-center-gap">
            {#if updatingApp}
              <span class="text-sm">{updateMessage}</span>
            {:else}
              {#if updateMessage}
                <span class="text-sm">{updateMessage}</span>
              {/if}
              <button class="btn btn-sm btn-primary" onclick={doBackupAndUpdate} disabled={updatingApp}>Backup & Update</button>
              <button class="btn btn-sm btn-ghost" onclick={() => updateDismissed = true}>Dismiss</button>
            {/if}
          </div>
        </div>
      {/if}

      {#if currentView === 'settings'}
        <SettingsPage initialTheme={theme} onerror={(msg) => error = msg} onsave={handleSettingsSave} oncancel={handleSettingsCancel} />

      {:else if currentView === 'stats'}
        <GlobalStatsPage onerror={(msg) => error = msg} />

      {:else if currentView === 'compare'}
        <ComparePage {sessions} initialA={compareSessionA} initialB={compareSessionB}
          onerror={(msg) => error = msg} onnavigate={navigateTo} />

      {:else if currentView === 'logs'}
        <LogsPage onerror={(msg) => error = msg} />

      {:else if currentView === 'all-projects'}
        <AllProjectsPage onerror={(msg) => error = msg}
          onselectproject={selectProject}
          oncreateproject={() => navigateTo('/new-crawl')} />

      {:else if currentView === 'api'}
        <APIManagementPage onerror={(msg) => error = msg} onprojectschanged={(p) => projects = p} />

      {:else if currentView === 'new-crawl'}
        <NewCrawlForm {projects} onstart={onCrawlStarted} oncancel={() => navigateTo('/')} onerror={(msg) => error = msg} />

      {:else if currentView === 'project' && selectedProject}
        <!-- Project View -->
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Dashboard</a>
          <span>/</span>
          {#if renamingProject}
            <input class="project-rename-input" type="text" bind:value={renameValue}
              use:focusOnMount
              onkeydown={(e) => { if (e.key === 'Enter') confirmRenameProject(); if (e.key === 'Escape') cancelRenameProject(); }}
              onblur={confirmRenameProject} />
          {:else}
            <button class="inline-btn breadcrumb-active" ondblclick={startRenameProject} title="Double-click to rename">{selectedProject.name}</button>
          {/if}
          <button class="project-delete-btn" onclick={() => handleDeleteProject(selectedProject.id)} title="Delete project" aria-label="Delete project">
            <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
          </button>
        </div>

        <div class="tab-bar">
          <button class="tab" class:tab-active={projectTab === 'sessions'} onclick={() => switchProjectTab('sessions')}>Sessions</button>
          <button class="tab" class:tab-active={projectTab === 'gsc'} onclick={() => switchProjectTab('gsc')}>Search Console</button>
          <button class="tab" class:tab-active={projectTab === 'providers'} onclick={() => switchProjectTab('providers')}>SEO Data</button>
        </div>

        <div class="card card-flush card-tab-body">
          {#if projectTab === 'sessions'}
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
                      <td class="nowrap text-muted text-sm">{s.StartedAt ? timeAgo(s.StartedAt) : '-'}</td>
                      <td>
                        <button class="btn btn-sm" onclick={() => selectSession(s)}>View</button>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
              {#if projSessionsTotal > PROJ_SESSIONS_LIMIT}
                <div class="pagination-controls">
                  <button class="btn btn-sm" onclick={() => { projSessionsOffset = Math.max(0, projSessionsOffset - PROJ_SESSIONS_LIMIT); loadProjectSessions(); }} disabled={projSessionsOffset === 0}>Previous</button>
                  <span class="text-sm text-muted">{projSessionsOffset + 1}-{Math.min(projSessionsOffset + PROJ_SESSIONS_LIMIT, projSessionsTotal)} of {projSessionsTotal}</span>
                  <button class="btn btn-sm" onclick={() => { projSessionsOffset += PROJ_SESSIONS_LIMIT; loadProjectSessions(); }} disabled={projSessionsOffset + PROJ_SESSIONS_LIMIT >= projSessionsTotal}>Next</button>
                </div>
              {/if}
            {:else}
              <p class="loading-msg">No crawl sessions in this project yet.</p>
            {/if}
          {:else if projectTab === 'gsc'}
            <GSCTab projectId={selectedProject.id} initialSubView={gscSubView} onerror={(msg) => error = msg} onpushurl={(u) => pushURL(u)} />
          {:else if projectTab === 'providers'}
            <ProvidersTab projectId={selectedProject.id} initialSubView={providerSubView} onerror={(msg) => error = msg} onpushurl={(u) => pushURL(u)} />
          {/if}
        </div>

      {:else if currentView === 'home'}
        <SessionsList {sessions} {projects} {liveProgress} {sessionStorageMap} {loading}
          onselectsession={selectSession} onstop={handleStop} onresume={openResumeModal}
          ondelete={handleDelete} onnewcrawl={() => navigateTo('/new-crawl')} onrefresh={loadSessions} />

      {:else if currentView === 'session' && tab === 'url-detail' && selectedSession}
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
          <span>/</span>
          <a href={`/sessions/${selectedSession.ID}/overview`} onclick={(e) => { e.preventDefault(); navigateTo(`/sessions/${selectedSession.ID}/overview`); }}>{selectedSession.SeedURLs?.[0] || selectedSession.ID}</a>
          <span>/</span>
          <span class="breadcrumb-active">URL Detail</span>
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
          <span class="breadcrumb-active">{selectedSession.SeedURLs?.[0] || selectedSession.ID}</span>
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
            <div class="stats-mini mt-sm">
              {#each Object.entries(stats.status_codes).sort((a, b) => Number(a[0]) - Number(b[0])) as [code, count]}
                <span class="stats-mini-item"><span class="badge {statusBadge(Number(code))}">{code}</span> {fmtN(count)}</span>
              {/each}
            </div>
          {/if}
          <div class="stats-secondary stats-secondary-gap">
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

        <div class="card card-flush card-tab-body">

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
                  <td class="text-accent font-medium">{p.PageRank > 0 ? p.PageRank.toFixed(1) : '-'}</td>
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

          {:else if tab === 'resources'}
            <ResourceChecksTab sessionId={selectedSession.ID} initialSubView={resourcesSubView} initialFilters={filters}
              onpushurl={(u) => pushURL(u)}
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

<style>
  .breadcrumb-active {
    color: var(--text);
  }
  .card-tab-body {
    border-top-left-radius: 0;
    border-top-right-radius: 0;
    border-top: none;
  }
  .pagination-controls {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    padding: 12px 0;
  }
  .stats-secondary-gap {
    margin-top: 10px;
  }
</style>
