<script>
  import { getNearDuplicates } from '../api.js';
  import { t } from '../i18n/index.svelte.js';

  let { sessionId, onerror, onnavigate } = $props();

  let pairs = $state([]);
  let total = $state(0);
  let loading = $state(false);
  let offset = $state(0);
  let hasMore = $state(false);
  let threshold = $state(3);
  const PAGE_SIZE = 50;

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
  </div>

  {#if loading}
    <div class="nd-empty">{t('common.loading')}</div>
  {:else if pairs.length === 0}
    <div class="nd-empty">{t('neardup.noPairs')}</div>
  {:else}
    <table class="nd-table">
      <thead>
        <tr>
          <th>{t('neardup.urlA')}</th>
          <th>{t('neardup.titleA')}</th>
          <th>{t('neardup.urlB')}</th>
          <th>{t('neardup.titleB')}</th>
          <th class="num">{t('neardup.wordCountA')}</th>
          <th class="num">{t('neardup.wordCountB')}</th>
          <th class="num">{t('neardup.similarity')}</th>
        </tr>
      </thead>
      <tbody>
        {#each pairs as p}
          <tr>
            <td class="cell-url"
              ><a
                href={`/sessions/${sessionId}/url/${encodeURIComponent(p.url_a)}`}
                onclick={(e) => {
                  e.preventDefault();
                  onnavigate?.(`/sessions/${sessionId}/url/${encodeURIComponent(p.url_a)}`);
                }}>{p.url_a}</a
              ></td
            >
            <td class="cell-title">{p.title_a || '-'}</td>
            <td class="cell-url"
              ><a
                href={`/sessions/${sessionId}/url/${encodeURIComponent(p.url_b)}`}
                onclick={(e) => {
                  e.preventDefault();
                  onnavigate?.(`/sessions/${sessionId}/url/${encodeURIComponent(p.url_b)}`);
                }}>{p.url_b}</a
              ></td
            >
            <td class="cell-title">{p.title_b || '-'}</td>
            <td class="num">{p.word_count_a}</td>
            <td class="num">{p.word_count_b}</td>
            <td class="num"
              ><span class="badge {similarityClass(p.similarity)}"
                >{(p.similarity * 100).toFixed(1)}%</span
              ></td
            >
          </tr>
        {/each}
      </tbody>
    </table>

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
    margin-bottom: 16px;
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
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }
  .nd-table tbody tr:hover {
    background: var(--bg-hover);
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
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
