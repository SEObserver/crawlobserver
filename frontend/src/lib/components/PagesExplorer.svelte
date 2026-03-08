<script>
  import { onMount } from 'svelte';
  import { getPages, getRedirectPages } from '../api.js';
  import { statusBadge, fmt, fmtSize, fmtN, trunc, fetchAll, downloadCSV } from '../utils.js';
  import { PAGE_SIZE, TAB_FILTERS } from '../tabColumns.js';
  import { t } from '../i18n/index.svelte.js';
  import DataTable from './DataTable.svelte';
  import UrlActions from './UrlActions.svelte';

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
    { id: 'redirects', label: () => t('pages.redirects') },
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
  let sortColumn = $state('');
  let sortOrder = $state('');
  let redirectPages = $state([]);
  let redirectPagesOffset = $state(0);
  let hasMoreRedirectPages = $state(false);

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

  function effectiveFilters() {
    if (filters.content_type || filters.status_code) return filters;
    return { content_type: 'text/html', ...filters };
  }

  async function loadData() {
    try {
      if (subView === 'redirects') {
        const result = await getRedirectPages(
          sessionId,
          PAGE_SIZE,
          redirectPagesOffset,
          filters,
          sortColumn,
          sortOrder,
        );
        redirectPages = result || [];
        hasMoreRedirectPages = redirectPages.length === PAGE_SIZE;
      } else {
        const result = await getPages(
          sessionId,
          PAGE_SIZE,
          pagesOffset,
          effectiveFilters(),
          sortColumn,
          sortOrder,
        );
        pages = result || [];
        hasMorePages = pages.length === PAGE_SIZE;
      }
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function switchSubView(sv) {
    subView = sv;
    filters = {};
    pagesOffset = 0;
    redirectPagesOffset = 0;
    sortColumn = '';
    sortOrder = '';
    pushFilters(sv, {}, 0);
    loadData();
  }

  function handleSort(col, ord) {
    sortColumn = col;
    sortOrder = ord;
    pagesOffset = 0;
    loadData();
  }

  async function nextPage() {
    if (subView === 'redirects') {
      redirectPagesOffset += PAGE_SIZE;
      pushFilters(null, null, redirectPagesOffset);
    } else {
      pagesOffset += PAGE_SIZE;
      pushFilters(null, null, pagesOffset);
    }
    await loadData();
  }

  async function prevPage() {
    if (subView === 'redirects') {
      redirectPagesOffset = Math.max(0, redirectPagesOffset - PAGE_SIZE);
      pushFilters(null, null, redirectPagesOffset);
    } else {
      pagesOffset = Math.max(0, pagesOffset - PAGE_SIZE);
      pushFilters(null, null, pagesOffset);
    }
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

  const CSV_CONFIGS = {
    all: {
      headers: [
        'URL',
        'Status',
        'Title',
        'Words',
        'Internal Links Out',
        'External Links Out',
        'Size',
        'Time (ms)',
        'Depth',
        'PageRank',
      ],
      keys: [
        'URL',
        'StatusCode',
        'Title',
        'WordCount',
        'InternalLinksOut',
        'ExternalLinksOut',
        'BodySize',
        'FetchDurationMs',
        'Depth',
        'PageRank',
      ],
    },
    titles: {
      headers: ['URL', 'Title', 'Title Length', 'H1'],
      keys: ['URL', 'Title', 'TitleLength'],
      transform: (row) => ({ ...row, H1: row.H1?.[0] || '' }),
    },
    meta: {
      headers: ['URL', 'Meta Description', 'Meta Desc Length', 'Meta Keywords', 'OG Title'],
      keys: ['URL', 'MetaDescription', 'MetaDescLength', 'MetaKeywords', 'OGTitle'],
    },
    headings: {
      headers: ['URL', 'H1', 'H1 Count', 'H2', 'H2 Count'],
      keys: ['URL'],
      transform: (row) => ({
        ...row,
        H1_text: row.H1?.[0] || '',
        H1Count: row.H1?.length || 0,
        H2_text: row.H2?.[0] || '',
        H2Count: row.H2?.length || 0,
      }),
      customKeys: ['URL', 'H1_text', 'H1Count', 'H2_text', 'H2Count'],
    },
    images: {
      headers: ['URL', 'Images', 'Without Alt', 'Title', 'Words'],
      keys: ['URL', 'ImagesCount', 'ImagesNoAlt', 'Title', 'WordCount'],
    },
    indexability: {
      headers: ['URL', 'Indexable', 'Reason', 'Meta Robots', 'Canonical', 'Canonical Is Self'],
      keys: ['URL', 'IsIndexable', 'IndexReason', 'MetaRobots', 'Canonical', 'CanonicalIsSelf'],
    },
    response: {
      headers: ['URL', 'Status', 'Content Type', 'Encoding', 'Size', 'Time (ms)', 'Final URL'],
      keys: [
        'URL',
        'StatusCode',
        'ContentType',
        'ContentEncoding',
        'BodySize',
        'FetchDurationMs',
        'FinalURL',
      ],
    },
    redirects: {
      headers: ['URL', 'Status', 'Final URL', 'Inbound Internal Links'],
      keys: ['url', 'status_code', 'final_url', 'inbound_internal_links'],
    },
  };

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
    const cfg = CSV_CONFIGS[subView];
    if (!cfg) return;
    const fetcher =
      subView === 'redirects'
        ? (limit, offset) => getRedirectPages(sessionId, limit, offset, filters)
        : (limit, offset) => getPages(sessionId, limit, offset, effectiveFilters());
    const allData = await fetchAll(fetcher);
    const keys = cfg.customKeys || cfg.keys;
    let rows = allData;
    if (cfg.transform) rows = allData.map(cfg.transform);
    // For titles sub-view, H1 needs special handling
    if (subView === 'titles') {
      rows = allData.map((r) => ({ ...r, H1_first: r.H1?.[0] || '' }));
      downloadCSV(`pages-${subView}.csv`, cfg.headers, [...cfg.keys, 'H1_first'], rows);
      return;
    }
    downloadCSV(`pages-${subView}.csv`, cfg.headers, keys, rows);
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
  <div class="explorer-toolbar">
    <div class="pr-subview-bar">
      {#each SUB_VIEWS as sv}
        <button
          class="pr-subview-btn"
          class:pr-subview-active={subView === sv.id}
          onclick={() => switchSubView(sv.id)}>{sv.label()}</button
        >
      {/each}
    </div>
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
  </div>

  {#if subView === 'all'}
    <DataTable
      columns={[
        { label: t('session.url'), sortKey: 'url' },
        { label: t('session.status'), sortKey: 'status_code' },
        { label: t('session.title'), sortKey: 'title' },
        { label: t('session.words'), sortKey: 'word_count' },
        { label: t('session.intOut'), sortKey: 'internal_links_out' },
        { label: t('session.extOut'), sortKey: 'external_links_out' },
        { label: t('common.size'), sortKey: 'body_size' },
        { label: t('session.time'), sortKey: 'fetch_duration_ms' },
        { label: t('session.depth'), sortKey: 'depth' },
        { label: t('session.pr'), sortKey: 'pagerank' },
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
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a
              ><UrlActions url={p.URL} /></span
            ></td
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
        { label: t('session.url'), sortKey: 'url' },
        { label: t('session.title'), sortKey: 'title' },
        { label: t('session.length'), sortKey: 'title_length' },
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
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a
              ><UrlActions url={p.URL} /></span
            ></td
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
        { label: t('session.url'), sortKey: 'url' },
        { label: t('session.metaDescription'), sortKey: 'meta_description' },
        { label: t('session.length'), sortKey: 'meta_desc_length' },
        { label: t('session.metaKeywords'), sortKey: 'meta_keywords' },
        { label: t('session.ogTitle'), sortKey: 'og_title' },
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
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a
              ><UrlActions url={p.URL} /></span
            ></td
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
        { label: t('session.url'), sortKey: 'url' },
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
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a
              ><UrlActions url={p.URL} /></span
            ></td
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
        { label: t('session.url'), sortKey: 'url' },
        { label: t('session.images'), sortKey: 'images_count' },
        { label: t('session.withoutAlt'), sortKey: 'images_no_alt' },
        { label: t('session.title'), sortKey: 'title' },
        { label: t('session.words'), sortKey: 'word_count' },
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
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a
              ><UrlActions url={p.URL} /></span
            ></td
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
        { label: t('session.url'), sortKey: 'url' },
        { label: t('session.indexable'), sortKey: 'is_indexable' },
        { label: t('session.reason'), sortKey: 'index_reason' },
        { label: t('session.metaRobots'), sortKey: 'meta_robots' },
        { label: t('session.canonical'), sortKey: 'canonical' },
        { label: t('session.self'), sortKey: 'canonical_is_self' },
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
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a
              ><UrlActions url={p.URL} /></span
            ></td
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
        { label: t('session.url'), sortKey: 'url' },
        { label: t('session.status'), sortKey: 'status_code' },
        { label: t('session.contentType'), sortKey: 'content_type' },
        { label: t('session.encoding'), sortKey: 'content_encoding' },
        { label: t('common.size'), sortKey: 'body_size' },
        { label: t('session.time'), sortKey: 'fetch_duration_ms' },
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
      {sortColumn}
      {sortOrder}
      onsort={handleSort}
    >
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a
              ><UrlActions url={p.URL} /></span
            ></td
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
  {:else if subView === 'redirects'}
    <DataTable
      columns={[
        { label: t('session.url'), sortKey: 'url' },
        { label: t('session.status'), sortKey: 'status_code' },
        { label: t('session.finalUrl'), sortKey: 'final_url' },
        { label: t('session.inboundLinks'), sortKey: 'inbound_internal_links' },
      ]}
      filterKeys={TAB_FILTERS.redirects}
      {filters}
      data={redirectPages}
      offset={redirectPagesOffset}
      pageSize={PAGE_SIZE}
      hasMore={hasMoreRedirectPages}
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
      {#snippet row(p)}
        <tr>
          <td class="cell-url"
            ><span class="cell-url-inner"
              ><a href={urlDetailHref(p.url)} onclick={(e) => goToUrlDetail(e, p.url)}>{p.url}</a
              ><UrlActions url={p.url} /></span
            ></td
          >
          <td><span class="badge {statusBadge(p.status_code)}">{p.status_code}</span></td>
          <td class="cell-url">{p.final_url || '-'}</td>
          <td class:cell-warn={p.inbound_internal_links > 0}>{p.inbound_internal_links}</td>
        </tr>
      {/snippet}
    </DataTable>
  {/if}
</div>

<style>
  .pages-explorer {
    padding: 24px;
  }
</style>
