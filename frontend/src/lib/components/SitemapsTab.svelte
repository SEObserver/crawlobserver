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
      <p style="padding: 20px; color: var(--text-muted);">Loading...</p>
    {:else if sitemaps.length === 0}
      <p style="padding: 20px; color: var(--text-muted);">No sitemaps found. Run a crawl first.</p>
    {:else}
      <table>
        <thead>
          <tr><th>URL</th><th>Type</th><th>URLs</th><th>Status</th></tr>
        </thead>
        <tbody>
          {#each sitemaps as s}
            <tr class:robots-host-active={selectedSitemap === s.URL} role="button" tabindex="0" style="cursor:pointer" onclick={() => selectSitemap(s.URL)} onkeydown={a11yKeydown(() => selectSitemap(s.URL))}>
              <td style="font-weight: 500; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title={s.URL}>{s.URL.replace(/^https?:\/\/[^/]+/, '')}</td>
              <td><span class="badge {s.Type === 'index' ? 'badge-warning' : s.Type === 'urlset' ? 'badge-success' : 'badge-muted'}">{s.Type || '?'}</span></td>
              <td style="text-align: right;">{s.URLCount?.toLocaleString() || 0}</td>
              <td><span class="badge {s.StatusCode === 200 ? 'badge-success' : s.StatusCode >= 400 ? 'badge-error' : 'badge-warning'}">{s.StatusCode}</span></td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
  <div class="robots-detail">
    {#if selectedSitemap}
      <h3 style="font-size: 14px; font-weight: 600; margin-bottom: 12px; color: var(--text-secondary); word-break: break-all;">{selectedSitemap}</h3>
      {#if sitemapURLsLoading && sitemapURLs.length === 0}
        <p style="color: var(--text-muted);">Loading...</p>
      {:else if sitemapURLs.length === 0}
        <p style="color: var(--text-muted);">No URLs in this sitemap.</p>
      {:else}
        <table>
          <thead>
            <tr><th>URL</th><th>Last Modified</th><th>Change Freq</th><th>Priority</th></tr>
          </thead>
          <tbody>
            {#each sitemapURLs as u}
              <tr>
                <td style="max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title={u.Loc}>{u.Loc}</td>
                <td style="color: var(--text-muted); font-size: 12px; white-space: nowrap;">{u.LastMod || '-'}</td>
                <td style="color: var(--text-muted); font-size: 12px;">{u.ChangeFreq || '-'}</td>
                <td style="color: var(--text-muted); font-size: 12px;">{u.Priority || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <div class="pagination" style="margin-top: 12px;">
          <button class="btn btn-sm" onclick={sitemapURLsPrev} disabled={sitemapURLsOffset === 0 || sitemapURLsLoading}>Prev</button>
          <span style="font-size: 12px; color: var(--text-muted);">
            {sitemapURLsOffset + 1}&ndash;{sitemapURLsOffset + sitemapURLs.length}
          </span>
          <button class="btn btn-sm" onclick={sitemapURLsNext} disabled={!hasMoreSitemapURLs || sitemapURLsLoading}>Next</button>
        </div>
      {/if}
    {:else}
      <div style="padding: 40px; text-align: center; color: var(--text-muted);">
        <p>Select a sitemap to view its URLs</p>
      </div>
    {/if}
  </div>
</div>
