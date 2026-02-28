<script>
  import { recomputeDepths, computePageRank, retryFailed, exportSession } from '../api.js';
  import { fmtN } from '../utils.js';

  let { session, stats, liveProgress, onerror, onstop, onresume, ondelete, onrefresh, oncompare } = $props();

  let showExportDialog = $state(false);
  let exportIncludeHTML = $state(false);

  function handleExport() {
    exportSession(session.ID, exportIncludeHTML);
    showExportDialog = false;
    exportIncludeHTML = false;
  }

  let recomputing = $state(false);
  let computingPR = $state(false);
  let retryingFailed = $state(false);

  async function handleRecomputeDepths() {
    recomputing = true;
    try {
      await recomputeDepths(session.ID);
      onrefresh?.();
    } catch (e) { onerror?.(e.message); }
    finally { recomputing = false; }
  }

  async function handleComputePageRank() {
    computingPR = true;
    try {
      await computePageRank(session.ID);
      onrefresh?.();
    } catch (e) { onerror?.(e.message); }
    finally { computingPR = false; }
  }

  async function handleRetryFailed() {
    retryingFailed = true;
    try {
      await retryFailed(session.ID);
      setTimeout(() => onrefresh?.(), 2000);
    } catch (e) { onerror?.(e.message); }
    finally { retryingFailed = false; }
  }
</script>

<div class="action-bar">
  {#if session.is_running}
    <span class="badge badge-info">Running
      {#if liveProgress[session.ID]}
        &middot; {fmtN(liveProgress[session.ID].pages_crawled)} pages &middot; {fmtN(liveProgress[session.ID].queue_size)} in queue
      {/if}
    </span>
    <button class="btn btn-sm btn-danger" onclick={() => onstop?.(session.ID)}>Stop</button>
  {:else}
    <span class="badge" class:badge-success={session.Status==='completed'} class:badge-error={session.Status==='failed'} class:badge-warning={session.Status==='stopped'}>{session.Status}</span>
    <button class="btn btn-sm" onclick={() => onresume?.(session.ID)}>Resume</button>
    <button class="btn btn-sm" onclick={handleRecomputeDepths} disabled={recomputing}>
      {recomputing ? 'Recomputing...' : 'Recompute Depths'}
    </button>
    <button class="btn btn-sm" onclick={handleComputePageRank} disabled={computingPR}>
      {computingPR ? 'Computing...' : 'Compute PageRank'}
    </button>
    {#if stats?.status_codes?.[0] > 0}
      <button class="btn btn-sm" onclick={handleRetryFailed} disabled={retryingFailed} title="Retry {stats.status_codes[0]} failed pages (status 0)">
        {retryingFailed ? 'Retrying...' : `Retry Failed (${stats.status_codes[0]})`}
      </button>
    {/if}
    <button class="btn btn-sm" onclick={() => showExportDialog = true} title="Export session as JSONL.gz">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
      Export
    </button>
    <button class="btn btn-sm btn-danger" onclick={() => ondelete?.(session.ID)}>Delete</button>
  {/if}
  <button class="btn btn-sm" onclick={() => onrefresh?.()}>
    <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
    Refresh
  </button>
  <button class="btn btn-sm" onclick={() => oncompare?.(session.ID)} title="Compare with another crawl">
    <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
    Compare
  </button>
</div>

{#if showExportDialog}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="html-modal-overlay" onclick={() => showExportDialog = false}>
    <div class="html-modal" style="max-width: 400px; height: auto;" onclick={(e) => e.stopPropagation()}>
      <div class="html-modal-header">
        <div class="html-modal-url">Export Session</div>
        <div class="html-modal-actions">
          <button class="btn btn-sm" title="Close" onclick={() => showExportDialog = false}>
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
      </div>
      <div style="padding: 20px; display: flex; flex-direction: column; gap: 16px;">
        <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
          <input type="checkbox" bind:checked={exportIncludeHTML} />
          Include HTML body (larger file)
        </label>
        <div style="display: flex; gap: 8px; justify-content: flex-end;">
          <button class="btn btn-sm" onclick={() => showExportDialog = false}>Cancel</button>
          <button class="btn btn-sm btn-primary" onclick={handleExport}>Download .jsonl.gz</button>
        </div>
      </div>
    </div>
  </div>
{/if}
