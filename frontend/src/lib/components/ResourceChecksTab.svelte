<script>
  import { getPageResourceChecks, getPageResourceChecksSummary } from '../api.js';

  let { sessionId, initialSubView = 'summary', initialFilters = {}, onpushurl, onerror } = $props();

  let view = $state(initialSubView); // 'summary' | 'urls'
  let summary = $state([]);
  let checks = $state([]);
  let loading = $state(false);
  let checksOffset = $state(0);
  let hasMoreChecks = $state(false);
  let urlFilters = $state({
    url: initialFilters.url || '',
    resource_type: initialFilters.resource_type || '',
    is_internal: initialFilters.is_internal || '',
    status_code: initialFilters.status_code || ''
  });
  const PAGE_SIZE = 100;

  function pushFilters() {
    const base = `/sessions/${sessionId}/resources/${view}`;
    const params = new URLSearchParams();
    if (view === 'urls') {
      if (urlFilters.url) params.set('url', urlFilters.url);
      if (urlFilters.resource_type) params.set('resource_type', urlFilters.resource_type);
      if (urlFilters.is_internal) params.set('is_internal', urlFilters.is_internal);
      if (urlFilters.status_code) params.set('status_code', urlFilters.status_code);
    }
    const qs = params.toString();
    onpushurl?.(qs ? `${base}?${qs}` : base);
  }

  async function loadSummary() {
    loading = true;
    try {
      const result = await getPageResourceChecksSummary(sessionId);
      summary = result || [];
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function loadChecks() {
    loading = true;
    try {
      const result = await getPageResourceChecks(sessionId, PAGE_SIZE, checksOffset, urlFilters);
      checks = result || [];
      hasMoreChecks = checks.length === PAGE_SIZE;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function switchToUrls(type) {
    urlFilters = type ? { url: '', resource_type: type, is_internal: '', status_code: '' } : { url: '', resource_type: '', is_internal: '', status_code: '' };
    checksOffset = 0;
    view = 'urls';
    pushFilters();
    loadChecks();
  }

  function switchToSummary() {
    view = 'summary';
    pushFilters();
    loadSummary();
  }

  function statusClass(code) {
    if (code === 0) return 'badge-dead';
    if (code >= 200 && code < 300) return 'badge-success';
    if (code >= 300 && code < 400) return 'badge-redirect';
    if (code >= 400 && code < 500) return 'badge-error';
    return 'badge-error';
  }

  function typeIcon(type) {
    switch (type) {
      case 'css': return 'CSS';
      case 'js': return 'JS';
      case 'font': return 'Font';
      case 'icon': return 'Icon';
      default: return type;
    }
  }

  $effect(() => {
    if (sessionId) {
      if (view === 'summary') loadSummary();
      else loadChecks();
    }
  });
</script>

<div class="res-checks">
  <div class="res-checks-header">
    <div class="res-checks-views">
      <button class="btn-view" class:active={view === 'summary'} onclick={switchToSummary}>Summary</button>
      <button class="btn-view" class:active={view === 'urls'} onclick={() => switchToUrls('')}>URLs</button>
    </div>
    {#if view === 'urls'}
      <input type="text" class="res-filter-input" placeholder="Filter URLs..." bind:value={urlFilters.url}
        onkeydown={(e) => { if (e.key === 'Enter') { checksOffset = 0; pushFilters(); loadChecks(); } }} />
      <select class="res-filter-select" bind:value={urlFilters.resource_type}
        onchange={() => { checksOffset = 0; pushFilters(); loadChecks(); }}>
        <option value="">All types</option>
        <option value="css">CSS</option>
        <option value="js">JS</option>
        <option value="font">Font</option>
        <option value="icon">Icon</option>
      </select>
      <select class="res-filter-select" bind:value={urlFilters.is_internal}
        onchange={() => { checksOffset = 0; pushFilters(); loadChecks(); }}>
        <option value="">All sources</option>
        <option value="true">Internal</option>
        <option value="false">Hotlink</option>
      </select>
      <select class="res-filter-select" bind:value={urlFilters.status_code}
        onchange={() => { checksOffset = 0; pushFilters(); loadChecks(); }}>
        <option value="">All status</option>
        <option value="0">Dead (0)</option>
        <option value="200-299">2xx OK</option>
        <option value="300-399">3xx Redirect</option>
        <option value="400-499">4xx Client Error</option>
        <option value=">=500">5xx Server Error</option>
      </select>
    {/if}
  </div>

  {#if loading}
    <div class="res-loading">Loading...</div>
  {:else if view === 'summary'}
    <table class="res-table">
      <thead>
        <tr>
          <th>Type</th>
          <th>Total</th>
          <th>Internal</th>
          <th>External (Hotlink)</th>
          <th>OK</th>
          <th>Errors</th>
          <th>Distribution</th>
        </tr>
      </thead>
      <tbody>
        {#each summary as s}
          <tr>
            <td><span class="badge badge-type">{typeIcon(s.resource_type)}</span></td>
            <td class="num"><button class="link-btn" onclick={() => switchToUrls(s.resource_type)}>{s.total}</button></td>
            <td class="num">{s.internal}</td>
            <td class="num">{#if s.external > 0}<span class="badge badge-hotlink">{s.external}</span>{:else}0{/if}</td>
            <td class="num">{s.ok}</td>
            <td class="num">{#if s.errors > 0}<span class="badge badge-error">{s.errors}</span>{:else}0{/if}</td>
            <td class="cell-bar">
              <div class="status-bar">
                {#if s.ok > 0}<div class="bar-ok" style="width: {s.total > 0 ? (s.ok / s.total * 100) : 0}%" title="{s.ok} OK"></div>{/if}
                {#if s.errors > 0}<div class="bar-err" style="width: {s.total > 0 ? (s.errors / s.total * 100) : 0}%" title="{s.errors} errors"></div>{/if}
              </div>
            </td>
          </tr>
        {/each}
        {#if summary.length === 0}
          <tr><td colspan="7" class="res-empty">No resource checks found</td></tr>
        {/if}
      </tbody>
    </table>

  {:else}
    <table class="res-table">
      <thead>
        <tr>
          <th>URL</th>
          <th>Type</th>
          <th>Source</th>
          <th>Status</th>
          <th>Content-Type</th>
          <th>Redirect</th>
          <th>Pages</th>
          <th>Time (ms)</th>
        </tr>
      </thead>
      <tbody>
        {#each checks as c}
          <tr>
            <td class="cell-url"><a href={c.url} target="_blank" rel="noopener">{c.url}</a></td>
            <td><span class="badge badge-type">{typeIcon(c.resource_type)}</span></td>
            <td>{#if c.is_internal}<span class="badge badge-internal">Internal</span>{:else}<span class="badge badge-hotlink">Hotlink</span>{/if}</td>
            <td><span class="badge {statusClass(c.status_code)}">{c.status_code || 'Dead'}</span></td>
            <td>{c.content_type || '-'}</td>
            <td class="cell-url">{c.redirect_url || '-'}</td>
            <td class="num">{c.page_count || 0}</td>
            <td class="num">{c.response_time_ms}</td>
          </tr>
        {/each}
        {#if checks.length === 0}
          <tr><td colspan="8" class="res-empty">No checks found</td></tr>
        {/if}
      </tbody>
    </table>

    <div class="res-pagination">
      <button disabled={checksOffset === 0} onclick={() => { checksOffset = Math.max(0, checksOffset - PAGE_SIZE); loadChecks(); }}>Previous</button>
      <span>{checksOffset + 1} - {checksOffset + checks.length}</span>
      <button disabled={!hasMoreChecks} onclick={() => { checksOffset += PAGE_SIZE; loadChecks(); }}>Next</button>
    </div>
  {/if}
</div>

<style>
  .res-checks { padding: 16px; }
  .res-checks-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
    flex-wrap: wrap;
  }
  .res-checks-views { display: flex; gap: 4px; }
  .btn-view {
    padding: 6px 14px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
  }
  .btn-view.active {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .res-filter-input {
    padding: 6px 10px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    font-size: 13px;
    min-width: 200px;
  }
  .res-filter-select {
    padding: 6px 10px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    font-size: 13px;
  }
  .res-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .res-table th {
    text-align: left;
    padding: 8px 10px;
    border-bottom: 2px solid var(--border);
    font-weight: 600;
    color: var(--text-muted);
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .res-table td {
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }
  .res-table tbody tr:hover { background: var(--bg-hover); }
  .cell-url {
    max-width: 400px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cell-url a { color: var(--accent); text-decoration: none; }
  .cell-url a:hover { text-decoration: underline; }
  .cell-bar { min-width: 120px; }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .link-btn {
    background: none;
    border: none;
    color: var(--accent);
    cursor: pointer;
    font-size: 13px;
    padding: 0;
    text-align: left;
  }
  .link-btn:hover { text-decoration: underline; }
  .badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 600;
  }
  .badge-type { background: #e0e7ff; color: #3730a3; }
  .badge-success { background: #dcfce7; color: #166534; }
  .badge-redirect { background: #fef9c3; color: #854d0e; }
  .badge-error { background: #fee2e2; color: #991b1b; }
  .badge-dead { background: #f3f4f6; color: #6b7280; }
  .badge-internal { background: #dcfce7; color: #166534; }
  .badge-hotlink { background: #ffedd5; color: #9a3412; }
  :global([data-theme="dark"]) .badge-type { background: #3730a3; color: #e0e7ff; }
  :global([data-theme="dark"]) .badge-success { background: #166534; color: #dcfce7; }
  :global([data-theme="dark"]) .badge-redirect { background: #854d0e; color: #fef9c3; }
  :global([data-theme="dark"]) .badge-error { background: #991b1b; color: #fee2e2; }
  :global([data-theme="dark"]) .badge-dead { background: #374151; color: #9ca3af; }
  :global([data-theme="dark"]) .badge-internal { background: #166534; color: #dcfce7; }
  :global([data-theme="dark"]) .badge-hotlink { background: #9a3412; color: #ffedd5; }
  .status-bar {
    display: flex;
    height: 14px;
    border-radius: 3px;
    overflow: hidden;
    background: var(--border);
  }
  .bar-ok { background: #22c55e; }
  .bar-err { background: #ef4444; }
  .res-pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-top: 16px;
    font-size: 13px;
  }
  .res-pagination button {
    padding: 4px 12px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
  }
  .res-pagination button:disabled { opacity: 0.4; cursor: default; }
  .res-loading, .res-empty {
    text-align: center;
    padding: 32px;
    color: var(--text-muted);
  }
</style>
