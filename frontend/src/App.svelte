<script>
  // TODO: Continue component decomposition:
  //   - GSCTab → extract overview chart + countries/devices into sub-components
  //   - ProvidersTab → extract overview chart + settings into sub-components
  //   - Consider a lightweight store to reduce prop drilling from App.svelte

  // TODO: Increase test coverage (frontend + backend):
  //   - Frontend: add tests for Sidebar, SessionDetailPage, ComparePage, NewCrawlForm
  //   - Frontend: add E2E tests with Playwright
  //   - Backend: add tests for engine shutdown/cancellation, buffer overflow, rate limiting
  //   - Backend: add tests for SSRF protection edge cases, auth middleware

  import { t, setLocale } from './lib/i18n/index.svelte.js';
  import { onDestroy } from 'svelte';
  import {
    getSessions,
    getStats,
    getProgress,
    stopCrawl,
    deleteSession,
    getStorageStats,
    getGlobalStats,
    getSessionStorage,
    getSystemStats,
    getProjects,
    createProject,
    getUpdateStatus,
    applyUpdate,
    createBackup,
    getSetupStatus,
    getTelemetry,
  } from './lib/api.js';
  import { initTelemetry, trackPageView, trackEvent, disableTelemetry } from './lib/telemetry.js';
  import { fmtSize } from './lib/utils.js';
  import { pushURL, parseRoute } from './lib/router.js';
  import { createSSEManager } from './lib/sse.js';
  import { applyTheme, loadThemeFromServer, saveDarkMode } from './lib/theme.js';
  import OnboardingWizard from './lib/components/OnboardingWizard.svelte';
  import ResumeModal from './lib/components/ResumeModal.svelte';
  import NewCrawlForm from './lib/components/NewCrawlForm.svelte';
  import GlobalStatsPage from './lib/components/GlobalStatsPage.svelte';
  import SettingsPage from './lib/components/SettingsPage.svelte';
  import APIManagementPage from './lib/components/APIManagementPage.svelte';
  import SessionsList from './lib/components/SessionsList.svelte';
  import Sidebar from './lib/components/Sidebar.svelte';
  import ComparePage from './lib/components/ComparePage.svelte';
  import LogsPage from './lib/components/LogsPage.svelte';
  import AllProjectsPage from './lib/components/AllProjectsPage.svelte';
  import ConfirmModal from './lib/components/ConfirmModal.svelte';
  import SessionDetailPage from './lib/components/SessionDetailPage.svelte';
  import ProjectPage from './lib/components/ProjectPage.svelte';

  // --- Named constants ---
  const STATS_REFRESH_MS = 5000;
  const UPDATE_POLL_MAX = 6;
  const UPDATE_POLL_MS = 10000;
  const RELOAD_DELAY_MS = 500;
  const STOP_RELOAD_DELAY_MS = 1000;

  function showConfirm(message, onConfirm, opts = {}) {
    confirmState = { message, onConfirm, ...opts };
  }

  // --- Setup state ---
  let showOnboarding = $state(false);
  let onboardingStartStep = $state(1); // 1 = full wizard, 3 = telemetry only (existing users)
  let setupChecked = $state(false);

  // --- Crawl state ---
  let sessions = $state([]);
  let selectedSession = $state(null);
  let stats = $state(null);
  let sessionStorageMap = $state({});
  let loading = $state(true);
  let error = $state(null);

  // --- UI state ---
  let theme = $state({
    app_name: 'CrawlObserver',
    logo_url: '',
    accent_color: '#7c3aed',
    mode: 'light',
  });
  let darkMode = $state(false);
  /** @type {'home'|'settings'|'stats'|'compare'|'logs'|'all-projects'|'api'|'new-crawl'|'project'|'session'} */
  let currentView = $state('home');
  let showResumeModal = $state(false);
  let resumeSessionId = $state(null);
  let resumeModalMode = $state('resume');
  let retryStatusCode = $state(0);
  let retryCount = $state(0);
  let confirmState = $state(null);
  let globalStats = $state(null);
  let compareSessionA = $state('');
  let compareSessionB = $state('');
  let updateInfo = $state(null);
  let appVersion = $state('');
  let updateDismissed = $state(false);
  let updatingApp = $state(false);
  let updateMessage = $state('');
  let storageStats = $state(null);
  let sessionRecordingActive = $state(false);
  let systemStats = $state(null);

  // --- Route-derived state (passed as initial props to page components) ---
  let routeTab = $state('reports');
  let routeFilters = $state({});
  let routeOffset = $state(0);
  let routeDetailUrl = $state('');
  let routeSubView = $state(null);
  let routeProjectTab = $state('sessions');
  let routeGscSubView = $state('overview');
  let routeProviderSubView = $state('overview');
  let routeVersion = $state(0);

  // --- Project state ---
  let projects = $state([]);
  let selectedProject = $state(null);

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
  function selectProject(proj) {
    currentView = 'project';
    selectedProject = proj;
    selectedSession = null;
    routeProjectTab = 'sessions';
    pushURL(`/projects/${proj.id}`);
  }

  // Project CRUD
  async function handleCreateProject(name) {
    try {
      const created = await createProject(name);
      projects = await getProjects();
      const proj = projects.find((p) => p.id === created.id) || created;
      selectProject(proj);
    } catch (e) {
      error = e.message;
    }
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
      // Use server language as fallback when no localStorage preference exists
      if (!localStorage.getItem('locale') && result.theme.language) {
        setLocale(result.theme.language);
      }
    } catch (e) {
      console.warn('Failed to load theme:', e);
    }
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

  async function navigateTo(path, queryFilters = {}) {
    pushURL(path, queryFilters);
    routeVersion++;
    await applyRoute();
  }

  async function applyRoute() {
    trackPageView(location.pathname + location.hash);
    const route = parseRoute();

    // Top-level pages (home, new-crawl, settings, stats, api, project)
    if (route.page) {
      selectedSession = null;
      stats = null;
      loading = false;
      currentView =
        route.page === 'all-projects'
          ? 'all-projects'
          : route.page === 'new-crawl'
            ? 'new-crawl'
            : route.page;

      if (route.page === 'project') {
        currentView = 'project';
        selectedProject = projects.find((p) => p.id === route.projectId) || null;
        routeProjectTab = route.projectTab || 'sessions';
        routeGscSubView =
          route.projectTab === 'gsc' ? route.projectSubView || 'overview' : 'overview';
        routeProviderSubView =
          route.projectTab === 'providers' || route.projectTab?.startsWith('provider:')
            ? route.projectSubView || 'overview'
            : 'overview';
        if (!selectedProject && projects.length === 0) {
          getProjects()
            .then((p) => {
              projects = p;
              selectedProject = p.find((pr) => pr.id === route.projectId) || null;
            })
            .catch(() => {});
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
      if (route.page === 'new-crawl')
        getProjects()
          .then((p) => (projects = p))
          .catch(() => {});
      return;
    }

    // Session detail routes
    currentView = 'session';
    selectedProject = null;

    // Handle old tab redirects
    if (route.redirectFrom) {
      const newPath = route.subView
        ? `/sessions/${route.sessionId}/${route.tab}/${route.subView}`
        : `/sessions/${route.sessionId}/${route.tab}`;
      const sp = new URLSearchParams(window.location.search);
      const qs = sp.toString();
      history.replaceState(null, '', qs ? `${newPath}?${qs}` : newPath);
    }

    // Set route state BEFORE selectedSession to avoid rendering with stale defaults
    if (route.tab === 'url-detail') {
      routeTab = 'url-detail';
      routeDetailUrl = route.detailUrl;
      routeFilters = {};
      routeSubView = null;
    } else {
      routeTab = route.tab;
      routeFilters = route.filters || {};
      routeOffset = route.offset || 0;
      routeSubView = route.subView || null;
    }

    if (!selectedSession || selectedSession.ID !== route.sessionId) {
      if (sessions.length === 0) {
        await loadSessions();
      }
      const found = sessions.find((s) => s.ID === route.sessionId);
      if (found) {
        try {
          stats = await getStats(found.ID);
        } catch (e) {
          console.warn('Stats fetch failed in applyRoute:', e);
          stats = null;
        }
        selectedSession = found;
        loadStorageStats();
      }
    }
  }

  window.addEventListener('popstate', () => {
    routeVersion++;
    applyRoute();
  });

  async function selectSession(session) {
    currentView = 'session';
    selectedProject = null;
    routeTab = 'reports';
    routeFilters = {};
    routeOffset = 0;
    routeSubView = null;
    routeVersion++;
    pushURL(`/sessions/${session.ID}/reports`);
    try {
      stats = await getStats(session.ID);
    } catch (e) {
      error = e.message;
      stats = null;
    }
    selectedSession = session;
    loadStorageStats();
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
        if ((s.is_running || s.is_queued) && !sse.isConnected(s.ID)) {
          sse.connect(
            s.ID,
            (data) => {
              liveProgress[s.ID] = data;
              liveProgress = { ...liveProgress };
              if (data.is_running) scheduleStatsRefresh(s.ID);
            },
            (id) => {
              if (selectedSession?.ID === id) {
                getStats(id)
                  .then((st) => (stats = st))
                  .catch(() => {});
              }
              loadSessions();
            },
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
    if (statsRefreshTimer) return;
    statsRefreshTimer = setTimeout(async () => {
      statsRefreshTimer = null;
      if (selectedSession?.ID === sessionId) {
        try {
          stats = await getStats(sessionId);
        } catch (e) {
          console.warn('Stats refresh failed:', e);
        }
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
        if (status.current_version) appVersion = status.current_version;
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
      updateMessage = t('app.creatingBackup');
      await createBackup();
      updateMessage = t('app.downloadingUpdate');
      const result = await applyUpdate();
      updateMessage = result.message || t('app.updateInstalled');
      updateInfo = null;
    } catch (e) {
      updateMessage = t('app.updateFailed', { error: e.message });
    } finally {
      updatingApp = false;
    }
  }

  function onCrawlStarted() {
    trackEvent('crawl_started_ui');
    currentView = 'home';
    pushURL('/');
    setTimeout(() => loadSessions(), RELOAD_DELAY_MS);
  }

  function updateSessionState(id, patch) {
    sessions = sessions.map((s) => (s.ID === id ? { ...s, ...patch } : s));
    if (selectedSession?.ID === id) {
      selectedSession = { ...selectedSession, ...patch };
    }
  }

  async function handleStop(id) {
    try {
      trackEvent('crawl_stopped_ui');
      updateSessionState(id, { Status: 'stopping', is_running: false });
      await stopCrawl(id);
      setTimeout(async () => {
        await loadSessions();
        const sess = sessions.find((s) => s.ID === id);
        if (sess) {
          if (selectedSession?.ID === id) {
            // Already viewing: update in place without recreating the component
            try {
              stats = await getStats(id);
            } catch (e) {
              console.warn('Stats refresh after stop failed:', e);
            }
            selectedSession = sess;
          } else {
            selectSession(sess);
          }
        }
      }, STOP_RELOAD_DELAY_MS);
    } catch (e) {
      error = e.message;
    }
  }

  function openResumeModal(id) {
    resumeSessionId = id;
    resumeModalMode = 'resume';
    retryStatusCode = 0;
    retryCount = 0;
    showResumeModal = true;
  }

  function openRetryModal(id, statusCode, count) {
    resumeSessionId = id;
    resumeModalMode = 'retry';
    retryStatusCode = statusCode;
    retryCount = count;
    showResumeModal = true;
  }

  function closeResumeModal() {
    showResumeModal = false;
    resumeSessionId = null;
  }

  async function onResumeComplete() {
    trackEvent(resumeModalMode === 'retry' ? 'crawl_retried_ui' : 'crawl_resumed_ui');
    const sid = resumeSessionId;
    updateSessionState(sid, { Status: 'running', is_running: true });
    closeResumeModal();
    await loadSessions();
    const sess = sessions.find((s) => s.ID === sid);
    if (sess) {
      if (selectedSession?.ID === sid) {
        // Already viewing this session: update in place without recreating the component
        try {
          stats = await getStats(sid);
        } catch (e) {
          console.warn('Stats refresh after resume failed:', e);
        }
        selectedSession = sess;
      } else {
        await selectSession(sess);
      }
    }
  }

  function handleDelete(id) {
    const sizeBytes = sessionStorageMap[id];
    const sizeText = sizeBytes ? ` and free ${fmtSize(sizeBytes)}` : '';
    showConfirm(
      `Delete this session${sizeText}?`,
      async () => {
        try {
          await deleteSession(id);
          if (selectedSession?.ID === id) {
            selectedSession = null;
            pushURL('/');
          }
          loadSessions();
          getGlobalStats()
            .then((gs) => (globalStats = gs))
            .catch(() => {});
        } catch (e) {
          error = e.message;
        }
      },
      { danger: true, confirmLabel: t('common.delete') },
    );
  }

  async function loadStorageStats() {
    try {
      storageStats = await getStorageStats();
    } catch (e) {
      console.warn('Failed to load storage stats:', e);
    }
  }

  async function loadSystemStats() {
    try {
      systemStats = await getSystemStats();
    } catch (e) {
      console.warn('Failed to load system stats:', e);
    }
  }

  function startSystemStatsPolling() {
    if (systemStatsInterval) return;
    loadSystemStats();
    systemStatsInterval = setInterval(loadSystemStats, 3000);
  }

  // Boot: check setup status first, then load app
  async function boot() {
    await loadTheme();
    try {
      const setupStatus = await getSetupStatus();
      if (!setupStatus.setup_complete) {
        // Fresh install: full onboarding
        showOnboarding = true;
        onboardingStartStep = 1;
        setupChecked = true;
        return;
      }
      if (!setupStatus.telemetry_asked) {
        // Existing user upgrading: show only telemetry step
        showOnboarding = true;
        onboardingStartStep = 3;
        setupChecked = true;
        return;
      }
    } catch {
      // If setup endpoint fails (e.g. CLI mode), proceed normally
    }
    setupChecked = true;
    await bootApp();
  }

  async function bootApp() {
    startSystemStatsPolling();
    getProjects()
      .then((p) => (projects = p))
      .catch(() => {});
    if (!globalStats)
      getGlobalStats()
        .then((gs) => (globalStats = gs))
        .catch(() => {});

    // Init telemetry BEFORE first applyRoute so pageviews are tracked
    try {
      const tel = await getTelemetry();
      if (tel.enabled) {
        await initTelemetry(tel.instance_id, tel.session_recording);
      }
      sessionRecordingActive = tel.enabled && tel.session_recording;
    } catch {
      // Telemetry init failure is non-fatal
    }

    applyRoute();
  }

  function onOnboardingComplete() {
    showOnboarding = false;
    bootApp();
  }

  boot();

  // Cleanup on destroy
  onDestroy(() => {
    if (systemStatsInterval) clearInterval(systemStatsInterval);
    if (updatePollTimer) clearInterval(updatePollTimer);
    sse.disconnectAll();
  });
</script>

{#if showOnboarding}
  <OnboardingWizard startStep={onboardingStartStep} oncomplete={onOnboardingComplete} />
{:else if setupChecked}
  <a class="skip-link" href="#main-content">{t('app.skipToContent')}</a>
  <div class="layout">
    <div class="drag-bar"><span class="drag-bar-title">{theme.app_name}</span></div>
    <Sidebar
      {theme}
      {darkMode}
      {sessions}
      {projects}
      {globalStats}
      {systemStats}
      {selectedSession}
      {selectedProject}
      {currentView}
      {liveProgress}
      ontoggledarkmode={toggleDarkMode}
      onselectsession={selectSession}
      onselectproject={selectProject}
      onnavigate={navigateTo}
      onopensettings={openSettings}
      onopenstats={openGlobalStats}
      onopenapi={openAPI}
      onopenlogs={openLogs}
      ongohome={goHome}
      oncreateproject={handleCreateProject}
      onviewallprojects={openAllProjects}
      {appVersion}
    />

    <!-- Main Content -->
    <main class="main" id="main-content">
      <div class="main-content">
        {#if error}
          <div class="alert alert-error">
            <span>{error}</span>
            <button class="btn btn-sm btn-ghost" onclick={() => (error = null)}
              >{t('common.dismiss')}</button
            >
          </div>
        {/if}

        {#if updateInfo && !updateDismissed}
          <div class="alert alert-info">
            <span>{t('app.updateAvailable', { version: updateInfo.latest_version })}</span>
            <div class="flex-center-gap">
              {#if updatingApp}
                <span class="text-sm">{updateMessage}</span>
              {:else}
                {#if updateMessage}
                  <span class="text-sm">{updateMessage}</span>
                {/if}
                <button
                  class="btn btn-sm btn-primary"
                  onclick={doBackupAndUpdate}
                  disabled={updatingApp}>{t('app.backupAndUpdate')}</button
                >
                <button class="btn btn-sm btn-ghost" onclick={() => (updateDismissed = true)}
                  >{t('common.dismiss')}</button
                >
              {/if}
            </div>
          </div>
        {/if}

        {#if sessionRecordingActive}
          <div class="alert alert-session-recording">
            <span>{t('settings.sessionRecordingBanner')}</span>
          </div>
        {/if}

        {#if currentView === 'settings'}
          <SettingsPage
            initialTheme={theme}
            onerror={(msg) => (error = msg)}
            onsave={handleSettingsSave}
            oncancel={handleSettingsCancel}
            onsessionrecording={(v) => (sessionRecordingActive = v)}
          />
        {:else if currentView === 'stats'}
          <GlobalStatsPage onerror={(msg) => (error = msg)} />
        {:else if currentView === 'compare'}
          <ComparePage
            {sessions}
            initialA={compareSessionA}
            initialB={compareSessionB}
            onerror={(msg) => (error = msg)}
            onnavigate={navigateTo}
          />
        {:else if currentView === 'logs'}
          <LogsPage onerror={(msg) => (error = msg)} />
        {:else if currentView === 'all-projects'}
          <AllProjectsPage
            onerror={(msg) => (error = msg)}
            onselectproject={selectProject}
            oncreateproject={() => navigateTo('/new-crawl')}
          />
        {:else if currentView === 'api'}
          <APIManagementPage
            onerror={(msg) => (error = msg)}
            onprojectschanged={(p) => (projects = p)}
          />
        {:else if currentView === 'new-crawl'}
          <NewCrawlForm
            {projects}
            initialProjectId={new URLSearchParams(window.location.search).get('project') || ''}
            onstart={onCrawlStarted}
            oncancel={() => {
              const proj = new URLSearchParams(window.location.search).get('project');
              proj ? navigateTo(`/projects/${proj}`) : navigateTo('/');
            }}
            onerror={(msg) => (error = msg)}
          />
        {:else if currentView === 'project' && selectedProject}
          {#key selectedProject.id}
            <ProjectPage
              project={selectedProject}
              initialProjectTab={routeProjectTab}
              initialGscSubView={routeGscSubView}
              initialProviderSubView={routeProviderSubView}
              onerror={(msg) => (error = msg)}
              onselectsession={selectSession}
              ongohome={goHome}
              onnewcrawl={() => navigateTo('/new-crawl', { project: selectedProject?.id })}
              onprojectrenamed={async (id) => {
                projects = await getProjects();
                selectedProject = projects.find((p) => p.id === id) || selectedProject;
              }}
              onprojectdeleted={async () => {
                projects = await getProjects();
                goHome();
              }}
              onpushurl={(u) => pushURL(u)}
            />
          {/key}
        {:else if currentView === 'home'}
          <SessionsList
            {sessions}
            {projects}
            {liveProgress}
            {sessionStorageMap}
            {loading}
            onselectsession={selectSession}
            onstop={handleStop}
            onresume={openResumeModal}
            ondelete={handleDelete}
            onnewcrawl={() => navigateTo('/new-crawl')}
            onrefresh={loadSessions}
          />
        {:else if currentView === 'session' && selectedSession}
          {#key selectedSession.ID + '-' + routeVersion}
            <SessionDetailPage
              session={selectedSession}
              {stats}
              {liveProgress}
              {projects}
              initialTab={routeTab}
              initialFilters={routeFilters}
              initialOffset={routeOffset}
              initialDetailUrl={routeDetailUrl}
              initialSubView={routeSubView}
              onerror={(msg) => (error = msg)}
              onstop={handleStop}
              onresume={openResumeModal}
              onretry={openRetryModal}
              ondelete={handleDelete}
              onrefresh={() => selectSession(selectedSession)}
              oncompare={(id) => navigateTo(`/compare?a=${id}`)}
              onnavigate={navigateTo}
              ongohome={goHome}
            />
          {/key}
        {/if}
      </div>
    </main>
  </div>

  {#if showResumeModal}
    <ResumeModal
      sessionId={resumeSessionId}
      {sessions}
      {projects}
      mode={resumeModalMode}
      {retryStatusCode}
      {retryCount}
      onresume={onResumeComplete}
      onclose={closeResumeModal}
      onerror={(msg) => (error = msg)}
    />
  {/if}

  {#if confirmState}
    <ConfirmModal
      message={confirmState.message}
      danger={confirmState.danger}
      confirmLabel={confirmState.confirmLabel}
      onconfirm={() => {
        confirmState.onConfirm();
        confirmState = null;
      }}
      oncancel={() => (confirmState = null)}
    />
  {/if}
{/if}
