<script>
  import { fmtN, a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import Pagination from './Pagination.svelte';
  import SearchSelect from './SearchSelect.svelte';

  let {
    data,
    offset = 0,
    limit = 50,
    onnavigate,
    onlimitchange,
    onpagechange,
    ontooltip,
  } = $props();

  function goToUrlDetail(url) {
    onnavigate?.(url);
  }
</script>

{#if data?.pages?.length > 0}
  <div class="pr-controls">
    <label>{t('pagerank.show')}</label>
    <SearchSelect
      small
      value={limit}
      onchange={(v) => onlimitchange?.(Number(v))}
      options={[
        { value: 20, label: '20' },
        { value: 50, label: '50' },
        { value: 100, label: '100' },
      ]}
    />
    <span class="text-muted text-xs"
      >{t('pagerank.ofPagesWithPR', { total: fmtN(data.total) })}</span
    >
  </div>
  {@const maxPR = data.pages[0]?.pagerank || 1}
  {#each data.pages as p, i}
    <div
      class="pr-top-row pr-top-row-clickable"
      role="button"
      tabindex="0"
      onclick={() => goToUrlDetail(p.url)}
      onkeydown={a11yKeydown(() => goToUrlDetail(p.url))}
      onmouseenter={(e) =>
        ontooltip?.({
          x: e.clientX,
          y: e.clientY,
          url: p.url,
          pr: p.pagerank,
          depth: p.depth,
          intLinks: p.internal_links_out,
          extLinks: p.external_links_out,
          words: p.word_count,
        })}
      onmouseleave={() => ontooltip?.(null)}
    >
      <span class="pr-top-rank">{offset + i + 1}</span>
      <span class="pr-top-url">{p.url.replace(/^https?:\/\/[^/]+/, '') || '/'}</span>
      <div>
        <div
          class="pr-top-bar"
          style="width: {(p.pagerank / maxPR) * 100}%; opacity: {0.4 + 0.6 * (p.pagerank / maxPR)};"
        ></div>
      </div>
      <span class="pr-top-score">{p.pagerank.toFixed(1)}</span>
      <div class="pr-top-badges">
        <span class="pr-top-badge">D{p.depth}</span>
        <span class="pr-top-badge">{p.internal_links_out}int</span>
      </div>
    </div>
  {/each}
  <Pagination {offset} {limit} total={data.total} onchange={(o) => onpagechange?.(o)} />
{:else}
  <p class="chart-empty">{t('pagerank.noData')}</p>
{/if}

<style>
  .pr-controls {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
    flex-wrap: wrap;
  }
  .pr-controls label {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
  }
  .pr-controls :global(.ss-wrap) {
    width: 80px;
    flex-shrink: 0;
  }
  .pr-top-row {
    display: grid;
    grid-template-columns: 36px 1fr 200px 50px 90px;
    align-items: center;
    gap: 10px;
    padding: 6px 0;
    border-bottom: 1px solid var(--border-light);
    transition: background 0.1s;
    font-size: 13px;
  }
  .pr-top-row:hover {
    background: var(--bg-hover);
  }
  .pr-top-row-clickable {
    cursor: pointer;
  }
  .pr-top-rank {
    text-align: right;
    font-weight: 700;
    color: var(--text-muted);
    font-size: 12px;
  }
  .pr-top-url {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--accent);
    cursor: pointer;
  }
  .pr-top-url:hover {
    color: var(--accent-hover);
  }
  .pr-top-bar {
    height: 22px;
    border-radius: 4px;
    background: var(--accent);
    transition: width 0.3s ease;
  }
  .pr-top-score {
    text-align: right;
    font-weight: 600;
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }
  .pr-top-badges {
    display: flex;
    gap: 4px;
  }
  .pr-top-badge {
    font-size: 11px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--bg);
    color: var(--text-muted);
    font-weight: 500;
    white-space: nowrap;
  }
</style>
