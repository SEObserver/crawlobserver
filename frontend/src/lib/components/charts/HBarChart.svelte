<script>
  import { fmtN, a11yKeydown } from '../../utils.js';

  let { bars = [], maxValue = 0, barHeight = 32, gap = 6, labelWidth = 180 } = $props();

  const effectiveMax = $derived(maxValue > 0 ? maxValue : Math.max(...bars.map((b) => b.value), 1));
  const svgH = $derived(bars.length * (barHeight + gap));
  const barStartX = $derived(labelWidth + 10);
  const barMaxW = $derived(600 - barStartX - 60);

  function truncate(s, max) {
    if (!s) return '';
    return s.length > max ? s.slice(0, max - 1) + '…' : s;
  }

  // With barHeight=32 and typical font size, ~24 chars fit comfortably in 180px.
  const labelMaxChars = $derived(Math.floor(labelWidth / 7.5));
</script>

{#if bars.length > 0}
  <svg class="chart-svg" viewBox="0 0 600 {svgH}" preserveAspectRatio="xMinYMin meet">
    {#each bars as bar, i}
      {@const barW = effectiveMax > 0 ? (bar.value / effectiveMax) * barMaxW : 0}
      {@const y = i * (barHeight + gap)}
      {@const colorClass = bar.color || 'chart-bar-accent'}
      {@const display = truncate(bar.label, labelMaxChars)}
      {#if bar.onclick}
        <g
          class="chart-bar-clickable"
          role="button"
          tabindex="0"
          onclick={() => bar.onclick()}
          onkeydown={a11yKeydown(() => bar.onclick())}
        >
          <text x={labelWidth} y={y + barHeight / 2 + 5} text-anchor="end" class="chart-label"
            >{display}<title>{bar.label}</title></text
          >
          <rect
            x={barStartX}
            {y}
            width={Math.max(barW, 2)}
            height={barHeight}
            rx="4"
            class="chart-bar {colorClass}"
          />
          <text x={barStartX + 4 + barW} y={y + barHeight / 2 + 5} class="chart-value"
            >{fmtN(bar.value)}</text
          >
        </g>
      {:else}
        <g>
          <text x={labelWidth} y={y + barHeight / 2 + 5} text-anchor="end" class="chart-label"
            >{display}<title>{bar.label}</title></text
          >
          <rect
            x={barStartX}
            {y}
            width={Math.max(barW, 2)}
            height={barHeight}
            rx="4"
            class="chart-bar {colorClass}"
          />
          <text x={barStartX + 4 + barW} y={y + barHeight / 2 + 5} class="chart-value"
            >{fmtN(bar.value)}</text
          >
        </g>
      {/if}
    {/each}
  </svg>
{:else}
  <p class="chart-empty">No data available.</p>
{/if}
