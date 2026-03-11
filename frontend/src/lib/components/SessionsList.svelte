<script>
  import { t } from '../i18n/index.svelte.js';
  import { fmtN, fmtSize, timeAgo } from '../utils.js';
  import { importSession, importCSVSession, batchAssignSessions } from '../api.js';

  let {
    sessions,
    projects,
    liveProgress,
    sessionStorageMap,
    loading,
    onselectsession,
    onstop,
    onresume,
    ondelete,
    onnewcrawl,
    onrefresh,
  } = $props();

  let importing = $state(false);
  let importError = $state('');
  let selectedIds = $state(new Set());
  let showBulkAssignMenu = $state(false);

  function toggleSelect(id, e) {
    e.stopPropagation();
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selectedIds = next;
  }

  function toggleSelectAll(e) {
    e.stopPropagation();
    if (selectedIds.size === sessions.length) {
      selectedIds = new Set();
    } else {
      selectedIds = new Set(sessions.map((s) => s.ID));
    }
  }

  async function handleBulkAssign(projectId) {
    showBulkAssignMenu = false;
    try {
      await batchAssignSessions(projectId, [...selectedIds]);
      selectedIds = new Set();
      onrefresh?.();
    } catch (err) {
      importError = err.message;
    }
  }

  async function handleImport(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    importing = true;
    importError = '';
    try {
      if (file.name.endsWith('.csv')) {
        await importCSVSession(file);
      } else {
        await importSession(file);
      }
      onrefresh?.();
    } catch (err) {
      importError = err.message;
    } finally {
      importing = false;
      e.target.value = '';
    }
  }
</script>

<div class="page-header">
  <h1>{t('sessions.title')}</h1>
  <div class="flex-center-gap">
    <label class="btn btn-sm import-label">
      <svg
        viewBox="0 0 24 24"
        width="14"
        height="14"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        ><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
          points="17 8 12 3 7 8"
        /><line x1="12" y1="3" x2="12" y2="15" /></svg
      >
      {importing ? t('common.importing') : t('common.import')}
      <input
        type="file"
        accept=".gz,.jsonl.gz,.csv"
        onchange={handleImport}
        disabled={importing}
        class="sr-only-input"
      />
    </label>
    <button class="btn btn-primary" onclick={() => onnewcrawl?.()}>
      <svg
        viewBox="0 0 24 24"
        width="16"
        height="16"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        ><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg
      >
      {t('sessions.newCrawl')}
    </button>
  </div>
</div>

