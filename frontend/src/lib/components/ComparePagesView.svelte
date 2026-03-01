<script>
  import { fmtN, trunc } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import Pagination from './Pagination.svelte';

  let {
    loading = false,
    pagesResult = null,
    diffType = 'changed',
    onswitchtype,
    onpagechange,
  } = $props();

  function cellChanged(a, b) {
    return a !== b ? 'cell-diff' : '';
  }
</script>

<div class="sub-tabs">
  <button
    class="sub-tab"
    class:sub-tab-active={diffType === 'changed'}
    onclick={() => onswitchtype?.('changed')}
  >
    {t('compare.changed')}
    {#if pagesResult}<span class="badge-count">{fmtN(pagesResult.total_changed)}</span>{/if}
  </button>
  <button
    class="sub-tab"
    class:sub-tab-active={diffType === 'added'}
    onclick={() => onswitchtype?.('added')}
  >
    {t('compare.added')}
    {#if pagesResult}<span class="badge-count">{fmtN(pagesResult.total_added)}</span>{/if}
  </button>
  <button
    class="sub-tab"
    class:sub-tab-active={diffType === 'removed'}
    onclick={() => onswitchtype?.('removed')}
  >
    {t('compare.removed')}
    {#if pagesResult}<span class="badge-count">{fmtN(pagesResult.total_removed)}</span>{/if}
  </button>
</div>

{#if loading}
  <div class="loading-state">{t('common.loading')}</div>
{:else if pagesResult && pagesResult.pages?.length > 0}
  <div class="table-scroll">
    <table class="table">
      <thead>
        <tr>
          <th>URL</th>
          {#if diffType === 'changed'}
            <th>Status A</th><th>Status B</th>
            <th>Title A</th><th>Title B</th>
            <th>Words A</th><th>Words B</th>
            <th>Depth A</th><th>Depth B</th>
          {:else}
            <th>Status</th><th>Title</th><th>Words</th><th>Depth</th>
          {/if}
        </tr>
      </thead>
      <tbody>
        {#each pagesResult.pages as p}
          <tr>
            <td class="cell-url">{p.url}</td>
            {#if diffType === 'changed'}
              <td class={cellChanged(p.status_code_a, p.status_code_b)}>{p.status_code_a}</td>
              <td class={cellChanged(p.status_code_a, p.status_code_b)}>{p.status_code_b}</td>
              <td class="cell-title {cellChanged(p.title_a, p.title_b)}">{trunc(p.title_a, 40)}</td>
              <td class="cell-title {cellChanged(p.title_a, p.title_b)}">{trunc(p.title_b, 40)}</td>
              <td class={cellChanged(p.word_count_a, p.word_count_b)}>{fmtN(p.word_count_a)}</td>
              <td class={cellChanged(p.word_count_a, p.word_count_b)}>{fmtN(p.word_count_b)}</td>
              <td class={cellChanged(p.depth_a, p.depth_b)}>{p.depth_a}</td>
              <td class={cellChanged(p.depth_a, p.depth_b)}>{p.depth_b}</td>
            {:else if diffType === 'added'}
              <td>{p.status_code_b}</td>
              <td class="cell-title">{trunc(p.title_b, 60)}</td>
              <td>{fmtN(p.word_count_b)}</td>
              <td>{p.depth_b}</td>
            {:else}
              <td>{p.status_code_a}</td>
              <td class="cell-title">{trunc(p.title_a, 60)}</td>
              <td>{fmtN(p.word_count_a)}</td>
              <td>{p.depth_a}</td>
            {/if}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  <Pagination
    offset={pagesResult.offset || 0}
    limit={100}
    total={pagesResult.pages.length < 100
      ? (pagesResult.offset || 0) + pagesResult.pages.length
      : Infinity}
    onchange={(o) => onpagechange?.(o)}
  />
{:else if pagesResult}
  <div class="empty-state">{t('compare.noPages', { type: diffType })}</div>
{/if}

<style>
  .sub-tabs {
    display: flex;
    gap: 0;
    padding: 12px 16px 0;
    border-bottom: 1px solid var(--border);
  }
  .sub-tab {
    padding: 8px 16px;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    font-size: 13px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .sub-tab-active {
    color: var(--accent);
    border-bottom-color: var(--accent);
    font-weight: 500;
  }
  .badge-count {
    background: var(--accent-light, rgba(124, 58, 237, 0.1));
    color: var(--accent);
    padding: 1px 7px;
    border-radius: 10px;
    font-size: 11px;
    font-weight: 600;
  }
  .table-scroll {
    overflow-x: auto;
    padding: 0;
  }
  .table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .table th {
    text-align: left;
    padding: 8px 12px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--text-secondary);
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }
  .table td {
    padding: 6px 12px;
    border-bottom: 1px solid var(--border);
    color: var(--text);
  }
  .cell-url {
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
  }
  .cell-title {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cell-diff {
    background: rgba(234, 179, 8, 0.1);
    font-weight: 500;
  }
  .loading-state,
  .empty-state {
    padding: 32px;
    text-align: center;
    color: var(--text-secondary);
    font-size: 14px;
  }
</style>
