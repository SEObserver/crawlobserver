<script>
  import { getExternalLinkChecks, getExternalLinkCheckDomains } from '../api.js';
  import { fetchAll, downloadCSV } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';
  import DataTable from './DataTable.svelte';

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
  let sortColumn = $state('');
  let sortOrder = $state('');

  let domainFilters = $state({
    domain: initialFilters.domain || '',
  });
  let urlFilters = $state({
    url: initialFilters.url || '',
    status_code: initialFilters.status_code || '',
    error: initialFilters.error || '',
    source_url: initialFilters.source_url || '',
  });
  const PAGE_SIZE = 100;

  function pushFilters() {
    const base = `${basePath || `/sessions/${sessionId}/ext-checks`}/${view}`;
    const params = new URLSearchParams();
    const f = view === 'domains' ? domainFilters : urlFilters;
    for (const [k, v] of Object.entries(f)) {
      if (v) params.set(k, v);
    }
    const qs = params.toString();
    onpushurl?.(qs ? `${base}?${qs}` : base);
  }

  async function loadDomains() {
    loading = true;
    try {
      const result = await getExternalLinkCheckDomains(
        sessionId,
        PAGE_SIZE,
        domainsOffset,
        domainFilters,
        sortColumn,
        sortOrder,
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
      const result = await getExternalLinkChecks(
        sessionId,
        PAGE_SIZE,
        checksOffset,
        urlFilters,
        sortColumn,
        sortOrder,
      );
      checks = result || [];
      hasMoreChecks = checks.length === PAGE_SIZE;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function loadData() {
    if (view === 'domains') loadDomains();
    else loadChecks();
  }

  function switchToUrls(domain) {
    urlFilters = domain
      ? { url: domain, status_code: '', error: '', source_url: '' }
      : { url: '', status_code: '', error: '', source_url: '' };
    checksOffset = 0;
    sortColumn = '';
    sortOrder = '';
    view = 'urls';
    pushFilters();
    loadChecks();
  }

  function switchToDomains() {
    domainsOffset = 0;
    sortColumn = '';
    sortOrder = '';
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

  function handleSort(col, ord) {
    sortColumn = col;
    sortOrder = ord;
    if (view === 'domains') domainsOffset = 0;
    else checksOffset = 0;
    loadData();
  }

  function setFilter(key, val) {
    if (view === 'domains') {
      domainFilters[key] = val;
      domainFilters = { ...domainFilters };
    } else {
      urlFilters[key] = val;
      urlFilters = { ...urlFilters };
    }
  }

  function applyFilters() {
    if (view === 'domains') domainsOffset = 0;
    else checksOffset = 0;
    pushFilters();
    loadData();
  }

  function clearFilters() {
    if (view === 'domains') {
      domainFilters = { domain: '' };
      domainsOffset = 0;
    } else {
      urlFilters = { url: '', status_code: '', error: '', source_url: '' };
      checksOffset = 0;
    }
    pushFilters();
    loadData();
  }

  function hasActiveFilters() {
    const f = view === 'domains' ? domainFilters : urlFilters;
    return Object.values(f).some((v) => v && v !== '');
  }

  let exporting = $state(false);

  async function handleExportCSV() {
    if (exporting) return;
    exporting = true;
    try {
      if (view === 'domains') {
        const allData = await fetchAll((limit, offset) =>
          getExternalLinkCheckDomains(sessionId, limit, offset, domainFilters, sortColumn, sortOrder),
        );
        downloadCSV(
          'external-checks-domains.csv',
          [
            'Domain',
            'Total URLs',
            'OK',
            'Redirects',
            'Client Errors',
            'Server Errors',
            'Unreachable',
            'Avg Response (ms)',
          ],
          [
            'domain',
            'total_urls',
            'ok',
            'redirects',
            'client_errors',
            'server_errors',
            'unreachable',
            'avg_response_ms',
          ],
          allData,
        );
      } else {
        const allData = await fetchAll((limit, offset) =>
          getExternalLinkChecks(sessionId, limit, offset, urlFilters, sortColumn, sortOrder),
        );
        downloadCSV(
          'external-checks-urls.csv',
          [
            'URL',
            'Status',
            'Content Type',
            'Redirect URL',
            'Error',
            'Time (ms)',
            'Source Page',
            'Source PR',
            'Source Depth',
          ],
          [
            'url',
            'status_code',
            'content_type',
            'redirect_url',
            'error',
            'response_time_ms',
            'source_url',
            'source_pagerank',
            'source_depth',
          ],
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
      {/if}
    </button>
  </div>

  {#if loading}
    <div class="ext-loading">{t('common.loading')}</div>
  {:else if view === 'domains'}
    <DataTable
      columns={[
        { label: t('extChecks.domain'), sortKey: 'domain' },
        { label: t('extChecks.urls'), sortKey: 'total_urls' },
        { label: t('extChecks.statusDist') },
        { label: t('extChecks.ok'), sortKey: 'ok', class: 'num' },
        { label: '3xx', sortKey: 'redirects', class: 'num' },
        { label: '4xx', sortKey: 'client_errors', class: 'num' },
        { label: '5xx', sortKey: 'server_errors', class: 'num' },
        { label: 'Dead', sortKey: 'unreachable', class: 'num' },
        { label: t('extChecks.avgMs'), sortKey: 'avg_response_ms', class: 'num' },
      ]}
      filterKeys={['domain', null, null, null, null, null, null, 'unreachable', null]}
      filters={domainFilters}
      data={domains}
      offset={domainsOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMoreDomains}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={() => {
        domainsOffset += PAGE_SIZE;
        loadDomains();
      }}
      onprevpage={() => {
        domainsOffset = Math.max(0, domainsOffset - PAGE_SIZE);
        loadDomains();
      }}
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(d)}
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
      {/snippet}
    </DataTable>
  {:else}
    <DataTable
      columns={[
        { label: t('common.url'), sortKey: 'url' },
        { label: t('common.status'), sortKey: 'status_code' },
        { label: t('extChecks.contentType'), sortKey: 'content_type' },
        { label: t('extChecks.redirect') },
        { label: t('common.error'), sortKey: 'error' },
        { label: t('extChecks.timeMs'), sortKey: 'response_time_ms', class: 'num' },
        { label: t('extChecks.sourcePage'), sortKey: 'source_url' },
        { label: t('extChecks.sourcePr'), sortKey: 'source_pagerank', class: 'num' },
        { label: t('extChecks.sourceDepth'), sortKey: 'source_depth', class: 'num' },
      ]}
      filterKeys={['url', 'status_code', null, null, 'error', null, 'source_url', null, null]}
      filters={urlFilters}
      data={checks}
      offset={checksOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMoreChecks}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={() => {
        checksOffset += PAGE_SIZE;
        loadChecks();
      }}
      onprevpage={() => {
        checksOffset = Math.max(0, checksOffset - PAGE_SIZE);
        loadChecks();
      }}
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(c)}
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
          <td class="cell-url"
            >{#if c.source_url}<a
                href={`/sessions/${sessionId}/url/${encodeURIComponent(c.source_url)}`}
                onclick={(e) => e.stopPropagation()}>{c.source_url}</a
              >{:else}-{/if}</td
          >
          <td class="num text-accent font-medium"
            >{c.source_pagerank > 0 ? c.source_pagerank.toFixed(1) : '-'}</td
          >
          <td class="num">{c.source_depth || '-'}</td>
        </tr>
      {/snippet}
    </DataTable>
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
  .cell-url {
    max-width: 300px;
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
  .ext-loading {
    text-align: center;
    padding: 32px;
    color: var(--text-muted);
  }
</style>
