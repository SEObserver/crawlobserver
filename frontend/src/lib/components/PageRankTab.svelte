<script>
  import { getPageRankTop, getPageRankTreemap, getPageRankDistribution } from '../api.js';
  import { fmtN, statusBadge, trunc, a11yKeydown, squarify } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { sessionId, initialSubView = 'top', onnavigate, onpushurl, onerror } = $props();

  let prSubView = $state(initialSubView);
  let prLoading = $state(false);
  let prTopData = $state(null);
  let prTopLimit = $state(50);
  let prTopOffset = $state(0);
  let prDistData = $state(null);
  let prTreemapData = $state(null);
  let prTreemapDepth = $state(2);
  let prTreemapMinPages = $state(1);
  let prTableData = $state(null);
  let prTableOffset = $state(0);
  let prTableDir = $state('');
  let prTooltip = $state(null);

  async function loadPRSubView(view) {
    prLoading = true;
    try {
      if (view === 'top') {
        prTopData = await getPageRankTop(sessionId, prTopLimit, prTopOffset);
      } else if (view === 'directory') {
        prTreemapData = await getPageRankTreemap(sessionId, prTreemapDepth, prTreemapMinPages);
      } else if (view === 'distribution') {
        prDistData = await getPageRankDistribution(sessionId, 20);
      } else if (view === 'table') {
        prTableData = await getPageRankTop(sessionId, 50, prTableOffset, prTableDir);
      }
    } catch (e) {
      onerror?.(e.message);
    } finally {
      prLoading = false;
    }
  }

  function switchPRSubView(view) {
    prSubView = view;
    if (view === 'top') { prTopOffset = 0; }
    if (view === 'table') { prTableOffset = 0; }
    onpushurl?.(`/sessions/${sessionId}/pagerank/${view}`);
    loadPRSubView(view);
  }

  function prDrillToTable(dir) {
    prTableDir = dir;
    prTableOffset = 0;
    prSubView = 'table';
    onpushurl?.(`/sessions/${sessionId}/pagerank/table`);
    loadPRSubView('table');
  }

  function prDrillHistToTable(minPR, maxPR) {
    prTableDir = '';
    prTableOffset = 0;
    prSubView = 'table';
    onpushurl?.(`/sessions/${sessionId}/pagerank/table`);
    loadPRSubView('table');
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(url);
  }

  // Load initial view
  loadPRSubView(prSubView);
</script>

