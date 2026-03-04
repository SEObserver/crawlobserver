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

  function tfColor(tf) {
    if (tf >= 40) return '#2ecc71';
    if (tf >= 20) return '#f39c12';
    if (tf >= 10) return '#e67e22';
    return '#e74c3c';
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
  {@const maxWPR = data.pages[0]?.weighted_pr || 1}
  {#each data.pages as p, i}
    <div
      class="wpr-row wpr-row-clickable"
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
          weightedPr: p.weighted_pr,
          depth: p.depth,
          intLinks: p.internal_links_out,
          tf: p.trust_flow,
          cf: p.citation_flow,
          extBL: p.ext_backlinks,
          refDomains: p.ref_domains,
        })}
      onmouseleave={() => ontooltip?.(null)}
    >
      <span class="wpr-rank">{offset + i + 1}</span>
      <span class="wpr-url">{p.url.replace(/^https?:\/\/[^/]+/, '') || '/'}</span>
      <div>
        <div
          class="wpr-bar"
          style="width: {(p.weighted_pr / maxWPR) * 100}%; opacity: {0.4 + 0.6 * (p.weighted_pr / maxWPR)};"
        ></div>
      </div>
      <span class="wpr-score">{p.weighted_pr.toFixed(1)}</span>
      <span class="wpr-pr-int">{p.pagerank.toFixed(1)}</span>
      <span class="wpr-tf">
        {#if p.trust_flow != null}
          <span class="wpr-tf-badge" style="background: {tfColor(p.trust_flow)};"
            >{p.trust_flow}</span
          >
        {:else}
          <span class="wpr-na">-</span>
        {/if}
      </span>
      <span class="wpr-cf">{p.citation_flow != null ? p.citation_flow : '-'}</span>
      <span class="wpr-rd">{p.ref_domains != null ? fmtN(p.ref_domains) : '-'}</span>
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
  .wpr-row {
    display: grid;
    grid-template-columns: 36px 1fr 160px 50px 45px 40px 40px 65px;
    align-items: center;
    gap: 8px;
    padding: 6px 0;
    border-bottom: 1px solid var(--border-light);
    transition: background 0.1s;
    font-size: 13px;
  }
  .wpr-row:hover {
    background: var(--bg-hover);
  }
  .wpr-row-clickable {
    cursor: pointer;
  }
  .wpr-rank {
    text-align: right;
    font-weight: 700;
    color: var(--text-muted);
    font-size: 12px;
  }
  .wpr-url {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--accent);
    cursor: pointer;
  }
  .wpr-url:hover {
    color: var(--accent-hover);
  }
  .wpr-bar {
    height: 22px;
    border-radius: 4px;
    background: linear-gradient(90deg, var(--accent), #c9a227);
    transition: width 0.3s ease;
  }
  .wpr-score {
    text-align: right;
    font-weight: 600;
    color: #c9a227;
    font-variant-numeric: tabular-nums;
  }
  .wpr-pr-int {
    text-align: right;
    font-size: 11px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
  .wpr-tf {
    text-align: center;
  }
  .wpr-tf-badge {
    display: inline-block;
    padding: 1px 5px;
    border-radius: 3px;
    color: #fff;
    font-size: 11px;
    font-weight: 600;
    min-width: 24px;
    text-align: center;
  }
  .wpr-cf {
    text-align: center;
    font-size: 12px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
  .wpr-rd {
    text-align: right;
    font-size: 12px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
  .wpr-na {
    color: var(--text-muted);
    opacity: 0.5;
  }
</style>
