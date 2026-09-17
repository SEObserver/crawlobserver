<script>
  import { onMount } from 'svelte';
  import { getProjectCrawlHeaders, setProjectCrawlHeaders } from '../api.js';
  import { t } from '../i18n/index.svelte.js';

  let { projectId, onerror, onsaved } = $props();

  /** Rows rather than a map, so that two half-typed names can coexist while
   * editing without one silently replacing the other. */
  function toRows(map) {
    const rows = Object.entries(map || {}).map(([name, value]) => ({ name, value }));
    return rows.length > 0 ? rows : [{ name: '', value: '' }];
  }

  // Fetched rather than passed in: the project payload carries no headers, so
  // that a read-only key listing projects never receives them.
  let rows = $state([{ name: '', value: '' }]);
  let loading = $state(true);
  let saving = $state(false);
  let saved = $state(false);
  let error = $state('');

  onMount(async () => {
    try {
      const res = await getProjectCrawlHeaders(projectId);
      rows = toRows(res?.headers);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function addRow() {
    rows = [...rows, { name: '', value: '' }];
    saved = false;
  }

  function removeRow(index) {
    rows = rows.filter((_, i) => i !== index);
    if (rows.length === 0) rows = [{ name: '', value: '' }];
    saved = false;
  }

  function touched() {
    saved = false;
    error = '';
  }

  async function save() {
    if (saving) return;
    saving = true;
    error = '';

    const map = {};
    for (const row of rows) {
      const name = row.name.trim();
      if (name === '') continue;
      map[name] = row.value.trim();
    }

    try {
      await setProjectCrawlHeaders(projectId, map);
      rows = toRows(map);
      saved = true;
      onsaved?.(map);
    } catch (e) {
      // Shown here rather than only in the page banner: the message names the
      // header at fault, and it is read next to the field that holds it.
      error = e.message;
      onerror?.(e.message);
    } finally {
      saving = false;
    }
  }
</script>

<details class="crawl-headers">
  <summary>{t('project.crawlHeaders')}</summary>

  <div class="crawl-headers-body">
    <p class="crawl-headers-desc">{t('project.crawlHeadersDesc')}</p>

    <div class="crawl-headers-grid" class:crawl-headers-loading={loading}>
      <span class="crawl-headers-label">{t('project.crawlHeaderName')}</span>
      <span class="crawl-headers-label">{t('project.crawlHeaderValue')}</span>
      <span></span>

      {#each rows as row, i (i)}
        <input
          type="text"
          bind:value={row.name}
          oninput={touched}
          aria-label={t('project.crawlHeaderName')}
        />
        <input
          type="text"
          class="crawl-headers-value"
          bind:value={row.value}
          oninput={touched}
          aria-label={t('project.crawlHeaderValue')}
        />
        <button
          type="button"
          class="btn btn-sm btn-ghost"
          onclick={() => removeRow(i)}
          title={t('common.delete')}
          aria-label={t('common.delete')}
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
            ><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg
          >
        </button>
      {/each}
    </div>

    {#if error}
      <p class="crawl-headers-error" role="alert">{error}</p>
    {/if}

    <div class="crawl-headers-actions">
      <button type="button" class="btn" onclick={addRow} disabled={loading}
        >{t('project.addCrawlHeader')}</button
      >
      <button type="button" class="btn btn-primary" onclick={save} disabled={saving || loading}>
        {saving ? t('common.saving') : t('common.save')}
      </button>
      {#if saved}
        <span class="crawl-headers-saved">{t('project.crawlHeadersSaved')}</span>
      {/if}
    </div>
  </div>
</details>

<style>
  /* Laid out on the same values as the danger zone below it, so the two blocks
     read as one family: same width, same frame, same summary type. */
  .crawl-headers {
    margin-top: 32px;
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .crawl-headers summary {
    padding: 12px 16px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-muted);
    cursor: pointer;
    list-style: none;
  }
  .crawl-headers summary::-webkit-details-marker {
    display: none;
  }
  .crawl-headers[open] summary {
    border-bottom: 1px solid var(--border);
  }
  .crawl-headers-body {
    padding: 16px;
  }
  .crawl-headers-desc {
    margin: 0 0 16px;
    font-size: 13px;
    color: var(--text-muted);
    line-height: 1.5;
    max-width: 78ch;
  }

  .crawl-headers-grid {
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr) auto;
    gap: 8px 12px;
    align-items: center;
  }
  .crawl-headers-label {
    font-size: 13px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  /* The field rules of .form-group in the global sheet. They are repeated
     rather than borrowed because the rows are a grid, not the stacked
     label-and-field that class lays out. */
  .crawl-headers-grid input {
    padding: 9px 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
    color: var(--text);
    font-size: 14px;
    font-family: inherit;
    transition: border-color 0.15s;
    min-width: 0;
  }
  .crawl-headers-grid input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-light);
  }
  .crawl-headers-grid input.crawl-headers-value {
    font-family: var(--font-mono, monospace);
    font-size: 13px;
  }

  .crawl-headers-error {
    margin: 16px 0 0;
    font-size: 13px;
    color: var(--error);
  }
  .crawl-headers-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 16px;
  }
  .crawl-headers-saved {
    font-size: 13px;
    color: var(--text-muted);
  }
  .crawl-headers-loading {
    opacity: 0.5;
  }
</style>
