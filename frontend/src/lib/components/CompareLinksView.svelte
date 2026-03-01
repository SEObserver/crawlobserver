<script>
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import Pagination from './Pagination.svelte';

  let {
    loading = false,
    linksResult = null,
    diffType = 'added',
    onswitchtype,
    onpagechange,
  } = $props();
</script>

<div class="sub-tabs">
  <button
    class="sub-tab"
    class:sub-tab-active={diffType === 'added'}
    onclick={() => onswitchtype?.('added')}
  >
    {t('compare.added')}
    {#if linksResult}<span class="badge-count">{fmtN(linksResult.total_added)}</span>{/if}
  </button>
  <button
    class="sub-tab"
    class:sub-tab-active={diffType === 'removed'}
    onclick={() => onswitchtype?.('removed')}
  >
    {t('compare.removed')}
    {#if linksResult}<span class="badge-count">{fmtN(linksResult.total_removed)}</span>{/if}
  </button>
</div>

{#if loading}
  <div class="loading-state">{t('common.loading')}</div>
{:else if linksResult && linksResult.links?.length > 0}
  <div class="table-scroll">
    <table class="table">
      <thead
        ><tr
          ><th>{t('common.source')}</th><th>{t('common.target')}</th><th
            >{t('session.anchorText')}</th
          ></tr
        ></thead
      >
      <tbody>
        {#each linksResult.links as l}
          <tr>
            <td class="cell-url">{l.source_url}</td>
            <td class="cell-url">{l.target_url}</td>
            <td class="cell-title">{l.anchor_text || '-'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  <Pagination
    offset={linksResult.offset || 0}
    limit={100}
    total={linksResult.links.length < 100
      ? (linksResult.offset || 0) + linksResult.links.length
      : Infinity}
    onchange={(o) => onpagechange?.(o)}
  />
{:else if linksResult}
  <div class="empty-state">{t('compare.noLinks', { type: diffType })}</div>
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
  .loading-state,
  .empty-state {
    padding: 32px;
    text-align: center;
    color: var(--text-secondary);
    font-size: 14px;
  }
</style>
