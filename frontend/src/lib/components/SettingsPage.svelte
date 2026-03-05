<script>
  import {
    updateTheme,
    getBackups,
    createBackup,
    restoreBackup,
    deleteBackup,
    getTelemetry,
    updateTelemetry,
  } from '../api.js';
  import { enableTelemetry, disableTelemetry } from '../telemetry.js';
  import { fmtSize } from '../utils.js';
  import { t, setLocale, getLocale } from '../i18n/index.svelte.js';
  import ConfirmModal from './ConfirmModal.svelte';

  let { initialTheme, onerror, onsave, oncancel } = $props();

  let confirmState = $state(null);

  function showConfirm(message, onConfirm, opts = {}) {
    confirmState = { message, onConfirm, ...opts };
  }

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
      backupMessage = t('settings.backupCreated');
      await loadBackups();
    } catch (e) {
      backupMessage = t('settings.backupFailed', { error: e.message });
    } finally {
      creatingBackup = false;
    }
  }

  function doRestoreBackup(filename) {
    showConfirm(
      t('settings.restoreConfirm', { name: filename }),
      async () => {
        restoringBackup = filename;
        backupMessage = '';
        try {
          const result = await restoreBackup(filename);
          backupMessage = result.message || t('settings.restoreComplete');
        } catch (e) {
          backupMessage = t('settings.restoreFailed', { error: e.message });
        } finally {
          restoringBackup = null;
        }
      },
      { danger: true, confirmLabel: t('settings.restore') },
    );
  }

  function doDeleteBackup(name) {
    showConfirm(
      t('settings.deleteBackupConfirm', { name }),
      async () => {
        try {
          await deleteBackup(name);
          await loadBackups();
        } catch (e) {
          backupMessage = t('settings.deleteFailed', { error: e.message });
        }
      },
      { danger: true, confirmLabel: t('common.delete') },
    );
  }

  // Telemetry
  let telemetryEnabled = $state(false);
  let telemetryLoading = $state(true);

  async function loadTelemetry() {
    try {
      const tel = await getTelemetry();
      telemetryEnabled = tel.enabled;
    } catch {
      // Telemetry endpoint may not exist in CLI mode
    } finally {
      telemetryLoading = false;
    }
  }

  async function toggleTelemetry() {
    telemetryEnabled = !telemetryEnabled;
    try {
      await updateTelemetry(telemetryEnabled);
      if (telemetryEnabled) {
        enableTelemetry();
      } else {
        disableTelemetry();
      }
    } catch (e) {
      telemetryEnabled = !telemetryEnabled; // revert
      onerror?.(e.message);
    }
  }

  loadBackups();
  loadTelemetry();
</script>

<!-- Settings -->
<div class="page-header">
  <h1>{t('settings.title')}</h1>
