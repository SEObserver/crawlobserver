<script>
  import { onMount } from 'svelte';
  import {
    getSessionsPaginated,
    renameProject,
    deleteProject,
    deleteProjectWithSessions,
  } from '../api.js';
  import { fmtN, timeAgo } from '../utils.js';
  import { pushURL } from '../router.js';
  import { t } from '../i18n/index.svelte.js';
  import GSCTab from './GSCTab.svelte';
  import ProvidersTab from './ProvidersTab.svelte';
  import ConfirmModal from './ConfirmModal.svelte';

  const PROJ_SESSIONS_LIMIT = 30;

  /** @param {HTMLElement} node */
  function focusOnMount(node) {
    node.focus();
  }

  let {
    project,
    initialProjectTab = 'sessions',
    initialGscSubView = 'overview',
    initialProviderSubView = 'overview',
    onerror,
    onselectsession,
    ongohome,
    onnewcrawl,
    onprojectrenamed,
    onprojectdeleted,
    onpushurl,
  } = $props();

  // --- Local state ---
  let projectTab = $state(initialProjectTab);
  let projSessions = $state([]);
  let projSessionsTotal = $state(0);
  let projSessionsOffset = $state(0);
  let renamingProject = $state(false);
  let renameValue = $state('');
  let gscSubView = $state(initialGscSubView);
  let providerSubView = $state(initialProviderSubView);
  let confirmState = $state(null);

  function showConfirm(message, onConfirm, opts = {}) {
    confirmState = { message, onConfirm, ...opts };
  }

  // --- Data loading ---
  async function loadProjectSessions() {
    if (!project) return;
    try {
      const res = await getSessionsPaginated(PROJ_SESSIONS_LIMIT, projSessionsOffset, {
        projectId: project.id,
      });
      projSessions = res.sessions || [];
      projSessionsTotal = res.total || 0;
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function switchProjectTab(tab) {
    projectTab = tab;
    if (project) pushURL(`/projects/${project.id}/${tab}`);
  }

  // --- Rename ---
  function startRenameProject() {
    renamingProject = true;
    renameValue = project?.name || '';
  }

  async function confirmRenameProject() {
    const name = renameValue.trim();
    if (name && name !== project?.name) {
      try {
        await renameProject(project.id, name);
        onprojectrenamed?.(project.id);
      } catch (e) {
        onerror?.(e.message);
      }
    }
    renamingProject = false;
  }

  function cancelRenameProject() {
    renamingProject = false;
  }

  // --- Delete ---
  function handleDeleteProject() {
    showConfirm(
      t('project.deleteProject') + ` "${project?.name}"?`,
      async () => {
        try {
          await deleteProject(project.id);
          onprojectdeleted?.();
        } catch (e) {
          onerror?.(e.message);
        }
      },
      { danger: true, confirmLabel: t('common.delete') },
    );
  }

  function handleDeleteProjectWithSessions() {
    showConfirm(
      t('project.deleteProjectWithSessions') + ` "${project?.name}"?`,
      async () => {
        try {
          await deleteProjectWithSessions(project.id);
          onprojectdeleted?.();
        } catch (e) {
          onerror?.(e.message);
        }
      },
      { danger: true, confirmLabel: t('common.delete') },
    );
  }

  // --- Mount ---
  onMount(() => {
    loadProjectSessions();
  });
</script>

<div class="breadcrumb">
  <a
    href="/"
    onclick={(e) => {
      e.preventDefault();
      ongohome?.();
    }}>{t('project.dashboard')}</a
  >
  <span>/</span>
  {#if renamingProject}
    <input
      class="project-rename-input"
      type="text"
      bind:value={renameValue}
      use:focusOnMount
      onkeydown={(e) => {
        if (e.key === 'Enter') confirmRenameProject();
        if (e.key === 'Escape') cancelRenameProject();
      }}
      onblur={confirmRenameProject}
    />
  {:else}
    <button
      class="inline-btn breadcrumb-active"
      ondblclick={startRenameProject}
      title={t('project.doubleClickRename')}>{project.name}</button
    >
  {/if}
  <button class="btn btn-primary btn-sm project-new-crawl" onclick={() => onnewcrawl?.()}>
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

<div class="tab-bar">
  <button
    class="tab"
    class:tab-active={projectTab === 'sessions'}
    onclick={() => switchProjectTab('sessions')}>{t('project.sessions')}</button
  >
  <button
    class="tab"
    class:tab-active={projectTab === 'gsc'}
    onclick={() => switchProjectTab('gsc')}>{t('project.searchConsole')}</button
  >
  <button
    class="tab"
    class:tab-active={projectTab === 'providers'}
    onclick={() => switchProjectTab('providers')}>{t('project.seoData')}</button
  >
</div>

<div class="card card-flush card-tab-body">
  {#if projectTab === 'sessions'}
    {#if projSessions.length > 0}
      <table>
        <thead>
          <tr>
            <th>{t('project.seedUrl')}</th>
            <th>{t('common.status')}</th>
            <th>{t('common.pages')}</th>
            <th>{t('actionBar.started')}</th>
          </tr>
        </thead>
        <tbody>
          {#each projSessions as s}
            <tr class="clickable-row" onclick={() => onselectsession?.(s)}>
              <td class="cell-url">{s.SeedURLs?.[0] || s.ID}</td>
              <td>
                {#if s.is_running}
                  <span class="badge badge-info">{t('common.running')}</span>
                {:else if s.Status === 'completed'}
                  <span class="badge badge-success">{t('common.completed')}</span>
                {:else if s.Status === 'failed' || s.Status === 'crashed'}
                  <span class="badge badge-error">{s.Status}</span>
                {:else}
                  <span class="badge">{s.Status || t('common.unknown')}</span>
                {/if}
              </td>
              <td>{fmtN(s.PagesCrawled || 0)}</td>
              <td class="nowrap text-muted text-sm">{s.StartedAt ? timeAgo(s.StartedAt) : '-'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      {#if projSessionsTotal > PROJ_SESSIONS_LIMIT}
        <div class="pagination-controls">
          <button
            class="btn btn-sm"
            onclick={() => {
              projSessionsOffset = Math.max(0, projSessionsOffset - PROJ_SESSIONS_LIMIT);
              loadProjectSessions();
            }}
            disabled={projSessionsOffset === 0}>{t('common.previous')}</button
          >
          <span class="text-sm text-muted"
            >{projSessionsOffset + 1}-{Math.min(
              projSessionsOffset + PROJ_SESSIONS_LIMIT,
              projSessionsTotal,
            )}
            {t('common.of')}
            {projSessionsTotal}</span
          >
          <button
            class="btn btn-sm"
            onclick={() => {
              projSessionsOffset += PROJ_SESSIONS_LIMIT;
              loadProjectSessions();
            }}
            disabled={projSessionsOffset + PROJ_SESSIONS_LIMIT >= projSessionsTotal}
            >{t('common.next')}</button
          >
        </div>
      {/if}
    {:else}
      <div class="empty-state">
        <p>{t('project.noSessions')}</p>
        <button class="btn btn-primary mt-md" onclick={() => onnewcrawl?.()}>
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
    {/if}
  {:else if projectTab === 'gsc'}
    <GSCTab
      projectId={project.id}
      initialSubView={gscSubView}
      onerror={(msg) => onerror?.(msg)}
      onpushurl={(u) => onpushurl?.(u)}
    />
  {:else if projectTab === 'providers'}
    <ProvidersTab
      projectId={project.id}
      initialSubView={providerSubView}
      onerror={(msg) => onerror?.(msg)}
      onpushurl={(u) => onpushurl?.(u)}
    />
  {/if}
</div>

{#if projectTab === 'sessions'}
  <details class="danger-zone">
    <summary>{t('project.dangerZone')}</summary>
    <div class="danger-zone-item">
      <div class="danger-zone-text">
        <strong>{t('project.deleteProject')}</strong>
        <p>{t('project.deleteProjectDesc')}</p>
      </div>
      <button class="btn btn-danger" onclick={handleDeleteProject}>
        <svg
          viewBox="0 0 24 24"
          width="14"
          height="14"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><polyline points="3 6 5 6 21 6" /><path
            d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
          /></svg
        >
        {t('project.deleteProject')}
      </button>
    </div>
    <div class="danger-zone-item">
      <div class="danger-zone-text">
        <strong>{t('project.deleteProjectWithSessions')}</strong>
        <p>{t('project.deleteProjectWithSessionsDesc')}</p>
      </div>
      <button class="btn btn-danger" onclick={handleDeleteProjectWithSessions}>
        <svg
          viewBox="0 0 24 24"
          width="14"
          height="14"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><polyline points="3 6 5 6 21 6" /><path
            d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
          /></svg
        >
        {t('project.deleteProjectWithSessions')}
      </button>
    </div>
  </details>
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

<style>
  .breadcrumb-active {
    color: var(--text);
  }
  .project-new-crawl {
    margin-left: auto;
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
  .danger-zone {
    margin-top: 32px;
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .danger-zone summary {
    padding: 12px 16px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-muted);
    cursor: pointer;
    list-style: none;
  }
  .danger-zone summary::-webkit-details-marker {
    display: none;
  }
  .danger-zone[open] summary {
    color: #dc2626;
    border-bottom: 1px solid var(--border);
  }
  .danger-zone-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 16px;
  }
  .danger-zone-item + .danger-zone-item {
    border-top: 1px solid var(--border);
  }
  .danger-zone-text p {
    margin: 4px 0 0;
    font-size: 13px;
    color: var(--text-muted);
  }
  .danger-zone-text strong {
    font-size: 13px;
  }
  .btn-danger {
    background: var(--bg-card);
    color: #dc2626;
    border: 1px solid var(--border);
    padding: 6px 14px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
    flex-shrink: 0;
  }
  .btn-danger:hover {
    background: #dc2626;
    color: #fff;
    border-color: #dc2626;
  }
</style>
