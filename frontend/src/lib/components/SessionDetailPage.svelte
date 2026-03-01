<script>
  import { t } from '../i18n/index.svelte.js';
  import { getTabs, TAB_DEFAULT_SUB_VIEW } from '../tabColumns.js';
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
  import AuthorityTab from './AuthorityTab.svelte';

  let {
    session, stats, liveProgress,
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
        onerror={(msg) => onerror?.(msg)}
        onrefresh={() => onrefresh?.()} />

    {:else if tab === 'directives'}
      <DirectivesTab sessionId={session.ID} initialSubView={subView || 'robots'}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'authority'}
      <AuthorityTab sessionId={session.ID} projectId={session.ProjectID}
        onerror={(msg) => onerror?.(msg)}
        onnavigate={() => {
          if (session.ProjectID) {
            onnavigate?.(`/projects/${session.ProjectID}/providers`);
          }
        }} />

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
</style>
