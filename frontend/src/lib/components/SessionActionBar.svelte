<script>
  import { recomputeDepths, exportSession } from '../api.js';
  import { fmtN, a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let {
    session,
    stats,
    liveProgress,
    onerror,
    onstop,
    onresume,
    onretry,
    ondelete,
    onrefresh,
    oncompare,
  } = $props();

  let showExportDialog = $state(false);
  let exportIncludeHTML = $state(false);
  let showConfigModal = $state(false);
  let showActionsMenu = $state(false);

  function parsedConfig() {
    if (!session?.Config) return null;
    try {
      return JSON.parse(session.Config);
    } catch {
      return null;
    }
  }

  function fmtDuration(ns) {
    if (!ns || ns <= 0) return '0';
    const ms = ns / 1e6;
    if (ms < 1000) return `${Math.round(ms)}ms`;
    const s = ms / 1000;
    if (s < 60) return `${s}s`;
    return `${Math.floor(s / 60)}m ${Math.round(s % 60)}s`;
  }

  function handleExport() {
    exportSession(session.ID, exportIncludeHTML);
    showExportDialog = false;
    exportIncludeHTML = false;
  }

  let recomputing = $state(false);

  async function handleRecomputeDepths() {
    showActionsMenu = false;
    recomputing = true;
    try {
      await recomputeDepths(session.ID);
      onrefresh?.();
    } catch (e) {
      onerror?.(e.message);
    } finally {
      recomputing = false;
    }
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
    const end =
      session.FinishedAt && session.FinishedAt !== '1970-01-01T00:00:00Z'
        ? new Date(session.FinishedAt)
        : new Date();
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

  function closeMenu(e) {
    // Close dropdown when clicking outside
    if (showActionsMenu) {
      showActionsMenu = false;
    }
  }
</script>

<svelte:window onclick={closeMenu} />

<div class="action-bar">
  {#if session.Status === 'stopping'}
    <span class="badge badge-warning">{t('actionBar.stopping')}</span>
  {:else if session.is_running}
    {@const live = liveProgress[session.ID]}
    <span class="badge badge-info">
      {#if live && live.phase === 'fetching_sitemaps'}
        {t('common.fetchingSitemaps')}
      {:else if live && live.queue_size === 0 && live.pages_crawled > 0}
        {t('common.finalizing')}
        &middot; {fmtN(live.pages_crawled)}
        {t('common.pages')}
      {:else}
        {t('common.running')}
        {#if live}
          &middot; {fmtN(live.pages_crawled)}
          {t('common.pages')} &middot; {fmtN(live.queue_size)}
          {t('actionBar.inQueue')}
          {#if live.lost_pages > 0}
            <span class="text-error font-semibold"
              >&middot; {fmtN(live.lost_pages)} {t('sessions.lost')}</span
            >
          {/if}
        {/if}
      {/if}
    </span>
    {#if session.StartedAt && session.StartedAt !== '1970-01-01T00:00:00Z'}
      <span class="action-bar-meta"
        >{t('actionBar.started')} {fmtDate(session.StartedAt)} &middot; {elapsed()}</span
      >
    {/if}
    <button class="btn btn-sm btn-danger" onclick={() => onstop?.(session.ID)}
      >{t('common.stop')}</button
    >
  {:else}
    <span
      class="badge"
      class:badge-success={session.Status === 'completed'}
      class:badge-error={session.Status === 'failed' || session.Status === 'crashed'}
      class:badge-warning={session.Status === 'stopped' ||
        session.Status === 'completed_with_errors'}>{session.Status}</span
    >
    {#if session.StartedAt && session.StartedAt !== '1970-01-01T00:00:00Z'}
      <span class="action-bar-meta">{fmtDate(session.StartedAt)} &middot; {elapsed()}</span>
    {/if}

    <!-- Single "Actions" dropdown -->
    <div class="dropdown-wrapper">
      <button
        class="btn btn-sm"
        onclick={(e) => {
          e.stopPropagation();
          showActionsMenu = !showActionsMenu;
        }}
      >
        {t('actionBar.actions')}
        <svg
          viewBox="0 0 24 24"
          width="12"
          height="12"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"><polyline points="6 9 12 15 18 9" /></svg
        >
      </button>
      {#if showActionsMenu}
        <div class="dropdown-menu" onclick={(e) => e.stopPropagation()}>
          <button
            class="dropdown-item"
            onclick={() => {
              showActionsMenu = false;
              onresume?.(session.ID);
            }}
          >
            <svg
              viewBox="0 0 24 24"
              width="14"
              height="14"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3" /></svg
            >
            {t('sessions.resume')}
          </button>
          <button class="dropdown-item" onclick={handleRecomputeDepths} disabled={recomputing}>
            <svg
              viewBox="0 0 24 24"
              width="14"
              height="14"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              ><polyline points="23 4 23 10 17 10" /><path
                d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"
              /></svg
            >
            {recomputing ? t('actionBar.recomputing') : t('actionBar.recomputeDepths')}
          </button>
          {#if stats?.status_codes?.[0] > 0}
            <button
              class="dropdown-item"
              onclick={() => {
                showActionsMenu = false;
                onretry?.(session.ID, 0, stats.status_codes[0]);
              }}
            >
              <svg
                viewBox="0 0 24 24"
                width="14"
                height="14"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                ><polyline points="1 4 1 10 7 10" /><path
                  d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"
                /></svg
              >
              {t('actionBar.retryFailed', { count: stats.status_codes[0] })}
            </button>
          {/if}
          {#each retryableStatusCodes() as [code, count]}
            <button
              class="dropdown-item"
              onclick={() => {
                showActionsMenu = false;
                onretry?.(session.ID, +code, count);
              }}
            >
              <svg
                viewBox="0 0 24 24"
                width="14"
                height="14"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                ><polyline points="1 4 1 10 7 10" /><path
                  d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"
                /></svg
              >
              {t('actionBar.retryStatus', { count: fmtN(count), status: code })}
            </button>
          {/each}
          <div class="dropdown-divider"></div>
          <button
            class="dropdown-item"
            onclick={() => {
              showActionsMenu = false;
              showExportDialog = true;
            }}
          >
            <svg
              viewBox="0 0 24 24"
              width="14"
              height="14"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              ><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
                points="7 10 12 15 17 10"
              /><line x1="12" y1="15" x2="12" y2="3" /></svg
            >
            {t('common.export')}
          </button>
          <div class="dropdown-divider"></div>
          <button
            class="dropdown-item dropdown-item-danger"
            onclick={() => {
              showActionsMenu = false;
              ondelete?.(session.ID);
            }}
          >
            <svg
              viewBox="0 0 24 24"
              width="14"
              height="14"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              ><polyline points="3 6 5 6 21 6" /><path
                d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
              /></svg
            >
            {t('common.delete')}
          </button>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Icon buttons always visible -->
  <div class="action-bar-icons">
    <button class="btn btn-sm btn-icon" onclick={() => onrefresh?.()} title={t('common.refresh')}>
      <svg
        viewBox="0 0 24 24"
        width="14"
        height="14"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        ><polyline points="23 4 23 10 17 10" /><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" /></svg
      >
    </button>
    <button
      class="btn btn-sm btn-icon"
      onclick={() => (showConfigModal = true)}
      title={t('actionBar.showConfig')}
    >
      <svg
        viewBox="0 0 24 24"
        width="14"
        height="14"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        ><circle cx="12" cy="12" r="3" /><path
          d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
        /></svg
      >
    </button>
    <button
      class="btn btn-sm btn-icon"
      onclick={() => oncompare?.(session.ID)}
      title={t('actionBar.compareWith')}
    >
      <svg
        viewBox="0 0 24 24"
        width="14"
        height="14"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        ><line x1="18" y1="20" x2="18" y2="10" /><line x1="12" y1="20" x2="12" y2="4" /><line
          x1="6"
          y1="20"
          x2="6"
          y2="14"
        /></svg
      >
    </button>
  </div>
</div>

{#if showExportDialog}
  <div
    class="html-modal-overlay"
    role="button"
    tabindex="0"
    onclick={() => (showExportDialog = false)}
    onkeydown={a11yKeydown(() => (showExportDialog = false))}
  >
    <div class="html-modal export-modal" role="dialog" onclick={(e) => e.stopPropagation()}>
      <div class="html-modal-header">
        <div class="html-modal-url">{t('actionBar.exportSession')}</div>
        <div class="html-modal-actions">
          <button
            class="btn btn-sm"
            title={t('common.close')}
            onclick={() => (showExportDialog = false)}
          >
            <svg
              viewBox="0 0 24 24"
              width="16"
              height="16"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              ><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg
            >
          </button>
        </div>
      </div>
      <div class="export-body">
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={exportIncludeHTML} />
          {t('actionBar.includeHtml')}
        </label>
        <div class="export-actions">
          <button class="btn btn-sm" onclick={() => (showExportDialog = false)}
            >{t('common.cancel')}</button
          >
          <button class="btn btn-sm btn-primary" onclick={handleExport}
            >{t('actionBar.downloadJsonl')}</button
          >
        </div>
      </div>
    </div>
  </div>
{/if}

{#if showConfigModal}
  {@const cfg = parsedConfig()}
  <div
    class="html-modal-overlay"
    role="button"
    tabindex="0"
    onclick={() => (showConfigModal = false)}
    onkeydown={a11yKeydown(() => (showConfigModal = false))}
  >
    <div class="html-modal config-modal" role="dialog" onclick={(e) => e.stopPropagation()}>
      <div class="html-modal-header">
        <div class="html-modal-url">{t('actionBar.crawlConfig')}</div>
        <div class="html-modal-actions">
          <button
            class="btn btn-sm"
            title={t('common.close')}
            onclick={() => (showConfigModal = false)}
          >
            <svg
              viewBox="0 0 24 24"
              width="16"
              height="16"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              ><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg
            >
          </button>
        </div>
      </div>
      <div class="config-body">
        {#if cfg?.Crawler}
          <table class="config-table">
            <tbody>
              <tr
                ><td class="config-key">{t('actionBar.cfgWorkers')}</td><td
                  >{cfg.Crawler.Workers}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgDelay')}</td><td
                  >{fmtDuration(cfg.Crawler.Delay)}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgMaxPages')}</td><td
                  >{cfg.Crawler.MaxPages || '∞'}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgMaxDepth')}</td><td
                  >{cfg.Crawler.MaxDepth || '∞'}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgScope')}</td><td
                  >{cfg.Crawler.CrawlScope}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgUserAgent')}</td><td class="config-ua"
                  >{cfg.Crawler.UserAgent}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgTLS')}</td><td
                  >{cfg.Crawler.TLSProfile || 'default'}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgRobots')}</td><td
                  >{cfg.Crawler.RespectRobots ? t('common.yes') : t('common.no')}</td
                ></tr
              >
              <tr
                ><td class="config-key">{t('actionBar.cfgStoreHTML')}</td><td
                  >{cfg.Crawler.StoreHTML ? t('common.yes') : t('common.no')}</td
                ></tr
              >
              {#if cfg.Crawler.SourceIP}
                <tr
                  ><td class="config-key">{t('actionBar.cfgSourceIP')}</td><td
                    >{cfg.Crawler.SourceIP}</td
                  ></tr
                >
              {/if}
              {#if cfg.Crawler.ForceIPv4}
                <tr><td class="config-key">IPv4</td><td>{t('common.yes')}</td></tr>
              {/if}
              {#if cfg.Crawler.JSRender?.Mode && cfg.Crawler.JSRender.Mode !== 'off'}
                <tr
                  ><td class="config-key">JS Render</td><td
                    >{cfg.Crawler.JSRender.Mode} ({cfg.Crawler.JSRender.MaxPages} pages)</td
                  ></tr
                >
              {/if}
              {#if cfg.Crawler.ExcludePatterns?.length}
                <tr>
                  <td class="config-key">{t('actionBar.cfgExclude')}</td>
                  <td
                    ><code class="config-patterns">{cfg.Crawler.ExcludePatterns.join('\n')}</code
                    ></td
                  >
                </tr>
              {/if}
            </tbody>
          </table>
        {:else}
          <p class="text-muted">{t('actionBar.noConfig')}</p>
        {/if}
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
  .action-bar-icons {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-left: auto;
  }
  .btn-icon {
    padding: 4px 6px;
    min-width: unset;
  }

  /* Dropdown */
  .dropdown-wrapper {
    position: relative;
  }
  .dropdown-wrapper > .btn svg {
    margin-left: 4px;
    vertical-align: middle;
  }
  .dropdown-menu {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 100;
    min-width: 220px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
    padding: 4px 0;
  }
  .dropdown-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 12px;
    font-size: 13px;
    color: var(--text-primary);
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
  }
  .dropdown-item:hover {
    background: var(--bg-hover);
  }
  .dropdown-item:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .dropdown-item svg {
    flex-shrink: 0;
    color: var(--text-muted);
  }
  .dropdown-item-danger {
    color: var(--color-error, #ef4444);
  }
  .dropdown-item-danger svg {
    color: var(--color-error, #ef4444);
  }
  .dropdown-divider {
    height: 1px;
    background: var(--border);
    margin: 4px 0;
  }

  /* Modals */
  .html-modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
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
  .export-modal {
    max-width: 400px;
    height: auto;
  }
  .export-body {
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
  }
  .export-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .config-modal {
    max-width: 520px;
    height: auto;
  }
  .config-body {
    padding: 16px 20px;
  }
  .config-table {
    width: 100%;
    border-collapse: collapse;
  }
  .config-table td {
    padding: 6px 0;
    font-size: 13px;
    border-bottom: 1px solid var(--border);
    vertical-align: top;
  }
  .config-table tr:last-child td {
    border-bottom: none;
  }
  .config-key {
    color: var(--text-muted);
    white-space: nowrap;
    padding-right: 16px;
    width: 1%;
  }
  .config-ua {
    word-break: break-all;
    font-size: 12px;
  }
  .config-patterns {
    white-space: pre-wrap;
    font-size: 12px;
    background: var(--bg-input);
    padding: 4px 8px;
    border-radius: var(--radius-sm);
    display: block;
  }
</style>
