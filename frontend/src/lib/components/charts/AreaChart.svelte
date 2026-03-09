<script>
  import { fmtN } from '../../utils.js';

  let { series = [], labels = [], height = 220, yLabel = '' } = $props();

  // series: [{ key, label, color, values: number[] }]
  // labels: string[] (x-axis labels, same length as values)

  const W = 700;
  const ML = 50; // margin left
  const MR = 16;
  const MT = 12;
  const MB = 36;
  const chartW = W - ML - MR;
  const chartH = $derived(height - MT - MB);

  // Stack values: for each x, compute cumulative y
  const stacked = $derived.by(() => {
    if (!series.length || !series[0].values?.length) return [];
    const n = series[0].values.length;
    const layers = [];
    for (let si = 0; si < series.length; si++) {
      const layer = {
        key: series[si].key,
        color: series[si].color,
        label: series[si].label,
        opacity: series[si].opacity,
        points: [],
      };
      for (let i = 0; i < n; i++) {
        const y0 = si > 0 ? layers[si - 1].points[i].y1 : 0;
        const y1 = y0 + (series[si].values[i] || 0);
        layer.points.push({ y0, y1 });
      }
      layers.push(layer);
    }
    return layers;
  });

  const maxY = $derived.by(() => {
    if (!stacked.length) return 1;
    const top = stacked[stacked.length - 1];
    return Math.max(...top.points.map((p) => p.y1), 1);
  });

  const nPoints = $derived(series[0]?.values?.length || 0);

  function x(i) {
    if (nPoints <= 1) return ML + chartW / 2;
    return ML + (i / (nPoints - 1)) * chartW;
  }
  function y(val) {
    return MT + chartH - (val / maxY) * chartH;
  }

  function areaPath(layer) {
    const pts = layer.points;
    if (!pts.length) return '';
    let d = `M${x(0)},${y(pts[0].y1)}`;
    for (let i = 1; i < pts.length; i++) d += `L${x(i)},${y(pts[i].y1)}`;
    for (let i = pts.length - 1; i >= 0; i--) d += `L${x(i)},${y(pts[i].y0)}`;
    d += 'Z';
    return d;
  }

  // Y-axis grid lines (4 ticks)
  const yTicks = $derived.by(() => {
    const ticks = [];
    for (let i = 0; i <= 4; i++) {
      const val = Math.round((maxY / 4) * i);
      ticks.push(val);
    }
    return ticks;
  });

  // X-axis labels: show ~6 evenly spaced
  const xLabelIndices = $derived.by(() => {
    if (nPoints <= 6) return Array.from({ length: nPoints }, (_, i) => i);
    const step = (nPoints - 1) / 5;
    return Array.from({ length: 6 }, (_, i) => Math.round(step * i));
  });

  // Tooltip state
  let hoverIdx = $state(-1);
  let tooltipX = $state(0);
  let tooltipLeft = $state(0);
  let tooltipTop = $state(0);
  let wrapperEl = $state(null);

  function onMouseMove(e) {
    const svg = e.currentTarget;
    const rect = svg.getBoundingClientRect();
    const mx = ((e.clientX - rect.left) / rect.width) * W;
    const idx = Math.round(((mx - ML) / chartW) * (nPoints - 1));
    if (idx >= 0 && idx < nPoints) {
      hoverIdx = idx;
      tooltipX = x(idx);
      // Position HTML tooltip relative to wrapper
      const pxPerUnit = rect.width / W;
      const lineX = tooltipX * pxPerUnit;
      tooltipLeft = lineX > rect.width / 2 ? lineX - 140 : lineX + 12;
      tooltipTop = MT * pxPerUnit;
    } else {
      hoverIdx = -1;
    }
  }
</script>

