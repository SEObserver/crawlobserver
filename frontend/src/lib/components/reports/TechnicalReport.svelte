<script>
  import { fmtN, fmt, a11yKeydown } from '../../utils.js';
  import DonutChart from '../charts/DonutChart.svelte';
  import HBarChart from '../charts/HBarChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

  function nav(tab, filters = {}) { onnavigate?.(`/sessions/${sessionId}/${tab}`, filters); }

  const tech = $derived(audit?.technical);

  function indexSegments(t) {
    if (!t) return [];
    const segs = [];
    if (t.indexable > 0) segs.push({ value: t.indexable, color: 'var(--success)', label: 'Indexable', onclick: () => nav('indexability', { is_indexable: 'true' }) });
    if (t.non_indexable > 0) segs.push({ value: t.non_indexable, color: 'var(--error)', label: 'Non-Indexable', onclick: () => nav('indexability', { is_indexable: 'false' }) });
    return segs;
  }

  function canonicalSegments(t) {
    if (!t) return [];
    const segs = [];
    if (t.canonical_self > 0) segs.push({ value: t.canonical_self, color: 'var(--success)', label: 'Self', onclick: () => nav('indexability', { canonical_is_self: 'true' }) });
    if (t.canonical_other > 0) segs.push({ value: t.canonical_other, color: 'var(--info)', label: 'Other', onclick: () => nav('indexability', { canonical_is_self: 'false' }) });
    if (t.canonical_missing > 0) segs.push({ value: t.canonical_missing, color: 'var(--warning)', label: 'Missing', onclick: () => nav('indexability', { canonical: '' }) });
    return segs;
  }

  function noindexBars(t) {
    if (!t?.noindex_reasons) return [];
    return t.noindex_reasons.map(r => ({
      label: r.reason || '(empty)',
      value: r.count,
      color: 'chart-bar-error',
      onclick: () => nav('indexability', { index_reason: r.reason }),
    }));
  }

  function responseBars(t) {
    if (!t) return [];
    const bars = [];
    if (t.response_fast > 0) bars.push({ label: '<200ms', value: t.response_fast, color: 'chart-bar-success', onclick: () => nav('response', { fetch_duration_ms: '<200' }) });
    if (t.response_ok > 0) bars.push({ label: '200-500ms', value: t.response_ok, color: 'chart-bar-accent', onclick: () => nav('response', { fetch_duration_ms: '200-500' }) });
    if (t.response_slow > 0) bars.push({ label: '500ms-1s', value: t.response_slow, color: 'chart-bar-warning', onclick: () => nav('response', { fetch_duration_ms: '500-1000' }) });
    if (t.response_very_slow > 0) bars.push({ label: '>1s', value: t.response_very_slow, color: 'chart-bar-error', onclick: () => nav('response', { fetch_duration_ms: '>1000' }) });
    return bars;
  }

  function contentTypeBars(t) {
    if (!t?.content_types) return [];
    return t.content_types.map(ct => ({
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
    <h3 class="chart-title">Indexability</h3>
    <div class="report-grid">
      <DonutChart segments={idxSegs} size={200} strokeWidth={28}
        centerLabel={fmtN((tech.indexable || 0) + (tech.non_indexable || 0))} centerSubLabel="pages" />
      <div>
        <div class="stats-grid" style="margin-bottom: 16px;">
          <div class="stat-card stat-card-link" role="button" tabindex="0"
            onclick={() => nav('indexability', { is_indexable: 'true' })} onkeydown={a11yKeydown(() => nav('indexability', { is_indexable: 'true' }))}>
            <div class="stat-value" style="color: var(--success)">{fmtN(tech.indexable)}</div><div class="stat-label">Indexable</div>
          </div>
          <div class="stat-card stat-card-link" role="button" tabindex="0"
            onclick={() => nav('indexability', { is_indexable: 'false' })} onkeydown={a11yKeydown(() => nav('indexability', { is_indexable: 'false' }))}>
            <div class="stat-value" style="color: var(--error)">{fmtN(tech.non_indexable)}</div><div class="stat-label">Non-Indexable</div>
          </div>
        </div>
        {#if niBars.length > 0}
          <h4 style="font-size: 14px; color: var(--text-secondary); margin-bottom: 12px;">Non-Indexable Reasons</h4>
          <HBarChart bars={niBars} />
        {/if}
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">Canonical Tags</h3>
    <div class="report-grid">
      <DonutChart segments={canSegs} size={180} strokeWidth={24} />
      <div class="stats-grid">
        <div class="stat-card stat-card-link" role="button" tabindex="0"
          onclick={() => nav('indexability', { canonical_is_self: 'true' })} onkeydown={a11yKeydown(() => nav('indexability', { canonical_is_self: 'true' }))}>
          <div class="stat-value" style="color: var(--success)">{fmtN(tech.canonical_self)}</div><div class="stat-label">Self-canonical</div>
        </div>
        <div class="stat-card stat-card-link" role="button" tabindex="0"
          onclick={() => nav('indexability', { canonical_is_self: 'false' })} onkeydown={a11yKeydown(() => nav('indexability', { canonical_is_self: 'false' }))}>
          <div class="stat-value" style="color: var(--info)">{fmtN(tech.canonical_other)}</div><div class="stat-label">Other canonical</div>
        </div>
        <div class="stat-card stat-card-link" role="button" tabindex="0"
          onclick={() => nav('indexability', { canonical: '' })} onkeydown={a11yKeydown(() => nav('indexability', { canonical: '' }))}>
          <div class="stat-value" style="color: var(--warning)">{fmtN(tech.canonical_missing)}</div><div class="stat-label">Missing</div>
        </div>
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">Redirects</h3>
    <div class="stats-grid">
      <div class="stat-card stat-card-link" role="button" tabindex="0"
        onclick={() => nav('response', { status_code: '3' })} onkeydown={a11yKeydown(() => nav('response', { status_code: '3' }))}>
        <div class="stat-value">{fmtN(tech.has_redirect || 0)}</div><div class="stat-label">Pages with Redirect</div>
      </div>
      <div class="stat-card stat-card-link" role="button" tabindex="0"
        onclick={() => nav('response', { status_code: '3' })} onkeydown={a11yKeydown(() => nav('response', { status_code: '3' }))}>
        <div class="stat-value" style="color: var(--warning)">{fmtN(tech.redirect_chains_over_2 || 0)}</div><div class="stat-label">Chains > 2 hops</div>
      </div>
      <div class="stat-card stat-card-link" role="button" tabindex="0"
        onclick={() => nav('response', { status_code: '5' })} onkeydown={a11yKeydown(() => nav('response', { status_code: '5' }))}>
        <div class="stat-value" style="color: var(--error)">{fmtN(tech.error_pages || 0)}</div><div class="stat-label">Error Pages</div>
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">Response Time</h3>
    <HBarChart bars={resBars} />
    {#if stats}
      <div class="stats-grid" style="margin-top: 16px;">
        <div class="stat-card stat-card-link" role="button" tabindex="0"
          onclick={() => nav('response')} onkeydown={a11yKeydown(() => nav('response'))}>
          <div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">Average</div>
        </div>
      </div>
    {/if}
  </div>

  <div class="report-section">
    <h3 class="chart-title">Content Types</h3>
    <HBarChart bars={ctBars} />
  </div>
{:else}
  <p class="chart-empty">No technical audit data available.</p>
{/if}
