<script>
  import { fmtN, fmtSize, timeAgo } from '../utils.js';

  let { sessions, projects, liveProgress, sessionStorageMap, loading, onselectsession, onstop, onresume, ondelete, onnewcrawl } = $props();
</script>

<div class="page-header">
  <h1>Crawl Sessions</h1>
  <button class="btn btn-primary" onclick={() => onnewcrawl?.()}>
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
    New Crawl
  </button>
</div>

{#if loading}
  <p style="color: var(--text-muted); padding: 40px 0;">Loading...</p>
{:else if sessions.length === 0}
  <div class="empty-state">
    <h2>No crawl sessions yet</h2>
    <p>Start your first crawl to begin analyzing your site.</p>
    <button class="btn btn-primary" style="margin-top: 16px;" onclick={() => onnewcrawl?.()}>Start a Crawl</button>
  </div>
{:else}
  <div class="card card-flush">
    {#each sessions as s}
      <div class="session-row">
        <div class="session-info">
          <div class="session-seed">{s.SeedURLs?.[0] || 'Unknown'}</div>
          <div class="session-meta">
            {#if s.is_running}
              <span class="badge badge-info">
                Running
                {#if liveProgress[s.ID]}
                  &middot; {fmtN(liveProgress[s.ID].pages_crawled)} pages &middot; {fmtN(liveProgress[s.ID].queue_size)} queued
                {/if}
              </span>
            {:else}
              <span class="badge" class:badge-success={s.Status==='completed'} class:badge-error={s.Status==='failed'} class:badge-warning={s.Status==='stopped'}>{s.Status}</span>
            {/if}
            {#if s.ProjectID}
              <span class="badge" style="background: var(--accent-light); color: var(--accent);">{projects.find(p => p.id === s.ProjectID)?.name || 'Project'}</span>
            {/if}
            <span>{fmtN(s.PagesCrawled)} pages</span>
            {#if sessionStorageMap[s.ID]}<span>{fmtSize(sessionStorageMap[s.ID])}</span>{/if}
            <span>{timeAgo(s.StartedAt)}</span>
          </div>
        </div>
        <div class="session-actions">
          <button class="btn btn-sm" onclick={() => onselectsession?.(s)}>View</button>
          {#if s.is_running}
            <button class="btn btn-sm btn-danger" onclick={() => onstop?.(s.ID)}>Stop</button>
          {:else}
            <button class="btn btn-sm" onclick={() => onresume?.(s.ID)}>Resume</button>
            <button class="btn btn-sm btn-danger" onclick={() => ondelete?.(s.ID)}>Delete</button>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}
