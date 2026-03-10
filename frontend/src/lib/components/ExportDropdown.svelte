<script>
  import { getServerInfo } from '../api.js';
  import { copyToClipboard } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { onexportcsv, exporting = false, apiPath = '', disabled = false } = $props();

  let open = $state(false);
  let copied = $state(false);

  /** @type {string|null} */
  let cachedApiUrl = null;

  async function getApiUrl() {
    if (cachedApiUrl) return cachedApiUrl;
    try {
      const info = await getServerInfo();
      cachedApiUrl = info?.api_url || '';
    } catch {
      cachedApiUrl = '';
    }
    return cachedApiUrl;
  }

  function toggle() {
    if (disabled && !open) return;
    open = !open;
  }

  function close() {
    open = false;
  }

  function handleExportCSV() {
    close();
    onexportcsv?.();
  }

  async function handleCopyApiUrl() {
    close();
    const base = await getApiUrl();
    const fullUrl = base ? `${base}${apiPath}` : apiPath;
    const cmd = `curl -H "X-API-Key: YOUR_KEY" "${fullUrl}"`;
    copyToClipboard(cmd);
    copied = true;
    setTimeout(() => (copied = false), 2000);
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') close();
  }

  function handleClickOutside(e) {
    if (!e.target.closest('.export-dropdown')) close();
  }

  $effect(() => {
    if (open) {
      document.addEventListener('click', handleClickOutside, true);
      document.addEventListener('keydown', handleKeydown, true);
      return () => {
        document.removeEventListener('click', handleClickOutside, true);
        document.removeEventListener('keydown', handleKeydown, true);
      };
    }
  });
</script>

<div class="export-dropdown">
  <button class="btn btn-sm export-toggle" onclick={toggle} {disabled}>
    {#if exporting}
      <svg
        class="csv-spinner"
        viewBox="0 0 24 24"
        width="14"
        height="14"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        ><path
          d="M12 2v4m0 12v4m-7.07-3.93l2.83-2.83m8.48-8.48l2.83-2.83M2 12h4m12 0h4m-3.93 7.07l-2.83-2.83M7.76 7.76L4.93 4.93"
        /></svg
      >
      {t('common.exportingCsv')}
    {:else}
      {t('common.export')}
      <svg
        viewBox="0 0 24 24"
        width="12"
        height="12"
        fill="none"
        stroke="currentColor"
        stroke-width="2"><polyline points="6 9 12 15 18 9" /></svg
      >
    {/if}
  </button>
  {#if open}
    <div class="export-menu">
      <button class="export-menu-item" onclick={handleExportCSV}>
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
            points="7 10 12 15 17 10"
          /><line x1="12" y1="15" x2="12" y2="3" /></svg
        >
        {t('common.exportCsv')}
      </button>
      <button class="export-menu-item" onclick={handleCopyApiUrl}>
        {#if copied}
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg
          >
          {t('common.copied')}
        {:else}
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            ><rect x="9" y="9" width="13" height="13" rx="2" ry="2" /><path
              d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"
            /></svg
          >
          {t('common.copyApiUrl')}
        {/if}
      </button>
    </div>
  {/if}
</div>

<style>
  .export-dropdown {
    position: relative;
    display: inline-block;
  }
  .export-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .export-menu {
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 4px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
    min-width: 170px;
    z-index: 100;
    padding: 4px;
  }
  .export-menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 10px;
    border: none;
    background: none;
    color: var(--fg);
    font-size: 13px;
    cursor: pointer;
    border-radius: 5px;
    text-align: left;
    white-space: nowrap;
  }
  .export-menu-item:hover {
    background: var(--bg-hover);
  }
</style>
