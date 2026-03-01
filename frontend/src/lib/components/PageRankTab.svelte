<script>
  import { getPageRankTop, getPageRankTreemap, getPageRankDistribution, computePageRank } from '../api.js';
  import { t } from '../i18n/index.svelte.js';
  import PageRankTopView from './PageRankTopView.svelte';
  import PageRankTreemap from './PageRankTreemap.svelte';
  import PageRankDistribution from './PageRankDistribution.svelte';
  import PageRankTableView from './PageRankTableView.svelte';
  import PageRankTooltip from './PageRankTooltip.svelte';

  let { sessionId, initialSubView = 'top', onnavigate, onpushurl, onerror, onrefresh } = $props();

  let prSubView = $state(initialSubView);
  let prLoading = $state(false);
  let prTopData = $state(null);
  let prTopLimit = $state(50);
  let prTopOffset = $state(0);
  let prDistData = $state(null);
  let prTreemapData = $state(null);
  let prTreemapDepth = $state(2);
  let prTreemapMinPages = $state(1);
  let prTableData = $state(null);
  let prTableOffset = $state(0);
  let prTableDir = $state('');
  let prTooltip = $state(null);
  let hasData = $state(null); // null = unknown, true/false after first load
  let computingPR = $state(false);

  async function loadPRSubView(view) {
    prLoading = true;
    try {
      if (view === 'top') {
        prTopData = await getPageRankTop(sessionId, prTopLimit, prTopOffset);
        if (hasData === null) hasData = (prTopData?.total > 0);
      }
      else if (view === 'directory') prTreemapData = await getPageRankTreemap(sessionId, prTreemapDepth, prTreemapMinPages);
      else if (view === 'distribution') prDistData = await getPageRankDistribution(sessionId, 20);
      else if (view === 'table') prTableData = await getPageRankTop(sessionId, 50, prTableOffset, prTableDir);
    } catch (e) { onerror?.(e.message); }
    finally { prLoading = false; }
  }

  function switchPRSubView(view) {
    prSubView = view;
    if (view === 'top') prTopOffset = 0;
    if (view === 'table') prTableOffset = 0;
    onpushurl?.(`/sessions/${sessionId}/pagerank/${view}`);
    loadPRSubView(view);
  }

  function drillToTable(dir) {
    prTableDir = dir;
    prTableOffset = 0;
    prSubView = 'table';
    onpushurl?.(`/sessions/${sessionId}/pagerank/table`);
    loadPRSubView('table');
  }

  function drillHistToTable() {
    prTableDir = '';
    prTableOffset = 0;
    prSubView = 'table';
    onpushurl?.(`/sessions/${sessionId}/pagerank/table`);
    loadPRSubView('table');
  }

  async function handleComputePageRank() {
    computingPR = true;
    try {
      await computePageRank(sessionId);
      hasData = true;
      loadPRSubView(prSubView);
      onrefresh?.();
    } catch (e) { onerror?.(e.message); }
    finally { computingPR = false; }
  }

  loadPRSubView(prSubView);
</script>

<div class="pr-container">
  <div class="pr-subview-bar">
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'top'} onclick={() => switchPRSubView('top')}>{t('pagerank.topPages')}</button>
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'directory'} onclick={() => switchPRSubView('directory')}>{t('pagerank.byDirectory')}</button>
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'distribution'} onclick={() => switchPRSubView('distribution')}>{t('pagerank.distribution')}</button>
    <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'table'} onclick={() => switchPRSubView('table')}>{t('pagerank.fullTable')}</button>
  </div>

  {#if prLoading}
    <p class="loading-msg">{t('common.loading')}</p>
  {:else if hasData === false}
    <div class="pr-empty-state">
      <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="pr-empty-icon">
        <polyline points="23 6 13.5 15.5 8.5 10.5 1 18"/><polyline points="17 6 23 6 23 12"/>
      </svg>
      <p class="pr-empty-text">{t('pagerank.noData')}</p>
      <button class="btn btn-primary" onclick={handleComputePageRank} disabled={computingPR}>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 6 13.5 15.5 8.5 10.5 1 18"/><polyline points="17 6 23 6 23 12"/></svg>
        {computingPR ? t('actionBar.computing') : t('actionBar.computePageRank')}
      </button>
    </div>
  {:else if prSubView === 'top'}
    <PageRankTopView data={prTopData} offset={prTopOffset} limit={prTopLimit}
      {onnavigate} ontooltip={(t) => prTooltip = t}
      onlimitchange={(l) => { prTopLimit = l; prTopOffset = 0; loadPRSubView('top'); }}
      onpagechange={(o) => { prTopOffset = o; loadPRSubView('top'); }} />
  {:else if prSubView === 'directory'}
    <PageRankTreemap data={prTreemapData} depth={prTreemapDepth} minPages={prTreemapMinPages}
      ontooltip={(t) => prTooltip = t} ondrill={drillToTable}
      ondepthchange={(d) => { prTreemapDepth = d; loadPRSubView('directory'); }}
      onminpageschange={(m) => { prTreemapMinPages = m; loadPRSubView('directory'); }} />
  {:else if prSubView === 'distribution'}
    <PageRankDistribution data={prDistData} ontooltip={(t) => prTooltip = t} ondrill={drillHistToTable} />
  {:else if prSubView === 'table'}
    <PageRankTableView data={prTableData} offset={prTableOffset} dirFilter={prTableDir}
      {onnavigate}
      onfilterchange={(dir, apply) => { prTableDir = dir; if (apply) { prTableOffset = 0; loadPRSubView('table'); } }}
      onpagechange={(o) => { prTableOffset = o; loadPRSubView('table'); }} />
  {/if}
</div>

<PageRankTooltip tooltip={prTooltip} />

<style>
  .pr-empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    padding: 64px 20px;
    text-align: center;
  }
  .pr-empty-icon {
    color: var(--text-muted);
    opacity: 0.4;
  }
  .pr-empty-text {
    color: var(--text-muted);
    font-size: 15px;
    margin: 0;
  }
</style>