{#if importError}
  <div class="alert alert-error mb-sm">{importError}</div>
{/if}

{#if loading}
  <p class="loading-msg">{t('common.loading')}</p>
{:else if sessions.length === 0}
  <div class="empty-state">
    <h2>{t('sessions.noSessions')}</h2>
    <p>{t('sessions.noSessionsDesc')}</p>
    <button class="btn btn-primary mt-md" onclick={() => onnewcrawl?.()}
      >{t('sessions.startCrawl')}</button
    >
  </div>
{:else}
  <!-- Bulk action bar -->
  {#if selectedIds.size > 0}
    <div class="bulk-bar">
      <span class="bulk-count">{t('session.bulkSelected', { count: selectedIds.size })}</span>
      <div class="bulk-assign-wrapper">
        <button class="btn btn-sm btn-primary" onclick={() => (showBulkAssignMenu = !showBulkAssignMenu)}>
          {t('session.bulkAssign')}
          <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9" /></svg>
        </button>
        {#if showBulkAssignMenu}
          <div class="bulk-assign-menu">
            {#each projects as p}
              <button class="dropdown-item" onclick={() => handleBulkAssign(p.id)}>{p.name}</button>
            {/each}
          </div>
        {/if}
      </div>
      <button class="btn btn-sm" onclick={() => (selectedIds = new Set())}>{t('common.cancel')}</button>
    </div>
  {/if}

  <div class="card card-flush">
    {#each sessions as s}
      {@const live = liveProgress[s.ID]}
      {@const isQueued = live ? live.is_queued : s.is_queued}
      {@const isRunning = live ? live.is_running : s.is_running}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="session-row" onclick={() => onselectsession?.(s)}>
        <label class="session-checkbox" onclick={(e) => e.stopPropagation()}>
          <input type="checkbox" checked={selectedIds.has(s.ID)} onchange={(e) => toggleSelect(s.ID, e)} />
        </label>
        <div class="session-info">
          <div class="session-seed">
            {s.SeedURLs?.[0] || 'Unknown'}
            {#if s.Label}
              <span class="session-label">{s.Label}</span>
            {/if}
          </div>
          <div class="session-meta">
            {#if isQueued}
              <span class="badge badge-queued">{t('session.queued')}</span>
            {:else if s.Status === 'stopping'}
              <span class="badge badge-warning">{t('actionBar.stopping')}</span>
            {:else if isRunning}
              <span class="badge badge-info">
                {#if live && live.phase === 'fetching_sitemaps'}
                  {t('common.fetchingSitemaps')}
                {:else if live && live.queue_size === 0 && live.pages_crawled > 0}
                  {t('common.finalizing')}
                  &middot; {fmtN(live.pages_crawled)}
                  {t('common.pages')}
                {:else}
                  {t('common.running')}
                  {#if live}
                    &middot; {fmtN(live.pages_crawled)}
                    {t('common.pages')} &middot; {fmtN(live.queue_size)}
                    {t('sessions.queued')}
                    {#if live.lost_pages > 0}
                      <span class="text-error font-semibold"
                        >&middot; {fmtN(live.lost_pages)} {t('sessions.lost')}</span
                      >
                    {/if}
                  {/if}
                {/if}
              </span>
            {:else}
              <span
                class="badge"
                class:badge-success={s.Status === 'completed'}
                class:badge-error={s.Status === 'failed' || s.Status === 'crashed'}
                class:badge-warning={s.Status === 'stopped' || s.Status === 'completed_with_errors'}
                >{s.Status}</span
              >
            {/if}
            {#if s.ProjectID}
              <span class="badge badge-project"
                >{projects.find((p) => p.id === s.ProjectID)?.name || 'Project'}</span
              >
            {:else}
              <span class="badge badge-orphan">{t('session.noProject')}</span>
            {/if}
            <span>{t('sessions.pagesCount', { count: fmtN(s.PagesCrawled) })}</span>
            {#if sessionStorageMap[s.ID]}<span>{fmtSize(sessionStorageMap[s.ID])}</span>{/if}
            <span>{timeAgo(s.StartedAt)}</span>
          </div>
        </div>
        <div class="session-actions" onclick={(e) => e.stopPropagation()}>
          {#if s.Status === 'stopping'}
            <!-- no actions while stopping -->
          {:else if isRunning || isQueued}
            <button class="btn btn-sm btn-danger" onclick={() => onstop?.(s.ID)}
              >{t('common.stop')}</button
            >
          {:else}
            <button class="btn btn-sm" onclick={() => onresume?.(s.ID)}
              >{t('sessions.resume')}</button
            >
            <button
              class="btn-ghost btn-delete-icon"
              onclick={() => ondelete?.(s.ID)}
              title={t('common.delete')}
            >
              <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                class="icon-trash"
                ><polyline points="3 6 5 6 21 6" /><path
                  d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
                /></svg
              >
            </button>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  /* Bulk bar */
  .bulk-bar {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 16px;
    background: var(--accent-light, #eff6ff);
    border: 1px solid var(--accent, #3b82f6);
    border-radius: var(--radius);
    margin-bottom: 8px;
  }
  .bulk-count {
    font-size: 13px;
    font-weight: 500;
    color: var(--accent);
  }
  .bulk-assign-wrapper {
    position: relative;
  }
  .bulk-assign-menu {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 100;
    min-width: 180px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
    padding: 4px 0;
  }
  .dropdown-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 12px;
    font-size: 13px;
    color: var(--text-primary);
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
  }
  .dropdown-item:hover {
    background: var(--bg-hover);
  }

  .session-checkbox {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    cursor: pointer;
  }
  .session-checkbox input {
    cursor: pointer;
  }
  .session-label {
    font-size: 12px;
    font-weight: 400;
    color: var(--text-muted);
    margin-left: 8px;
    padding: 1px 6px;
    background: var(--bg-hover);
    border-radius: var(--radius-sm);
  }
  .badge-orphan {
    background: #fff7ed;
    color: #c2410c;
  }

  .session-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border-light);
    transition: background 0.1s;
    gap: 16px;
  }
  .session-row:last-child {
    border-bottom: none;
  }
  .session-row:hover {
    background: var(--bg-hover);
    cursor: pointer;
  }
  .session-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
    flex: 1;
  }
  .session-seed {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .session-meta {
    font-size: 12px;
    color: var(--text-muted);
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .session-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }
  .import-label {
    cursor: pointer;
    position: relative;
  }
  .sr-only-input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }
  .badge-queued {
    background: #fef3c7;
    color: #92400e;
  }
  .badge-project {
    background: var(--accent-light);
    color: var(--accent);
  }
  .btn-delete-icon {
    padding: 4px;
    color: var(--text-muted);
  }
  .btn-delete-icon:hover {
    color: var(--error);
  }
  .icon-trash {
    width: 16px;
    height: 16px;
  }
</style>
