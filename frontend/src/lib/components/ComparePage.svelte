<script>
  import { getCompareStats, getComparePages, getCompareLinks } from '../api.js';
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import CompareStatsView from './CompareStatsView.svelte';
  import ComparePagesView from './ComparePagesView.svelte';
  import CompareLinksView from './CompareLinksView.svelte';

  let { sessions = [], initialA = '', initialB = '', onerror, onnavigate } = $props();

  let sessionA = $state(initialA);
  let sessionB = $state(initialB);
  let activeTab = $state('stats');
  let loading = $state(false);

  let compareStats = $state(null);
  let pagesDiffType = $state('changed');
  let pagesResult = $state(null);
  let pagesOffset = $state(0);
  let linksDiffType = $state('added');
  let linksResult = $state(null);
  let linksOffset = $state(0);
  const PAGE_SIZE = 100;

  function sessionLabel(s) {
    try {
      const host = new URL(s.SeedURLs?.[0] || 'https://unknown').hostname;
      const date = new Date(s.StartedAt).toLocaleDateString();
      return `${host} - ${date} (${t('sessions.pagesCount', { count: fmtN(s.PagesCrawled) })})`;
    } catch { return s.ID.slice(0, 8); }
  }

  async function doCompare() {
    if (!sessionA || !sessionB) return;
    loading = true;
    compareStats = null; pagesResult = null; linksResult = null;
    pagesOffset = 0; linksOffset = 0;
    try {
      compareStats = await getCompareStats(sessionA, sessionB);
      if (onnavigate) history.replaceState(null, '', `/compare?a=${sessionA}&b=${sessionB}`);
    } catch (e) { onerror?.(e.message); }
    finally { loading = false; }
  }

  async function loadPages() {
    if (!sessionA || !sessionB) return;
    loading = true;
    try { pagesResult = await getComparePages(sessionA, sessionB, pagesDiffType, PAGE_SIZE, pagesOffset); }
    catch (e) { onerror?.(e.message); }
    finally { loading = false; }
  }

  async function loadLinks() {
    if (!sessionA || !sessionB) return;
    loading = true;
    try { linksResult = await getCompareLinks(sessionA, sessionB, linksDiffType, PAGE_SIZE, linksOffset); }
    catch (e) { onerror?.(e.message); }
    finally { loading = false; }
  }

  function switchMainTab(tab) {
    activeTab = tab;
    if (tab === 'pages' && !pagesResult) loadPages();
    if (tab === 'links' && !linksResult) loadLinks();
  }

  $effect(() => {
    if (initialA && initialB && !compareStats) {
      sessionA = initialA; sessionB = initialB;
      doCompare();
    }
  });
</script>

<div class="compare-page">
  <h2>{t('compare.title')}</h2>

  <div class="compare-selectors">
    <div class="selector-group">
      <label>{t('compare.sessionA')}</label>
      <select bind:value={sessionA}>
        <option value="">{t('compare.selectSession')}</option>
        {#each sessions as s}<option value={s.ID}>{sessionLabel(s)}</option>{/each}
      </select>
    </div>
    <div class="selector-swap">
      <button class="btn btn-sm" title={t('compare.swap')} onclick={() => { const tmp = sessionA; sessionA = sessionB; sessionB = tmp; compareStats = null; pagesResult = null; linksResult = null; }}>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/></svg>
      </button>
    </div>
    <div class="selector-group">
      <label>{t('compare.sessionB')}</label>
      <select bind:value={sessionB}>
        <option value="">{t('compare.selectSession')}</option>
        {#each sessions as s}<option value={s.ID}>{sessionLabel(s)}</option>{/each}
      </select>
    </div>
    <button class="btn btn-primary" onclick={doCompare} disabled={!sessionA || !sessionB || loading}>
      {loading ? t('compare.comparing') : t('sidebar.compare')}
    </button>
  </div>

  {#if compareStats}
    <div class="tab-bar compare-tab-bar">
      <button class="tab" class:tab-active={activeTab === 'stats'} onclick={() => switchMainTab('stats')}>{t('compare.stats')}</button>
      <button class="tab" class:tab-active={activeTab === 'pages'} onclick={() => switchMainTab('pages')}>{t('common.pages')}</button>
      <button class="tab" class:tab-active={activeTab === 'links'} onclick={() => switchMainTab('links')}>{t('compare.internalLinks')}</button>
    </div>

    <div class="card card-flush compare-card-flush">
      {#if activeTab === 'stats'}
        <CompareStatsView {compareStats} />
      {:else if activeTab === 'pages'}
        <ComparePagesView {loading} {pagesResult} diffType={pagesDiffType}
          onswitchtype={(type) => { pagesDiffType = type; pagesOffset = 0; loadPages(); }}
          onpagechange={(o) => { pagesOffset = o; loadPages(); }} />
      {:else if activeTab === 'links'}
        <CompareLinksView {loading} {linksResult} diffType={linksDiffType}
          onswitchtype={(type) => { linksDiffType = type; linksOffset = 0; loadLinks(); }}
          onpagechange={(o) => { linksOffset = o; loadLinks(); }} />
      {/if}
    </div>
  {/if}
</div>

<style>
  .compare-page h2 { margin: 0 0 20px; font-size: 20px; font-weight: 600; }
  .compare-selectors { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
  .selector-group { flex: 1; min-width: 200px; }
  .selector-group label { display: block; font-size: 12px; font-weight: 500; color: var(--text-secondary); margin-bottom: 4px; }
  .selector-group select { width: 100%; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); font-size: 13px; }
  .selector-swap { display: flex; align-items: center; padding-bottom: 2px; }
  .compare-tab-bar { margin-top: 24px; }
  .compare-card-flush { border-top-left-radius: 0; border-top-right-radius: 0; border-top: none; }
</style>
