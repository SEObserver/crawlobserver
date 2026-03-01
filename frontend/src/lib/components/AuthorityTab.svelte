<script>
  import { getSessionAuthority } from '../api.js';
  import { fmtN } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  import { getProviderStatus } from '../api.js';

  let { sessionId, projectId, onerror, onnavigate } = $props();

  let providerConnected = $state(false);

  let loading = $state(false);
  let rows = $state(null);
  let total = $state(0);
  let offset = $state(0);
  const PAGE_LIMIT = 100;

  async function loadData() {
    if (!sessionId || !projectId || !providerConnected) return;
    loading = true;
    try {
      const res = await getSessionAuthority(sessionId, projectId, PAGE_LIMIT, offset);
      rows = res?.rows ?? [];
      total = res?.total ?? 0;
    } catch (e) {
      onerror?.(e.message);
      rows = [];
      total = 0;
    } finally {
      loading = false;
    }
  }

  async function checkProvider() {
    if (!projectId) return;
    try {
      const s = await getProviderStatus(projectId, 'seobserver');
      providerConnected = !!s?.connected;
    } catch {
      providerConnected = false;
    }
  }

  $effect(() => {
    checkProvider();
  });

  $effect(() => {
    if (providerConnected && sessionId && projectId) {
      loadData();
    }
  });

  function tfCfBadgeClass(val) {
    if (val > 40) return 'badge-green';
    if (val >= 20) return 'badge-yellow';
    return 'badge-red';
  }

  function mismatchIndicator(tf, pagerank) {
    const prScaled = pagerank * 100;
    const diff = tf - prScaled;
    if (diff > 5) return 'up';
    if (diff < -5) return 'down';
    return 'neutral';
  }

  const PLACEHOLDER_ROWS = [
    { url: 'https://example.com/page-one', pagerank: 0.004231, tf: 42, cf: 38, ref_domains: 1240 },
    { url: 'https://example.com/blog/article', pagerank: 0.003102, tf: 35, cf: 29, ref_domains: 870 },
    { url: 'https://example.com/products/item', pagerank: 0.001894, tf: 18, cf: 44, ref_domains: 520 },
    { url: 'https://example.com/about', pagerank: 0.001250, tf: 25, cf: 21, ref_domains: 310 },
    { url: 'https://example.com/contact', pagerank: 0.000780, tf: 12, cf: 15, ref_domains: 95 },
  ];
</script>

