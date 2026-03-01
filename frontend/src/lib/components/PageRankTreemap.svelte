<script>
  import { fmtN, a11yKeydown, squarify } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';

  let {
    data,
    depth = 2,
    minPages = 1,
    ondepthchange,
    onminpageschange,
    ondrill,
    ontooltip,
  } = $props();
</script>

{#if data?.length > 0}
  <div class="pr-controls">
    <label>{t('urlDetail.depth')}</label>
    <SearchSelect
      small
      value={depth}
      onchange={(v) => ondepthchange?.(Number(v))}
      options={[
        { value: 1, label: '1' },
        { value: 2, label: '2' },
        { value: 3, label: '3' },
      ]}
    />
    <label>{t('pagerank.minPages')}</label>
    <SearchSelect
      small
      value={minPages}
      onchange={(v) => onminpageschange?.(Number(v))}
      options={[
        { value: 1, label: '1' },
        { value: 5, label: '5' },
        { value: 10, label: '10' },
        { value: 25, label: '25' },
      ]}
    />
    <span class="text-muted text-xs">{t('pagerank.directories', { count: data.length })}</span>
  </div>
  {@const treemapItems = data.map((d) => ({ ...d, value: d.total_pr }))}
  {@const treemapRects = squarify(treemapItems, 0, 0, 100, 100)}
  {@const maxAvgPR = Math.max(...data.map((d) => d.avg_pr), 1)}
  <div class="pr-treemap-container">
    {#each treemapRects as rect}
      {@const opacity = 0.35 + 0.65 * (rect.avg_pr / maxAvgPR)}
      <div
        class="pr-treemap-rect"
        role="button"
        tabindex="0"
        style="left: {rect.x}%; top: {rect.y}%; width: {rect.w}%; height: {rect.h}%; background: var(--accent); opacity: {opacity};"
        onclick={() => ondrill?.(rect.path)}
        onkeydown={a11yKeydown(() => ondrill?.(rect.path))}
        onmouseenter={(e) =>
          ontooltip?.({
            x: e.clientX,
            y: e.clientY,
            path: rect.path,
            pages: rect.page_count,
            totalPR: rect.total_pr,
            avgPR: rect.avg_pr,
            maxPR: rect.max_pr,
          })}
        onmouseleave={() => ontooltip?.(null)}
      >
        {#if rect.w > 6 && rect.h > 5}
          <div class="pr-treemap-label">
            {rect.path || '/'}
            {#if rect.w > 10 && rect.h > 8}
              <small
                >{rect.page_count}
                {t('common.pages')} &middot; {t('pagerank.avg')}
                {rect.avg_pr.toFixed(1)}</small
              >
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>
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
  .pr-treemap-container {
    position: relative;
    width: 100%;
    height: 500px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  .pr-treemap-rect {
    position: absolute;
    border: 1px solid var(--bg-card);
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .pr-treemap-rect:hover {
    opacity: 0.85;
  }
  .pr-treemap-label {
    font-size: 11px;
    font-weight: 600;
    color: #fff;
    text-align: center;
    padding: 4px;
    line-height: 1.2;
    text-shadow: 0 1px 2px rgba(0, 0, 0, 0.4);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
  .pr-treemap-label small {
    display: block;
    font-size: 10px;
    font-weight: 400;
    opacity: 0.85;
  }
</style>