<div class="pr-container">
  <div class="pr-subview-bar">
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'top'} onclick={() => switchPRSubView('top')}>{t('pagerank.topPages')}</button>
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'directory'} onclick={() => switchPRSubView('directory')}>{t('pagerank.byDirectory')}</button>
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'distribution'} onclick={() => switchPRSubView('distribution')}>{t('pagerank.distribution')}</button>
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'table'} onclick={() => switchPRSubView('table')}>{t('pagerank.fullTable')}</button>
  </div>

  {#if prLoading}
    <p class="loading-msg">{t('common.loading')}</p>

  {:else if prSubView === 'top'}
    {#if prTopData?.pages?.length > 0}
      <div class="pr-controls">
        <label>{t('pagerank.show')}</label>
        <select class="pr-select" value={prTopLimit} onchange={(e) => { prTopLimit = Number(e.target.value); prTopOffset = 0; loadPRSubView('top'); }}>
          <option value={20}>20</option>
          <option value={50}>50</option>
          <option value={100}>100</option>
        </select>
        <span class="text-muted text-xs">{t('pagerank.ofPagesWithPR', { total: fmtN(prTopData.total) })}</span>
      </div>
      {@const maxPR = prTopData.pages[0]?.pagerank || 1}
      {#each prTopData.pages as p, i}
        <div class="pr-top-row pr-top-row-clickable" role="button" tabindex="0"
          onclick={() => goToUrlDetail({preventDefault:()=>{}}, p.url)}
          onkeydown={a11yKeydown(() => goToUrlDetail({preventDefault:()=>{}}, p.url))}
          onmouseenter={(e) => { prTooltip = { x: e.clientX, y: e.clientY, url: p.url, pr: p.pagerank, depth: p.depth, intLinks: p.internal_links_out, extLinks: p.external_links_out, words: p.word_count }; }}
          onmouseleave={() => { prTooltip = null; }}>
          <span class="pr-top-rank">{prTopOffset + i + 1}</span>
          <span class="pr-top-url">{p.url.replace(/^https?:\/\/[^/]+/, '') || '/'}</span>
          <div>
            <div class="pr-top-bar" style="width: {(p.pagerank / maxPR) * 100}%; opacity: {0.4 + 0.6 * (p.pagerank / maxPR)};"></div>
          </div>
          <span class="pr-top-score">{p.pagerank.toFixed(1)}</span>
          <div class="pr-top-badges">
            <span class="pr-top-badge">D{p.depth}</span>
            <span class="pr-top-badge">{p.internal_links_out}int</span>
          </div>
        </div>
      {/each}
      {#if prTopData.total > prTopLimit}
        <div class="pagination">
          <button class="btn btn-sm" disabled={prTopOffset === 0} onclick={() => { prTopOffset = Math.max(0, prTopOffset - prTopLimit); loadPRSubView('top'); }}>{t('common.previous')}</button>
          <span class="pagination-info">{prTopOffset + 1} - {Math.min(prTopOffset + prTopLimit, prTopData.total)} of {fmtN(prTopData.total)}</span>
          <button class="btn btn-sm" disabled={prTopOffset + prTopLimit >= prTopData.total} onclick={() => { prTopOffset += prTopLimit; loadPRSubView('top'); }}>{t('common.next')}</button>
        </div>
      {/if}
    {:else}
      <p class="chart-empty">{t('pagerank.noData')}</p>
    {/if}

  {:else if prSubView === 'directory'}
    {#if prTreemapData?.length > 0}
      <div class="pr-controls">
        <label>{t('urlDetail.depth')}</label>
        <select class="pr-select" value={prTreemapDepth} onchange={(e) => { prTreemapDepth = Number(e.target.value); loadPRSubView('directory'); }}>
          <option value={1}>1</option>
          <option value={2}>2</option>
          <option value={3}>3</option>
        </select>
        <label>{t('pagerank.minPages')}</label>
        <select class="pr-select" value={prTreemapMinPages} onchange={(e) => { prTreemapMinPages = Number(e.target.value); loadPRSubView('directory'); }}>
          <option value={1}>1</option>
          <option value={5}>5</option>
          <option value={10}>10</option>
          <option value={25}>25</option>
        </select>
        <span class="text-muted text-xs">{t('pagerank.directories', { count: prTreemapData.length })}</span>
      </div>
      {@const treemapItems = prTreemapData.map(d => ({ ...d, value: d.total_pr }))}
      {@const treemapRects = squarify(treemapItems, 0, 0, 100, 100)}
      {@const maxAvgPR = Math.max(...prTreemapData.map(d => d.avg_pr), 1)}
      <div class="pr-treemap-container">
        {#each treemapRects as rect}
          {@const opacity = 0.35 + 0.65 * (rect.avg_pr / maxAvgPR)}
          <div class="pr-treemap-rect" role="button" tabindex="0"
            style="left: {rect.x}%; top: {rect.y}%; width: {rect.w}%; height: {rect.h}%; background: var(--accent); opacity: {opacity};"
            onclick={() => prDrillToTable(rect.path)}
            onkeydown={a11yKeydown(() => prDrillToTable(rect.path))}
            onmouseenter={(e) => { prTooltip = { x: e.clientX, y: e.clientY, path: rect.path, pages: rect.page_count, totalPR: rect.total_pr, avgPR: rect.avg_pr, maxPR: rect.max_pr }; }}
            onmouseleave={() => { prTooltip = null; }}>
            {#if rect.w > 6 && rect.h > 5}
              <div class="pr-treemap-label">
                {rect.path || '/'}
                {#if rect.w > 10 && rect.h > 8}
                  <small>{rect.page_count} {t('common.pages')} &middot; {t('pagerank.avg')} {rect.avg_pr.toFixed(1)}</small>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {:else}
      <p class="chart-empty">{t('pagerank.noData')}</p>
    {/if}

  {:else if prSubView === 'distribution'}
    {#if prDistData && prDistData.total_with_pr > 0}
      <div class="stats-grid pr-stats-grid">
        <div class="stat-card"><div class="stat-value">{fmtN(prDistData.total_with_pr)}</div><div class="stat-label">{t('pagerank.pagesWithPR')}</div></div>
        <div class="stat-card"><div class="stat-value">{prDistData.avg.toFixed(2)}</div><div class="stat-label">{t('pagerank.mean')}</div></div>
        <div class="stat-card"><div class="stat-value">{prDistData.median.toFixed(2)}</div><div class="stat-label">{t('pagerank.median')}</div></div>
        <div class="stat-card"><div class="stat-value">{prDistData.p90.toFixed(2)}</div><div class="stat-label">P90</div></div>
        <div class="stat-card"><div class="stat-value">{prDistData.p99.toFixed(2)}</div><div class="stat-label">P99</div></div>
      </div>
      {@const distBuckets = prDistData.buckets || []}
      {@const distMaxCount = Math.max(...distBuckets.map(b => b.count), 1)}
      {@const histW = 600}
      {@const histH = 300}
      {@const histMargin = { top: 20, right: 20, bottom: 40, left: 60 }}
      {@const plotW = histW - histMargin.left - histMargin.right}
      {@const plotH = histH - histMargin.top - histMargin.bottom}
      {@const barGap = 1}
      {@const barW = distBuckets.length > 0 ? (plotW - (distBuckets.length - 1) * barGap) / distBuckets.length : 0}
      {@const logMax = Math.log10(distMaxCount + 1)}
      <svg viewBox="0 0 {histW} {histH}" class="pr-hist-svg">
        {#each [1, 10, 100, 1000, 10000, 100000] as tick}
          {#if tick <= distMaxCount * 1.5}
            {@const ty = histMargin.top + plotH - (logMax > 0 ? (Math.log10(tick + 1) / logMax) * plotH : 0)}
            <line x1={histMargin.left} y1={ty} x2={histW - histMargin.right} y2={ty} stroke="var(--border)" stroke-dasharray="3,3" />
            <text x={histMargin.left - 8} y={ty + 4} text-anchor="end" class="pr-axis-tick">{tick >= 1000 ? (tick/1000) + 'k' : tick}</text>
          {/if}
        {/each}
        {#each distBuckets as bucket, i}
          {@const barH = logMax > 0 ? (Math.log10(bucket.count + 1) / logMax) * plotH : 0}
          {@const bx = histMargin.left + i * (barW + barGap)}
          {@const by = histMargin.top + plotH - barH}
          {@const opacity = 0.4 + 0.6 * (bucket.count / distMaxCount)}
          <rect class="pr-hist-bar" role="button" tabindex="0" x={bx} y={by} width={barW} height={barH} rx="2" fill="var(--accent)" opacity={opacity}
            onmouseenter={(e) => { prTooltip = { x: e.clientX, y: e.clientY, bucketMin: bucket.min, bucketMax: bucket.max, count: bucket.count, avgPR: bucket.avg_pr }; }}
            onmouseleave={() => { prTooltip = null; }}
            onclick={() => prDrillHistToTable(bucket.min, bucket.max)}
            onkeydown={a11yKeydown(() => prDrillHistToTable(bucket.min, bucket.max))} />
          {#if distBuckets.length <= 25 || i % Math.ceil(distBuckets.length / 10) === 0}
            <text x={bx + barW / 2} y={histH - histMargin.bottom + 16} text-anchor="middle" class="pr-axis-label-sm">{bucket.min.toFixed(0)}</text>
          {/if}
        {/each}
        <text x={histW / 2} y={histH - 4} text-anchor="middle" class="pr-axis-label">{t('pagerank.score')}</text>
        <text x={14} y={histH / 2} text-anchor="middle" transform="rotate(-90, 14, {histH / 2})" class="pr-axis-label">{t('pagerank.pagesLog')}</text>
      </svg>
    {:else}
      <p class="chart-empty">{t('pagerank.noData')}</p>
    {/if}

  {:else if prSubView === 'table'}
    {#if prTableData}
      <div class="pr-controls">
        <label>{t('pagerank.directoryFilter')}</label>
        <input class="pr-dir-filter" type="text" placeholder={t('pagerank.filterPlaceholder')} bind:value={prTableDir} onkeydown={(e) => { if (e.key === 'Enter') { prTableOffset = 0; loadPRSubView('table'); } }} />
        <button class="btn btn-sm" onclick={() => { prTableOffset = 0; loadPRSubView('table'); }}>{t('common.filter')}</button>
        {#if prTableDir}
          <button class="btn btn-sm" onclick={() => { prTableDir = ''; prTableOffset = 0; loadPRSubView('table'); }}>{t('pagerank.clear')}</button>
        {/if}
        <span class="text-muted text-xs">{t('pagerank.pagesCount', { count: fmtN(prTableData.total) })}</span>
      </div>
      <table>
        <thead>
          <tr><th>#</th><th>{t('common.url')}</th><th>{t('urlDetail.pageRank')}</th><th>{t('urlDetail.depth')}</th><th>{t('pagerank.intLinks')}</th><th>{t('pagerank.extLinks')}</th><th>{t('session.words')}</th><th>{t('common.status')}</th><th>{t('session.title')}</th></tr>
        </thead>
        <tbody>
          {#each prTableData.pages || [] as p, i}
            <tr>
              <td class="row-num">{prTableOffset + i + 1}</td>
              <td class="cell-url"><a href="#detail" onclick={(e) => goToUrlDetail(e, p.url)}>{p.url}</a></td>
              <td class="text-accent font-semibold">{p.pagerank.toFixed(1)}</td>
              <td>{p.depth}</td>
              <td>{fmtN(p.internal_links_out)}</td>
              <td>{fmtN(p.external_links_out)}</td>
              <td>{fmtN(p.word_count)}</td>
              <td><span class="badge {statusBadge(p.status_code)}">{p.status_code}</span></td>
              <td class="cell-title">{trunc(p.title, 50)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      {#if prTableData.total > 50}
        <div class="pagination">
          <button class="btn btn-sm" disabled={prTableOffset === 0} onclick={() => { prTableOffset = Math.max(0, prTableOffset - 50); loadPRSubView('table'); }}>{t('common.previous')}</button>
          <span class="pagination-info">{prTableOffset + 1} - {Math.min(prTableOffset + 50, prTableData.total)} of {fmtN(prTableData.total)}</span>
          <button class="btn btn-sm" disabled={prTableOffset + 50 >= prTableData.total} onclick={() => { prTableOffset += 50; loadPRSubView('table'); }}>{t('common.next')}</button>
        </div>
      {/if}
    {:else}
      <p class="chart-empty">{t('pagerank.noData')}</p>
    {/if}
  {/if}
</div>

{#if prTooltip}
  <div class="pr-tooltip" style="left: {prTooltip.x + 12}px; top: {prTooltip.y - 10}px;">
    {#if prTooltip.url}
      <div class="pr-tooltip-title">{prTooltip.url}</div>
      <div class="pr-tooltip-row"><span>{t('urlDetail.pageRank')}</span><span>{prTooltip.pr.toFixed(2)}</span></div>
      <div class="pr-tooltip-row"><span>{t('urlDetail.depth')}</span><span>{prTooltip.depth}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.intLinks')}</span><span>{fmtN(prTooltip.intLinks)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.extLinks')}</span><span>{fmtN(prTooltip.extLinks)}</span></div>
      <div class="pr-tooltip-row"><span>{t('session.words')}</span><span>{fmtN(prTooltip.words)}</span></div>
    {:else if prTooltip.path !== undefined}
      <div class="pr-tooltip-title">{prTooltip.path || '/'}</div>
      <div class="pr-tooltip-row"><span>{t('common.pages')}</span><span>{fmtN(prTooltip.pages)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.totalPR')}</span><span>{prTooltip.totalPR.toFixed(1)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.avgPR')}</span><span>{prTooltip.avgPR.toFixed(2)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.maxPR')}</span><span>{prTooltip.maxPR.toFixed(2)}</span></div>
    {:else if prTooltip.bucketMin !== undefined}
      <div class="pr-tooltip-title">PR {prTooltip.bucketMin.toFixed(1)} - {prTooltip.bucketMax.toFixed(1)}</div>
      <div class="pr-tooltip-row"><span>{t('common.pages')}</span><span>{fmtN(prTooltip.count)}</span></div>
      <div class="pr-tooltip-row"><span>{t('pagerank.avgPR')}</span><span>{prTooltip.avgPR.toFixed(2)}</span></div>
    {/if}
  </div>
{/if}

<style>
  .pr-select {
    padding: 5px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
    color: var(--text);
    font-size: 13px;
    font-family: inherit;
  }
  .pr-controls {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
    flex-wrap: wrap;
  }
  .pr-controls label {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
  }
  .pr-top-row {
    display: grid;
    grid-template-columns: 36px 1fr 200px 50px 90px;
    align-items: center;
    gap: 10px;
    padding: 6px 0;
    border-bottom: 1px solid var(--border-light);
    transition: background 0.1s;
    font-size: 13px;
  }
  .pr-top-row:hover { background: var(--bg-hover); }
  .pr-top-rank {
    text-align: right;
    font-weight: 700;
    color: var(--text-muted);
    font-size: 12px;
  }
  .pr-top-url {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--accent);
    cursor: pointer;
  }
  .pr-top-url:hover { color: var(--accent-hover); }
  .pr-top-bar {
    height: 22px;
    border-radius: 4px;
    background: var(--accent);
    transition: width 0.3s ease;
  }
  .pr-top-score {
    text-align: right;
    font-weight: 600;
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }
  .pr-top-badges {
    display: flex;
    gap: 4px;
  }
  .pr-top-badge {
    font-size: 11px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--bg);
    color: var(--text-muted);
    font-weight: 500;
    white-space: nowrap;
  }
  .pr-treemap-container {
    position: relative;
    width: 100%;
    height: 500px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  .pr-treemap-rect {
    position: absolute;
    border: 1px solid var(--bg-card);
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .pr-treemap-rect:hover { opacity: 0.85; }
  .pr-treemap-label {
    font-size: 11px;
    font-weight: 600;
    color: #fff;
    text-align: center;
    padding: 4px;
    line-height: 1.2;
    text-shadow: 0 1px 2px rgba(0,0,0,0.4);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
  .pr-treemap-label small {
    display: block;
    font-size: 10px;
    font-weight: 400;
    opacity: 0.85;
  }
  .pr-tooltip {
    position: fixed;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 10px 14px;
    font-size: 12px;
    box-shadow: var(--shadow-md);
    z-index: 2000;
    pointer-events: none;
    max-width: 360px;
  }
  .pr-tooltip-title {
    font-weight: 600;
    margin-bottom: 4px;
    color: var(--text);
    word-break: break-all;
  }
  .pr-tooltip-row {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    color: var(--text-secondary);
  }
  .pr-tooltip-row span:last-child {
    font-weight: 600;
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }
  .pr-hist-bar {
    transition: opacity 0.15s;
    cursor: pointer;
  }
  .pr-hist-bar:hover { opacity: 0.75; }
  .pr-dir-filter {
    padding: 7px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
    color: var(--text);
    font-size: 13px;
    font-family: inherit;
    width: 300px;
    max-width: 100%;
  }
  .pr-dir-filter:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-light);
  }
  .pr-top-row-clickable {
    cursor: pointer;
  }
  .pr-stats-grid {
    margin-bottom: 20px;
  }
  .pr-hist-svg {
    width: 100%;
    max-width: 700px;
    height: auto;
  }
  .pr-axis-tick {
    font-size: 10px;
    fill: var(--text-muted);
  }
  .pr-axis-label-sm {
    font-size: 9px;
    fill: var(--text-muted);
  }
  .pr-axis-label {
    font-size: 11px;
    fill: var(--text-muted);
  }
</style>