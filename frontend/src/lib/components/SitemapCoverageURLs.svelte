<script>
  import { getSitemapCoverageURLs, buildApiPath } from '../api.js';
  import { fetchAll, downloadCSV } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import UrlActions from './UrlActions.svelte';
  import ExportDropdown from './ExportDropdown.svelte';

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

  let apiPath = $derived(
    buildApiPath(`/sessions/${sessionId}/sitemap-coverage-urls`, { filter, limit: 100, offset: 0 }),
  );

  load(0);
</script>

<div class="coverage-urls">
  <div class="coverage-header">
    <h3 class="coverage-title">{title}</h3>
    <ExportDropdown
      onexportcsv={handleExportCSV}
      {exporting}
      {apiPath}
      disabled={urls.length === 0 && !loading}
    />
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
