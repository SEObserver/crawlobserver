<script>
  import { fmtN } from '../../utils.js';
  import DonutChart from '../charts/DonutChart.svelte';
  import HBarChart from '../charts/HBarChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

  const links = $derived(audit?.links);

  function extSegments(l) {
    if (!l) return [];
    const segs = [];
    if (l.external_dofollow > 0) segs.push({ value: l.external_dofollow, color: 'var(--success)', label: 'Dofollow' });
    if (l.external_nofollow > 0) segs.push({ value: l.external_nofollow, color: 'var(--warning)', label: 'Nofollow' });
    return segs;
  }

  function topDomainBars(l) {
    if (!l?.top_external_domains) return [];
    return l.top_external_domains.map(d => ({
      label: d.domain,
      value: d.count,
      color: 'chart-bar-info',
    }));
  }

  function anchorBars(l) {
    if (!l?.top_anchors) return [];
    return l.top_anchors.map(a => ({
      label: a.anchor || '(empty)',
      value: a.count,
      color: 'chart-bar-accent',
    }));
  }

  const eSegs = $derived(extSegments(links));
  const domBars = $derived(topDomainBars(links));
  const aBars = $derived(anchorBars(links));
</script>

{#if links}
  <div class="report-section">
    <h3 class="chart-title">Internal Links</h3>
    <div class="stats-grid">
      <div class="stat-card"><div class="stat-value">{fmtN(links.total_internal)}</div><div class="stat-label">Total Internal</div></div>
      <div class="stat-card"><div class="stat-value" style="color: var(--warning)">{fmtN(links.pages_no_internal_out || 0)}</div><div class="stat-label">Pages No Outlinks</div></div>
      <div class="stat-card"><div class="stat-value" style="color: var(--warning)">{fmtN(links.pages_high_internal_out || 0)}</div><div class="stat-label">Pages >100 Outlinks</div></div>
      <div class="stat-card"><div class="stat-value" style="color: var(--error)">{fmtN(links.broken_internal || 0)}</div><div class="stat-label">Broken Internal</div></div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">External Links</h3>
    <div class="report-grid">
      <div>
        <DonutChart segments={eSegs} size={180} strokeWidth={24}
          centerLabel={fmtN(links.total_external)} centerSubLabel="external" />
      </div>
      <div class="stats-grid">
        <div class="stat-card"><div class="stat-value">{fmtN(links.total_external)}</div><div class="stat-label">Total External</div></div>
        <div class="stat-card"><div class="stat-value">{fmtN(links.pages_no_external || 0)}</div><div class="stat-label">Pages No External</div></div>
      </div>
    </div>
  </div>

  {#if domBars.length > 0}
    <div class="report-section">
      <h3 class="chart-title">Top External Domains</h3>
      <HBarChart bars={domBars} />
    </div>
  {/if}

  {#if aBars.length > 0}
    <div class="report-section">
      <h3 class="chart-title">Top Internal Anchor Texts</h3>
      <HBarChart bars={aBars} />
    </div>
  {/if}
{:else}
  <p class="chart-empty">No links audit data available.</p>
{/if}
