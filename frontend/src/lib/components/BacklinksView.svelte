<script>
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import Pagination from './Pagination.svelte';
  import SearchSelect from './SearchSelect.svelte';

  let {
    data = [],
    total = 0,
    offset = 0,
    limit = 100,
    sortColumn = '',
    sortOrder = '',
    filters = {},
    sessionId = '',
    onsort,
    onpagechange,
    onlimitchange,
    onnavigate,
    onsetfilter,
    onapplyfilters,
    onclearfilters,
  } = $props();

  const COLUMNS = [
    { label: () => t('links.sourceUrl'), sortKey: 'source_url', filterKey: 'source_url' },
    { label: () => t('links.targetUrl'), sortKey: 'target_url', filterKey: 'target_url' },
    { label: () => t('links.anchor'), sortKey: 'anchor_text', filterKey: 'anchor_text' },
    { label: () => 'TF', sortKey: 'trust_flow', filterKey: 'trust_flow', small: true },
    { label: () => 'CF', sortKey: 'citation_flow', filterKey: 'citation_flow', small: true },
    { label: () => 'NF', sortKey: 'nofollow', filterKey: 'nofollow', small: true },
    {
      label: () => t('links.firstSeen'),
      sortKey: 'first_seen',
      filterKey: 'first_seen',
      small: true,
    },
    { label: () => t('links.lastSeen'), sortKey: 'last_seen', filterKey: 'last_seen', small: true },
  ];

  function handleSort(sortKey) {
    if (!sortKey || !onsort) return;
    if (sortColumn !== sortKey) {
      onsort(sortKey, 'desc');
    } else if (sortOrder === 'desc') {
      onsort(sortKey, 'asc');
    } else {
      onsort('', '');
    }
  }

  function ttfClass(topic) {
    if (!topic) return '';
    return topic.split('/')[0].toLowerCase();
  }

  function tfFallbackStyle(val) {
    if (val >= 40) return 'background: #2ecc71; color: #fff;';
    if (val >= 20) return 'background: #f39c12; color: #fff;';
    if (val >= 10) return 'background: #e67e22; color: #fff;';
    return 'background: #e74c3c; color: #fff;';
  }

  function urlDetailHref(url) {
    return `/sessions/${sessionId}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(urlDetailHref(url));
  }

  function faviconUrl(sourceUrl) {
    try {
      const host = new URL(sourceUrl).hostname;
      return `https://www.google.com/s2/favicons?domain=${host}&sz=16`;
    } catch {
      return '';
    }
  }

  function hasActiveFilters() {
    return Object.values(filters).some((v) => v && v !== '');
  }
</script>

