<script>
  import { t } from '../i18n/index.svelte.js';
  import { fmtN, fmtSize } from '../utils.js';
  import { getProjectsPaginated } from '../api.js';

  let {
    theme,
    darkMode,
    sessions,
    projects,
    globalStats,
    systemStats,
    selectedSession,
    selectedProject,
    currentView,
    liveProgress,
    ontoggledarkmode,
    onselectsession,
    onselectproject,
    onnavigate,
    onopensettings,
    onopenstats,
    onopenapi,
    onopenlogs,
    ongohome,
    oncreateproject,
    onviewallprojects,
    appVersion,
  } = $props();

  let isDark = $derived(
    darkMode === 'auto'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : !!darkMode,
  );

  /** @param {HTMLElement} node */
  function focusOnMount(node) {
    node.focus();
  }

  let creatingProject = $state(false);
  let newProjectName = $state('');

  // Search state
  let projectSearch = $state('');
  let searchResults = $state(null);
  let searchTimer = null;

  function startCreate() {
    creatingProject = true;
    newProjectName = '';
  }

  function confirmCreate() {
    const name = newProjectName.trim();
    if (name) {
      oncreateproject?.(name);
    }
    creatingProject = false;
    newProjectName = '';
  }

  function cancelCreate() {
    creatingProject = false;
    newProjectName = '';
  }

  function onSearchInput(e) {
    const val = e.target.value;
    projectSearch = val;
    if (searchTimer) clearTimeout(searchTimer);
    if (!val.trim()) {
      searchResults = null;
      return;
    }
    searchTimer = setTimeout(async () => {
      try {
        const res = await getProjectsPaginated(30, 0, val.trim());
        searchResults = res.projects;
      } catch {
        searchResults = null;
      }
    }, 300);
  }

  let displayedProjects = $derived(searchResults !== null ? searchResults : projects.slice(0, 30));

  let collapsed = $state(localStorage.getItem('sidebar-collapsed') === 'true');

  function applySidebarWidth() {
    document.documentElement.style.setProperty('--sidebar-width', collapsed ? '56px' : '260px');
  }
  applySidebarWidth();

  function toggleCollapse() {
    collapsed = !collapsed;
    localStorage.setItem('sidebar-collapsed', collapsed);
    applySidebarWidth();
  }
</script>

