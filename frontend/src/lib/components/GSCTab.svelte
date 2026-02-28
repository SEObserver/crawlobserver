<script>
  import { onDestroy } from 'svelte';
  import { getGSCStatus, startGSCAuthorize, fetchGSCData, stopGSCFetch, disconnectGSC,
    getGSCOverview, getGSCQueries, getGSCPages, getGSCCountries, getGSCDevices,
    getGSCTimeline, getGSCInspection } from '../api.js';
  import { fmtN } from '../utils.js';

  let { projectId, initialSubView = 'overview', onerror, onpushurl } = $props();

  let subView = $state(initialSubView);
  let loading = $state(false);
  let status = $state(null);
  let overview = $state(null);
  let queries = $state(null);
  let queriesOffset = $state(0);
  let pages = $state(null);
  let pagesOffset = $state(0);
  let countries = $state(null);
  let devices = $state(null);
  let timeline = $state(null);
  let inspection = $state(null);
  let inspectionOffset = $state(0);
  let fetchingData = $state(false);
  let fetchStatus = $state(null);
  let selectedProperty = $state('');
  let pollTimer = null;
  const PAGE_LIMIT = 100;

  async function loadStatus() {
    if (!projectId) return;
    try {
      status = await getGSCStatus(projectId);
      // Track fetch status from server
      if (status.fetch_status?.fetching) {
        fetchingData = true;
        fetchStatus = status.fetch_status;
        startPolling();
      } else if (fetchingData && !status.fetch_status?.fetching) {
        // Fetch just completed
        fetchingData = false;
        fetchStatus = null;
        stopPolling();
        loadSubView(subView);
      }
    } catch (e) {
      status = { connected: false };
    }
  }

  function startPolling() {
    if (pollTimer) return;
    pollTimer = setInterval(async () => {
      await loadStatus();
      // Refresh data view while fetching
      if (fetchingData) {
        await loadSubView(subView);
      }
    }, 5000);
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  onDestroy(() => stopPolling());

  async function authorize() {
    try {
      const data = await startGSCAuthorize(projectId);
      if (data.url) window.location.href = data.url;
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function doFetch(propertyUrl = '') {
    fetchingData = true;
    fetchStatus = { fetching: true, rows_so_far: 0 };
    try {
      await fetchGSCData(projectId, propertyUrl);
      startPolling();
    } catch (e) {
      onerror?.(e.message);
      fetchingData = false;
      fetchStatus = null;
    }
  }

  async function selectPropertyAndFetch() {
    if (!selectedProperty) return;
    await doFetch(selectedProperty);
    await loadStatus();
  }

  async function doStop() {
    try {
      await stopGSCFetch(projectId);
      fetchingData = false;
      fetchStatus = null;
      stopPolling();
      loadSubView(subView);
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function doDisconnect() {
    try {
      await disconnectGSC(projectId);
      stopPolling();
      status = { connected: false };
      fetchingData = false;
      fetchStatus = null;
      overview = null;
      queries = null;
      pages = null;
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function loadSubView(view) {
    if (!fetchingData) loading = true;
    try {
      if (view === 'overview') {
        const [ov, tl] = await Promise.all([
          getGSCOverview(projectId),
          getGSCTimeline(projectId),
        ]);
        overview = ov;
        timeline = tl;
      } else if (view === 'queries') {
        queries = await getGSCQueries(projectId, PAGE_LIMIT, queriesOffset);
      } else if (view === 'pages') {
        pages = await getGSCPages(projectId, PAGE_LIMIT, pagesOffset);
      } else if (view === 'countries') {
        const [c, d] = await Promise.all([
          getGSCCountries(projectId),
          getGSCDevices(projectId),
        ]);
        countries = c;
        devices = d;
      } else if (view === 'inspection') {
        inspection = await getGSCInspection(projectId, PAGE_LIMIT, inspectionOffset);
      }
    } catch (e) {
      // No data yet is OK
      if (view === 'overview') { overview = null; timeline = null; }
    } finally {
      loading = false;
    }
  }

  function switchSubView(view) {
    subView = view;
    if (view === 'queries') queriesOffset = 0;
    if (view === 'pages') pagesOffset = 0;
    if (view === 'inspection') inspectionOffset = 0;
    onpushurl?.(`/projects/${projectId}/gsc/${view}`);
    loadSubView(view);
  }

  // Init
  loadStatus();
  if (projectId) loadSubView(subView);
</script>

<div class="pr-container">
  {#if !projectId}
    <div class="gsc-empty">
      <p>This session is not associated with a project.</p>
      <p class="text-muted text-sm">Associate it with a project first, then connect Google Search Console.</p>
    </div>

  {:else if !status}
    <p class="loading-msg">Loading...</p>

  {:else if !status.connected}
    <div class="gsc-empty">
      <h3 class="gsc-connect-title">Connect Google Search Console</h3>
      <p class="text-muted text-sm mb-md">
        Link this project to GSC to see search queries, impressions, clicks, and indexation status.
      </p>
      <button class="btn btn-primary" onclick={authorize}>Connect GSC</button>
    </div>

  {:else if status.connected && !status.property_url}
    <div class="gsc-empty">
      <h3 class="gsc-connect-title">Select a Property</h3>
      <p class="text-muted text-sm mb-md">
        Choose the Search Console property to associate with this project.
      </p>
      {#if status.properties?.length > 0}
        <div class="flex-center-gap gsc-property-wrap">
          <select class="pr-select gsc-property-select" bind:value={selectedProperty}>
            <option value="">-- Select property --</option>
            {#each status.properties as p}
              <option value={p.site_url}>{p.site_url} ({p.permission_level})</option>
            {/each}
          </select>
          <button class="btn btn-primary" onclick={selectPropertyAndFetch} disabled={!selectedProperty || fetchingData}>
            {fetchingData ? 'Fetching...' : 'Select & Fetch Data'}
          </button>
        </div>
      {:else}
        <p class="text-muted">No properties found. Make sure your Google account has access to Search Console properties.</p>
      {/if}
      <button class="btn btn-sm gsc-disconnect-btn" onclick={doDisconnect}>Disconnect</button>
    </div>

  {:else}
    <!-- Connected with property selected -->
    <div class="gsc-toolbar">
      <span class="text-sm text-secondary">
        Property: <strong>{status.property_url}</strong>
      </span>
      <div class="flex-center-gap">
        {#if fetchingData}
          <span class="fetch-indicator">
            <span class="fetch-spinner"></span>
            Fetching{fetchStatus?.rows_so_far ? ` — ${fmtN(fetchStatus.rows_so_far)} rows` : '...'}
          </span>
          <button class="btn btn-sm text-danger" onclick={doStop}>Stop</button>
        {:else}
          <button class="btn btn-sm" onclick={() => doFetch()}>Refresh Data</button>
        {/if}
        <button class="btn btn-sm text-muted" onclick={doDisconnect}>Disconnect</button>
      </div>
    </div>

    <div class="pr-subview-bar">
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'overview'} onclick={() => switchSubView('overview')}>Overview</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'queries'} onclick={() => switchSubView('queries')}>Queries</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'pages'} onclick={() => switchSubView('pages')}>Pages</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'countries'} onclick={() => switchSubView('countries')}>Countries</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'inspection'} onclick={() => switchSubView('inspection')}>Inspection</button>
    </div>

    {#if loading}
      <p class="loading-msg">Loading...</p>

    {:else if subView === 'overview'}
      {#if overview && overview.total_clicks > 0}
        <div class="stats-grid gsc-stats">
          <div class="stat-card"><div class="stat-value">{fmtN(overview.total_clicks)}</div><div class="stat-label">Total Clicks</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(overview.total_impressions)}</div><div class="stat-label">Total Impressions</div></div>
          <div class="stat-card"><div class="stat-value">{(overview.avg_ctr * 100).toFixed(1)}%</div><div class="stat-label">Avg CTR</div></div>
          <div class="stat-card"><div class="stat-value">{overview.avg_position.toFixed(1)}</div><div class="stat-label">Avg Position</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(overview.total_queries)}</div><div class="stat-label">Unique Queries</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(overview.total_pages)}</div><div class="stat-label">Unique Pages</div></div>
        </div>

        <!-- Timeline Chart -->
        {#if timeline?.length > 1}
          {@const maxClicks = Math.max(...timeline.map(t => t.clicks), 1)}
          {@const maxImpr = Math.max(...timeline.map(t => t.impressions), 1)}
          {@const chartW = 700}
          {@const chartH = 200}
          {@const margin = { left: 50, right: 20, top: 10, bottom: 30 }}
          {@const plotW = chartW - margin.left - margin.right}
          {@const plotH = chartH - margin.top - margin.bottom}
          <h4 class="sub-heading">Clicks & Impressions over Time</h4>
          <svg viewBox="0 0 {chartW} {chartH}" class="gsc-chart-svg">
            <!-- Impressions area -->
            <path d="M {margin.left},{margin.top + plotH}
              {timeline.map((t, i) => `L ${margin.left + (i / (timeline.length - 1)) * plotW},${margin.top + plotH - (t.impressions / maxImpr) * plotH}`).join(' ')}
              L {margin.left + plotW},{margin.top + plotH} Z"
              fill="var(--accent)" opacity="0.1" />
            <!-- Impressions line -->
            <polyline
              points={timeline.map((t, i) => `${margin.left + (i / (timeline.length - 1)) * plotW},${margin.top + plotH - (t.impressions / maxImpr) * plotH}`).join(' ')}
              fill="none" stroke="var(--accent)" stroke-width="1" opacity="0.4" />
            <!-- Clicks line -->
            <polyline
              points={timeline.map((t, i) => `${margin.left + (i / (timeline.length - 1)) * plotW},${margin.top + plotH - (t.clicks / maxClicks) * plotH}`).join(' ')}
              fill="none" stroke="var(--accent)" stroke-width="2" />
            <!-- Axis labels -->
            {#each [0, Math.floor(timeline.length / 2), timeline.length - 1] as idx}
              <text x={margin.left + (idx / (timeline.length - 1)) * plotW} y={chartH - 4} text-anchor="middle" class="gsc-axis-label">
                {timeline[idx].date.slice(5)}
              </text>
            {/each}
            <text x={12} y={margin.top + 10} class="gsc-chart-legend">clicks</text>
          </svg>
        {/if}

        <!-- Quick top queries preview -->
        <div class="gsc-grid-2col">
          <div>
            <h4 class="sub-heading">Top Queries (by clicks)</h4>
            {#await getGSCQueries(projectId, 10, 0) then data}
              {#if data.rows?.length > 0}
                <table>
                  <thead><tr><th>Query</th><th>Clicks</th><th>Impressions</th><th>CTR</th><th>Pos</th></tr></thead>
                  <tbody>
                    {#each data.rows as r}
                      <tr>
                        <td class="cell-url gsc-cell-query">{r.query}</td>
                        <td>{fmtN(r.clicks)}</td>
                        <td>{fmtN(r.impressions)}</td>
                        <td>{(r.ctr * 100).toFixed(1)}%</td>
                        <td>{r.position.toFixed(1)}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              {/if}
            {/await}
          </div>
          <div>
            <h4 class="sub-heading">Top Pages (by clicks)</h4>
            {#await getGSCPages(projectId, 10, 0) then data}
              {#if data.rows?.length > 0}
                <table>
                  <thead><tr><th>Page</th><th>Clicks</th><th>Impressions</th><th>CTR</th><th>Pos</th></tr></thead>
                  <tbody>
                    {#each data.rows as r}
                      <tr>
                        <td class="cell-url gsc-cell-page">{r.page.replace(/^https?:\/\/[^/]+/, '') || '/'}</td>
                        <td>{fmtN(r.clicks)}</td>
                        <td>{fmtN(r.impressions)}</td>
                        <td>{(r.ctr * 100).toFixed(1)}%</td>
                        <td>{r.position.toFixed(1)}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              {/if}
            {/await}
          </div>
        </div>
      {:else}
        <p class="chart-empty">No Search Console data yet. Click "Refresh Data" to fetch.</p>
      {/if}

    {:else if subView === 'queries'}
      {#if queries?.rows?.length > 0}
        <div class="table-meta">{fmtN(queries.total)} queries</div>
        <table>
          <thead><tr><th>#</th><th>Query</th><th>Clicks</th><th>Impressions</th><th>CTR</th><th>Position</th></tr></thead>
          <tbody>
            {#each queries.rows as r, i}
              <tr>
                <td class="row-num">{queriesOffset + i + 1}</td>
                <td class="cell-url">{r.query}</td>
                <td>{fmtN(r.clicks)}</td>
                <td>{fmtN(r.impressions)}</td>
                <td>{(r.ctr * 100).toFixed(1)}%</td>
                <td>{r.position.toFixed(1)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if queries.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={queriesOffset === 0} onclick={() => { queriesOffset = Math.max(0, queriesOffset - PAGE_LIMIT); loadSubView('queries'); }}>Previous</button>
            <span class="pagination-info">{queriesOffset + 1} - {Math.min(queriesOffset + PAGE_LIMIT, queries.total)} of {fmtN(queries.total)}</span>
            <button class="btn btn-sm" disabled={queriesOffset + PAGE_LIMIT >= queries.total} onclick={() => { queriesOffset += PAGE_LIMIT; loadSubView('queries'); }}>Next</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">No query data available.</p>
      {/if}

    {:else if subView === 'pages'}
      {#if pages?.rows?.length > 0}
        <div class="table-meta">{fmtN(pages.total)} pages</div>
        <table>
          <thead><tr><th>#</th><th>Page</th><th>Clicks</th><th>Impressions</th><th>CTR</th><th>Position</th></tr></thead>
          <tbody>
            {#each pages.rows as r, i}
              <tr>
                <td class="row-num">{pagesOffset + i + 1}</td>
                <td class="cell-url">{r.page}</td>
                <td>{fmtN(r.clicks)}</td>
                <td>{fmtN(r.impressions)}</td>
                <td>{(r.ctr * 100).toFixed(1)}%</td>
                <td>{r.position.toFixed(1)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if pages.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={pagesOffset === 0} onclick={() => { pagesOffset = Math.max(0, pagesOffset - PAGE_LIMIT); loadSubView('pages'); }}>Previous</button>
            <span class="pagination-info">{pagesOffset + 1} - {Math.min(pagesOffset + PAGE_LIMIT, pages.total)} of {fmtN(pages.total)}</span>
            <button class="btn btn-sm" disabled={pagesOffset + PAGE_LIMIT >= pages.total} onclick={() => { pagesOffset += PAGE_LIMIT; loadSubView('pages'); }}>Next</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">No page data available.</p>
      {/if}

    {:else if subView === 'countries'}
      <div class="gsc-grid-2col">
        <div>
          <h4 class="sub-heading">By Country</h4>
          {#if countries?.length > 0}
            {@const totalCountryClicks = countries.reduce((s, c) => s + c.clicks, 0) || 1}
            {@const maxCountryClicks = Math.max(...countries.map(c => c.clicks), 1)}
            {#each countries as c}
              <div class="gsc-bar-row">
                <span class="gsc-country-code">{c.country}</span>
                <div class="gsc-bar-track">
                  <div class="gsc-bar-fill" style="width: {(c.clicks / maxCountryClicks) * 100}%;"></div>
                </div>
                <span class="gsc-bar-value">{fmtN(c.clicks)}</span>
                <span class="gsc-bar-pct">{(c.clicks / totalCountryClicks * 100).toFixed(1)}%</span>
              </div>
            {/each}
          {:else}
            <p class="chart-empty">No country data.</p>
          {/if}
        </div>
        <div>
          <h4 class="sub-heading">By Device</h4>
          {#if devices?.length > 0}
            {@const totalDeviceClicks = devices.reduce((s, d) => s + d.clicks, 0) || 1}
            {@const maxDeviceClicks = Math.max(...devices.map(d => d.clicks), 1)}
            {#each devices as d}
              <div class="gsc-bar-row">
                <span class="gsc-device-name">{d.device}</span>
                <div class="gsc-bar-track">
                  <div class="gsc-bar-fill" style="width: {(d.clicks / maxDeviceClicks) * 100}%;"></div>
                </div>
                <span class="gsc-bar-value">{fmtN(d.clicks)}</span>
                <span class="gsc-bar-pct">{(d.clicks / totalDeviceClicks * 100).toFixed(1)}%</span>
              </div>
            {/each}
          {:else}
            <p class="chart-empty">No device data.</p>
          {/if}
        </div>
      </div>

    {:else if subView === 'inspection'}
      {#if inspection?.rows?.length > 0}
        <div class="table-meta">{fmtN(inspection.total)} URLs inspected</div>
        <table>
          <thead>
            <tr>
              <th>#</th><th>URL</th><th>Verdict</th><th>Coverage</th><th>Indexing</th>
              <th>Robots</th><th>Last Crawl</th><th>Canonical</th><th>Mobile</th><th>Rich</th>
            </tr>
          </thead>
          <tbody>
            {#each inspection.rows as r, i}
              <tr>
                <td class="row-num">{inspectionOffset + i + 1}</td>
                <td class="cell-url gsc-cell-insp-url">{r.url}</td>
                <td><span class="badge" class:badge-success={r.verdict === 'PASS'} class:badge-danger={r.verdict !== 'PASS' && r.verdict !== ''}>{r.verdict || '-'}</span></td>
                <td class="text-xs">{r.coverage_state || '-'}</td>
                <td class="text-xs">{r.indexing_state || '-'}</td>
                <td class="text-xs">{r.robots_txt_state || '-'}</td>
                <td class="text-xs nowrap">{r.last_crawl_time ? r.last_crawl_time.slice(0, 10) : '-'}</td>
                <td class="cell-url gsc-cell-canonical">{r.canonical_url || '-'}</td>
                <td class="text-xs">{r.mobile_usability || '-'}</td>
                <td>{r.rich_results_items || 0}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if inspection.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={inspectionOffset === 0} onclick={() => { inspectionOffset = Math.max(0, inspectionOffset - PAGE_LIMIT); loadSubView('inspection'); }}>Previous</button>
            <span class="pagination-info">{inspectionOffset + 1} - {Math.min(inspectionOffset + PAGE_LIMIT, inspection.total)} of {fmtN(inspection.total)}</span>
            <button class="btn btn-sm" disabled={inspectionOffset + PAGE_LIMIT >= inspection.total} onclick={() => { inspectionOffset += PAGE_LIMIT; loadSubView('inspection'); }}>Next</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">No inspection data available.</p>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .gsc-empty {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-primary);
  }
  .btn-primary {
    background: var(--accent);
    color: white;
    border: none;
    padding: 8px 20px;
    border-radius: 6px;
    cursor: pointer;
    font-weight: 600;
  }
  .btn-primary:hover { opacity: 0.9; }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .badge-success { background: #22c55e22; color: #16a34a; }
  .badge-danger { background: #ef444422; color: #dc2626; }
  .fetch-indicator {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--accent);
    font-weight: 500;
  }
  .fetch-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid var(--accent);
    border-top-color: transparent;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  .gsc-connect-title { margin-bottom: 12px; font-size: 16px; }
  .gsc-property-select { min-width: 300px; }
  .gsc-property-wrap { flex-wrap: wrap; }
  .gsc-disconnect-btn { margin-top: 12px; }
  .gsc-toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
  .gsc-stats { margin-bottom: 20px; }
  .gsc-chart-svg { width: 100%; max-width: 800px; height: auto; margin-bottom: 24px; }
  .gsc-axis-label { font-size: 10px; fill: var(--text-muted); }
  .gsc-chart-legend { font-size: 9px; fill: var(--accent); }
  .gsc-grid-2col { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
  .gsc-cell-query { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .gsc-cell-page { max-width: 250px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .gsc-bar-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; font-size: 13px; }
  .gsc-country-code { width: 36px; font-weight: 600; text-transform: uppercase; }
  .gsc-device-name { width: 70px; font-weight: 600; text-transform: capitalize; }
  .gsc-bar-track { flex: 1; height: 18px; background: var(--bg-alt); border-radius: 4px; overflow: hidden; }
  .gsc-bar-fill { height: 100%; background: var(--accent); opacity: 0.7; border-radius: 4px; }
  .gsc-bar-value { width: 60px; text-align: right; color: var(--text-muted); }
  .gsc-bar-pct { width: 50px; text-align: right; color: var(--text-muted); font-size: 11px; }
  .gsc-cell-insp-url { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .gsc-cell-canonical { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
  .pr-select { padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 13px; }
</style>
