<script>
  import { t } from '../i18n/index.svelte.js';

  let {
    columns,
    filterKeys,
    filters,
    data,
    offset,
    pageSize,
    hasMore,
    onsetfilter,
    onapplyfilters,
    onclearfilters,
    onnextpage,
    onprevpage,
    hasActiveFilters,
    row,
    extraHeaderCols = 0,
    sortColumn = '',
    sortOrder = '',
    onsort,
  } = $props();

  function handleSort(sortKey) {
    if (!sortKey || !onsort) return;
    if (sortColumn !== sortKey) {
      onsort(sortKey, 'asc');
    } else if (sortOrder === 'asc') {
      onsort(sortKey, 'desc');
    } else {
      onsort('', '');
    }
  }
</script>

<table>
  <thead>
    <tr>
      {#each columns as col}
        {#if col.sortKey && onsort}
          <th
            style={col.headerStyle || ''}
            class="{col.class || ''} sortable"
            onclick={() => handleSort(col.sortKey)}
          >
            <span class="sort-header">
              {col.label}
              <span class="sort-indicator" class:sort-active={sortColumn === col.sortKey}>
                {#if sortColumn === col.sortKey && sortOrder === 'asc'}
                  <svg
                    viewBox="0 0 24 24"
                    width="14"
                    height="14"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"><path d="M12 19V5m-7 7l7-7 7 7" /></svg
                  >
                {:else if sortColumn === col.sortKey && sortOrder === 'desc'}
                  <svg
                    viewBox="0 0 24 24"
                    width="14"
                    height="14"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"><path d="M12 5v14m7-7l-7 7-7-7" /></svg
                  >
                {:else}
                  <svg
                    viewBox="0 0 24 24"
                    width="14"
                    height="14"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    opacity="0.3"><path d="M12 5v14m7-7l-7 7-7-7" /></svg
                  >
                {/if}
              </span>
            </span>
          </th>
        {:else}
          <th style={col.headerStyle || ''} class={col.class || ''}>{col.label}</th>
        {/if}
      {/each}
    </tr>
    <tr class="filter-row">
      {#each filterKeys as key}
        <td
          ><input
            class="filter-input"
            placeholder={key}
            value={filters[key] || ''}
            oninput={(e) => onsetfilter?.(key, e.target.value)}
            onkeydown={(e) => e.key === 'Enter' && onapplyfilters?.()}
          /></td
        >
      {/each}
      {#if columns.length > filterKeys.length}
        {#each Array(columns.length - filterKeys.length) as _}
          <td
            >{#if hasActiveFilters && _ === undefined}<button
                class="btn-filter-clear"
                title={t('dataTable.clearFilters')}
                onclick={() => onclearfilters?.()}
                ><svg
                  viewBox="0 0 24 24"
                  width="14"
                  height="14"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  ><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg
                ></button
              >{/if}</td
          >
        {/each}
      {/if}
    </tr>
  </thead>
  <tbody>
    {#each data as item}
      {@render row(item)}
    {/each}
  </tbody>
</table>

{#if data.length > 0}
  <div class="pagination">
    <button class="btn btn-sm" onclick={() => onprevpage?.()} disabled={offset === 0}
      >{t('common.previous')}</button
    >
    <span class="pagination-info">{offset + 1} - {offset + data.length}</span>
    <button class="btn btn-sm" onclick={() => onnextpage?.()} disabled={!hasMore}
      >{t('common.next')}</button
    >
  </div>
{/if}

<style>
  th.sortable {
    cursor: pointer;
    user-select: none;
  }
  th.sortable:hover {
    background: var(--hover-bg, rgba(255, 255, 255, 0.05));
  }
  .sort-header {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .sort-indicator {
    display: inline-flex;
    flex-shrink: 0;
  }
  .sort-indicator.sort-active svg {
    opacity: 1;
  }
</style>
