<script>
  import { getProjects, createProject, renameProject, deleteProject, getAPIKeys, createAPIKey, deleteAPIKey, getServerInfo } from '../api.js';
  import { timeAgo, copyToClipboard } from '../utils.js';
  import ConfirmModal from './ConfirmModal.svelte';

  let { onerror, onprojectschanged } = $props();

  let confirmState = $state(null);

  function showConfirm(message, onConfirm, opts = {}) {
    confirmState = { message, onConfirm, ...opts };
  }

  let serverInfo = $state(null);

  let projects = $state([]);
  let apiKeys = $state([]);
  let newProjectName = $state('');
  let newKeyName = $state('');
  let newKeyType = $state('general');
  let newKeyProjectId = $state('');
  let createdKeyFull = $state(null);
  let renamingProject = $state(null);
  let renameValue = $state('');

  async function loadAPIData() {
    try {
      projects = await getProjects();
      apiKeys = await getAPIKeys();
      onprojectschanged?.(projects);
    } catch (e) { onerror?.(e.message); }
  }

  async function handleCreateProject() {
    if (!newProjectName.trim()) return;
    try {
      await createProject(newProjectName.trim());
      newProjectName = '';
      await loadAPIData();
    } catch (e) { onerror?.(e.message); }
  }

  async function handleRenameProject(id) {
    if (!renameValue.trim()) return;
    try {
      await renameProject(id, renameValue.trim());
      renamingProject = null;
      renameValue = '';
      await loadAPIData();
    } catch (e) { onerror?.(e.message); }
  }

  function handleDeleteProject(id) {
    showConfirm('Delete this project? Associated API keys will also be deleted.', async () => {
      try {
        await deleteProject(id);
        await loadAPIData();
      } catch (e) { onerror?.(e.message); }
    }, { danger: true, confirmLabel: 'Delete' });
  }

  async function handleCreateAPIKey() {
    if (!newKeyName.trim() || !newKeyType) return;
    try {
      const pid = newKeyType === 'project' && newKeyProjectId ? newKeyProjectId : null;
      const result = await createAPIKey(newKeyName.trim(), newKeyType, pid);
      createdKeyFull = result.key;
      newKeyName = '';
      newKeyType = 'general';
      newKeyProjectId = '';
      await loadAPIData();
    } catch (e) { onerror?.(e.message); }
  }

  function handleDeleteAPIKey(id) {
    showConfirm('Revoke this API key?', async () => {
      try {
        await deleteAPIKey(id);
        await loadAPIData();
      } catch (e) { onerror?.(e.message); }
    }, { danger: true, confirmLabel: 'Delete' });
  }

  async function loadServerInfo() {
    try { serverInfo = await getServerInfo(); } catch (e) { /* non-critical */ }
  }

  loadAPIData();
  loadServerInfo();
</script>

