<script>
  import { getGlobalStats, deleteUnassignedSessions } from '../api.js';
  import { fmtN, fmtSize } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import ConfirmModal from './ConfirmModal.svelte';

  let { onerror } = $props();

  let globalStats = $state(null);
  let globalStatsLoading = $state(false);
  let confirmState = $state(null);
  let deletingUnassigned = $state(false);

  async function loadData() {
    globalStatsLoading = true;
    try {
      globalStats = await getGlobalStats();
    } catch (e) {
      onerror?.(e.message);
    }
    globalStatsLoading = false;
  }

  function handleDeleteUnassigned() {
    confirmState = {
      message: t('stats.confirmDeleteUnassigned'),
      danger: true,
      confirmLabel: t('common.delete'),
      onConfirm: async () => {
        deletingUnassigned = true;
        try {
          await deleteUnassignedSessions();
          await loadData();
        } catch (e) {
          onerror?.(e.message);
        }
        deletingUnassigned = false;
      },
    };
  }

  loadData();
</script>

<div class="page-header">
  <h1>{t('stats.title')}</h1>
  <button class="btn btn-sm" onclick={loadData} disabled={globalStatsLoading}>
    {globalStatsLoading ? t('common.loading') : t('common.refresh')}
  </button>
</div>

{#if globalStatsLoading && !globalStats}
  <div class="loading">{t('stats.loadingStats')}</div>
{:else if globalStats}
  <div class="stats-grid mb-lg">
    <div class="stat-card">
      <div class="stat-value">{fmtN(globalStats.total_pages)}</div>
      <div class="stat-label">{t('stats.totalPages')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{fmtN(globalStats.total_links)}</div>
      <div class="stat-label">{t('stats.totalLinks')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{fmtN(globalStats.total_errors)}</div>
      <div class="stat-label">{t('stats.totalErrors')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{globalStats.avg_fetch_ms?.toFixed(0) || 0}ms</div>
      <div class="stat-label">{t('stats.avgResponse')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{fmtSize(globalStats.total_storage)}</div>
      <div class="stat-label">{t('stats.totalStorage')}</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">{fmtN(globalStats.total_sessions)}</div>
      <div class="stat-label">{t('stats.sessions')}</div>
    </div>
  </div>

  {#if globalStats.projects?.length > 0}
    <div class="card overflow-auto">
      <h3 class="card-heading">{t('stats.byProject')}</h3>
      <table class="data-table">
        <thead>
          <tr>
            <th>{t('stats.project')}</th>
            <th class="text-right">{t('stats.sessions')}</th>
            <th class="text-right">{t('common.pages')}</th>
            <th class="text-right">{t('stats.links')}</th>
            <th class="text-right">{t('stats.errors')}</th>
            <th class="text-right">{t('stats.avgResponse')}</th>
            <th class="text-right">{t('stats.storage')}</th>
            <th class="col-proportion">{t('stats.proportion')}</th>
          </tr>
        </thead>
        <tbody>
          {#each [...globalStats.projects].sort((a, b) => b.storage_bytes - a.storage_bytes) as p}
            <tr>
              <td>
                <strong>{p.project_name}</strong>
                {#if !p.project_id}
                  <button
                    class="btn btn-sm btn-danger btn-inline-delete"
                    onclick={handleDeleteUnassigned}
                    disabled={deletingUnassigned}
                    title={t('stats.deleteUnassigned')}
                  >
                    <svg
                      viewBox="0 0 24 24"
                      width="12"
                      height="12"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      ><polyline points="3 6 5 6 21 6" /><path
                        d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
                      /></svg
                    >
                  </button>
                {/if}
              </td>
              <td class="text-right">{fmtN(p.sessions)}</td>
              <td class="text-right">{fmtN(p.total_pages)}</td>
              <td class="text-right">{fmtN(p.total_links)}</td>
              <td class="text-right">{fmtN(p.error_count)}</td>
              <td class="text-right">{p.avg_fetch_ms?.toFixed(0) || 0}ms</td>
              <td class="text-right">{fmtSize(p.storage_bytes)}</td>
              <td>
                <div class="progress-track">
                  <div
                    class="progress-fill"
                    style="width: {globalStats.total_storage > 0
                      ? (p.storage_bytes / globalStats.total_storage) * 100
                      : 0}%;"
                  ></div>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if globalStats.storage_tables?.length > 0}
    <div class="card mt-md">
      <h3 class="card-heading">{t('stats.storageByTable')}</h3>
      <table class="data-table">
        <thead>
          <tr>
            <th>{t('stats.table')}</th>
            <th class="text-right">{t('stats.rows')}</th>
            <th class="text-right">{t('stats.diskUsage')}</th>
          </tr>
        </thead>
        <tbody>
          {#each globalStats.storage_tables as tbl}
            <tr>
              <td><code>{tbl.name}</code></td>
              <td class="text-right">{fmtN(tbl.rows)}</td>
              <td class="text-right">{fmtSize(tbl.bytes_on_disk)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
{/if}

{#if confirmState}
  <ConfirmModal
    message={confirmState.message}
    danger={confirmState.danger}
    confirmLabel={confirmState.confirmLabel}
    onconfirm={() => {
      confirmState.onConfirm();
      confirmState = null;
    }}
    oncancel={() => (confirmState = null)}
  />
{/if}

<style>
  .card-heading {
    margin: 0 0 16px 0;
    font-size: 1rem;
  }
  .col-proportion {
    min-width: 120px;
  }
  .progress-track {
    background: var(--bg-secondary);
    border-radius: 4px;
    height: 8px;
    overflow: hidden;
  }
  .progress-fill {
    background: var(--accent);
    height: 100%;
    border-radius: 4px;
    transition: width 0.3s;
  }
  .btn-inline-delete {
    margin-left: 8px;
    padding: 2px 6px;
    vertical-align: middle;
  }
</style>
