<script>
  import { fmtN, a11yKeydown } from '../../utils.js';

  let { bars = [], maxValue = 0, barHeight = 32, gap = 6 } = $props();

  const effectiveMax = $derived(maxValue > 0 ? maxValue : Math.max(...bars.map(b => b.value), 1));
  const svgH = $derived(bars.length * (barHeight + gap));
</script>

{#if bars.length > 0}
  <svg class="chart-svg" viewBox="0 0 600 {svgH}" preserveAspectRatio="xMinYMin meet">
    {#each bars as bar, i}
      {@const barW = effectiveMax > 0 ? (bar.value / effectiveMax) * 440 : 0}
      {@const y = i * (barHeight + gap)}
      {@const colorClass = bar.color || 'chart-bar-accent'}
      {#if bar.onclick}
        <g class="chart-bar-clickable" role="button" tabindex="0"
          onclick={() => bar.onclick()} onkeydown={a11yKeydown(() => bar.onclick())}>
          <text x="90" y={y + barHeight / 2 + 5} text-anchor="end" class="chart-label">{bar.label}</text>
          <rect x="100" y={y} width={Math.max(barW, 2)} height={barHeight} rx="4" class="chart-bar {colorClass}" />
          <text x={104 + barW} y={y + barHeight / 2 + 5} class="chart-value">{fmtN(bar.value)}</text>
        </g>
      {:else}
        <g>
          <text x="90" y={y + barHeight / 2 + 5} text-anchor="end" class="chart-label">{bar.label}</text>
          <rect x="100" y={y} width={Math.max(barW, 2)} height={barHeight} rx="4" class="chart-bar {colorClass}" />
          <text x={104 + barW} y={y + barHeight / 2 + 5} class="chart-value">{fmtN(bar.value)}</text>
        </g>
      {/if}
    {/each}
  </svg>
{:else}
  <p class="chart-empty">No data available.</p>
{/if}
