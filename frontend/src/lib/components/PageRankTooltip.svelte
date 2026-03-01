<script>
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { tooltip } = $props();
</script>

{#if tooltip}
  <div class="pr-tooltip" style="left: {tooltip.x + 12}px; top: {tooltip.y - 10}px;">
    {#if tooltip.url}
      <div class="pr-tooltip-title">{tooltip.url}</div>
      <div class="pr-tooltip-row"><span>{t('urlDetail.pageRank')}</span><span>{tooltip.pr.toFixed(2)}</span></div>
      <div class="pr-tooltip-row"><span>{t('urlDetail.depth')}</span><span>{tooltip.depth}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.intLinks')}</span><span>{fmtN(tooltip.intLinks)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.extLinks')}</span><span>{fmtN(tooltip.extLinks)}</span></div>
      <div class="pr-tooltip-row"><span>{t('session.words')}</span><span>{fmtN(tooltip.words)}</span></div>
    {:else if tooltip.path !== undefined}
      <div class="pr-tooltip-title">{tooltip.path || '/'}</div>
      <div class="pr-tooltip-row"><span>{t('common.pages')}</span><span>{fmtN(tooltip.pages)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.totalPR')}</span><span>{tooltip.totalPR.toFixed(1)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.avgPR')}</span><span>{tooltip.avgPR.toFixed(2)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.maxPR')}</span><span>{tooltip.maxPR.toFixed(2)}</span></div>
    {:else if tooltip.bucketMin !== undefined}
      <div class="pr-tooltip-title">PR {tooltip.bucketMin.toFixed(1)} - {tooltip.bucketMax.toFixed(1)}</div>
      <div class="pr-tooltip-row"><span>{t('common.pages')}</span><span>{fmtN(tooltip.count)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.avgPR')}</span><span>{tooltip.avgPR.toFixed(2)}</span></div>
    {/if}
  </div>
{/if}

<style>
  .pr-tooltip { position: fixed; background: var(--bg-card); border: 1px solid var(--border); border-radius: var(--radius-sm); padding: 10px 14px; font-size: 12px; box-shadow: var(--shadow-md); z-index: 2000; pointer-events: none; max-width: 360px; }
  .pr-tooltip-title { font-weight: 600; margin-bottom: 4px; color: var(--text); word-break: break-all; }
  .pr-tooltip-row { display: flex; justify-content: space-between; gap: 16px; color: var(--text-secondary); }
  .pr-tooltip-row span:last-child { font-weight: 600; color: var(--text); font-variant-numeric: tabular-nums; }
</style>
