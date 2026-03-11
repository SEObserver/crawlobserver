<script>
  import { t } from '../i18n/index.svelte.js';
  import { getURLPatterns, getURLParams, getURLDirectories, getURLHosts } from '../api.js';

  let {
    sessionId,
    initialSubView = 'patterns',
    onpushurl,
    onerror,
    embedded = false,
  } = $props();

  let subView = $state(initialSubView);
  let loading = $state(false);
  let error = $state(null);

  // Data
  let patterns = $state(null);
  let params = $state(null);
  let directories = $state(null);
  let hosts = $state(null);

  // Controls
  let patternDepth = $state(2);
  let dirDepth = $state(2);
  let dirMinPages = $state(1);

  const SUB_VIEW_IDS = ['patterns', 'parameters', 'directories', 'hosts'];
  const SUB_VIEW_KEYS = {
    patterns: 'urlPatterns.patterns',
    parameters: 'urlPatterns.parameters',
    directories: 'urlPatterns.directories',
    hosts: 'urlPatterns.hosts',
  };

  function switchSubView(id) {
    subView = id;
    onpushurl?.(`/sessions/${sessionId}/urlpatterns/${id}`);
    loadData(id);
  }

  async function loadData(view) {
    if (view === 'patterns' && patterns) return;
    if (view === 'parameters' && params) return;
    if (view === 'directories' && directories) return;
    if (view === 'hosts' && hosts) return;
    await fetchView(view);
  }

  async function fetchView(view) {
    loading = true;
    error = null;
    try {
      if (view === 'patterns') {
        patterns = await getURLPatterns(sessionId, patternDepth);
      } else if (view === 'parameters') {
        params = await getURLParams(sessionId, 100);
      } else if (view === 'directories') {
        directories = await getURLDirectories(sessionId, dirDepth, dirMinPages);
      } else if (view === 'hosts') {
        hosts = await getURLHosts(sessionId);
      }
    } catch (e) {
      error = e.message;
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function reloadPatterns() {
    patterns = null;
    await fetchView('patterns');
  }

  async function reloadDirectories() {
    directories = null;
    await fetchView('directories');
  }

  function pct(n, total) {
    if (!total) return '0';
    return ((n / total) * 100).toFixed(1);
  }

  function fmtPR(v) {
    if (!v) return '—';
    return v.toFixed(6);
  }

  // Auto-load initial view
  loadData(subView);
</script>

<div class="pr-container">
  {#if !embedded}
    <div class="pr-subview-bar">
      {#each SUB_VIEW_IDS as id}
        <button
          class="pr-subview-btn"
          class:pr-subview-active={subView === id}
          onclick={() => switchSubView(id)}
        >
          {t(SUB_VIEW_KEYS[id])}
        </button>
      {/each}
    </div>
  {/if}

  {#if loading}
    <p class="reports-msg-muted">{t('common.loading')}</p>
  {:else if error}
    <p class="reports-msg-error">{error}</p>

  {:else if subView === 'patterns'}
    <div class="urlp-controls">
      <label class="urlp-label">
        {t('urlPatterns.depth')}
        <select bind:value={patternDepth} onchange={reloadPatterns}>
          {#each [1, 2, 3, 4, 5] as d}
            <option value={d}>{d}</option>
          {/each}
        </select>
      </label>
    </div>
    {#if patterns && patterns.length > 0}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{t('urlPatterns.pattern')}</th>
              <th class="num">{t('urlPatterns.total')}</th>
              <th class="num">{t('urlPatterns.indexable')}</th>
              <th class="num">{t('urlPatterns.nonIndexable')}</th>
              <th class="num">{t('urlPatterns.withParams')}</th>
              <th class="num">{t('urlPatterns.avgPR')}</th>
              <th style="min-width:180px">{t('urlPatterns.statusBreakdown')}</th>
            </tr>
          </thead>
          <tbody>
            {#each patterns as row}
              <tr>
                <td class="cell-url"><code>{row.pattern}</code></td>
                <td class="num">{row.total.toLocaleString()}</td>
                <td class="num">{row.indexable.toLocaleString()}</td>
                <td class="num">{row.non_indexable.toLocaleString()}</td>
                <td class="num">{row.with_params.toLocaleString()}</td>
                <td class="num">{fmtPR(row.avg_pagerank)}</td>
                <td>
                  <div class="urlp-status-bar">
                    {#if row.status_200}
                      <div class="urlp-bar-seg urlp-bar-200" style="width:{pct(row.status_200, row.total)}%" title="2xx: {row.status_200}"></div>
                    {/if}
                    {#if row.status_3xx}
                      <div class="urlp-bar-seg urlp-bar-3xx" style="width:{pct(row.status_3xx, row.total)}%" title="3xx: {row.status_3xx}"></div>
                    {/if}
                    {#if row.status_4xx}
                      <div class="urlp-bar-seg urlp-bar-4xx" style="width:{pct(row.status_4xx, row.total)}%" title="4xx: {row.status_4xx}"></div>
                    {/if}
                    {#if row.status_other}
                      <div class="urlp-bar-seg urlp-bar-other" style="width:{pct(row.status_other, row.total)}%" title="Other: {row.status_other}"></div>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="reports-msg-muted">{t('common.noData')}</p>
    {/if}

  {:else if subView === 'parameters'}
    {#if params && params.length > 0}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{t('urlPatterns.param')}</th>
              <th class="num">{t('urlPatterns.occurrences')}</th>
              <th class="num">{t('urlPatterns.uniqueURLs')}</th>
              <th class="num">{t('urlPatterns.indexable')}</th>
              <th class="num">{t('urlPatterns.nonIndexable')}</th>
              <th style="min-width:120px">{t('urlPatterns.indexableRatio')}</th>
            </tr>
          </thead>
          <tbody>
            {#each params as row}
              <tr>
                <td><code>{row.param}</code></td>
                <td class="num">{row.occurrences.toLocaleString()}</td>
                <td class="num">{row.unique_urls.toLocaleString()}</td>
                <td class="num">{row.indexable.toLocaleString()}</td>
                <td class="num">{row.non_indexable.toLocaleString()}</td>
                <td>
                  <div class="urlp-status-bar">
                    {#if row.indexable}
                      <div class="urlp-bar-seg urlp-bar-200" style="width:{pct(row.indexable, row.indexable + row.non_indexable)}%" title="Indexable: {row.indexable}"></div>
                    {/if}
                    {#if row.non_indexable}
                      <div class="urlp-bar-seg urlp-bar-4xx" style="width:{pct(row.non_indexable, row.indexable + row.non_indexable)}%" title="Non-indexable: {row.non_indexable}"></div>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="reports-msg-muted">{t('common.noData')}</p>
    {/if}

  {:else if subView === 'directories'}
    <div class="urlp-controls">
      <label class="urlp-label">
        {t('urlPatterns.depth')}
        <select bind:value={dirDepth} onchange={reloadDirectories}>
          {#each [1, 2, 3, 4, 5] as d}
            <option value={d}>{d}</option>
          {/each}
        </select>
      </label>
      <label class="urlp-label">
        {t('urlPatterns.minPages')}
        <select bind:value={dirMinPages} onchange={reloadDirectories}>
          {#each [1, 5, 10, 50, 100] as m}
            <option value={m}>{m}</option>
          {/each}
        </select>
      </label>
    </div>
    {#if directories && directories.length > 0}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{t('urlPatterns.directory')}</th>
              <th class="num">{t('urlPatterns.total')}</th>
              <th class="num">{t('urlPatterns.indexable')}</th>
              <th class="num">{t('urlPatterns.withParams')}</th>
              <th class="num">{t('urlPatterns.avgPR')}</th>
              <th class="num">{t('urlPatterns.errors')}</th>
            </tr>
          </thead>
          <tbody>
            {#each directories as row}
              <tr>
                <td class="cell-url"><code>{row.path}</code></td>
                <td class="num">{row.total.toLocaleString()}</td>
                <td class="num">{row.indexable.toLocaleString()}</td>
                <td class="num">{row.with_params.toLocaleString()}</td>
                <td class="num">{fmtPR(row.avg_pr)}</td>
                <td class="num">{row.errors.toLocaleString()}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="reports-msg-muted">{t('common.noData')}</p>
    {/if}

  {:else if subView === 'hosts'}
    {#if hosts && hosts.length > 0}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{t('urlPatterns.host')}</th>
              <th class="num">{t('urlPatterns.total')}</th>
              <th class="num">{t('urlPatterns.indexable')}</th>
              <th class="num">{t('urlPatterns.status200')}</th>
              <th class="num">{t('urlPatterns.errors')}</th>
              <th class="num">{t('urlPatterns.avgPR')}</th>
            </tr>
          </thead>
          <tbody>
            {#each hosts as row}
              <tr>
                <td><code>{row.host}</code></td>
                <td class="num">{row.total.toLocaleString()}</td>
                <td class="num">{row.indexable.toLocaleString()}</td>
                <td class="num">{row.status_200.toLocaleString()}</td>
                <td class="num">{row.errors.toLocaleString()}</td>
                <td class="num">{fmtPR(row.avg_pr)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="reports-msg-muted">{t('common.noData')}</p>
    {/if}
  {/if}
</div>

<style>
  .urlp-controls {
    display: flex;
    gap: 16px;
    margin-bottom: 12px;
    align-items: center;
  }
  .urlp-label {
    font-size: 13px;
    color: var(--text-muted);
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .urlp-label select {
    padding: 4px 8px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--text);
    font-size: 13px;
  }
  .urlp-status-bar {
    display: flex;
    height: 16px;
    border-radius: 3px;
    overflow: hidden;
    background: var(--bg-hover);
    min-width: 80px;
  }
  .urlp-bar-seg {
    height: 100%;
    min-width: 2px;
  }
  .urlp-bar-200 { background: #22c55e; }
  .urlp-bar-3xx { background: #3b82f6; }
  .urlp-bar-4xx { background: #ef4444; }
  .urlp-bar-other { background: #a3a3a3; }
  .num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
</style>
