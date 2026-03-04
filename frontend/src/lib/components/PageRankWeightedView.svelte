<script>
  import { fmtN, a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import Pagination from './Pagination.svelte';
  import SearchSelect from './SearchSelect.svelte';

  let {
    data,
    offset = 0,
    limit = 50,
    sortColumn = '',
    sortOrder = '',
    dirFilter = '',
    onnavigate,
    onlimitchange,
    onpagechange,
    ontooltip,
    onsort,
    onfilterchange,
  } = $props();

  function goToUrlDetail(url) {
    onnavigate?.(url);
  }

  function handleSort(sortKey) {
    if (!sortKey || !onsort) return;
    if (sortColumn !== sortKey) {
      onsort(sortKey, 'asc');
    } else if (sortOrder === 'asc') {
      onsort(sortKey, 'desc');
    } else {
      onsort('', '');
    }
  }

  function ttfClass(topic) {
    if (!topic) return '';
    return topic.split('/')[0].toLowerCase();
  }
</script>

{#snippet sortArrow(key)}
  <span class="sort-indicator" class:sort-active={sortColumn === key}>
    {#if sortColumn === key && sortOrder === 'asc'}
      <svg
        viewBox="0 0 24 24"
        width="12"
        height="12"
        fill="none"
        stroke="currentColor"
        stroke-width="2"><path d="M12 19V5m-7 7l7-7 7 7" /></svg
      >
    {:else if sortColumn === key && sortOrder === 'desc'}
      <svg
        viewBox="0 0 24 24"
        width="12"
        height="12"
        fill="none"
        stroke="currentColor"
        stroke-width="2"><path d="M12 5v14m7-7l-7 7-7-7" /></svg
      >
    {:else}
      <svg
        viewBox="0 0 24 24"
        width="12"
        height="12"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        opacity="0.3"><path d="M12 5v14m7-7l-7 7-7-7" /></svg
      >
    {/if}
  </span>
{/snippet}

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
    <label class="wpr-dir-label">{t('pagerank.directoryFilter')}</label>
    <input
      class="wpr-dir-filter"
      type="text"
      placeholder={t('pagerank.filterPlaceholder')}
      value={dirFilter}
      oninput={(e) => onfilterchange?.(e.target.value, false)}
      onkeydown={(e) => {
        if (e.key === 'Enter') onfilterchange?.(e.target.value, true);
      }}
    />
    <button class="btn btn-sm" onclick={() => onfilterchange?.(dirFilter, true)}
      >{t('common.filter')}</button
    >
    {#if dirFilter}
      <button class="btn btn-sm" onclick={() => onfilterchange?.('', true)}
        >{t('pagerank.clear')}</button
      >
    {/if}
  </div>
  {@const maxWPR = Math.max(...data.pages.map((p) => p.weighted_pr), 1)}
  <div class="wpr-row wpr-header">
    <span class="wpr-rank">#</span>
    <span>URL</span>
    <span></span>
    <span class="wpr-score sortable" onclick={() => handleSort('weighted_pr')}
      ><span class="sort-header">{t('pagerank.weightedPR')} {@render sortArrow('weighted_pr')}</span
      ></span
    >
    <span class="wpr-pr-int sortable" onclick={() => handleSort('pagerank')}
      ><span class="sort-header">{t('pagerank.internalPR')} {@render sortArrow('pagerank')}</span
      ></span
    >
    <span class="wpr-delta sortable" onclick={() => handleSort('delta')}
      ><span class="sort-header">Delta {@render sortArrow('delta')}</span></span
    >
    <span class="wpr-tf sortable" onclick={() => handleSort('trust_flow')}
      ><span class="sort-header">TF {@render sortArrow('trust_flow')}</span></span
    >
    <span class="wpr-cf sortable" onclick={() => handleSort('citation_flow')}
      ><span class="sort-header">CF {@render sortArrow('citation_flow')}</span></span
    >
    <span class="wpr-rd sortable" onclick={() => handleSort('ref_domains')}
      ><span class="sort-header">RD {@render sortArrow('ref_domains')}</span></span
    >
  </div>
  {#each data.pages as p, i}
    {@const delta = p.weighted_pr - p.pagerank}
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
          ttfTopic: p.ttf_topic,
        })}
      onmouseleave={() => ontooltip?.(null)}
    >
      <span class="wpr-rank">{offset + i + 1}</span>
      <span class="wpr-url">{p.url.replace(/^https?:\/\/[^/]+/, '') || '/'}</span>
      <div>
        <div
          class="wpr-bar"
          style="width: {(p.weighted_pr / maxWPR) * 100}%; opacity: {0.4 +
            0.6 * (p.weighted_pr / maxWPR)};"
        ></div>
      </div>
      <span class="wpr-score">{p.weighted_pr.toFixed(1)}</span>
      <span class="wpr-pr-int">{p.pagerank.toFixed(1)}</span>
      <span class="wpr-delta" class:wpr-delta-pos={delta > 0.1}>{delta.toFixed(1)}</span>
      <span class="wpr-tf">
        {#if p.trust_flow != null}
          {#if p.ttf_topic}
            <span class="ttf_label {ttfClass(p.ttf_topic)}">{p.trust_flow}</span>
          {:else}
            <span
              class="wpr-tf-badge"
              style="background: {p.trust_flow >= 40
                ? '#2ecc71'
                : p.trust_flow >= 20
                  ? '#f39c12'
                  : p.trust_flow >= 10
                    ? '#e67e22'
                    : '#e74c3c'};">{p.trust_flow}</span
            >
          {/if}
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
  .wpr-dir-label {
    margin-left: 16px;
  }
  .wpr-dir-filter {
    background: var(--bg-input);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 4px 8px;
    font-size: 12px;
    color: var(--text);
    width: 180px;
  }
  .wpr-row {
    display: grid;
    grid-template-columns: 36px 1fr 140px 55px 50px 50px 50px 40px 65px;
    align-items: center;
    gap: 8px;
    padding: 6px 0;
    border-bottom: 1px solid var(--border-light);
    transition: background 0.1s;
    font-size: 13px;
  }
  .wpr-row:hover:not(.wpr-header) {
    background: var(--bg-hover);
  }
  .wpr-header {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.03em;
    border-bottom: 2px solid var(--border);
    padding-bottom: 8px;
    cursor: default;
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
  .wpr-delta {
    text-align: right;
    font-size: 11px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
  .wpr-delta-pos {
    color: #2ecc71;
    font-weight: 600;
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
  .sortable {
    cursor: pointer;
    user-select: none;
  }
  .sortable:hover {
    color: var(--text);
  }
  .sort-header {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }
  .sort-indicator {
    display: inline-flex;
    align-items: center;
  }
  .sort-active {
    color: var(--accent);
  }
  /* TTF topic label styles */
  .ttf_label {
    font-weight: 700;
    font-size: 8.5pt;
    border-radius: 4px;
    padding: 1px 5px;
    display: inline-block;
    white-space: nowrap;
  }
  .ttf_label.arts {
    background: #ff6700;
    color: #fff;
  }
  .ttf_label.news {
    background: #76d54b;
    color: #333;
  }
  .ttf_label.society {
    background: #7a69cd;
    color: #fff;
  }
  .ttf_label.computers {
    background: #f33;
    color: #fff;
  }
  .ttf_label.business {
    background: #c5c88e;
    color: #333;
  }
  .ttf_label.regional {
    background: #f582b9;
    color: #fff;
  }
  .ttf_label.recreation {
    background: #89c7cb;
    color: #333;
  }
  .ttf_label.sports {
    background: #55355d;
    color: #fff;
  }
  .ttf_label.kids {
    background: #fc0;
    color: #333;
  }
  .ttf_label.reference {
    background: #c84770;
    color: #fff;
  }
  .ttf_label.games {
    background: #557832;
    color: #fff;
  }
  .ttf_label.home {
    background: #d95;
    color: #fff;
  }
  .ttf_label.shopping {
    background: #600;
    color: #fff;
  }
  .ttf_label.health {
    background: #009;
    color: #fff;
  }
  .ttf_label.science {
    background: #6bd39a;
    color: #333;
  }
  .ttf_label.world {
    background: #577;
    color: #fff;
  }
  .ttf_label.adult {
    background: #333;
    color: #fff;
  }
</style>
