<script>
  import { t } from '../i18n/index.svelte.js';
  import { getSitemaps } from '../api.js';
  import RobotsTab from './RobotsTab.svelte';
  import SitemapsTab from './SitemapsTab.svelte';
  import SitemapCoverageURLs from './SitemapCoverageURLs.svelte';

  let { sessionId, initialSubView = 'robots', onpushurl, onerror } = $props();

  const ALL_SUB_VIEWS = [
    { id: 'robots', label: () => t('directives.robots') },
    { id: 'sitemaps', label: () => t('directives.sitemaps') },
    { id: 'sitemap_only', label: () => t('directives.sitemapOnly') },
    { id: 'in_both', label: () => t('directives.inBoth') },
    { id: 'crawl_only', label: () => t('directives.crawlOnly') },
  ];

  let hasSitemaps = $state(true); // optimistic default
  let subView = $state(initialSubView);

  let visibleSubViews = $derived(
    hasSitemaps ? ALL_SUB_VIEWS : ALL_SUB_VIEWS.filter((sv) => sv.id === 'robots'),
  );

  // Check if sitemaps exist for this session
  getSitemaps(sessionId)
    .then((data) => {
      hasSitemaps = Array.isArray(data) && data.length > 0;
      if (!hasSitemaps && subView !== 'robots') {
        subView = 'robots';
      }
    })
    .catch(() => {
      hasSitemaps = false;
      if (subView !== 'robots') subView = 'robots';
    });

  function switchSubView(sv) {
    subView = sv;
    onpushurl?.(`/sessions/${sessionId}/directives/${sv}`);
  }
</script>

<div class="directives-tab">
  <div class="pr-subview-bar">
    {#each visibleSubViews as sv}
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === sv.id}
        onclick={() => switchSubView(sv.id)}>{sv.label()}</button
      >
    {/each}
  </div>

  {#if subView === 'robots'}
    <RobotsTab {sessionId} onerror={(msg) => onerror?.(msg)} />
  {:else if subView === 'sitemaps'}
    <SitemapsTab {sessionId} onerror={(msg) => onerror?.(msg)} />
  {:else if subView === 'sitemap_only'}
    <SitemapCoverageURLs {sessionId} filter="sitemap_only" onerror={(msg) => onerror?.(msg)} />
  {:else if subView === 'in_both'}
    <SitemapCoverageURLs {sessionId} filter="in_both" onerror={(msg) => onerror?.(msg)} />
  {:else if subView === 'crawl_only'}
    <SitemapCoverageURLs {sessionId} filter="crawl_only" onerror={(msg) => onerror?.(msg)} />
  {/if}
</div>

<style>
  .directives-tab {
    padding: 24px;
  }
</style>
