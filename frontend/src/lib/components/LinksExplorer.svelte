<script>
  import { onMount } from 'svelte';
  import { getInternalLinks, getExternalLinks, getBacklinksTop } from '../api.js';
  import { fmtN, trunc, fetchAll, downloadCSV } from '../utils.js';
  import { PAGE_SIZE, TAB_FILTERS } from '../tabColumns.js';
  import { t } from '../i18n/index.svelte.js';
  import DataTable from './DataTable.svelte';
  import ExternalChecksTab from './ExternalChecksTab.svelte';
  import BacklinksView from './BacklinksView.svelte';
  import UrlActions from './UrlActions.svelte';

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
    { id: 'checks', label: () => t('links.checks') },
    { id: 'backlinks', label: () => t('links.backlinks'), premium: true },
  ];

  let subView = $state(initialSubView);
  let intLinks = $state([]);
  let extLinks = $state([]);
  let intLinksOffset = $state(initialSubView === 'internal' ? initialOffset : 0);
  let extLinksOffset = $state(initialSubView === 'external' ? initialOffset : 0);
  let hasMoreIntLinks = $state(false);
  let hasMoreExtLinks = $state(false);
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
      } else if (subView === 'external') {
        const result = await getExternalLinks(
          sessionId,
          PAGE_SIZE,
          extLinksOffset,
          filters,
          sortColumn,
          sortOrder,
        );
        extLinks = result || [];
        hasMoreExtLinks = extLinks.length === PAGE_SIZE;
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
    extLinksOffset = 0;
    sortColumn = '';
    sortOrder = '';
    if (sv === 'backlinks') {
      blOffset = 0;
      blSort = 'trust_flow';
      blOrder = 'desc';
      blFilters = {};
      pushFilters(sv, {}, 0);
      loadData();
    } else if (sv !== 'checks') {
      pushFilters(sv, {}, 0);
      loadData();
    } else {
      pushFilters(sv, {}, 0);
    }
  }

  function handleSort(col, ord) {
    sortColumn = col;
    sortOrder = ord;
    intLinksOffset = 0;
    extLinksOffset = 0;
    loadData();
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
    } else if (subView === 'external') {
      const allData = await fetchAll((limit, offset) =>
        getExternalLinks(sessionId, limit, offset, filters),
      );
      downloadCSV(
        'links-external.csv',
        ['Source URL', 'Target URL', 'Anchor Text', 'Rel'],
        ['SourceURL', 'TargetURL', 'AnchorText', 'Rel'],
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
    {#if subView !== 'checks' && subView !== 'backlinks'}
      <button
        class="btn btn-sm"
        onclick={handleExportCSV}
        disabled={exporting}
        title={t('common.exportCsv')}
      >
        {#if exporting}
          <svg
            class="csv-spinner"
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            ><path
              d="M12 2v4m0 12v4m-7.07-3.93l2.83-2.83m8.48-8.48l2.83-2.83M2 12h4m12 0h4m-3.93 7.07l-2.83-2.83M7.76 7.76L4.93 4.93"
            /></svg
          >
          {t('common.exportingCsv')}
        {:else}
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            ><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
              points="7 10 12 15 17 10"
            /><line x1="12" y1="15" x2="12" y2="3" /></svg
          >
          {t('common.exportCsv')}
        {/if}
      </button>
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
    <DataTable
      columns={[
        { label: t('common.source'), sortKey: 'source_url' },
        { label: t('common.target'), sortKey: 'target_url' },
        { label: t('session.anchorText'), sortKey: 'anchor_text' },
        { label: t('session.rel'), sortKey: 'rel' },
      ]}
      filterKeys={TAB_FILTERS.external}
      {filters}
      data={extLinks}
      offset={extLinksOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMoreExtLinks}
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
              ><a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a><UrlActions
                url={l.TargetURL}
              /></span
            ></td
          >
          <td class="cell-title">{l.AnchorText || '-'}</td>
          <td>{l.Rel || '-'}</td>
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'checks'}
    <ExternalChecksTab
      {sessionId}
      basePath={`/sessions/${sessionId}/links/checks`}
      onpushurl={(u) => onpushurl?.(u)}
      onnavigate={(tab, f) => handleChecksViewSources(f?.target_url || '')}
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
  {/if}
</div>

<style>
  .links-explorer {
    padding: 24px;
  }
</style>
