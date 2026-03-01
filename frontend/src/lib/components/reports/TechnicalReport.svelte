<script>
  import { fmtN, fmt, a11yKeydown } from '../../utils.js';
  import { t } from '../../i18n/index.svelte.js';
  import DonutChart from '../charts/DonutChart.svelte';
  import HBarChart from '../charts/HBarChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

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
            <div class="stat-value text-success">{fmtN(tech.indexable)}</div>
            <div class="stat-label">{t('report.technical.indexable')}</div>
          </div>
          <div
            class="stat-card stat-card-link"
            role="button"
            tabindex="0"
            onclick={() => nav('indexability', { is_indexable: 'false' })}
            onkeydown={a11yKeydown(() => nav('indexability', { is_indexable: 'false' }))}
          >
            <div class="stat-value text-error">{fmtN(tech.non_indexable)}</div>
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
          <div class="stat-value text-success">{fmtN(tech.canonical_self)}</div>
          <div class="stat-label">{t('report.technical.selfCanonical')}</div>
        </div>
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('indexability', { canonical_is_self: 'false' })}
          onkeydown={a11yKeydown(() => nav('indexability', { canonical_is_self: 'false' }))}
        >
          <div class="stat-value text-info">{fmtN(tech.canonical_other)}</div>
          <div class="stat-label">{t('report.technical.otherCanonical')}</div>
        </div>
        <div
          class="stat-card stat-card-link"
          role="button"
          tabindex="0"
          onclick={() => nav('indexability', { canonical: '' })}
          onkeydown={a11yKeydown(() => nav('indexability', { canonical: '' }))}
        >
          <div class="stat-value text-warning">{fmtN(tech.canonical_missing)}</div>
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
        <div class="stat-value">{fmtN(tech.has_redirect || 0)}</div>
        <div class="stat-label">{t('report.technical.pagesWithRedirect')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        role="button"
        tabindex="0"
        onclick={() => nav('response', { status_code: '3' })}
        onkeydown={a11yKeydown(() => nav('response', { status_code: '3' }))}
      >
        <div class="stat-value text-warning">{fmtN(tech.redirect_chains_over_2 || 0)}</div>
        <div class="stat-label">{t('report.technical.chainsOver2')}</div>
      </div>
      <div
        class="stat-card stat-card-link"
        role="button"
        tabindex="0"
        onclick={() => nav('response', { status_code: '5' })}
        onkeydown={a11yKeydown(() => nav('response', { status_code: '5' }))}
      >
        <div class="stat-value text-error">{fmtN(tech.error_pages || 0)}</div>
        <div class="stat-label">{t('report.technical.errorPages')}</div>
      </div>
    </div>
  </div>

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
          <div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div>
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
