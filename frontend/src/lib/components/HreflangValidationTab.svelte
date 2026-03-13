<script>
  import { getHreflangValidation, computeHreflangValidation, buildApiPath } from '../api.js';
  import { fetchAll, downloadCSV } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import ExportDropdown from './ExportDropdown.svelte';

  let { sessionId, onerror, onnavigate } = $props();

  let issues = $state([]);
  let total = $state(0);
  let summary = $state({});
  let loading = $state(false);
  let computing = $state(false);
  let offset = $state(0);
  let hasMore = $state(false);
  let issueType = $state('');
  const PAGE_SIZE = 50;

  const ISSUE_TYPES = [
    { id: '', label: () => t('hreflang.filterAll') },
    { id: 'missing_reciprocal', label: () => t('hreflang.missingReciprocal') },
    { id: 'missing_self_ref', label: () => t('hreflang.missingSelfRef') },
    { id: 'xdefault_is_lang_page', label: () => t('hreflang.xdefaultIsLang') },
    { id: 'target_not_crawled', label: () => t('hreflang.targetNotCrawled') },
    { id: 'inconsistent_cluster', label: () => t('hreflang.inconsistentCluster') },
  ];

  function issueClass(type) {
    switch (type) {
      case 'missing_reciprocal':
      case 'xdefault_is_lang_page':
        return 'issue-error';
      case 'missing_self_ref':
      case 'inconsistent_cluster':
        return 'issue-warning';
      case 'target_not_crawled':
        return 'issue-info';
      default:
        return '';
    }
  }

  function issueLabel(type) {
    switch (type) {
      case 'missing_reciprocal':
        return t('hreflang.missingReciprocal');
      case 'missing_self_ref':
        return t('hreflang.missingSelfRef');
      case 'xdefault_is_lang_page':
        return t('hreflang.xdefaultIsLang');
      case 'target_not_crawled':
        return t('hreflang.targetNotCrawled');
      case 'inconsistent_cluster':
        return t('hreflang.inconsistentCluster');
      default:
        return type;
    }
  }

  let totalIssues = $derived(Object.values(summary).reduce((a, b) => a + b, 0));
  let errorCount = $derived(
    (summary['missing_reciprocal'] || 0) +
      (summary['xdefault_is_lang_page'] || 0) +
      (summary['missing_self_ref'] || 0),
  );
  let warningCount = $derived(
    (summary['target_not_crawled'] || 0) + (summary['inconsistent_cluster'] || 0),
  );

  async function loadData() {
    loading = true;
    try {
      const result = await getHreflangValidation(sessionId, PAGE_SIZE, offset, issueType);
      issues = result?.issues || [];
      total = result?.total || 0;
      summary = result?.summary || {};
      hasMore = issues.length === PAGE_SIZE;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function handleCompute() {
    computing = true;
    try {
      await computeHreflangValidation(sessionId);
      // Wait a bit for async computation, then reload
      setTimeout(() => {
        loadData();
        computing = false;
      }, 2000);
    } catch (e) {
      onerror?.(e.message);
      computing = false;
    }
  }

  function selectType(type) {
    issueType = type;
    offset = 0;
    loadData();
  }

  let exporting = $state(false);

  async function handleExportCSV() {
    if (exporting) return;
    exporting = true;
    try {
      const allData = await fetchAll(
        (limit, off) =>
          getHreflangValidation(sessionId, limit, off, issueType).then((r) => r?.issues || []),
        PAGE_SIZE,
      );
      downloadCSV(
        'hreflang-issues.csv',
        [
          t('hreflang.colType'),
          t('hreflang.colSourceUrl'),
          t('hreflang.colSourceLang'),
          t('hreflang.colTargetUrl'),
          t('hreflang.colTargetLang'),
          t('hreflang.colDetail'),
        ],
        ['issue_type', 'source_url', 'source_lang', 'target_url', 'target_lang', 'detail'],
        allData,
      );
    } finally {
      exporting = false;
    }
  }

  let apiPath = $derived(
    buildApiPath(`/sessions/${sessionId}/hreflang-validation`, {
      limit: PAGE_SIZE,
      offset: 0,
      ...(issueType ? { issue_type: issueType } : {}),
    }),
  );

  $effect(() => {
    if (sessionId) loadData();
  });
</script>

<div class="hl-tab">
  <div class="hl-header">
    <h3>{t('hreflang.title')}</h3>
    <button class="btn btn-sm" onclick={handleCompute} disabled={computing}>
      {computing ? t('common.loading') : t('hreflang.compute')}
    </button>
    {#if totalIssues > 0}
      <div class="hl-export">
        <ExportDropdown onexportcsv={handleExportCSV} {exporting} {apiPath} />
      </div>
    {/if}
  </div>

  {#if loading}
    <div class="hl-empty">{t('common.loading')}</div>
  {:else if totalIssues === 0}
    <div class="hl-empty">{t('hreflang.noIssues')}</div>
  {:else}
    <!-- Summary chips -->
    <div class="hl-summary">
      {#if errorCount > 0}
        <span class="hl-chip hl-chip-error">{errorCount} {t('hreflang.errors')}</span>
      {/if}
      {#if warningCount > 0}
        <span class="hl-chip hl-chip-warning">{warningCount} {t('hreflang.warnings')}</span>
      {/if}
      {#each Object.entries(summary) as [type, count]}
        <span class="hl-chip-detail {issueClass(type)}">{issueLabel(type)}: {count}</span>
      {/each}
    </div>

    <!-- Filter tabs -->
    <div class="hl-filters">
      {#each ISSUE_TYPES as it}
        <button
          class="hl-filter"
          class:active={issueType === it.id}
          onclick={() => selectType(it.id)}
        >
          {it.label()}
          {it.id ? `(${summary[it.id] || 0})` : `(${totalIssues})`}
        </button>
      {/each}
    </div>

    {#if issues.length === 0}
      <div class="hl-empty">{t('hreflang.noIssues')}</div>
    {:else}
      <table class="hl-table">
        <thead>
          <tr>
            <th>{t('hreflang.colType')}</th>
            <th>{t('hreflang.colSourceUrl')}</th>
            <th>{t('hreflang.colSourceLang')}</th>
            <th>{t('hreflang.colTargetUrl')}</th>
            <th>{t('hreflang.colTargetLang')}</th>
            <th>{t('hreflang.colDetail')}</th>
          </tr>
        </thead>
        <tbody>
          {#each issues as issue}
            <tr>
              <td>
                <span class="issue-badge {issueClass(issue.issue_type)}">
                  {issueLabel(issue.issue_type)}
                </span>
              </td>
              <td class="cell-url">
                <a
                  href={`/sessions/${sessionId}/url/${encodeURIComponent(issue.source_url)}`}
                  onclick={(e) => {
                    e.preventDefault();
                    onnavigate?.(
                      `/sessions/${sessionId}/url/${encodeURIComponent(issue.source_url)}`,
                    );
                  }}>{issue.source_url}</a
                >
              </td>
              <td>{issue.source_lang}</td>
              <td class="cell-url">
                {#if issue.target_url}
                  <a
                    href={`/sessions/${sessionId}/url/${encodeURIComponent(issue.target_url)}`}
                    onclick={(e) => {
                      e.preventDefault();
                      onnavigate?.(
                        `/sessions/${sessionId}/url/${encodeURIComponent(issue.target_url)}`,
                      );
                    }}>{issue.target_url}</a
                  >
                {:else}
                  -
                {/if}
              </td>
              <td>{issue.target_lang}</td>
              <td class="cell-detail">{issue.detail}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}

    <div class="hl-pagination">
      <button
        disabled={offset === 0}
        onclick={() => {
          offset = Math.max(0, offset - PAGE_SIZE);
          loadData();
        }}>{t('common.previous')}</button
      >
      <span>{offset + 1} - {offset + issues.length} / {total}</span>
      <button
        disabled={!hasMore}
        onclick={() => {
          offset += PAGE_SIZE;
          loadData();
        }}>{t('common.next')}</button
      >
    </div>
  {/if}
</div>

<style>
  .hl-tab {
    padding: 16px;
  }
  .hl-header {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 12px;
  }
  .hl-header h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
  .hl-empty {
    padding: 32px;
    text-align: center;
    color: var(--text-secondary, #888);
    font-size: 13px;
  }
  .hl-summary {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
  }
  .hl-chip {
    display: inline-block;
    padding: 4px 10px;
    border-radius: 12px;
    font-size: 12px;
    font-weight: 600;
  }
  .hl-chip-error {
    background: #fde8e8;
    color: #b91c1c;
  }
  .hl-chip-warning {
    background: #fff3cd;
    color: #92400e;
  }
  .hl-chip-detail {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 8px;
    font-size: 11px;
  }
  .hl-chip-detail.issue-error {
    background: #fee2e2;
    color: #991b1b;
  }
  .hl-chip-detail.issue-warning {
    background: #fef3c7;
    color: #92400e;
  }
  .hl-chip-detail.issue-info {
    background: #fef9c3;
    color: #854d0e;
  }
  .hl-filters {
    display: flex;
    gap: 4px;
    margin-bottom: 12px;
    flex-wrap: wrap;
  }
  .hl-filter {
    padding: 4px 12px;
    border: 1px solid var(--border, #ddd);
    border-radius: 16px;
    background: var(--bg, #fff);
    cursor: pointer;
    font-size: 12px;
    transition: all 0.15s;
  }
  .hl-filter:hover {
    border-color: var(--accent, #2563eb);
  }
  .hl-filter.active {
    background: var(--accent, #2563eb);
    color: #fff;
    border-color: var(--accent, #2563eb);
  }
  .hl-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .hl-table th {
    text-align: left;
    padding: 8px;
    border-bottom: 2px solid var(--border, #ddd);
    font-size: 11px;
    text-transform: uppercase;
    color: var(--text-secondary, #888);
  }
  .hl-table td {
    padding: 6px 8px;
    border-bottom: 1px solid var(--border, #eee);
    vertical-align: top;
  }
  .hl-table tbody tr:nth-child(even) {
    background: var(--row-even, #f9f9f9);
  }
  .cell-url {
    max-width: 300px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cell-url a {
    color: var(--accent, #2563eb);
    text-decoration: none;
  }
  .cell-url a:hover {
    text-decoration: underline;
  }
  .cell-detail {
    max-width: 250px;
    font-size: 12px;
    color: var(--text-secondary, #888);
  }
  .issue-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    white-space: nowrap;
  }
  .issue-error {
    background: #fee2e2;
    color: #991b1b;
  }
  .issue-warning {
    background: #fef3c7;
    color: #92400e;
  }
  .issue-info {
    background: #fef9c3;
    color: #854d0e;
  }
  .hl-pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-top: 12px;
    font-size: 13px;
  }
  .hl-pagination button {
    padding: 4px 12px;
  }
  .hl-export {
    margin-left: auto;
  }
</style>
