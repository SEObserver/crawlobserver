<script>
  import { getSitemapCoverageURLs } from '../api.js';
  import { fetchAll, downloadCSV } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import UrlActions from './UrlActions.svelte';

  let { sessionId, filter, onerror } = $props();

  const PAGE_SIZE = 100;

  let urls = $state([]);
  let loading = $state(false);
  let offset = $state(0);
  let hasMore = $state(false);

  async function load(newOffset) {
    loading = true;
    try {
      const data = await getSitemapCoverageURLs(sessionId, filter, PAGE_SIZE, newOffset);
      urls = data;
      offset = newOffset;
      hasMore = data.length === PAGE_SIZE;
    } catch (e) {
      urls = [];
      onerror?.(e.message || 'Failed to load sitemap coverage URLs');
    } finally {
      loading = false;
    }
  }

  function next() {
    load(offset + PAGE_SIZE);
  }

  function prev() {
    load(Math.max(0, offset - PAGE_SIZE));
  }

  const title = $derived(
    filter === 'sitemap_only' ? t('directives.sitemapOnly') : t('directives.inBoth'),
  );

  let exporting = $state(false);

  async function handleExportCSV() {
    if (exporting) return;
    exporting = true;
    try {
      const allData = await fetchAll((limit, offset) =>
        getSitemapCoverageURLs(sessionId, filter, limit, offset),
      );
      downloadCSV(
        `sitemap-${filter}.csv`,
        ['URL', 'Last Modified', 'Change Freq', 'Priority'],
        ['Loc', 'LastMod', 'ChangeFreq', 'Priority'],
        allData,
      );
    } finally {
      exporting = false;
    }
  }

  load(0);
</script>

<div class="coverage-urls">
  <div class="coverage-header">
    <h3 class="coverage-title">{title}</h3>
    <button
      class="btn btn-sm"
      onclick={handleExportCSV}
      disabled={exporting || (urls.length === 0 && !loading)}
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

  {#if loading && urls.length === 0}
    <p class="text-muted">{t('common.loading')}</p>
  {:else if urls.length === 0}
    <p class="text-muted">{t('sitemaps.noUrls')}</p>
  {:else}
    <table>
      <thead>
        <tr>
          <th>{t('common.url')}</th>
          <th>{t('sitemaps.lastModified')}</th>
          <th>{t('sitemaps.changeFreq')}</th>
          <th>{t('sitemaps.priority')}</th>
        </tr>
      </thead>
      <tbody>
        {#each urls as u}
          <tr>
            <td class="cell-url">
              <span class="cell-url-inner">
                <a href={u.Loc} target="_blank" rel="noopener">{u.Loc}</a>
                <UrlActions url={u.Loc} />
              </span>
            </td>
            <td class="row-num nowrap">{u.LastMod || '-'}</td>
            <td class="row-num">{u.ChangeFreq || '-'}</td>
            <td class="row-num">{u.Priority || '-'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    <div class="pagination pagination-gap">
      <button class="btn btn-sm" onclick={prev} disabled={offset === 0 || loading}>
        {t('common.prev')}
      </button>
      <span class="row-num">
        {offset + 1}&ndash;{offset + urls.length}
      </span>
      <button class="btn btn-sm" onclick={next} disabled={!hasMore || loading}>
        {t('common.next')}
      </button>
    </div>
  {/if}
</div>

<style>
  .coverage-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .coverage-title {
    font-size: 14px;
    font-weight: 600;
    margin: 0;
  }

  .pagination-gap {
    margin-top: 12px;
  }
</style>
