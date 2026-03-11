<script>
  import { getAudit } from '../api.js';
  import { t } from '../i18n/index.svelte.js';
  import OverviewReport from './reports/OverviewReport.svelte';
  import ContentReport from './reports/ContentReport.svelte';
  import TechnicalReport from './reports/TechnicalReport.svelte';
  import LinksReport from './reports/LinksReport.svelte';
  import StructureReport from './reports/StructureReport.svelte';
  import SitemapReport from './reports/SitemapReport.svelte';
  import InternationalReport from './reports/InternationalReport.svelte';

  let {
    sessionId,
    stats,
    isRunning = false,
    statsVersion = 0,
    initialSubView = 'overview',
    onnavigate,
    onpushurl,
    onerror,
  } = $props();

  let subView = $state(initialSubView);
  let auditData = $state(null);
  let auditLoading = $state(false);
  let auditError = $state(null);

  const SUB_VIEW_IDS = [
    'overview',
    'content',
    'technical',
    'links',
    'structure',
    'sitemaps',
    'international',
  ];
  const SUB_VIEW_KEYS = {
    overview: 'reports.overview',
    content: 'reports.content',
    technical: 'reports.technical',
    links: 'reports.links',
    structure: 'reports.structure',
    sitemaps: 'reports.sitemaps',
    international: 'reports.international',
  };

  async function loadAudit() {
    if (auditData || auditLoading) return;
    auditLoading = true;
    auditError = null;
    try {
      auditData = await getAudit(sessionId);
    } catch (e) {
      auditError = e.message;
      onerror?.(e.message);
    } finally {
      auditLoading = false;
    }
  }

  function switchSubView(id) {
    subView = id;
    onpushurl?.(`/sessions/${sessionId}/reports/${id}`);
    if (id !== 'overview' && !auditData) {
      loadAudit();
    }
  }

  // Auto-load audit if initial sub-view requires it
  if (initialSubView !== 'overview') {
    loadAudit();
  }
</script>

<div class="pr-container">
  <div class="pr-subview-bar">
    {#each SUB_VIEW_IDS as id}
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === id}
        onclick={() => switchSubView(id)}
      >
        {t(SUB_VIEW_KEYS[id])}
      </button>
    {/each}
  </div>

  {#if subView === 'overview'}
    <OverviewReport {stats} {sessionId} {isRunning} {onnavigate} {statsVersion} />
  {:else if auditLoading}
    <p class="reports-msg-muted">{t('reports.loadingAudit')}</p>
  {:else if auditError && !auditData}
    <p class="reports-msg-error">{t('reports.loadFailed', { error: auditError })}</p>
  {:else if !auditData}
    <p class="reports-msg-muted">{t('reports.requiresAudit')}</p>
  {:else if subView === 'content'}
    <ContentReport {stats} audit={auditData} {sessionId} {onnavigate} />
  {:else if subView === 'technical'}
    <TechnicalReport {stats} audit={auditData} {sessionId} {isRunning} {onnavigate} {statsVersion} />
  {:else if subView === 'links'}
    <LinksReport {stats} audit={auditData} {sessionId} {onnavigate} />
  {:else if subView === 'structure'}
    <StructureReport {stats} audit={auditData} {sessionId} {onnavigate} />
  {:else if subView === 'sitemaps'}
    <SitemapReport {stats} audit={auditData} {sessionId} {onnavigate} />
  {:else if subView === 'international'}
    <InternationalReport {stats} audit={auditData} {sessionId} {onnavigate} />
  {/if}
</div>

<style>
  .reports-msg-muted {
    color: var(--text-muted);
    padding: 32px 0;
  }
  .reports-msg-error {
    color: var(--error);
    padding: 32px 0;
  }
</style>