<!-- API Endpoint -->
{#if serverInfo}
  <div class="card mb-lg api-endpoint-card">
    <div class="flex-center-gap api-endpoint-header">
      <h3 class="api-endpoint-title">API Endpoint</h3>
      <span class="badge badge-success badge-xs">Running</span>
    </div>
    <div class="flex-center-gap api-url-row">
      <code class="text-sm word-break api-url-code">{serverInfo.api_url}</code>
      <button class="btn btn-sm" onclick={() => copyToClipboard(serverInfo.api_url)}>Copy</button>
    </div>
    {#if serverInfo.has_auth}
      <div class="text-xs text-muted mb-sm">
        Auth: Basic (<code>{serverInfo.username}</code>) or API key via <code>X-API-Key</code> header
      </div>
    {/if}
    <details class="text-xs text-muted">
      <summary class="usage-summary">Usage examples</summary>
      <div class="mt-sm flex-col gap-sm">
        <div>
          <strong>curl:</strong>
          <code class="code-example code-example-wrap">curl {serverInfo.has_auth ? `-u ${serverInfo.username}:PASSWORD ` : ''}{serverInfo.api_url}/sessions</code>
        </div>
        <div>
          <strong>API key:</strong>
          <code class="code-example code-example-wrap">curl -H "X-API-Key: YOUR_KEY" {serverInfo.api_url}/sessions</code>
        </div>
        <div>
          <strong>Discovery file:</strong>
          <code class="code-example">cat .crawlobserver-api.json</code>
        </div>
      </div>
    </details>
  </div>
{/if}

<!-- Projects -->
<div class="page-header">
  <h1>Projects</h1>
</div>

<!-- Create Project -->
<div class="card">
  <div class="form-grid">
    <div class="form-group form-group-full">
      <label for="new-project">New project</label>
      <div class="flex-center-gap">
        <input id="new-project" type="text" bind:value={newProjectName} placeholder="Project name" onkeydown={(e) => e.key === 'Enter' && handleCreateProject()} />
        <button class="btn btn-primary" onclick={handleCreateProject} disabled={!newProjectName.trim()}>Create</button>
      </div>
    </div>
  </div>
</div>

<!-- Projects List -->
{#if projects.length === 0}
  <div class="card text-center text-muted empty-state">No projects yet.</div>
{:else}
  <div class="card card-flush">
    {#each projects as p}
      <div class="session-row">
        <div class="session-info">
          {#if renamingProject === p.id}
            <div class="flex-center-gap rename-row">
              <input type="text" bind:value={renameValue} class="flex-1" onkeydown={(e) => e.key === 'Enter' && handleRenameProject(p.id)} />
              <button class="btn btn-sm btn-primary" onclick={() => handleRenameProject(p.id)}>Save</button>
              <button class="btn btn-sm" onclick={() => renamingProject = null}>Cancel</button>
            </div>
          {:else}
            <div class="session-seed">{p.name}</div>
            <div class="session-meta">
              <span>{new Date(p.created_at).toLocaleDateString()}</span>
            </div>
          {/if}
        </div>
        {#if renamingProject !== p.id}
          <div class="session-actions">
            <button class="btn btn-sm" onclick={() => { renamingProject = p.id; renameValue = p.name; }}>Rename</button>
            <button class="btn btn-sm btn-danger" onclick={() => handleDeleteProject(p.id)}>Delete</button>
          </div>
        {/if}
      </div>
    {/each}
  </div>
{/if}

<!-- API Keys Header -->
<div class="page-header api-keys-header">
  <h1>API Keys</h1>
</div>

{#if createdKeyFull}
  <div class="card key-created-card">
    <div class="key-created-inner">
      <div class="flex-1">
        <strong>API Key created!</strong> Copy it now — it won't be shown again:<br/>
        <code class="word-break key-created-code">{createdKeyFull}</code>
      </div>
      <div class="key-created-actions">
        <button class="btn btn-sm" onclick={() => copyToClipboard(createdKeyFull)}>Copy</button>
        <button class="btn btn-sm" onclick={() => createdKeyFull = null}>Dismiss</button>
      </div>
    </div>
  </div>
{/if}

<!-- Create API Key -->
<div class="card">
  <div class="form-grid">
    <div class="form-group">
      <label for="key-name">Key name</label>
      <input id="key-name" type="text" bind:value={newKeyName} placeholder="My API key" />
    </div>
    <div class="form-group">
      <label for="key-type">Type</label>
      <select id="key-type" bind:value={newKeyType}>
        <option value="general">General (full access)</option>
        <option value="project">Project (read-only)</option>
      </select>
    </div>
    {#if newKeyType === 'project'}
      <div class="form-group">
        <label for="key-project">Project</label>
        <select id="key-project" bind:value={newKeyProjectId}>
          <option value="">Select project...</option>
          {#each projects as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
      </div>
    {/if}
  </div>
  <div class="mt-md">
    <button class="btn btn-primary" onclick={handleCreateAPIKey} disabled={!newKeyName.trim() || (newKeyType === 'project' && !newKeyProjectId)}>Create Key</button>
  </div>
</div>

<!-- API Keys List -->
{#if apiKeys.length === 0}
  <div class="card text-center text-muted empty-state">No API keys yet.</div>
{:else}
  <div class="card card-flush">
    {#each apiKeys as k}
      <div class="session-row">
        <div class="session-info">
          <div class="session-seed">{k.name}</div>
          <div class="session-meta">
            <span class="badge" class:badge-info={k.type === 'general'} class:badge-warning={k.type === 'project'}>{k.type}</span>
            {#if k.project_id}
              <span class="badge badge-accent">{projects.find(p => p.id === k.project_id)?.name || k.project_id}</span>
            {/if}
            <code class="key-prefix-code">{k.key_prefix}</code>
            <span>{new Date(k.created_at).toLocaleDateString()}</span>
            <span>{k.last_used_at ? 'Used ' + timeAgo(k.last_used_at) : 'Never used'}</span>
          </div>
        </div>
        <div class="session-actions">
          <button class="btn btn-sm btn-danger" onclick={() => handleDeleteAPIKey(k.id)}>Revoke</button>
        </div>
      </div>
    {/each}
  </div>
{/if}

{#if confirmState}<ConfirmModal message={confirmState.message} danger={confirmState.danger} confirmLabel={confirmState.confirmLabel} onconfirm={() => { confirmState.onConfirm(); confirmState = null; }} oncancel={() => confirmState = null} />{/if}

<style>
  .api-endpoint-card {
    border: 1px solid var(--border);
  }

  .api-endpoint-header {
    margin-bottom: 12px;
  }

  .api-endpoint-title {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }

  .badge-xs {
    font-size: 11px;
  }

  .api-url-row {
    margin-bottom: 10px;
  }

  .api-url-code {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg-secondary);
    border-radius: 6px;
  }

  .usage-summary {
    cursor: pointer;
    user-select: none;
  }

  .code-example {
    display: block;
    padding: 6px 10px;
    background: var(--bg-secondary);
    border-radius: 4px;
    margin-top: 4px;
    font-size: 12px;
  }

  .code-example-wrap {
    white-space: pre-wrap;
  }

  .form-group-full {
    grid-column: 1 / -1;
  }

  .empty-state {
    padding: 32px;
  }

  .rename-row {
    flex: 1;
  }

  .flex-1 {
    flex: 1;
  }

  .api-keys-header {
    margin-top: 32px;
  }

  .key-created-card {
    border: 1px solid var(--success);
    background: var(--success-bg);
  }

  .key-created-inner {
    display: flex;
    align-items: flex-start;
    gap: 12px;
  }

  .key-created-code {
    font-size: 0.85rem;
    margin-top: 6px;
    display: inline-block;
  }

  .key-created-actions {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
  }

  .badge-accent {
    background: var(--accent-light);
    color: var(--accent);
  }

  .key-prefix-code {
    font-size: 0.8rem;
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
  .session-row:last-child { border-bottom: none; }
  .session-row:hover { background: var(--bg-hover); }
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
</style>