{#if nPoints > 0}
  <div class="area-chart-wrapper" bind:this={wrapperEl}>
    <svg
      class="area-chart"
      viewBox="0 0 {W} {height}"
      preserveAspectRatio="xMinYMin meet"
      onmousemove={onMouseMove}
      onmouseleave={() => (hoverIdx = -1)}
      role="img"
    >
      <!-- Y grid -->
      {#each yTicks as tick}
        <line x1={ML} y1={y(tick)} x2={W - MR} y2={y(tick)} class="grid-line" />
        <text x={ML - 6} y={y(tick) + 4} text-anchor="end" class="axis-label">{fmtN(tick)}</text>
      {/each}

      <!-- Stacked areas (bottom to top) -->
      {#each stacked as layer}
        <path d={areaPath(layer)} fill={layer.color} opacity={layer.opacity ?? 0.75} />
      {/each}

      <!-- X axis labels -->
      {#each xLabelIndices as idx}
        <text x={x(idx)} y={height - 4} text-anchor="middle" class="axis-label"
          >{labels[idx] || ''}</text
        >
      {/each}

      <!-- Hover line -->
      {#if hoverIdx >= 0}
        <line x1={tooltipX} y1={MT} x2={tooltipX} y2={MT + chartH} class="hover-line" />
      {/if}

      <!-- Y label -->
      {#if yLabel}
        <text
          x={12}
          y={MT + chartH / 2}
          transform="rotate(-90,12,{MT + chartH / 2})"
          class="axis-label y-label">{yLabel}</text
        >
      {/if}
    </svg>

    <!-- HTML tooltip -->
    {#if hoverIdx >= 0}
      {@const mainRows = [...series].filter(s => !s.key?.startsWith('retried_') && (s.values[hoverIdx] || 0) > 0).reverse()}
      {@const retryRows = [...series].filter(s => s.key?.startsWith('retried_') && (s.values[hoverIdx] || 0) > 0).reverse()}
      <div class="chart-tooltip" style="left:{tooltipLeft}px;top:{tooltipTop}px">
        <div class="chart-tooltip-title">{labels[hoverIdx] || ''}</div>
        {#each mainRows as s}
          <div class="chart-tooltip-row">
            <span class="chart-tooltip-swatch" style="background:{s.color}"></span>
            <span class="chart-tooltip-label">{s.label}</span>
            <span class="chart-tooltip-value">{fmtN(s.values[hoverIdx] || 0)}</span>
          </div>
        {/each}
        {#if retryRows.length > 0}
          <div class="chart-tooltip-divider"></div>
          {#each retryRows as s}
            <div class="chart-tooltip-row">
              <span class="chart-tooltip-swatch" style="background:{s.color};opacity:0.5"></span>
              <span class="chart-tooltip-label">{s.label}</span>
              <span class="chart-tooltip-value">{fmtN(s.values[hoverIdx] || 0)}</span>
            </div>
          {/each}
        {/if}
      </div>
    {/if}
  </div>

  <!-- Legend -->
  <div class="area-legend">
    {#each [...series].reverse() as s}
      <span class="legend-item">
        <span class="legend-swatch" style="background:{s.color}"></span>
        {s.label}
      </span>
    {/each}
  </div>
{/if}

<style>
  .area-chart-wrapper {
    position: relative;
  }
  .area-chart {
    width: 100%;
    display: block;
    overflow: visible;
  }
  .grid-line {
    stroke: var(--border);
    stroke-width: 0.5;
    stroke-dasharray: 3 3;
  }
  .axis-label {
    font-size: 10px;
    fill: var(--text-muted);
    font-family: inherit;
  }
  .y-label {
    font-size: 11px;
  }
  .hover-line {
    stroke: var(--text-muted);
    stroke-width: 1;
    stroke-dasharray: 4 2;
  }

  /* HTML tooltip */
  .chart-tooltip {
    position: absolute;
    z-index: 50;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm, 4px);
    box-shadow: var(--shadow-md, 0 4px 12px rgba(0,0,0,0.15));
    padding: 8px 10px;
    pointer-events: none;
    white-space: nowrap;
    font-size: 12px;
    line-height: 1.5;
    color: var(--text);
  }
  .chart-tooltip-title {
    font-weight: 600;
    font-size: 11px;
    color: var(--text-secondary);
    margin-bottom: 4px;
  }
  .chart-tooltip-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .chart-tooltip-label {
    flex: 1;
  }
  .chart-tooltip-value {
    font-variant-numeric: tabular-nums;
    font-weight: 600;
    text-align: right;
  }
  .chart-tooltip-swatch {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 2px;
    flex-shrink: 0;
  }
  .chart-tooltip-divider {
    height: 1px;
    background: var(--border);
    margin: 4px 0;
  }

  .area-legend {
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
    margin-top: 8px;
    font-size: 12px;
    color: var(--text-secondary);
  }
  .legend-item {
    display: flex;
    align-items: center;
    gap: 5px;
  }
  .legend-swatch {
    width: 10px;
    height: 10px;
    border-radius: 2px;
    flex-shrink: 0;
  }
</style>
