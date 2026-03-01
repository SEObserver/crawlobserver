<script>
  import { onMount } from 'svelte';
  import { getPages } from '../api.js';
  import { statusBadge, fmt, fmtSize, fmtN, trunc } from '../utils.js';
  import { PAGE_SIZE, TAB_FILTERS } from '../tabColumns.js';
  import { t } from '../i18n/index.svelte.js';
  import DataTable from './DataTable.svelte';

  let {
    sessionId,
    initialSubView = 'all',
    initialFilters = {},
    initialOffset = 0,
    onpushurl,
    onnavigate,
    onerror,
    onopenhtml,
  } = $props();

  const SUB_VIEWS = [
    { id: 'all', label: () => t('pages.all') },
    { id: 'titles', label: () => t('pages.titles') },
    { id: 'meta', label: () => t('pages.meta') },
    { id: 'headings', label: () => t('pages.headings') },
    { id: 'images', label: () => t('pages.images') },
    { id: 'indexability', label: () => t('pages.indexability') },
    { id: 'response', label: () => t('pages.response') },
  ];

  // Map sub-view id to TAB_FILTERS key
  function filterKey(sv) {
    return sv === 'all' ? 'overview' : sv;
  }

  let subView = $state(initialSubView);
  let pages = $state([]);
  let pagesOffset = $state(initialOffset);
  let hasMorePages = $state(false);
  let filters = $state({ ...initialFilters });

  function basePath() {
    return `/sessions/${sessionId}/pages`;
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
      const result = await getPages(sessionId, PAGE_SIZE, pagesOffset, filters);
      pages = result || [];
      hasMorePages = pages.length === PAGE_SIZE;
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function switchSubView(sv) {
    subView = sv;
    filters = {};
    pagesOffset = 0;
    pushFilters(sv, {}, 0);
    loadData();
  }

  async function nextPage() {
    pagesOffset += PAGE_SIZE;
    pushFilters(null, null, pagesOffset);
    await loadData();
  }

  async function prevPage() {
    pagesOffset = Math.max(0, pagesOffset - PAGE_SIZE);
    pushFilters(null, null, pagesOffset);
    await loadData();
  }

  function applyFilters() {
    pagesOffset = 0;
    pushFilters();
    loadData();
  }

  function clearFilters() {
    filters = {};
    pagesOffset = 0;
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

  function urlDetailHref(url) {
    return `/sessions/${sessionId}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(urlDetailHref(url));
  }

  onMount(() => {
    loadData();
  });
</script>

<div class="pages-explorer">
  <div class="pr-subview-bar">
    {#each SUB_VIEWS as sv}
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === sv.id}
        onclick={() => switchSubView(sv.id)}>{sv.label()}</button
      >
    {/each}
  </div>

  {#if subView === 'all'}
    <DataTable
      columns={[
        { label: t('session.url') },
        { label: t('session.status') },
        { label: t('session.title') },
        { label: t('session.words') },
        { label: t('session.intOut') },
        { label: t('session.extOut') },
        { label: t('common.size') },
        { label: t('session.time') },
        { label: t('session.depth') },
        { label: t('session.pr') },
        { label: '' },
      ]}
      filterKeys={TAB_FILTERS.overview}
      {filters}
      data={pages}
      offset={pagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMorePages}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td
          >
          <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
          <td class="cell-title">{trunc(p.Title, 60)}</td>
          <td>{fmtN(p.WordCount)}</td>
          <td>{fmtN(p.InternalLinksOut)}</td>
          <td>{fmtN(p.ExternalLinksOut)}</td>
          <td>{fmtSize(p.BodySize)}</td>
          <td>{fmt(p.FetchDurationMs)}</td>
          <td>{p.Depth}</td>
          <td class="text-accent font-medium">{p.PageRank > 0 ? p.PageRank.toFixed(1) : '-'}</td>
          <td
            >{#if p.BodySize > 0}<button
                class="btn-html"
                title={t('session.viewHtml')}
                onclick={() => onopenhtml?.(p.URL)}
                ><svg
                  viewBox="0 0 24 24"
                  width="16"
                  height="16"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  ><polyline points="16 18 22 12 16 6" /><polyline points="8 6 2 12 8 18" /></svg
                ></button
              >{/if}</td
          >
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'titles'}
    <DataTable
      columns={[
        { label: t('session.url') },
        { label: t('session.title') },
        { label: t('session.length') },
        { label: t('session.h1') },
      ]}
      filterKeys={TAB_FILTERS.titles}
      {filters}
      data={pages}
      offset={pagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMorePages}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td
          >
          <td class="cell-title" class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}
            >{p.Title || '-'}</td
          >
          <td class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.TitleLength}</td>
          <td class="cell-title">{p.H1?.[0] || '-'}</td>
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'meta'}
    <DataTable
      columns={[
        { label: t('session.url') },
        { label: t('session.metaDescription') },
        { label: t('session.length') },
        { label: t('session.metaKeywords') },
        { label: t('session.ogTitle') },
      ]}
      filterKeys={TAB_FILTERS.meta}
      {filters}
      data={pages}
      offset={pagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMorePages}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td
          >
          <td class="cell-title" class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}
            >{trunc(p.MetaDescription, 80)}</td
          >
          <td class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}
            >{p.MetaDescLength}</td
          >
          <td class="cell-title">{trunc(p.MetaKeywords, 60)}</td>
          <td class="cell-title">{trunc(p.OGTitle, 60)}</td>
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'headings'}
    <DataTable
      columns={[
        { label: t('session.url') },
        { label: t('session.h1') },
        { label: t('session.h1Count') },
        { label: t('session.h2') },
        { label: t('session.h2Count') },
      ]}
      filterKeys={TAB_FILTERS.headings}
      {filters}
      data={pages}
      offset={pagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMorePages}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td
          >
          <td class="cell-title" class:cell-warn={!p.H1?.length || p.H1.length > 1}
            >{p.H1?.[0] || '-'}</td
          >
          <td class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.length || 0}</td>
          <td class="cell-title">{p.H2?.[0] || '-'}</td>
          <td>{p.H2?.length || 0}</td>
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'images'}
    <DataTable
      columns={[
        { label: t('session.url') },
        { label: t('session.images') },
        { label: t('session.withoutAlt') },
        { label: t('session.title') },
        { label: t('session.words') },
      ]}
      filterKeys={TAB_FILTERS.images}
      {filters}
      data={pages}
      offset={pagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMorePages}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td
          >
          <td>{p.ImagesCount}</td>
          <td class:cell-warn={p.ImagesNoAlt > 0}>{p.ImagesNoAlt}</td>
          <td class="cell-title">{trunc(p.Title, 50)}</td>
          <td>{fmtN(p.WordCount)}</td>
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'indexability'}
    <DataTable
      columns={[
        { label: t('session.url') },
        { label: t('session.indexable') },
        { label: t('session.reason') },
        { label: t('session.metaRobots') },
        { label: t('session.canonical') },
        { label: t('session.self') },
      ]}
      filterKeys={TAB_FILTERS.indexability}
      {filters}
      data={pages}
      offset={pagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMorePages}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td
          >
          <td
            ><span
              class="badge"
              class:badge-success={p.IsIndexable}
              class:badge-error={!p.IsIndexable}
              >{p.IsIndexable ? t('common.yes') : t('common.no')}</span
            ></td
          >
          <td>{p.IndexReason || '-'}</td>
          <td>{p.MetaRobots || '-'}</td>
          <td class="cell-url">{trunc(p.Canonical, 60)}</td>
          <td>{p.CanonicalIsSelf ? t('common.yes') : '-'}</td>
        </tr>
      {/snippet}
    </DataTable>
  {:else if subView === 'response'}
    <DataTable
      columns={[
        { label: t('session.url') },
        { label: t('session.status') },
        { label: t('session.contentType') },
        { label: t('session.encoding') },
        { label: t('common.size') },
        { label: t('session.time') },
        { label: t('session.redirects') },
      ]}
      filterKeys={TAB_FILTERS.response}
      {filters}
      data={pages}
      offset={pagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMorePages}
      hasActiveFilters={hasActiveFilters()}
      onsetfilter={setFilter}
      onapplyfilters={applyFilters}
      onclearfilters={clearFilters}
      onnextpage={nextPage}
      onprevpage={prevPage}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td
          >
          <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
          <td>{p.ContentType || '-'}</td>
          <td>{p.ContentEncoding || '-'}</td>
          <td>{fmtSize(p.BodySize)}</td>
          <td>{fmt(p.FetchDurationMs)}</td>
          <td>{p.FinalURL !== p.URL ? p.FinalURL : '-'}</td>
        </tr>
      {/snippet}
    </DataTable>
  {/if}
</div>

<style>
  .pages-explorer {
    padding: 16px;
  }
</style>
