<script>
  import { onMount } from 'svelte';
  import { getPages, getExternalLinks, getInternalLinks } from '../api.js';
  import { statusBadge, fmt, fmtSize, fmtN, trunc } from '../utils.js';
  import { PAGE_SIZE, TAB_FILTERS, TABS } from '../tabColumns.js';
  import { pushURL } from '../router.js';
  import HtmlModal from './HtmlModal.svelte';
  import SessionActionBar from './SessionActionBar.svelte';
  import DataTable from './DataTable.svelte';
  import UrlDetailView from './UrlDetailView.svelte';
  import PageRankTab from './PageRankTab.svelte';
  import RobotsTab from './RobotsTab.svelte';
  import SitemapsTab from './SitemapsTab.svelte';
  import ReportsHub from './ReportsHub.svelte';
  import CustomTestsTab from './CustomTestsTab.svelte';
  import ExternalChecksTab from './ExternalChecksTab.svelte';
  import ResourceChecksTab from './ResourceChecksTab.svelte';

  let {
    session, stats, liveProgress, sessionStorageMap,
    initialTab = 'overview', initialFilters = {}, initialOffset = 0,
    initialDetailUrl = '',
    initialPrSubView = 'top', initialReportsSubView = 'overview',
    initialExtChecksSubView = 'domains', initialResourcesSubView = 'summary',
    onerror, onstop, onresume, ondelete, onrefresh, oncompare, onnavigate, ongohome,
  } = $props();

  // --- Local state (initialized from props) ---
  let tab = $state(initialTab);
  let detailUrl = $state(initialDetailUrl);
  let filters = $state({ ...initialFilters });
  let pages = $state([]);
  let extLinks = $state([]);
  let intLinks = $state([]);
  let pagesOffset = $state(initialOffset);
  let extLinksOffset = $state(initialOffset);
  let intLinksOffset = $state(initialOffset);
  let hasMorePages = $state(false);
  let hasMoreExtLinks = $state(false);
  let hasMoreIntLinks = $state(false);
  let prSubView = $state(initialPrSubView);
  let reportsSubView = $state(initialReportsSubView);
  let extChecksSubView = $state(initialExtChecksSubView);
  let resourcesSubView = $state(initialResourcesSubView);
  let showHtmlModal = $state(false);
  let htmlModalUrl = $state('');

  // Initialize offsets based on tab
  if (['internal'].includes(initialTab)) {
    intLinksOffset = initialOffset;
    pagesOffset = 0;
    extLinksOffset = 0;
  } else if (['external'].includes(initialTab)) {
    extLinksOffset = initialOffset;
    pagesOffset = 0;
    intLinksOffset = 0;
  } else {
    pagesOffset = initialOffset;
    extLinksOffset = 0;
    intLinksOffset = 0;
  }

  // --- Data loading ---
  async function loadTabData() {
    if (!session) return;
    const id = session.ID;
    try {
      if (['overview','titles','meta','headings','images','indexability','response'].includes(tab)) {
        const result = await getPages(id, PAGE_SIZE, pagesOffset, filters);
        pages = result || [];
        hasMorePages = pages.length === PAGE_SIZE;
      } else if (tab === 'internal') {
        const result = await getInternalLinks(id, PAGE_SIZE, intLinksOffset, filters);
        intLinks = result || [];
        hasMoreIntLinks = intLinks.length === PAGE_SIZE;
      } else if (tab === 'external') {
        const result = await getExternalLinks(id, PAGE_SIZE, extLinksOffset, filters);
        extLinks = result || [];
        hasMoreExtLinks = extLinks.length === PAGE_SIZE;
      }
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function switchTab(newTab) {
    tab = newTab;
    filters = {};
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (session) {
      const path = newTab === 'pagerank' ? `${newTab}/${prSubView}` : newTab === 'reports' ? `${newTab}/${reportsSubView}` : newTab;
      pushURL(`/sessions/${session.ID}/${path}`);
    }
    if (newTab !== 'pagerank' && newTab !== 'robots' && newTab !== 'sitemaps' && newTab !== 'reports' && newTab !== 'tests' && newTab !== 'ext-checks' && newTab !== 'resources') {
      loadTabData();
    }
  }

  async function nextPage() {
    if (tab === 'internal') { intLinksOffset += PAGE_SIZE; }
    else if (tab === 'external') { extLinksOffset += PAGE_SIZE; }
    else { pagesOffset += PAGE_SIZE; }
    if (session) pushURL(`/sessions/${session.ID}/${tab}`, filters, currentOffset());
    await loadTabData();
  }

  async function prevPage() {
    if (tab === 'internal') { intLinksOffset = Math.max(0, intLinksOffset - PAGE_SIZE); }
    else if (tab === 'external') { extLinksOffset = Math.max(0, extLinksOffset - PAGE_SIZE); }
    else { pagesOffset = Math.max(0, pagesOffset - PAGE_SIZE); }
    if (session) pushURL(`/sessions/${session.ID}/${tab}`, filters, currentOffset());
    await loadTabData();
  }

  function currentOffset() {
    if (tab === 'internal') return intLinksOffset;
    if (tab === 'external') return extLinksOffset;
    return pagesOffset;
  }

  // --- Filter helpers ---
  function applyFilters() {
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (session) {
      pushURL(`/sessions/${session.ID}/${tab}`, filters);
    }
    loadTabData();
  }

  function clearFilters() {
    filters = {};
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (session) {
      pushURL(`/sessions/${session.ID}/${tab}`);
    }
    loadTabData();
  }

  function setFilter(key, val) {
    filters[key] = val;
    filters = { ...filters };
  }

  function hasActiveFilters() {
    return Object.values(filters).some(v => v && v !== '');
  }

  // --- URL detail helpers ---
  function urlDetailHref(url) {
    if (!session) return '#';
    return `/sessions/${session.ID}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(urlDetailHref(url));
  }

  // --- HTML modal ---
  function openHtmlModal(url) {
    htmlModalUrl = url;
    showHtmlModal = true;
  }

  function closeHtmlModal() {
    showHtmlModal = false;
    htmlModalUrl = '';
  }

  // --- Mount: load initial data ---
  onMount(() => {
    if (tab !== 'url-detail' && tab !== 'pagerank' && tab !== 'robots' && tab !== 'sitemaps' && tab !== 'reports' && tab !== 'tests' && tab !== 'ext-checks' && tab !== 'resources') {
      loadTabData();
    }
  });
</script>

{#if tab === 'url-detail' && session}
  <div class="breadcrumb">
    <a href="/" onclick={(e) => { e.preventDefault(); ongohome?.(); }}>Sessions</a>
    <span>/</span>
    <a href={`/sessions/${session.ID}/overview`} onclick={(e) => { e.preventDefault(); onnavigate?.(`/sessions/${session.ID}/overview`); }}>{session.SeedURLs?.[0] || session.ID}</a>
    <span>/</span>
    <span class="breadcrumb-active">URL Detail</span>
  </div>
  {#key detailUrl}
    <UrlDetailView sessionId={session.ID} url={detailUrl}
      onerror={(msg) => onerror?.(msg)} onnavigate={(url) => onnavigate?.(url)}
      onopenhtml={openHtmlModal} />
  {/key}

{:else if session}
  <!-- Session Detail -->
  <div class="breadcrumb">
    <a href="/" onclick={(e) => { e.preventDefault(); ongohome?.(); }}>Sessions</a>
    <span>/</span>
    <span class="breadcrumb-active">{session.SeedURLs?.[0] || session.ID}</span>
  </div>

  <SessionActionBar {session} {stats} {liveProgress}
    onerror={(msg) => onerror?.(msg)} onstop={(id) => onstop?.(id)} onresume={(id) => onresume?.(id)}
    ondelete={(id) => ondelete?.(id)} onrefresh={() => onrefresh?.()}
    oncompare={(id) => oncompare?.(id)} />

  {#if stats}
    {@const non200 = stats.status_codes ? Object.entries(stats.status_codes).filter(([k]) => k !== '200').reduce((a, [, v]) => a + v, 0) : stats.error_count}
    {@const maxDepth = stats.depth_distribution ? Math.max(...Object.keys(stats.depth_distribution).map(Number)) : 0}
    <div class="stats-grid">
      <div class="stat-card"><div class="stat-value">{fmtN(stats.total_pages)}</div><div class="stat-label">Pages</div></div>
      <div class="stat-card"><div class="stat-value" style={non200 > 0 ? 'color: var(--error)' : ''}>{fmtN(non200)}</div><div class="stat-label">Non-200</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(stats.internal_links)}</div><div class="stat-label">Internal links</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(stats.external_links)}</div><div class="stat-label">External links</div></div>
      <div class="stat-card"><div class="stat-value">{maxDepth}</div><div class="stat-label">Max depth</div></div>
      <div class="stat-card"><div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">Avg response</div></div>
    </div>
    {#if stats.status_codes && Object.keys(stats.status_codes).length > 1}
      <div class="stats-mini mt-sm">
        {#each Object.entries(stats.status_codes).sort((a, b) => Number(a[0]) - Number(b[0])) as [code, count]}
          <span class="stats-mini-item"><span class="badge {statusBadge(Number(code))}">{code}</span> {fmtN(count)}</span>
        {/each}
      </div>
    {/if}
    <div class="stats-secondary stats-secondary-gap">
      {#if stats.pages_per_second > 0}<span>{stats.pages_per_second.toFixed(1)} pages/sec</span>{/if}
      {#if stats.crawl_duration_sec > 0}<span>{stats.crawl_duration_sec < 60 ? stats.crawl_duration_sec.toFixed(0) + 's' : (stats.crawl_duration_sec / 60).toFixed(1) + 'min'}</span>{/if}
      {#if sessionStorageMap[session.ID]}<span>{fmtSize(sessionStorageMap[session.ID])} storage</span>{/if}
    </div>
  {/if}

  <div class="tab-bar">
    {#each TABS as t}
      <button class="tab" class:tab-active={tab === t.id} onclick={() => switchTab(t.id)}>{t.label}</button>
    {/each}
  </div>

  <div class="card card-flush card-tab-body">

    {#if tab === 'overview'}
      <DataTable columns={[{label:'URL'},{label:'Status'},{label:'Title'},{label:'Words'},{label:'Int Out'},{label:'Ext Out'},{label:'Size'},{label:'Time'},{label:'Depth'},{label:'PR'},{label:''}]}
        filterKeys={TAB_FILTERS.overview} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
        hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
        onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
        {#snippet row(p)}
          <tr>
            <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
            <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
            <td class="cell-title">{trunc(p.Title, 60)}</td>
            <td>{fmtN(p.WordCount)}</td>
            <td>{fmtN(p.InternalLinksOut)}</td>
            <td>{fmtN(p.ExternalLinksOut)}</td>
            <td>{fmtSize(p.BodySize)}</td>
            <td>{fmt(p.FetchDurationMs)}</td>
            <td>{p.Depth}</td>
            <td class="text-accent font-medium">{p.PageRank > 0 ? p.PageRank.toFixed(1) : '-'}</td>
            <td>{#if p.BodySize > 0}<button class="btn-html" title="View HTML" onclick={() => openHtmlModal(p.URL)}><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg></button>{/if}</td>
          </tr>
        {/snippet}
      </DataTable>

    {:else if tab === 'titles'}
      <DataTable columns={[{label:'URL'},{label:'Title'},{label:'Length'},{label:'H1'}]}
        filterKeys={TAB_FILTERS.titles} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
        hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
        onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
        {#snippet row(p)}
          <tr>
            <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
            <td class="cell-title" class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.Title || '-'}</td>
            <td class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.TitleLength}</td>
            <td class="cell-title">{p.H1?.[0] || '-'}</td>
          </tr>
        {/snippet}
      </DataTable>

    {:else if tab === 'meta'}
      <DataTable columns={[{label:'URL'},{label:'Meta Description'},{label:'Length'},{label:'Meta Keywords'},{label:'OG Title'}]}
        filterKeys={TAB_FILTERS.meta} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
        hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
        onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
        {#snippet row(p)}
          <tr>
            <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
            <td class="cell-title" class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{trunc(p.MetaDescription, 80)}</td>
            <td class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{p.MetaDescLength}</td>
            <td class="cell-title">{trunc(p.MetaKeywords, 60)}</td>
            <td class="cell-title">{trunc(p.OGTitle, 60)}</td>
          </tr>
        {/snippet}
      </DataTable>

    {:else if tab === 'headings'}
      <DataTable columns={[{label:'URL'},{label:'H1'},{label:'H1 Count'},{label:'H2'},{label:'H2 Count'}]}
        filterKeys={TAB_FILTERS.headings} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
        hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
        onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
        {#snippet row(p)}
          <tr>
            <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
            <td class="cell-title" class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.[0] || '-'}</td>
            <td class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.length || 0}</td>
            <td class="cell-title">{p.H2?.[0] || '-'}</td>
            <td>{p.H2?.length || 0}</td>
          </tr>
        {/snippet}
      </DataTable>

    {:else if tab === 'images'}
      <DataTable columns={[{label:'URL'},{label:'Images'},{label:'Without Alt'},{label:'Title'},{label:'Words'}]}
        filterKeys={TAB_FILTERS.images} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
        hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
        onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
        {#snippet row(p)}
          <tr>
            <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
            <td>{p.ImagesCount}</td>
            <td class:cell-warn={p.ImagesNoAlt > 0}>{p.ImagesNoAlt}</td>
            <td class="cell-title">{trunc(p.Title, 50)}</td>
            <td>{fmtN(p.WordCount)}</td>
          </tr>
        {/snippet}
      </DataTable>

    {:else if tab === 'indexability'}
      <DataTable columns={[{label:'URL'},{label:'Indexable'},{label:'Reason'},{label:'Meta Robots'},{label:'Canonical'},{label:'Self'}]}
        filterKeys={TAB_FILTERS.indexability} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
        hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
        onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
        {#snippet row(p)}
          <tr>
            <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
            <td><span class="badge" class:badge-success={p.IsIndexable} class:badge-error={!p.IsIndexable}>{p.IsIndexable ? 'Yes' : 'No'}</span></td>
            <td>{p.IndexReason || '-'}</td>
            <td>{p.MetaRobots || '-'}</td>
            <td class="cell-url">{trunc(p.Canonical, 60)}</td>
            <td>{p.CanonicalIsSelf ? 'Yes' : '-'}</td>
          </tr>
        {/snippet}
      </DataTable>

    {:else if tab === 'response'}
      <DataTable columns={[{label:'URL'},{label:'Status'},{label:'Content Type'},{label:'Encoding'},{label:'Size'},{label:'Time'},{label:'Redirects'}]}
        filterKeys={TAB_FILTERS.response} {filters} data={pages} offset={pagesOffset} pageSize={PAGE_SIZE}
        hasMore={hasMorePages} hasActiveFilters={hasActiveFilters()}
        onsetfilter={setFilter} onapplyfilters={applyFilters} onclearfilters={clearFilters} onnextpage={nextPage} onprevpage={prevPage}>
        {#snippet row(p)}
          <tr>
            <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
            <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
            <td>{p.ContentType || '-'}</td>
            <td>{p.ContentEncoding || '-'}</td>
            <td>{fmtSize(p.BodySize)}</td>
            <td>{fmt(p.FetchDurationMs)}</td>
            <td>{p.FinalURL !== p.URL ? p.FinalURL : '-'}</td>
          </tr>
        {/snippet}
      </DataTable>

    {:else if tab === 'internal'}
      <DataTable columns={[{label:'Source'},{label:'Target'},{label:'Anchor Text'},{label:'Tag'}]}
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

    {:else if tab === 'external'}
      <DataTable columns={[{label:'Source'},{label:'Target'},{label:'Anchor Text'},{label:'Rel'}]}
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

    {:else if tab === 'ext-checks'}
      <ExternalChecksTab sessionId={session.ID} initialSubView={extChecksSubView} initialFilters={filters}
        onpushurl={(u) => pushURL(u)}
        onnavigate={(t, f) => onnavigate?.(`/sessions/${session.ID}/${t}`, f)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'resources'}
      <ResourceChecksTab sessionId={session.ID} initialSubView={resourcesSubView} initialFilters={filters}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'pagerank'}
      <PageRankTab sessionId={session.ID} initialSubView={prSubView}
        onnavigate={(url) => goToUrlDetail({preventDefault:()=>{}}, url)}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'robots'}
      <RobotsTab sessionId={session.ID} onerror={(msg) => onerror?.(msg)} />

    {:else if tab === 'sitemaps'}
      <SitemapsTab sessionId={session.ID} onerror={(msg) => onerror?.(msg)} />
    {:else if tab === 'reports'}
      <ReportsHub sessionId={session.ID} {stats} initialSubView={reportsSubView}
        onnavigate={(url, f) => onnavigate?.(url, f)}
        onpushurl={(u) => pushURL(u)}
        onerror={(msg) => onerror?.(msg)} />
    {:else if tab === 'tests'}
      <CustomTestsTab sessionId={session.ID} onerror={(msg) => onerror?.(msg)} />
    {/if}
  </div>
{/if}

{#if showHtmlModal && session}
  <HtmlModal sessionId={session.ID} url={htmlModalUrl} onclose={closeHtmlModal} onerror={(msg) => onerror?.(msg)} />
{/if}

<style>
  .breadcrumb-active {
    color: var(--text);
  }
  .card-tab-body {
    border-top-left-radius: 0;
    border-top-right-radius: 0;
    border-top: none;
  }
  .stats-secondary-gap {
    margin-top: 10px;
  }
</style>
