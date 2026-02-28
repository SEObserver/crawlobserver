<script>
  import { getPageDetail } from '../api.js';
  import { statusBadge, fmt, fmtSize, fmtN } from '../utils.js';

  let { sessionId, url, onerror, onnavigate, onopenhtml } = $props();

  let pageDetail = $state(null);
  let pageDetailLoading = $state(false);
  let inLinksPage = $state(0);
  const IN_LINKS_PER_PAGE = 100;

  async function loadPageDetail(inOffset = 0) {
    pageDetailLoading = true;
    try {
      pageDetail = await getPageDetail(sessionId, url, IN_LINKS_PER_PAGE, inOffset);
      inLinksPage = Math.floor(inOffset / IN_LINKS_PER_PAGE);
    } catch (e) {
      onerror?.(e.message);
    } finally {
      pageDetailLoading = false;
    }
  }

  async function loadInLinksPage(offset) {
    if (!pageDetail?.page) return;
    try {
      const data = await getPageDetail(sessionId, pageDetail.page.URL, IN_LINKS_PER_PAGE, offset);
      pageDetail = { ...pageDetail, links: { ...pageDetail.links, in_links: data.links.in_links } };
      inLinksPage = Math.floor(offset / IN_LINKS_PER_PAGE);
    } catch (e) { onerror?.(e.message); }
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
  <p style="color: var(--text-muted); padding: 40px 0;">Loading...</p>
{:else if pageDetail?.page}
  {@const pg = pageDetail.page}
  {@const outLinks = pageDetail.links?.out_links || []}
  {@const inLinks = pageDetail.links?.in_links || []}
  {@const outLinksCount = pageDetail.links?.out_links_count || 0}
  {@const inLinksCount = pageDetail.links?.in_links_count || 0}

  <!-- Header -->
  <div class="page-header" style="gap: 12px; flex-wrap: wrap;">
    <div style="display: flex; align-items: center; gap: 8px; min-width: 0; flex: 1;">
      <button class="btn btn-sm" onclick={() => onnavigate?.(`/sessions/${sessionId}/overview`)} title="Back">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      </button>
      <h1 style="font-size: 1rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title={pg.URL}>{pg.URL}</h1>
      <span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span>
    </div>
    <div style="display: flex; gap: 6px;">
      <a class="btn btn-sm" href={pg.URL} target="_blank" rel="noopener">Open URL</a>
      {#if pg.BodySize > 0}
        <button class="btn btn-sm" onclick={() => onopenhtml?.(pg.URL)}>View HTML</button>
      {/if}
    </div>
  </div>

  <!-- Summary Cards -->
  <div class="stats-grid">
    <div class="stat-card"><div class="stat-value"><span class="badge {statusBadge(pg.StatusCode)}" style="font-size: 1.2rem;">{pg.StatusCode}</span></div><div class="stat-label">Status Code</div></div>
    <div class="stat-card"><div class="stat-value" style="font-size: 0.95rem;">{pg.ContentType || '-'}</div><div class="stat-label">Content-Type</div></div>
    <div class="stat-card"><div class="stat-value">{fmtSize(pg.BodySize)}</div><div class="stat-label">Size</div></div>
    <div class="stat-card"><div class="stat-value">{fmt(pg.FetchDurationMs)}</div><div class="stat-label">Response Time</div></div>
    <div class="stat-card"><div class="stat-value">{pg.Depth}</div><div class="stat-label">Depth</div></div>
    {#if pg.PageRank > 0}
      <div class="stat-card"><div class="stat-value" style="color: var(--accent)">{pg.PageRank.toFixed(1)}</div><div class="stat-label">PageRank</div></div>
    {/if}
    {#if pg.FoundOn}
      <div class="stat-card"><div class="stat-value" style="font-size: 0.8rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"><a href={urlDetailHref(pg.FoundOn)} onclick={(e) => goToUrlDetail(e, pg.FoundOn)} style="color: var(--accent);">{pg.FoundOn}</a></div><div class="stat-label">Found On</div></div>
    {/if}
    <div class="stat-card"><div class="stat-value" style="font-size: 0.8rem;">{new Date(pg.CrawledAt).toLocaleString()}</div><div class="stat-label">Crawled At</div></div>
  </div>

  {#if pg.Error}
    <div class="alert alert-error" style="margin-bottom: 16px;">
      <strong>Error:</strong> {pg.Error}
    </div>
  {/if}

  <!-- Response Headers -->
  {#if pg.Headers && Object.keys(pg.Headers).length > 0}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Response Headers</h3>
      <table>
        <thead><tr><th>Header</th><th>Value</th></tr></thead>
        <tbody>
          {#each Object.entries(pg.Headers).sort((a,b) => a[0].localeCompare(b[0])) as [key, val]}
            <tr><td style="font-weight: 500; white-space: nowrap;">{key}</td><td style="word-break: break-all;">{val}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- SEO -->
  <div class="card" style="margin-bottom: 16px;">
    <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">SEO</h3>
    <table>
      <tbody>
        <tr><td style="font-weight: 500; width: 160px;">Title</td><td>{pg.Title || '-'} <span style="color: var(--text-muted);">({pg.TitleLength} chars)</span></td></tr>
        <tr><td style="font-weight: 500;">Meta Description</td><td>{pg.MetaDescription || '-'} <span style="color: var(--text-muted);">({pg.MetaDescLength} chars)</span></td></tr>
        {#if pg.MetaKeywords}<tr><td style="font-weight: 500;">Meta Keywords</td><td>{pg.MetaKeywords}</td></tr>{/if}
        <tr><td style="font-weight: 500;">Meta Robots</td><td>{pg.MetaRobots || '-'}</td></tr>
        {#if pg.XRobotsTag}<tr><td style="font-weight: 500;">X-Robots-Tag</td><td>{pg.XRobotsTag}</td></tr>{/if}
        <tr><td style="font-weight: 500;">Canonical</td><td>{pg.Canonical || '-'} {#if pg.CanonicalIsSelf}<span class="badge badge-success" style="font-size: 0.7rem;">self</span>{/if}</td></tr>
        <tr><td style="font-weight: 500;">Indexable</td><td><span class="badge" class:badge-success={pg.IsIndexable} class:badge-error={!pg.IsIndexable}>{pg.IsIndexable ? 'Yes' : 'No'}</span> {#if pg.IndexReason}<span style="color: var(--text-muted);">({pg.IndexReason})</span>{/if}</td></tr>
        {#if pg.Lang}<tr><td style="font-weight: 500;">Language</td><td>{pg.Lang}</td></tr>{/if}
      </tbody>
    </table>
  </div>

  <!-- Open Graph -->
  {#if pg.OGTitle || pg.OGDescription || pg.OGImage}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Open Graph</h3>
      <table>
        <tbody>
          {#if pg.OGTitle}<tr><td style="font-weight: 500; width: 160px;">OG Title</td><td>{pg.OGTitle}</td></tr>{/if}
          {#if pg.OGDescription}<tr><td style="font-weight: 500;">OG Description</td><td>{pg.OGDescription}</td></tr>{/if}
          {#if pg.OGImage}
            <tr><td style="font-weight: 500;">OG Image</td><td><a href={pg.OGImage} target="_blank" rel="noopener">{pg.OGImage}</a></td></tr>
            <tr><td></td><td><img src={pg.OGImage} alt="OG preview" style="max-width: 300px; max-height: 200px; border-radius: 6px; border: 1px solid var(--border); margin-top: 4px;" /></td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Headings -->
  {#if pg.H1?.length || pg.H2?.length || pg.H3?.length || pg.H4?.length || pg.H5?.length || pg.H6?.length}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Headings</h3>
      {#each [['H1', pg.H1], ['H2', pg.H2], ['H3', pg.H3], ['H4', pg.H4], ['H5', pg.H5], ['H6', pg.H6]] as [label, items]}
        {#if items?.length}
          <div style="margin-bottom: 8px;">
            <strong style="font-size: 0.85rem;">{label}</strong> <span style="color: var(--text-muted);">({items.length})</span>
            <ul style="margin: 4px 0 0 20px; padding: 0;">
              {#each items as h}<li style="font-size: 0.85rem; color: var(--text-secondary);">{h}</li>{/each}
            </ul>
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  <!-- Redirect Chain -->
  {#if pg.RedirectChain?.length}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Redirect Chain</h3>
      <table>
        <thead><tr><th>#</th><th>URL</th><th>Status</th></tr></thead>
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
            <td class="cell-url" style="font-weight: 500;">{pg.FinalURL || pg.URL}</td>
            <td><span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span></td>
          </tr>
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Hreflang -->
  {#if pg.Hreflang?.length}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Hreflang</h3>
      <table>
        <thead><tr><th>Language</th><th>URL</th></tr></thead>
        <tbody>
          {#each pg.Hreflang as h}
            <tr><td style="font-weight: 500;">{h.Lang}</td><td class="cell-url">{h.URL}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Schema.org -->
  {#if pg.SchemaTypes?.length}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Schema.org</h3>
      <div style="display: flex; flex-wrap: wrap; gap: 6px;">
        {#each pg.SchemaTypes as t}
          <span class="badge badge-info">{t}</span>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Content -->
  <div class="card" style="margin-bottom: 16px;">
    <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Content</h3>
    <div class="stats-grid">
      <div class="stat-card"><div class="stat-value">{fmtN(pg.WordCount)}</div><div class="stat-label">Words</div></div>
      <div class="stat-card"><div class="stat-value">{pg.ImagesCount}</div><div class="stat-label">Images</div></div>
      <div class="stat-card"><div class="stat-value" style={pg.ImagesNoAlt > 0 ? 'color: var(--warning)' : ''}>{pg.ImagesNoAlt}</div><div class="stat-label">Images without alt</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(pg.InternalLinksOut)}</div><div class="stat-label">Internal Links Out</div></div>
      <div class="stat-card"><div class="stat-value">{fmtN(pg.ExternalLinksOut)}</div><div class="stat-label">External Links Out</div></div>
    </div>
  </div>

  <!-- Outbound Links -->
  {#if outLinks.length > 0}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Outbound Links <span style="color: var(--text-muted);">({fmtN(outLinksCount)})</span></h3>
      <table>
        <thead><tr><th>Target URL</th><th>Anchor</th><th>Type</th><th>Tag</th><th>Rel</th></tr></thead>
        <tbody>
          {#each outLinks as l}
            <tr>
              <td class="cell-url">
                {#if l.IsInternal}
                  <a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a>
                {:else}
                  <a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a>
                {/if}
              </td>
              <td class="cell-title">{l.AnchorText || '-'}</td>
              <td><span class="badge" class:badge-success={l.IsInternal} class:badge-warning={!l.IsInternal}>{l.IsInternal ? 'Internal' : 'External'}</span></td>
              <td>{l.Tag || '-'}</td>
              <td>{l.Rel || '-'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Inbound Links -->
  {#if inLinksCount > 0}
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Inbound Links <span style="color: var(--text-muted);">({fmtN(inLinksCount)})</span></h3>
      <table>
        <thead><tr><th>Source URL</th><th>Anchor</th><th>Tag</th><th>Rel</th></tr></thead>
        <tbody>
          {#each inLinks as l}
            <tr>
              <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
              <td class="cell-title">{l.AnchorText || '-'}</td>
              <td>{l.Tag || '-'}</td>
              <td>{l.Rel || '-'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      {#if inLinksCount > IN_LINKS_PER_PAGE}
        <div style="display: flex; gap: 8px; align-items: center; margin-top: 12px; justify-content: center;">
          <button class="btn btn-sm" disabled={inLinksPage === 0} onclick={() => loadInLinksPage((inLinksPage - 1) * IN_LINKS_PER_PAGE)}>Prev</button>
          <span style="color: var(--text-muted); font-size: 0.85rem;">{inLinksPage * IN_LINKS_PER_PAGE + 1}–{Math.min((inLinksPage + 1) * IN_LINKS_PER_PAGE, inLinksCount)} of {fmtN(inLinksCount)}</span>
          <button class="btn btn-sm" disabled={(inLinksPage + 1) * IN_LINKS_PER_PAGE >= inLinksCount} onclick={() => loadInLinksPage((inLinksPage + 1) * IN_LINKS_PER_PAGE)}>Next</button>
        </div>
      {/if}
    </div>
  {/if}
{:else}
  <p style="color: var(--text-muted); padding: 40px 0;">Page not found.</p>
{/if}
