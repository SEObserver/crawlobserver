<script>
  import { t } from '../i18n/index.svelte.js';
  import RobotsTab from './RobotsTab.svelte';
  import SitemapsTab from './SitemapsTab.svelte';
  import SitemapCoverageURLs from './SitemapCoverageURLs.svelte';

  let { sessionId, initialSubView = 'robots', onpushurl, onerror } = $props();

  const SUB_VIEWS = [
    { id: 'robots', label: () => t('directives.robots') },
    { id: 'sitemaps', label: () => t('directives.sitemaps') },
    { id: 'sitemap_only', label: () => t('directives.sitemapOnly') },
    { id: 'in_both', label: () => t('directives.inBoth') },
  ];

  let subView = $state(initialSubView);

  function switchSubView(sv) {
    subView = sv;
    onpushurl?.(`/sessions/${sessionId}/directives/${sv}`);
  }
</script>

<div class="directives-tab">
  <div class="pr-subview-bar">
    {#each SUB_VIEWS as sv}
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
  {/if}
</div>

<style>
  .directives-tab {
    padding: 24px;
  }
</style>
