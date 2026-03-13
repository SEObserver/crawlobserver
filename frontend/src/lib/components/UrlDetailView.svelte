<script>
  import {
    getPageDetail,
    getBacklinksTop,
    getStructuredData,
    getHreflangValidation,
    computeHreflangValidation,
  } from '../api.js';
  import { statusBadge, fmt, fmtSize, fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import BacklinksView from './BacklinksView.svelte';

  let { sessionId, projectId = null, url, onerror, onnavigate, onopenhtml } = $props();

  let pageDetail = $state(null);
  let pageDetailLoading = $state(false);
  let outLinksPage = $state(0);
  let inLinksPage = $state(0);
  const LINKS_PER_PAGE = 100;

  // Structured data
  let sdItems = $state(null);
  let sdLoading = $state(false);
  let sdExpanded = $state(false);

  async function loadStructuredData() {
    if (sdLoading || sdItems) return;
    sdLoading = true;
    try {
      sdItems = await getStructuredData(sessionId, url);
    } catch (e) {
      sdItems = [];
    }
    sdLoading = false;
    sdExpanded = true;
  }

  // Hreflang issues for this URL
  let hlIssues = $state([]);
  let hlLoaded = $state(false);
  let hlComputed = $state(false); // true if validation has been computed for this session
  let hlComputing = $state(false);

  async function loadHreflangIssues() {
    if (hlLoaded) return;
    hlLoaded = true;
    try {
      const result = await getHreflangValidation(sessionId, 100, 0, '', {}, '', '', url);
      hlIssues = result?.issues || [];
      // If summary has any key, validation was computed for this session
      hlComputed = result?.summary && Object.keys(result.summary).length > 0;
      // Also mark computed if we got issues for this specific URL
      if (hlIssues.length > 0) hlComputed = true;
    } catch {
      hlIssues = [];
      hlComputed = false;
    }
  }

  async function handleHlCompute() {
    hlComputing = true;
    try {
      await computeHreflangValidation(sessionId);
      // Wait for async computation
      setTimeout(() => {
        hlLoaded = false;
        hlComputing = false;
        loadHreflangIssues();
      }, 3000);
    } catch (e) {
      onerror?.(e.message);
      hlComputing = false;
    }
  }

  function hlIssueClass(type) {
    switch (type) {
      case 'missing_reciprocal':
      case 'xdefault_is_lang_page':
        return 'badge-error';
      case 'missing_self_ref':
      case 'inconsistent_cluster':
        return 'badge-warning';
      case 'target_not_crawled':
        return 'badge-info';
      default:
        return '';
    }
  }

  function hlIssueLabel(type) {
    switch (type) {
      case 'missing_reciprocal':
        return t('hreflang.missingReciprocal');
      case 'missing_self_ref':
        return t('hreflang.missingSelfRef');
      case 'xdefault_is_lang_page':
        return t('hreflang.xdefaultIsLang');
      case 'target_not_crawled':
        return t('hreflang.targetNotCrawled');
      case 'inconsistent_cluster':
        return t('hreflang.inconsistentCluster');
      default:
        return type;
    }
  }

  function cwvRating(value, good, mid) {
    return value <= good ? 'good' : value <= mid ? 'needs-improvement' : 'poor';
  }
  function cwvRatingLabel(rating) {
    const map = { good: 'cwvGood', 'needs-improvement': 'cwvNeedsImprovement', poor: 'cwvPoor' };
    return t('urlDetail.' + map[rating]);
  }

  // Bottom tabs
  let linksTab = $state('outbound');

  // Backlinks state
  let blData = $state([]);
  let blTotal = $state(0);
  let blOffset = $state(0);
  let blLimit = $state(100);
  let blSort = $state('trust_flow');
  let blOrder = $state('desc');
  let blFilters = $state({});
  let blLoaded = $state(false);

  async function loadPageDetail(outOffset = 0, inOffset = 0) {
    pageDetailLoading = true;
    try {
      pageDetail = await getPageDetail(
        sessionId,
        url,
        LINKS_PER_PAGE,
        outOffset,
        LINKS_PER_PAGE,
        inOffset,
      );
      outLinksPage = Math.floor(outOffset / LINKS_PER_PAGE);
      inLinksPage = Math.floor(inOffset / LINKS_PER_PAGE);
    } catch (e) {
      onerror?.(e.message);
    } finally {
      pageDetailLoading = false;
    }
  }

  async function loadOutLinksPage(offset) {
    if (!pageDetail?.page) return;
    try {
      const data = await getPageDetail(
        sessionId,
        pageDetail.page.URL,
        LINKS_PER_PAGE,
        offset,
        LINKS_PER_PAGE,
        inLinksPage * LINKS_PER_PAGE,
      );
      pageDetail = {
        ...pageDetail,
        links: { ...pageDetail.links, out_links: data.links.out_links },
      };
      outLinksPage = Math.floor(offset / LINKS_PER_PAGE);
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function loadInLinksPage(offset) {
    if (!pageDetail?.page) return;
    try {
      const data = await getPageDetail(
        sessionId,
        pageDetail.page.URL,
        LINKS_PER_PAGE,
        outLinksPage * LINKS_PER_PAGE,
        LINKS_PER_PAGE,
        offset,
      );
      pageDetail = { ...pageDetail, links: { ...pageDetail.links, in_links: data.links.in_links } };
      inLinksPage = Math.floor(offset / LINKS_PER_PAGE);
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function loadBacklinks() {
    if (!projectId) return;
    try {
      const filters = { ...blFilters, target_url: url };
      const result = await getBacklinksTop(projectId, blLimit, blOffset, filters, blSort, blOrder);
      blData = result?.backlinks || [];
      blTotal = result?.total || 0;
      blLoaded = true;
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function switchLinksTab(tab) {
    linksTab = tab;
    if (tab === 'backlinks' && !blLoaded) {
      loadBacklinks();
    }
  }

  function urlDetailHref(u) {
    return `/sessions/${sessionId}/url/${encodeURIComponent(u)}`;
  }

  function goToUrlDetail(e, u) {
    e.preventDefault();
    onnavigate?.(urlDetailHref(u));
  }

  loadPageDetail();
</script>

{#if pageDetailLoading}
  <p class="loading-msg">{t('common.loading')}</p>
{:else if pageDetail?.page}
  {@const pg = pageDetail.page}
  {@const outLinks = pageDetail.links?.out_links || []}
  {@const inLinks = pageDetail.links?.in_links || []}
  {@const outLinksCount = pageDetail.links?.out_links_count || 0}
  {@const inLinksCount = pageDetail.links?.in_links_count || 0}

  <!-- Header -->
  <div class="page-header detail-header-wrap">
    <div class="detail-header-left">
      <button
        class="btn btn-sm"
        onclick={() => onnavigate?.(`/sessions/${sessionId}/overview`)}
        title={t('common.back')}
      >
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"><polyline points="15 18 9 12 15 6" /></svg
        >
      </button>
      <h1 class="detail-title" title={pg.URL}>{pg.URL}</h1>
      <span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span>
    </div>
    <div class="detail-actions">
      <a class="btn btn-sm" href={pg.URL} target="_blank" rel="noopener">{t('urlDetail.openUrl')}</a
      >
      {#if pg.BodySize > 0}
        <button class="btn btn-sm" onclick={() => onopenhtml?.(pg.URL)}
          >{t('urlDetail.viewHtml')}</button
        >
      {/if}
    </div>
  </div>

  <!-- Summary Cards -->
  <div class="stats-grid">
    <div class="stat-card">
      <div class="stat-value">
        <span class="badge {statusBadge(pg.StatusCode)} badge-lg">{pg.StatusCode}</span>
      </div>
      <div class="stat-label">{t('urlDetail.statusCode')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value stat-value-sm">{pg.ContentType || '-'}</div>
      <div class="stat-label">{t('urlDetail.contentType')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{fmtSize(pg.BodySize)}</div>
      <div class="stat-label">{t('common.size')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{fmt(pg.FetchDurationMs)}</div>
      <div class="stat-label">{t('urlDetail.responseTime')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{pg.Depth}</div>
      <div class="stat-label">{t('urlDetail.depth')}</div>
    </div>
    {#if pg.PageRank > 0}
      <div class="stat-card">
        <div class="stat-value text-accent">{pg.PageRank.toFixed(1)}</div>
        <div class="stat-label">{t('urlDetail.pageRank')}</div>
      </div>
    {/if}
    {#if pg.FoundOn}
      <div class="stat-card">
        <div class="stat-value stat-value-xs truncate">
          <a href={urlDetailHref(pg.FoundOn)} onclick={(e) => goToUrlDetail(e, pg.FoundOn)}
            >{pg.FoundOn}</a
          >
        </div>
        <div class="stat-label">{t('urlDetail.foundOn')}</div>
      </div>
    {/if}
    <div class="stat-card">
      <div class="stat-value stat-value-xs">{new Date(pg.CrawledAt).toLocaleString()}</div>
      <div class="stat-label">{t('urlDetail.crawledAt')}</div>
    </div>
  </div>

  {#if pg.Error}
    <div class="alert alert-error card-section">
      <strong>{t('urlDetail.errorLabel')}</strong>
      {pg.Error}
    </div>
  {/if}

  <!-- Response Headers -->
  {#if pg.Headers && Object.keys(pg.Headers).length > 0}
    <div class="card card-section">
      <h3 class="section-title">{t('urlDetail.responseHeaders')}</h3>
      <table>
        <thead><tr><th>{t('urlDetail.header')}</th><th>{t('common.value')}</th></tr></thead>
        <tbody>
          {#each Object.entries(pg.Headers).sort((a, b) => a[0].localeCompare(b[0])) as [key, val]}
            <tr><td class="font-medium nowrap">{key}</td><td class="word-break">{val}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- SEO -->
  <div class="card card-section">
    <h3 class="section-title">{t('urlDetail.seo')}</h3>
    <table>
      <tbody>
        <tr
          ><td class="detail-label">{t('urlDetail.title')}</td><td
            >{pg.Title || '-'}
            <span class="text-muted">({pg.TitleLength} {t('urlDetail.chars')})</span></td
          ></tr
        >
        <tr
          ><td class="font-medium">{t('urlDetail.metaDescription')}</td><td
            >{pg.MetaDescription || '-'}
            <span class="text-muted">({pg.MetaDescLength} {t('urlDetail.chars')})</span></td
          ></tr
        >
        {#if pg.MetaKeywords}<tr
            ><td class="font-medium">{t('urlDetail.metaKeywords')}</td><td>{pg.MetaKeywords}</td
            ></tr
          >{/if}
        <tr
          ><td class="font-medium">{t('urlDetail.metaRobots')}</td><td>{pg.MetaRobots || '-'}</td
          ></tr
        >
        {#if pg.XRobotsTag}<tr
            ><td class="font-medium">{t('urlDetail.xRobotsTag')}</td><td>{pg.XRobotsTag}</td></tr
          >{/if}
        <tr
          ><td class="font-medium">{t('urlDetail.canonical')}</td><td
            >{pg.Canonical || '-'}
            {#if pg.CanonicalIsSelf}<span class="badge badge-success badge-xs"
                >{t('urlDetail.selfCanonical')}</span
              >{/if}</td
          ></tr
        >
        <tr
          ><td class="font-medium">{t('urlDetail.indexable')}</td><td
            ><span
              class="badge"
              class:badge-success={pg.IsIndexable}
              class:badge-error={!pg.IsIndexable}
              >{pg.IsIndexable ? t('common.yes') : t('common.no')}</span
            >
            {#if pg.IndexReason}<span class="text-muted">({pg.IndexReason})</span>{/if}</td
          ></tr
        >
        {#if pg.Lang}<tr
            ><td class="font-medium">{t('urlDetail.language')}</td><td>{pg.Lang}</td></tr
          >{/if}
      </tbody>
    </table>
  </div>

  <!-- Open Graph -->
  {#if pg.OGTitle || pg.OGDescription || pg.OGImage}
    <div class="card card-section">
      <h3 class="section-title">{t('urlDetail.openGraph')}</h3>
      <table>
        <tbody>
          {#if pg.OGTitle}<tr
              ><td class="detail-label">{t('urlDetail.ogTitle')}</td><td>{pg.OGTitle}</td></tr
            >{/if}
          {#if pg.OGDescription}<tr
              ><td class="font-medium">{t('urlDetail.ogDescription')}</td><td>{pg.OGDescription}</td
              ></tr
            >{/if}
          {#if pg.OGImage}
            <tr
              ><td class="font-medium">{t('urlDetail.ogImage')}</td><td
                ><a href={pg.OGImage} target="_blank" rel="noopener">{pg.OGImage}</a></td
              ></tr
            >
            <tr
              ><td></td><td
                ><img src={pg.OGImage} alt={t('urlDetail.ogPreview')} class="og-preview" /></td
              ></tr
            >
          {/if}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Headings -->
  {#if pg.H1?.length || pg.H2?.length || pg.H3?.length || pg.H4?.length || pg.H5?.length || pg.H6?.length}
    <div class="card card-section">
      <h3 class="section-title">{t('urlDetail.headings')}</h3>
      {#each [['H1', pg.H1], ['H2', pg.H2], ['H3', pg.H3], ['H4', pg.H4], ['H5', pg.H5], ['H6', pg.H6]] as [label, items]}
        {#if items?.length}
          <div class="mb-sm">
            <strong class="text-detail">{label}</strong>
            <span class="text-muted">({items.length})</span>
            <ul class="heading-list">
              {#each items as h}<li class="heading-item">{h}</li>{/each}
            </ul>
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  <!-- Redirect Chain -->
  {#if pg.RedirectChain?.length}
    <div class="card card-section">
      <h3 class="section-title">{t('urlDetail.redirectChain')}</h3>
      <table>
        <thead><tr><th>#</th><th>{t('common.url')}</th><th>{t('common.status')}</th></tr></thead>
        <tbody>
          {#each pg.RedirectChain as hop, i}
            <tr>
              <td>{i + 1}</td>
              <td class="cell-url">{hop.URL}</td>
              <td><span class="badge {statusBadge(hop.StatusCode)}">{hop.StatusCode}</span></td>
            </tr>
          {/each}
          <tr>
            <td>{pg.RedirectChain.length + 1}</td>
            <td class="cell-url font-medium">{pg.FinalURL || pg.URL}</td>
            <td><span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span></td>
          </tr>
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Hreflang -->
  {#if pg.Hreflang?.length}
    <div class="card card-section">
      <h3 class="section-title">{t('urlDetail.hreflang')}</h3>
      <table>
        <thead><tr><th>{t('urlDetail.language')}</th><th>{t('common.url')}</th></tr></thead>
        <tbody>
          {#each pg.Hreflang as h}
            <tr><td class="font-medium">{h.Lang}</td><td class="cell-url">{h.URL}</td></tr>
          {/each}
        </tbody>
      </table>
      {#if !hlLoaded}
        <button class="btn btn-sm hl-check-btn" onclick={loadHreflangIssues}
          >{t('hreflang.checkIssues')}</button
        >
      {:else if hlIssues.length > 0}
        <div class="hl-issues">
          <h4 class="hl-issues-title">{t('hreflang.issuesFound', { count: hlIssues.length })}</h4>
          <table class="hl-issues-table">
            <thead>
              <tr>
                <th>{t('hreflang.colType')}</th>
                <th>{t('hreflang.colTargetUrl')}</th>
                <th>{t('hreflang.colTargetLang')}</th>
                <th>{t('hreflang.colDetail')}</th>
              </tr>
            </thead>
            <tbody>
              {#each hlIssues as issue}
                <tr>
                  <td
                    ><span class="badge {hlIssueClass(issue.issue_type)}"
                      >{hlIssueLabel(issue.issue_type)}</span
                    ></td
                  >
                  <td class="cell-url">
                    {#if issue.target_url}
                      <a
                        href={`/sessions/${sessionId}/url/${encodeURIComponent(issue.target_url)}`}
                        onclick={(e) => {
                          e.preventDefault();
                          onnavigate?.(
                            `/sessions/${sessionId}/url/${encodeURIComponent(issue.target_url)}`,
                          );
                        }}>{issue.target_url}</a
                      >
                    {:else}
                      -
                    {/if}
                  </td>
                  <td>{issue.target_lang}</td>
                  <td class="hl-detail">{issue.detail}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else if !hlComputed}
        <div class="hl-not-computed">
          <span>{t('hreflang.notComputed')}</span>
          <button class="btn btn-sm" onclick={handleHlCompute} disabled={hlComputing}>
            {hlComputing ? t('common.loading') : t('hreflang.compute')}
          </button>
        </div>
      {:else}
        <div class="hl-ok">{t('hreflang.noIssuesForPage')}</div>
      {/if}
    </div>
  {/if}

  <!-- Structured Data -->
  {#if pg.SchemaTypes?.length}
    <div class="card card-section">
      <h3 class="section-title">{t('urlDetail.structuredData')}</h3>
      <div class="schema-badges">
        {#each pg.SchemaTypes as st}
          <span class="badge badge-info">{st}</span>
        {/each}
      </div>
      {#if pg.SchemaErrorCount > 0 || pg.SchemaWarningCount > 0 || pg.SchemaValidCount > 0}
        <div class="sd-summary">
          {#if pg.SchemaValidCount > 0}
            <span class="badge badge-success">{pg.SchemaValidCount} {t('urlDetail.sdValid')}</span>
          {/if}
          {#if pg.SchemaErrorCount > 0}
            <span class="badge badge-danger">{pg.SchemaErrorCount} {t('urlDetail.sdErrors')}</span>
          {/if}
          {#if pg.SchemaWarningCount > 0}
            <span class="badge badge-warning"
              >{pg.SchemaWarningCount} {t('urlDetail.sdWarnings')}</span
            >
          {/if}
        </div>
      {/if}
      {#if !sdExpanded}
        <button class="btn btn-sm btn-outline" onclick={loadStructuredData} disabled={sdLoading}>
          {sdLoading ? '...' : t('urlDetail.sdShowDetails')}
        </button>
      {/if}
      {#if sdExpanded && sdItems?.length}
        <div class="sd-details">
          {#each sdItems as item}
            <div
              class="sd-item"
              class:sd-item-valid={item.IsValid}
              class:sd-item-invalid={!item.IsValid}
            >
              <div class="sd-item-header">
                <span
                  class="badge"
                  class:badge-success={item.IsValid}
                  class:badge-danger={!item.IsValid}
                >
                  {item.SchemaType}
                </span>
                <span class="sd-source">{item.Source}</span>
              </div>
              {#if item.Errors?.length}
                <ul class="sd-issues">
                  {#each item.Errors as err}
                    <li class="sd-issue-error">{err}</li>
                  {/each}
                </ul>
              {/if}
              {#if item.Warnings?.length}
                <ul class="sd-issues">
                  {#each item.Warnings as warn}
                    <li class="sd-issue-warning">{warn}</li>
                  {/each}
                </ul>
              {/if}
              {#if item.IsValid && !item.Errors?.length && !item.Warnings?.length}
                <div class="sd-all-valid">{t('urlDetail.sdAllValid')}</div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Core Web Vitals -->
  {#if pg.CWVMeasured}
    <div class="card card-section">
      <h3 class="section-title">
        {t('urlDetail.coreWebVitals')}
        <span class="cwv-lab-badge">{t('urlDetail.cwvLabData')}</span>
      </h3>
      <div class="cwv-gauges">
        <div class="cwv-gauge">
          <div class="cwv-value cwv-{cwvRating(pg.CWVLCP, 2500, 4000)}">
            {Math.round(pg.CWVLCP)}ms
          </div>
          <div class="cwv-bar">
            <div
              class="cwv-fill cwv-{cwvRating(pg.CWVLCP, 2500, 4000)}"
              style="width:{Math.min(100, pg.CWVLCP / 60)}%"
            ></div>
          </div>
          <div class="cwv-label">LCP</div>
          <div class="cwv-rating">{cwvRatingLabel(cwvRating(pg.CWVLCP, 2500, 4000))}</div>
        </div>
        <div class="cwv-gauge">
          <div class="cwv-value cwv-{cwvRating(pg.CWVCLS, 0.1, 0.25)}">{pg.CWVCLS.toFixed(3)}</div>
          <div class="cwv-bar">
            <div
              class="cwv-fill cwv-{cwvRating(pg.CWVCLS, 0.1, 0.25)}"
              style="width:{Math.min(100, pg.CWVCLS / 0.005)}%"
            ></div>
          </div>
          <div class="cwv-label">CLS</div>
          <div class="cwv-rating">{cwvRatingLabel(cwvRating(pg.CWVCLS, 0.1, 0.25))}</div>
        </div>
        <div class="cwv-gauge">
          <div class="cwv-value cwv-{cwvRating(pg.CWVTTFB, 800, 1800)}">
            {Math.round(pg.CWVTTFB)}ms
          </div>
          <div class="cwv-bar">
            <div
              class="cwv-fill cwv-{cwvRating(pg.CWVTTFB, 800, 1800)}"
              style="width:{Math.min(100, pg.CWVTTFB / 30)}%"
            ></div>
          </div>
          <div class="cwv-label">TTFB</div>
          <div class="cwv-rating">{cwvRatingLabel(cwvRating(pg.CWVTTFB, 800, 1800))}</div>
        </div>
      </div>
    </div>
  {/if}

  <!-- Content -->
  <div class="card card-section">
    <h3 class="section-title">{t('urlDetail.content')}</h3>
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{fmtN(pg.WordCount)}</div>
        <div class="stat-label">{t('urlDetail.words')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{pg.ImagesCount}</div>
        <div class="stat-label">{t('urlDetail.imagesCount')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value" style={pg.ImagesNoAlt > 0 ? 'color: var(--warning)' : ''}>
          {pg.ImagesNoAlt}
        </div>
        <div class="stat-label">{t('urlDetail.imagesNoAlt')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{fmtN(pg.InternalLinksOut)}</div>
        <div class="stat-label">{t('urlDetail.internalLinksOut')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{fmtN(pg.ExternalLinksOut)}</div>
        <div class="stat-label">{t('urlDetail.externalLinksOut')}</div>
      </div>
    </div>
  </div>

  <!-- JS Rendering -->
  {#if pg.JSRendered}
    <div class="card card-section">
      <h3 class="section-title">{t('urlDetail.jsRendering')}</h3>
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">{fmt(pg.JSRenderDurationMs)}</div>
          <div class="stat-label">{t('urlDetail.renderTime')}</div>
        </div>
        {#if pg.JSRenderError}
          <div class="stat-card">
            <div class="stat-value stat-value-xs" style="color: var(--error)">
              {pg.JSRenderError}
            </div>
            <div class="stat-label">{t('urlDetail.renderError')}</div>
          </div>
        {/if}
      </div>
      {#if !pg.JSRenderError}
        <h4 class="subsection-title">{t('urlDetail.staticVsRendered')}</h4>
        <table>
          <thead
            ><tr
              ><th>{t('urlDetail.field')}</th><th>{t('urlDetail.staticValue')}</th><th
                >{t('urlDetail.renderedValue')}</th
              ><th>{t('common.status')}</th></tr
            ></thead
          >
          <tbody>
            <tr>
              <td class="font-medium">{t('urlDetail.title')}</td>
              <td class="cell-title">{pg.Title || '-'}</td>
              <td class="cell-title">{pg.RenderedTitle || '-'}</td>
              <td
                >{#if pg.JSChangedTitle}<span class="badge badge-warning"
                    >{t('urlDetail.changed')}</span
                  >{:else}<span class="badge badge-success">{t('urlDetail.same')}</span>{/if}</td
              >
            </tr>
            <tr>
              <td class="font-medium">H1</td>
              <td class="cell-title">{pg.H1?.join(', ') || '-'}</td>
              <td class="cell-title">{pg.RenderedH1?.join(', ') || '-'}</td>
              <td
                >{#if pg.JSChangedH1}<span class="badge badge-warning"
                    >{t('urlDetail.changed')}</span
                  >{:else}<span class="badge badge-success">{t('urlDetail.same')}</span>{/if}</td
              >
            </tr>
            <tr>
              <td class="font-medium">{t('urlDetail.metaDescription')}</td>
              <td class="cell-title">{pg.MetaDescription || '-'}</td>
              <td class="cell-title">{pg.RenderedMetaDescription || '-'}</td>
              <td
                >{#if pg.JSChangedDescription}<span class="badge badge-warning"
                    >{t('urlDetail.changed')}</span
                  >{:else}<span class="badge badge-success">{t('urlDetail.same')}</span>{/if}</td
              >
            </tr>
            <tr>
              <td class="font-medium">{t('urlDetail.canonical')}</td>
              <td class="cell-url">{pg.Canonical || '-'}</td>
              <td class="cell-url">{pg.RenderedCanonical || '-'}</td>
              <td
                >{#if pg.JSChangedCanonical}<span class="badge badge-warning"
                    >{t('urlDetail.changed')}</span
                  >{:else}<span class="badge badge-success">{t('urlDetail.same')}</span>{/if}</td
              >
            </tr>
          </tbody>
        </table>
        <div class="stats-grid" style="margin-top: 12px">
          <div class="stat-card">
            <div class="stat-value">{pg.WordCount} → {pg.RenderedWordCount}</div>
            <div class="stat-label">
              {t('urlDetail.words')}
              {#if pg.JSChangedContent}<span class="badge badge-warning badge-xs"
                  >{t('urlDetail.changed')}</span
                >{/if}
            </div>
          </div>
          <div class="stat-card">
            <div class="stat-value" style={pg.JSAddedLinks > 0 ? 'color: var(--warning)' : ''}>
              {pg.JSAddedLinks > 0 ? '+' : ''}{pg.JSAddedLinks}
            </div>
            <div class="stat-label">{t('urlDetail.deltaLinks')}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value" style={pg.JSAddedImages > 0 ? 'color: var(--warning)' : ''}>
              {pg.JSAddedImages > 0 ? '+' : ''}{pg.JSAddedImages}
            </div>
            <div class="stat-label">{t('urlDetail.deltaImages')}</div>
          </div>
          {#if pg.JSAddedSchema}
            <div class="stat-card">
              <div class="stat-value" style="color: var(--warning)">{t('common.yes')}</div>
              <div class="stat-label">{t('urlDetail.newSchema')}</div>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Links Tabs -->
  <div class="card card-section">
    <div class="links-tabs">
      <button
        class="links-tab-btn"
        class:links-tab-active={linksTab === 'outbound'}
        onclick={() => switchLinksTab('outbound')}
      >
        {t('urlDetail.outboundLinks')} <span class="tab-count">({fmtN(outLinksCount)})</span>
      </button>
      <button
        class="links-tab-btn"
        class:links-tab-active={linksTab === 'inbound'}
        onclick={() => switchLinksTab('inbound')}
      >
        {t('urlDetail.inboundLinks')} <span class="tab-count">({fmtN(inLinksCount)})</span>
      </button>
      <button
        class="links-tab-btn links-tab-premium"
        class:links-tab-active={linksTab === 'backlinks'}
        onclick={() => switchLinksTab('backlinks')}
      >
        <span class="premium-star">&#9733;</span>
        {t('urlDetail.backlinks')}
        {#if blLoaded}<span class="tab-count">({fmtN(blTotal)})</span>{/if}
      </button>
    </div>

    {#if linksTab === 'outbound'}
      {#if outLinks.length > 0}
        <table>
          <thead>
            <tr>
              <th>{t('urlDetail.targetUrl')}</th>
              <th>{t('urlDetail.anchor')}</th>
              <th>{t('common.type')}</th>
              <th>{t('session.tag')}</th>
              <th>{t('session.rel')}</th>
            </tr>
          </thead>
          <tbody>
            {#each outLinks as l}
              <tr>
                <td class="cell-url">
                  {#if l.IsInternal}
                    <a
                      href={urlDetailHref(l.TargetURL)}
                      onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a
                    >
                  {:else}
                    <a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a>
                  {/if}
                </td>
                <td class="cell-title">{l.AnchorText || '-'}</td>
                <td>
                  <span
                    class="badge"
                    class:badge-success={l.IsInternal}
                    class:badge-warning={!l.IsInternal}
                  >
                    {l.IsInternal ? t('common.internal') : t('common.external')}
                  </span>
                </td>
                <td>{l.Tag || '-'}</td>
                <td>{l.Rel || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if outLinksCount > LINKS_PER_PAGE}
          <div class="links-pagination">
            <button
              class="btn btn-sm"
              disabled={outLinksPage === 0}
              onclick={() => loadOutLinksPage((outLinksPage - 1) * LINKS_PER_PAGE)}
              >{t('common.prev')}</button
            >
            <span class="links-info"
              >{outLinksPage * LINKS_PER_PAGE + 1}–{Math.min(
                (outLinksPage + 1) * LINKS_PER_PAGE,
                outLinksCount,
              )}
              {t('common.of')}
              {fmtN(outLinksCount)}</span
            >
            <button
              class="btn btn-sm"
              disabled={(outLinksPage + 1) * LINKS_PER_PAGE >= outLinksCount}
              onclick={() => loadOutLinksPage((outLinksPage + 1) * LINKS_PER_PAGE)}
              >{t('common.next')}</button
            >
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('urlDetail.outboundLinks')} — 0</p>
      {/if}
    {:else if linksTab === 'inbound'}
      {#if inLinks.length > 0}
        <table>
          <thead>
            <tr>
              <th>{t('urlDetail.sourceUrl')}</th>
              <th>{t('urlDetail.anchor')}</th>
              <th>{t('session.tag')}</th>
              <th>{t('session.rel')}</th>
            </tr>
          </thead>
          <tbody>
            {#each inLinks as l}
              <tr>
                <td class="cell-url"
                  ><a
                    href={urlDetailHref(l.SourceURL)}
                    onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a
                  ></td
                >
                <td class="cell-title">{l.AnchorText || '-'}</td>
                <td>{l.Tag || '-'}</td>
                <td>{l.Rel || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if inLinksCount > LINKS_PER_PAGE}
          <div class="links-pagination">
            <button
              class="btn btn-sm"
              disabled={inLinksPage === 0}
              onclick={() => loadInLinksPage((inLinksPage - 1) * LINKS_PER_PAGE)}
              >{t('common.prev')}</button
            >
            <span class="links-info"
              >{inLinksPage * LINKS_PER_PAGE + 1}–{Math.min(
                (inLinksPage + 1) * LINKS_PER_PAGE,
                inLinksCount,
              )}
              {t('common.of')}
              {fmtN(inLinksCount)}</span
            >
            <button
              class="btn btn-sm"
              disabled={(inLinksPage + 1) * LINKS_PER_PAGE >= inLinksCount}
              onclick={() => loadInLinksPage((inLinksPage + 1) * LINKS_PER_PAGE)}
              >{t('common.next')}</button
            >
          </div>
        {/if}
      {:else}
        <p class="chart-empty">{t('urlDetail.inboundLinks')} — 0</p>
      {/if}
    {:else if linksTab === 'backlinks'}
      {#if !projectId}
        <div class="bl-locked">
          <div class="bl-locked-icon">&#9733;</div>
          <p>{t('urlDetail.backlinksLocked')}</p>
        </div>
      {:else if blLoaded && blData.length === 0 && blTotal === 0}
        <p class="chart-empty">{t('urlDetail.backlinksEmpty')}</p>
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
          onnavigate={(u) => onnavigate?.(u)}
          onsort={(col, ord) => {
            blSort = col;
            blOrder = ord;
            blOffset = 0;
            loadBacklinks();
          }}
          onpagechange={(o) => {
            blOffset = o;
            loadBacklinks();
          }}
          onlimitchange={(l) => {
            blLimit = l;
            blOffset = 0;
            loadBacklinks();
          }}
          onsetfilter={(k, v) => {
            blFilters = { ...blFilters, [k]: v };
          }}
          onapplyfilters={() => {
            blOffset = 0;
            loadBacklinks();
          }}
          onclearfilters={() => {
            blFilters = {};
            blOffset = 0;
            loadBacklinks();
          }}
        />
      {/if}
    {/if}
  </div>
{:else}
  <p class="loading-msg">{t('urlDetail.pageNotFound')}</p>
{/if}

<style>
  .hl-check-btn {
    margin-top: 8px;
  }
  .hl-issues {
    margin-top: 12px;
    border-top: 1px solid var(--border, #eee);
    padding-top: 10px;
  }
  .hl-issues-title {
    font-size: 13px;
    font-weight: 600;
    margin: 0 0 8px 0;
    color: var(--text-secondary, #666);
  }
  .hl-issues-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 12px;
  }
  .hl-issues-table th {
    text-align: left;
    padding: 4px 8px;
    font-size: 11px;
    text-transform: uppercase;
    color: var(--text-secondary, #888);
    border-bottom: 1px solid var(--border, #ddd);
  }
  .hl-issues-table td {
    padding: 4px 8px;
    border-bottom: 1px solid var(--border, #eee);
    vertical-align: top;
  }
  .hl-issues-table tbody tr:nth-child(even) {
    background: var(--row-even, #f9f9f9);
  }
  .hl-detail {
    color: var(--text-secondary, #888);
    font-size: 11px;
  }
  .badge-error {
    background: #fee2e2;
    color: #991b1b;
  }
  .badge-warning {
    background: #fef3c7;
    color: #92400e;
  }
  .badge-info {
    background: #fef9c3;
    color: #854d0e;
  }
  .hl-not-computed {
    margin-top: 8px;
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 12px;
    color: var(--text-secondary, #888);
  }
  .hl-ok {
    margin-top: 8px;
    font-size: 12px;
    color: #16a34a;
  }
  .detail-header-wrap {
    gap: 12px;
    flex-wrap: wrap;
  }
  .detail-header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    flex: 1;
  }
  .detail-title {
    font-size: 1rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .detail-actions {
    display: flex;
    gap: 6px;
  }
  .badge-lg {
    font-size: 1.2rem;
  }
  .badge-xs {
    font-size: 0.7rem;
  }
  .stat-value-sm {
    font-size: 0.95rem;
  }
  .stat-value-xs {
    font-size: 0.8rem;
  }
  .detail-label {
    font-weight: 500;
    width: 160px;
  }
  .text-detail {
    font-size: 0.85rem;
  }
  .heading-list {
    margin: 4px 0 0 20px;
    padding: 0;
  }
  .heading-item {
    font-size: 0.85rem;
    color: var(--text-secondary);
  }
  .og-preview {
    max-width: 300px;
    max-height: 200px;
    border-radius: 6px;
    border: 1px solid var(--border);
    margin-top: 4px;
  }
  .schema-badges {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .sd-summary {
    display: flex;
    gap: 6px;
    margin-top: 8px;
  }
  .sd-details {
    margin-top: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .sd-item {
    padding: 8px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
  }
  .sd-item-valid {
    border-left: 3px solid var(--success);
  }
  .sd-item-invalid {
    border-left: 3px solid var(--danger);
  }
  .sd-item-header {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .sd-source {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .sd-issues {
    margin: 4px 0 0 18px;
    padding: 0;
    list-style: none;
    font-size: 0.85rem;
  }
  .sd-issue-error::before {
    content: '\2716 ';
    color: var(--danger);
  }
  .sd-issue-warning::before {
    content: '\26A0 ';
    color: var(--warning);
  }
  .sd-all-valid {
    font-size: 0.85rem;
    color: var(--success);
    margin-top: 4px;
  }

  /* Core Web Vitals gauges */
  .cwv-gauges {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 16px;
  }
  .cwv-gauge {
    text-align: center;
  }
  .cwv-value {
    font-size: 1.4rem;
    font-weight: 700;
  }
  .cwv-good {
    color: var(--success);
  }
  .cwv-needs-improvement {
    color: var(--warning);
  }
  .cwv-poor {
    color: var(--danger);
  }
  .cwv-bar {
    height: 6px;
    background: var(--bg-alt);
    border-radius: 3px;
    margin: 6px 0;
    overflow: hidden;
  }
  .cwv-fill {
    height: 100%;
    border-radius: 3px;
    transition: width 0.3s;
  }
  .cwv-fill.cwv-good {
    background: var(--success);
  }
  .cwv-fill.cwv-needs-improvement {
    background: var(--warning);
  }
  .cwv-fill.cwv-poor {
    background: var(--danger);
  }
  .cwv-label {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-muted);
  }
  .cwv-rating {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .cwv-lab-badge {
    font-size: 0.65rem;
    font-weight: 500;
    background: var(--bg-alt);
    color: var(--text-muted);
    padding: 2px 6px;
    border-radius: 4px;
    vertical-align: middle;
    margin-left: 6px;
  }
  .links-pagination {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-top: 12px;
    justify-content: center;
  }
  .links-info {
    color: var(--text-muted);
    font-size: 0.85rem;
  }
  .subsection-title {
    font-size: 0.9rem;
    margin: 16px 0 8px;
    color: var(--text-secondary);
  }
  .links-tabs {
    display: flex;
    gap: 0;
    border-bottom: 2px solid var(--border);
    margin-bottom: 16px;
  }
  .links-tab-btn {
    padding: 10px 20px;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    margin-bottom: -2px;
    cursor: pointer;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-muted);
    transition:
      color 0.15s,
      border-color 0.15s;
  }
  .links-tab-btn:hover {
    color: var(--text-primary);
  }
  .links-tab-active {
    color: var(--accent, #4f8ef7);
    border-bottom-color: var(--accent, #4f8ef7);
  }
  .links-tab-premium {
    color: #b8960c;
  }
  .links-tab-premium:hover {
    color: #a68400;
  }
  .links-tab-premium.links-tab-active {
    background: #c9a227;
    color: #fff;
    border-bottom-color: #c9a227;
    border-radius: 6px 6px 0 0;
  }
  .tab-count {
    font-weight: 400;
    opacity: 0.7;
    font-size: 12px;
  }
  .links-tab-active .premium-star {
    color: inherit;
  }
  .bl-locked {
    text-align: center;
    padding: 48px 24px;
    color: var(--text-muted);
  }
  .bl-locked-icon {
    font-size: 48px;
    color: #c9a227;
    margin-bottom: 12px;
  }
  .bl-locked p {
    font-size: 14px;
    max-width: 400px;
    margin: 0 auto;
    line-height: 1.5;
  }
</style>
