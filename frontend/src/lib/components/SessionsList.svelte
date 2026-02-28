<script>
  import { fmtN, fmtSize, timeAgo } from '../utils.js';
  import { importSession } from '../api.js';

  let { sessions, projects, liveProgress, sessionStorageMap, loading, onselectsession, onstop, onresume, ondelete, onnewcrawl, onrefresh } = $props();

  let importing = $state(false);
  let importError = $state('');

  async function handleImport(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    importing = true;
    importError = '';
    try {
      await importSession(file);
      onrefresh?.();
    } catch (err) {
      importError = err.message;
    } finally {
      importing = false;
      e.target.value = '';
    }
  }
</script>

<div class="page-header">
  <h1>Crawl Sessions</h1>
  <div style="display:flex; gap:8px; align-items:center;">
    <label class="btn btn-sm" style="cursor:pointer; position:relative;">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
      {importing ? 'Importing...' : 'Import'}
      <input type="file" accept=".gz,.jsonl.gz" onchange={handleImport} disabled={importing} style="position:absolute;opacity:0;width:0;height:0;" />
    </label>
    <button class="btn btn-primary" onclick={() => onnewcrawl?.()}>
      <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      New Crawl
    </button>
  </div>
</div>

{#if importError}
  <div class="alert alert-error" style="margin-bottom: 12px;">{importError}</div>
{/if}

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
