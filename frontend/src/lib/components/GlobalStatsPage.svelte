<script>
  import { getGlobalStats } from '../api.js';
  import { fmtN, fmtSize } from '../utils.js';

  let { onerror } = $props();

  let globalStats = $state(null);
  let globalStatsLoading = $state(false);

  async function loadData() {
    globalStatsLoading = true;
    try {
      globalStats = await getGlobalStats();
    } catch (e) { onerror?.(e.message); }
    globalStatsLoading = false;
  }

  loadData();
</script>

<div class="page-header">
  <h1>Global Stats</h1>
  <button class="btn btn-sm" onclick={loadData} disabled={globalStatsLoading}>
    {globalStatsLoading ? 'Loading...' : 'Refresh'}
  </button>
</div>

{#if globalStatsLoading && !globalStats}
  <div class="loading">Loading global stats...</div>
{:else if globalStats}
  <div class="stats-grid" style="margin-bottom: 24px;">
    <div class="stat-card"><div class="stat-value">{fmtN(globalStats.total_pages)}</div><div class="stat-label">Total Pages</div></div>
    <div class="stat-card"><div class="stat-value">{fmtN(globalStats.total_links)}</div><div class="stat-label">Total Links</div></div>
    <div class="stat-card"><div class="stat-value">{fmtN(globalStats.total_errors)}</div><div class="stat-label">Total Errors</div></div>
    <div class="stat-card"><div class="stat-value">{globalStats.avg_fetch_ms?.toFixed(0) || 0}ms</div><div class="stat-label">Avg Response</div></div>
    <div class="stat-card"><div class="stat-value">{fmtSize(globalStats.total_storage)}</div><div class="stat-label">Total Storage</div></div>
    <div class="stat-card"><div class="stat-value">{fmtN(globalStats.total_sessions)}</div><div class="stat-label">Sessions</div></div>
  </div>

  {#if globalStats.projects?.length > 0}
    <div class="card" style="overflow-x: auto;">
      <h3 style="margin: 0 0 16px 0; font-size: 1rem;">Stats by Project</h3>
      <table class="data-table">
        <thead>
          <tr>
            <th>Project</th>
            <th style="text-align: right;">Sessions</th>
            <th style="text-align: right;">Pages</th>
            <th style="text-align: right;">Links</th>
            <th style="text-align: right;">Errors</th>
            <th style="text-align: right;">Avg Response</th>
            <th style="text-align: right;">Storage</th>
            <th style="min-width: 120px;">Proportion</th>
          </tr>
        </thead>
        <tbody>
          {#each [...globalStats.projects].sort((a, b) => b.storage_bytes - a.storage_bytes) as p}
            <tr>
              <td><strong>{p.project_name}</strong></td>
              <td style="text-align: right;">{fmtN(p.sessions)}</td>
              <td style="text-align: right;">{fmtN(p.total_pages)}</td>
              <td style="text-align: right;">{fmtN(p.total_links)}</td>
              <td style="text-align: right;">{fmtN(p.error_count)}</td>
              <td style="text-align: right;">{p.avg_fetch_ms?.toFixed(0) || 0}ms</td>
              <td style="text-align: right;">{fmtSize(p.storage_bytes)}</td>
              <td>
                <div style="background: var(--bg-secondary); border-radius: 4px; height: 8px; overflow: hidden;">
                  <div style="background: var(--accent); height: 100%; width: {globalStats.total_storage > 0 ? (p.storage_bytes / globalStats.total_storage * 100) : 0}%; border-radius: 4px; transition: width 0.3s;"></div>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if globalStats.storage_tables?.length > 0}
    <div class="card" style="margin-top: 16px;">
      <h3 style="margin: 0 0 16px 0; font-size: 1rem;">Storage by Table</h3>
      <table class="data-table">
        <thead>
          <tr>
            <th>Table</th>
            <th style="text-align: right;">Rows</th>
            <th style="text-align: right;">Disk Usage</th>
          </tr>
        </thead>
        <tbody>
          {#each globalStats.storage_tables as t}
            <tr>
              <td><code>{t.name}</code></td>
              <td style="text-align: right;">{fmtN(t.rows)}</td>
              <td style="text-align: right;">{fmtSize(t.bytes_on_disk)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
{/if}
