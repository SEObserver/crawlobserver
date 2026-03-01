<script>
  import { fmtN, a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { data, ondrill, ontooltip } = $props();
</script>

{#if data && data.total_with_pr > 0}
  <div class="stats-grid pr-stats-grid">
    <div class="stat-card"><div class="stat-value">{fmtN(data.total_with_pr)}</div><div class="stat-label">{t('pagerank.pagesWithPR')}</div></div>
    <div class="stat-card"><div class="stat-value">{data.avg.toFixed(2)}</div><div class="stat-label">{t('pagerank.mean')}</div></div>
    <div class="stat-card"><div class="stat-value">{data.median.toFixed(2)}</div><div class="stat-label">{t('pagerank.median')}</div></div>
    <div class="stat-card"><div class="stat-value">{data.p90.toFixed(2)}</div><div class="stat-label">P90</div></div>
    <div class="stat-card"><div class="stat-value">{data.p99.toFixed(2)}</div><div class="stat-label">P99</div></div>
  </div>
  {@const distBuckets = data.buckets || []}
  {@const distMaxCount = Math.max(...distBuckets.map(b => b.count), 1)}
  {@const histW = 600}
  {@const histH = 300}
  {@const histMargin = { top: 20, right: 20, bottom: 40, left: 60 }}
  {@const plotW = histW - histMargin.left - histMargin.right}
  {@const plotH = histH - histMargin.top - histMargin.bottom}
  {@const barGap = 1}
  {@const barW = distBuckets.length > 0 ? (plotW - (distBuckets.length - 1) * barGap) / distBuckets.length : 0}
  {@const logMax = Math.log10(distMaxCount + 1)}
  <svg viewBox="0 0 {histW} {histH}" class="pr-hist-svg">
    {#each [1, 10, 100, 1000, 10000, 100000] as tick}
      {#if tick <= distMaxCount * 1.5}
        {@const ty = histMargin.top + plotH - (logMax > 0 ? (Math.log10(tick + 1) / logMax) * plotH : 0)}
        <line x1={histMargin.left} y1={ty} x2={histW - histMargin.right} y2={ty} stroke="var(--border)" stroke-dasharray="3,3" />
        <text x={histMargin.left - 8} y={ty + 4} text-anchor="end" class="pr-axis-tick">{tick >= 1000 ? (tick/1000) + 'k' : tick}</text>
      {/if}
    {/each}
    {#each distBuckets as bucket, i}
      {@const barH = logMax > 0 ? (Math.log10(bucket.count + 1) / logMax) * plotH : 0}
      {@const bx = histMargin.left + i * (barW + barGap)}
      {@const by = histMargin.top + plotH - barH}
      {@const opacity = 0.4 + 0.6 * (bucket.count / distMaxCount)}
      <rect class="pr-hist-bar" role="button" tabindex="0" x={bx} y={by} width={barW} height={barH} rx="2" fill="var(--accent)" opacity={opacity}
        onmouseenter={(e) => ontooltip?.({ x: e.clientX, y: e.clientY, bucketMin: bucket.min, bucketMax: bucket.max, count: bucket.count, avgPR: bucket.avg_pr })}
        onmouseleave={() => ontooltip?.(null)}
        onclick={() => ondrill?.(bucket.min, bucket.max)}
        onkeydown={a11yKeydown(() => ondrill?.(bucket.min, bucket.max))} />
      {#if distBuckets.length <= 25 || i % Math.ceil(distBuckets.length / 10) === 0}
        <text x={bx + barW / 2} y={histH - histMargin.bottom + 16} text-anchor="middle" class="pr-axis-label-sm">{bucket.min.toFixed(0)}</text>
      {/if}
    {/each}
    <text x={histW / 2} y={histH - 4} text-anchor="middle" class="pr-axis-label">{t('pagerank.score')}</text>
    <text x={14} y={histH / 2} text-anchor="middle" transform="rotate(-90, 14, {histH / 2})" class="pr-axis-label">{t('pagerank.pagesLog')}</text>
  </svg>
{:else}
  <p class="chart-empty">{t('pagerank.noData')}</p>
{/if}

<style>
  .pr-stats-grid { margin-bottom: 20px; }
  .pr-hist-svg { width: 100%; max-width: 700px; height: auto; }
  .pr-axis-tick { font-size: 10px; fill: var(--text-muted); }
  .pr-axis-label-sm { font-size: 9px; fill: var(--text-muted); }
  .pr-axis-label { font-size: 11px; fill: var(--text-muted); }
  .pr-hist-bar { transition: opacity 0.15s; cursor: pointer; }
  .pr-hist-bar:hover { opacity: 0.75; }
</style>
