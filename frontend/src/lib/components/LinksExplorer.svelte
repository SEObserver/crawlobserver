<script>
  import { onMount } from 'svelte';
  import { getInternalLinks, getBacklinksTop, buildApiPath } from '../api.js';
  import { fmtN, trunc, fetchAll, downloadCSV } from '../utils.js';
  import { PAGE_SIZE, TAB_FILTERS } from '../tabColumns.js';
  import { t } from '../i18n/index.svelte.js';
  import DataTable from './DataTable.svelte';
  import ExternalChecksTab from './ExternalChecksTab.svelte';
  import BacklinksView from './BacklinksView.svelte';
  import InterlinkingTab from './InterlinkingTab.svelte';
  import UrlActions from './UrlActions.svelte';
  import ExportDropdown from './ExportDropdown.svelte';

  let {
    sessionId,
    projectId = null,
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
    { id: 'backlinks', label: () => t('links.backlinks'), premium: true },
    { id: 'interlinking', label: () => t('tabs.interlinking') },
  ];

  let subView = $state(initialSubView);
  let intLinks = $state([]);
  let intLinksOffset = $state(initialSubView === 'internal' ? initialOffset : 0);
  let hasMoreIntLinks = $state(false);
  let filters = $state({ ...initialFilters });
  let sortColumn = $state('');
  let sortOrder = $state('');

  // Backlinks state
  let blData = $state([]);
  let blTotal = $state(0);
  let blOffset = $state(initialSubView === 'backlinks' ? initialOffset : 0);
  let blLimit = $state(100);
  let blSort = $state('trust_flow');
  let blOrder = $state('desc');
  let blFilters = $state({});

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
        const result = await getInternalLinks(
          sessionId,
          PAGE_SIZE,
          intLinksOffset,
          filters,
          sortColumn,
          sortOrder,
        );
        intLinks = result || [];
        hasMoreIntLinks = intLinks.length === PAGE_SIZE;
      } else if (subView === 'backlinks' && projectId) {
        const result = await getBacklinksTop(
          projectId,
          blLimit,
          blOffset,
          blFilters,
          blSort,
          blOrder,
        );
        blData = result?.backlinks || [];
        blTotal = result?.total || 0;
      }
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function switchSubView(sv) {
    subView = sv;
    filters = {};
    intLinksOffset = 0;
    sortColumn = '';
    sortOrder = '';
    if (sv === 'backlinks') {
      blOffset = 0;
      blSort = 'trust_flow';
      blOrder = 'desc';
      blFilters = {};
      pushFilters(sv, {}, 0);
      loadData();
    } else if (sv === 'external' || sv === 'interlinking') {
      // These sub-views manage their own data loading
      pushFilters(sv, {}, 0);
    } else {
      pushFilters(sv, {}, 0);
      loadData();
    }
  }

  function handleSort(col, ord) {
    sortColumn = col;
    sortOrder = ord;
    intLinksOffset = 0;
    loadData();
  }

  async function nextPage() {
    intLinksOffset += PAGE_SIZE;
    pushFilters(null, null, intLinksOffset);
    await loadData();
  }

  async function prevPage() {
    intLinksOffset = Math.max(0, intLinksOffset - PAGE_SIZE);
    pushFilters(null, null, intLinksOffset);
    await loadData();
  }

  function applyFilters() {
    intLinksOffset = 0;
    pushFilters();
    loadData();
  }

  function clearFilters() {
    filters = {};
    intLinksOffset = 0;
    pushFilters(null, {}, 0);
    loadData();
  }

  function setFilter(key, val) {
    filters[key] = val;
    filters = { ...filters };
  }

  function hasActiveFilters() {
    return Object.values(filters).some((v) => v && v !== '');
  }

  let exporting = $state(false);

  async function handleExportCSV() {
    if (exporting) return;
    exporting = true;
    try {
      await exportCSV();
    } finally {
      exporting = false;
    }
  }

  async function exportCSV() {
    if (subView === 'internal') {
      const allData = await fetchAll((limit, offset) =>
        getInternalLinks(sessionId, limit, offset, filters),
      );
      downloadCSV(
        'links-internal.csv',
        ['Source URL', 'Target URL', 'Anchor Text', 'Tag'],
        ['SourceURL', 'TargetURL', 'AnchorText', 'Tag'],
        allData,
      );
    }
  }

  function urlDetailHref(url) {
    return `/sessions/${sessionId}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(urlDetailHref(url));
  }

  let apiPath = $derived(
    buildApiPath(`/sessions/${sessionId}/internal-links`, {
      limit: PAGE_SIZE,
      offset: 0,
      ...filters,
      sort: sortColumn,
      order: sortOrder,
    }),
  );

  onMount(() => {
    if (subView !== 'external') loadData();
  });
</script>

<div class="links-explorer">
  <div class="explorer-toolbar">
    <div class="pr-subview-bar">
      {#each SUB_VIEWS as sv}
        <button
          class="pr-subview-btn"
          class:pr-subview-active={subView === sv.id}
          class:pr-subview-premium={sv.premium}
          onclick={() => switchSubView(sv.id)}
          >{#if sv.premium}<span class="premium-star">&#9733;</span>
          {/if}{sv.label()}</button
        >
      {/each}
    </div>
    {#if subView !== 'external' && subView !== 'backlinks' && subView !== 'interlinking'}
      <ExportDropdown onexportcsv={handleExportCSV} {exporting} {apiPath} />
    {/if}
  </div>

  {#if subView === 'internal'}
    <DataTable
      columns={[
        { label: t('common.source'), sortKey: 'source_url' },
        { label: t('common.target'), sortKey: 'target_url' },
        { label: t('session.anchorText'), sortKey: 'anchor_text' },
        { label: t('session.tag'), sortKey: 'tag' },
      ]}
      filterKeys={TAB_FILTERS.internal}
      {filters}
      data={intLinks}
      offset={intLinksOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMoreIntLinks}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(l)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}
                >{l.SourceURL}</a
              ><UrlActions url={l.SourceURL} /></span
            ></td
          >
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}
                >{l.TargetURL}</a
              ><UrlActions url={l.TargetURL} /></span
            ></td
          >
          <td class="cell-title">{l.AnchorText || '-'}</td>
          <td>{l.Tag}</td>
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'external'}
    <ExternalChecksTab
      {sessionId}
      basePath={`/sessions/${sessionId}/links/external`}
      onpushurl={(u) => onpushurl?.(u)}
      onnavigate={(url) => onnavigate?.(url)}
      onerror={(msg) => onerror?.(msg)}
    />
  {:else if subView === 'backlinks'}
    {#if !projectId}
      <p class="chart-empty">{t('links.backlinksNeedProject')}</p>
    {:else}
      <BacklinksView
        data={blData}
        total={blTotal}
        offset={blOffset}
        limit={blLimit}
        sortColumn={blSort}
        sortOrder={blOrder}
        filters={blFilters}
        {sessionId}
        onnavigate={(url) => onnavigate?.(url)}
        onsort={(col, ord) => {
          blSort = col;
          blOrder = ord;
          blOffset = 0;
          loadData();
        }}
        onpagechange={(o) => {
          blOffset = o;
          pushFilters(null, null, o);
          loadData();
        }}
        onlimitchange={(l) => {
          blLimit = l;
          blOffset = 0;
          loadData();
        }}
        onsetfilter={(k, v) => {
          blFilters = { ...blFilters, [k]: v };
        }}
        onapplyfilters={() => {
          blOffset = 0;
          loadData();
        }}
        onclearfilters={() => {
          blFilters = {};
          blOffset = 0;
          loadData();
        }}
      />
    {/if}
  {:else if subView === 'interlinking'}
    <InterlinkingTab sessionId={sessionId} onerror={(msg) => onerror?.(msg)} />
  {/if}
</div>

<style>
  .links-explorer {
    padding: 24px;
  }
</style>
