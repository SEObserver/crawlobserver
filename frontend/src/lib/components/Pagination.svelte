<script>
  import { t } from '../i18n/index.svelte.js';
  import { fmtN } from '../utils.js';

  let { offset = 0, limit = 100, total = 0, onchange } = $props();
</script>

{#if total > limit}
  <div class="pagination">
    <button class="btn btn-sm" disabled={offset === 0} onclick={() => onchange?.(Math.max(0, offset - limit))}>{t('common.previous')}</button>
    <span class="pagination-info">{offset + 1} - {Math.min(offset + limit, total)} of {fmtN(total)}</span>
    <button class="btn btn-sm" disabled={offset + limit >= total} onclick={() => onchange?.(offset + limit)}>{t('common.next')}</button>
  </div>
{/if}
