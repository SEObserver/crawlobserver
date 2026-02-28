<script>
  import { recomputeDepths, computePageRank, retryFailed, exportSession } from '../api.js';
  import { fmtN, a11yKeydown } from '../utils.js';

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
  let retryingStatus = $state(null);

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

  async function handleRetryStatus(code) {
    retryingStatus = code;
    try {
      await retryFailed(session.ID, code);
      setTimeout(() => onrefresh?.(), 2000);
    } catch (e) { onerror?.(e.message); }
    finally { retryingStatus = null; }
  }

  function retryableStatusCodes() {
    if (!stats?.status_codes) return [];
    return Object.entries(stats.status_codes)
      .filter(([code, count]) => +code >= 400 && count > 0)
      .sort((a, b) => +a[0] - +b[0]);
  }

  function elapsed() {
    if (!session.StartedAt || session.StartedAt === '1970-01-01T00:00:00Z') return '';
    const start = new Date(session.StartedAt);
    const end = session.FinishedAt && session.FinishedAt !== '1970-01-01T00:00:00Z' ? new Date(session.FinishedAt) : new Date();
    const secs = Math.floor((end - start) / 1000);
    if (secs < 60) return `${secs}s`;
    if (secs < 3600) return `${Math.floor(secs / 60)}m ${secs % 60}s`;
    const h = Math.floor(secs / 3600);
    const m = Math.floor((secs % 3600) / 60);
    return `${h}h ${m}m`;
  }

  function fmtDate(d) {
    if (!d || d === '1970-01-01T00:00:00Z') return '';
    return new Date(d).toLocaleString();
  }
</script>

<div class="action-bar">
  {#if session.is_running}
    <span class="badge badge-info">Running
      {#if liveProgress[session.ID]}
        &middot; {fmtN(liveProgress[session.ID].pages_crawled)} pages &middot; {fmtN(liveProgress[session.ID].queue_size)} in queue
        {#if liveProgress[session.ID].lost_pages > 0}
          <span style="color: var(--error); font-weight: 600;">&middot; {fmtN(liveProgress[session.ID].lost_pages)} lost</span>
        {/if}
      {/if}
    </span>
    {#if session.StartedAt && session.StartedAt !== '1970-01-01T00:00:00Z'}
      <span class="action-bar-meta">Started {fmtDate(session.StartedAt)} &middot; {elapsed()}</span>
    {/if}
    <button class="btn btn-sm btn-danger" onclick={() => onstop?.(session.ID)}>Stop</button>
  {:else}
    <span class="badge" class:badge-success={session.Status==='completed'} class:badge-error={session.Status==='failed'} class:badge-warning={session.Status==='stopped' || session.Status==='completed_with_errors'}>{session.Status}</span>
    {#if session.StartedAt && session.StartedAt !== '1970-01-01T00:00:00Z'}
      <span class="action-bar-meta">{fmtDate(session.StartedAt)} &middot; {elapsed()}</span>
    {/if}
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
    {#each retryableStatusCodes() as [code, count]}
      <button class="btn btn-sm" onclick={() => handleRetryStatus(+code)} disabled={retryingStatus === +code} title="Retry {count} pages with status {code}">
        {retryingStatus === +code ? 'Retrying...' : `Retry ${code} (${fmtN(count)})`}
      </button>
    {/each}
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
  <div class="html-modal-overlay" role="button" tabindex="0"
    onclick={() => showExportDialog = false}
    onkeydown={a11yKeydown(() => showExportDialog = false)}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="html-modal" role="dialog" style="max-width: 400px; height: auto;" onclick={(e) => e.stopPropagation()}>
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

<style>
  .action-bar-meta {
    font-size: 12px;
    color: var(--text-muted);
    white-space: nowrap;
  }
  .html-modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.5);
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }
  .html-modal {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
    width: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .html-modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border);
    gap: 16px;
    flex-shrink: 0;
  }
  .html-modal-url {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-secondary);
  }
  .html-modal-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }
</style>
