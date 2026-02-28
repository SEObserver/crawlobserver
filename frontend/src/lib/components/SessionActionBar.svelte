<script>
  import { recomputeDepths, computePageRank, retryFailed } from '../api.js';
  import { fmtN } from '../utils.js';

  let { session, stats, liveProgress, onerror, onstop, onresume, ondelete, onrefresh } = $props();

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
        &middot; {fmtN(liveProgress[session.ID].pages_crawled)} pages
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
    <button class="btn btn-sm btn-danger" onclick={() => ondelete?.(session.ID)}>Delete</button>
  {/if}
  <button class="btn btn-sm" onclick={() => onrefresh?.()}>
    <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
    Refresh
  </button>
</div>
