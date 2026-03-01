<script>
  import { fmtN } from '../../utils.js';

  let {
    segments = [],
    size = 200,
    strokeWidth = 30,
    centerLabel = '',
    centerSubLabel = '',
  } = $props();

  const cx = size / 2;
  const cy = size / 2;
  const outerR = (size - 2) / 2;
  const innerR = outerR - strokeWidth;

  let hoveredIndex = $state(-1);

  function polarToXY(angle, r) {
    return [cx + r * Math.cos(angle), cy + r * Math.sin(angle)];
  }

  function computeArcs(segs) {
    const total = segs.reduce((s, seg) => s + seg.value, 0);
    if (total === 0) return [];
    let startAngle = -Math.PI / 2; // start at top
    return segs.map((seg, i) => {
      const pct = seg.value / total;
      const sweep = pct * Math.PI * 2;
      const endAngle = startAngle + sweep;
      const largeArc = sweep > Math.PI ? 1 : 0;

      // Full circle special case (single segment = 100%)
      let d;
      if (pct >= 0.9999) {
        const [ox1, oy1] = polarToXY(startAngle, outerR);
        const [ox2, oy2] = polarToXY(startAngle + Math.PI, outerR);
        const [ix1, iy1] = polarToXY(startAngle + Math.PI, innerR);
        const [ix2, iy2] = polarToXY(startAngle, innerR);
        d = `M${ox1},${oy1} A${outerR},${outerR} 0 1 1 ${ox2},${oy2} A${outerR},${outerR} 0 1 1 ${ox1},${oy1} L${ix2},${iy2} A${innerR},${innerR} 0 1 0 ${ix1},${iy1} A${innerR},${innerR} 0 1 0 ${ix2},${iy2} Z`;
      } else {
        const [ox1, oy1] = polarToXY(startAngle, outerR);
        const [ox2, oy2] = polarToXY(endAngle, outerR);
        const [ix1, iy1] = polarToXY(endAngle, innerR);
        const [ix2, iy2] = polarToXY(startAngle, innerR);
        d = `M${ox1},${oy1} A${outerR},${outerR} 0 ${largeArc} 1 ${ox2},${oy2} L${ix1},${iy1} A${innerR},${innerR} 0 ${largeArc} 0 ${ix2},${iy2} Z`;
      }

      const arc = { ...seg, pct, d, index: i };
      startAngle = endAngle;
      return arc;
    });
  }

  const arcs = $derived(computeArcs(segments));
  const total = $derived(segments.reduce((s, seg) => s + seg.value, 0));
</script>

<div class="donut-container">
  <svg width={size} height={size} viewBox="0 0 {size} {size}">
    {#each arcs as arc}
      <path
        d={arc.d}
        fill={arc.color}
        stroke="none"
        class="donut-arc"
        style="opacity: {hoveredIndex >= 0 && hoveredIndex !== arc.index ? 0.4 : 1};"
        onmouseenter={() => (hoveredIndex = arc.index)}
        onmouseleave={() => (hoveredIndex = -1)}
        onclick={() => arc.onclick?.()}
      />
    {/each}
    {#if centerLabel}
      <text
        x={cx}
        y={centerSubLabel ? cy - 6 : cy + 6}
        text-anchor="middle"
        dominant-baseline="middle"
        class="donut-center-label"
        style="font-size: {size * 0.16}px;">{centerLabel}</text
      >
    {/if}
    {#if centerSubLabel}
      <text
        x={cx}
        y={cy + 16}
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
