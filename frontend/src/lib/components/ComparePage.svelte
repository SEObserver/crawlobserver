<script>
  import { getCompareStats, getComparePages, getCompareLinks } from '../api.js';
  import { fmtN, fmt, trunc, timeAgo } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { sessions = [], initialA = '', initialB = '', onerror, onnavigate } = $props();

  let sessionA = $state(initialA);
  let sessionB = $state(initialB);
  let activeTab = $state('stats');
  let loading = $state(false);

  // Stats
  let compareStats = $state(null);

  // Pages
  let pagesDiffType = $state('changed');
  let pagesResult = $state(null);
  let pagesOffset = $state(0);
  const PAGE_SIZE = 100;

  // Links
  let linksDiffType = $state('added');
  let linksResult = $state(null);
  let linksOffset = $state(0);

  function sessionLabel(s) {
    try {
      const host = new URL(s.SeedURLs?.[0] || 'https://unknown').hostname;
      const date = new Date(s.StartedAt).toLocaleDateString();
      return `${host} - ${date} (${t('sessions.pagesCount', { count: fmtN(s.PagesCrawled) })})`;
    } catch {
      return s.ID.slice(0, 8);
    }
  }

  async function doCompare() {
    if (!sessionA || !sessionB) return;
    loading = true;
    compareStats = null;
    pagesResult = null;
    linksResult = null;
    pagesOffset = 0;
    linksOffset = 0;

    try {
      compareStats = await getCompareStats(sessionA, sessionB);
      if (onnavigate) {
        const url = `/compare?a=${sessionA}&b=${sessionB}`;
        history.replaceState(null, '', url);
      }
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function loadPages() {
    if (!sessionA || !sessionB) return;
    loading = true;
    try {
      pagesResult = await getComparePages(sessionA, sessionB, pagesDiffType, PAGE_SIZE, pagesOffset);
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function loadLinks() {
    if (!sessionA || !sessionB) return;
    loading = true;
    try {
      linksResult = await getCompareLinks(sessionA, sessionB, linksDiffType, PAGE_SIZE, linksOffset);
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function switchMainTab(tab) {
    activeTab = tab;
    if (tab === 'pages' && !pagesResult) loadPages();
    if (tab === 'links' && !linksResult) loadLinks();
  }

  function switchPagesDiffType(type) {
    pagesDiffType = type;
    pagesOffset = 0;
    loadPages();
  }

  function switchLinksDiffType(type) {
    linksDiffType = type;
    linksOffset = 0;
    loadLinks();
  }

  // Auto-compare if both sessions are pre-filled
  $effect(() => {
    if (initialA && initialB && !compareStats) {
      sessionA = initialA;
      sessionB = initialB;
      doCompare();
    }
  });

  function delta(a, b) {
    const d = b - a;
    if (d === 0) return '';
    return d > 0 ? `+${fmtN(d)}` : fmtN(d);
  }

  function deltaClass(a, b) {
    const d = b - a;
    if (d > 0) return 'delta-up';
    if (d < 0) return 'delta-down';
    return '';
  }

  function cellChanged(a, b) {
    return a !== b ? 'cell-diff' : '';
  }
</script>

<div class="compare-page">
  <h2>{t('compare.title')}</h2>

  <div class="compare-selectors">
    <div class="selector-group">
      <label>{t('compare.sessionA')}</label>
      <select bind:value={sessionA}>
        <option value="">{t('compare.selectSession')}</option>
        {#each sessions as s}
          <option value={s.ID}>{sessionLabel(s)}</option>
        {/each}
      </select>
    </div>
    <div class="selector-swap">
      <button class="btn btn-sm" title={t('compare.swap')} onclick={() => { const tmp = sessionA; sessionA = sessionB; sessionB = tmp; compareStats = null; pagesResult = null; linksResult = null; }}>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/></svg>
      </button>
    </div>
    <div class="selector-group">
      <label>{t('compare.sessionB')}</label>
      <select bind:value={sessionB}>
        <option value="">{t('compare.selectSession')}</option>
        {#each sessions as s}
          <option value={s.ID}>{sessionLabel(s)}</option>
        {/each}
      </select>
    </div>
    <button class="btn btn-primary" onclick={doCompare} disabled={!sessionA || !sessionB || loading}>
      {loading ? t('compare.comparing') : t('sidebar.compare')}
    </button>
  </div>

  {#if compareStats}
    <div class="tab-bar compare-tab-bar">
      <button class="tab" class:tab-active={activeTab === 'stats'} onclick={() => switchMainTab('stats')}>{t('compare.stats')}</button>
      <button class="tab" class:tab-active={activeTab === 'pages'} onclick={() => switchMainTab('pages')}>{t('common.pages')}</button>
      <button class="tab" class:tab-active={activeTab === 'links'} onclick={() => switchMainTab('links')}>{t('compare.internalLinks')}</button>
    </div>

    <div class="card card-flush compare-card-flush">
      {#if activeTab === 'stats'}
        {@const sa = compareStats.stats_a}
        {@const sb = compareStats.stats_b}
        <div class="compare-stats-grid">
          <div class="compare-stat-card">
            <div class="compare-stat-label">{t('compare.totalPages')}</div>
            <div class="compare-stat-values">
              <span class="val-a">{fmtN(sa.total_pages)}</span>
              <span class="val-arrow">→</span>
              <span class="val-b">{fmtN(sb.total_pages)}</span>
            </div>
            <div class="compare-delta {deltaClass(sa.total_pages, sb.total_pages)}">{delta(sa.total_pages, sb.total_pages)}</div>
          </div>
          <div class="compare-stat-card">
            <div class="compare-stat-label">{t('compare.internalLinks')}</div>
            <div class="compare-stat-values">
              <span class="val-a">{fmtN(sa.internal_links)}</span>
              <span class="val-arrow">→</span>
              <span class="val-b">{fmtN(sb.internal_links)}</span>
            </div>
            <div class="compare-delta {deltaClass(sa.internal_links, sb.internal_links)}">{delta(sa.internal_links, sb.internal_links)}</div>
          </div>
          <div class="compare-stat-card">
            <div class="compare-stat-label">{t('session.externalLinks')}</div>
            <div class="compare-stat-values">
              <span class="val-a">{fmtN(sa.external_links)}</span>
              <span class="val-arrow">→</span>
              <span class="val-b">{fmtN(sb.external_links)}</span>
            </div>
            <div class="compare-delta {deltaClass(sa.external_links, sb.external_links)}">{delta(sa.external_links, sb.external_links)}</div>
          </div>
          <div class="compare-stat-card">
            <div class="compare-stat-label">{t('compare.errors')}</div>
            <div class="compare-stat-values">
              <span class="val-a">{fmtN(sa.error_count)}</span>
              <span class="val-arrow">→</span>
              <span class="val-b">{fmtN(sb.error_count)}</span>
            </div>
            <div class="compare-delta {deltaClass(sa.error_count, sb.error_count)}">{delta(sa.error_count, sb.error_count)}</div>
          </div>
          <div class="compare-stat-card">
            <div class="compare-stat-label">{t('compare.avgResponse')}</div>
            <div class="compare-stat-values">
              <span class="val-a">{fmt(Math.round(sa.avg_fetch_ms))}</span>
              <span class="val-arrow">→</span>
              <span class="val-b">{fmt(Math.round(sb.avg_fetch_ms))}</span>
            </div>
            <div class="compare-delta {deltaClass(sa.avg_fetch_ms, sb.avg_fetch_ms)}">{delta(Math.round(sa.avg_fetch_ms), Math.round(sb.avg_fetch_ms))}</div>
          </div>
          <div class="compare-stat-card">
            <div class="compare-stat-label">{t('compare.pagesPerSec')}</div>
            <div class="compare-stat-values">
              <span class="val-a">{sa.pages_per_second?.toFixed(1) || '0'}</span>
              <span class="val-arrow">→</span>
              <span class="val-b">{sb.pages_per_second?.toFixed(1) || '0'}</span>
            </div>
          </div>
        </div>

        {#if sa.status_codes || sb.status_codes}
          <h3 class="status-codes-heading">{t('compare.statusCodes')}</h3>
          {@const allCodes = [...new Set([...Object.keys(sa.status_codes || {}), ...Object.keys(sb.status_codes || {})])].sort()}
          <table class="table">
            <thead><tr><th>{t('compare.code')}</th><th>{t('compare.sessionA')}</th><th>{t('compare.sessionB')}</th><th>{t('compare.delta')}</th></tr></thead>
            <tbody>
              {#each allCodes as code}
                {@const countA = (sa.status_codes || {})[code] || 0}
                {@const countB = (sb.status_codes || {})[code] || 0}
                <tr>
                  <td><span class="badge">{code}</span></td>
                  <td>{fmtN(countA)}</td>
                  <td>{fmtN(countB)}</td>
                  <td class={deltaClass(countA, countB)}>{delta(countA, countB)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}

      {:else if activeTab === 'pages'}
        <div class="sub-tabs">
          <button class="sub-tab" class:sub-tab-active={pagesDiffType === 'changed'} onclick={() => switchPagesDiffType('changed')}>
            {t('compare.changed')} {#if pagesResult}<span class="badge-count">{fmtN(pagesResult.total_changed)}</span>{/if}
          </button>
          <button class="sub-tab" class:sub-tab-active={pagesDiffType === 'added'} onclick={() => switchPagesDiffType('added')}>
            {t('compare.added')} {#if pagesResult}<span class="badge-count">{fmtN(pagesResult.total_added)}</span>{/if}
          </button>
          <button class="sub-tab" class:sub-tab-active={pagesDiffType === 'removed'} onclick={() => switchPagesDiffType('removed')}>
            {t('compare.removed')} {#if pagesResult}<span class="badge-count">{fmtN(pagesResult.total_removed)}</span>{/if}
          </button>
        </div>

        {#if loading}
          <div class="loading-state">{t('common.loading')}</div>
        {:else if pagesResult && pagesResult.pages?.length > 0}
          <div class="table-scroll">
            <table class="table">
              <thead>
                <tr>
                  <th>URL</th>
                  {#if pagesDiffType === 'changed'}
                    <th>Status A</th><th>Status B</th>
                    <th>Title A</th><th>Title B</th>
                    <th>Words A</th><th>Words B</th>
                    <th>Depth A</th><th>Depth B</th>
                  {:else if pagesDiffType === 'added'}
                    <th>Status</th><th>Title</th><th>Words</th><th>Depth</th>
                  {:else}
                    <th>Status</th><th>Title</th><th>Words</th><th>Depth</th>
                  {/if}
                </tr>
              </thead>
              <tbody>
                {#each pagesResult.pages as p}
                  <tr>
                    <td class="cell-url">{p.url}</td>
                    {#if pagesDiffType === 'changed'}
                      <td class={cellChanged(p.status_code_a, p.status_code_b)}>{p.status_code_a}</td>
                      <td class={cellChanged(p.status_code_a, p.status_code_b)}>{p.status_code_b}</td>
                      <td class="cell-title {cellChanged(p.title_a, p.title_b)}">{trunc(p.title_a, 40)}</td>
                      <td class="cell-title {cellChanged(p.title_a, p.title_b)}">{trunc(p.title_b, 40)}</td>
                      <td class={cellChanged(p.word_count_a, p.word_count_b)}>{fmtN(p.word_count_a)}</td>
                      <td class={cellChanged(p.word_count_a, p.word_count_b)}>{fmtN(p.word_count_b)}</td>
                      <td class={cellChanged(p.depth_a, p.depth_b)}>{p.depth_a}</td>
                      <td class={cellChanged(p.depth_a, p.depth_b)}>{p.depth_b}</td>
                    {:else if pagesDiffType === 'added'}
                      <td>{p.status_code_b}</td>
                      <td class="cell-title">{trunc(p.title_b, 60)}</td>
                      <td>{fmtN(p.word_count_b)}</td>
                      <td>{p.depth_b}</td>
                    {:else}
                      <td>{p.status_code_a}</td>
                      <td class="cell-title">{trunc(p.title_a, 60)}</td>
                      <td>{fmtN(p.word_count_a)}</td>
                      <td>{p.depth_a}</td>
                    {/if}
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          <div class="pagination">
            <button class="btn btn-sm" disabled={pagesOffset === 0} onclick={() => { pagesOffset -= PAGE_SIZE; loadPages(); }}>{t('common.previous')}</button>
            <span>{t('common.showing', { start: pagesOffset + 1, end: pagesOffset + pagesResult.pages.length })}</span>
            <button class="btn btn-sm" disabled={pagesResult.pages.length < PAGE_SIZE} onclick={() => { pagesOffset += PAGE_SIZE; loadPages(); }}>{t('common.next')}</button>
          </div>
        {:else if pagesResult}
          <div class="empty-state">{t('compare.noPages', { type: pagesDiffType })}</div>
        {/if}

      {:else if activeTab === 'links'}
        <div class="sub-tabs">
          <button class="sub-tab" class:sub-tab-active={linksDiffType === 'added'} onclick={() => switchLinksDiffType('added')}>
            {t('compare.added')} {#if linksResult}<span class="badge-count">{fmtN(linksResult.total_added)}</span>{/if}
          </button>
          <button class="sub-tab" class:sub-tab-active={linksDiffType === 'removed'} onclick={() => switchLinksDiffType('removed')}>
            {t('compare.removed')} {#if linksResult}<span class="badge-count">{fmtN(linksResult.total_removed)}</span>{/if}
          </button>
        </div>

        {#if loading}
          <div class="loading-state">{t('common.loading')}</div>
        {:else if linksResult && linksResult.links?.length > 0}
          <div class="table-scroll">
            <table class="table">
              <thead><tr><th>{t('common.source')}</th><th>{t('common.target')}</th><th>{t('session.anchorText')}</th></tr></thead>
              <tbody>
                {#each linksResult.links as l}
                  <tr>
                    <td class="cell-url">{l.source_url}</td>
                    <td class="cell-url">{l.target_url}</td>
                    <td class="cell-title">{l.anchor_text || '-'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          <div class="pagination">
            <button class="btn btn-sm" disabled={linksOffset === 0} onclick={() => { linksOffset -= PAGE_SIZE; loadLinks(); }}>{t('common.previous')}</button>
            <span>{t('common.showing', { start: linksOffset + 1, end: linksOffset + linksResult.links.length })}</span>
            <button class="btn btn-sm" disabled={linksResult.links.length < PAGE_SIZE} onclick={() => { linksOffset += PAGE_SIZE; loadLinks(); }}>{t('common.next')}</button>
          </div>
        {:else if linksResult}
          <div class="empty-state">{t('compare.noLinks', { type: linksDiffType })}</div>
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .compare-page h2 {
    margin: 0 0 20px;
    font-size: 20px;
    font-weight: 600;
  }
  .compare-selectors {
    display: flex;
    gap: 12px;
    align-items: flex-end;
    flex-wrap: wrap;
  }
  .selector-group {
    flex: 1;
    min-width: 200px;
  }
  .selector-group label {
    display: block;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary);
    margin-bottom: 4px;
  }
  .selector-group select {
    width: 100%;
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
  }
  .selector-swap {
    display: flex;
    align-items: center;
    padding-bottom: 2px;
  }
  .compare-stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 12px;
    padding: 16px;
  }
  .compare-stat-card {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 14px;
    text-align: center;
  }
  .compare-stat-label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--text-secondary);
    margin-bottom: 8px;
  }
  .compare-stat-values {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    font-size: 18px;
    font-weight: 600;
  }
  .val-a { color: var(--text-secondary); }
  .val-arrow { color: var(--text-secondary); font-size: 14px; opacity: 0.5; }
  .val-b { color: var(--text); }
  .compare-delta {
    font-size: 13px;
    font-weight: 500;
    margin-top: 4px;
  }
  .delta-up { color: var(--success, #22c55e); }
  .delta-down { color: var(--error, #ef4444); }

  .sub-tabs {
    display: flex;
    gap: 0;
    padding: 12px 16px 0;
    border-bottom: 1px solid var(--border);
  }
  .sub-tab {
    padding: 8px 16px;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    font-size: 13px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .sub-tab-active {
    color: var(--accent);
    border-bottom-color: var(--accent);
    font-weight: 500;
  }
  .badge-count {
    background: var(--accent-light, rgba(124, 58, 237, 0.1));
    color: var(--accent);
    padding: 1px 7px;
    border-radius: 10px;
    font-size: 11px;
    font-weight: 600;
  }

  .table-scroll {
    overflow-x: auto;
    padding: 0;
  }
  .table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .table th {
    text-align: left;
    padding: 8px 12px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--text-secondary);
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }
  .table td {
    padding: 6px 12px;
    border-bottom: 1px solid var(--border);
    color: var(--text);
  }
  .cell-url {
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
  }
  .cell-title {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cell-diff {
    background: rgba(234, 179, 8, 0.1);
    font-weight: 500;
  }

  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    padding: 12px;
    font-size: 13px;
    color: var(--text-secondary);
  }
  .loading-state, .empty-state {
    padding: 32px;
    text-align: center;
    color: var(--text-secondary);
    font-size: 14px;
  }

  h3 {
    padding: 0 16px;
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
  }
  .compare-tab-bar {
    margin-top: 24px;
  }
  .compare-card-flush {
    border-top-left-radius: 0;
    border-top-right-radius: 0;
    border-top: none;
  }
  .status-codes-heading {
    margin: 20px 0 12px;
  }
</style>
