<script>
  import { getAudit } from '../api.js';
  import OverviewReport from './reports/OverviewReport.svelte';
  import ContentReport from './reports/ContentReport.svelte';
  import TechnicalReport from './reports/TechnicalReport.svelte';
  import LinksReport from './reports/LinksReport.svelte';
  import StructureReport from './reports/StructureReport.svelte';
  import SitemapReport from './reports/SitemapReport.svelte';
  import InternationalReport from './reports/InternationalReport.svelte';

  let { sessionId, stats, initialSubView = 'overview', onnavigate, onpushurl, onerror } = $props();

  let subView = $state(initialSubView);
  let auditData = $state(null);
  let auditLoading = $state(false);
  let auditError = $state(null);

  const SUB_VIEWS = [
    { id: 'overview', label: 'Overview' },
    { id: 'content', label: 'Content' },
    { id: 'technical', label: 'Technical' },
    { id: 'links', label: 'Links' },
    { id: 'structure', label: 'Structure' },
    { id: 'sitemaps', label: 'Sitemaps' },
    { id: 'international', label: 'International' },
  ];

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
</script>

<div class="pr-container">
  <div class="pr-subview-bar">
    {#each SUB_VIEWS as sv}
      <button class="pr-subview-btn" class:pr-subview-active={subView === sv.id}
        onclick={() => switchSubView(sv.id)}>
        {sv.label}
      </button>
    {/each}
  </div>

  {#if subView === 'overview'}
    <OverviewReport {stats} {sessionId} {onnavigate} />
  {:else if auditLoading}
    <p style="color: var(--text-muted); padding: 32px 0;">Loading audit data...</p>
  {:else if auditError && !auditData}
    <p style="color: var(--error); padding: 32px 0;">Failed to load audit: {auditError}</p>
  {:else if !auditData}
    <p style="color: var(--text-muted); padding: 32px 0;">Requires audit computation. Click a sub-view to load.</p>
  {:else if subView === 'content'}
    <ContentReport {stats} audit={auditData} {sessionId} {onnavigate} />
  {:else if subView === 'technical'}
    <TechnicalReport {stats} audit={auditData} {sessionId} {onnavigate} />
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
