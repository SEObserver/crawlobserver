<script>
  import { getSitemaps, getSitemapURLs } from '../api.js';
  import { a11yKeydown } from '../utils.js';

  let { sessionId, onerror } = $props();

  const PAGE_SIZE = 100;

  let sitemaps = $state([]);
  let selectedSitemap = $state(null);
  let sitemapURLs = $state([]);
  let sitemapsLoading = $state(false);
  let sitemapURLsLoading = $state(false);
  let sitemapURLsOffset = $state(0);
  let hasMoreSitemapURLs = $state(false);

  async function loadSitemaps() {
    sitemapsLoading = true;
    selectedSitemap = null;
    sitemapURLs = [];
    sitemapURLsOffset = 0;
    hasMoreSitemapURLs = false;
    try {
      sitemaps = await getSitemaps(sessionId);
    } catch (e) {
      sitemaps = [];
    } finally {
      sitemapsLoading = false;
    }
  }

  async function selectSitemap(url) {
    selectedSitemap = url;
    sitemapURLs = [];
    sitemapURLsOffset = 0;
    hasMoreSitemapURLs = false;
    sitemapURLsLoading = true;
    try {
      const data = await getSitemapURLs(sessionId, url, PAGE_SIZE, 0);
      sitemapURLs = data;
      hasMoreSitemapURLs = data.length === PAGE_SIZE;
    } catch (e) {
      sitemapURLs = [];
    } finally {
      sitemapURLsLoading = false;
    }
  }

  async function sitemapURLsNext() {
    if (!selectedSitemap) return;
    sitemapURLsOffset += PAGE_SIZE;
    sitemapURLsLoading = true;
    try {
      const data = await getSitemapURLs(sessionId, selectedSitemap, PAGE_SIZE, sitemapURLsOffset);
      sitemapURLs = data;
      hasMoreSitemapURLs = data.length === PAGE_SIZE;
    } catch (e) {
      sitemapURLs = [];
    } finally {
      sitemapURLsLoading = false;
    }
  }

  async function sitemapURLsPrev() {
    if (!selectedSitemap) return;
    sitemapURLsOffset = Math.max(0, sitemapURLsOffset - PAGE_SIZE);
    sitemapURLsLoading = true;
    try {
      const data = await getSitemapURLs(sessionId, selectedSitemap, PAGE_SIZE, sitemapURLsOffset);
      sitemapURLs = data;
      hasMoreSitemapURLs = data.length === PAGE_SIZE;
    } catch (e) {
      sitemapURLs = [];
    } finally {
      sitemapURLsLoading = false;
    }
  }

  loadSitemaps();
</script>

<div class="robots-layout">
  <div class="robots-hosts">
    {#if sitemapsLoading && sitemaps.length === 0}
      <p class="panel-message text-muted">Loading...</p>
    {:else if sitemaps.length === 0}
      <p class="panel-message text-muted">No sitemaps found. Run a crawl first.</p>
    {:else}
      <table>
        <thead>
          <tr><th>URL</th><th>Type</th><th>URLs</th><th>Status</th></tr>
        </thead>
        <tbody>
          {#each sitemaps as s}
            <tr class:robots-host-active={selectedSitemap === s.URL} class="clickable-row" role="button" tabindex="0" onclick={() => selectSitemap(s.URL)} onkeydown={a11yKeydown(() => selectSitemap(s.URL))}>
              <td class="font-medium truncate sitemap-url-cell" title={s.URL}>{s.URL.replace(/^https?:\/\/[^/]+/, '')}</td>
              <td><span class="badge {s.Type === 'index' ? 'badge-warning' : s.Type === 'urlset' ? 'badge-success' : 'badge-muted'}">{s.Type || '?'}</span></td>
              <td class="text-right">{s.URLCount?.toLocaleString() || 0}</td>
              <td><span class="badge {s.StatusCode === 200 ? 'badge-success' : s.StatusCode >= 400 ? 'badge-error' : 'badge-warning'}">{s.StatusCode}</span></td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
  <div class="robots-detail">
    {#if selectedSitemap}
      <h3 class="sitemap-detail-title font-semibold text-secondary word-break">{selectedSitemap}</h3>
      {#if sitemapURLsLoading && sitemapURLs.length === 0}
        <p class="text-muted">Loading...</p>
      {:else if sitemapURLs.length === 0}
        <p class="text-muted">No URLs in this sitemap.</p>
      {:else}
        <table>
          <thead>
            <tr><th>URL</th><th>Last Modified</th><th>Change Freq</th><th>Priority</th></tr>
          </thead>
          <tbody>
            {#each sitemapURLs as u}
              <tr>
                <td class="truncate url-loc-cell" title={u.Loc}>{u.Loc}</td>
                <td class="row-num nowrap">{u.LastMod || '-'}</td>
                <td class="row-num">{u.ChangeFreq || '-'}</td>
                <td class="row-num">{u.Priority || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <div class="pagination pagination-gap">
          <button class="btn btn-sm" onclick={sitemapURLsPrev} disabled={sitemapURLsOffset === 0 || sitemapURLsLoading}>Prev</button>
          <span class="row-num">
            {sitemapURLsOffset + 1}&ndash;{sitemapURLsOffset + sitemapURLs.length}
          </span>
          <button class="btn btn-sm" onclick={sitemapURLsNext} disabled={!hasMoreSitemapURLs || sitemapURLsLoading}>Next</button>
        </div>
      {/if}
    {:else}
      <div class="empty-state">
        <p>Select a sitemap to view its URLs</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .panel-message {
    padding: 20px;
  }

  .clickable-row {
    cursor: pointer;
  }

  .sitemap-url-cell {
    max-width: 200px;
  }

  .sitemap-detail-title {
    font-size: 14px;
    margin-bottom: 12px;
  }

  .url-loc-cell {
    max-width: 400px;
  }

  .pagination-gap {
    margin-top: 12px;
  }

  .empty-state {
    padding: 40px;
    text-align: center;
    color: var(--text-muted);
  }
</style>
