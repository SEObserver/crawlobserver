<script>
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { tooltip } = $props();

  function ttfClass(topic) {
    if (!topic) return '';
    return topic.split('/')[0].toLowerCase();
  }
</script>

{#if tooltip}
  <div class="pr-tooltip" style="left: {tooltip.x + 12}px; top: {tooltip.y - 10}px;">
    {#if tooltip.url}
      <div class="pr-tooltip-title">{tooltip.url}</div>
      <div class="pr-tooltip-row">
        <span>{t('urlDetail.pageRank')}</span><span>{tooltip.pr.toFixed(2)}</span>
      </div>
      {#if tooltip.weightedPr != null}
        <div class="pr-tooltip-row">
          <span>{t('pagerank.weightedPR')}</span><span>{tooltip.weightedPr.toFixed(2)}</span>
        </div>
      {/if}
      <div class="pr-tooltip-row">
        <span>{t('urlDetail.depth')}</span><span>{tooltip.depth}</span>
      </div>
      <div class="pr-tooltip-row">
        <span>{t('pagerank.intLinks')}</span><span>{fmtN(tooltip.intLinks)}</span>
      </div>
      {#if tooltip.weightedPr != null}
        <div class="pr-tooltip-row">
          <span>TF</span><span>{#if tooltip.tf != null}{tooltip.tf}{#if tooltip.ttfTopic} <span class="ttf_label {ttfClass(tooltip.ttfTopic)}">{tooltip.ttfTopic}</span>{/if}{:else}-{/if}</span>
        </div>
        <div class="pr-tooltip-row">
          <span>CF</span><span>{tooltip.cf != null ? tooltip.cf : '-'}</span>
        </div>
        <div class="pr-tooltip-row">
          <span>Ref Domains</span><span>{tooltip.refDomains != null ? fmtN(tooltip.refDomains) : '-'}</span>
        </div>
        <div class="pr-tooltip-row">
          <span>Ext Backlinks</span><span>{tooltip.extBL != null ? fmtN(tooltip.extBL) : '-'}</span>
        </div>
      {:else}
        <div class="pr-tooltip-row">
          <span>{t('pagerank.extLinks')}</span><span>{fmtN(tooltip.extLinks)}</span>
        </div>
        <div class="pr-tooltip-row">
          <span>{t('session.words')}</span><span>{fmtN(tooltip.words)}</span>
        </div>
      {/if}
    {:else if tooltip.path !== undefined}
      <div class="pr-tooltip-title">{tooltip.path || '/'}</div>
      <div class="pr-tooltip-row">
        <span>{t('common.pages')}</span><span>{fmtN(tooltip.pages)}</span>
      </div>
      <div class="pr-tooltip-row">
        <span>{t('pagerank.totalPR')}</span><span>{tooltip.totalPR.toFixed(1)}</span>
      </div>
      <div class="pr-tooltip-row">
        <span>{t('pagerank.avgPR')}</span><span>{tooltip.avgPR.toFixed(2)}</span>
      </div>
      <div class="pr-tooltip-row">
        <span>{t('pagerank.maxPR')}</span><span>{tooltip.maxPR.toFixed(2)}</span>
      </div>
    {:else if tooltip.bucketMin !== undefined}
      <div class="pr-tooltip-title">
        PR {tooltip.bucketMin.toFixed(1)} - {tooltip.bucketMax.toFixed(1)}
      </div>
      <div class="pr-tooltip-row">
        <span>{t('common.pages')}</span><span>{fmtN(tooltip.count)}</span>
      </div>
      <div class="pr-tooltip-row">
        <span>{t('pagerank.avgPR')}</span><span>{tooltip.avgPR.toFixed(2)}</span>
      </div>
    {/if}
  </div>
{/if}

<style>
  .pr-tooltip {
    position: fixed;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 10px 14px;
    font-size: 12px;
    box-shadow: var(--shadow-md);
    z-index: 2000;
    pointer-events: none;
    max-width: 360px;
  }
  .pr-tooltip-title {
    font-weight: 600;
    margin-bottom: 4px;
    color: var(--text);
    word-break: break-all;
  }
  .pr-tooltip-row {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    color: var(--text-secondary);
  }
  .pr-tooltip-row span:last-child {
    font-weight: 600;
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }
  /* TTF topic label styles */
  .ttf_label { font-weight: 700; font-size: 8pt; border-radius: 4px; padding: 1px 4px; display: inline-block; white-space: nowrap; margin-left: 4px; }
  .ttf_label.arts { background: #ff6700; color: #fff; }
  .ttf_label.news { background: #76D54B; color: #333; }
  .ttf_label.society { background: #7A69CD; color: #fff; }
  .ttf_label.computers { background: #f33; color: #fff; }
  .ttf_label.business { background: #C5C88E; color: #333; }
  .ttf_label.regional { background: #F582B9; color: #fff; }
  .ttf_label.recreation { background: #89C7CB; color: #333; }
  .ttf_label.sports { background: #55355D; color: #fff; }
  .ttf_label.kids { background: #fc0; color: #333; }
  .ttf_label.reference { background: #C84770; color: #fff; }
  .ttf_label.games { background: #557832; color: #fff; }
  .ttf_label.home { background: #d95; color: #fff; }
  .ttf_label.shopping { background: #600; color: #fff; }
  .ttf_label.health { background: #009; color: #fff; }
  .ttf_label.science { background: #6BD39A; color: #333; }
  .ttf_label.world { background: #577; color: #fff; }
  .ttf_label.adult { background: #333; color: #fff; }
</style>