{#if data.length > 0 || hasActiveFilters()}
  <div class="bl-controls">
    <label>{t('pagerank.show')}</label>
    <SearchSelect
      small
      value={limit}
      onchange={(v) => onlimitchange?.(Number(v))}
      options={[
        { value: 20, label: '20' },
        { value: 50, label: '50' },
        { value: 100, label: '100' },
        { value: 200, label: '200' },
      ]}
    />
    <span class="text-muted text-xs">{t('links.backlinksCount', { count: fmtN(total) })}</span>
  </div>

  <table>
    <thead>
      <tr>
        <th class="col-rank">#</th>
        {#each COLUMNS as col}
          <th class="{col.small ? 'col-sm' : ''} sortable" onclick={() => handleSort(col.sortKey)}>
            <span class="sort-header">
              {col.label()}
              <span class="sort-indicator" class:sort-active={sortColumn === col.sortKey}>
                {#if sortColumn === col.sortKey && sortOrder === 'asc'}
                  <svg
                    viewBox="0 0 24 24"
                    width="14"
                    height="14"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"><path d="M12 19V5m-7 7l7-7 7 7" /></svg
                  >
                {:else if sortColumn === col.sortKey && sortOrder === 'desc'}
                  <svg
                    viewBox="0 0 24 24"
                    width="14"
                    height="14"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"><path d="M12 5v14m7-7l-7 7-7-7" /></svg
                  >
                {:else}
                  <svg
                    viewBox="0 0 24 24"
                    width="14"
                    height="14"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    opacity="0.3"><path d="M12 5v14m7-7l-7 7-7-7" /></svg
                  >
                {/if}
              </span>
            </span>
          </th>
        {/each}
      </tr>
      <tr class="filter-row">
        <td></td>
        {#each COLUMNS as col, idx}
          {#if col.filterKey}
            <td
              ><input
                class="filter-input"
                placeholder={col.filterKey}
                value={filters[col.filterKey] || ''}
                oninput={(e) => onsetfilter?.(col.filterKey, e.target.value)}
                onkeydown={(e) => e.key === 'Enter' && onapplyfilters?.()}
              /></td
            >
          {:else}
            <td></td>
          {/if}
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each data as bl, i}
        <tr>
          <td class="col-rank">{offset + i + 1}</td>
          <td class="cell-url"
            ><span class="cell-url-inner">
              {#if faviconUrl(bl.source_url)}<img
                  class="bl-favicon"
                  src={faviconUrl(bl.source_url)}
                  alt=""
                  width="16"
                  height="16"
                  loading="lazy"
                />{/if}
              <a href={bl.source_url} target="_blank" rel="noopener" title={bl.source_url}
                >{bl.source_url}</a
              >
            </span></td
          >
          <td class="cell-url"
            ><a
              href={urlDetailHref(bl.target_url)}
              onclick={(e) => goToUrlDetail(e, bl.target_url)}
              title={bl.target_url}>{bl.target_url}</a
            ></td
          >
          <td class="cell-title" title={bl.anchor_text}>{bl.anchor_text || '-'}</td>
          <td class="col-sm text-center">
            {#if bl.trust_flow != null}
              {#if bl.source_ttf_topic}
                <span class="ttf_label {ttfClass(bl.source_ttf_topic)}">{bl.trust_flow}</span>
              {:else}
                <span class="ttf_label" style={tfFallbackStyle(bl.trust_flow)}>{bl.trust_flow}</span
                >
              {/if}
            {:else}
              -
            {/if}
          </td>
          <td class="col-sm text-center text-muted"
            >{bl.citation_flow != null ? bl.citation_flow : '-'}</td
          >
          <td class="col-sm text-center">
            {#if bl.nofollow}
              <span class="badge-nf">{t('links.nofollow')}</span>
            {:else}
              <span class="badge-df">{t('links.dofollow')}</span>
            {/if}
          </td>
          <td class="col-sm text-center text-muted text-xs"
            >{bl.first_seen ? bl.first_seen.slice(0, 10) : '-'}</td
          >
          <td class="col-sm text-center text-muted text-xs"
            >{bl.last_seen ? bl.last_seen.slice(0, 10) : '-'}</td
          >
        </tr>
      {/each}
    </tbody>
  </table>

  {#if data.length > 0}
    <Pagination {offset} {limit} {total} onchange={(o) => onpagechange?.(o)} />
  {:else}
    <p class="chart-empty">{t('links.noBacklinks')}</p>
  {/if}
{:else}
  <p class="chart-empty">{t('links.noBacklinks')}</p>
{/if}

<style>
  .bl-controls {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
    flex-wrap: wrap;
  }
  .bl-controls label {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
  }
  .bl-controls :global(.ss-wrap) {
    width: 80px;
    flex-shrink: 0;
  }
  .col-rank {
    width: 36px;
    text-align: right;
    font-weight: 700;
    color: var(--text-muted);
    font-size: 12px;
  }
  .col-sm {
    width: 70px;
    white-space: nowrap;
  }
  .cell-url-inner {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
    max-width: 100%;
  }
  .cell-url-inner a {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .bl-favicon {
    flex-shrink: 0;
    border-radius: 2px;
  }
  .text-center {
    text-align: center;
  }
  .text-muted {
    color: var(--text-muted);
  }
  .text-xs {
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .badge-nf {
    display: inline-block;
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 10px;
    font-weight: 600;
    background: #e74c3c;
    color: #fff;
  }
  .badge-df {
    display: inline-block;
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 10px;
    font-weight: 600;
    background: #2ecc71;
    color: #fff;
  }
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
  th.sortable {
    cursor: pointer;
    user-select: none;
  }
  th.sortable:hover {
    background: var(--hover-bg, rgba(255, 255, 255, 0.05));
  }
  .sort-header {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .sort-indicator {
    display: inline-flex;
    flex-shrink: 0;
  }
  .sort-indicator.sort-active svg {
    opacity: 1;
  }
</style>
