<script>
  import { updateTheme, getBackups, createBackup, restoreBackup, deleteBackup } from '../api.js';

  let { initialTheme, onerror, onsave, oncancel } = $props();

  let editTheme = $state({ ...initialTheme });
  let savingTheme = $state(false);

  // Backups
  let backups = $state([]);
  let loadingBackups = $state(false);
  let creatingBackup = $state(false);
  let restoringBackup = $state(null);
  let backupMessage = $state('');

  function previewTheme() {
    onsave?.({ ...editTheme }, true);
  }

  async function saveTheme() {
    savingTheme = true;
    try {
      const saved = await updateTheme(editTheme);
      onsave?.(saved, false);
    } catch (e) {
      onerror?.(e.message);
    } finally {
      savingTheme = false;
    }
  }

  async function loadBackups() {
    loadingBackups = true;
    try {
      backups = await getBackups();
    } catch (e) {
      backupMessage = e.message;
    } finally {
      loadingBackups = false;
    }
  }

  async function doCreateBackup() {
    creatingBackup = true;
    backupMessage = '';
    try {
      await createBackup();
      backupMessage = 'Backup created successfully.';
      await loadBackups();
    } catch (e) {
      backupMessage = 'Backup failed: ' + e.message;
    } finally {
      creatingBackup = false;
    }
  }

  async function doRestoreBackup(filename) {
    if (!confirm(`Restore from ${filename}? The application should be restarted after restore.`)) return;
    restoringBackup = filename;
    backupMessage = '';
    try {
      const result = await restoreBackup(filename);
      backupMessage = result.message || 'Restore complete. Restart to apply.';
    } catch (e) {
      backupMessage = 'Restore failed: ' + e.message;
    } finally {
      restoringBackup = null;
    }
  }

  async function doDeleteBackup(name) {
    if (!confirm(`Delete backup ${name}?`)) return;
    try {
      await deleteBackup(name);
      await loadBackups();
    } catch (e) {
      backupMessage = 'Delete failed: ' + e.message;
    }
  }

  function formatBytes(bytes) {
    if (!bytes) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    let i = 0;
    let val = bytes;
    while (val >= 1024 && i < units.length - 1) { val /= 1024; i++; }
    return val.toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
  }

  loadBackups();
</script>

<!-- Settings -->
<div class="page-header">
  <h1>Settings</h1>
</div>
<div class="card">
  <div class="form-grid">
    <div class="form-group">
      <label for="set-appname">App Name</label>
      <input id="set-appname" type="text" bind:value={editTheme.app_name} oninput={previewTheme} />
    </div>
    <div class="form-group">
      <label for="set-logo">Logo URL</label>
      <input id="set-logo" type="text" bind:value={editTheme.logo_url} oninput={previewTheme} placeholder="https://example.com/logo.png" />
    </div>
    {#if editTheme.logo_url}
      <div class="form-group" style="grid-column: 1 / -1;">
        <span style="font-weight: 500; font-size: 0.85rem; color: var(--text-secondary);">Logo Preview</span>
        <img src={editTheme.logo_url} alt="Logo preview" style="max-height: 48px; border-radius: 6px; background: var(--bg-card);" />
      </div>
    {/if}
    <div class="form-group">
      <label for="set-accent">Accent Color</label>
      <div style="display: flex; align-items: center; gap: 10px;">
        <input id="set-accent" type="color" value={editTheme.accent_color} oninput={(e) => { editTheme.accent_color = e.target.value; previewTheme(); }} style="width: 48px; height: 36px; border: 1px solid var(--border); border-radius: 6px; cursor: pointer; padding: 2px;" />
        <span style="font-family: monospace; color: var(--text-secondary);">{editTheme.accent_color}</span>
      </div>
    </div>
    <div class="form-group">
      <span style="font-weight: 500; font-size: 0.85rem; color: var(--text-secondary); display: block; margin-bottom: 4px;">Mode</span>
      <div style="display: flex; gap: 8px;">
        <button class="btn btn-sm" class:btn-primary={editTheme.mode === 'light'} onclick={() => { editTheme.mode = 'light'; previewTheme(); }}>Light</button>
        <button class="btn btn-sm" class:btn-primary={editTheme.mode === 'dark'} onclick={() => { editTheme.mode = 'dark'; previewTheme(); }}>Dark</button>
      </div>
    </div>
  </div>
  <div style="display: flex; gap: 8px; margin-top: 20px;">
    <button class="btn btn-primary" onclick={saveTheme} disabled={savingTheme}>
      {savingTheme ? 'Saving...' : 'Save'}
    </button>
    <button class="btn" onclick={() => oncancel?.()}>Cancel</button>
  </div>
</div>

<!-- Backups -->
<div class="page-header" style="margin-top:32px">
  <h1>Backups</h1>
  <button class="btn btn-sm btn-primary" onclick={doCreateBackup} disabled={creatingBackup}>
    {creatingBackup ? 'Creating...' : 'Create Backup'}
  </button>
</div>
{#if backupMessage}
  <div class="alert alert-info" style="margin-bottom:16px">
    <span>{backupMessage}</span>
    <button class="btn btn-sm btn-ghost" onclick={() => backupMessage = ''}>Dismiss</button>
  </div>
{/if}
<div class="card card-flush">
  {#if loadingBackups}
    <div style="padding:20px;color:var(--text-muted)">Loading backups...</div>
  {:else if backups.length === 0}
    <div style="padding:20px;color:var(--text-muted)">No backups yet.</div>
  {:else}
    <table>
      <thead>
        <tr>
          <th>Filename</th>
          <th>Version</th>
          <th>Date</th>
          <th>Size</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each backups as b}
          <tr>
            <td class="cell-url">{b.filename}</td>
            <td>{b.version || '-'}</td>
            <td>{new Date(b.created_at).toLocaleString()}</td>
            <td>{formatBytes(b.size)}</td>
            <td style="white-space:nowrap">
              <button class="btn btn-sm" onclick={() => doRestoreBackup(b.filename)}
                disabled={restoringBackup === b.filename}>
                {restoringBackup === b.filename ? 'Restoring...' : 'Restore'}
              </button>
              <button class="btn btn-sm btn-danger" onclick={() => doDeleteBackup(b.filename)}>Delete</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>