</div>
<div class="card">
  <div class="form-grid">
    <div class="form-group">
      <label for="set-appname">{t('settings.appName')}</label>
      <input id="set-appname" type="text" bind:value={editTheme.app_name} oninput={previewTheme} />
    </div>
    <div class="form-group">
      <label for="set-logo">{t('settings.logoUrl')}</label>
      <input
        id="set-logo"
        type="text"
        bind:value={editTheme.logo_url}
        oninput={previewTheme}
        placeholder="https://example.com/logo.png"
      />
    </div>
    {#if editTheme.logo_url}
      <div class="form-group full-width">
        <span class="field-label">{t('settings.logoPreview')}</span>
        <img src={editTheme.logo_url} alt={t('settings.logoPreview')} class="logo-preview" />
      </div>
    {/if}
    <div class="form-group">
      <label for="set-accent">{t('settings.accentColor')}</label>
      <div class="color-picker-row">
        <input
          id="set-accent"
          type="color"
          value={editTheme.accent_color}
          oninput={(e) => {
            editTheme.accent_color = e.target.value;
            previewTheme();
          }}
          class="color-swatch"
        />
        <span class="color-value">{editTheme.accent_color}</span>
      </div>
    </div>
    <div class="form-group">
      <span class="field-label mb-xs">{t('settings.mode')}</span>
      <div class="flex-center-gap">
        <button
          class="btn btn-sm"
          class:btn-primary={editTheme.mode === 'light'}
          onclick={() => {
            editTheme.mode = 'light';
            previewTheme();
          }}>{t('settings.light')}</button
        >
        <button
          class="btn btn-sm"
          class:btn-primary={editTheme.mode === 'dark'}
          onclick={() => {
            editTheme.mode = 'dark';
            previewTheme();
          }}>{t('settings.dark')}</button
        >
      </div>
    </div>
    <div class="form-group">
      <label>{t('settings.language')}</label>
      <div class="flex-center-gap" style="flex-wrap: wrap;">
        {#each ['en','fr','es','pt','nl','it','de','ru','zh','ja','tr','id','ko','pl','he','ar'] as lang}
          <button
            class="btn btn-sm"
            class:btn-primary={getLocale() === lang}
            onclick={() => setLocale(lang)}>{t('settings.lang' + lang.charAt(0).toUpperCase() + lang.slice(1))}</button
          >
        {/each}
      </div>
    </div>
  </div>
  <div class="form-actions">
    <button class="btn btn-primary" onclick={saveTheme} disabled={savingTheme}>
      {savingTheme ? t('common.saving') : t('common.save')}
    </button>
    <button class="btn" onclick={() => oncancel?.()}>{t('common.cancel')}</button>
  </div>
</div>

<!-- Analytics -->
<div class="page-header section-gap">
  <h1>{t('settings.telemetryTitle')}</h1>
</div>
<div class="card">
  {#if !telemetryLoading}
    <div class="telemetry-section">
      <label class="telemetry-toggle">
        <input type="checkbox" checked={telemetryEnabled} onchange={toggleTelemetry} />
        <span>{t('settings.telemetryEnabled')}</span>
      </label>
      <p class="telemetry-desc">{t('settings.telemetryDesc')}</p>
    </div>
  {/if}
</div>

<!-- Backups -->
<div class="page-header section-gap">
  <h1>{t('settings.backups')}</h1>
  <button class="btn btn-sm btn-primary" onclick={doCreateBackup} disabled={creatingBackup}>
    {creatingBackup ? t('settings.creating') : t('settings.createBackup')}
  </button>
</div>
{#if backupMessage}
  <div class="alert alert-info mb-md">
    <span>{backupMessage}</span>
    <button class="btn btn-sm btn-ghost" onclick={() => (backupMessage = '')}
      >{t('common.dismiss')}</button
    >
  </div>
{/if}
<div class="card card-flush">
  {#if loadingBackups}
    <div class="card-empty-msg">{t('settings.loadingBackups')}</div>
  {:else if backups.length === 0}
    <div class="card-empty-msg">{t('settings.noBackups')}</div>
  {:else}
    <table>
      <thead>
        <tr>
          <th>{t('settings.filename')}</th>
          <th>{t('settings.version')}</th>
          <th>{t('common.date')}</th>
          <th>{t('common.size')}</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each backups as b}
          <tr>
            <td class="cell-url">{b.filename}</td>
            <td>{b.version || '-'}</td>
            <td>{new Date(b.created_at).toLocaleString()}</td>
            <td>{fmtSize(b.size)}</td>
            <td class="nowrap">
              <button
                class="btn btn-sm"
                onclick={() => doRestoreBackup(b.filename)}
                disabled={restoringBackup === b.filename}
              >
                {restoringBackup === b.filename ? t('settings.restoring') : t('settings.restore')}
              </button>
              <button class="btn btn-sm btn-danger" onclick={() => doDeleteBackup(b.filename)}
                >{t('common.delete')}</button
              >
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

{#if confirmState}<ConfirmModal
    message={confirmState.message}
    danger={confirmState.danger}
    confirmLabel={confirmState.confirmLabel}
    onconfirm={() => {
      confirmState.onConfirm();
      confirmState = null;
    }}
    oncancel={() => (confirmState = null)}
  />{/if}

<style>
  .full-width {
    grid-column: 1 / -1;
  }

  .field-label {
    font-weight: 500;
    font-size: 0.85rem;
    color: var(--text-secondary);
    display: block;
  }

  .logo-preview {
    max-height: 48px;
    border-radius: 6px;
    background: var(--bg-card);
  }

  .color-picker-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .color-swatch {
    width: 48px;
    height: 36px;
    border: 1px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
    padding: 2px;
  }

  .color-value {
    font-family: monospace;
    color: var(--text-secondary);
  }

  .form-actions {
    display: flex;
    gap: 8px;
    margin-top: 20px;
  }

  .section-gap {
    margin-top: 32px;
  }

  .card-empty-msg {
    padding: 20px;
    color: var(--text-muted);
  }

  .telemetry-section {
    padding: 20px;
  }

  .telemetry-toggle {
    display: flex;
    align-items: center;
    gap: 10px;
    cursor: pointer;
    font-size: 0.95rem;
    margin-bottom: 8px;
  }

  .telemetry-toggle input[type='checkbox'] {
    width: 18px;
    height: 18px;
    accent-color: var(--accent);
  }

  .telemetry-desc {
    font-size: 0.82rem;
    color: var(--text-muted);
    line-height: 1.5;
  }
</style>
