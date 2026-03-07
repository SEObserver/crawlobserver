<script>
  import { onMount } from 'svelte';
  import { fmtN, fmt, a11yKeydown } from '../../utils.js';
  import { t } from '../../i18n/index.svelte.js';
  import { getStatusTimeline, getStatusTimelineRecent } from '../../api.js';
  import DonutChart from '../charts/DonutChart.svelte';
  import HBarChart from '../charts/HBarChart.svelte';
  import AreaChart from '../charts/AreaChart.svelte';

  let { stats, sessionId, isRunning = false, onnavigate } = $props();

  // Status timeline charts
  let timeline = $state(null);
  let recentTimeline = $state(null);
  let timelineLoading = $state(false);
  let pollTimer = null; // NOT $state — avoids read/write loop in $effect

  function fetchTimeline() {
    if (!sessionId || timelineLoading) return;
    timelineLoading = true;
    Promise.all([
      getStatusTimeline(sessionId),
      getStatusTimelineRecent(sessionId),
    ])
      .then(([data, recent]) => {
        timeline = data ?? [];
        recentTimeline = recent ?? [];
      })
      .catch((err) => {
        console.error('[OverviewReport] timeline fetch failed:', err);
        timeline = [];
        recentTimeline = [];
      })
      .finally(() => {
        timelineLoading = false;
      });
  }

  onMount(() => {
    fetchTimeline();
  });

  // Poll while running — also fetch immediately when isRunning transitions to true
  $effect(() => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    if (isRunning && sessionId) {
      fetchTimeline();
      pollTimer = setInterval(fetchTimeline, 5_000);
    }
    return () => {
      if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
    };
  });

  function fmtTime(ts) {
    const d = new Date(ts);
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function fmtTimeSec(ts) {
    const d = new Date(ts);
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }

  function buildSeries(data) {
    if (!data?.length) return [];
    return [
      {
        key: 'retried_403',
        label: t('report.technical.retried403'),
        color: '#f97316',
        opacity: 0.3,
        values: data.map((b) => b.retried_403 || 0),
      },
      {
        key: 'retried_429',
        label: t('report.technical.retried429'),
        color: '#ef4444',
        opacity: 0.3,
        values: data.map((b) => b.retried_429 || 0),
      },
      {
        key: 'retried_5xx',
        label: t('report.technical.retried5xx'),
        color: '#dc2626',
        opacity: 0.3,
        values: data.map((b) => b.retried_5xx || 0),
      },
      {
        key: 'ok',
        label: '2xx',
        color: 'var(--success, #22c55e)',
        values: data.map((b) => b.ok),
      },
      {
        key: 'redirect',
        label: '3xx',
        color: 'var(--info, #6366f1)',
        values: data.map((b) => b.redirect),
      },
      {
        key: 's403',
        label: '403',
        color: '#f97316',
        values: data.map((b) => b.s403),
      },
      {
        key: 's429',
        label: '429',
        color: '#ef4444',
        values: data.map((b) => b.s429),
      },
      {
        key: 'client_err',
        label: '4xx',
        color: '#f59e0b',
        values: data.map((b) => b.client_err),
      },
      {
        key: 'server_err',
        label: '5xx',
        color: '#dc2626',
        values: data.map((b) => b.server_err),
      },
      {
        key: 'fetch_err',
        label: t('report.technical.fetchErrors'),
        color: '#9ca3af',
        values: data.map((b) => b.fetch_err),
      },
    ];
  }

  const timelineSeries = $derived(buildSeries(timeline));
  const recentSeries = $derived(buildSeries(recentTimeline));

  const timelineLabels = $derived(timeline?.map((b) => fmtTime(b.ts)) || []);
  const recentLabels = $derived(recentTimeline?.map((b) => fmtTimeSec(b.ts)) || []);

  function nav(tab, filters = {}) {
    onnavigate?.(`/sessions/${sessionId}/${tab}`, filters);
  }

  function statusCodeSegments(s) {
    if (!s?.status_codes) return [];
    const groups = { '2xx': 0, '3xx': 0, '4xx': 0, '5xx': 0 };
    for (const [code, count] of Object.entries(s.status_codes)) {
      const c = Number(code);
      if (c >= 200 && c < 300) groups['2xx'] += count;
      else if (c >= 300 && c < 400) groups['3xx'] += count;
      else if (c >= 400 && c < 500) groups['4xx'] += count;
      else groups['5xx'] += count;
    }
    const segs = [];
    if (groups['2xx'] > 0)
      segs.push({
        value: groups['2xx'],
        color: 'var(--success)',
        label: t('report.overview.2xxSuccess'),
        onclick: () => nav('overview', { status_code: '2' }),
      });
    if (groups['3xx'] > 0)
      segs.push({
        value: groups['3xx'],
        color: 'var(--info)',
        label: t('report.overview.3xxRedirect'),
        onclick: () => nav('overview', { status_code: '3' }),
      });
    if (groups['4xx'] > 0)
      segs.push({
        value: groups['4xx'],
        color: 'var(--warning)',
        label: t('report.overview.4xxClientError'),
        onclick: () => nav('overview', { status_code: '4' }),
      });
    if (groups['5xx'] > 0)
      segs.push({
        value: groups['5xx'],
        color: 'var(--error)',
        label: t('report.overview.5xxServerError'),
        onclick: () => nav('overview', { status_code: '5' }),
      });
    return segs;
  }

  const scSegments = $derived(statusCodeSegments(stats));

  function statusCodeBars(s) {
    if (!s?.status_codes) return [];
    return Object.entries(s.status_codes)
      .map(([code, count]) => [Number(code), count])
      .sort((a, b) => a[0] - b[0])
      .map(([code, count]) => ({
        label: String(code),
        value: count,
        color:
          code >= 200 && code < 300
            ? 'chart-bar-success'
            : code >= 300 && code < 400
              ? 'chart-bar-info'
              : code >= 400 && code < 500
                ? 'chart-bar-warning'
                : 'chart-bar-error',
        onclick: () => nav('overview', { status_code: String(code) }),
      }));
  }

  const scBars = $derived(statusCodeBars(stats));

  function depthBars(s) {
    if (!s?.depth_distribution) return [];
    return Object.entries(s.depth_distribution)
      .map(([d, count]) => [Number(d), count])
      .sort((a, b) => a[0] - b[0])
      .map(([d, count]) => ({
        label: `Depth ${d}`,
        value: count,
        color: 'chart-bar-accent',
        onclick: () => nav('overview', { depth: String(d) }),
      }));
  }

  const dBars = $derived(depthBars(stats));
  const maxDepth = $derived(
    stats?.depth_distribution ? Math.max(...Object.keys(stats.depth_distribution).map(Number)) : 0,
  );
  const errorCount = $derived(
    stats?.status_codes
      ? Object.entries(stats.status_codes)
          .filter(([k]) => Number(k) >= 400 || Number(k) === 0)
          .reduce((a, [, v]) => a + v, 0)
      : stats?.error_count || 0,
  );
  const duration = $derived(
    stats?.crawl_duration_sec
      ? stats.crawl_duration_sec < 60
        ? stats.crawl_duration_sec.toFixed(0) + 's'
        : (stats.crawl_duration_sec / 60).toFixed(1) + 'min'
      : '-',
  );
</script>

{#if stats}
  {#if timelineSeries.length > 0 || recentSeries.length > 0}
    <div class="report-section">
      <div class="report-grid">
        {#if timelineSeries.length > 0}
          <div class="chart-section">
            <h3 class="chart-title">{t('report.technical.statusTimeline')}</h3>
            <AreaChart series={timelineSeries} labels={timelineLabels} height={120} yLabel={t('common.pages')} />
          </div>
        {/if}
        {#if recentSeries.length > 0}
          <div class="chart-section">
            <h3 class="chart-title">{t('report.technical.statusTimelineRecent')}</h3>
            <AreaChart series={recentSeries} labels={recentLabels} height={120} yLabel={t('common.pages')} />
          </div>
        {/if}
      </div>
    </div>
  {/if}

  <div class="report-section">
    <div class="stats-grid">
      <div
        class="stat-card stat-card-link"
        onclick={() => nav('overview')}
        role="button"
        tabindex="0"
        onkeydown={a11yKeydown(() => nav('overview'))}
      >
        <div class="stat-value">{fmtN(stats.total_pages)}</div>
        <div class="stat-label">{t('report.overview.totalPages')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        onclick={() => nav('internal')}
        role="button"
        tabindex="0"
        onkeydown={a11yKeydown(() => nav('internal'))}
      >
        <div class="stat-value">{fmtN(stats.internal_links)}</div>
        <div class="stat-label">{t('report.overview.internalLinks')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        onclick={() => nav('external')}
        role="button"
        tabindex="0"
        onkeydown={a11yKeydown(() => nav('external'))}
      >
        <div class="stat-value">{fmtN(stats.external_links)}</div>
        <div class="stat-label">{t('report.overview.externalLinks')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        onclick={() => nav('response', { status_code: '>=400' })}
        role="button"
        tabindex="0"
        onkeydown={a11yKeydown(() => nav('response', { status_code: '>=400' }))}
      >
        <div class="stat-value" style={errorCount > 0 ? 'color: var(--error)' : ''}>
          {fmtN(errorCount)}
        </div>
        <div class="stat-label">{t('report.overview.errors')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        onclick={() => nav('response')}
        role="button"
        tabindex="0"
        onkeydown={a11yKeydown(() => nav('response'))}
      >
        <div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div>
        <div class="stat-label">{t('report.overview.avgResponse')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">
          {stats.pages_per_second > 0 ? stats.pages_per_second.toFixed(1) : '-'}
        </div>
        <div class="stat-label">{t('report.overview.pagesPerSec')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{duration}</div>
        <div class="stat-label">{t('report.overview.duration')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        onclick={() => nav('overview', { depth: String(maxDepth) })}
        role="button"
        tabindex="0"
        onkeydown={a11yKeydown(() => nav('overview', { depth: String(maxDepth) }))}
      >
        <div class="stat-value">{maxDepth}</div>
        <div class="stat-label">{t('report.overview.maxDepth')}</div>
      </div>
    </div>
  </div>

  <div class="report-section">
    <div class="report-grid">
      <div class="chart-section">
        <h3 class="chart-title">{t('report.overview.statusCodes')}</h3>
        <DonutChart
          segments={scSegments}
          size={200}
          strokeWidth={28}
          centerLabel={fmtN(stats.total_pages)}
          centerSubLabel={t('common.pages')}
        />
      </div>
      <div class="chart-section">
        <h3 class="chart-title">{t('report.overview.statusCodesDetail')}</h3>
        <HBarChart bars={scBars} />
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">{t('report.overview.depthDist')}</h3>
    <HBarChart bars={dBars} />
  </div>

  {#if stats.top_pagerank?.length > 0}
    <div class="report-section">
      <h3 class="chart-title">{t('report.overview.topPageRank')}</h3>
      <div class="report-mini-table">
        <table>
          <thead
            ><tr><th>#</th><th>{t('common.url')}</th><th>{t('urlDetail.pageRank')}</th></tr></thead
          >
          <tbody>
            {#each stats.top_pagerank.slice(0, 10) as entry, i}
              <tr>
                <td class="text-muted font-semibold">{i + 1}</td>
                <td class="cell-url">
                  <a
                    href={`/sessions/${sessionId}/url/${encodeURIComponent(entry.url)}`}
                    onclick={(e) => {
                      e.preventDefault();
                      onnavigate?.(`/sessions/${sessionId}/url/${encodeURIComponent(entry.url)}`);
                    }}
                  >
                    {entry.url}
                  </a>
                </td>
                <td class="text-accent font-semibold">{entry.pagerank.toFixed(2)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
{:else}
  <p class="chart-empty">{t('report.overview.noStats')}</p>
{/if}
