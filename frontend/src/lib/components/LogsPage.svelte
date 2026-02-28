<script>
  import { getLogs, exportLogs } from '../api.js';
  import { onDestroy } from 'svelte';

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
    } catch (e) { onerror?.(e.message); }
    loading = false;
  }

  function applySearch() {
    search = searchInput;
    offset = 0;
    loadLogs();
  }

  function changeLevel(e) {
    level = e.target.value;
    offset = 0;
    loadLogs();
  }

  function changeComponent(e) {
    component = e.target.value;
    offset = 0;
    loadLogs();
  }

  function prevPage() {
    if (offset >= limit) { offset -= limit; loadLogs(); }
  }

  function nextPage() {
    if (offset + limit < total) { offset += limit; loadLogs(); }
  }

  function fmtTime(ts) {
    if (!ts) return '';
    const d = new Date(ts);
    return d.toLocaleString('en-GB', { hour12: false, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 });
  }

  // Auto-refresh every 5s
  const interval = setInterval(loadLogs, 5000);
  onDestroy(() => clearInterval(interval));

  loadLogs();
</script>

<div class="page-header">
  <h1>Application Logs</h1>
  <div class="flex-center-gap">
    <button class="btn btn-sm" onclick={loadLogs} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
    <button class="btn btn-sm" onclick={exportLogs}>Export JSONL</button>
  </div>
</div>

<div class="logs-filters">
  <select class="logs-select" onchange={changeLevel} value={level}>
    <option value="">All Levels</option>
    <option value="debug">Debug</option>
    <option value="info">Info</option>
    <option value="warn">Warn</option>
    <option value="error">Error</option>
  </select>

  <select class="logs-select" onchange={changeComponent} value={component}>
    <option value="">All Components</option>
    <option value="server">Server</option>
    <option value="crawler">Crawler</option>
    <option value="gsc">GSC</option>
    <option value="storage">Storage</option>
  </select>

  <form class="logs-search-form" onsubmit={(e) => { e.preventDefault(); applySearch(); }}>
    <input class="logs-search" type="text" placeholder="Search messages..." bind:value={searchInput} />
    <button class="btn btn-sm" type="submit">Search</button>
  </form>
</div>

{#if loading && logs.length === 0}
  <div class="loading">Loading logs...</div>
{:else}
  <div class="card card-flush">
    <table>
      <thead>
        <tr>
          <th class="col-timestamp">Timestamp</th>
          <th class="col-level">Level</th>
          <th class="col-component">Component</th>
          <th>Message</th>
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
          <tr><td colspan="4" class="logs-empty">No logs found</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  {#if total > limit}
    <div class="pagination">
      <button class="btn btn-sm" onclick={prevPage} disabled={offset === 0}>Previous</button>
      <span class="pagination-info">{offset + 1}-{Math.min(offset + limit, total)} of {total}</span>
      <button class="btn btn-sm" onclick={nextPage} disabled={offset + limit >= total}>Next</button>
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
  .logs-select {
    padding: 6px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
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
  .log-badge-debug { background: var(--surface-hover, #f0f0f0); color: var(--text-muted); }
  .log-badge-info { background: #dbeafe; color: #1e40af; }
  .log-badge-warn { background: #fef3c7; color: #92400e; }
  .log-badge-error { background: #fee2e2; color: #991b1b; }
  :global([data-theme="dark"]) .log-badge-info { background: #1e3a5f; color: #93c5fd; }
  :global([data-theme="dark"]) .log-badge-warn { background: #422006; color: #fcd34d; }
  :global([data-theme="dark"]) .log-badge-error { background: #450a0a; color: #fca5a5; }
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
  .col-timestamp { width: 180px; }
  .col-level { width: 70px; }
  .col-component { width: 100px; }
  .logs-empty {
    text-align: center;
    padding: 24px;
    color: var(--text-muted);
  }
</style>
