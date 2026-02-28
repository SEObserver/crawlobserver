<script>
  import { fmtN } from '../../utils.js';
  import DonutChart from '../charts/DonutChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

  const sitemap = $derived(audit?.sitemaps);

  function coverageSegments(s) {
    if (!s) return [];
    const segs = [];
    if (s.in_both > 0) segs.push({ value: s.in_both, color: 'var(--success)', label: 'In Both' });
    if (s.crawled_only > 0) segs.push({ value: s.crawled_only, color: 'var(--warning)', label: 'Crawled Only' });
    if (s.sitemap_only > 0) segs.push({ value: s.sitemap_only, color: 'var(--error)', label: 'Sitemap Only' });
    return segs;
  }

  const covSegs = $derived(coverageSegments(sitemap));
  const total = $derived((sitemap?.in_both || 0) + (sitemap?.crawled_only || 0) + (sitemap?.sitemap_only || 0));
  const coveragePct = $derived(total > 0 ? ((sitemap?.in_both || 0) / total * 100).toFixed(1) : '0');
</script>

{#if sitemap}
  <div class="report-section">
    <h3 class="chart-title">Sitemap Coverage</h3>
    <div class="report-grid">
      <DonutChart segments={covSegs} size={200} strokeWidth={28}
        centerLabel="{coveragePct}%" centerSubLabel="coverage" />
      <div class="stats-grid">
        <div class="stat-card"><div class="stat-value">{fmtN(sitemap.in_both || 0)}</div><div class="stat-label">In Both</div></div>
        <div class="stat-card"><div class="stat-value" style="color: var(--warning)">{fmtN(sitemap.crawled_only || 0)}</div><div class="stat-label">Crawled Only</div></div>
        <div class="stat-card"><div class="stat-value" style="color: var(--error)">{fmtN(sitemap.sitemap_only || 0)}</div><div class="stat-label">Sitemap Only</div></div>
        <div class="stat-card"><div class="stat-value">{fmtN(sitemap.total_sitemap_urls || 0)}</div><div class="stat-label">Total Sitemap URLs</div></div>
      </div>
    </div>
  </div>
{:else}
  <p class="chart-empty">No sitemap audit data available.</p>
{/if}
