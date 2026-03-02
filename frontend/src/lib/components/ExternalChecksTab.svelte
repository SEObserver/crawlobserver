<script>
  import { getExternalLinkChecks, getExternalLinkCheckDomains } from '../api.js';
  import { fetchAll, downloadCSV } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';

  let {
    sessionId,
    initialSubView = 'domains',
    initialFilters = {},
    basePath = null,
    onpushurl,
    onnavigate,
    onerror,
  } = $props();

  let view = $state(initialSubView); // 'domains' | 'urls'
  let domains = $state([]);
  let checks = $state([]);
  let loading = $state(false);
  let domainsOffset = $state(0);
  let checksOffset = $state(0);
  let hasMoreDomains = $state(false);
  let hasMoreChecks = $state(false);
  let domainFilter = $state(initialFilters.domain || '');
  let urlFilters = $state({
    url: initialFilters.url || '',
    status_code: initialFilters.status_code || '',
  });
  const PAGE_SIZE = 100;

  function pushFilters() {
    const base = `${basePath || `/sessions/${sessionId}/ext-checks`}/${view}`;
    const params = new URLSearchParams();
    if (view === 'domains' && domainFilter) params.set('domain', domainFilter);
    if (view === 'urls' && urlFilters.url) params.set('url', urlFilters.url);
    if (view === 'urls' && urlFilters.status_code)
      params.set('status_code', urlFilters.status_code);
    const qs = params.toString();
    onpushurl?.(qs ? `${base}?${qs}` : base);
  }

  async function loadDomains() {
    loading = true;
    try {
      const filters = domainFilter ? { domain: domainFilter } : {};
      const result = await getExternalLinkCheckDomains(
        sessionId,
        PAGE_SIZE,
        domainsOffset,
        filters,
      );
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
    return total > 0 ? (n / total) * 100 : 0;
  }

  let exporting = $state(false);

  async function handleExportCSV() {
    if (exporting) return;
    exporting = true;
    try {
      if (view === 'domains') {
        const allData = await fetchAll(
          (limit, offset) => getExternalLinkCheckDomains(sessionId, limit, offset, domainFilter ? { domain: domainFilter } : {}),
        );
        downloadCSV('external-checks-domains.csv',
          ['Domain', 'Total URLs', 'OK', 'Redirects', 'Client Errors', 'Server Errors', 'Unreachable', 'Avg Response (ms)'],
          ['domain', 'total_urls', 'ok', 'redirects', 'client_errors', 'server_errors', 'unreachable', 'avg_response_ms'],
          allData,
        );
      } else {
        const allData = await fetchAll(
          (limit, offset) => getExternalLinkChecks(sessionId, limit, offset, urlFilters),
        );
        downloadCSV('external-checks-urls.csv',
          ['URL', 'Status', 'Content Type', 'Redirect URL', 'Error', 'Time (ms)'],
          ['url', 'status_code', 'content_type', 'redirect_url', 'error', 'response_time_ms'],
          allData,
        );
      }
    } finally {
      exporting = false;
    }
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
      <button class="btn-view" class:active={view === 'domains'} onclick={switchToDomains}
        >{t('extChecks.domains')}</button
      >
      <button class="btn-view" class:active={view === 'urls'} onclick={() => switchToUrls('')}
        >{t('extChecks.urls')}</button
      >
    </div>
    <button class="btn btn-sm ext-export" onclick={handleExportCSV} disabled={exporting}>
      {#if exporting}
        <svg class="csv-spinner" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4m0 12v4m-7.07-3.93l2.83-2.83m8.48-8.48l2.83-2.83M2 12h4m12 0h4m-3.93 7.07l-2.83-2.83M7.76 7.76L4.93 4.93"/></svg>
        {t('common.exportingCsv')}
      {:else}
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        {t('common.exportCsv')}
      {/if}
    </button>
    {#if view === 'domains'}
      <input
        type="text"
        class="ext-filter-input"
        placeholder={t('extChecks.filterDomains')}
        bind:value={domainFilter}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            domainsOffset = 0;
            loadDomains();
          }
        }}
      />
    {:else}
      <input
        type="text"
        class="ext-filter-input"
        placeholder={t('extChecks.filterUrls')}
        bind:value={urlFilters.url}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            checksOffset = 0;
            pushFilters();
            loadChecks();
          }
        }}
      />
      <SearchSelect
        small
        bind:value={urlFilters.status_code}
        onchange={() => {
          checksOffset = 0;
          pushFilters();
          loadChecks();
        }}
        options={[
          { value: '', label: t('extChecks.allStatus') },
          { value: '0', label: t('extChecks.dead') },
          { value: '200-299', label: t('extChecks.ok2xx') },
          { value: '300-399', label: t('extChecks.redirect3xx') },
          { value: '400-499', label: t('extChecks.client4xx') },
          { value: '>=500', label: t('extChecks.server5xx') },
        ]}
      />
    {/if}
  </div>

  {#if loading}
    <div class="ext-loading">{t('common.loading')}</div>
  {:else if view === 'domains'}
    <table class="ext-table">
      <thead>
        <tr>
          <th>{t('extChecks.domain')}</th>
          <th>{t('extChecks.urls')}</th>
          <th>{t('extChecks.statusDist')}</th>
          <th>{t('extChecks.ok')}</th>
          <th>3xx</th>
          <th>4xx</th>
          <th>5xx</th>
          <th>Dead</th>
          <th>{t('extChecks.avgMs')}</th>
        </tr>
      </thead>
      <tbody>
        {#each domains as d}
          <tr>
            <td
              ><button class="link-btn" onclick={() => switchToUrls(d.domain)}>{d.domain}</button
              ></td
            >
            <td>{d.total_urls}</td>
            <td class="cell-bar">
              <div class="status-bar">
                {#if d.ok > 0}<div
                    class="bar-ok"
                    style="width: {pct(d.ok, maxCount(d))}%"
                    title="{d.ok} OK"
                  ></div>{/if}
                {#if d.redirects > 0}<div
                    class="bar-redirect"
                    style="width: {pct(d.redirects, maxCount(d))}%"
                    title="{d.redirects} redirects"
                  ></div>{/if}
                {#if d.client_errors > 0}<div
                    class="bar-client"
                    style="width: {pct(d.client_errors, maxCount(d))}%"
                    title="{d.client_errors} 4xx"
                  ></div>{/if}
                {#if d.server_errors > 0}<div
                    class="bar-server"
                    style="width: {pct(d.server_errors, maxCount(d))}%"
                    title="{d.server_errors} 5xx"
                  ></div>{/if}
                {#if d.unreachable > 0}<div
                    class="bar-dead"
                    style="width: {pct(d.unreachable, maxCount(d))}%"
                    title="{d.unreachable} dead"
                  ></div>{/if}
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
          <tr><td colspan="9" class="ext-empty">{t('extChecks.noChecks')}</td></tr>
        {/if}
      </tbody>
    </table>

    <div class="ext-pagination">
      <button
        disabled={domainsOffset === 0}
        onclick={() => {
          domainsOffset = Math.max(0, domainsOffset - PAGE_SIZE);
          loadDomains();
        }}>{t('common.previous')}</button
      >
      <span>{domainsOffset + 1} - {domainsOffset + domains.length}</span>
      <button
        disabled={!hasMoreDomains}
        onclick={() => {
          domainsOffset += PAGE_SIZE;
          loadDomains();
        }}>{t('common.next')}</button
      >
    </div>
  {:else}
    <table class="ext-table">
      <thead>
        <tr>
          <th>{t('common.url')}</th>
          <th>{t('common.status')}</th>
          <th>{t('extChecks.contentType')}</th>
          <th>{t('extChecks.redirect')}</th>
          <th>{t('common.error')}</th>
          <th>{t('extChecks.timeMs')}</th>
        </tr>
      </thead>
      <tbody>
        {#each checks as c}
          <tr class="clickable-row" onclick={() => viewSources(c.url)}>
            <td class="cell-url"
              ><a href={c.url} target="_blank" rel="noopener" onclick={(e) => e.stopPropagation()}
                >{c.url}</a
              ></td
            >
            <td
              ><span class="badge {statusClass(c.status_code)}"
                >{c.status_code || t('extChecks.deadLabel')}</span
              ></td
            >
            <td>{c.content_type || '-'}</td>
            <td class="cell-url">{c.redirect_url || '-'}</td>
            <td class="cell-error" title={c.error}>{c.error ? c.error.substring(0, 60) : '-'}</td>
            <td class="num">{c.response_time_ms}</td>
          </tr>
        {/each}
        {#if checks.length === 0}
          <tr><td colspan="6" class="ext-empty">{t('extChecks.noChecksFound')}</td></tr>
        {/if}
      </tbody>
    </table>

    <div class="ext-pagination">
      <button
        disabled={checksOffset === 0}
        onclick={() => {
          checksOffset = Math.max(0, checksOffset - PAGE_SIZE);
          loadChecks();
        }}>{t('common.previous')}</button
      >
      <span>{checksOffset + 1} - {checksOffset + checks.length}</span>
      <button
        disabled={!hasMoreChecks}
        onclick={() => {
          checksOffset += PAGE_SIZE;
          loadChecks();
        }}>{t('common.next')}</button
      >
    </div>
  {/if}
</div>

<style>
  .ext-checks {
    padding: 16px;
  }
  .ext-checks-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
    flex-wrap: wrap;
  }
  .ext-checks-views {
    display: flex;
    gap: 4px;
  }
  .ext-export {
    margin-left: auto;
  }
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
  .ext-checks-header :global(.ss-wrap) {
    width: 150px;
    flex-shrink: 0;
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
  .ext-table tbody tr:hover {
    background: var(--bg-hover);
  }
  .cell-url {
    max-width: 400px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cell-url a {
    color: var(--accent);
    text-decoration: none;
  }
  .cell-url a:hover {
    text-decoration: underline;
  }
  .cell-error {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-muted);
    font-size: 12px;
  }
  .cell-bar {
    min-width: 120px;
  }
  .num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .link-btn {
    background: none;
    border: none;
    color: var(--accent);
    cursor: pointer;
    font-size: 13px;
    padding: 0;
    text-align: left;
  }
  .link-btn:hover {
    text-decoration: underline;
  }
  .badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 600;
  }
  .badge-success {
    background: #dcfce7;
    color: #166534;
  }
  .badge-redirect {
    background: #fef9c3;
    color: #854d0e;
  }
  .badge-error {
    background: #fee2e2;
    color: #991b1b;
  }
  .badge-dead {
    background: #f3f4f6;
    color: #6b7280;
  }
  :global([data-theme='dark']) .badge-success {
    background: #166534;
    color: #dcfce7;
  }
  :global([data-theme='dark']) .badge-redirect {
    background: #854d0e;
    color: #fef9c3;
  }
  :global([data-theme='dark']) .badge-error {
    background: #991b1b;
    color: #fee2e2;
  }
  :global([data-theme='dark']) .badge-dead {
    background: #374151;
    color: #9ca3af;
  }
  .status-bar {
    display: flex;
    height: 14px;
    border-radius: 3px;
    overflow: hidden;
    background: var(--border);
  }
  .bar-ok {
    background: #22c55e;
  }
  .bar-redirect {
    background: #eab308;
  }
  .bar-client {
    background: #ef4444;
  }
  .bar-server {
    background: #dc2626;
  }
  .bar-dead {
    background: #9ca3af;
  }
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
  .ext-pagination button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .ext-loading,
  .ext-empty {
    text-align: center;
    padding: 32px;
    color: var(--text-muted);
  }
</style>