<aside class="sidebar" class:collapsed>
  <div class="sidebar-header">
    {#if theme.logo_url}
      <img class="sidebar-logo" src={theme.logo_url} alt="Logo" />
    {:else if isDark}
      <svg
        class="sidebar-logo"
        viewBox="0 0 224 213"
        width="36"
        height="36"
        xmlns="http://www.w3.org/2000/svg"
      >
        <g fill="#fff" fill-rule="evenodd" clip-rule="evenodd">
          <circle cx="9.2" cy="37.5" r="9.2" />
          <circle cx="13.2" cy="11.5" r="8.2" />
          <circle cx="40.8" cy="50.9" r="15.8" />
          <path
            d="M219.7 199.8l-42.1-42C190.9 141.1 199 120 199 97c0-53.6-43.4-97-97-97C76.8 0 53.9 9.6 36.6 25.3c1.3-.2 2.7-.3 4.1-.3 6.5 0 12.5 2.5 17 6.6C70.3 23 85.5 18 102 18c43.6 0 79 35.4 79 79s-35.4 79-79 79-79-35.4-79-79c0-9 1.5-17.7 4.3-25.8-6.4-5-10.8-11.8-11.5-19.7C8.9 65.8 5 81 5 97c0 53.6 43.4 97 97 97 19.9 0 38.4-6 53.8-16.3l43 43c3.9 3.9 11.5 2.6 17-2.9l1-1c5.5-5.5 6.7-13.1 2.9-17z"
          />
          <path d="M56 124l34-28 4 5-35 28-3-5z" />
          <path d="M106 99l24 28-5 3-22-27 3-4z" />
          <path d="M96 82l12-39 4 2-11 39-5-2z" />
          <path d="M156 85l-20 44 4 2 20-45-4-1z" />
          <path d="M153 72l-36-22-2 4 35 22 3-4z" />
          <path d="M90 83L56 58l-3 4 33 24 4-3z" />
          <path d="M106 90l44-10v5l-44 10v-5z" />
          <path
            d="M87.7 79l18.7.4c2.9.1 5.1 2.5 5.1 5.3l-.5 18.7c0 2.8-2.4 5.1-5.3 5l-18.7-.4c-2.8-.1-5.1-2.4-5-5.3l.4-18.6c.1-2.9 2.4-5.1 5.3-5.1z"
          />
          <circle cx="53.5" cy="129.8" r="12.5" />
          <circle cx="110.5" cy="45.8" r="12.5" />
          <circle cx="155.8" cy="80.5" r="12.5" />
          <circle cx="131.8" cy="132.5" r="12.5" />
        </g>
      </svg>
    {:else}
      <svg
        class="sidebar-logo"
        viewBox="0 0 224 213"
        width="36"
        height="36"
        xmlns="http://www.w3.org/2000/svg"
      >
        <ellipse fill="#fff" cx="97.667" cy="91.346" rx="91.333" ry="89.346" />
        <circle fill="#FF8F00" cx="9.167" cy="37.5" r="9.167" />
        <circle fill="#FF8F00" cx="13.167" cy="11.5" r="8.167" />
        <circle fill="#FF8F00" cx="40.75" cy="50.916" r="15.75" />
        <path
          fill="#FFA300"
          d="M102,23c-15.7,0-30.248,4.903-42.224,13.242C63.2,40.296,65.25,45.421,65.25,51c0,13.117-11.305,23.75-25.25,23.75c-2.856,0-5.599-.453-8.159-1.275C29.363,80.868,28,88.772,28,97c0,40.869,33.131,74,74,74s74-33.131,74-74S142.869,23,102,23z"
        />
        <path fill="#fff" d="M56,124l34-28l4,5l-35,28L56,124z" />
        <path fill="#fff" d="M106,99l24,28l-5,3l-22-27L106,99z" />
        <path fill="#fff" d="M96,82l12-39l4,2l-11,39L96,82z" />
        <path fill="#fff" d="M156,85l-20,44l4,2l20-45L156,85z" />
        <path fill="#fff" d="M153,72l-36-22l-2,4l35,22L153,72z" />
        <path fill="#fff" d="M90,83L56,58l-3,4l33,24L90,83z" />
        <path fill="#fff" d="M106,90l44-10v5l-44,10V90z" />
        <path
          fill="#fff"
          d="M87.72,79.005l18.681.433c2.866.066,5.135,2.438,5.067,5.298l-.438,18.64c-.066,2.859-2.444,5.124-5.311,5.058l-18.681-.434c-2.866-.066-5.135-2.438-5.067-5.298l.438-18.64C82.476,81.203,84.854,78.939,87.72,79.005z"
        />
        <circle fill="#fff" cx="53.5" cy="129.834" r="12.5" />
        <circle fill="#fff" cx="110.5" cy="45.834" r="12.5" />
        <circle fill="#fff" cx="155.833" cy="80.5" r="12.5" />
        <circle fill="#fff" cx="131.833" cy="132.501" r="12.5" />
        <path
          fill="#3D3D3D"
          d="M219.674,199.824l-42.065-42.065C190.988,141.132,199,120.003,199,97c0-53.572-43.429-97-97-97C76.821,0,53.884,9.595,36.642,25.327c1.311-.212,2.654-.327,4.025-.327,6.546,0,12.502,2.519,16.959,6.637C70.275,23.033,85.548,18,102,18c43.631,0,79,35.37,79,79c0,43.631-35.369,79-79,79s-79-35.369-79-79c0-9.056,1.543-17.746,4.349-25.847C20.996,67.145,16.572,60.36,15.792,52.5C8.897,65.828,5,80.958,5,97c0,53.572,43.429,97,97,97c19.921,0,38.438-6.009,53.842-16.309l42.982,42.982c3.905,3.904,11.516,2.625,16.999-2.857l.993-.993C222.299,211.34,223.578,203.729,219.674,199.824z"
        />
      </svg>
    {/if}
    {#if !collapsed}<span class="sidebar-app-name">{theme.app_name}</span>{/if}
  </div>

  <div class="sidebar-section">
    {#if !collapsed}<div class="sidebar-section-title">{t('sidebar.mainMenu')}</div>{/if}
    <nav class="sidebar-nav">
      <button
        class="sidebar-link"
        class:active={currentView === 'home'}
        onclick={() => ongohome?.()}
        title={collapsed ? t('sidebar.dashboard') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><rect x="3" y="3" width="7" height="7" /><rect x="14" y="3" width="7" height="7" /><rect
            x="3"
            y="14"
            width="7"
            height="7"
          /><rect x="14" y="14" width="7" height="7" /></svg
        >
        {#if !collapsed}{t('sidebar.dashboard')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'new-crawl'}
        onclick={() => onnavigate?.('/new-crawl')}
        title={collapsed ? t('sidebar.newCrawl') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="16" /><line
            x1="8"
            y1="12"
            x2="16"
            y2="12"
          /></svg
        >
        {#if !collapsed}{t('sidebar.newCrawl')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'compare'}
        onclick={() => onnavigate?.('/compare')}
        title={collapsed ? t('sidebar.compare') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><line x1="18" y1="20" x2="18" y2="10" /><line x1="12" y1="20" x2="12" y2="4" /><line
            x1="6"
            y1="20"
            x2="6"
            y2="14"
          /></svg
        >
        {#if !collapsed}{t('sidebar.compare')}{/if}
      </button>
    </nav>
  </div>

  {#if !collapsed}
    <details class="sidebar-details" open>
      <summary class="sidebar-section-title flex-between">
        <span>{t('sidebar.projects')}</span>
        <button
          class="sidebar-add-btn"
          onclick={(e) => {
            e.stopPropagation();
            startCreate();
          }}
          title={t('sidebar.newProject')}
        >
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            ><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg
          >
        </button>
      </summary>
      <div class="sidebar-details-body">
        <div class="sidebar-search">
          <input
            class="sidebar-search-input"
            type="text"
            placeholder={t('sidebar.searchProjects')}
            value={projectSearch}
            oninput={onSearchInput}
          />
        </div>
        {#if creatingProject}
          <div class="sidebar-inline-input">
            <input
              type="text"
              bind:value={newProjectName}
              placeholder={t('sidebar.projectName')}
              use:focusOnMount
              onkeydown={(e) => {
                if (e.key === 'Enter') confirmCreate();
                if (e.key === 'Escape') cancelCreate();
              }}
              onblur={cancelCreate}
            />
          </div>
        {/if}
        <nav class="sidebar-nav">
          {#each displayedProjects as proj}
            {@const projStats = globalStats?.projects?.find((p) => p.project_id === proj.id)}
            <div class="sidebar-project">
              <button
                class="sidebar-link sidebar-project-header"
                class:active={selectedProject?.id === proj.id}
                onclick={() => onselectproject?.(proj)}
              >
                <svg
                  viewBox="0 0 24 24"
                  width="15"
                  height="15"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  ><path
                    d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"
                  /></svg
                >
                <span class="truncate sidebar-project-name">{proj.name}</span>
                {#if projStats}
                  <span class="sidebar-badge">{fmtN(projStats.total_pages)}</span>
                {/if}
              </button>
            </div>
          {/each}
        </nav>
        {#if projects.length > 30 || searchResults !== null}
          <button
            class="sidebar-link sidebar-view-all"
            class:active={currentView === 'all-projects'}
            onclick={() => onviewallprojects?.()}
          >
            {t('sidebar.viewAllProjects')} &rarr;
          </button>
        {/if}
      </div>
    </details>

    {#if sessions.filter((s) => !s.ProjectID).length > 0}
      <div class="sidebar-section">
        <div class="sidebar-section-title">{t('sidebar.unassigned')}</div>
        <nav class="sidebar-nav">
          {#each sessions.filter((s) => !s.ProjectID).slice(0, 5) as s}
            <button
              class="sidebar-link"
              class:active={selectedSession?.ID === s.ID}
              onclick={() => onselectsession?.(s)}
            >
              <svg
                viewBox="0 0 24 24"
                width="14"
                height="14"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                ><path
                  d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"
                /></svg
              >
              <span class="truncate">
                {#if s.is_running}
                  <span class="text-info"
                    >{new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}</span
                  >
                {:else}
                  {new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}
                {/if}
              </span>
            </button>
          {/each}
        </nav>
      </div>
    {/if}

    {#if systemStats}
      <details class="sidebar-details">
        <summary class="sidebar-section-title">{t('sidebar.system')}</summary>
        <div class="sidebar-details-body">
          <div class="sidebar-stats">
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.memory')}</span>
              <span class="sidebar-stat-value">{fmtSize(systemStats.mem_alloc)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.heap')}</span>
              <span class="sidebar-stat-value">{fmtSize(systemStats.mem_heap_inuse)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.sys')}</span>
              <span class="sidebar-stat-value">{fmtSize(systemStats.mem_sys)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.goroutines')}</span>
              <span class="sidebar-stat-value">{fmtN(systemStats.num_goroutines)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.gcCycles')}</span>
              <span class="sidebar-stat-value">{fmtN(systemStats.num_gc)}</span>
            </div>
          </div>
        </div>
      </details>
    {/if}
  {/if}

  <div class="sidebar-section sidebar-section-push">
    {#if !collapsed}<div class="sidebar-section-title">{t('sidebar.general')}</div>{/if}
    <nav class="sidebar-nav">
      <button
        class="sidebar-link"
        class:active={currentView === 'stats'}
        onclick={() => onopenstats?.()}
        title={collapsed ? t('sidebar.stats') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><line x1="18" y1="20" x2="18" y2="10" /><line x1="12" y1="20" x2="12" y2="4" /><line
            x1="6"
            y1="20"
            x2="6"
            y2="14"
          /></svg
        >
        {#if !collapsed}{t('sidebar.stats')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'settings'}
        onclick={() => onopensettings?.()}
        title={collapsed ? t('sidebar.settings') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><circle cx="12" cy="12" r="3" /><path
            d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
          /></svg
        >
        {#if !collapsed}{t('sidebar.settings')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'logs'}
        onclick={() => onopenlogs?.()}
        title={collapsed ? t('sidebar.logs') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><polyline points="4 17 10 11 4 5" /><line x1="12" y1="19" x2="20" y2="19" /></svg
        >
        {#if !collapsed}{t('sidebar.logs')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'api'}
        onclick={() => onopenapi?.()}
        title={collapsed ? t('sidebar.api') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><path
            d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"
          /></svg
        >
        {#if !collapsed}{t('sidebar.api')}{/if}
      </button>
    </nav>
  </div>

  <div class="sidebar-footer">
    <div class="sidebar-footer-actions">
      <button
        class="sidebar-icon-btn"
        onclick={() => ontoggledarkmode?.()}
        title={darkMode === 'auto' ? t('settings.auto') : darkMode ? t('sidebar.lightMode') : t('sidebar.darkMode')}
      >
        {#if darkMode === 'auto'}
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9" /><path d="M12 3a9 9 0 0 1 0 18z" fill="currentColor" /></svg>
        {:else if darkMode}
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            ><circle cx="12" cy="12" r="5" /><line x1="12" y1="1" x2="12" y2="3" /><line
              x1="12"
              y1="21"
              x2="12"
              y2="23"
            /><line x1="4.22" y1="4.22" x2="5.64" y2="5.64" /><line
              x1="18.36"
              y1="18.36"
              x2="19.78"
              y2="19.78"
            /><line x1="1" y1="12" x2="3" y2="12" /><line x1="21" y1="12" x2="23" y2="12" /><line
              x1="4.22"
              y1="19.78"
              x2="5.64"
              y2="18.36"
            /><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" /></svg
          >
        {:else}
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" /></svg
          >
        {/if}
      </button>
      <button
        class="sidebar-icon-btn"
        onclick={toggleCollapse}
        title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
      >
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          {#if collapsed}
            <polyline points="9 18 15 12 9 6" />
          {:else}
            <polyline points="15 18 9 12 15 6" />
          {/if}
        </svg>
      </button>
    </div>
    <a class="sidebar-branding" href="https://www.seobserver.com" target="_blank" rel="noopener">
      {#if isDark}
        <svg viewBox="0 0 224 213" width="16" height="16"
          ><g fill="#fff" fill-rule="evenodd" clip-rule="evenodd"
            ><circle cx="9.2" cy="37.5" r="9.2" /><circle cx="13.2" cy="11.5" r="8.2" /><circle
              cx="40.8"
              cy="50.9"
              r="15.8"
            /><path
              d="M219.7 199.8l-42.1-42C190.9 141.1 199 120 199 97c0-53.6-43.4-97-97-97C76.8 0 53.9 9.6 36.6 25.3c1.3-.2 2.7-.3 4.1-.3 6.5 0 12.5 2.5 17 6.6C70.3 23 85.5 18 102 18c43.6 0 79 35.4 79 79s-35.4 79-79 79-79-35.4-79-79c0-9 1.5-17.7 4.3-25.8-6.4-5-10.8-11.8-11.5-19.7C8.9 65.8 5 81 5 97c0 53.6 43.4 97 97 97 19.9 0 38.4-6 53.8-16.3l43 43c3.9 3.9 11.5 2.6 17-2.9l1-1c5.5-5.5 6.7-13.1 2.9-17z"
            /><path d="M56 124l34-28 4 5-35 28-3-5z" /><path
              d="M106 99l24 28-5 3-22-27 3-4z"
            /><path d="M96 82l12-39 4 2-11 39-5-2z" /><path
              d="M156 85l-20 44 4 2 20-45-4-1z"
            /><path d="M153 72l-36-22-2 4 35 22 3-4z" /><path
              d="M90 83L56 58l-3 4 33 24 4-3z"
            /><path d="M106 90l44-10v5l-44 10v-5z" /><path
              d="M87.7 79l18.7.4c2.9.1 5.1 2.5 5.1 5.3l-.5 18.7c0 2.8-2.4 5.1-5.3 5l-18.7-.4c-2.8-.1-5.1-2.4-5-5.3l.4-18.6c.1-2.9 2.4-5.1 5.3-5.1z"
            /><circle cx="53.5" cy="129.8" r="12.5" /><circle
              cx="110.5"
              cy="45.8"
              r="12.5"
            /><circle cx="155.8" cy="80.5" r="12.5" /><circle cx="131.8" cy="132.5" r="12.5" /></g
          ></svg
        >
      {:else}
        <svg viewBox="0 0 224 213" width="16" height="16"
          ><ellipse fill="#fff" cx="97.667" cy="91.346" rx="91.333" ry="89.346" /><circle
            fill="#FF8F00"
            cx="9.167"
            cy="37.5"
            r="9.167"
          /><circle fill="#FF8F00" cx="13.167" cy="11.5" r="8.167" /><circle
            fill="#FF8F00"
            cx="40.75"
            cy="50.916"
            r="15.75"
          /><path
            fill="#FFA300"
            d="M102,23c-15.7,0-30.248,4.903-42.224,13.242C63.2,40.296,65.25,45.421,65.25,51c0,13.117-11.305,23.75-25.25,23.75c-2.856,0-5.599-.453-8.159-1.275C29.363,80.868,28,88.772,28,97c0,40.869,33.131,74,74,74s74-33.131,74-74S142.869,23,102,23z"
          /><path fill="#fff" d="M56,124l34-28l4,5l-35,28L56,124z" /><path
            fill="#fff"
            d="M106,99l24,28l-5,3l-22-27L106,99z"
          /><path fill="#fff" d="M96,82l12-39l4,2l-11,39L96,82z" /><path
            fill="#fff"
            d="M156,85l-20,44l4,2l20-45L156,85z"
          /><path fill="#fff" d="M153,72l-36-22l-2,4l35,22L153,72z" /><path
            fill="#fff"
            d="M90,83L56,58l-3,4l33,24L90,83z"
          /><path fill="#fff" d="M106,90l44-10v5l-44,10V90z" /><path
            fill="#fff"
            d="M87.72,79.005l18.681.433c2.866.066,5.135,2.438,5.067,5.298l-.438,18.64c-.066,2.859-2.444,5.124-5.311,5.058l-18.681-.434c-2.866-.066-5.135-2.438-5.067-5.298l.438-18.64C82.476,81.203,84.854,78.939,87.72,79.005z"
          /><circle fill="#fff" cx="53.5" cy="129.834" r="12.5" /><circle
            fill="#fff"
            cx="110.5"
            cy="45.834"
            r="12.5"
          /><circle fill="#fff" cx="155.833" cy="80.5" r="12.5" /><circle
            fill="#fff"
            cx="131.833"
            cy="132.501"
            r="12.5"
          /><path
            fill="#3D3D3D"
            d="M219.674,199.824l-42.065-42.065C190.988,141.132,199,120.003,199,97c0-53.572-43.429-97-97-97C76.821,0,53.884,9.595,36.642,25.327c1.311-.212,2.654-.327,4.025-.327,6.546,0,12.502,2.519,16.959,6.637C70.275,23.033,85.548,18,102,18c43.631,0,79,35.37,79,79c0,43.631-35.369,79-79,79s-79-35.369-79-79c0-9.056,1.543-17.746,4.349-25.847C20.996,67.145,16.572,60.36,15.792,52.5C8.897,65.828,5,80.958,5,97c0,53.572,43.429,97,97,97c19.921,0,38.438-6.009,53.842-16.309l42.982,42.982c3.905,3.904,11.516,2.625,16.999-2.857l.993-.993C222.299,211.34,223.578,203.729,219.674,199.824z"
          /></svg
        >
      {/if}
      {#if !collapsed}{t('sidebar.byBrand')}{/if}
    </a>
    {#if appVersion && !collapsed}
      <span class="sidebar-version">v{appVersion}</span>
    {/if}
  </div>
</aside>

<style>
  .sidebar {
    width: var(--sidebar-width);
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    position: fixed;
    top: var(--topbar-height);
    left: 0;
    bottom: 0;
    z-index: 100;
    overflow-y: auto;
    overflow-x: hidden;
    transition: width 0.2s ease;
  }
  .sidebar-header {
    padding: 20px 20px 16px;
    display: flex;
    align-items: center;
    gap: 12px;
    border-bottom: 1px solid var(--border-light);
  }
  .sidebar-logo {
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    object-fit: contain;
  }
  .sidebar-logo-placeholder {
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--accent-text);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 16px;
    flex-shrink: 0;
  }
  svg.sidebar-logo {
    flex-shrink: 0;
  }
  .sidebar-app-name {
    font-weight: 700;
    font-size: 17px;
    color: var(--text);
    letter-spacing: -0.02em;
  }
  .sidebar-section {
    padding: 16px 12px 8px;
  }
  .sidebar-section-title {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
    padding: 0 8px 8px;
  }
  .sidebar-add-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 2px 4px;
    border-radius: 4px;
    display: flex;
    align-items: center;
    transition:
      color 0.15s,
      background 0.15s;
  }
  .sidebar-add-btn:hover {
    color: var(--accent);
    background: var(--bg-hover);
  }
  .sidebar-search {
    padding: 0 8px 6px;
  }
  .sidebar-search-input {
    width: 100%;
    padding: 5px 8px;
    font-size: 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    outline: none;
    box-sizing: border-box;
  }
  .sidebar-search-input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 20%, transparent);
  }
  .sidebar-search-input::placeholder {
    color: var(--text-muted);
  }
  .sidebar-inline-input {
    padding: 0 8px 6px;
  }
  .sidebar-inline-input input {
    width: 100%;
    padding: 5px 8px;
    font-size: 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    outline: none;
  }
  .sidebar-inline-input input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 20%, transparent);
  }
  .sidebar-nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .sidebar-link {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
    border: none;
    background: none;
    width: 100%;
    text-align: left;
  }
  .sidebar-link:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .sidebar-link.active {
    background: var(--accent-light);
    color: var(--accent);
    font-weight: 600;
  }
  .sidebar-link svg {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    opacity: 0.7;
  }
  .sidebar-link.active svg {
    opacity: 1;
  }
  .sidebar-project {
    margin-bottom: 2px;
  }
  .sidebar-badge {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    background: var(--bg-hover);
    padding: 1px 6px;
    border-radius: 8px;
    flex-shrink: 0;
  }
  .sidebar-link.sidebar-project-header {
    font-weight: 600;
    font-size: 13px;
  }
  .sidebar-link.sidebar-view-all {
    font-size: 12px;
    color: var(--accent);
    padding: 6px 12px;
    justify-content: center;
  }
  .sidebar-stats {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 0 8px;
  }
  .sidebar-stat {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 3px 6px;
    border-radius: var(--radius-sm);
    font-size: 12px;
  }
  .sidebar-stat-label {
    color: var(--text-muted);
    font-weight: 500;
  }
  .sidebar-stat-value {
    color: var(--text-secondary);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .sidebar-footer {
    margin-top: auto;
    padding: 16px 12px;
    border-top: 1px solid var(--border-light);
  }
  .sidebar-branding {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    font-size: 11px;
    color: var(--text-muted);
    text-decoration: none;
    opacity: 0.6;
    transition: opacity 0.15s;
  }
  .sidebar-branding:hover {
    opacity: 1;
  }
  .sidebar-version {
    font-size: 10px;
    color: var(--text-muted);
    opacity: 0.5;
    padding: 4px 12px;
  }
  /* Sidebar collapsible details */
  .sidebar-details {
    padding: 0 12px;
    margin-top: 8px;
  }
  .sidebar-details > summary {
    cursor: pointer;
    user-select: none;
    list-style: none;
  }
  .sidebar-details > summary::-webkit-details-marker {
    display: none;
  }
  .sidebar-details-body {
    padding-top: 4px;
  }
  .sidebar-project-name {
    flex: 1;
  }
  .sidebar-section-push {
    margin-top: auto;
  }
  /* Footer actions row */
  .sidebar-footer-actions {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    margin-bottom: 8px;
  }
  .sidebar-icon-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 7px;
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    transition:
      color 0.15s,
      background 0.15s;
  }
  .sidebar-icon-btn:hover {
    color: var(--text);
    background: var(--bg-hover);
  }

  /* Collapsed state */
  .sidebar.collapsed .sidebar-header {
    padding: 16px 10px;
    justify-content: center;
  }
  .sidebar.collapsed .sidebar-logo {
    width: 28px;
    height: 28px;
  }
  .sidebar.collapsed .sidebar-section {
    padding: 8px 6px 4px;
  }
  .sidebar.collapsed .sidebar-link {
    justify-content: center;
    padding: 9px 0;
  }
  .sidebar.collapsed .sidebar-footer {
    padding: 12px 6px;
  }
  .sidebar.collapsed .sidebar-branding {
    justify-content: center;
    padding: 6px;
  }
  .sidebar.collapsed .sidebar-footer-actions {
    flex-direction: column;
  }

  @media (max-width: 768px) {
    .sidebar {
      display: none;
    }
  }
</style>
