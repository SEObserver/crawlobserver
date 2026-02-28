<script>
  import { getExternalLinkChecks, getExternalLinkCheckDomains } from '../api.js';

  let { sessionId, initialSubView = 'domains', initialFilters = {}, onpushurl, onnavigate, onerror } = $props();

  let view = $state(initialSubView); // 'domains' | 'urls'
  let domains = $state([]);
  let checks = $state([]);
  let loading = $state(false);
  let domainsOffset = $state(0);
  let checksOffset = $state(0);
  let hasMoreDomains = $state(false);
  let hasMoreChecks = $state(false);
  let domainFilter = $state(initialFilters.domain || '');
  let urlFilters = $state({ url: initialFilters.url || '', status_code: initialFilters.status_code || '' });
  const PAGE_SIZE = 100;

  function pushFilters() {
    const base = `/sessions/${sessionId}/ext-checks/${view}`;
    const params = new URLSearchParams();
    if (view === 'domains' && domainFilter) params.set('domain', domainFilter);
    if (view === 'urls' && urlFilters.url) params.set('url', urlFilters.url);
    if (view === 'urls' && urlFilters.status_code) params.set('status_code', urlFilters.status_code);
    const qs = params.toString();
    onpushurl?.(qs ? `${base}?${qs}` : base);
  }

  async function loadDomains() {
    loading = true;
    try {
      const filters = domainFilter ? { domain: domainFilter } : {};
      const result = await getExternalLinkCheckDomains(sessionId, PAGE_SIZE, domainsOffset, filters);
      domains = result || [];
      hasMoreDomains = domains.length === PAGE_SIZE;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function loadChecks() {
    loading = true;
    try {
      const result = await getExternalLinkChecks(sessionId, PAGE_SIZE, checksOffset, urlFilters);
      checks = result || [];
      hasMoreChecks = checks.length === PAGE_SIZE;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function switchToUrls(domain) {
    urlFilters = domain ? { url: domain } : { url: '', status_code: '' };
    checksOffset = 0;
    view = 'urls';
    pushFilters();
    loadChecks();
  }

  function switchToDomains() {
    domainsOffset = 0;
    view = 'domains';
    pushFilters();
    loadDomains();
  }

  function viewSources(extUrl) {
    onnavigate?.('external', { target_url: extUrl });
  }

  function statusClass(code) {
    if (code === 0) return 'badge-dead';
    if (code >= 200 && code < 300) return 'badge-success';
    if (code >= 300 && code < 400) return 'badge-redirect';
    if (code >= 400 && code < 500) return 'badge-error';
    return 'badge-error';
  }

  function maxCount(d) {
    return d.ok + d.redirects + d.client_errors + d.server_errors + d.unreachable;
  }

  function pct(n, total) {
    return total > 0 ? (n / total * 100) : 0;
  }

  $effect(() => {
    if (sessionId) {
      if (view === 'domains') loadDomains();
      else loadChecks();
    }
  });
</script>

<div class="ext-checks">
  <div class="ext-checks-header">
    <div class="ext-checks-views">
      <button class="btn-view" class:active={view === 'domains'} onclick={switchToDomains}>Domains</button>
      <button class="btn-view" class:active={view === 'urls'} onclick={() => switchToUrls('')}>URLs</button>
    </div>
    {#if view === 'domains'}
      <input type="text" class="ext-filter-input" placeholder="Filter domains..." bind:value={domainFilter}
        onkeydown={(e) => { if (e.key === 'Enter') { domainsOffset = 0; loadDomains(); } }} />
    {:else}
      <input type="text" class="ext-filter-input" placeholder="Filter URLs..." bind:value={urlFilters.url}
        onkeydown={(e) => { if (e.key === 'Enter') { checksOffset = 0; pushFilters(); loadChecks(); } }} />
      <select class="ext-filter-select" bind:value={urlFilters.status_code}
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
    <div class="ext-loading">Loading...</div>
  {:else if view === 'domains'}
    <table class="ext-table">
      <thead>
        <tr>
          <th>Domain</th>
          <th>URLs</th>
          <th>Status Distribution</th>
          <th>OK</th>
          <th>3xx</th>
          <th>4xx</th>
          <th>5xx</th>
          <th>Dead</th>
          <th>Avg ms</th>
        </tr>
      </thead>
      <tbody>
        {#each domains as d}
          <tr>
            <td><button class="link-btn" onclick={() => switchToUrls(d.domain)}>{d.domain}</button></td>
            <td>{d.total_urls}</td>
            <td class="cell-bar">
              <div class="status-bar">
                {#if d.ok > 0}<div class="bar-ok" style="width: {pct(d.ok, maxCount(d))}%" title="{d.ok} OK"></div>{/if}
                {#if d.redirects > 0}<div class="bar-redirect" style="width: {pct(d.redirects, maxCount(d))}%" title="{d.redirects} redirects"></div>{/if}
                {#if d.client_errors > 0}<div class="bar-client" style="width: {pct(d.client_errors, maxCount(d))}%" title="{d.client_errors} 4xx"></div>{/if}
                {#if d.server_errors > 0}<div class="bar-server" style="width: {pct(d.server_errors, maxCount(d))}%" title="{d.server_errors} 5xx"></div>{/if}
                {#if d.unreachable > 0}<div class="bar-dead" style="width: {pct(d.unreachable, maxCount(d))}%" title="{d.unreachable} dead"></div>{/if}
              </div>
            </td>
            <td class="num">{d.ok}</td>
            <td class="num">{d.redirects}</td>
            <td class="num">{d.client_errors}</td>
            <td class="num">{d.server_errors}</td>
            <td class="num">{d.unreachable}</td>
            <td class="num">{d.avg_response_ms}</td>
          </tr>
        {/each}
        {#if domains.length === 0}
          <tr><td colspan="9" class="ext-empty">No external link checks found</td></tr>
        {/if}
      </tbody>
    </table>

    <div class="ext-pagination">
      <button disabled={domainsOffset === 0} onclick={() => { domainsOffset = Math.max(0, domainsOffset - PAGE_SIZE); loadDomains(); }}>Previous</button>
      <span>{domainsOffset + 1} - {domainsOffset + domains.length}</span>
      <button disabled={!hasMoreDomains} onclick={() => { domainsOffset += PAGE_SIZE; loadDomains(); }}>Next</button>
    </div>

  {:else}
    <table class="ext-table">
      <thead>
        <tr>
          <th>URL</th>
          <th>Status</th>
          <th>Content-Type</th>
          <th>Redirect</th>
          <th>Error</th>
          <th>Time (ms)</th>
          <th>Sources</th>
        </tr>
      </thead>
      <tbody>
        {#each checks as c}
          <tr>
            <td class="cell-url"><a href={c.url} target="_blank" rel="noopener">{c.url}</a></td>
            <td><span class="badge {statusClass(c.status_code)}">{c.status_code || 'Dead'}</span></td>
            <td>{c.content_type || '-'}</td>
            <td class="cell-url">{c.redirect_url || '-'}</td>
            <td class="cell-error" title={c.error}>{c.error ? c.error.substring(0, 60) : '-'}</td>
            <td class="num">{c.response_time_ms}</td>
            <td><button class="link-btn" onclick={() => viewSources(c.url)} title="View pages linking to this URL">View</button></td>
          </tr>
        {/each}
        {#if checks.length === 0}
          <tr><td colspan="7" class="ext-empty">No checks found</td></tr>
        {/if}
      </tbody>
    </table>

    <div class="ext-pagination">
      <button disabled={checksOffset === 0} onclick={() => { checksOffset = Math.max(0, checksOffset - PAGE_SIZE); loadChecks(); }}>Previous</button>
      <span>{checksOffset + 1} - {checksOffset + checks.length}</span>
      <button disabled={!hasMoreChecks} onclick={() => { checksOffset += PAGE_SIZE; loadChecks(); }}>Next</button>
    </div>
  {/if}
</div>

<style>
  .ext-checks { padding: 16px; }
  .ext-checks-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
    flex-wrap: wrap;
  }
  .ext-checks-views { display: flex; gap: 4px; }
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
  .ext-filter-input {
    padding: 6px 10px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    font-size: 13px;
    min-width: 200px;
  }
  .ext-filter-select {
    padding: 6px 10px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    font-size: 13px;
  }
  .ext-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .ext-table th {
    text-align: left;
    padding: 8px 10px;
    border-bottom: 2px solid var(--border);
    font-weight: 600;
    color: var(--text-muted);
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .ext-table td {
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }
  .ext-table tbody tr:hover { background: var(--bg-hover); }
  .cell-url {
    max-width: 400px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cell-url a { color: var(--accent); text-decoration: none; }
  .cell-url a:hover { text-decoration: underline; }
  .cell-error {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-muted);
    font-size: 12px;
  }
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
  .badge-success { background: #dcfce7; color: #166534; }
  .badge-redirect { background: #fef9c3; color: #854d0e; }
  .badge-error { background: #fee2e2; color: #991b1b; }
  .badge-dead { background: #f3f4f6; color: #6b7280; }
  :global([data-theme="dark"]) .badge-success { background: #166534; color: #dcfce7; }
  :global([data-theme="dark"]) .badge-redirect { background: #854d0e; color: #fef9c3; }
  :global([data-theme="dark"]) .badge-error { background: #991b1b; color: #fee2e2; }
  :global([data-theme="dark"]) .badge-dead { background: #374151; color: #9ca3af; }
  .status-bar {
    display: flex;
    height: 14px;
    border-radius: 3px;
    overflow: hidden;
    background: var(--border);
  }
  .bar-ok { background: #22c55e; }
  .bar-redirect { background: #eab308; }
  .bar-client { background: #ef4444; }
  .bar-server { background: #dc2626; }
  .bar-dead { background: #9ca3af; }
  .ext-pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-top: 16px;
    font-size: 13px;
  }
  .ext-pagination button {
    padding: 4px 12px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
  }
  .ext-pagination button:disabled { opacity: 0.4; cursor: default; }
  .ext-loading, .ext-empty {
    text-align: center;
    padding: 32px;
    color: var(--text-muted);
  }
</style>
