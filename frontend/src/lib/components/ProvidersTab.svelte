<script>
  import { onDestroy } from 'svelte';
  import { getProviderStatus, connectProvider, disconnectProvider, fetchProviderData, stopProviderFetch,
    getProviderMetrics, getProviderBacklinks, getProviderRefDomains, getProviderRankings, getProviderVisibility,
    getProviderTopPages, getProviderAPICalls } from '../api.js';
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

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
  let topPages = $state(null);
  let topPagesOffset = $state(0);
  let apiCalls = $state(null);
  let apiCallsOffset = $state(0);
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
      topPages = null;
      apiCalls = null;
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
      } else if (view === 'top_pages') {
        topPages = await getProviderTopPages(projectId, provider, PAGE_LIMIT, topPagesOffset);
      } else if (view === 'api_calls') {
        apiCalls = await getProviderAPICalls(projectId, provider, 50, apiCallsOffset);
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
    if (view === 'top_pages') topPagesOffset = 0;
    if (view === 'api_calls') apiCallsOffset = 0;
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
      <p>{t('providers.notAssociated')}</p>
      <p class="text-muted text-sm">{t('providers.associateFirst')}</p>
    </div>

  {:else if !status}
    <p class="loading-msg">{t('common.loading')}</p>

  {:else if !status.connected}
    <div class="prov-empty">
      <h3 class="prov-connect-title">{t('providers.connectTitle')}</h3>
      <p class="text-muted text-sm mb-md">
        {t('providers.connectDesc')}
      </p>
      <div class="prov-connect-form">
        <input type="text" class="pr-input" placeholder={t('providers.domainPlaceholder')} bind:value={domainInput} />
        <input type="password" class="pr-input" placeholder={t('providers.apiKey')} bind:value={apiKeyInput} />
        <button class="btn btn-primary" onclick={doConnect} disabled={connecting || !apiKeyInput || !domainInput}>
          {connecting ? t('providers.connecting') : t('common.connect')}
        </button>
      </div>
    </div>

  {:else}
    <!-- Connected -->
    <div class="prov-toolbar">
      <span class="text-sm text-secondary">
        {t('providers.domain')} <strong>{status.domain}</strong>
        <span class="prov-provider-tag">({provider})</span>
      </span>
      <div class="flex-center-gap">
        {#if fetchingData}
          <span class="fetch-indicator">
            <span class="fetch-spinner"></span>
            {fetchStatus?.phase || t('providers.fetchingPhase')}{fetchStatus?.rows_so_far ? ` — ${fmtN(fetchStatus.rows_so_far)} rows` : '...'}
          </span>
          <button class="btn btn-sm text-danger" onclick={doStop}>{t('common.stop')}</button>
        {:else}
          <button class="btn btn-sm" onclick={doFetch}>{t('providers.fetchData')}</button>
        {/if}
      </div>
    </div>

    <div class="pr-subview-bar">
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'overview'} onclick={() => switchSubView('overview')}>{t('providers.overview')}</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'backlinks'} onclick={() => switchSubView('backlinks')}>{t('providers.backlinks')}</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'refdomains'} onclick={() => switchSubView('refdomains')}>{t('providers.refDomains')}</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'rankings'} onclick={() => switchSubView('rankings')}>{t('providers.rankings')}</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'top_pages'} onclick={() => switchSubView('top_pages')}>{t('providers.topPagesTab')}</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'api_calls'} onclick={() => switchSubView('api_calls')}>{t('providers.apiCallsTab')}</button>
      <button class="pr-subview-btn" class:pr-subview-active={subView === 'settings'} onclick={() => switchSubView('settings')}>{t('providers.settings')}</button>
    </div>

    {#if loading}
      <p class="loading-msg">{t('common.loading')}</p>

    {:else if subView === 'overview'}
      {#if metrics && (metrics.backlinks_total > 0 || metrics.domain_rank > 0)}
        <div class="stats-grid prov-stats">
          <div class="stat-card"><div class="stat-value">{metrics.domain_rank?.toFixed(1) || '0'}</div><div class="stat-label">{t('providers.domainRank')}</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.backlinks_total)}</div><div class="stat-label">{t('providers.totalBacklinks')}</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.refdomains_total)}</div><div class="stat-label">{t('providers.referringDomains')}</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.organic_keywords)}</div><div class="stat-label">{t('providers.organicKeywords')}</div></div>
          <div class="stat-card"><div class="stat-value">{fmtN(metrics.organic_traffic)}</div><div class="stat-label">{t('providers.organicTraffic')}</div></div>
          <div class="stat-card"><div class="stat-value">${metrics.organic_cost?.toFixed(0) || '0'}</div><div class="stat-label">{t('providers.trafficValue')}</div></div>
        </div>

        <!-- Visibility Chart -->
        {#if visibility?.length > 1}
          {@const maxVis = Math.max(...visibility.map(v => v.visibility), 1)}
          {@const chartW = 700}
          {@const chartH = 200}
          {@const margin = { left: 50, right: 20, top: 10, bottom: 30 }}
          {@const plotW = chartW - margin.left - margin.right}
          {@const plotH = chartH - margin.top - margin.bottom}
          <h4 class="sub-heading">{t('providers.visibilityOverTime')}</h4>
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
            <text x={12} y={margin.top + 10} class="prov-chart-legend">{t('providers.visibility')}</text>
          </svg>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noData')}</p>
      {/if}

    {:else if subView === 'backlinks'}
      {#if backlinks?.rows?.length > 0}
        <div class="table-meta">{t('providers.backlinksCount', { count: fmtN(backlinks.total) })}</div>
        <table>
          <thead><tr><th>#</th><th>{t('providers.sourceURL')}</th><th>{t('providers.targetURL')}</th><th>{t('providers.anchor')}</th><th>{t('providers.dr')}</th><th>{t('providers.nf')}</th><th>{t('providers.firstSeen')}</th></tr></thead>
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
                <td>{r.nofollow ? t('providers.nf') : t('providers.df')}</td>
                <td class="text-xs nowrap">{r.first_seen?.slice?.(0, 10) || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if backlinks.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={backlinksOffset === 0} onclick={() => { backlinksOffset = Math.max(0, backlinksOffset - PAGE_LIMIT); loadSubView('backlinks'); }}>{t('common.previous')}</button>
            <span class="pagination-info">{backlinksOffset + 1} - {Math.min(backlinksOffset + PAGE_LIMIT, backlinks.total)} of {fmtN(backlinks.total)}</span>
            <button class="btn btn-sm" disabled={backlinksOffset + PAGE_LIMIT >= backlinks.total} onclick={() => { backlinksOffset += PAGE_LIMIT; loadSubView('backlinks'); }}>{t('common.next')}</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noBacklinks')}</p>
      {/if}

    {:else if subView === 'refdomains'}
      {#if refdomains?.rows?.length > 0}
        <div class="table-meta">{t('providers.refDomainsCount', { count: fmtN(refdomains.total) })}</div>
        <table>
          <thead><tr><th>#</th><th>{t('providers.domainCol')}</th><th>{t('providers.backlinks')}</th><th>{t('providers.dr')}</th><th>{t('providers.firstSeen')}</th><th>{t('providers.lastSeen')}</th></tr></thead>
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
            <button class="btn btn-sm" disabled={refdomainsOffset === 0} onclick={() => { refdomainsOffset = Math.max(0, refdomainsOffset - PAGE_LIMIT); loadSubView('refdomains'); }}>{t('common.previous')}</button>
            <span class="pagination-info">{refdomainsOffset + 1} - {Math.min(refdomainsOffset + PAGE_LIMIT, refdomains.total)} of {fmtN(refdomains.total)}</span>
            <button class="btn btn-sm" disabled={refdomainsOffset + PAGE_LIMIT >= refdomains.total} onclick={() => { refdomainsOffset += PAGE_LIMIT; loadSubView('refdomains'); }}>{t('common.next')}</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noRefDomains')}</p>
      {/if}

    {:else if subView === 'rankings'}
      {#if rankings?.rows?.length > 0}
        <div class="table-meta">{t('providers.keywordsCount', { count: fmtN(rankings.total) })}</div>
        <table>
          <thead><tr><th>#</th><th>{t('providers.keyword')}</th><th>{t('providers.pos')}</th><th>{t('common.url')}</th><th>{t('providers.volume')}</th><th>{t('providers.cpc')}</th><th>{t('providers.traffic')}</th></tr></thead>
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
            <button class="btn btn-sm" disabled={rankingsOffset === 0} onclick={() => { rankingsOffset = Math.max(0, rankingsOffset - PAGE_LIMIT); loadSubView('rankings'); }}>{t('common.previous')}</button>
            <span class="pagination-info">{rankingsOffset + 1} - {Math.min(rankingsOffset + PAGE_LIMIT, rankings.total)} of {fmtN(rankings.total)}</span>
            <button class="btn btn-sm" disabled={rankingsOffset + PAGE_LIMIT >= rankings.total} onclick={() => { rankingsOffset += PAGE_LIMIT; loadSubView('rankings'); }}>{t('common.next')}</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noRankings')}</p>
      {/if}

    {:else if subView === 'top_pages'}
      {#if topPages?.rows?.length > 0}
        <div class="table-meta">{t('providers.topPagesCount', { count: fmtN(topPages.total) })}</div>
        <table>
          <thead><tr><th>#</th><th>{t('common.url')}</th><th>{t('providers.title')}</th><th>{t('providers.tf')}</th><th>{t('providers.cf')}</th><th>{t('providers.backlinks')}</th><th>{t('providers.refDomains')}</th><th>{t('providers.topic')}</th><th>{t('providers.lang')}</th></tr></thead>
          <tbody>
            {#each topPages.rows as r, i}
              <tr>
                <td class="row-num">{topPagesOffset + i + 1}</td>
                <td class="cell-url prov-cell-url">
                  <a href={r.url} target="_blank" rel="noopener">{r.url}</a>
                </td>
                <td class="prov-cell-anchor">{r.title || '-'}</td>
                <td><span class="badge prov-metric-badge" style="background-color: {r.trust_flow > 40 ? '#22c55e22' : r.trust_flow >= 20 ? '#f59e0b22' : '#ef444422'}; color: {r.trust_flow > 40 ? '#16a34a' : r.trust_flow >= 20 ? '#d97706' : '#dc2626'}">{r.trust_flow ?? '-'}</span></td>
                <td><span class="badge prov-metric-badge" style="background-color: {r.citation_flow > 40 ? '#22c55e22' : r.citation_flow >= 20 ? '#f59e0b22' : '#ef444422'}; color: {r.citation_flow > 40 ? '#16a34a' : r.citation_flow >= 20 ? '#d97706' : '#dc2626'}">{r.citation_flow ?? '-'}</span></td>
                <td>{fmtN(r.backlinks ?? 0)}</td>
                <td>{fmtN(r.ref_domains ?? 0)}</td>
                <td class="text-xs">{r.topical_tf?.[0]?.topic || '-'}</td>
                <td class="text-xs">{r.language || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if topPages.total > PAGE_LIMIT}
          <div class="pagination">
            <button class="btn btn-sm" disabled={topPagesOffset === 0} onclick={() => { topPagesOffset = Math.max(0, topPagesOffset - PAGE_LIMIT); loadSubView('top_pages'); }}>{t('common.previous')}</button>
            <span class="pagination-info">{topPagesOffset + 1} - {Math.min(topPagesOffset + PAGE_LIMIT, topPages.total)} of {fmtN(topPages.total)}</span>
            <button class="btn btn-sm" disabled={topPagesOffset + PAGE_LIMIT >= topPages.total} onclick={() => { topPagesOffset += PAGE_LIMIT; loadSubView('top_pages'); }}>{t('common.next')}</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noTopPages')}</p>
      {/if}

    {:else if subView === 'api_calls'}
      {#if apiCalls?.rows?.length > 0}
        <div class="table-meta">{t('providers.apiCallsCount', { count: fmtN(apiCalls.total) })}</div>
        <table>
          <thead><tr><th>#</th><th>{t('providers.time')}</th><th>{t('providers.endpoint')}</th><th>{t('providers.status')}</th><th>{t('providers.duration')}</th><th>{t('providers.rows')}</th><th>{t('providers.error')}</th></tr></thead>
          <tbody>
            {#each apiCalls.rows as r, i}
              <tr>
                <td class="row-num">{apiCallsOffset + i + 1}</td>
                <td class="text-xs nowrap">{r.timestamp ? new Date(r.timestamp).toLocaleString() : '-'}</td>
                <td class="prov-cell-anchor">{r.endpoint || '-'}</td>
                <td><span class="badge" style="background-color: {r.status_code >= 200 && r.status_code < 300 ? '#22c55e22' : r.status_code >= 400 ? '#ef444422' : '#f59e0b22'}; color: {r.status_code >= 200 && r.status_code < 300 ? '#16a34a' : r.status_code >= 400 ? '#dc2626' : '#d97706'}">{r.status_code ?? '-'}</span></td>
                <td class="text-xs nowrap">{r.duration_ms != null ? `${r.duration_ms}ms` : '-'}</td>
                <td>{r.rows_returned != null ? fmtN(r.rows_returned) : '-'}</td>
                <td class="text-xs prov-cell-anchor">{r.error || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if apiCalls.total > 50}
          <div class="pagination">
            <button class="btn btn-sm" disabled={apiCallsOffset === 0} onclick={() => { apiCallsOffset = Math.max(0, apiCallsOffset - 50); loadSubView('api_calls'); }}>{t('common.previous')}</button>
            <span class="pagination-info">{apiCallsOffset + 1} - {Math.min(apiCallsOffset + 50, apiCalls.total)} of {fmtN(apiCalls.total)}</span>
            <button class="btn btn-sm" disabled={apiCallsOffset + 50 >= apiCalls.total} onclick={() => { apiCallsOffset += 50; loadSubView('api_calls'); }}>{t('common.next')}</button>
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noAPICalls')}</p>
      {/if}

    {:else if subView === 'settings'}
      <div class="settings-section">
        <h4 class="prov-settings-title">{t('providers.connection')}</h4>
        <div class="settings-info">
          <div class="settings-row">
            <span class="settings-label">{t('providers.provider')}</span>
            <span class="settings-value">{status.provider}</span>
          </div>
          <div class="settings-row">
            <span class="settings-label">{t('providers.domainCol')}</span>
            <span class="settings-value">{status.domain}</span>
          </div>
          <div class="settings-row">
            <span class="settings-label">{t('providers.connected')}</span>
            <span class="settings-value">{status.created_at ? new Date(status.created_at).toLocaleDateString() : '-'}</span>
          </div>
        </div>

        <h4 class="prov-settings-subtitle">{t('providers.updateConnection')}</h4>
        <div class="prov-settings-form">
          <label class="settings-field-label">
            {t('providers.domainCol')}
            <input type="text" class="pr-input" bind:value={settingsDomain} placeholder="example.com" />
          </label>
          <label class="settings-field-label">
            {t('providers.apiKeyLabel')}
            <input type="password" class="pr-input" bind:value={settingsApiKey} placeholder={t('providers.keepCurrentKey')} />
          </label>
          <button class="btn btn-primary prov-settings-submit" onclick={doUpdate} disabled={updating || !settingsDomain}>
            {updating ? t('providers.updating') : t('common.update')}
          </button>
        </div>

        <div class="prov-danger-zone">
          <h4 class="prov-danger-title">{t('providers.dangerZone')}</h4>
          <p class="prov-danger-text">{t('providers.disconnectWarning')}</p>
          <button class="btn btn-sm prov-disconnect-btn" onclick={doDisconnect}>{t('common.disconnect')}</button>
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