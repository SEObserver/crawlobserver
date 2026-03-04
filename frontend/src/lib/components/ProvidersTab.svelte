<script>
  import { onDestroy } from 'svelte';
  import {
    getProviderStatus,
    connectProvider,
    disconnectProvider,
    fetchProviderData,
    stopProviderFetch,
    getProviderMetrics,
    getProviderBacklinks,
    getProviderRefDomains,
    getProviderRankings,
    getProviderVisibility,
    getProviderTopPages,
    getProviderAPICalls,
    getProviderData,
  } from '../api.js';
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import UrlActions from './UrlActions.svelte';
  import DataTable from './DataTable.svelte';

  let {
    projectId,
    provider = 'seobserver',
    initialSubView = 'overview',
    onerror,
    onpushurl,
  } = $props();

  let subView = $state(initialSubView);
  let loading = $state(false);
  let status = $state(null);
  let apiKeyInput = $state('');
  let domainInput = $state('');
  let connecting = $state(false);
  let settingsDomain = $state('');
  let settingsApiKey = $state('');
  let settingsLimitBacklinks = $state(1000);
  let settingsLimitRefdomains = $state(1000);
  let settingsLimitRankings = $state(1000);
  let settingsLimitTopPages = $state(1000);
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
  let topPagesFilters = $state({});
  let topPagesSortCol = $state('');
  let topPagesSortOrder = $state('');
  let apiCalls = $state(null);
  let apiCallsOffset = $state(0);
  let visibility = $state(null);

  let fetchingData = $state(false);
  let fetchStatus = $state(null);
  let pollTimer = null;
  const PAGE_LIMIT = 100;

  // Placeholder data for disconnected locked previews
  const PH_METRICS = {
    domain_rank: 42.3,
    backlinks_total: 128450,
    refdomains_total: 3280,
    organic_keywords: 15670,
    organic_traffic: 45200,
    organic_cost: 12800,
  };
  const PH_BACKLINKS = [
    {
      source_url: 'https://blog.example.com/review',
      target_url: '/products/main',
      anchor_text: 'best product',
      domain_rank: 45.2,
      nofollow: false,
      first_seen: '2025-11-15',
    },
    {
      source_url: 'https://news.sample.org/roundup',
      target_url: '/blog/latest',
      anchor_text: 'latest news',
      domain_rank: 38.7,
      nofollow: false,
      first_seen: '2025-10-22',
    },
    {
      source_url: 'https://forum.tech.io/thread',
      target_url: '/',
      anchor_text: 'homepage',
      domain_rank: 29.1,
      nofollow: true,
      first_seen: '2025-09-03',
    },
    {
      source_url: 'https://review.site.com/top-10',
      target_url: '/about',
      anchor_text: 'company info',
      domain_rank: 52.8,
      nofollow: false,
      first_seen: '2025-08-17',
    },
    {
      source_url: 'https://wiki.reference.net/entry',
      target_url: '/resources',
      anchor_text: 'resource page',
      domain_rank: 61.3,
      nofollow: false,
      first_seen: '2025-07-01',
    },
  ];
  const PH_REFDOMAINS = [
    {
      ref_domain: 'blog.example.com',
      backlink_count: 342,
      domain_rank: 45.2,
      first_seen: '2024-03-12',
      last_seen: '2025-11-15',
    },
    {
      ref_domain: 'news.sample.org',
      backlink_count: 128,
      domain_rank: 38.7,
      first_seen: '2024-06-01',
      last_seen: '2025-10-22',
    },
    {
      ref_domain: 'forum.tech.io',
      backlink_count: 89,
      domain_rank: 29.1,
      first_seen: '2024-09-15',
      last_seen: '2025-09-03',
    },
    {
      ref_domain: 'review.site.com',
      backlink_count: 56,
      domain_rank: 52.8,
      first_seen: '2025-01-20',
      last_seen: '2025-08-17',
    },
    {
      ref_domain: 'wiki.reference.net',
      backlink_count: 234,
      domain_rank: 61.3,
      first_seen: '2023-11-05',
      last_seen: '2025-07-01',
    },
  ];
  const PH_RANKINGS = [
    {
      keyword: 'best seo tool 2025',
      position: 3,
      url: '/products/seo-tool',
      search_volume: 12400,
      cpc: 4.5,
      traffic: 3200,
    },
    {
      keyword: 'website audit software',
      position: 7,
      url: '/products/audit',
      search_volume: 8200,
      cpc: 3.2,
      traffic: 890,
    },
    {
      keyword: 'backlink checker free',
      position: 12,
      url: '/tools/backlinks',
      search_volume: 22000,
      cpc: 2.8,
      traffic: 620,
    },
    {
      keyword: 'seo crawler comparison',
      position: 2,
      url: '/blog/comparison',
      search_volume: 4800,
      cpc: 5.1,
      traffic: 2100,
    },
    {
      keyword: 'domain authority tool',
      position: 18,
      url: '/tools/authority',
      search_volume: 9600,
      cpc: 3.7,
      traffic: 280,
    },
  ];
  const PH_TOP_PAGES = [
    {
      url: 'https://example.com/page-one',
      title: 'Main Product Page',
      trust_flow: 42,
      citation_flow: 38,
      backlinks: 1240,
      ref_domains: 320,
      topical_tf: [{ topic: 'Business' }],
      language: 'en',
    },
    {
      url: 'https://example.com/blog/article',
      title: 'Top Blog Article',
      trust_flow: 35,
      citation_flow: 29,
      backlinks: 870,
      ref_domains: 215,
      topical_tf: [{ topic: 'Technology' }],
      language: 'en',
    },
    {
      url: 'https://example.com/products/item',
      title: 'Featured Product',
      trust_flow: 18,
      citation_flow: 44,
      backlinks: 520,
      ref_domains: 142,
      topical_tf: [{ topic: 'Shopping' }],
      language: 'en',
    },
    {
      url: 'https://example.com/about',
      title: 'About Us',
      trust_flow: 25,
      citation_flow: 21,
      backlinks: 310,
      ref_domains: 98,
      topical_tf: [{ topic: 'Business' }],
      language: 'en',
    },
    {
      url: 'https://example.com/contact',
      title: 'Contact Page',
      trust_flow: 12,
      citation_flow: 15,
      backlinks: 95,
      ref_domains: 34,
      topical_tf: [{ topic: 'Reference' }],
      language: 'en',
    },
  ];
  const PH_API_CALLS = [
    {
      timestamp: '2025-11-15T14:32:00Z',
      endpoint: '/api/v1/backlinks',
      status_code: 200,
      duration_ms: 342,
      rows_returned: 1000,
      error: '',
    },
    {
      timestamp: '2025-11-15T14:33:00Z',
      endpoint: '/api/v1/refdomains',
      status_code: 200,
      duration_ms: 256,
      rows_returned: 500,
      error: '',
    },
    {
      timestamp: '2025-11-15T14:34:00Z',
      endpoint: '/api/v1/rankings',
      status_code: 200,
      duration_ms: 890,
      rows_returned: 2000,
      error: '',
    },
    {
      timestamp: '2025-11-15T14:35:00Z',
      endpoint: '/api/v1/top-pages',
      status_code: 200,
      duration_ms: 178,
      rows_returned: 100,
      error: '',
    },
    {
      timestamp: '2025-11-15T14:36:00Z',
      endpoint: '/api/v1/visibility',
      status_code: 429,
      duration_ms: 45,
      rows_returned: 0,
      error: 'Rate limited',
    },
  ];

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
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
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

  let fetchMenuOpen = $state(false);

  async function doFetch(dataTypes = [], force = false) {
    fetchMenuOpen = false;
    fetchingData = true;
    fetchStatus = { fetching: true, phase: 'starting', rows_so_far: 0 };
    try {
      await fetchProviderData(projectId, provider, dataTypes, force);
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
      await connectProvider(projectId, provider, settingsApiKey || undefined, settingsDomain, {
        backlinks: settingsLimitBacklinks,
        refdomains: settingsLimitRefdomains,
        rankings: settingsLimitRankings,
        top_pages: settingsLimitTopPages,
      });
      settingsApiKey = '';
      await loadStatus();
      settingsDomain = status.domain || '';
      settingsLimitBacklinks = status?.limit_backlinks || 1000;
      settingsLimitRefdomains = status?.limit_refdomains || 1000;
      settingsLimitRankings = status?.limit_rankings || 1000;
      settingsLimitTopPages = status?.limit_top_pages || 1000;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      updating = false;
    }
  }

  function filterByTopic(topic) {
    topPagesFilters = { ...topPagesFilters, topic };
    topPagesOffset = 0;
    loadSubView('top_pages');
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
        const raw = await getProviderData(
          projectId,
          provider,
          'top_pages',
          PAGE_LIMIT,
          topPagesOffset,
          topPagesFilters,
          topPagesSortCol,
          topPagesSortOrder,
        );
        topPages = {
          total: raw.total,
          rows: (raw.rows || []).map((r, idx) => ({
            _index: idx,
            url: r.item_url,
            title: r.str_data?.title || '',
            trust_flow: r.trust_flow,
            citation_flow: r.citation_flow,
            backlinks: r.ext_backlinks,
            ref_domains: r.ref_domains,
            language: r.str_data?.language || '',
            last_crawl_result: r.str_data?.last_crawl_result || '',
            last_crawl_date: r.str_data?.last_crawl_date || '',
            topical_tf: Object.keys(r.str_data || {})
              .filter((k) => k.startsWith('ttf_topic_'))
              .sort()
              .map((k) => {
                const idx = k.replace('ttf_topic_', '');
                return { topic: r.str_data[k], value: r.num_data?.[`ttf_value_${idx}`] || 0 };
              }),
          })),
        };
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
      settingsLimitBacklinks = status?.limit_backlinks || 1000;
      settingsLimitRefdomains = status?.limit_refdomains || 1000;
      settingsLimitRankings = status?.limit_rankings || 1000;
      settingsLimitTopPages = status?.limit_top_pages || 1000;
    }
    onpushurl?.(`/projects/${projectId}/providers/${view}`);
    loadSubView(view);
  }

  loadStatus().then(() => {
    if (projectId) loadSubView(subView);
  });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="pr-container" onclick={() => (fetchMenuOpen = false)}>
  {#if !projectId}
    <div class="prov-empty">
      <p>{t('providers.notAssociated')}</p>
      <p class="text-muted text-sm">{t('providers.associateFirst')}</p>
    </div>
  {:else if !status}
    <p class="loading-msg">{t('common.loading')}</p>
  {:else}
    <!-- Toolbar: only when connected -->
    {#if status.connected}
      <div class="prov-toolbar">
        <span class="text-sm text-secondary">
          {t('providers.domain')} <strong>{status.domain}</strong>
          <span class="prov-provider-tag">({provider})</span>
        </span>
        <div class="flex-center-gap">
          {#if fetchingData}
            <span class="fetch-indicator">
              <span class="fetch-spinner"></span>
              {fetchStatus?.phase || t('providers.fetchingPhase')}{fetchStatus?.rows_so_far
                ? ` — ${fmtN(fetchStatus.rows_so_far)} rows`
                : '...'}
            </span>
            <button class="btn btn-sm text-danger" onclick={doStop}>{t('common.stop')}</button>
          {:else}
            <div class="split-btn-wrap">
              <button class="btn btn-sm" onclick={() => doFetch()}
                >{t('providers.fetchData')}</button
              >
              <button
                class="btn btn-sm split-btn-arrow"
                onclick={(e) => {
                  e.stopPropagation();
                  fetchMenuOpen = !fetchMenuOpen;
                }}
                aria-label="More fetch options"
              >
                <svg width="10" height="6" viewBox="0 0 10 6" fill="currentColor"
                  ><path d="M0 0l5 6 5-6z" /></svg
                >
              </button>
              {#if fetchMenuOpen}
                <div class="split-btn-menu" role="menu">
                  <button class="split-btn-item" onclick={() => doFetch([], true)}
                    >{t('providers.forceRefresh')}</button
                  >
                </div>
              {/if}
            </div>
          {/if}
        </div>
      </div>
    {/if}

    <div class="pr-subview-bar">
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === 'overview'}
        onclick={() => switchSubView('overview')}>{t('providers.overview')}</button
      >
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === 'backlinks'}
        onclick={() => switchSubView('backlinks')}>{t('providers.backlinks')}</button
      >
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === 'refdomains'}
        onclick={() => switchSubView('refdomains')}>{t('providers.refDomains')}</button
      >
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === 'rankings'}
        onclick={() => switchSubView('rankings')}>{t('providers.rankings')}</button
      >
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === 'top_pages'}
        onclick={() => switchSubView('top_pages')}>{t('providers.topPagesTab')}</button
      >
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === 'api_calls'}
        onclick={() => switchSubView('api_calls')}>{t('providers.apiCallsTab')}</button
      >
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === 'settings'}
        onclick={() => switchSubView('settings')}>{t('providers.settings')}</button
      >
    </div>

    {#if !status.connected}
      <!-- Disconnected: locked previews to create desire -->
      {#if subView === 'settings'}
        <div class="prov-empty">
          <h3 class="prov-connect-title">{t('providers.connectTitle')}</h3>
          <p class="text-muted text-sm mb-md">{t('providers.connectDesc')}</p>
          <div class="prov-connect-form">
            <input
              type="text"
              class="pr-input"
              placeholder={t('providers.domainPlaceholder')}
              bind:value={domainInput}
            />
            <input
              type="password"
              class="pr-input"
              placeholder={t('providers.apiKey')}
              bind:value={apiKeyInput}
            />
            <button
              class="btn btn-primary"
              onclick={doConnect}
              disabled={connecting || !apiKeyInput || !domainInput}
            >
              {connecting ? t('providers.connecting') : t('common.connect')}
            </button>
          </div>
        </div>
      {:else}
        <div class="prov-lock-cta">
          <svg
            width="36"
            height="36"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" /><path
              d="M7 11V7a5 5 0 0 1 10 0v4"
            />
          </svg>
          <p class="prov-lock-text">{t('providers.lockCta')}</p>
          <button class="btn btn-primary" onclick={() => switchSubView('settings')}
            >{t('providers.connectTitle')}</button
          >
        </div>
        <div class="prov-preview-wrapper">
          <div class="prov-preview-blur">
            {#if subView === 'overview'}
              <div class="stats-grid prov-stats">
                <div class="stat-card">
                  <div class="stat-value">{PH_METRICS.domain_rank}</div>
                  <div class="stat-label">{t('providers.domainRank')}</div>
                </div>
                <div class="stat-card">
                  <div class="stat-value">{fmtN(PH_METRICS.backlinks_total)}</div>
                  <div class="stat-label">{t('providers.totalBacklinks')}</div>
                </div>
                <div class="stat-card">
                  <div class="stat-value">{fmtN(PH_METRICS.refdomains_total)}</div>
                  <div class="stat-label">{t('providers.referringDomains')}</div>
                </div>
                <div class="stat-card">
                  <div class="stat-value">{fmtN(PH_METRICS.organic_keywords)}</div>
                  <div class="stat-label">{t('providers.organicKeywords')}</div>
                </div>
                <div class="stat-card">
                  <div class="stat-value">{fmtN(PH_METRICS.organic_traffic)}</div>
                  <div class="stat-label">{t('providers.organicTraffic')}</div>
                </div>
                <div class="stat-card">
                  <div class="stat-value">${PH_METRICS.organic_cost}</div>
                  <div class="stat-label">{t('providers.trafficValue')}</div>
                </div>
              </div>
            {:else if subView === 'backlinks'}
              <table>
                <thead
                  ><tr
                    ><th>#</th><th>{t('providers.sourceURL')}</th><th>{t('providers.targetURL')}</th
                    ><th>{t('providers.anchor')}</th><th>{t('providers.dr')}</th><th
                      >{t('providers.nf')}</th
                    ><th>{t('providers.firstSeen')}</th></tr
                  ></thead
                >
                <tbody>
                  {#each PH_BACKLINKS as r, i}
                    <tr
                      ><td class="row-num">{i + 1}</td><td class="cell-url prov-cell-url"
                        >{r.source_url}</td
                      ><td class="cell-url prov-cell-target">{r.target_url}</td><td
                        class="prov-cell-anchor">{r.anchor_text}</td
                      ><td>{r.domain_rank}</td><td
                        >{r.nofollow ? t('providers.nf') : t('providers.df')}</td
                      ><td class="text-xs nowrap">{r.first_seen}</td></tr
                    >
                  {/each}
                </tbody>
              </table>
            {:else if subView === 'refdomains'}
              <table>
                <thead
                  ><tr
                    ><th>#</th><th>{t('providers.domainCol')}</th><th>{t('providers.backlinks')}</th
                    ><th>{t('providers.dr')}</th><th>{t('providers.firstSeen')}</th><th
                      >{t('providers.lastSeen')}</th
                    ></tr
                  ></thead
                >
                <tbody>
                  {#each PH_REFDOMAINS as r, i}
                    <tr
                      ><td class="row-num">{i + 1}</td><td><strong>{r.ref_domain}</strong></td><td
                        >{fmtN(r.backlink_count)}</td
                      ><td>{r.domain_rank}</td><td class="text-xs nowrap">{r.first_seen}</td><td
                        class="text-xs nowrap">{r.last_seen}</td
                      ></tr
                    >
                  {/each}
                </tbody>
              </table>
            {:else if subView === 'rankings'}
              <table>
                <thead
                  ><tr
                    ><th>#</th><th>{t('providers.keyword')}</th><th>{t('providers.pos')}</th><th
                      >{t('common.url')}</th
                    ><th>{t('providers.volume')}</th><th>{t('providers.cpc')}</th><th
                      >{t('providers.traffic')}</th
                    ></tr
                  ></thead
                >
                <tbody>
                  {#each PH_RANKINGS as r, i}
                    <tr
                      ><td class="row-num">{i + 1}</td><td><strong>{r.keyword}</strong></td><td
                        ><span
                          class="badge"
                          class:badge-success={r.position <= 3}
                          class:badge-warn={r.position > 3 && r.position <= 10}>{r.position}</span
                        ></td
                      ><td class="cell-url prov-cell-url">{r.url}</td><td
                        >{fmtN(r.search_volume)}</td
                      ><td>{r.cpc.toFixed(2)}</td><td>{r.traffic}</td></tr
                    >
                  {/each}
                </tbody>
              </table>
            {:else if subView === 'top_pages'}
              <table>
                <thead
                  ><tr
                    ><th>#</th><th>{t('common.url')}</th><th>{t('providers.title')}</th><th
                      >{t('providers.tf')}</th
                    ><th>{t('providers.cf')}</th><th>{t('providers.backlinks')}</th><th
                      >{t('providers.refDomains')}</th
                    ><th>{t('providers.topic')}</th><th>{t('providers.lang')}</th></tr
                  ></thead
                >
                <tbody>
                  {#each PH_TOP_PAGES as r, i}
                    <tr>
                      <td class="row-num">{i + 1}</td>
                      <td class="cell-url prov-cell-url">{r.url}</td>
                      <td class="prov-cell-anchor">{r.title}</td>
                      <td
                        ><span
                          class="badge prov-metric-badge"
                          style="background-color: {r.trust_flow > 40
                            ? '#22c55e22'
                            : r.trust_flow >= 20
                              ? '#f59e0b22'
                              : '#ef444422'}; color: {r.trust_flow > 40
                            ? '#16a34a'
                            : r.trust_flow >= 20
                              ? '#d97706'
                              : '#dc2626'}">{r.trust_flow}</span
                        ></td
                      >
                      <td
                        ><span
                          class="badge prov-metric-badge"
                          style="background-color: {r.citation_flow > 40
                            ? '#22c55e22'
                            : r.citation_flow >= 20
                              ? '#f59e0b22'
                              : '#ef444422'}; color: {r.citation_flow > 40
                            ? '#16a34a'
                            : r.citation_flow >= 20
                              ? '#d97706'
                              : '#dc2626'}">{r.citation_flow}</span
                        ></td
                      >
                      <td>{fmtN(r.backlinks)}</td>
                      <td>{fmtN(r.ref_domains)}</td>
                      <td class="text-xs"
                        >{#if r.topical_tf?.[0]?.topic}<span
                            class="ttf_label {r.topical_tf[0].topic.split('/')[0].toLowerCase()}"
                            >{r.topical_tf[0].topic}</span
                          >{:else}-{/if}</td
                      >
                      <td class="text-xs">{r.language}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {:else if subView === 'api_calls'}
              <table>
                <thead
                  ><tr
                    ><th>#</th><th>{t('providers.time')}</th><th>{t('providers.endpoint')}</th><th
                      >{t('providers.status')}</th
                    ><th>{t('providers.duration')}</th><th>{t('providers.rows')}</th><th
                      >{t('providers.error')}</th
                    ></tr
                  ></thead
                >
                <tbody>
                  {#each PH_API_CALLS as r, i}
                    <tr>
                      <td class="row-num">{i + 1}</td>
                      <td class="text-xs nowrap">{new Date(r.timestamp).toLocaleString()}</td>
                      <td class="prov-cell-anchor">{r.endpoint}</td>
                      <td
                        ><span
                          class="badge"
                          style="background-color: {r.status_code >= 200 && r.status_code < 300
                            ? '#22c55e22'
                            : r.status_code >= 400
                              ? '#ef444422'
                              : '#f59e0b22'}; color: {r.status_code >= 200 && r.status_code < 300
                            ? '#16a34a'
                            : r.status_code >= 400
                              ? '#dc2626'
                              : '#d97706'}">{r.status_code}</span
                        ></td
                      >
                      <td class="text-xs nowrap">{r.duration_ms}ms</td>
                      <td>{fmtN(r.rows_returned)}</td>
                      <td class="text-xs prov-cell-anchor">{r.error || '-'}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {/if}
          </div>
        </div>
      {/if}
    {:else if loading}
      <p class="loading-msg">{t('common.loading')}</p>
    {:else if subView === 'overview'}
      {#if metrics && (metrics.backlinks_total > 0 || metrics.domain_rank > 0)}
        <div class="stats-grid prov-stats">
          <div class="stat-card">
            <div class="stat-value">{metrics.domain_rank?.toFixed(1) || '0'}</div>
            <div class="stat-label">{t('providers.domainRank')}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{fmtN(metrics.backlinks_total)}</div>
            <div class="stat-label">{t('providers.totalBacklinks')}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{fmtN(metrics.refdomains_total)}</div>
            <div class="stat-label">{t('providers.referringDomains')}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{fmtN(metrics.organic_keywords)}</div>
            <div class="stat-label">{t('providers.organicKeywords')}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{fmtN(metrics.organic_traffic)}</div>
            <div class="stat-label">{t('providers.organicTraffic')}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">${metrics.organic_cost?.toFixed(0) || '0'}</div>
            <div class="stat-label">{t('providers.trafficValue')}</div>
          </div>
        </div>

        <!-- Visibility Chart -->
        {#if visibility?.length > 1}
          {@const maxVis = Math.max(...visibility.map((v) => v.visibility), 1)}
          {@const chartW = 700}
          {@const chartH = 200}
          {@const margin = { left: 50, right: 20, top: 10, bottom: 30 }}
          {@const plotW = chartW - margin.left - margin.right}
          {@const plotH = chartH - margin.top - margin.bottom}
          <h4 class="sub-heading">{t('providers.visibilityOverTime')}</h4>
          <svg viewBox="0 0 {chartW} {chartH}" class="prov-chart-svg">
            <path
              d="M {margin.left},{margin.top + plotH}
              {visibility
                .map(
                  (v, i) =>
                    `L ${margin.left + (i / (visibility.length - 1)) * plotW},${margin.top + plotH - (v.visibility / maxVis) * plotH}`,
                )
                .join(' ')}
              L {margin.left + plotW},{margin.top + plotH} Z"
              fill="var(--accent)"
              opacity="0.1"
            />
            <polyline
              points={visibility
                .map(
                  (v, i) =>
                    `${margin.left + (i / (visibility.length - 1)) * plotW},${margin.top + plotH - (v.visibility / maxVis) * plotH}`,
                )
                .join(' ')}
              fill="none"
              stroke="var(--accent)"
              stroke-width="2"
            />
            {#each [0, Math.floor(visibility.length / 2), visibility.length - 1] as idx}
              <text
                x={margin.left + (idx / (visibility.length - 1)) * plotW}
                y={chartH - 4}
                text-anchor="middle"
                class="prov-axis-label"
              >
                {visibility[idx].date?.slice?.(0, 10) || ''}
              </text>
            {/each}
            <text x={12} y={margin.top + 10} class="prov-chart-legend"
              >{t('providers.visibility')}</text
            >
          </svg>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noData')}</p>
      {/if}
    {:else if subView === 'backlinks'}
      {#if backlinks?.rows?.length > 0}
        <div class="table-meta">
          {t('providers.backlinksCount', { count: fmtN(backlinks.total) })}
        </div>
        <table>
          <thead
            ><tr
              ><th>#</th><th>{t('providers.sourceURL')}</th><th>{t('providers.targetURL')}</th><th
                >{t('providers.anchor')}</th
              ><th>{t('providers.dr')}</th><th>{t('providers.nf')}</th><th
                >{t('providers.firstSeen')}</th
              ></tr
            ></thead
          >
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
            <button
              class="btn btn-sm"
              disabled={backlinksOffset === 0}
              onclick={() => {
                backlinksOffset = Math.max(0, backlinksOffset - PAGE_LIMIT);
                loadSubView('backlinks');
              }}>{t('common.previous')}</button
            >
            <span class="pagination-info"
              >{backlinksOffset + 1} - {Math.min(backlinksOffset + PAGE_LIMIT, backlinks.total)} of {fmtN(
                backlinks.total,
              )}</span
            >
            <button
              class="btn btn-sm"
              disabled={backlinksOffset + PAGE_LIMIT >= backlinks.total}
              onclick={() => {
                backlinksOffset += PAGE_LIMIT;
                loadSubView('backlinks');
              }}>{t('common.next')}</button
            >
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noBacklinks')}</p>
      {/if}
    {:else if subView === 'refdomains'}
      {#if refdomains?.rows?.length > 0}
        <div class="table-meta">
          {t('providers.refDomainsCount', { count: fmtN(refdomains.total) })}
        </div>
        <table>
          <thead
            ><tr
              ><th>#</th><th>{t('providers.domainCol')}</th><th>{t('providers.backlinks')}</th><th
                >{t('providers.dr')}</th
              ><th>{t('providers.firstSeen')}</th><th>{t('providers.lastSeen')}</th></tr
            ></thead
          >
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
            <button
              class="btn btn-sm"
              disabled={refdomainsOffset === 0}
              onclick={() => {
                refdomainsOffset = Math.max(0, refdomainsOffset - PAGE_LIMIT);
                loadSubView('refdomains');
              }}>{t('common.previous')}</button
            >
            <span class="pagination-info"
              >{refdomainsOffset + 1} - {Math.min(refdomainsOffset + PAGE_LIMIT, refdomains.total)} of
              {fmtN(refdomains.total)}</span
            >
            <button
              class="btn btn-sm"
              disabled={refdomainsOffset + PAGE_LIMIT >= refdomains.total}
              onclick={() => {
                refdomainsOffset += PAGE_LIMIT;
                loadSubView('refdomains');
              }}>{t('common.next')}</button
            >
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noRefDomains')}</p>
      {/if}
    {:else if subView === 'rankings'}
      {#if rankings?.rows?.length > 0}
        <div class="table-meta">
          {t('providers.keywordsCount', { count: fmtN(rankings.total) })}
        </div>
        <table>
          <thead
            ><tr
              ><th>#</th><th>{t('providers.keyword')}</th><th>{t('providers.pos')}</th><th
                >{t('common.url')}</th
              ><th>{t('providers.volume')}</th><th>{t('providers.cpc')}</th><th
                >{t('providers.traffic')}</th
              ></tr
            ></thead
          >
          <tbody>
            {#each rankings.rows as r, i}
              <tr>
                <td class="row-num">{rankingsOffset + i + 1}</td>
                <td><strong>{r.keyword}</strong></td>
                <td
                  ><span
                    class="badge"
                    class:badge-success={r.position <= 3}
                    class:badge-warn={r.position > 3 && r.position <= 10}>{r.position}</span
                  ></td
                >
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
            <button
              class="btn btn-sm"
              disabled={rankingsOffset === 0}
              onclick={() => {
                rankingsOffset = Math.max(0, rankingsOffset - PAGE_LIMIT);
                loadSubView('rankings');
              }}>{t('common.previous')}</button
            >
            <span class="pagination-info"
              >{rankingsOffset + 1} - {Math.min(rankingsOffset + PAGE_LIMIT, rankings.total)} of {fmtN(
                rankings.total,
              )}</span
            >
            <button
              class="btn btn-sm"
              disabled={rankingsOffset + PAGE_LIMIT >= rankings.total}
              onclick={() => {
                rankingsOffset += PAGE_LIMIT;
                loadSubView('rankings');
              }}>{t('common.next')}</button
            >
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('providers.noRankings')}</p>
      {/if}
    {:else if subView === 'top_pages'}
      {#if topPages?.rows?.length > 0 || Object.values(topPagesFilters).some((v) => v)}
        <div class="table-meta">
          {t('providers.topPagesCount', { count: fmtN(topPages?.total ?? 0) })}
        </div>
        <DataTable
          columns={[
            { label: '#' },
            { label: t('common.url'), sortKey: 'item_url' },
            { label: t('providers.title') },
            { label: t('providers.tf'), sortKey: 'trust_flow' },
            { label: t('providers.cf'), sortKey: 'citation_flow' },
            { label: t('providers.backlinks'), sortKey: 'ext_backlinks' },
            { label: t('providers.refDomains'), sortKey: 'ref_domains' },
            { label: t('providers.topic') },
            { label: t('providers.lastCrawl') },
            { label: t('providers.lang') },
          ]}
          filterKeys={[
            '',
            'item_url',
            'title',
            'trust_flow',
            'citation_flow',
            'ext_backlinks',
            'ref_domains',
            'topic',
            '',
            'language',
          ]}
          filters={topPagesFilters}
          data={topPages?.rows || []}
          offset={topPagesOffset}
          pageSize={PAGE_LIMIT}
          hasMore={topPages ? topPagesOffset + PAGE_LIMIT < topPages.total : false}
          hasActiveFilters={Object.values(topPagesFilters).some((v) => v)}
          sortColumn={topPagesSortCol}
          sortOrder={topPagesSortOrder}
          onsetfilter={(key, val) => {
            topPagesFilters[key] = val;
            topPagesFilters = { ...topPagesFilters };
          }}
          onapplyfilters={() => {
            topPagesOffset = 0;
            loadSubView('top_pages');
          }}
          onclearfilters={() => {
            topPagesFilters = {};
            topPagesOffset = 0;
            loadSubView('top_pages');
          }}
          onnextpage={() => {
            topPagesOffset += PAGE_LIMIT;
            loadSubView('top_pages');
          }}
          onprevpage={() => {
            topPagesOffset = Math.max(0, topPagesOffset - PAGE_LIMIT);
            loadSubView('top_pages');
          }}
          onsort={(col, ord) => {
            topPagesSortCol = col;
            topPagesSortOrder = ord;
            topPagesOffset = 0;
            loadSubView('top_pages');
          }}
        >
          {#snippet row(r)}
            <tr>
              <td class="row-num">{topPagesOffset + r._index + 1}</td>
              <td class="cell-url">
                <span class="cell-url-inner"
                  ><a href={r.url} target="_blank" rel="noopener">{r.url}</a><UrlActions
                    url={r.url}
                  /></span
                >
              </td>
              <td class="cell-title">{r.title || '-'}</td>
              <td
                >{#if r.topical_tf?.[0]?.topic}<span
                    class="ttf_label ttf-clickable {r.topical_tf[0].topic
                      .split('/')[0]
                      .toLowerCase()}"
                    onclick={() => filterByTopic(r.topical_tf[0].topic)}
                    role="button"
                    tabindex="0">{r.trust_flow}</span
                  >{:else}{r.trust_flow ?? '-'}{/if}</td
              >
              <td>{r.citation_flow ?? '-'}</td>
              <td>{fmtN(r.backlinks ?? 0)}</td>
              <td>{fmtN(r.ref_domains ?? 0)}</td>
              <td class="text-xs"
                >{#if r.topical_tf?.[0]?.topic}<span
                    class="ttf_label ttf-clickable {r.topical_tf[0].topic
                      .split('/')[0]
                      .toLowerCase()}"
                    onclick={() => filterByTopic(r.topical_tf[0].topic)}
                    role="button"
                    tabindex="0">{r.topical_tf[0].topic}</span
                  >{:else}-{/if}</td
              >
              <td class="text-xs nowrap"
                >{#if r.last_crawl_result}<span
                    class="crawl-status-icon tooltip-wrap"
                    class:crawl-ok={r.last_crawl_result === 'DownloadedSuccessfully'}
                    class:crawl-redirect={r.last_crawl_result.includes('Redirect')}
                    class:crawl-unknown={r.last_crawl_result === 'NotCrawled'}
                    class:crawl-error={r.last_crawl_result !== 'DownloadedSuccessfully' &&
                      !r.last_crawl_result.includes('Redirect') &&
                      r.last_crawl_result !== 'NotCrawled'}
                    ><span class="tooltip-text"
                      >{r.last_crawl_result.replace(/_/g, ' ')}{r.last_crawl_date
                        ? ' — ' + r.last_crawl_date
                        : ''}</span
                    >{#if r.last_crawl_result === 'DownloadedSuccessfully'}<svg
                        viewBox="0 0 24 24"
                        width="14"
                        height="14"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"><path d="M20 6L9 17l-5-5" /></svg
                      >{:else if r.last_crawl_result.includes('Redirect')}<svg
                        viewBox="0 0 24 24"
                        width="14"
                        height="14"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"><path d="M5 12h14m-4-4l4 4-4 4" /></svg
                      >{:else if r.last_crawl_result === 'NotCrawled'}<svg
                        viewBox="0 0 24 24"
                        width="14"
                        height="14"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        ><circle cx="12" cy="12" r="10" /><line
                          x1="12"
                          y1="8"
                          x2="12"
                          y2="12"
                        /><circle cx="12" cy="16" r="0.5" fill="currentColor" /></svg
                      >{:else}<svg
                        viewBox="0 0 24 24"
                        width="14"
                        height="14"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        ><circle cx="12" cy="12" r="10" /><line
                          x1="15"
                          y1="9"
                          x2="9"
                          y2="15"
                        /><line x1="9" y1="9" x2="15" y2="15" /></svg
                      >{/if}</span
                  >{:else}-{/if}</td
              >
              <td class="text-xs">{r.language || '-'}</td>
            </tr>
          {/snippet}
        </DataTable>
      {:else}
        <div class="chart-empty">
          <p>{t('providers.noTopPages')}</p>
          {#if !fetchingData}
            <button
              class="btn btn-primary"
              style="margin-top: 0.75rem"
              onclick={() => doFetch(['top_pages'])}>{t('providers.fetchData')}</button
            >
          {/if}
        </div>
      {/if}
    {:else if subView === 'api_calls'}
      {#if apiCalls?.rows?.length > 0}
        <div class="table-meta">
          {t('providers.apiCallsCount', { count: fmtN(apiCalls.total) })}
        </div>
        <table>
          <thead
            ><tr
              ><th>#</th><th>{t('providers.time')}</th><th>{t('providers.endpoint')}</th><th
                >{t('providers.status')}</th
              ><th>{t('providers.duration')}</th><th>{t('providers.rows')}</th><th
                >{t('providers.error')}</th
              ></tr
            ></thead
          >
          <tbody>
            {#each apiCalls.rows as r, i}
              <tr>
                <td class="row-num">{apiCallsOffset + i + 1}</td>
                <td class="text-xs nowrap"
                  >{r.timestamp ? new Date(r.timestamp).toLocaleString() : '-'}</td
                >
                <td class="prov-cell-anchor">{r.endpoint || '-'}</td>
                <td
                  ><span
                    class="badge"
                    style="background-color: {r.status_code >= 200 && r.status_code < 300
                      ? '#22c55e22'
                      : r.status_code >= 400
                        ? '#ef444422'
                        : '#f59e0b22'}; color: {r.status_code >= 200 && r.status_code < 300
                      ? '#16a34a'
                      : r.status_code >= 400
                        ? '#dc2626'
                        : '#d97706'}">{r.status_code ?? '-'}</span
                  ></td
                >
                <td class="text-xs nowrap">{r.duration_ms != null ? `${r.duration_ms}ms` : '-'}</td>
                <td>{r.rows_returned != null ? fmtN(r.rows_returned) : '-'}</td>
                <td class="text-xs prov-cell-anchor">{r.error || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if apiCalls.total > 50}
          <div class="pagination">
            <button
              class="btn btn-sm"
              disabled={apiCallsOffset === 0}
              onclick={() => {
                apiCallsOffset = Math.max(0, apiCallsOffset - 50);
                loadSubView('api_calls');
              }}>{t('common.previous')}</button
            >
            <span class="pagination-info"
              >{apiCallsOffset + 1} - {Math.min(apiCallsOffset + 50, apiCalls.total)} of {fmtN(
                apiCalls.total,
              )}</span
            >
            <button
              class="btn btn-sm"
              disabled={apiCallsOffset + 50 >= apiCalls.total}
              onclick={() => {
                apiCallsOffset += 50;
                loadSubView('api_calls');
              }}>{t('common.next')}</button
            >
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
            <span class="settings-value"
              >{status.created_at ? new Date(status.created_at).toLocaleDateString() : '-'}</span
            >
          </div>
        </div>

        <h4 class="prov-settings-subtitle">{t('providers.updateConnection')}</h4>
        <div class="prov-settings-form">
          <label class="settings-field-label">
            {t('providers.domainCol')}
            <input
              type="text"
              class="pr-input"
              bind:value={settingsDomain}
              placeholder="example.com"
            />
          </label>
          <label class="settings-field-label">
            {t('providers.apiKeyLabel')}
            <input
              type="password"
              class="pr-input"
              bind:value={settingsApiKey}
              placeholder={t('providers.keepCurrentKey')}
            />
          </label>
          <h4 class="prov-settings-subtitle">{t('providers.fetchLimits')}</h4>
          <p class="text-muted text-sm" style="margin-bottom:8px">
            {t('providers.fetchLimitsHelp')}
          </p>
          <div class="prov-limits-grid">
            <label class="settings-field-label">
              {t('providers.limitBacklinks')}
              <input
                type="number"
                class="pr-input"
                bind:value={settingsLimitBacklinks}
                min="1"
                max="10000"
              />
            </label>
            <label class="settings-field-label">
              {t('providers.limitRefdomains')}
              <input
                type="number"
                class="pr-input"
                bind:value={settingsLimitRefdomains}
                min="1"
                max="10000"
              />
            </label>
            <label class="settings-field-label">
              {t('providers.limitRankings')}
              <input
                type="number"
                class="pr-input"
                bind:value={settingsLimitRankings}
                min="1"
                max="10000"
              />
            </label>
            <label class="settings-field-label">
              {t('providers.limitTopPages')}
              <input
                type="number"
                class="pr-input"
                bind:value={settingsLimitTopPages}
                min="1"
                max="10000"
              />
            </label>
          </div>

          <button
            class="btn btn-primary prov-settings-submit"
            onclick={doUpdate}
            disabled={updating || !settingsDomain}
          >
            {updating ? t('providers.updating') : t('common.update')}
          </button>
        </div>

        <div class="prov-danger-zone">
          <h4 class="prov-danger-title">{t('providers.dangerZone')}</h4>
          <p class="prov-danger-text">{t('providers.disconnectWarning')}</p>
          <button class="btn btn-sm prov-disconnect-btn" onclick={doDisconnect}
            >{t('common.disconnect')}</button
          >
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
  .btn-primary:hover {
    opacity: 0.9;
  }
  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .badge-success {
    background: #22c55e22;
    color: #16a34a;
  }
  .badge-warn {
    background: #f59e0b22;
    color: #d97706;
  }
  .split-btn-wrap {
    position: relative;
    display: inline-flex;
  }
  .split-btn-wrap > .btn:first-child {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
  }
  .split-btn-arrow {
    border-top-left-radius: 0 !important;
    border-bottom-left-radius: 0 !important;
    border-left: 1px solid var(--border) !important;
    padding-inline: 6px !important;
    min-width: 0;
  }
  .split-btn-menu {
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 4px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
    z-index: 20;
    min-width: 160px;
  }
  .split-btn-item {
    display: block;
    width: 100%;
    padding: 7px 12px;
    text-align: left;
    font-size: 13px;
    border: none;
    background: none;
    color: var(--text-primary);
    cursor: pointer;
    border-radius: 6px;
  }
  .split-btn-item:hover {
    background: var(--bg-hover);
  }
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
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .settings-section {
    max-width: 520px;
  }
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
  .settings-row + .settings-row {
    border-top: 1px solid var(--border);
  }
  .settings-label {
    font-size: 13px;
    color: var(--text-muted);
  }
  .settings-value {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
  }
  .settings-field-label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: var(--text-secondary);
    font-weight: 500;
  }
  .prov-connect-title {
    margin-bottom: 12px;
    font-size: 16px;
  }
  .prov-connect-form {
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-width: 400px;
    margin: 0 auto;
  }
  .prov-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }
  .prov-provider-tag {
    color: var(--text-muted);
    margin-left: 8px;
  }
  .prov-stats {
    margin-bottom: 20px;
  }
  .prov-chart-svg {
    width: 100%;
    max-width: 800px;
    height: auto;
    margin-bottom: 24px;
  }
  .prov-axis-label {
    font-size: 10px;
    fill: var(--text-muted);
  }
  .prov-chart-legend {
    font-size: 9px;
    fill: var(--accent);
  }
  .prov-cell-url {
    max-width: 250px;
    white-space: nowrap;
  }
  .prov-cell-target {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .prov-cell-anchor {
    max-width: 150px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .prov-settings-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 16px;
  }
  .prov-settings-subtitle {
    font-size: 14px;
    font-weight: 600;
    margin: 24px 0 12px;
  }
  .prov-settings-form {
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-width: 400px;
  }
  .prov-settings-submit {
    align-self: flex-start;
  }
  .prov-limits-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
    margin-bottom: 12px;
  }
  .prov-danger-zone {
    margin-top: 32px;
    padding-top: 20px;
    border-top: 1px solid var(--border);
  }
  .prov-danger-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 8px;
    color: var(--danger, #e53e3e);
  }
  .prov-danger-text {
    font-size: 13px;
    color: var(--text-muted);
    margin-bottom: 12px;
  }
  .prov-disconnect-btn {
    color: var(--danger, #e53e3e);
    border-color: var(--danger, #e53e3e);
  }

  /* Lock overlay for disconnected state */
  .prov-lock-cta {
    text-align: center;
    padding: 48px 20px 24px;
    color: var(--text-primary);
    position: relative;
    z-index: 1;
  }
  .prov-lock-cta svg {
    color: var(--text-muted);
  }
  .prov-lock-text {
    max-width: 460px;
    margin: 16px auto 20px;
    font-size: 14px;
    color: var(--text-secondary);
    line-height: 1.5;
  }
  .prov-preview-wrapper {
    position: relative;
    overflow: hidden;
    border-radius: 8px;
  }
  .prov-preview-blur {
    filter: blur(4px);
    opacity: 0.45;
    pointer-events: none;
    user-select: none;
  }
  .ttf_label {
    font-weight: 700;
    font-size: 8.5pt;
    border-radius: 4px;
    padding: 1px 5px;
    display: inline-block;
    white-space: nowrap;
  }
  .ttf_label.arts {
    background: #ff6700;
    color: #fff;
  }
  .ttf_label.news {
    background: #76d54b;
    color: #333;
  }
  .ttf_label.society {
    background: #7a69cd;
    color: #fff;
  }
  .ttf_label.computers {
    background: #f33;
    color: #fff;
  }
  .ttf_label.business {
    background: #c5c88e;
    color: #333;
  }
  .ttf_label.regional {
    background: #f582b9;
    color: #fff;
  }
  .ttf_label.recreation {
    background: #89c7cb;
    color: #333;
  }
  .ttf_label.sports {
    background: #55355d;
    color: #fff;
  }
  .ttf_label.kids {
    background: #fc0;
    color: #333;
  }
  .ttf_label.reference {
    background: #c84770;
    color: #fff;
  }
  .ttf_label.games {
    background: #557832;
    color: #fff;
  }
  .ttf_label.home {
    background: #d95;
    color: #fff;
  }
  .ttf_label.shopping {
    background: #600;
    color: #fff;
  }
  .ttf_label.health {
    background: #009;
    color: #fff;
  }
  .ttf_label.science {
    background: #6bd39a;
    color: #333;
  }
  .ttf_label.world {
    background: #577;
    color: #fff;
  }
  .ttf_label.adult {
    background: #333;
    color: #fff;
  }
  .ttf-clickable {
    cursor: pointer;
  }
  .ttf-clickable:hover {
    opacity: 0.8;
  }
  .crawl-status-icon {
    display: inline-flex;
    align-items: center;
    cursor: help;
  }
  .crawl-ok {
    color: #16a34a;
  }
  .crawl-redirect {
    color: #d97706;
  }
  .crawl-unknown {
    color: #eab308;
  }
  .crawl-error {
    color: #dc2626;
  }
  .tooltip-wrap {
    position: relative;
  }
  .tooltip-text {
    display: none;
    position: absolute;
    bottom: 100%;
    left: 50%;
    transform: translateX(-50%);
    background: var(--bg-secondary, #1e1e2e);
    color: var(--text-primary, #cdd6f4);
    border: 1px solid var(--border-color, #45475a);
    padding: 4px 8px;
    border-radius: 4px;
    font-size: 11px;
    white-space: nowrap;
    z-index: 100;
    pointer-events: none;
  }
  .tooltip-wrap:hover .tooltip-text {
    display: block;
  }
</style>
