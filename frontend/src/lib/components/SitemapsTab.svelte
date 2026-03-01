<script>
  import { getSitemaps, getSitemapURLs } from '../api.js';
  import { a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

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
      <p class="panel-message text-muted">{t('common.loading')}</p>
    {:else if sitemaps.length === 0}
      <p class="panel-message text-muted">{t('sitemaps.noData')}</p>
    {:else}
      <table>
        <thead>
          <tr
            ><th>{t('common.url')}</th><th>{t('common.type')}</th><th>{t('sitemaps.urls')}</th><th
              >{t('common.status')}</th
            ></tr
          >
        </thead>
        <tbody>
          {#each sitemaps as s}
            <tr
              class:robots-host-active={selectedSitemap === s.URL}
              class="clickable-row"
              role="button"
              tabindex="0"
              onclick={() => selectSitemap(s.URL)}
              onkeydown={a11yKeydown(() => selectSitemap(s.URL))}
            >
              <td class="font-medium truncate sitemap-url-cell" title={s.URL}
                >{s.URL.replace(/^https?:\/\/[^/]+/, '')}</td
              >
              <td
                ><span
                  class="badge {s.Type === 'index'
                    ? 'badge-warning'
                    : s.Type === 'urlset'
                      ? 'badge-success'
                      : 'badge-muted'}">{s.Type || '?'}</span
                ></td
              >
              <td class="text-right">{s.URLCount?.toLocaleString() || 0}</td>
              <td
                ><span
                  class="badge {s.StatusCode === 200
                    ? 'badge-success'
                    : s.StatusCode >= 400
                      ? 'badge-error'
                      : 'badge-warning'}">{s.StatusCode}</span
                ></td
              >
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
  <div class="robots-detail">
    {#if selectedSitemap}
      <h3 class="sitemap-detail-title font-semibold text-secondary word-break">
        {selectedSitemap}
      </h3>
      {#if sitemapURLsLoading && sitemapURLs.length === 0}
        <p class="text-muted">{t('common.loading')}</p>
      {:else if sitemapURLs.length === 0}
        <p class="text-muted">{t('sitemaps.noUrls')}</p>
      {:else}
        <table>
          <thead>
            <tr
              ><th>{t('common.url')}</th><th>{t('sitemaps.lastModified')}</th><th
                >{t('sitemaps.changeFreq')}</th
              ><th>{t('sitemaps.priority')}</th></tr
            >
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
          <button
            class="btn btn-sm"
            onclick={sitemapURLsPrev}
            disabled={sitemapURLsOffset === 0 || sitemapURLsLoading}>{t('common.prev')}</button
          >
          <span class="row-num">
            {sitemapURLsOffset + 1}&ndash;{sitemapURLsOffset + sitemapURLs.length}
          </span>
          <button
            class="btn btn-sm"
            onclick={sitemapURLsNext}
            disabled={!hasMoreSitemapURLs || sitemapURLsLoading}>{t('common.next')}</button
          >
        </div>
      {/if}
    {:else}
      <div class="empty-state">
        <p>{t('sitemaps.selectSitemap')}</p>
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
