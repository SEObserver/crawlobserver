<script>
  import { getLogs, exportLogs } from '../api.js';
  import { onDestroy } from 'svelte';
  import { t, getLocale } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';

  let { onerror } = $props();

  let logs = $state([]);
  let total = $state(0);
  let loading = $state(false);
  let offset = $state(0);
  const limit = 100;

  // Filters
  let level = $state('');
  let component = $state('');
  let search = $state('');
  let searchInput = $state('');

  async function loadLogs() {
    loading = true;
    try {
      const res = await getLogs(limit, offset, level, component, search);
      logs = res.logs || [];
      total = res.total || 0;
    } catch (e) {
      onerror?.(e.message);
    }
    loading = false;
  }

  function applySearch() {
    search = searchInput;
    offset = 0;
    loadLogs();
  }

  function changeLevel(v) {
    level = v;
    offset = 0;
    loadLogs();
  }

  function changeComponent(v) {
    component = v;
    offset = 0;
    loadLogs();
  }

  function prevPage() {
    if (offset >= limit) {
      offset -= limit;
      loadLogs();
    }
  }

  function nextPage() {
    if (offset + limit < total) {
      offset += limit;
      loadLogs();
    }
  }

  function fmtTime(ts) {
    if (!ts) return '';
    const d = new Date(ts);
    return d.toLocaleString(getLocale(), {
      hour12: false,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      fractionalSecondDigits: 3,
    });
  }

  // Auto-refresh every 5s
  const interval = setInterval(loadLogs, 5000);
  onDestroy(() => clearInterval(interval));

  loadLogs();
</script>

<div class="page-header">
  <h1>{t('logs.title')}</h1>
  <div class="flex-center-gap">
    <button class="btn btn-sm" onclick={loadLogs} disabled={loading}>
      {loading ? t('common.loading') : t('common.refresh')}
    </button>
    <button class="btn btn-sm" onclick={exportLogs}>{t('logs.exportJsonl')}</button>
  </div>
</div>

<div class="logs-filters">
  <SearchSelect
    small
    bind:value={level}
    onchange={changeLevel}
    options={[
      { value: '', label: t('logs.allLevels') },
      { value: 'debug', label: t('logs.debug') },
      { value: 'info', label: t('logs.info') },
      { value: 'warn', label: t('logs.warn') },
      { value: 'error', label: t('logs.error') },
    ]}
  />

  <SearchSelect
    small
    bind:value={component}
    onchange={changeComponent}
    options={[
      { value: '', label: t('logs.allComponents') },
      { value: 'server', label: t('logs.server') },
      { value: 'crawler', label: t('logs.crawler') },
      { value: 'gsc', label: t('logs.gsc') },
      { value: 'storage', label: t('logs.storage') },
    ]}
  />

  <form
    class="logs-search-form"
    onsubmit={(e) => {
      e.preventDefault();
      applySearch();
    }}
  >
    <input
      class="logs-search"
      type="text"
      placeholder={t('logs.searchMessages')}
      bind:value={searchInput}
    />
    <button class="btn btn-sm" type="submit">{t('common.search')}</button>
  </form>
</div>

{#if loading && logs.length === 0}
  <div class="loading">{t('common.loading')}</div>
{:else}
  <div class="card card-flush">
    <table>
      <thead>
        <tr>
          <th class="col-timestamp">{t('logs.timestamp')}</th>
          <th class="col-level">{t('logs.level')}</th>
          <th class="col-component">{t('logs.component')}</th>
          <th>{t('logs.message')}</th>
        </tr>
      </thead>
      <tbody>
        {#each logs as log}
          <tr>
            <td class="td-mono">{fmtTime(log.timestamp)}</td>
            <td>
              <span class="log-badge log-badge-{log.level}">{log.level}</span>
            </td>
            <td><span class="log-component">{log.component}</span></td>
            <td class="td-mono td-msg">{log.message}</td>
          </tr>
        {:else}
          <tr><td colspan="4" class="logs-empty">{t('logs.noLogs')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  {#if total > limit}
    <div class="pagination">
      <button class="btn btn-sm" onclick={prevPage} disabled={offset === 0}
        >{t('common.previous')}</button
      >
      <span class="pagination-info"
        >{offset + 1}-{Math.min(offset + limit, total)} {t('common.of')} {total}</span
      >
      <button class="btn btn-sm" onclick={nextPage} disabled={offset + limit >= total}
        >{t('common.next')}</button
      >
    </div>
  {/if}
{/if}

<style>
  .logs-filters {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
    flex-wrap: wrap;
    align-items: center;
  }
  .logs-filters :global(.ss-wrap) {
    width: 160px;
    flex-shrink: 0;
  }
  .logs-search-form {
    display: flex;
    gap: 4px;
    flex: 1;
    min-width: 200px;
  }
  .logs-search {
    flex: 1;
    padding: 6px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
  }
  .td-mono {
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 12px;
  }
  .td-msg {
    word-break: break-all;
    max-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  tr:hover .td-msg {
    white-space: normal;
    overflow: visible;
    text-overflow: unset;
  }
  .log-badge {
    display: inline-block;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.3px;
  }
  .log-badge-debug {
    background: var(--surface-hover, #f0f0f0);
    color: var(--text-muted);
  }
  .log-badge-info {
    background: #dbeafe;
    color: #1e40af;
  }
  .log-badge-warn {
    background: #fef3c7;
    color: #92400e;
  }
  .log-badge-error {
    background: #fee2e2;
    color: #991b1b;
  }
  :global([data-theme='dark']) .log-badge-info {
    background: #1e3a5f;
    color: #93c5fd;
  }
  :global([data-theme='dark']) .log-badge-warn {
    background: #422006;
    color: #fcd34d;
  }
  :global([data-theme='dark']) .log-badge-error {
    background: #450a0a;
    color: #fca5a5;
  }
  .log-component {
    font-size: 12px;
    color: var(--text-muted);
  }
  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-top: 12px;
  }
  .pagination-info {
    font-size: 13px;
    color: var(--text-muted);
  }
  .col-timestamp {
    width: 180px;
  }
  .col-level {
    width: 70px;
  }
  .col-component {
    width: 100px;
  }
  .logs-empty {
    text-align: center;
    padding: 24px;
    color: var(--text-muted);
  }
</style>
