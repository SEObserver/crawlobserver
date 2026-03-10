<script>
  import { getNearDuplicates, buildApiPath } from '../api.js';
  import { fetchAll, downloadCSV } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import ExportDropdown from './ExportDropdown.svelte';

  let { sessionId, onerror, onnavigate } = $props();

  let pairs = $state([]);
  let total = $state(0);
  let loading = $state(false);
  let offset = $state(0);
  let hasMore = $state(false);
  let threshold = $state(3);
  let filter = $state('action'); // 'all' | 'action' | 'canon'
  const PAGE_SIZE = 50;

  /**
   * A pair is "canonicalized" (handled) if both pages defer to another URL.
   * If at least one page is self-canonical or has no canonical,
   * there's a risk of duplicate indexing → action needed.
   */
  function isCanonicalized(p) {
    const ca = p.canonical_a || '';
    const cb = p.canonical_b || '';
    const aDefers = ca && ca !== p.url_a;
    const bDefers = cb && cb !== p.url_b;
    // both pages defer (to each other, to same URL, or to different URLs) → handled
    if (aDefers && bDefers) return true;
    // one points to the other explicitly
    if (ca === p.url_b || cb === p.url_a) return true;
    return false;
  }

  function canonicalLabel(canonical, selfUrl, otherUrl) {
    if (!canonical) return t('neardup.noCanonical');
    if (canonical === otherUrl) return t('neardup.crossCanonical');
    if (canonical === selfUrl) return t('neardup.selfCanonical');
    return canonical;
  }

  let filtered = $derived.by(() => {
    if (filter === 'all') return pairs;
    if (filter === 'action') return pairs.filter((p) => !isCanonicalized(p));
    return pairs.filter((p) => isCanonicalized(p));
  });

  let actionCount = $derived(pairs.filter((p) => !isCanonicalized(p)).length);
  let canonCount = $derived(pairs.filter((p) => isCanonicalized(p)).length);

  async function loadData() {
    loading = true;
    try {
      const result = await getNearDuplicates(sessionId, PAGE_SIZE, offset, threshold);
      pairs = result?.pairs || [];
      total = result?.total || 0;
      hasMore = pairs.length === PAGE_SIZE;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function onThresholdChange(e) {
    threshold = Number(e.target.value);
    offset = 0;
    loadData();
  }

  function similarityClass(sim) {
    if (sim >= 0.95) return 'badge-high';
    if (sim >= 0.8) return 'badge-medium';
    return 'badge-low';
  }

  let exporting = $state(false);

  async function handleExportCSV() {
    if (exporting) return;
    exporting = true;
    try {
      const allData = await fetchAll(
        (limit, offset) =>
          getNearDuplicates(sessionId, limit, offset, threshold).then((r) => r?.pairs || []),
        PAGE_SIZE,
      );
      downloadCSV(
        'near-duplicates.csv',
        ['URL A', 'Title A', 'Canonical A', 'URL B', 'Title B', 'Canonical B', 'Similarity'],
        ['url_a', 'title_a', 'canonical_a', 'url_b', 'title_b', 'canonical_b', 'similarity'],
        allData,
      );
    } finally {
      exporting = false;
    }
  }

  let apiPath = $derived(
    buildApiPath(`/sessions/${sessionId}/near-duplicates`, {
      limit: PAGE_SIZE,
      offset: 0,
      threshold,
    }),
  );

  $effect(() => {
    if (sessionId) loadData();
  });
</script>

<div class="nd-tab">
  <div class="nd-header">
    <label class="nd-threshold">
      {t('neardup.threshold')}
      <select value={threshold} onchange={onThresholdChange}>
        <option value={1}>1 (exact)</option>
        <option value={2}>2</option>
        <option value={3}>3</option>
        <option value={4}>4</option>
        <option value={5}>5 (loose)</option>
      </select>
    </label>
    {#if total > 0}
      <span class="nd-count">{total} {t('neardup.pairs')}</span>
    {/if}
    {#if pairs.length > 0}
      <div class="nd-export">
        <ExportDropdown onexportcsv={handleExportCSV} {exporting} {apiPath} />
      </div>
    {/if}
  </div>

  {#if loading}
    <div class="nd-empty">{t('common.loading')}</div>
  {:else if pairs.length === 0}
    <div class="nd-empty">{t('neardup.noPairs')}</div>
  {:else}
    <!-- Summary chips -->
    <div class="nd-summary">
      {#if actionCount > 0}
        <span class="nd-chip nd-chip-action"
          >{t('neardup.summaryAction', { count: actionCount })}</span
        >
      {/if}
      {#if canonCount > 0}
        <span class="nd-chip nd-chip-canon">{t('neardup.summaryCanon', { count: canonCount })}</span
        >
      {/if}
    </div>

    <!-- Filter tabs -->
    <div class="nd-filters">
      <button class="nd-filter" class:active={filter === 'all'} onclick={() => (filter = 'all')}>
        {t('neardup.filterAll')} ({pairs.length})
      </button>
      <button
        class="nd-filter"
        class:active={filter === 'action'}
        onclick={() => (filter = 'action')}
      >
        {t('neardup.filterAction')} ({actionCount})
      </button>
      <button
        class="nd-filter"
        class:active={filter === 'canon'}
        onclick={() => (filter = 'canon')}
      >
        {t('neardup.filterCanon')} ({canonCount})
      </button>
    </div>

    {#if filtered.length === 0}
      <div class="nd-empty">{t('neardup.noPairs')}</div>
    {:else}
      <table class="nd-table">
        <thead>
          <tr>
            <th>{t('neardup.urlA')}</th>
            <th>{t('neardup.canonical')}</th>
            <th>{t('neardup.urlB')}</th>
            <th>{t('neardup.canonical')}</th>
            <th class="num">{t('neardup.similarity')}</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as p}
            {@const canon = isCanonicalized(p)}
            <tr class:row-canon={canon}>
              <td class="cell-url">
                <a
                  href={`/sessions/${sessionId}/url/${encodeURIComponent(p.url_a)}`}
                  onclick={(e) => {
                    e.preventDefault();
                    onnavigate?.(`/sessions/${sessionId}/url/${encodeURIComponent(p.url_a)}`);
                  }}>{p.url_a}</a
                >
                {#if p.title_a}<div class="cell-title">{p.title_a}</div>{/if}
              </td>
              <td class="cell-canon">
                <span
                  class="canon-tag"
                  class:canon-ok={p.canonical_a === p.url_b}
                  class:canon-self={p.canonical_a === p.url_a || !p.canonical_a}
                >
                  {canonicalLabel(p.canonical_a, p.url_a, p.url_b)}
                </span>
              </td>
              <td class="cell-url">
                <a
                  href={`/sessions/${sessionId}/url/${encodeURIComponent(p.url_b)}`}
                  onclick={(e) => {
                    e.preventDefault();
                    onnavigate?.(`/sessions/${sessionId}/url/${encodeURIComponent(p.url_b)}`);
                  }}>{p.url_b}</a
                >
                {#if p.title_b}<div class="cell-title">{p.title_b}</div>{/if}
              </td>
              <td class="cell-canon">
                <span
                  class="canon-tag"
                  class:canon-ok={p.canonical_b === p.url_a}
                  class:canon-self={p.canonical_b === p.url_b || !p.canonical_b}
                >
                  {canonicalLabel(p.canonical_b, p.url_b, p.url_a)}
                </span>
              </td>
              <td class="num">
                <span
                  class="badge"
                  class:badge-high={!canon && p.similarity >= 0.95}
                  class:badge-medium={!canon && p.similarity >= 0.8 && p.similarity < 0.95}
                  class:badge-low={!canon && p.similarity < 0.8}
                  class:badge-muted={canon}
                >
                  {(p.similarity * 100).toFixed(1)}%
                </span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}

    <div class="nd-pagination">
      <button
        disabled={offset === 0}
        onclick={() => {
          offset = Math.max(0, offset - PAGE_SIZE);
          loadData();
        }}>{t('common.previous')}</button
      >
      <span>{offset + 1} - {offset + pairs.length}</span>
      <button
        disabled={!hasMore}
        onclick={() => {
          offset += PAGE_SIZE;
          loadData();
        }}>{t('common.next')}</button
      >
    </div>
  {/if}
</div>

<style>
  .nd-tab {
    padding: 16px;
  }
  .nd-header {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 12px;
  }
  .nd-threshold {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--fg);
  }
  .nd-threshold select {
    padding: 4px 8px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    font-size: 13px;
  }
  .nd-count {
    font-size: 13px;
    color: var(--text-muted);
  }
  .nd-export {
    margin-left: auto;
  }

  /* Summary chips */
  .nd-summary {
    display: flex;
    gap: 10px;
    margin-bottom: 12px;
  }
  .nd-chip {
    font-size: 12px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 6px;
  }
  .nd-chip-action {
    background: #fee2e2;
    color: #991b1b;
  }
  .nd-chip-canon {
    background: #dcfce7;
    color: #166534;
  }
  :global([data-theme='dark']) .nd-chip-action {
    background: #991b1b;
    color: #fee2e2;
  }
  :global([data-theme='dark']) .nd-chip-canon {
    background: #166534;
    color: #dcfce7;
  }

  /* Filter tabs */
  .nd-filters {
    display: flex;
    gap: 4px;
    margin-bottom: 16px;
    border-bottom: 2px solid var(--border);
    padding-bottom: 0;
  }
  .nd-filter {
    background: none;
    border: none;
    padding: 6px 14px;
    font-size: 13px;
    color: var(--text-muted);
    cursor: pointer;
    border-bottom: 2px solid transparent;
    margin-bottom: -2px;
    transition:
      color 0.15s,
      border-color 0.15s;
  }
  .nd-filter:hover {
    color: var(--fg);
  }
  .nd-filter.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }

  /* Table */
  .nd-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .nd-table th {
    text-align: left;
    padding: 8px 10px;
    border-bottom: 2px solid var(--border);
    font-weight: 600;
    color: var(--text-muted);
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .nd-table td {
    padding: 8px 10px;
    border-bottom: 1px solid var(--border);
    vertical-align: top;
  }
  .nd-table tbody tr:hover {
    background: var(--bg-hover);
  }
  .row-canon {
    opacity: 0.65;
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
  .cell-title {
    font-size: 11px;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-top: 2px;
  }
  .cell-canon {
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .canon-tag {
    font-size: 11px;
    padding: 1px 6px;
    border-radius: 4px;
    white-space: nowrap;
  }
  .canon-ok {
    background: #dcfce7;
    color: #166534;
  }
  .canon-self {
    background: #fef3c7;
    color: #92400e;
  }
  :global([data-theme='dark']) .canon-ok {
    background: #166534;
    color: #dcfce7;
  }
  :global([data-theme='dark']) .canon-self {
    background: #92400e;
    color: #fef3c7;
  }
  .num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 600;
  }
  .badge-high {
    background: #fee2e2;
    color: #991b1b;
  }
  .badge-medium {
    background: #fef9c3;
    color: #854d0e;
  }
  .badge-low {
    background: #dcfce7;
    color: #166534;
  }
  .badge-muted {
    background: var(--bg-secondary);
    color: var(--text-muted);
  }
  :global([data-theme='dark']) .badge-high {
    background: #991b1b;
    color: #fee2e2;
  }
  :global([data-theme='dark']) .badge-medium {
    background: #854d0e;
    color: #fef9c3;
  }
  :global([data-theme='dark']) .badge-low {
    background: #166534;
    color: #dcfce7;
  }
  .nd-pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-top: 16px;
    font-size: 13px;
  }
  .nd-pagination button {
    padding: 4px 12px;
    border: 1px solid var(--border);
    background: var(--bg-card);
    color: var(--fg);
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
  }
  .nd-pagination button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .nd-empty {
    text-align: center;
    padding: 32px;
    color: var(--text-muted);
  }
</style>
