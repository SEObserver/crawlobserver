<script>
  import { t } from '../i18n/index.svelte.js';
  import { statusBadge, fmt, fmtSize, fmtN } from '../utils.js';
  import { getTabs, TAB_SUB_VIEWS, TAB_DEFAULT_SUB_VIEW } from '../tabColumns.js';
  import { pushURL } from '../router.js';
  import HtmlModal from './HtmlModal.svelte';
  import SessionActionBar from './SessionActionBar.svelte';
  import UrlDetailView from './UrlDetailView.svelte';
  import PageRankTab from './PageRankTab.svelte';
  import ReportsHub from './ReportsHub.svelte';
  import CustomTestsTab from './CustomTestsTab.svelte';
  import ResourceChecksTab from './ResourceChecksTab.svelte';
  import PagesExplorer from './PagesExplorer.svelte';
  import LinksExplorer from './LinksExplorer.svelte';
  import DirectivesTab from './DirectivesTab.svelte';

  let {
    session, stats, liveProgress, sessionStorageMap,
    initialTab = 'reports', initialFilters = {}, initialOffset = 0,
    initialDetailUrl = '',
    initialSubView = null,
    onerror, onstop, onresume, ondelete, onrefresh, oncompare, onnavigate, ongohome,
  } = $props();

  let tab = $state(initialTab);
  let detailUrl = $state(initialDetailUrl);
  let subView = $state(initialSubView);
  let showHtmlModal = $state(false);
  let htmlModalUrl = $state('');

  function switchTab(newTab) {
    tab = newTab;
    const defaultSv = TAB_DEFAULT_SUB_VIEW[newTab];
    subView = defaultSv || null;
    if (session) {
      const path = defaultSv ? `${newTab}/${defaultSv}` : newTab;
      pushURL(`/sessions/${session.ID}/${path}`);
    }
  }

  function urlDetailHref(url) {
    if (!session) return '#';
    return `/sessions/${session.ID}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(urlDetailHref(url));
  }

  function openHtmlModal(url) {
    htmlModalUrl = url;
    showHtmlModal = true;
  }

  function closeHtmlModal() {
    showHtmlModal = false;
    htmlModalUrl = '';
  }
</script>

{#if tab === 'url-detail' && session}
  <div class="breadcrumb">
    <a href="/" onclick={(e) => { e.preventDefault(); ongohome?.(); }}>{t('session.sessions')}</a>
    <span>/</span>
    <a href={`/sessions/${session.ID}/reports`} onclick={(e) => { e.preventDefault(); onnavigate?.(`/sessions/${session.ID}/reports`); }}>{session.SeedURLs?.[0] || session.ID}</a>
    <span>/</span>
    <span class="breadcrumb-active">{t('session.urlDetail')}</span>
  </div>
  {#key detailUrl}
    <UrlDetailView sessionId={session.ID} url={detailUrl}
      onerror={(msg) => onerror?.(msg)} onnavigate={(url) => onnavigate?.(url)}
      onopenhtml={openHtmlModal} />
  {/key}

{:else if session}
  <div class="breadcrumb">
    <a href="/" onclick={(e) => { e.preventDefault(); ongohome?.(); }}>{t('session.sessions')}</a>
    <span>/</span>
    <span class="breadcrumb-active">{session.SeedURLs?.[0] || session.ID}</span>
  </div>

  <SessionActionBar {session} {stats} {liveProgress}
    onerror={(msg) => onerror?.(msg)} onstop={(id) => onstop?.(id)} onresume={(id) => onresume?.(id)}
    ondelete={(id) => ondelete?.(id)} onrefresh={() => onrefresh?.()}
    oncompare={(id) => oncompare?.(id)} />

  {#if stats}
    {@const non200 = stats.status_codes ? Object.entries(stats.status_codes).filter(([k]) => k !== '200').reduce((a, [, v]) => a + v, 0) : stats.error_count}
    {@const maxDepth = stats.depth_distribution ? Math.max(...Object.keys(stats.depth_distribution).map(Number)) : 0}
    <div class="stats-grid">
      <div class="stat-card"><div class="stat-value">{fmtN(stats.total_pages)}</div><div class="stat-label">{t('common.pages')}</div></div>
      <div class="stat-card"><div class="stat-value" style={non200 > 0 ? 'color: var(--error)' : ''}>{fmtN(non200)}</div><div class="stat-label">{t('session.non200')}</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(stats.internal_links)}</div><div class="stat-label">{t('session.internalLinks')}</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(stats.external_links)}</div><div class="stat-label">{t('session.externalLinks')}</div></div>
      <div class="stat-card"><div class="stat-value">{maxDepth}</div><div class="stat-label">{t('session.maxDepth')}</div></div>
      <div class="stat-card"><div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">{t('session.avgResponse')}</div></div>
    </div>
    {#if stats.status_codes && Object.keys(stats.status_codes).length > 1}
      <div class="stats-mini mt-sm">
        {#each Object.entries(stats.status_codes).sort((a, b) => Number(a[0]) - Number(b[0])) as [code, count]}
          <span class="stats-mini-item"><span class="badge {statusBadge(Number(code))}">{code}</span> {fmtN(count)}</span>
        {/each}
      </div>
    {/if}
    <div class="stats-secondary stats-secondary-gap">
      {#if stats.pages_per_second > 0}<span>{stats.pages_per_second.toFixed(1)} pages/sec</span>{/if}
      {#if stats.crawl_duration_sec > 0}<span>{stats.crawl_duration_sec < 60 ? stats.crawl_duration_sec.toFixed(0) + 's' : (stats.crawl_duration_sec / 60).toFixed(1) + 'min'}</span>{/if}
      {#if sessionStorageMap[session.ID]}<span>{fmtSize(sessionStorageMap[session.ID])} {t('stats.storage')}</span>{/if}
    </div>
  {/if}

  <div class="tab-bar">
    {#each getTabs() as tb}
      <button class="tab" class:tab-active={tab === tb.id} onclick={() => switchTab(tb.id)}>
        <svg class="tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">{@html tb.icon}</svg>
        {tb.label}
      </button>
    {/each}
  </div>

  <div class="card card-flush card-tab-body">
    {#if tab === 'reports'}
      <ReportsHub sessionId={session.ID} {stats} initialSubView={subView || 'overview'}
        onnavigate={(url, f) => onnavigate?.(url, f)}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'pages'}
      <PagesExplorer sessionId={session.ID}
        initialSubView={subView || 'all'}
        initialFilters={initialFilters}
        initialOffset={initialOffset}
        onpushurl={(u) => pushURL(u)}
        onnavigate={(url) => onnavigate?.(url)}
        onerror={(msg) => onerror?.(msg)}
        onopenhtml={openHtmlModal} />

    {:else if tab === 'links'}
      <LinksExplorer sessionId={session.ID}
        initialSubView={subView || 'internal'}
        initialFilters={initialFilters}
        initialOffset={initialOffset}
        onpushurl={(u) => pushURL(u)}
        onnavigate={(url) => onnavigate?.(url)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'resources'}
      <ResourceChecksTab sessionId={session.ID} initialSubView={subView || 'summary'} initialFilters={initialFilters}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'pagerank'}
      <PageRankTab sessionId={session.ID} initialSubView={subView || 'top'}
        onnavigate={(url) => goToUrlDetail({preventDefault:()=>{}}, url)}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'directives'}
      <DirectivesTab sessionId={session.ID} initialSubView={subView || 'robots'}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'tests'}
      <CustomTestsTab sessionId={session.ID} onerror={(msg) => onerror?.(msg)} />
    {/if}
  </div>
{/if}

{#if showHtmlModal && session}
  <HtmlModal sessionId={session.ID} url={htmlModalUrl} onclose={closeHtmlModal} onerror={(msg) => onerror?.(msg)} />
{/if}

<style>
  .breadcrumb-active {
    color: var(--text);
  }
  .card-tab-body {
    border-top-left-radius: 0;
    border-top-right-radius: 0;
    border-top: none;
  }
  .stats-secondary-gap {
    margin-top: 10px;
  }
</style>
