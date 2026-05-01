<script>
  import { fmtN, a11yKeydown } from '../../utils.js';
  import { t } from '../../i18n/index.svelte.js';
  import DonutChart from '../charts/DonutChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

  function nav(tab, filters = {}) {
    onnavigate?.(`/sessions/${sessionId}/${tab}`, filters);
  }

  const sitemap = $derived(audit?.sitemaps);

  function coverageSegments(s) {
    if (!s) return [];
    const segs = [];
    if (s.in_both > 0)
      segs.push({
        value: s.in_both,
        color: 'var(--success)',
        label: t('report.sitemap.inBoth'),
        onclick: () => nav('directives/in_both'),
      });
    if (s.crawled_only > 0)
      segs.push({
        value: s.crawled_only,
        color: 'var(--warning)',
        label: t('report.sitemap.crawledOnly'),
        onclick: () => nav('directives/crawl_only'),
      });
    if (s.sitemap_only > 0)
      segs.push({
        value: s.sitemap_only,
        color: 'var(--error)',
        label: t('report.sitemap.sitemapOnly'),
        onclick: () => nav('directives/sitemap_only'),
      });
    return segs;
  }

  const covSegs = $derived(coverageSegments(sitemap));
  const total = $derived(
    (sitemap?.in_both || 0) + (sitemap?.crawled_only || 0) + (sitemap?.sitemap_only || 0),
  );
  const coveragePct = $derived(
    total > 0 ? (((sitemap?.in_both || 0) / total) * 100).toFixed(1) : '0',
  );
</script>

{#if sitemap}
  <div class="report-section">
    <h3 class="chart-title">{t('report.sitemap.coverage')}</h3>
    <div class="report-grid">
      <DonutChart
        segments={covSegs}
        size={200}
        strokeWidth={28}
        centerLabel="{coveragePct}%"
        centerSubLabel={t('report.sitemap.coverageLabel')}
      />
      <div class="stats-grid">
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('directives/in_both')}
          onkeydown={a11yKeydown(() => nav('directives/in_both'))}
        >
          <div class="stat-value">{fmtN(sitemap.in_both || 0)}</div>
          <div class="stat-label">{t('report.sitemap.inBoth')}</div>
        </div>
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('directives/crawl_only')}
          onkeydown={a11yKeydown(() => nav('directives/crawl_only'))}
        >
          <div class="stat-value text-warning">{fmtN(sitemap.crawled_only || 0)}</div>
          <div class="stat-label">{t('report.sitemap.crawledOnly')}</div>
        </div>
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('directives/sitemap_only')}
          onkeydown={a11yKeydown(() => nav('directives/sitemap_only'))}
        >
          <div class="stat-value text-error">{fmtN(sitemap.sitemap_only || 0)}</div>
          <div class="stat-label">{t('report.sitemap.sitemapOnly')}</div>
        </div>
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('sitemaps')}
          onkeydown={a11yKeydown(() => nav('sitemaps'))}
        >
          <div class="stat-value">{fmtN(sitemap.total_sitemap_urls || 0)}</div>
          <div class="stat-label">{t('report.sitemap.totalUrls')}</div>
        </div>
      </div>
    </div>
  </div>
{:else}
  <p class="chart-empty">{t('report.sitemap.noData')}</p>
{/if}
