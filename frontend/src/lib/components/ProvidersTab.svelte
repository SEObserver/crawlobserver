<script>
  import { onDestroy } from 'svelte';
  import { getProviderStatus, connectProvider, disconnectProvider, fetchProviderData, stopProviderFetch,
    getProviderMetrics, getProviderBacklinks, getProviderRefDomains, getProviderRankings, getProviderVisibility } from '../api.js';
  import { fmtN } from '../utils.js';

  let { projectId, provider = 'seobserver', initialSubView = 'overview', onerror, onpushurl } = $props();

  let subView = $state(initialSubView);
  let loading = $state(false);
  let status = $state(null);
  let apiKeyInput = $state('');
  let domainInput = $state('');
  let connecting = $state(false);
  let settingsDomain = $state('');
  let settingsApiKey = $state('');
  let updating = $state(false);

  // Data
  let metrics = $state(null);
  let backlinks = $state(null);
  let backlinksOffset = $state(0);
  let refdomains = $state(null);
  let refdomainsOffset = $state(0);
  let rankings = $state(null);
  let rankingsOffset = $state(0);
  let visibility = $state(null);

  let fetchingData = $state(false);
  let fetchStatus = $state(null);
  let pollTimer = null;
  const PAGE_LIMIT = 100;

  async function loadStatus() {
    if (!projectId) return;
    try {
      status = await getProviderStatus(projectId, provider);
      if (status.fetch_status?.fetching) {
        fetchingData = true;
        fetchStatus = status.fetch_status;
        startPolling();
      } else if (fetchingData && !status.fetch_status?.fetching) {
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
      if (fetchingData) await loadSubView(subView);
    }, 5000);
  }

  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }

  onDestroy(() => stopPolling());

  async function doConnect() {
    if (!apiKeyInput || !domainInput) return;
    connecting = true;
    try {
      await connectProvider(projectId, provider, apiKeyInput, domainInput);
      apiKeyInput = '';
      await loadStatus();
      loadSubView(subView);
    } catch (e) {
      onerror?.(e.message);
    } finally {
      connecting = false;
    }
  }

  async function doFetch() {
    fetchingData = true;
    fetchStatus = { fetching: true, phase: 'starting', rows_so_far: 0 };
    try {
      await fetchProviderData(projectId, provider);
      startPolling();
    } catch (e) {
      onerror?.(e.message);
      fetchingData = false;
      fetchStatus = null;
    }
  }

  async function doStop() {
    try {
      await stopProviderFetch(projectId, provider);
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
      await disconnectProvider(projectId, provider);
      stopPolling();
      status = { connected: false };
      fetchingData = false;
      fetchStatus = null;
      metrics = null;
      backlinks = null;
      refdomains = null;
      rankings = null;
      visibility = null;
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function doUpdate() {
    if (!settingsDomain) return;
    updating = true;
    try {
      await connectProvider(projectId, provider, settingsApiKey || undefined, settingsDomain);
      settingsApiKey = '';
      await loadStatus();
      settingsDomain = status.domain || '';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      updating = false;
    }
  }

  async function loadSubView(view) {
    if (!status?.connected) return;
    if (!fetchingData) loading = true;
    try {
      if (view === 'overview') {
        const [m, v] = await Promise.all([
          getProviderMetrics(projectId, provider).catch(() => null),
          getProviderVisibility(projectId, provider).catch(() => []),
        ]);
        metrics = m;
        visibility = v;
      } else if (view === 'backlinks') {
        backlinks = await getProviderBacklinks(projectId, provider, PAGE_LIMIT, backlinksOffset);
      } else if (view === 'refdomains') {
        refdomains = await getProviderRefDomains(projectId, provider, PAGE_LIMIT, refdomainsOffset);
      } else if (view === 'rankings') {
        rankings = await getProviderRankings(projectId, provider, PAGE_LIMIT, rankingsOffset);
      }
    } catch (e) {
      // No data yet is OK
    } finally {
      loading = false;
    }
  }

  function switchSubView(view) {
    subView = view;
    if (view === 'backlinks') backlinksOffset = 0;
    if (view === 'refdomains') refdomainsOffset = 0;
    if (view === 'rankings') rankingsOffset = 0;
    if (view === 'settings') {
      settingsDomain = status?.domain || '';
      settingsApiKey = '';
    }
    onpushurl?.(`/projects/${projectId}/providers/${view}`);
    loadSubView(view);
  }

  loadStatus();
  if (projectId) loadSubView(subView);
</script>

<div class="pr-container">
  {#if !projectId}
    <div class="prov-empty">
      <p>This session is not associated with a project.</p>
      <p class="text-muted text-sm">Associate it with a project first, then connect an SEO data provider.</p>
    </div>

  {:else if !status}
    <p class="loading-msg">Loading...</p>

  {:else if !status.connected}
    <div class="prov-empty">
      <h3 class="prov-connect-title">Connect SEObserver</h3>
      <p class="text-muted text-sm mb-md">
        Link this project to SEObserver to get backlinks, rankings, referring domains, and visibility data.
      </p>
      <div class="prov-connect-form">
        <input type="text" class="pr-input" placeholder="Domain (e.g. example.com)" bind:value={domainInput} />
        <input type="password" class="pr-input" placeholder="SEObserver API Key" bind:value={apiKeyInput} />
        <button class="btn btn-primary" onclick={doConnect} disabled={connecting || !apiKeyInput || !domainInput}>
          {connecting ? 'Connecting...' : 'Connect'}
        </button>
      </div>
    </div>

  {:else}
    <!-- Connected -->
    <div class="prov-toolbar">
      <span class="text-sm text-secondary">
        Domain: <strong>{status.domain}</strong>
        <span class="prov-provider-tag">({provider})</span>
      </span>
      <div class="flex-center-gap">
        {#if fetchingData}
          <span class="fetch-indicator">
            <span class="fetch-spinner"></span>
            {fetchStatus?.phase || 'Fetching'}{fetchStatus?.rows_so_far ? ` — ${fmtN(fetchStatus.rows_so_far)} rows` : '...'}
          </span>
          <button class="btn btn-sm text-danger" onclick={doStop}>Stop</button>
        {:else}
          <button class="btn btn-sm" onclick={doFetch}>Fetch Data</button>
        {/if}
      </div>
    </div>

    <div class="pr-subview-bar">
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'overview'} onclick={() => switchSubView('overview')}>Overview</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'backlinks'} onclick={() => switchSubView('backlinks')}>Backlinks</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'refdomains'} onclick={() => switchSubView('refdomains')}>Ref Domains</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'rankings'} onclick={() => switchSubView('rankings')}>Rankings</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'settings'} onclick={() => switchSubView('settings')}>Settings</button>
    </div>

    {#if loading}
      <p class="loading-msg">Loading...</p>

    {:else if subView === 'overview'}
      {#if metrics && (metrics.backlinks_total > 0 || metrics.domain_rank > 0)}
        <div class="stats-grid prov-stats">
          <div class="stat-card"><div class="stat-value">{metrics.domain_rank?.toFixed(1) || '0'}</div><div class="stat-label">Domain Rank</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.backlinks_total)}</div><div class="stat-label">Total Backlinks</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.refdomains_total)}</div><div class="stat-label">Referring Domains</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.organic_keywords)}</div><div class="stat-label">Organic Keywords</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.organic_traffic)}</div><div class="stat-label">Organic Traffic</div></div>
          <div class="stat-card"><div class="stat-value">${metrics.organic_cost?.toFixed(0) || '0'}</div><div class="stat-label">Traffic Value</div></div>
        </div>

        <!-- Visibility Chart -->
        {#if visibility?.length > 1}
          {@const maxVis = Math.max(...visibility.map(v => v.visibility), 1)}
          {@const chartW = 700}
          {@const chartH = 200}
          {@const margin = { left: 50, right: 20, top: 10, bottom: 30 }}
          {@const plotW = chartW - margin.left - margin.right}
          {@const plotH = chartH - margin.top - margin.bottom}
          <h4 class="sub-heading">Visibility over Time</h4>
          <svg viewBox="0 0 {chartW} {chartH}" class="prov-chart-svg">
            <path d="M {margin.left},{margin.top + plotH}
              {visibility.map((v, i) => `L ${margin.left + (i / (visibility.length - 1)) * plotW},${margin.top + plotH - (v.visibility / maxVis) * plotH}`).join(' ')}
              L {margin.left + plotW},{margin.top + plotH} Z"
              fill="var(--accent)" opacity="0.1" />
            <polyline
              points={visibility.map((v, i) => `${margin.left + (i / (visibility.length - 1)) * plotW},${margin.top + plotH - (v.visibility / maxVis) * plotH}`).join(' ')}
              fill="none" stroke="var(--accent)" stroke-width="2" />
            {#each [0, Math.floor(visibility.length / 2), visibility.length - 1] as idx}
              <text x={margin.left + (idx / (visibility.length - 1)) * plotW} y={chartH - 4} text-anchor="middle" class="prov-axis-label">
                {visibility[idx].date?.slice?.(0, 10) || ''}
              </text>
            {/each}
            <text x={12} y={margin.top + 10} class="prov-chart-legend">visibility</text>
          </svg>
        {/if}
      {:else}
        <p class="chart-empty">No SEO data yet. Click "Fetch Data" to retrieve data from SEObserver.</p>
      {/if}

    {:else if subView === 'backlinks'}
      {#if backlinks?.rows?.length > 0}
        <div class="table-meta">{fmtN(backlinks.total)} backlinks</div>
        <table>
          <thead><tr><th>#</th><th>Source URL</th><th>Target URL</th><th>Anchor</th><th>DR</th><th>NF</th><th>First Seen</th></tr></thead>
          <tbody>
            {#each backlinks.rows as r, i}
              <tr>
                <td class="row-num">{backlinksOffset + i + 1}</td>
                <td class="cell-url prov-cell-url">
                  <a href={r.source_url} target="_blank" rel="noopener">{r.source_url}</a>
                </td>
                <td class="cell-url prov-cell-target">{r.target_url}</td>
                <td class="prov-cell-anchor">{r.anchor_text || '-'}</td>
                <td>{r.domain_rank?.toFixed(1) || '-'}</td>
                <td>{r.nofollow ? 'NF' : 'DF'}</td>
                <td class="text-xs nowrap">{r.first_seen?.slice?.(0, 10) || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if backlinks.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={backlinksOffset === 0} onclick={() => { backlinksOffset = Math.max(0, backlinksOffset - PAGE_LIMIT); loadSubView('backlinks'); }}>Previous</button>
            <span class="pagination-info">{backlinksOffset + 1} - {Math.min(backlinksOffset + PAGE_LIMIT, backlinks.total)} of {fmtN(backlinks.total)}</span>
            <button class="btn btn-sm" disabled={backlinksOffset + PAGE_LIMIT >= backlinks.total} onclick={() => { backlinksOffset += PAGE_LIMIT; loadSubView('backlinks'); }}>Next</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">No backlink data available.</p>
      {/if}

    {:else if subView === 'refdomains'}
      {#if refdomains?.rows?.length > 0}
        <div class="table-meta">{fmtN(refdomains.total)} referring domains</div>
        <table>
          <thead><tr><th>#</th><th>Domain</th><th>Backlinks</th><th>DR</th><th>First Seen</th><th>Last Seen</th></tr></thead>
          <tbody>
            {#each refdomains.rows as r, i}
              <tr>
                <td class="row-num">{refdomainsOffset + i + 1}</td>
                <td><strong>{r.ref_domain}</strong></td>
                <td>{fmtN(r.backlink_count)}</td>
                <td>{r.domain_rank?.toFixed(1) || '-'}</td>
                <td class="text-xs nowrap">{r.first_seen?.slice?.(0, 10) || '-'}</td>
                <td class="text-xs nowrap">{r.last_seen?.slice?.(0, 10) || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if refdomains.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={refdomainsOffset === 0} onclick={() => { refdomainsOffset = Math.max(0, refdomainsOffset - PAGE_LIMIT); loadSubView('refdomains'); }}>Previous</button>
            <span class="pagination-info">{refdomainsOffset + 1} - {Math.min(refdomainsOffset + PAGE_LIMIT, refdomains.total)} of {fmtN(refdomains.total)}</span>
            <button class="btn btn-sm" disabled={refdomainsOffset + PAGE_LIMIT >= refdomains.total} onclick={() => { refdomainsOffset += PAGE_LIMIT; loadSubView('refdomains'); }}>Next</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">No referring domain data available.</p>
      {/if}

    {:else if subView === 'rankings'}
      {#if rankings?.rows?.length > 0}
        <div class="table-meta">{fmtN(rankings.total)} keywords</div>
        <table>
          <thead><tr><th>#</th><th>Keyword</th><th>Pos</th><th>URL</th><th>Volume</th><th>CPC</th><th>Traffic</th></tr></thead>
          <tbody>
            {#each rankings.rows as r, i}
              <tr>
                <td class="row-num">{rankingsOffset + i + 1}</td>
                <td><strong>{r.keyword}</strong></td>
                <td><span class="badge" class:badge-success={r.position <= 3} class:badge-warn={r.position > 3 && r.position <= 10}>{r.position}</span></td>
                <td class="cell-url prov-cell-url">{r.url}</td>
                <td>{fmtN(r.search_volume)}</td>
                <td>{r.cpc?.toFixed(2) || '-'}</td>
                <td>{r.traffic?.toFixed(0) || '0'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if rankings.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={rankingsOffset === 0} onclick={() => { rankingsOffset = Math.max(0, rankingsOffset - PAGE_LIMIT); loadSubView('rankings'); }}>Previous</button>
            <span class="pagination-info">{rankingsOffset + 1} - {Math.min(rankingsOffset + PAGE_LIMIT, rankings.total)} of {fmtN(rankings.total)}</span>
            <button class="btn btn-sm" disabled={rankingsOffset + PAGE_LIMIT >= rankings.total} onclick={() => { rankingsOffset += PAGE_LIMIT; loadSubView('rankings'); }}>Next</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">No ranking data available.</p>
      {/if}

    {:else if subView === 'settings'}
      <div class="settings-section">
        <h4 class="prov-settings-title">Connection</h4>
        <div class="settings-info">
          <div class="settings-row">
            <span class="settings-label">Provider</span>
            <span class="settings-value">{status.provider}</span>
          </div>
          <div class="settings-row">
            <span class="settings-label">Domain</span>
            <span class="settings-value">{status.domain}</span>
          </div>
          <div class="settings-row">
            <span class="settings-label">Connected</span>
            <span class="settings-value">{status.created_at ? new Date(status.created_at).toLocaleDateString() : '-'}</span>
          </div>
        </div>

        <h4 class="prov-settings-subtitle">Update Connection</h4>
        <div class="prov-settings-form">
          <label class="settings-field-label">
            Domain
            <input type="text" class="pr-input" bind:value={settingsDomain} placeholder="example.com" />
          </label>
          <label class="settings-field-label">
            API Key
            <input type="password" class="pr-input" bind:value={settingsApiKey} placeholder="Leave empty to keep current key" />
          </label>
          <button class="btn btn-primary prov-settings-submit" onclick={doUpdate} disabled={updating || !settingsDomain}>
            {updating ? 'Updating...' : 'Update'}
          </button>
        </div>

        <div class="prov-danger-zone">
          <h4 class="prov-danger-title">Danger Zone</h4>
          <p class="prov-danger-text">Disconnecting will remove the API key and all fetched data for this provider.</p>
          <button class="btn btn-sm prov-disconnect-btn" onclick={doDisconnect}>Disconnect</button>
        </div>
      </div>
    {/if}
  {/if}
</div>

<style>
  .prov-empty {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-primary);
  }
  .pr-input {
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text-primary);
    font-size: 13px;
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
  .badge-warn { background: #f59e0b22; color: #d97706; }
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
  .settings-section { max-width: 520px; }
  .settings-info {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 16px;
  }
  .settings-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 6px 0;
  }
  .settings-row + .settings-row { border-top: 1px solid var(--border); }
  .settings-label { font-size: 13px; color: var(--text-muted); }
  .settings-value { font-size: 13px; font-weight: 500; color: var(--text-primary); }
  .settings-field-label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: var(--text-secondary);
    font-weight: 500;
  }
  .prov-connect-title { margin-bottom: 12px; font-size: 16px; }
  .prov-connect-form { display: flex; flex-direction: column; gap: 10px; max-width: 400px; margin: 0 auto; }
  .prov-toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
  .prov-provider-tag { color: var(--text-muted); margin-left: 8px; }
  .prov-stats { margin-bottom: 20px; }
  .prov-chart-svg { width: 100%; max-width: 800px; height: auto; margin-bottom: 24px; }
  .prov-axis-label { font-size: 10px; fill: var(--text-muted); }
  .prov-chart-legend { font-size: 9px; fill: var(--accent); }
  .prov-cell-url { max-width: 250px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .prov-cell-target { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .prov-cell-anchor { max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .prov-settings-title { font-size: 14px; font-weight: 600; margin-bottom: 16px; }
  .prov-settings-subtitle { font-size: 14px; font-weight: 600; margin: 24px 0 12px; }
  .prov-settings-form { display: flex; flex-direction: column; gap: 10px; max-width: 400px; }
  .prov-settings-submit { align-self: flex-start; }
  .prov-danger-zone { margin-top: 32px; padding-top: 20px; border-top: 1px solid var(--border); }
  .prov-danger-title { font-size: 14px; font-weight: 600; margin-bottom: 8px; color: var(--danger, #e53e3e); }
  .prov-danger-text { font-size: 13px; color: var(--text-muted); margin-bottom: 12px; }
  .prov-disconnect-btn { color: var(--danger, #e53e3e); border-color: var(--danger, #e53e3e); }
</style>
