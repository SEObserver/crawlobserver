<script>
  import { fmtN, a11yKeydown } from '../utils.js';

  let { stats, sessionId, onnavigate } = $props();
</script>

<div class="charts-container">
  {#if stats?.depth_distribution && Object.keys(stats.depth_distribution).length > 0}
    {@const depthEntries = Object.entries(stats.depth_distribution).map(([k, v]) => [Number(k), v]).sort((a, b) => a[0] - b[0])}
    {@const depthMax = Math.max(...depthEntries.map(e => e[1]))}
    {@const depthBarH = 32}
    {@const depthGap = 6}
    {@const depthSvgH = depthEntries.length * (depthBarH + depthGap)}
    <div class="chart-section">
      <h3 class="chart-title">Depth Distribution</h3>
      <svg class="chart-svg" viewBox="0 0 600 {depthSvgH}" preserveAspectRatio="xMinYMin meet">
        {#each depthEntries as [depth, count], i}
          {@const barW = depthMax > 0 ? (count / depthMax) * 440 : 0}
          {@const y = i * (depthBarH + depthGap)}
          {@const opacity = 0.5 + (0.5 * (1 - i / Math.max(depthEntries.length - 1, 1)))}
          <g class="chart-bar-clickable" role="button" tabindex="0" style="cursor:pointer" onclick={() => onnavigate?.(`/sessions/${sessionId}/overview`, {depth: String(depth)})} onkeydown={a11yKeydown(() => onnavigate?.(`/sessions/${sessionId}/overview`, {depth: String(depth)}))}>
            <text x="30" y={y + depthBarH / 2 + 5} text-anchor="end" class="chart-label">{depth}</text>
            <rect x="40" y={y} width={Math.max(barW, 2)} height={depthBarH} rx="4" class="chart-bar chart-bar-accent" style="opacity: {opacity}" />
            <text x={44 + barW} y={y + depthBarH / 2 + 5} class="chart-value">{fmtN(count)}</text>
          </g>
        {/each}
      </svg>
    </div>
  {:else}
    <p class="chart-empty">No depth data available.</p>
  {/if}

  {#if stats?.status_codes && Object.keys(stats.status_codes).length > 0}
    {@const scEntries = Object.entries(stats.status_codes).map(([k, v]) => [Number(k), v]).sort((a, b) => a[0] - b[0])}
    {@const scMax = Math.max(...scEntries.map(e => e[1]))}
    {@const scBarH = 32}
    {@const scGap = 6}
    {@const scSvgH = scEntries.length * (scBarH + scGap)}
    <div class="chart-section">
      <h3 class="chart-title">Status Code Distribution</h3>
      <svg class="chart-svg" viewBox="0 0 600 {scSvgH}" preserveAspectRatio="xMinYMin meet">
        {#each scEntries as [code, count], i}
          {@const barW = scMax > 0 ? (count / scMax) * 440 : 0}
          {@const y = i * (scBarH + scGap)}
          {@const colorClass = code >= 200 && code < 300 ? 'chart-bar-success' : code >= 300 && code < 400 ? 'chart-bar-info' : code >= 400 && code < 500 ? 'chart-bar-warning' : 'chart-bar-error'}
          <g class="chart-bar-clickable" role="button" tabindex="0" style="cursor:pointer" onclick={() => onnavigate?.(`/sessions/${sessionId}/overview`, {status_code: String(code)})} onkeydown={a11yKeydown(() => onnavigate?.(`/sessions/${sessionId}/overview`, {status_code: String(code)}))}>
            <text x="30" y={y + scBarH / 2 + 5} text-anchor="end" class="chart-label">{code}</text>
            <rect x="40" y={y} width={Math.max(barW, 2)} height={scBarH} rx="4" class={`chart-bar ${colorClass}`} />
            <text x={44 + barW} y={y + scBarH / 2 + 5} class="chart-value">{fmtN(count)}</text>
          </g>
        {/each}
      </svg>
    </div>
  {:else}
    <p class="chart-empty">No status code data available.</p>
  {/if}

</div>
