<script>
  import { fmtN } from '../../utils.js';

  let {
    segments = [],
    size = 200,
    strokeWidth = 30,
    centerLabel = '',
    centerSubLabel = '',
  } = $props();

  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;

  let hoveredIndex = $state(-1);

  function computeArcs(segs) {
    const total = segs.reduce((s, seg) => s + seg.value, 0);
    if (total === 0) return [];
    let offset = 0;
    return segs.map((seg, i) => {
      const pct = seg.value / total;
      const dash = pct * circumference;
      const arc = { ...seg, pct, dash, gap: circumference - dash, offset, index: i };
      offset += dash;
      return arc;
    });
  }

  const arcs = $derived(computeArcs(segments));
  const total = $derived(segments.reduce((s, seg) => s + seg.value, 0));
</script>

<div class="donut-container">
  <svg width={size} height={size} viewBox="0 0 {size} {size}">
    {#each arcs as arc}
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill="none"
        stroke={arc.color}
        stroke-width={strokeWidth}
        stroke-dasharray="{arc.dash} {arc.gap}"
        stroke-dashoffset={-arc.offset}
        transform="rotate(-90 {size / 2} {size / 2})"
        class="donut-arc"
        style="opacity: {hoveredIndex >= 0 && hoveredIndex !== arc.index ? 0.4 : 1};"
        onmouseenter={() => (hoveredIndex = arc.index)}
        onmouseleave={() => (hoveredIndex = -1)}
        onclick={() => arc.onclick?.()}
      />
    {/each}
    {#if centerLabel}
      <text
        x={size / 2}
        y={centerSubLabel ? size / 2 - 6 : size / 2 + 6}
        text-anchor="middle"
        dominant-baseline="middle"
        class="donut-center-label"
        style="font-size: {size * 0.16}px;">{centerLabel}</text
      >
    {/if}
    {#if centerSubLabel}
      <text
        x={size / 2}
        y={size / 2 + 16}
        text-anchor="middle"
        dominant-baseline="middle"
        class="donut-center-sublabel"
        style="font-size: {size * 0.07}px;">{centerSubLabel}</text
      >
    {/if}
  </svg>

  {#if hoveredIndex >= 0 && arcs[hoveredIndex]}
    {@const h = arcs[hoveredIndex]}
    <div class="donut-tooltip">
      <strong>{h.label}</strong>: {fmtN(h.value)} ({(h.pct * 100).toFixed(1)}%)
    </div>
  {/if}

  <div class="donut-legend">
    {#each arcs as arc}
      <div
        class="donut-legend-item donut-legend-item-interactive"
        onmouseenter={() => (hoveredIndex = arc.index)}
        onmouseleave={() => (hoveredIndex = -1)}
        style="opacity: {hoveredIndex >= 0 && hoveredIndex !== arc.index ? 0.5 : 1};"
      >
        <span class="donut-legend-color" style="background: {arc.color};"></span>
        <span>{arc.label}</span>
        <span class="donut-legend-value">{fmtN(arc.value)}</span>
      </div>
    {/each}
  </div>
</div>

<style>
  .donut-arc {
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .donut-center-label {
    font-weight: 700;
    fill: var(--text);
  }
  .donut-center-sublabel {
    font-weight: 500;
    fill: var(--text-muted);
  }
  .donut-legend-item-interactive {
    cursor: pointer;
  }
  .donut-legend-value {
    color: var(--text-muted);
    margin-left: auto;
  }
</style>
