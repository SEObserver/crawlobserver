<script>
  import { onMount } from 'svelte';
  import { fmtN, fmt, a11yKeydown } from '../../utils.js';
  import { t } from '../../i18n/index.svelte.js';
  import { getStatusTimeline, getStatusTimelineRecent } from '../../api.js';
  import DonutChart from '../charts/DonutChart.svelte';
  import HBarChart from '../charts/HBarChart.svelte';
  import AreaChart from '../charts/AreaChart.svelte';
  import AnimatedNumber from '../AnimatedNumber.svelte';

  let { stats, audit, sessionId, isRunning = false, onnavigate, statsVersion = 0 } = $props();

  // Status timeline charts
  let timeline = $state(null);
  let recentTimeline = $state(null);
  let timelineLoading = $state(false);
  let pollTimer = null; // NOT $state — avoids read/write loop in $effect

  function fetchTimeline() {
    if (!sessionId || timelineLoading) return;
    timelineLoading = true;
    Promise.all([getStatusTimeline(sessionId), getStatusTimelineRecent(sessionId)])
      .then(([data, recent]) => {
        timeline = data ?? [];
        recentTimeline = recent ?? [];
      })
      .catch((err) => {
        console.error('[TechnicalReport] timeline fetch failed:', err);
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

  // Refresh when the server signals new data via SSE
  $effect(() => {
    if (statsVersion > 0 && isRunning && sessionId) {
      fetchTimeline();
    }
  });

  // Slow fallback poll (30s instead of 5s)
  $effect(() => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    if (isRunning && sessionId) {
      pollTimer = setInterval(fetchTimeline, 30_000);
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

  const tech = $derived(audit?.technical);

  function indexSegments(d) {
    if (!d) return [];
    const segs = [];
    if (d.indexable > 0)
      segs.push({
        value: d.indexable,
        color: 'var(--success)',
        label: t('report.technical.indexable'),
        onclick: () => nav('indexability', { is_indexable: 'true' }),
      });
    if (d.non_indexable > 0)
      segs.push({
        value: d.non_indexable,
        color: 'var(--error)',
        label: t('report.technical.nonIndexable'),
        onclick: () => nav('indexability', { is_indexable: 'false' }),
      });
    return segs;
  }

  function canonicalSegments(d) {
    if (!d) return [];
    const segs = [];
    if (d.canonical_self > 0)
      segs.push({
        value: d.canonical_self,
        color: 'var(--success)',
        label: t('report.technical.self'),
        onclick: () => nav('indexability', { canonical_is_self: 'true' }),
      });
    if (d.canonical_other > 0)
      segs.push({
        value: d.canonical_other,
        color: 'var(--info)',
        label: t('report.technical.other'),
        onclick: () => nav('indexability', { canonical_is_self: 'false' }),
      });
    if (d.canonical_missing > 0)
      segs.push({
        value: d.canonical_missing,
        color: 'var(--warning)',
        label: t('report.technical.missing'),
        onclick: () => nav('indexability', { canonical: '' }),
      });
    return segs;
  }

  function noindexBars(d) {
    if (!d?.noindex_reasons) return [];
    return d.noindex_reasons.map((r) => ({
      label: r.reason || '(empty)',
      value: r.count,
      color: 'chart-bar-error',
      onclick: () => nav('indexability', { index_reason: r.reason }),
    }));
  }

  function responseBars(d) {
    if (!d) return [];
    const bars = [];
    if (d.response_fast > 0)
      bars.push({
        label: '<200ms',
        value: d.response_fast,
        color: 'chart-bar-success',
        onclick: () => nav('response', { fetch_duration_ms: '<200' }),
      });
    if (d.response_ok > 0)
      bars.push({
        label: '200-500ms',
        value: d.response_ok,
        color: 'chart-bar-accent',
        onclick: () => nav('response', { fetch_duration_ms: '200-500' }),
      });
    if (d.response_slow > 0)
      bars.push({
        label: '500ms-1s',
        value: d.response_slow,
        color: 'chart-bar-warning',
        onclick: () => nav('response', { fetch_duration_ms: '500-1000' }),
      });
    if (d.response_very_slow > 0)
      bars.push({
        label: '>1s',
        value: d.response_very_slow,
        color: 'chart-bar-error',
        onclick: () => nav('response', { fetch_duration_ms: '>1000' }),
      });
    return bars;
  }

  function contentTypeBars(d) {
    if (!d?.content_types) return [];
    return d.content_types.map((ct) => ({
      label: ct.content_type || '(empty)',
      value: ct.count,
      color: 'chart-bar-accent',
      onclick: () => nav('response', { content_type: ct.content_type }),
    }));
  }

  const idxSegs = $derived(indexSegments(tech));
  const canSegs = $derived(canonicalSegments(tech));
  const niBars = $derived(noindexBars(tech));
  const resBars = $derived(responseBars(tech));
  const ctBars = $derived(contentTypeBars(tech));
</script>

{#if tech}
  <div class="report-section">
    <h3 class="chart-title">{t('report.technical.indexability')}</h3>
    <div class="report-grid">
      <DonutChart
        segments={idxSegs}
        size={200}
        strokeWidth={28}
        centerLabel={fmtN((tech.indexable || 0) + (tech.non_indexable || 0))}
        centerSubLabel={t('common.pages')}
      />
      <div>
        <div class="stats-grid mb-md">
          <div
            class="stat-card stat-card-link"
            role="button"
            tabindex="0"
            onclick={() => nav('indexability', { is_indexable: 'true' })}
            onkeydown={a11yKeydown(() => nav('indexability', { is_indexable: 'true' }))}
          >
            <div class="stat-value text-success"><AnimatedNumber value={tech.indexable} /></div>
            <div class="stat-label">{t('report.technical.indexable')}</div>
          </div>
          <div
            class="stat-card stat-card-link"
            role="button"
            tabindex="0"
            onclick={() => nav('indexability', { is_indexable: 'false' })}
            onkeydown={a11yKeydown(() => nav('indexability', { is_indexable: 'false' }))}
          >
            <div class="stat-value text-error"><AnimatedNumber value={tech.non_indexable} /></div>
            <div class="stat-label">{t('report.technical.nonIndexable')}</div>
          </div>
        </div>
        {#if niBars.length > 0}
          <h4 class="noindex-heading">{t('report.technical.nonIndexableReasons')}</h4>
          <HBarChart bars={niBars} />
        {/if}
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">{t('report.technical.canonicalTags')}</h3>
    <div class="report-grid">
      <DonutChart segments={canSegs} size={180} strokeWidth={24} />
      <div class="stats-grid">
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('indexability', { canonical_is_self: 'true' })}
          onkeydown={a11yKeydown(() => nav('indexability', { canonical_is_self: 'true' }))}
        >
          <div class="stat-value text-success"><AnimatedNumber value={tech.canonical_self} /></div>
          <div class="stat-label">{t('report.technical.selfCanonical')}</div>
        </div>
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('indexability', { canonical_is_self: 'false' })}
          onkeydown={a11yKeydown(() => nav('indexability', { canonical_is_self: 'false' }))}
        >
          <div class="stat-value text-info"><AnimatedNumber value={tech.canonical_other} /></div>
          <div class="stat-label">{t('report.technical.otherCanonical')}</div>
        </div>
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('indexability', { canonical: '' })}
          onkeydown={a11yKeydown(() => nav('indexability', { canonical: '' }))}
        >
          <div class="stat-value text-warning">
            <AnimatedNumber value={tech.canonical_missing} />
          </div>
          <div class="stat-label">{t('report.technical.missing')}</div>
        </div>
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">{t('report.technical.redirects')}</h3>
    <div class="stats-grid">
      <div
        class="stat-card stat-card-link"
        role="button"
        tabindex="0"
        onclick={() => nav('response', { status_code: '3' })}
        onkeydown={a11yKeydown(() => nav('response', { status_code: '3' }))}
      >
        <div class="stat-value"><AnimatedNumber value={tech.has_redirect || 0} /></div>
        <div class="stat-label">{t('report.technical.pagesWithRedirect')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        role="button"
        tabindex="0"
        onclick={() => nav('response', { status_code: '3' })}
        onkeydown={a11yKeydown(() => nav('response', { status_code: '3' }))}
      >
        <div class="stat-value text-warning">
          <AnimatedNumber value={tech.redirect_chains_over_2 || 0} />
        </div>
        <div class="stat-label">{t('report.technical.chainsOver2')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        role="button"
        tabindex="0"
        onclick={() => nav('response', { status_code: '5' })}
        onkeydown={a11yKeydown(() => nav('response', { status_code: '5' }))}
      >
        <div class="stat-value text-error"><AnimatedNumber value={tech.error_pages || 0} /></div>
        <div class="stat-label">{t('report.technical.errorPages')}</div>
      </div>
    </div>
  </div>

  {#if timelineSeries.length > 0 || recentSeries.length > 0}
    <div class="report-section">
      <div class="report-grid">
        {#if timelineSeries.length > 0}
          <div class="chart-section">
            <h3 class="chart-title">{t('report.technical.statusTimeline')}</h3>
            <AreaChart
              series={timelineSeries}
              labels={timelineLabels}
              height={120}
              yLabel={t('common.pages')}
            />
          </div>
        {/if}
        {#if recentSeries.length > 0}
          <div class="chart-section">
            <h3 class="chart-title">{t('report.technical.statusTimelineRecent')}</h3>
            <AreaChart
              series={recentSeries}
              labels={recentLabels}
              height={120}
              yLabel={t('common.pages')}
            />
          </div>
        {/if}
      </div>
    </div>
  {/if}

  <div class="report-section">
    <h3 class="chart-title">{t('report.technical.responseTime')}</h3>
    <HBarChart bars={resBars} />
    {#if stats}
      <div class="stats-grid mt-md">
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('response')}
          onkeydown={a11yKeydown(() => nav('response'))}
        >
          <div class="stat-value">
            <AnimatedNumber value={Math.round(stats.avg_fetch_ms)} format={fmt} />
          </div>
          <div class="stat-label">{t('report.technical.average')}</div>
        </div>
      </div>
    {/if}
  </div>

  <div class="report-section">
    <h3 class="chart-title">{t('report.technical.contentTypes')}</h3>
    <HBarChart bars={ctBars} />
  </div>
{:else}
  <p class="chart-empty">{t('report.technical.noData')}</p>
{/if}

<style>
  .noindex-heading {
    font-size: 14px;
    color: var(--text-secondary);
    margin-bottom: 12px;
  }
</style>
