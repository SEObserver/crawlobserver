<script>
  import { getPageResourceChecks, getPageResourceChecksSummary } from '../api.js';
  import { t } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';

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
    status_code: initialFilters.status_code || '',
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

  function switchToUrls(filters = {}) {
    urlFilters = {
      url: filters.url || '',
      resource_type: filters.resource_type || '',
      is_internal: filters.is_internal ?? '',
      status_code: filters.status_code || '',
    };
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
      case 'css':
        return t('resources.css');
      case 'js':
        return t('resources.js');
      case 'font':
        return t('resources.font');
      case 'icon':
        return t('resources.icon');
      default:
        return type;
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
    <div class="pr-subview-bar">
      <button
        class="pr-subview-btn"
        class:pr-subview-active={view === 'summary'}
        onclick={switchToSummary}>{t('resources.summary')}</button
      >
      <button
        class="pr-subview-btn"
        class:pr-subview-active={view === 'urls'}
        onclick={() => switchToUrls()}>{t('resources.urls')}</button
      >
    </div>
    {#if view === 'urls'}
      <input
        type="text"
        class="res-filter-input"
        placeholder={t('resources.filterUrls')}
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
        bind:value={urlFilters.resource_type}
        onchange={() => {
          checksOffset = 0;
          pushFilters();
          loadChecks();
        }}
        options={[
          { value: '', label: t('resources.allTypes') },
          { value: 'css', label: t('resources.css') },
          { value: 'js', label: t('resources.js') },
          { value: 'font', label: t('resources.font') },
          { value: 'icon', label: t('resources.icon') },
        ]}
      />
      <SearchSelect
        small
        bind:value={urlFilters.is_internal}
        onchange={() => {
          checksOffset = 0;
          pushFilters();
          loadChecks();
        }}
        options={[
          { value: '', label: t('resources.allSources') },
          { value: 'true', label: t('common.internal') },
          { value: 'false', label: t('resources.hotlink') },
        ]}
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
          { value: '', label: t('resources.allStatus') },
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
    <div class="res-loading">{t('common.loading')}</div>
  {:else if view === 'summary'}
    <table class="res-table">
      <thead>
        <tr>
          <th>{t('common.type')}</th>
          <th>{t('resources.total')}</th>
          <th>{t('common.internal')}</th>
          <th>{t('resources.externalHotlink')}</th>
          <th>{t('extChecks.ok')}</th>
          <th>{t('resources.errors')}</th>
          <th>{t('resources.distribution')}</th>
        </tr>
      </thead>
      <tbody>
        {#each summary as s}
          <tr>
            <td><span class="badge badge-type">{typeIcon(s.resource_type)}</span></td>
            <td class="num"
              ><button
                class="link-btn"
                onclick={() => switchToUrls({ resource_type: s.resource_type })}>{s.total}</button
              ></td
            >
            <td class="num">{s.internal}</td>
            <td class="num"
              >{#if s.external > 0}<button
                  class="link-btn-badge badge badge-hotlink"
                  onclick={() =>
                    switchToUrls({ resource_type: s.resource_type, is_internal: 'false' })}
                  >{s.external}</button
                >{:else}0{/if}</td
            >
            <td class="num">{s.ok}</td>
            <td class="num"
              >{#if s.errors > 0}<button
                  class="link-btn-badge badge badge-error"
                  onclick={() =>
                    switchToUrls({ resource_type: s.resource_type, status_code: '>=400' })}
                  >{s.errors}</button
                >{:else}0{/if}</td
            >
            <td class="cell-bar">
              <div class="status-bar">
                {#if s.ok > 0}<div
                    class="bar-ok"
                    style="width: {s.total > 0 ? (s.ok / s.total) * 100 : 0}%"
                    title="{s.ok} OK"
                  ></div>{/if}
                {#if s.errors > 0}<div
                    class="bar-err"
                    style="width: {s.total > 0 ? (s.errors / s.total) * 100 : 0}%"
                    title="{s.errors} errors"
                  ></div>{/if}
              </div>
            </td>
          </tr>
        {/each}
        {#if summary.length === 0}
          <tr><td colspan="7" class="res-empty">{t('resources.noChecks')}</td></tr>
        {/if}
      </tbody>
    </table>
  {:else}
    <table class="res-table">
      <thead>
        <tr>
          <th>{t('common.url')}</th>
          <th>{t('common.type')}</th>
          <th>{t('resources.source')}</th>
          <th>{t('common.status')}</th>
          <th>{t('extChecks.contentType')}</th>
          <th>{t('extChecks.redirect')}</th>
          <th>{t('common.pages')}</th>
          <th>{t('extChecks.timeMs')}</th>
        </tr>
      </thead>
      <tbody>
        {#each checks as c}
          <tr>
            <td class="cell-url"><a href={c.url} target="_blank" rel="noopener">{c.url}</a></td>
            <td><span class="badge badge-type">{typeIcon(c.resource_type)}</span></td>
            <td
              >{#if c.is_internal}<span class="badge badge-internal">{t('common.internal')}</span
                >{:else}<span class="badge badge-hotlink">{t('resources.hotlink')}</span>{/if}</td
            >
            <td
              ><span class="badge {statusClass(c.status_code)}"
                >{c.status_code || t('extChecks.deadLabel')}</span
              ></td
            >
            <td>{c.content_type || '-'}</td>
            <td class="cell-url">{c.redirect_url || '-'}</td>
            <td class="num">{c.page_count || 0}</td>
            <td class="num">{c.response_time_ms}</td>
          </tr>
        {/each}
        {#if checks.length === 0}
          <tr><td colspan="8" class="res-empty">{t('resources.noChecksFound')}</td></tr>
        {/if}
      </tbody>
    </table>

    <div class="res-pagination">
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
  .res-checks {
    padding: 24px;
  }
  .res-checks-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 24px;
    flex-wrap: wrap;
  }
  .res-checks-header :global(.pr-subview-bar) {
    margin-bottom: 0;
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
  .res-checks-header :global(.ss-wrap) {
    width: 150px;
    flex-shrink: 0;
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
  .res-table tbody tr:hover {
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
  .link-btn-badge {
    border: none;
    cursor: pointer;
    transition: filter 0.15s;
  }
  .link-btn-badge:hover {
    filter: brightness(0.9);
  }
  .badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 600;
  }
  .badge-type {
    background: #e0e7ff;
    color: #3730a3;
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
  .badge-internal {
    background: #dcfce7;
    color: #166534;
  }
  .badge-hotlink {
    background: #ffedd5;
    color: #9a3412;
  }
  :global([data-theme='dark']) .badge-type {
    background: #3730a3;
    color: #e0e7ff;
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
  :global([data-theme='dark']) .badge-internal {
    background: #166534;
    color: #dcfce7;
  }
  :global([data-theme='dark']) .badge-hotlink {
    background: #9a3412;
    color: #ffedd5;
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
  .bar-err {
    background: #ef4444;
  }
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
  .res-pagination button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .res-loading,
  .res-empty {
    text-align: center;
    padding: 32px;
    color: var(--text-muted);
  }
</style>
