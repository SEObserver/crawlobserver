<script>
  import { onMount } from 'svelte';
  import { getSessionsPaginated, renameProject, deleteProject } from '../api.js';
  import { fmtN, timeAgo } from '../utils.js';
  import { pushURL } from '../router.js';
  import GSCTab from './GSCTab.svelte';
  import ProvidersTab from './ProvidersTab.svelte';
  import ConfirmModal from './ConfirmModal.svelte';

  const PROJ_SESSIONS_LIMIT = 30;

  /** @param {HTMLElement} node */
  function focusOnMount(node) { node.focus(); }

  let {
    project,
    initialProjectTab = 'sessions',
    initialGscSubView = 'overview', initialProviderSubView = 'overview',
    onerror, onselectsession, ongohome,
    onprojectrenamed, onprojectdeleted, onpushurl,
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
      const res = await getSessionsPaginated(PROJ_SESSIONS_LIMIT, projSessionsOffset, { projectId: project.id });
      projSessions = res.sessions || [];
      projSessionsTotal = res.total || 0;
    } catch (e) { onerror?.(e.message); }
  }

  function switchProjectTab(t) {
    projectTab = t;
    if (project) pushURL(`/projects/${project.id}/${t}`);
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
      } catch (e) { onerror?.(e.message); }
    }
    renamingProject = false;
  }

  function cancelRenameProject() {
    renamingProject = false;
  }

  // --- Delete ---
  function handleDeleteProject() {
    showConfirm(`Delete project "${project?.name}"? Sessions will be unassigned.`, async () => {
      try {
        await deleteProject(project.id);
        onprojectdeleted?.();
      } catch (e) { onerror?.(e.message); }
    }, { danger: true, confirmLabel: 'Delete' });
  }

  // --- Mount ---
  onMount(() => {
    loadProjectSessions();
  });
</script>

<div class="breadcrumb">
  <a href="/" onclick={(e) => { e.preventDefault(); ongohome?.(); }}>Dashboard</a>
  <span>/</span>
  {#if renamingProject}
    <input class="project-rename-input" type="text" bind:value={renameValue}
      use:focusOnMount
      onkeydown={(e) => { if (e.key === 'Enter') confirmRenameProject(); if (e.key === 'Escape') cancelRenameProject(); }}
      onblur={confirmRenameProject} />
  {:else}
    <button class="inline-btn breadcrumb-active" ondblclick={startRenameProject} title="Double-click to rename">{project.name}</button>
  {/if}
  <button class="project-delete-btn" onclick={() => handleDeleteProject()} title="Delete project" aria-label="Delete project">
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
                <a href={`/sessions/${s.ID}/overview`} onclick={(e) => { e.preventDefault(); onselectsession?.(s); }}>
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
                <button class="btn btn-sm" onclick={() => onselectsession?.(s)}>View</button>
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
    <GSCTab projectId={project.id} initialSubView={gscSubView} onerror={(msg) => onerror?.(msg)} onpushurl={(u) => onpushurl?.(u)} />
  {:else if projectTab === 'providers'}
    <ProvidersTab projectId={project.id} initialSubView={providerSubView} onerror={(msg) => onerror?.(msg)} onpushurl={(u) => onpushurl?.(u)} />
  {/if}
</div>

{#if confirmState}
  <ConfirmModal
    message={confirmState.message}
    danger={confirmState.danger}
    confirmLabel={confirmState.confirmLabel}
    onconfirm={() => { confirmState.onConfirm(); confirmState = null; }}
    oncancel={() => confirmState = null}
  />
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
</style>
