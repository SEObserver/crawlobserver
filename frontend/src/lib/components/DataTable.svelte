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
  } = $props();
</script>

<table>
  <thead>
    <tr>
      {#each columns as col}
        <th style={col.headerStyle || ''} class={col.class || ''}>{col.label}</th>
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
