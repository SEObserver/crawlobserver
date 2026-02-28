<script>
  import { fmtN } from '../../utils.js';
  import DonutChart from '../charts/DonutChart.svelte';
  import HBarChart from '../charts/HBarChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

  const content = $derived(audit?.content);

  function titleSegments(c) {
    if (!c) return [];
    const ok = (c.total || 0) - (c.title_missing || 0) - (c.title_too_long || 0) - (c.title_too_short || 0);
    const segs = [];
    if (ok > 0) segs.push({ value: ok, color: 'var(--success)', label: 'OK' });
    if (c.title_missing > 0) segs.push({ value: c.title_missing, color: 'var(--error)', label: 'Missing', onclick: () => onnavigate?.(`/sessions/${sessionId}/titles`, { title: '' }) });
    if (c.title_too_long > 0) segs.push({ value: c.title_too_long, color: 'var(--warning)', label: 'Too Long (>60)' });
    if (c.title_too_short > 0) segs.push({ value: c.title_too_short, color: 'var(--info)', label: 'Too Short (<30)' });
    return segs;
  }

  function metaSegments(c) {
    if (!c) return [];
    const ok = (c.total || 0) - (c.meta_desc_missing || 0) - (c.meta_desc_too_long || 0) - (c.meta_desc_too_short || 0);
    const segs = [];
    if (ok > 0) segs.push({ value: ok, color: 'var(--success)', label: 'OK' });
    if (c.meta_desc_missing > 0) segs.push({ value: c.meta_desc_missing, color: 'var(--error)', label: 'Missing' });
    if (c.meta_desc_too_long > 0) segs.push({ value: c.meta_desc_too_long, color: 'var(--warning)', label: 'Too Long (>160)' });
    if (c.meta_desc_too_short > 0) segs.push({ value: c.meta_desc_too_short, color: 'var(--info)', label: 'Too Short (<70)' });
    return segs;
  }

  function h1Segments(c) {
    if (!c) return [];
    const ok = (c.total || 0) - (c.h1_missing || 0) - (c.h1_multiple || 0);
    const segs = [];
    if (ok > 0) segs.push({ value: ok, color: 'var(--success)', label: 'OK (1 H1)' });
    if (c.h1_missing > 0) segs.push({ value: c.h1_missing, color: 'var(--error)', label: 'Missing' });
    if (c.h1_multiple > 0) segs.push({ value: c.h1_multiple, color: 'var(--warning)', label: 'Multiple' });
    return segs;
  }

  function thinContentBars(c) {
    if (!c) return [];
    const bars = [];
    if (c.thin_under_100 > 0) bars.push({ label: '<100 words', value: c.thin_under_100, color: 'chart-bar-error' });
    if (c.thin_100_300 > 0) bars.push({ label: '100-300', value: c.thin_100_300, color: 'chart-bar-warning' });
    const over300 = (c.total || 0) - (c.thin_under_100 || 0) - (c.thin_100_300 || 0);
    if (over300 > 0) bars.push({ label: '300+', value: over300, color: 'chart-bar-success' });
    return bars;
  }

  const tSegs = $derived(titleSegments(content));
  const mSegs = $derived(metaSegments(content));
  const h1Segs = $derived(h1Segments(content));
  const thinBars = $derived(thinContentBars(content));
</script>

{#if content}
  <div class="report-section">
    <h3 class="chart-title">Titles</h3>
    <div class="report-grid">
      <DonutChart segments={tSegs} size={200} strokeWidth={28}
        centerLabel={fmtN(content.total)} centerSubLabel="pages" />
      <div>
        <div class="stats-grid" style="margin-bottom: 16px;">
          <div class="stat-card"><div class="stat-value" style="color: var(--error)">{fmtN(content.title_missing)}</div><div class="stat-label">Missing</div></div>
          <div class="stat-card"><div class="stat-value" style="color: var(--warning)">{fmtN(content.title_too_long)}</div><div class="stat-label">Too Long</div></div>
          <div class="stat-card"><div class="stat-value" style="color: var(--info)">{fmtN(content.title_too_short)}</div><div class="stat-label">Too Short</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(content.title_duplicates || 0)}</div><div class="stat-label">Duplicates</div></div>
        </div>
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">Meta Descriptions</h3>
    <div class="report-grid">
      <DonutChart segments={mSegs} size={200} strokeWidth={28}
        centerLabel={fmtN(content.total)} centerSubLabel="pages" />
      <div class="stats-grid">
        <div class="stat-card"><div class="stat-value" style="color: var(--error)">{fmtN(content.meta_desc_missing)}</div><div class="stat-label">Missing</div></div>
        <div class="stat-card"><div class="stat-value" style="color: var(--warning)">{fmtN(content.meta_desc_too_long)}</div><div class="stat-label">Too Long</div></div>
        <div class="stat-card"><div class="stat-value" style="color: var(--info)">{fmtN(content.meta_desc_too_short)}</div><div class="stat-label">Too Short</div></div>
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">H1 Tags</h3>
    <DonutChart segments={h1Segs} size={180} strokeWidth={24}
      centerLabel={fmtN(content.total)} centerSubLabel="pages" />
  </div>

  <div class="report-section">
    <h3 class="chart-title">Content Length</h3>
    <HBarChart bars={thinBars} />
  </div>

  <div class="report-section">
    <h3 class="chart-title">Images</h3>
    <div class="stats-grid">
      <div class="stat-card"><div class="stat-value">{fmtN(content.images_total)}</div><div class="stat-label">Total Images</div></div>
      <div class="stat-card"><div class="stat-value" style="color: var(--warning)">{fmtN(content.images_no_alt_total)}</div><div class="stat-label">Without Alt</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(content.pages_with_images_no_alt)}</div><div class="stat-label">Pages Affected</div></div>
    </div>
  </div>
{:else}
  <p class="chart-empty">No content audit data available.</p>
{/if}
