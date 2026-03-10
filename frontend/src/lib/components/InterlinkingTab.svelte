<script>
  import { t } from '../i18n/index.svelte.js';
  import {
    computeInterlinking,
    getInterlinkingOpportunities,
    simulateInterlinking,
    importVirtualLinks,
    getInterlinkingSimulations,
    getInterlinkingSimulation,
  } from '../api.js';

  let { sessionId, onerror } = $props();

  const PAGE_SIZE = 100;

  // State
  let view = $state('opportunities'); // 'opportunities' | 'simulation'
  let loading = $state(false);
  let computing = $state(false);

  // Opportunities state
  let opportunities = $state([]);
  let oppTotal = $state(0);
  let oppOffset = $state(0);
  let oppSort = $state('similarity');
  let oppOrder = $state('desc');
  let oppFilters = $state({});
  let selected = $state(new Set());

  // Simulation state
  let simulations = $state([]);
  let currentSim = $state(null);
  let simResults = $state([]);
  let simTotal = $state(0);
  let simOffset = $state(0);
  let simSort = $state('pagerank_diff');
  let simOrder = $state('desc');

  // Import modal
  let showImport = $state(false);
  let importText = $state('');

  async function loadOpportunities() {
    loading = true;
    try {
      const res = await getInterlinkingOpportunities(
        sessionId, PAGE_SIZE, oppOffset, oppSort, oppOrder, oppFilters
      );
      opportunities = res?.opportunities ?? [];
      oppTotal = res?.total ?? 0;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function handleCompute() {
    computing = true;
    try {
      await computeInterlinking(sessionId, { method: 'tfidf' });
      // Poll after a brief delay
      setTimeout(loadOpportunities, 2000);
    } catch (e) {
      onerror?.(e.message);
    } finally {
      computing = false;
    }
  }

  async function handleSimulate() {
    if (selected.size === 0) return;
    const links = opportunities
      .filter((_, i) => selected.has(i))
      .map((o) => ({ source: o.source_url, target: o.target_url }));
    try {
      const res = await simulateInterlinking(sessionId, links);
      if (res?.simulation_id) {
        view = 'simulation';
        setTimeout(() => loadSimulations(), 2000);
      }
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function handleImport() {
    const lines = importText.trim().split('\n').filter(Boolean);
    const links = lines.map((line) => {
      const [source, target] = line.split(/[\t,]/).map((s) => s.trim());
      return { source, target };
    }).filter((l) => l.source && l.target);
    if (links.length === 0) return;
    try {
      await importVirtualLinks(sessionId, links);
      showImport = false;
      importText = '';
      view = 'simulation';
      setTimeout(() => loadSimulations(), 2000);
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function loadSimulations() {
    try {
      const res = await getInterlinkingSimulations(sessionId);
      simulations = res?.simulations ?? [];
      if (simulations.length > 0 && !currentSim) {
        await loadSimResult(simulations[0].id);
      }
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function loadSimResult(simId) {
    try {
      const res = await getInterlinkingSimulation(
        sessionId, simId, PAGE_SIZE, simOffset, simSort, simOrder
      );
      currentSim = res?.simulation ?? null;
      simResults = res?.results ?? [];
      simTotal = res?.total ?? 0;
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function toggleSelect(idx) {
    const next = new Set(selected);
    if (next.has(idx)) next.delete(idx);
    else next.add(idx);
    selected = next;
  }

  function toggleSelectAll() {
    if (selected.size === opportunities.length) {
      selected = new Set();
    } else {
      selected = new Set(opportunities.map((_, i) => i));
    }
  }

  function handleOppSort(col) {
    if (oppSort === col) {
      oppOrder = oppOrder === 'desc' ? 'asc' : oppOrder === 'asc' ? '' : 'desc';
    } else {
      oppSort = col;
      oppOrder = 'desc';
    }
    if (!oppOrder) { oppSort = ''; }
    oppOffset = 0;
    loadOpportunities();
  }

  function handleSimSort(col) {
    if (simSort === col) {
      simOrder = simOrder === 'desc' ? 'asc' : simOrder === 'asc' ? '' : 'desc';
    } else {
      simSort = col;
      simOrder = 'desc';
    }
    if (!simOrder) { simSort = ''; }
    simOffset = 0;
    if (currentSim) loadSimResult(currentSim.id);
  }

  function handleOppFilterKey(e, key) {
    if (e.key === 'Enter') {
      oppFilters = { ...oppFilters, [key]: e.target.value };
      oppOffset = 0;
      loadOpportunities();
    }
  }

  function simBadge(diff) {
    if (diff > 0.01) return 'badge-green';
    if (diff < -0.01) return 'badge-red';
    return 'badge-neutral';
  }

  function simBadgeVal(val) {
    if (val >= 0.7) return 'badge-green';
    if (val >= 0.5) return 'badge-yellow';
    return 'badge-red';
  }

  function sortIcon(col, activeSort, activeOrder) {
    if (activeSort !== col) return '';
    return activeOrder === 'desc' ? ' ▼' : ' ▲';
  }

  $effect(() => {
    if (sessionId) loadOpportunities();
  });
</script>

<div class="interlink-tab">
  <div class="interlink-header">
    <div class="interlink-views">
      <button class:active={view === 'opportunities'} onclick={() => { view = 'opportunities'; loadOpportunities(); }}>
        {t('interlinking.opportunities')}
      </button>
      <button class:active={view === 'simulation'} onclick={() => { view = 'simulation'; loadSimulations(); }}>
        {t('interlinking.simulations')}
      </button>
    </div>
    <div class="interlink-actions">
      <button class="btn-secondary" onclick={() => showImport = true}>
        {t('interlinking.import')}
      </button>
      <button class="btn-primary" onclick={handleCompute} disabled={computing}>
        {computing ? t('common.loading') : t('interlinking.analyze')}
      </button>
    </div>
  </div>

  {#if view === 'opportunities'}
    {#if loading}
      <p class="loading-msg">{t('common.loading')}</p>
    {:else if opportunities.length === 0}
      <div class="empty-state">
        <p>{t('interlinking.noOpportunities')}</p>
        <p class="muted">{t('interlinking.noOpportunitiesHint')}</p>
      </div>
    {:else}
      <div class="interlink-toolbar">
        <span class="count">{t('common.showing')} {oppOffset + 1}-{Math.min(oppOffset + PAGE_SIZE, oppTotal)} / {oppTotal}</span>
        {#if selected.size > 0}
          <button class="btn-primary" onclick={handleSimulate}>
            {t('interlinking.simulate')} ({selected.size})
          </button>
        {/if}
      </div>
      <table>
        <thead>
          <tr>
            <th class="col-check"><input type="checkbox" checked={selected.size === opportunities.length} onchange={toggleSelectAll} /></th>
            <th class="sortable" onclick={() => handleOppSort('source_url')}>{t('interlinking.source')}{sortIcon('source_url', oppSort, oppOrder)}</th>
            <th class="sortable" onclick={() => handleOppSort('target_url')}>{t('interlinking.target')}{sortIcon('target_url', oppSort, oppOrder)}</th>
            <th class="sortable col-num" onclick={() => handleOppSort('similarity')}>{t('interlinking.similarity')}{sortIcon('similarity', oppSort, oppOrder)}</th>
            <th class="sortable col-num" onclick={() => handleOppSort('source_pagerank')}>PR Source{sortIcon('source_pagerank', oppSort, oppOrder)}</th>
            <th class="sortable col-num" onclick={() => handleOppSort('target_pagerank')}>PR Target{sortIcon('target_pagerank', oppSort, oppOrder)}</th>
          </tr>
          <tr class="filter-row">
            <td></td>
            <td><input class="filter-input" placeholder="source_url" onkeydown={(e) => handleOppFilterKey(e, 'source_url')} /></td>
            <td><input class="filter-input" placeholder="target_url" onkeydown={(e) => handleOppFilterKey(e, 'target_url')} /></td>
            <td><input class="filter-input" placeholder=">=0.5" onkeydown={(e) => handleOppFilterKey(e, 'similarity')} /></td>
            <td><input class="filter-input" placeholder=">=10" onkeydown={(e) => handleOppFilterKey(e, 'source_pagerank')} /></td>
            <td><input class="filter-input" placeholder=">=10" onkeydown={(e) => handleOppFilterKey(e, 'target_pagerank')} /></td>
          </tr>
        </thead>
        <tbody>
          {#each opportunities as opp, idx}
            <tr>
              <td class="col-check"><input type="checkbox" checked={selected.has(idx)} onchange={() => toggleSelect(idx)} /></td>
              <td class="cell-url" title={opp.source_url}>
                <span class="url-text">{opp.source_url}</span>
                {#if opp.source_title}<br/><span class="cell-title">{opp.source_title}</span>{/if}
              </td>
              <td class="cell-url" title={opp.target_url}>
                <span class="url-text">{opp.target_url}</span>
                {#if opp.target_title}<br/><span class="cell-title">{opp.target_title}</span>{/if}
              </td>
              <td class="col-num"><span class="badge {simBadgeVal(opp.similarity)}">{(opp.similarity * 100).toFixed(1)}%</span></td>
              <td class="col-num">{opp.source_pagerank?.toFixed(2) ?? '-'}</td>
              <td class="col-num">{opp.target_pagerank?.toFixed(2) ?? '-'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      <div class="pagination">
        <button disabled={oppOffset === 0} onclick={() => { oppOffset = Math.max(0, oppOffset - PAGE_SIZE); loadOpportunities(); }}>{t('common.previous')}</button>
        <button disabled={oppOffset + PAGE_SIZE >= oppTotal} onclick={() => { oppOffset += PAGE_SIZE; loadOpportunities(); }}>{t('common.next')}</button>
      </div>
    {/if}

  {:else}
    <!-- Simulation view -->
    {#if simulations.length === 0 && !currentSim}
      <div class="empty-state">
        <p>{t('interlinking.noSimulations')}</p>
      </div>
    {:else}
      {#if simulations.length > 1}
        <div class="sim-selector">
          <label>{t('interlinking.simHistory')}:</label>
          <select onchange={(e) => loadSimResult(e.target.value)}>
            {#each simulations as sim}
              <option value={sim.id} selected={currentSim?.id === sim.id}>
                {new Date(sim.computed_at).toLocaleString()} — {sim.virtual_links_count} links
              </option>
            {/each}
          </select>
        </div>
      {/if}

      {#if currentSim}
        <div class="sim-summary">
          <div class="sim-stat">
            <span class="sim-label">{t('interlinking.virtualLinks')}</span>
            <span class="sim-value">{currentSim.virtual_links_count}</span>
          </div>
          <div class="sim-stat">
            <span class="sim-label">{t('interlinking.pagesImproved')}</span>
            <span class="sim-value badge-green">{currentSim.pages_improved}</span>
          </div>
          <div class="sim-stat">
            <span class="sim-label">{t('interlinking.pagesDeclined')}</span>
            <span class="sim-value badge-red">{currentSim.pages_declined}</span>
          </div>
          <div class="sim-stat">
            <span class="sim-label">{t('interlinking.avgDiff')}</span>
            <span class="sim-value">{currentSim.avg_diff?.toFixed(4)}</span>
          </div>
          <div class="sim-stat">
            <span class="sim-label">{t('interlinking.maxDiff')}</span>
            <span class="sim-value">{currentSim.max_diff?.toFixed(4)}</span>
          </div>
        </div>

        <table>
          <thead>
            <tr>
              <th class="sortable" onclick={() => handleSimSort('url')}>URL{sortIcon('url', simSort, simOrder)}</th>
              <th class="sortable col-num" onclick={() => handleSimSort('pagerank_before')}>PR {t('interlinking.before')}{sortIcon('pagerank_before', simSort, simOrder)}</th>
              <th class="sortable col-num" onclick={() => handleSimSort('pagerank_after')}>PR {t('interlinking.after')}{sortIcon('pagerank_after', simSort, simOrder)}</th>
              <th class="sortable col-num" onclick={() => handleSimSort('pagerank_diff')}>Diff{sortIcon('pagerank_diff', simSort, simOrder)}</th>
            </tr>
          </thead>
          <tbody>
            {#each simResults as row}
              <tr>
                <td class="cell-url" title={row.url}>{row.url}</td>
                <td class="col-num">{row.pagerank_before?.toFixed(2)}</td>
                <td class="col-num">{row.pagerank_after?.toFixed(2)}</td>
                <td class="col-num"><span class="badge {simBadge(row.pagerank_diff)}">{row.pagerank_diff > 0 ? '+' : ''}{row.pagerank_diff?.toFixed(4)}</span></td>
              </tr>
            {/each}
          </tbody>
        </table>
        <div class="pagination">
          <span>{t('common.showing')} {simOffset + 1}-{Math.min(simOffset + PAGE_SIZE, simTotal)} / {simTotal}</span>
          <button disabled={simOffset === 0} onclick={() => { simOffset = Math.max(0, simOffset - PAGE_SIZE); loadSimResult(currentSim.id); }}>{t('common.previous')}</button>
          <button disabled={simOffset + PAGE_SIZE >= simTotal} onclick={() => { simOffset += PAGE_SIZE; loadSimResult(currentSim.id); }}>{t('common.next')}</button>
        </div>
      {/if}
    {/if}
  {/if}

  {#if showImport}
    <div class="modal-overlay" onclick={() => showImport = false}>
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="modal-content" onclick={(e) => e.stopPropagation()}>
        <h3>{t('interlinking.importTitle')}</h3>
        <p class="muted">{t('interlinking.importHint')}</p>
        <textarea bind:value={importText} rows="10" placeholder="https://example.com/page1,https://example.com/page2"></textarea>
        <div class="modal-actions">
          <button class="btn-secondary" onclick={() => showImport = false}>{t('common.cancel')}</button>
          <button class="btn-primary" onclick={handleImport}>{t('interlinking.simulate')}</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .interlink-tab { padding: 0; }
  .interlink-header {
    display: flex; justify-content: space-between; align-items: center;
    padding: 12px 0; border-bottom: 1px solid var(--border); margin-bottom: 12px;
  }
  .interlink-views { display: flex; gap: 4px; }
  .interlink-views button {
    padding: 6px 14px; border: 1px solid var(--border); background: var(--bg);
    border-radius: 6px; cursor: pointer; font-size: 13px; color: var(--text);
  }
  .interlink-views button.active { background: var(--accent); color: #fff; border-color: var(--accent); }
  .interlink-actions { display: flex; gap: 8px; }
  .interlink-toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
  .count { font-size: 13px; color: var(--text-muted); }

  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { padding: 6px 8px; text-align: left; border-bottom: 1px solid var(--border); }
  th { font-weight: 600; white-space: nowrap; }
  th.sortable { cursor: pointer; user-select: none; }
  th.sortable:hover { color: var(--accent); }
  tbody tr:nth-child(even) { background: var(--bg-secondary); }
  .col-check { width: 32px; text-align: center; }
  .col-num { text-align: right; white-space: nowrap; }
  .cell-url { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cell-title { font-size: 11px; color: var(--text-muted); }
  .url-text { font-family: monospace; font-size: 12px; }

  .filter-row td { padding: 2px 4px; }
  .filter-input { width: 100%; padding: 3px 6px; border: 1px solid var(--border); border-radius: 4px; font-size: 12px; background: var(--bg); color: var(--text); }

  .badge { padding: 2px 6px; border-radius: 4px; font-size: 12px; font-weight: 500; }
  .badge-green { background: #dcfce7; color: #166534; }
  .badge-yellow { background: #fef3c7; color: #92400e; }
  .badge-red { background: #fecaca; color: #991b1b; }
  .badge-neutral { background: var(--bg-secondary); color: var(--text-muted); }

  .pagination { display: flex; gap: 8px; align-items: center; justify-content: flex-end; padding: 8px 0; }
  .pagination button { padding: 4px 12px; }

  .empty-state { text-align: center; padding: 40px 20px; color: var(--text-muted); }
  .loading-msg { text-align: center; padding: 20px; color: var(--text-muted); }
  .muted { color: var(--text-muted); font-size: 13px; }

  .sim-summary {
    display: flex; gap: 16px; flex-wrap: wrap;
    padding: 12px 0; border-bottom: 1px solid var(--border); margin-bottom: 12px;
  }
  .sim-stat { display: flex; flex-direction: column; gap: 2px; }
  .sim-label { font-size: 11px; color: var(--text-muted); text-transform: uppercase; }
  .sim-value { font-size: 18px; font-weight: 600; }
  .sim-selector { margin-bottom: 12px; }
  .sim-selector label { font-size: 13px; margin-right: 8px; }
  .sim-selector select { padding: 4px 8px; border-radius: 4px; border: 1px solid var(--border); }

  .btn-primary {
    padding: 6px 14px; background: var(--accent); color: #fff;
    border: none; border-radius: 6px; cursor: pointer; font-size: 13px;
  }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-secondary {
    padding: 6px 14px; background: var(--bg); color: var(--text);
    border: 1px solid var(--border); border-radius: 6px; cursor: pointer; font-size: 13px;
  }

  .modal-overlay {
    position: fixed; inset: 0; background: rgba(0,0,0,0.5);
    display: flex; align-items: center; justify-content: center; z-index: 100;
  }
  .modal-content {
    background: var(--bg); border-radius: 12px; padding: 24px;
    max-width: 600px; width: 90%;
  }
  .modal-content h3 { margin: 0 0 8px; }
  .modal-content textarea {
    width: 100%; padding: 8px; border: 1px solid var(--border); border-radius: 6px;
    font-family: monospace; font-size: 12px; margin: 12px 0; resize: vertical;
    background: var(--bg); color: var(--text);
  }
  .modal-actions { display: flex; gap: 8px; justify-content: flex-end; }
</style>