<div class="auth-container">
  {#if !providerConnected}
    <!-- Disconnected: CTA + blurred preview -->
    <div class="auth-empty">
      <div class="auth-lock-icon">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
          <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
        </svg>
      </div>
      <p class="auth-cta-text">{t('authority.connectCta')}</p>
      <button class="btn btn-primary" onclick={() => onnavigate?.()}>
        {t('authority.connectBtn')}
      </button>
    </div>

    <!-- Blurred preview table -->
    <div class="auth-preview-wrapper">
      <div class="auth-preview-blur">
        <table>
          <thead>
            <tr>
              <th>#</th>
              <th>{t('authority.url')}</th>
              <th>{t('authority.pagerank')}</th>
              <th>{t('authority.trustFlow')}</th>
              <th>{t('authority.citationFlow')}</th>
              <th>{t('authority.refDomains')}</th>
              <th>{t('authority.mismatch')}</th>
            </tr>
          </thead>
          <tbody>
            {#each PLACEHOLDER_ROWS as r, i}
              {@const m = mismatchIndicator(r.tf, r.pagerank)}
              <tr>
                <td class="row-num">{i + 1}</td>
                <td class="cell-url auth-cell-url">{r.url}</td>
                <td>{r.pagerank.toFixed(6)}</td>
                <td><span class="auth-badge {tfCfBadgeClass(r.tf)}">{r.tf}</span></td>
                <td><span class="auth-badge {tfCfBadgeClass(r.cf)}">{r.cf}</span></td>
                <td>{fmtN(r.ref_domains)}</td>
                <td class="auth-mismatch-cell">
                  {#if m === 'up'}
                    <span class="auth-arrow auth-arrow-up" title="TF > PR">&#9650;</span>
                  {:else if m === 'down'}
                    <span class="auth-arrow auth-arrow-down" title="TF < PR">&#9660;</span>
                  {:else}
                    <span class="auth-arrow auth-arrow-neutral">&mdash;</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

  {:else if loading}
    <p class="loading-msg">{t('common.loading')}</p>

  {:else if !rows || rows.length === 0}
    <!-- Connected but no data -->
    <div class="auth-empty">
      <p class="auth-cta-text">{t('authority.fetchCta')}</p>
      <button class="btn btn-primary" onclick={() => onnavigate?.()}>
        {t('authority.fetchBtn')}
      </button>
    </div>

  {:else}
    <!-- Data table -->
    <div class="table-meta">{t('authority.title')} &mdash; {fmtN(total)} rows</div>
    <table>
      <thead>
        <tr>
          <th>#</th>
          <th>{t('authority.url')}</th>
          <th>{t('authority.pagerank')}</th>
          <th>{t('authority.trustFlow')}</th>
          <th>{t('authority.citationFlow')}</th>
          <th>{t('authority.refDomains')}</th>
          <th>{t('authority.mismatch')}</th>
        </tr>
      </thead>
      <tbody>
        {#each rows as r, i}
          {@const m = mismatchIndicator(r.trust_flow ?? 0, r.pagerank ?? 0)}
          <tr>
            <td class="row-num">{offset + i + 1}</td>
            <td class="cell-url auth-cell-url">
              <a href={r.url} target="_blank" rel="noopener">{r.url}</a>
            </td>
            <td class="auth-pr-cell">{(r.pagerank ?? 0).toFixed(6)}</td>
            <td><span class="auth-badge {tfCfBadgeClass(r.trust_flow ?? 0)}">{r.trust_flow ?? 0}</span></td>
            <td><span class="auth-badge {tfCfBadgeClass(r.citation_flow ?? 0)}">{r.citation_flow ?? 0}</span></td>
            <td>{fmtN(r.ref_domains ?? 0)}</td>
            <td class="auth-mismatch-cell">
              {#if m === 'up'}
                <span class="auth-arrow auth-arrow-up" title="TF > PR">&#9650;</span>
              {:else if m === 'down'}
                <span class="auth-arrow auth-arrow-down" title="TF < PR">&#9660;</span>
              {:else}
                <span class="auth-arrow auth-arrow-neutral">&mdash;</span>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>

    {#if total > PAGE_LIMIT}
      <div class="pagination">
        <button class="btn btn-sm" disabled={offset === 0} onclick={() => { offset = Math.max(0, offset - PAGE_LIMIT); loadData(); }}>
          {t('common.previous')}
        </button>
        <span class="pagination-info">
          {offset + 1} - {Math.min(offset + PAGE_LIMIT, total)} of {fmtN(total)}
        </span>
        <button class="btn btn-sm" disabled={offset + PAGE_LIMIT >= total} onclick={() => { offset += PAGE_LIMIT; loadData(); }}>
          {t('common.next')}
        </button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .auth-container {
    width: 100%;
  }
  .auth-empty {
    text-align: center;
    padding: 48px 20px 24px;
    color: var(--text-primary);
  }
  .auth-lock-icon {
    color: var(--text-muted);
    margin-bottom: 16px;
  }
  .auth-cta-text {
    max-width: 460px;
    margin: 0 auto 20px;
    font-size: 14px;
    color: var(--text-secondary);
    line-height: 1.5;
  }
  .btn-primary {
    background: var(--accent);
    color: white;
    border: none;
    padding: 8px 20px;
    border-radius: 6px;
    cursor: pointer;
    font-weight: 600;
  }
  .btn-primary:hover { opacity: 0.9; }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

  /* Blurred preview */
  .auth-preview-wrapper {
    margin-top: 24px;
    position: relative;
    overflow: hidden;
    border-radius: 8px;
  }
  .auth-preview-blur {
    filter: blur(4px);
    opacity: 0.45;
    pointer-events: none;
    user-select: none;
  }

  /* Table cells */
  .auth-cell-url {
    max-width: 320px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .auth-pr-cell {
    font-variant-numeric: tabular-nums;
    font-family: var(--font-mono, monospace);
    font-size: 12px;
  }

  /* TF/CF badges */
  .auth-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 600;
    min-width: 32px;
    text-align: center;
  }
  .badge-green {
    background: #22c55e22;
    color: #16a34a;
  }
  .badge-yellow {
    background: #f59e0b22;
    color: #d97706;
  }
  .badge-red {
    background: #ef444422;
    color: #dc2626;
  }

  /* Mismatch arrows */
  .auth-mismatch-cell {
    text-align: center;
  }
  .auth-arrow {
    font-size: 14px;
  }
  .auth-arrow-up {
    color: #16a34a;
  }
  .auth-arrow-down {
    color: #dc2626;
  }
  .auth-arrow-neutral {
    color: var(--text-muted);
  }
</style>
