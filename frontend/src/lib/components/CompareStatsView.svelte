<script>
  import { fmtN, fmt } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { compareStats } = $props();

  let sa = $derived(compareStats.stats_a);
  let sb = $derived(compareStats.stats_b);

  function delta(a, b) {
    const d = b - a;
    if (d === 0) return '';
    return d > 0 ? `+${fmtN(d)}` : fmtN(d);
  }

  function deltaClass(a, b) {
    const d = b - a;
    if (d > 0) return 'delta-up';
    if (d < 0) return 'delta-down';
    return '';
  }
</script>

<div class="compare-stats-grid">
  <div class="compare-stat-card">
    <div class="compare-stat-label">{t('compare.totalPages')}</div>
    <div class="compare-stat-values">
      <span class="val-a">{fmtN(sa.total_pages)}</span>
      <span class="val-arrow">→</span>
      <span class="val-b">{fmtN(sb.total_pages)}</span>
    </div>
    <div class="compare-delta {deltaClass(sa.total_pages, sb.total_pages)}">{delta(sa.total_pages, sb.total_pages)}</div>
  </div>
  <div class="compare-stat-card">
    <div class="compare-stat-label">{t('compare.internalLinks')}</div>
    <div class="compare-stat-values">
      <span class="val-a">{fmtN(sa.internal_links)}</span>
      <span class="val-arrow">→</span>
      <span class="val-b">{fmtN(sb.internal_links)}</span>
    </div>
    <div class="compare-delta {deltaClass(sa.internal_links, sb.internal_links)}">{delta(sa.internal_links, sb.internal_links)}</div>
  </div>
  <div class="compare-stat-card">
    <div class="compare-stat-label">{t('session.externalLinks')}</div>
    <div class="compare-stat-values">
      <span class="val-a">{fmtN(sa.external_links)}</span>
      <span class="val-arrow">→</span>
      <span class="val-b">{fmtN(sb.external_links)}</span>
    </div>
    <div class="compare-delta {deltaClass(sa.external_links, sb.external_links)}">{delta(sa.external_links, sb.external_links)}</div>
  </div>
  <div class="compare-stat-card">
    <div class="compare-stat-label">{t('compare.errors')}</div>
    <div class="compare-stat-values">
      <span class="val-a">{fmtN(sa.error_count)}</span>
      <span class="val-arrow">→</span>
      <span class="val-b">{fmtN(sb.error_count)}</span>
    </div>
    <div class="compare-delta {deltaClass(sa.error_count, sb.error_count)}">{delta(sa.error_count, sb.error_count)}</div>
  </div>
  <div class="compare-stat-card">
    <div class="compare-stat-label">{t('compare.avgResponse')}</div>
    <div class="compare-stat-values">
      <span class="val-a">{fmt(Math.round(sa.avg_fetch_ms))}</span>
      <span class="val-arrow">→</span>
      <span class="val-b">{fmt(Math.round(sb.avg_fetch_ms))}</span>
    </div>
    <div class="compare-delta {deltaClass(sa.avg_fetch_ms, sb.avg_fetch_ms)}">{delta(Math.round(sa.avg_fetch_ms), Math.round(sb.avg_fetch_ms))}</div>
  </div>
  <div class="compare-stat-card">
    <div class="compare-stat-label">{t('compare.pagesPerSec')}</div>
    <div class="compare-stat-values">
      <span class="val-a">{sa.pages_per_second?.toFixed(1) || '0'}</span>
      <span class="val-arrow">→</span>
      <span class="val-b">{sb.pages_per_second?.toFixed(1) || '0'}</span>
    </div>
  </div>
</div>

{#if sa.status_codes || sb.status_codes}
  <h3 class="status-codes-heading">{t('compare.statusCodes')}</h3>
  {@const allCodes = [...new Set([...Object.keys(sa.status_codes || {}), ...Object.keys(sb.status_codes || {})])].sort()}
  <table class="table">
    <thead><tr><th>{t('compare.code')}</th><th>{t('compare.sessionA')}</th><th>{t('compare.sessionB')}</th><th>{t('compare.delta')}</th></tr></thead>
    <tbody>
      {#each allCodes as code}
        {@const countA = (sa.status_codes || {})[code] || 0}
        {@const countB = (sb.status_codes || {})[code] || 0}
        <tr>
          <td><span class="badge">{code}</span></td>
          <td>{fmtN(countA)}</td>
          <td>{fmtN(countB)}</td>
          <td class={deltaClass(countA, countB)}>{delta(countA, countB)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  .compare-stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 12px;
    padding: 16px;
  }
  .compare-stat-card {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 14px;
    text-align: center;
  }
  .compare-stat-label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--text-secondary);
    margin-bottom: 8px;
  }
  .compare-stat-values {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    font-size: 18px;
    font-weight: 600;
  }
  .val-a { color: var(--text-secondary); }
  .val-arrow { color: var(--text-secondary); font-size: 14px; opacity: 0.5; }
  .val-b { color: var(--text); }
  .compare-delta {
    font-size: 13px;
    font-weight: 500;
    margin-top: 4px;
  }
  .delta-up { color: var(--success, #22c55e); }
  .delta-down { color: var(--error, #ef4444); }
  .status-codes-heading { margin: 20px 0 12px; padding: 0 16px; font-size: 14px; font-weight: 600; color: var(--text); }
  .table { width: 100%; border-collapse: collapse; font-size: 13px; }
  .table th { text-align: left; padding: 8px 12px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--text-secondary); border-bottom: 1px solid var(--border); white-space: nowrap; }
  .table td { padding: 6px 12px; border-bottom: 1px solid var(--border); color: var(--text); }
</style>
