<script>
  import { getProjects, createProject, renameProject, deleteProject, getAPIKeys, createAPIKey, deleteAPIKey } from '../api.js';
  import { timeAgo, copyToClipboard } from '../utils.js';

  let { onerror, onprojectschanged } = $props();

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

  async function handleDeleteProject(id) {
    if (!confirm('Delete this project? Associated API keys will also be deleted.')) return;
    try {
      await deleteProject(id);
      await loadAPIData();
    } catch (e) { onerror?.(e.message); }
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

  async function handleDeleteAPIKey(id) {
    if (!confirm('Revoke this API key?')) return;
    try {
      await deleteAPIKey(id);
      await loadAPIData();
    } catch (e) { onerror?.(e.message); }
  }

  loadAPIData();
</script>

<!-- Projects -->
<div class="page-header">
  <h1>Projects</h1>
</div>

<!-- Create Project -->
<div class="card">
  <div class="form-grid">
    <div class="form-group" style="grid-column: 1 / -1;">
      <label for="new-project">New project</label>
      <div style="display: flex; gap: 8px;">
        <input id="new-project" type="text" bind:value={newProjectName} placeholder="Project name" onkeydown={(e) => e.key === 'Enter' && handleCreateProject()} />
        <button class="btn btn-primary" onclick={handleCreateProject} disabled={!newProjectName.trim()}>Create</button>
      </div>
    </div>
  </div>
</div>

<!-- Projects List -->
{#if projects.length === 0}
  <div class="card" style="text-align: center; color: var(--text-muted); padding: 32px;">No projects yet.</div>
{:else}
  <div class="card card-flush">
    {#each projects as p}
      <div class="session-row">
        <div class="session-info">
          {#if renamingProject === p.id}
            <div style="display: flex; gap: 8px; align-items: center; flex: 1;">
              <input type="text" bind:value={renameValue} style="flex: 1;" onkeydown={(e) => e.key === 'Enter' && handleRenameProject(p.id)} />
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
<div class="page-header" style="margin-top: 32px;">
  <h1>API Keys</h1>
</div>

{#if createdKeyFull}
  <div class="card" style="border: 1px solid var(--success); background: var(--success-bg);">
    <div style="display: flex; align-items: flex-start; gap: 12px;">
      <div style="flex: 1;">
        <strong>API Key created!</strong> Copy it now — it won't be shown again:<br/>
        <code style="font-size: 0.85rem; word-break: break-all; margin-top: 6px; display: inline-block;">{createdKeyFull}</code>
      </div>
      <div style="display: flex; gap: 6px; flex-shrink: 0;">
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
  <div style="margin-top: 16px;">
    <button class="btn btn-primary" onclick={handleCreateAPIKey} disabled={!newKeyName.trim() || (newKeyType === 'project' && !newKeyProjectId)}>Create Key</button>
  </div>
</div>

<!-- API Keys List -->
{#if apiKeys.length === 0}
  <div class="card" style="text-align: center; color: var(--text-muted); padding: 32px;">No API keys yet.</div>
{:else}
  <div class="card card-flush">
    {#each apiKeys as k}
      <div class="session-row">
        <div class="session-info">
          <div class="session-seed">{k.name}</div>
          <div class="session-meta">
            <span class="badge" class:badge-info={k.type === 'general'} class:badge-warning={k.type === 'project'}>{k.type}</span>
            {#if k.project_id}
              <span class="badge" style="background: var(--accent-light); color: var(--accent);">{projects.find(p => p.id === k.project_id)?.name || k.project_id}</span>
            {/if}
            <code style="font-size: 0.8rem;">{k.key_prefix}</code>
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
