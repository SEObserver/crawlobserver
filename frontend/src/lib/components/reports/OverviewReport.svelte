<script>
  import { fmtN, fmt, a11yKeydown } from '../../utils.js';
  import DonutChart from '../charts/DonutChart.svelte';
  import HBarChart from '../charts/HBarChart.svelte';

  let { stats, sessionId, onnavigate } = $props();

  // Health score computation
  function computeHealth(s) {
    if (!s || !s.total_pages || s.total_pages === 0) return 0;
    const total = s.total_pages;
    const pct200 = (s.status_codes?.[200] || 0) / total;
    const noErrors = 1 - (s.error_count || 0) / total;
    const speedOK = Math.max(0, Math.min(1, 1 - (s.avg_fetch_ms || 0) / 2000));
    let depthOK = 0;
    if (s.depth_distribution) {
      const shallow = Object.entries(s.depth_distribution)
        .filter(([d]) => Number(d) <= 3)
        .reduce((sum, [, c]) => sum + c, 0);
      depthOK = shallow / total;
    }
    return Math.round((pct200 * 40 + noErrors * 20 + speedOK * 20 + depthOK * 20));
  }

  const healthScore = $derived(computeHealth(stats));
  const healthColor = $derived(healthScore > 80 ? 'var(--success)' : healthScore >= 50 ? 'var(--warning)' : 'var(--error)');

  // Status code grouped segments for donut
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
    if (groups['2xx'] > 0) segs.push({ value: groups['2xx'], color: 'var(--success)', label: '2xx Success', onclick: () => onnavigate?.(`/sessions/${sessionId}/overview`, { status_code: '2' }) });
    if (groups['3xx'] > 0) segs.push({ value: groups['3xx'], color: 'var(--info)', label: '3xx Redirect', onclick: () => onnavigate?.(`/sessions/${sessionId}/overview`, { status_code: '3' }) });
    if (groups['4xx'] > 0) segs.push({ value: groups['4xx'], color: 'var(--warning)', label: '4xx Client Error', onclick: () => onnavigate?.(`/sessions/${sessionId}/overview`, { status_code: '4' }) });
    if (groups['5xx'] > 0) segs.push({ value: groups['5xx'], color: 'var(--error)', label: '5xx Server Error', onclick: () => onnavigate?.(`/sessions/${sessionId}/overview`, { status_code: '5' }) });
    return segs;
  }

  const scSegments = $derived(statusCodeSegments(stats));

  // Status code bars (detailed)
  function statusCodeBars(s) {
    if (!s?.status_codes) return [];
    return Object.entries(s.status_codes)
      .map(([code, count]) => [Number(code), count])
      .sort((a, b) => a[0] - b[0])
      .map(([code, count]) => ({
        label: String(code),
        value: count,
        color: code >= 200 && code < 300 ? 'chart-bar-success' : code >= 300 && code < 400 ? 'chart-bar-info' : code >= 400 && code < 500 ? 'chart-bar-warning' : 'chart-bar-error',
        onclick: () => onnavigate?.(`/sessions/${sessionId}/overview`, { status_code: String(code) }),
      }));
  }

  const scBars = $derived(statusCodeBars(stats));

  // Depth bars
  function depthBars(s) {
    if (!s?.depth_distribution) return [];
    return Object.entries(s.depth_distribution)
      .map(([d, count]) => [Number(d), count])
      .sort((a, b) => a[0] - b[0])
      .map(([d, count], i, arr) => ({
        label: `Depth ${d}`,
        value: count,
        color: 'chart-bar-accent',
        onclick: () => onnavigate?.(`/sessions/${sessionId}/overview`, { depth: String(d) }),
      }));
  }

  const dBars = $derived(depthBars(stats));

  // Stat computations
  const maxDepth = $derived(stats?.depth_distribution ? Math.max(...Object.keys(stats.depth_distribution).map(Number)) : 0);
  const non200 = $derived(stats?.status_codes ? Object.entries(stats.status_codes).filter(([k]) => k !== '200').reduce((a, [, v]) => a + v, 0) : (stats?.error_count || 0));
  const duration = $derived(stats?.crawl_duration_sec ? (stats.crawl_duration_sec < 60 ? stats.crawl_duration_sec.toFixed(0) + 's' : (stats.crawl_duration_sec / 60).toFixed(1) + 'min') : '-');
</script>

{#if stats}
  <!-- Health Score -->
  <div class="report-section">
    <h3 class="chart-title">Health Score</h3>
    <div class="health-score">
      <DonutChart
        segments={[
          { value: healthScore, color: healthColor, label: 'Score' },
          { value: 100 - healthScore, color: 'var(--border)', label: 'Remaining' },
        ]}
        size={220}
        strokeWidth={24}
        centerLabel="{healthScore}%"
        centerSubLabel="Health"
      />
    </div>
  </div>

  <!-- Stat Cards -->
  <div class="report-section">
    <div class="stats-grid">
      <div class="stat-card"><div class="stat-value">{fmtN(stats.total_pages)}</div><div class="stat-label">Total Pages</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(stats.internal_links)}</div><div class="stat-label">Internal Links</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(stats.external_links)}</div><div class="stat-label">External Links</div></div>
      <div class="stat-card"><div class="stat-value" style={non200 > 0 ? 'color: var(--error)' : ''}>{fmtN(non200)}</div><div class="stat-label">Errors</div></div>
      <div class="stat-card"><div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">Avg Response</div></div>
      <div class="stat-card"><div class="stat-value">{stats.pages_per_second > 0 ? stats.pages_per_second.toFixed(1) : '-'}</div><div class="stat-label">Pages/sec</div></div>
      <div class="stat-card"><div class="stat-value">{duration}</div><div class="stat-label">Duration</div></div>
      <div class="stat-card"><div class="stat-value">{maxDepth}</div><div class="stat-label">Max Depth</div></div>
    </div>
  </div>

  <!-- Status Codes + Depth -->
  <div class="report-section">
    <div class="report-grid">
      <div class="chart-section">
        <h3 class="chart-title">Status Codes</h3>
        <DonutChart segments={scSegments} size={200} strokeWidth={28} centerLabel={fmtN(stats.total_pages)} centerSubLabel="pages" />
      </div>
      <div class="chart-section">
        <h3 class="chart-title">Status Codes Detail</h3>
        <HBarChart bars={scBars} />
      </div>
    </div>
  </div>

  <!-- Depth Distribution -->
  <div class="report-section">
    <h3 class="chart-title">Depth Distribution</h3>
    <HBarChart bars={dBars} />
  </div>

  <!-- Top PageRank -->
  {#if stats.top_pagerank?.length > 0}
    <div class="report-section">
      <h3 class="chart-title">Top PageRank</h3>
      <div class="report-mini-table">
        <table>
          <thead>
            <tr><th>#</th><th>URL</th><th>PageRank</th></tr>
          </thead>
          <tbody>
            {#each stats.top_pagerank.slice(0, 10) as entry, i}
              <tr>
                <td style="color: var(--text-muted); font-weight: 600;">{i + 1}</td>
                <td class="cell-url">
                  <a href={`/sessions/${sessionId}/url/${encodeURIComponent(entry.url)}`}
                    onclick={(e) => { e.preventDefault(); onnavigate?.(`/sessions/${sessionId}/url/${encodeURIComponent(entry.url)}`); }}>
                    {entry.url}
                  </a>
                </td>
                <td style="color: var(--accent); font-weight: 600;">{entry.pagerank.toFixed(2)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
{:else}
  <p class="chart-empty">No stats available. Run a crawl first.</p>
{/if}
