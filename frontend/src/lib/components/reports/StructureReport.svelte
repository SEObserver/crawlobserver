<script>
  import { fmtN, a11yKeydown } from '../../utils.js';
  import HBarChart from '../charts/HBarChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

  function nav(tab, filters = {}) { onnavigate?.(`/sessions/${sessionId}/${tab}`, filters); }

  const structure = $derived(audit?.structure);

  function directoryBars(s) {
    if (!s?.directories) return [];
    return s.directories.map(d => ({
      label: d.directory || '/',
      value: d.count,
      color: 'chart-bar-accent',
      onclick: () => nav('overview', { url: d.directory }),
    }));
  }

  function depthBars(st) {
    if (!st?.depth_distribution) return [];
    return Object.entries(st.depth_distribution)
      .map(([d, count]) => [Number(d), count])
      .sort((a, b) => a[0] - b[0])
      .map(([d, count]) => ({
        label: `Depth ${d}`,
        value: count,
        color: 'chart-bar-accent',
        onclick: () => nav('overview', { depth: String(d) }),
      }));
  }

  const dirBars = $derived(directoryBars(structure));
  const dBars = $derived(depthBars(stats));
</script>

{#if structure}
  <div class="report-section">
    <h3 class="chart-title">Directories</h3>
    <HBarChart bars={dirBars} />
  </div>

  <div class="report-section">
    <h3 class="chart-title">Depth Distribution</h3>
    <HBarChart bars={dBars} />
  </div>

  <div class="report-section">
    <h3 class="chart-title">Orphan Pages</h3>
    <div class="stats-grid">
      <div class="stat-card stat-card-link" role="button" tabindex="0"
        onclick={() => nav('overview')} onkeydown={a11yKeydown(() => nav('overview'))}>
        <div class="stat-value" style="color: var(--warning)">{fmtN(structure.orphan_pages || 0)}</div><div class="stat-label">Orphan Pages</div>
      </div>
    </div>
  </div>
{:else}
  <p class="chart-empty">No structure audit data available.</p>
{/if}
