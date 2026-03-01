<script>
  import { onMount } from 'svelte';
  import { getInternalLinks, getExternalLinks } from '../api.js';
  import { fmtN, trunc } from '../utils.js';
  import { PAGE_SIZE, TAB_FILTERS } from '../tabColumns.js';
  import { t } from '../i18n/index.svelte.js';
  import DataTable from './DataTable.svelte';
  import ExternalChecksTab from './ExternalChecksTab.svelte';

  let {
    sessionId,
    initialSubView = 'internal',
    initialFilters = {},
    initialOffset = 0,
    onpushurl,
    onnavigate,
    onerror,
  } = $props();

  const SUB_VIEWS = [
    { id: 'internal', label: () => t('links.internal') },
    { id: 'external', label: () => t('links.external') },
    { id: 'checks',   label: () => t('links.checks') },
  ];

  let subView = $state(initialSubView);
  let intLinks = $state([]);
  let extLinks = $state([]);
  let intLinksOffset = $state(initialSubView === 'internal' ? initialOffset : 0);
  let extLinksOffset = $state(initialSubView === 'external' ? initialOffset : 0);
  let hasMoreIntLinks = $state(false);
  let hasMoreExtLinks = $state(false);
  let filters = $state({ ...initialFilters });

  function basePath() {
    return `/sessions/${sessionId}/links`;
  }

  function pushFilters(sv, f, offset) {
    const path = `${basePath()}/${sv || subView}`;
    const params = new URLSearchParams();
    const activeFilters = f || filters;
    for (const [k, v] of Object.entries(activeFilters)) {
      if (v !== '' && v != null) params.set(k, v);
    }
    if ((offset || 0) > 0) params.set('offset', String(offset));
    const qs = params.toString();
    onpushurl?.(qs ? `${path}?${qs}` : path);
  }

  async function loadData() {
    try {
      if (subView === 'internal') {
        const result = await getInternalLinks(sessionId, PAGE_SIZE, intLinksOffset, filters);
        intLinks = result || [];
        hasMoreIntLinks = intLinks.length === PAGE_SIZE;
      } else if (subView === 'external') {
        const result = await getExternalLinks(sessionId, PAGE_SIZE, extLinksOffset, filters);
        extLinks = result || [];
        hasMoreExtLinks = extLinks.length === PAGE_SIZE;
      }
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function switchSubView(sv) {
    subView = sv;
    filters = {};
    intLinksOffset = 0;
    extLinksOffset = 0;
    if (sv !== 'checks') {
      pushFilters(sv, {}, 0);
      loadData();
    } else {
      pushFilters(sv, {}, 0);
    }
  }

  function currentOffset() {
    return subView === 'internal' ? intLinksOffset : extLinksOffset;
  }

  async function nextPage() {
    if (subView === 'internal') intLinksOffset += PAGE_SIZE;
    else extLinksOffset += PAGE_SIZE;
    pushFilters(null, null, currentOffset());
    await loadData();
  }

  async function prevPage() {
    if (subView === 'internal') intLinksOffset = Math.max(0, intLinksOffset - PAGE_SIZE);
    else extLinksOffset = Math.max(0, extLinksOffset - PAGE_SIZE);
    pushFilters(null, null, currentOffset());
    await loadData();
  }

  function applyFilters() {
    intLinksOffset = 0;
    extLinksOffset = 0;
    pushFilters();
    loadData();
  }

  function clearFilters() {
    filters = {};
    intLinksOffset = 0;
    extLinksOffset = 0;
    pushFilters(null, {}, 0);
    loadData();
  }

  function setFilter(key, val) {
    filters[key] = val;
    filters = { ...filters };
  }

  function hasActiveFilters() {
    return Object.values(filters).some(v => v && v !== '');
  }

  function urlDetailHref(url) {
    return `/sessions/${sessionId}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(urlDetailHref(url));
  }

  function handleChecksViewSources(extUrl) {
    // Navigate to links/external sub-view with target_url filter
    subView = 'external';
    filters = { target_url: extUrl };
    extLinksOffset = 0;
    pushFilters('external', filters, 0);
    loadData();
  }

  onMount(() => {
    if (subView !== 'checks') loadData();
  });
</script>

<div class="links-explorer">
  <div class="pr-subview-bar">
    {#each SUB_VIEWS as sv}
      <button class="pr-subview-btn" class:pr-subview-active={subView === sv.id}
        onclick={() => switchSubView(sv.id)}>{sv.label()}</button>
    {/each}
  </div>

  {#if subView === 'internal'}
    <DataTable columns={[{label:t('common.source')},{label:t('common.target')},{label:t('session.anchorText')},{label:t('session.tag')}]}
      filterKeys={TAB_FILTERS.internal} {filters} data={intLinks} offset={intLinksOffset} pageSize={PAGE_SIZE}
      hasMore={hasMoreIntLinks} hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
      {#snippet row(l)}
        <tr>
          <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
          <td class="cell-url"><a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a></td>
          <td class="cell-title">{l.AnchorText || '-'}</td>
          <td>{l.Tag}</td>
        </tr>
      {/snippet}
    </DataTable>

  {:else if subView === 'external'}
    <DataTable columns={[{label:t('common.source')},{label:t('common.target')},{label:t('session.anchorText')},{label:t('session.rel')}]}
      filterKeys={TAB_FILTERS.external} {filters} data={extLinks} offset={extLinksOffset} pageSize={PAGE_SIZE}
      hasMore={hasMoreExtLinks} hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
      {#snippet row(l)}
        <tr>
          <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
          <td class="cell-url"><a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a></td>
          <td class="cell-title">{l.AnchorText || '-'}</td>
          <td>{l.Rel || '-'}</td>
        </tr>
      {/snippet}
    </DataTable>

  {:else if subView === 'checks'}
    <ExternalChecksTab {sessionId}
      basePath={`/sessions/${sessionId}/links/checks`}
      onpushurl={(u) => onpushurl?.(u)}
      onnavigate={(tab, f) => handleChecksViewSources(f?.target_url || '')}
      onerror={(msg) => onerror?.(msg)} />
  {/if}
</div>

<style>
  .links-explorer { padding: 16px; }
</style>
