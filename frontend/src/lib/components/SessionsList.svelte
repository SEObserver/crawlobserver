<script>
  import { t } from '../i18n/index.svelte.js';
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
  <h1>{t('sessions.title')}</h1>
  <div class="flex-center-gap">
    <label class="btn btn-sm import-label">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
      {importing ? t('common.importing') : t('common.import')}
      <input type="file" accept=".gz,.jsonl.gz" onchange={handleImport} disabled={importing} class="sr-only-input" />
    </label>
    <button class="btn btn-primary" onclick={() => onnewcrawl?.()}>
      <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      {t('sessions.newCrawl')}
    </button>
  </div>
</div>

{#if importError}
  <div class="alert alert-error mb-sm">{importError}</div>
{/if}

{#if loading}
  <p class="loading-msg">{t('common.loading')}</p>
{:else if sessions.length === 0}
  <div class="empty-state">
    <h2>{t('sessions.noSessions')}</h2>
    <p>{t('sessions.noSessionsDesc')}</p>
    <button class="btn btn-primary mt-md" onclick={() => onnewcrawl?.()}>{t('sessions.startCrawl')}</button>
  </div>
{:else}
  <div class="card card-flush">
    {#each sessions as s}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="session-row" onclick={() => onselectsession?.(s)}>
        <div class="session-info">
          <div class="session-seed">{s.SeedURLs?.[0] || 'Unknown'}</div>
          <div class="session-meta">
            {#if s.is_queued}
              <span class="badge badge-queued">{t('session.queued')}</span>
            {:else if s.is_running}
              <span class="badge badge-info">
                {t('common.running')}
                {#if liveProgress[s.ID]}
                  &middot; {fmtN(liveProgress[s.ID].pages_crawled)} {t('common.pages')} &middot; {fmtN(liveProgress[s.ID].queue_size)} {t('sessions.queued')}
                  {#if liveProgress[s.ID].lost_pages > 0}
                    <span class="text-error font-semibold">&middot; {fmtN(liveProgress[s.ID].lost_pages)} {t('sessions.lost')}</span>
                  {/if}
                {/if}
              </span>
            {:else}
              <span class="badge" class:badge-success={s.Status==='completed'} class:badge-error={s.Status==='failed' || s.Status==='crashed'} class:badge-warning={s.Status==='stopped' || s.Status==='completed_with_errors'}>{s.Status}</span>
            {/if}
            {#if s.ProjectID}
              <span class="badge badge-project">{projects.find(p => p.id === s.ProjectID)?.name || 'Project'}</span>
            {/if}
            <span>{t('sessions.pagesCount', { count: fmtN(s.PagesCrawled) })}</span>
            {#if sessionStorageMap[s.ID]}<span>{fmtSize(sessionStorageMap[s.ID])}</span>{/if}
            <span>{timeAgo(s.StartedAt)}</span>
          </div>
        </div>
        <div class="session-actions" onclick={(e) => e.stopPropagation()}>
          {#if s.is_running || s.is_queued}
            <button class="btn btn-sm btn-danger" onclick={() => onstop?.(s.ID)}>{t('common.stop')}</button>
          {:else}
            <button class="btn btn-sm" onclick={() => onresume?.(s.ID)}>{t('sessions.resume')}</button>
            <button class="btn btn-sm btn-danger" onclick={() => ondelete?.(s.ID)}>{t('common.delete')}</button>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .session-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border-light);
    transition: background 0.1s;
    gap: 16px;
  }
  .session-row:last-child { border-bottom: none; }
  .session-row:hover { background: var(--bg-hover); cursor: pointer; }
  .session-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
    flex: 1;
  }
  .session-seed {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .session-meta {
    font-size: 12px;
    color: var(--text-muted);
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .session-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }
  .import-label {
    cursor: pointer;
    position: relative;
  }
  .sr-only-input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }
  .badge-queued {
    background: #fef3c7;
    color: #92400e;
  }
  .badge-project {
    background: var(--accent-light);
    color: var(--accent);
  }
</style>
