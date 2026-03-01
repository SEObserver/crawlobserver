<script>
  import { fmtN, statusBadge, trunc } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import Pagination from './Pagination.svelte';

  let { data, offset = 0, dirFilter = '', onnavigate, onfilterchange, onpagechange } = $props();

  function goToUrlDetail(e, url) {
    e.preventDefault();
    onnavigate?.(url);
  }
</script>

{#if data}
  <div class="pr-controls">
    <label>{t('pagerank.directoryFilter')}</label>
    <input class="pr-dir-filter" type="text" placeholder={t('pagerank.filterPlaceholder')} value={dirFilter}
      oninput={(e) => onfilterchange?.(e.target.value, false)}
      onkeydown={(e) => { if (e.key === 'Enter') onfilterchange?.(e.target.value, true); }} />
    <button class="btn btn-sm" onclick={() => onfilterchange?.(dirFilter, true)}>{t('common.filter')}</button>
    {#if dirFilter}
      <button class="btn btn-sm" onclick={() => onfilterchange?.('', true)}>{t('pagerank.clear')}</button>
    {/if}
    <span class="text-muted text-xs">{t('pagerank.pagesCount', { count: fmtN(data.total) })}</span>
  </div>
  <table>
    <thead>
      <tr><th>#</th><th>{t('common.url')}</th><th>{t('urlDetail.pageRank')}</th><th>{t('urlDetail.depth')}</th><th>{t('pagerank.intLinks')}</th><th>{t('pagerank.extLinks')}</th><th>{t('session.words')}</th><th>{t('common.status')}</th><th>{t('session.title')}</th></tr>
    </thead>
    <tbody>
      {#each data.pages || [] as p, i}
        <tr>
          <td class="row-num">{offset + i + 1}</td>
          <td class="cell-url"><a href="#detail" onclick={(e) => goToUrlDetail(e, p.url)}>{p.url}</a></td>
          <td class="text-accent font-semibold">{p.pagerank.toFixed(1)}</td>
          <td>{p.depth}</td>
          <td>{fmtN(p.internal_links_out)}</td>
          <td>{fmtN(p.external_links_out)}</td>
          <td>{fmtN(p.word_count)}</td>
          <td><span class="badge {statusBadge(p.status_code)}">{p.status_code}</span></td>
          <td class="cell-title">{trunc(p.title, 50)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
  <Pagination {offset} limit={50} total={data.total} onchange={(o) => onpagechange?.(o)} />
{:else}
  <p class="chart-empty">{t('pagerank.noData')}</p>
{/if}

<style>
  .pr-controls { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; flex-wrap: wrap; }
  .pr-controls label { font-size: 12px; color: var(--text-muted); font-weight: 500; }
  .pr-dir-filter { padding: 7px 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg-input); color: var(--text); font-size: 13px; font-family: inherit; width: 300px; max-width: 100%; }
  .pr-dir-filter:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-light); }
</style>
