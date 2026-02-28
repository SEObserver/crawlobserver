<script>
  import { fmtN, fmtSize } from '../utils.js';

  let {
    theme, darkMode, sessions, projects, globalStats, systemStats,
    selectedSession, selectedProject, showNewCrawl, showSettings, showGlobalStats, showAPI, showCompare, showLogs,
    liveProgress,
    ontoggledarkmmode, onselectsession, onselectproject, onnavigate, onopensettings,
    onopenstats, onopenapi, onopenlogs, ongohome
  } = $props();
</script>

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
      <button class="sidebar-link" class:active={!selectedSession && !showNewCrawl && !showSettings && !showGlobalStats && !showAPI} onclick={() => ongohome?.()}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
        Dashboard
      </button>
      <button class="sidebar-link" class:active={showNewCrawl && !selectedSession} onclick={() => onnavigate?.('/new-crawl')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="8" y1="12" x2="16" y2="12"/></svg>
        New Crawl
      </button>
      <button class="sidebar-link" class:active={showCompare} onclick={() => onnavigate?.('/compare')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
        Compare
      </button>
    </nav>
  </div>

  {#if projects.length > 0}
    <div class="sidebar-section">
      <div class="sidebar-section-title">Projects</div>
      <nav class="sidebar-nav">
        {#each projects as proj}
          {@const projStats = globalStats?.projects?.find(p => p.project_id === proj.id)}
          <div class="sidebar-project">
            <button class="sidebar-link sidebar-project-header" class:active={selectedProject?.id === proj.id} onclick={() => onselectproject?.(proj)}>
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
              <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1;">{proj.name}</span>
              {#if projStats}
                <span class="sidebar-badge">{fmtN(projStats.total_pages)}</span>
              {/if}
            </button>
          </div>
        {/each}
      </nav>
    </div>
  {/if}

  {#if sessions.filter(s => !s.ProjectID).length > 0}
    <div class="sidebar-section">
      <div class="sidebar-section-title">Unassigned Sessions</div>
      <nav class="sidebar-nav">
        {#each sessions.filter(s => !s.ProjectID).slice(0, 5) as s}
          <button class="sidebar-link" class:active={selectedSession?.ID === s.ID} onclick={() => onselectsession?.(s)}>
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
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

  {#if systemStats}
    <div class="sidebar-section">
      <div class="sidebar-section-title">System</div>
      <div class="sidebar-stats">
        <div class="sidebar-stat">
          <span class="sidebar-stat-label">Memory</span>
          <span class="sidebar-stat-value">{fmtSize(systemStats.mem_alloc)}</span>
        </div>
        <div class="sidebar-stat">
          <span class="sidebar-stat-label">Heap</span>
          <span class="sidebar-stat-value">{fmtSize(systemStats.mem_heap_inuse)}</span>
        </div>
        <div class="sidebar-stat">
          <span class="sidebar-stat-label">Sys</span>
          <span class="sidebar-stat-value">{fmtSize(systemStats.mem_sys)}</span>
        </div>
        <div class="sidebar-stat">
          <span class="sidebar-stat-label">Goroutines</span>
          <span class="sidebar-stat-value">{fmtN(systemStats.num_goroutines)}</span>
        </div>
        <div class="sidebar-stat">
          <span class="sidebar-stat-label">GC cycles</span>
          <span class="sidebar-stat-value">{fmtN(systemStats.num_gc)}</span>
        </div>
      </div>
    </div>
  {/if}

  <div class="sidebar-section" style="margin-top: auto;">
    <div class="sidebar-section-title">General</div>
    <nav class="sidebar-nav">
      <button class="sidebar-link" class:active={showGlobalStats} onclick={() => onopenstats?.()}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
        Stats
      </button>
      <button class="sidebar-link" class:active={showSettings} onclick={() => onopensettings?.()}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
        Settings
      </button>
      <button class="sidebar-link" class:active={showLogs} onclick={() => onopenlogs?.()}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
        Logs
      </button>
      <button class="sidebar-link" class:active={showAPI} onclick={() => onopenapi?.()}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>
        API
      </button>
    </nav>
  </div>

  <div class="sidebar-footer">
    <button class="theme-toggle" onclick={() => ontoggledarkmmode?.()}>
      {#if darkMode}
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
        Light mode
      {:else}
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
        Dark mode
      {/if}
    </button>
    <a class="sidebar-branding" href="https://www.seobserver.com" target="_blank" rel="noopener">
      <svg viewBox="-2 -2 228 228" width="16" height="16"><circle fill="#FF8F00" cx="9.167" cy="37.5" r="9.167"/><circle fill="#FF8F00" cx="13.167" cy="11.5" r="8.167"/><circle fill="#FF8F00" cx="40.75" cy="50.916" r="15.75"/><ellipse fill="#FFA300" cx="102" cy="97" rx="74" ry="74"/><path fill="#3D3D3D" d="M219.674,199.824l-42.065-42.065C190.988,141.132,199,120.003,199,97c0-53.572-43.429-97-97-97C76.821,0,53.884,9.595,36.642,25.327C37.953,25.115,39.296,25,40.667,25c6.546,0,12.502,2.519,16.959,6.637C70.275,23.033,85.548,18,102,18c43.631,0,79,35.37,79,79c0,43.631-35.369,79-79,79s-79-35.369-79-79c0-9.056,1.543-17.746,4.349-25.847C20.996,67.145,16.572,60.36,15.792,52.5C8.897,65.828,5,80.958,5,97c0,53.572,43.429,97,97,97c19.921,0,38.438-6.009,53.842-16.309l42.982,42.982c3.905,3.904,11.516,2.625,16.999-2.857l0.993-0.993C222.299,211.34,223.578,203.729,219.674,199.824z"/></svg>
      by SEObserver
    </a>
  </div>
</aside>
