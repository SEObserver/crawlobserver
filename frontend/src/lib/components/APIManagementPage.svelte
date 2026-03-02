<script>
  import { getProjects, getAPIKeys, createAPIKey, deleteAPIKey, getServerInfo } from '../api.js';
  import { timeAgo, copyToClipboard } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import ConfirmModal from './ConfirmModal.svelte';
  import SearchSelect from './SearchSelect.svelte';

  let { onerror, onprojectschanged } = $props();

  let confirmState = $state(null);
  let copiedRef = $state(false);

  function showConfirm(message, onConfirm, opts = {}) {
    confirmState = { message, onConfirm, ...opts };
  }

  let serverInfo = $state(null);

  let projects = $state([]);
  let apiKeys = $state([]);
  let newKeyName = $state('');
  let newKeyType = $state('general');
  let newKeyProjectId = $state('');
  let createdKeyFull = $state(null);

  async function loadAPIData() {
    try {
      projects = await getProjects();
      apiKeys = await getAPIKeys();
      onprojectschanged?.(projects);
    } catch (e) {
      onerror?.(e.message);
    }
  }

  async function handleCreateAPIKey() {
    if (!newKeyName.trim() || !newKeyType) return;
    try {
      const pid = newKeyType === 'project' && newKeyProjectId ? newKeyProjectId : null;
      const result = await createAPIKey(newKeyName.trim(), newKeyType, pid);
      createdKeyFull = result.key;
      newKeyName = '';
      newKeyType = 'general';
      newKeyProjectId = '';
      await loadAPIData();
    } catch (e) {
      onerror?.(e.message);
    }
  }

  function handleDeleteAPIKey(id) {
    showConfirm(
      t('api.confirmRevokeKey'),
      async () => {
        try {
          await deleteAPIKey(id);
          await loadAPIData();
        } catch (e) {
          onerror?.(e.message);
        }
      },
      { danger: true, confirmLabel: t('common.delete') },
    );
  }

  async function loadServerInfo() {
    try {
      serverInfo = await getServerInfo();
    } catch (e) {
      /* non-critical */
    }
  }

  const apiRef = [
    {
      section: 'Crawl Sessions',
      endpoints: [
        { method: 'GET', path: '/sessions', desc: 'List all crawl sessions' },
        {
          method: 'POST',
          path: '/crawl',
          desc: 'Start a new crawl (JSON body with seeds, config)',
        },
        { method: 'POST', path: '/sessions/{id}/stop', desc: 'Stop a running crawl' },
        { method: 'POST', path: '/sessions/{id}/resume', desc: 'Resume a stopped crawl' },
        { method: 'POST', path: '/sessions/{id}/retry-failed', desc: 'Retry failed URLs' },
        { method: 'DELETE', path: '/sessions/{id}', desc: 'Delete a session' },
        { method: 'GET', path: '/sessions/{id}/export', desc: 'Export session data (CSV/JSON)' },
        { method: 'POST', path: '/sessions/import', desc: 'Import a session archive' },
      ],
    },
    {
      section: 'Session Data (read)',
      endpoints: [
        {
          method: 'GET',
          path: '/sessions/{id}/pages',
          desc: 'List crawled pages (paginated, filterable)',
        },
        { method: 'GET', path: '/sessions/{id}/stats', desc: 'Session statistics summary' },
        { method: 'GET', path: '/sessions/{id}/audit', desc: 'SEO audit issues' },
        { method: 'GET', path: '/sessions/{id}/links', desc: 'All links (internal + external)' },
        { method: 'GET', path: '/sessions/{id}/internal-links', desc: 'Internal links only' },
        {
          method: 'GET',
          path: '/sessions/{id}/page-detail?url=',
          desc: 'Full detail for a single page',
        },
        { method: 'GET', path: '/sessions/{id}/page-html?url=', desc: 'Raw HTML body of a page' },
        { method: 'GET', path: '/sessions/{id}/robots', desc: 'Robots.txt hosts found' },
        {
          method: 'GET',
          path: '/sessions/{id}/robots-content?host=',
          desc: 'Robots.txt content for a host',
        },
        { method: 'GET', path: '/sessions/{id}/sitemaps', desc: 'Discovered sitemaps' },
        { method: 'GET', path: '/sessions/{id}/sitemap-urls?sitemap=', desc: 'URLs in a sitemap' },
        {
          method: 'GET',
          path: '/sessions/{id}/near-duplicates',
          desc: 'Near-duplicate page pairs',
        },
        {
          method: 'GET',
          path: '/sessions/{id}/external-checks',
          desc: 'External link check results',
        },
        {
          method: 'GET',
          path: '/sessions/{id}/external-checks/domains',
          desc: 'External checks grouped by domain',
        },
        {
          method: 'GET',
          path: '/sessions/{id}/external-checks/expired-domains',
          desc: 'Expired/dead domains',
        },
        {
          method: 'GET',
          path: '/sessions/{id}/resource-checks',
          desc: 'Page resource (JS/CSS/img) check results',
        },
        {
          method: 'GET',
          path: '/sessions/{id}/resource-checks/summary',
          desc: 'Resource checks summary by type',
        },
      ],
    },
    {
      section: 'PageRank & Authority',
      endpoints: [
        {
          method: 'GET',
          path: '/sessions/{id}/pagerank-top',
          desc: 'Top pages by internal PageRank',
        },
        {
          method: 'GET',
          path: '/sessions/{id}/pagerank-distribution',
          desc: 'PageRank distribution histogram',
        },
        { method: 'GET', path: '/sessions/{id}/pagerank-treemap', desc: 'PageRank treemap data' },
        { method: 'POST', path: '/sessions/{id}/compute-pagerank', desc: 'Recompute PageRank' },
        { method: 'POST', path: '/sessions/{id}/recompute-depths', desc: 'Recompute crawl depths' },
        {
          method: 'GET',
          path: '/sessions/{id}/authority',
          desc: 'Pages enriched with external authority data',
        },
      ],
    },
    {
      section: 'Robots & Testing',
      endpoints: [
        {
          method: 'POST',
          path: '/sessions/{id}/robots-test',
          desc: 'Test if a URL is allowed by robots.txt',
        },
        {
          method: 'POST',
          path: '/sessions/{id}/robots-simulate',
          desc: 'Simulate robots.txt rules for URLs',
        },
        {
          method: 'POST',
          path: '/sessions/{id}/run-tests',
          desc: 'Run custom test ruleset on session',
        },
      ],
    },
    {
      section: 'Compare Sessions',
      endpoints: [
        {
          method: 'GET',
          path: '/compare/stats?a={id}&b={id}',
          desc: 'Compare stats of two sessions',
        },
        {
          method: 'GET',
          path: '/compare/pages?a={id}&b={id}',
          desc: 'Page differences between two sessions',
        },
        {
          method: 'GET',
          path: '/compare/links?a={id}&b={id}',
          desc: 'Link differences between two sessions',
        },
      ],
    },
    {
      section: 'Projects',
      endpoints: [
        { method: 'GET', path: '/projects', desc: 'List all projects' },
        { method: 'POST', path: '/projects', desc: 'Create a project' },
        { method: 'PUT', path: '/projects/{id}', desc: 'Rename a project' },
        { method: 'DELETE', path: '/projects/{id}', desc: 'Delete a project' },
        {
          method: 'POST',
          path: '/projects/{pid}/sessions/{sid}',
          desc: 'Associate session to project',
        },
        {
          method: 'DELETE',
          path: '/projects/{pid}/sessions/{sid}',
          desc: 'Disassociate session from project',
        },
      ],
    },
    {
      section: 'API Keys',
      endpoints: [
        { method: 'GET', path: '/api-keys', desc: 'List API keys' },
        { method: 'POST', path: '/api-keys', desc: 'Create an API key' },
        { method: 'DELETE', path: '/api-keys/{id}', desc: 'Revoke an API key' },
      ],
    },
    {
      section: 'Custom Test Rulesets',
      endpoints: [
        { method: 'GET', path: '/rulesets', desc: 'List test rulesets' },
        { method: 'POST', path: '/rulesets', desc: 'Create a test ruleset' },
        { method: 'GET', path: '/rulesets/{id}', desc: 'Get a ruleset' },
        { method: 'PUT', path: '/rulesets/{id}', desc: 'Update a ruleset' },
        { method: 'DELETE', path: '/rulesets/{id}', desc: 'Delete a ruleset' },
      ],
    },
    {
      section: 'Providers (SEObserver, etc.)',
      endpoints: [
        { method: 'GET', path: '/projects/{id}/providers', desc: 'List provider connections' },
        {
          method: 'POST',
          path: '/projects/{id}/providers/{provider}/connect',
          desc: 'Connect a provider',
        },
        {
          method: 'DELETE',
          path: '/projects/{id}/providers/{provider}/disconnect',
          desc: 'Disconnect a provider',
        },
        {
          method: 'POST',
          path: '/projects/{id}/providers/{provider}/fetch',
          desc: 'Fetch provider data',
        },
        {
          method: 'GET',
          path: '/projects/{id}/providers/{provider}/metrics',
          desc: 'Domain metrics',
        },
        { method: 'GET', path: '/projects/{id}/providers/{provider}/backlinks', desc: 'Backlinks' },
        {
          method: 'GET',
          path: '/projects/{id}/providers/{provider}/refdomains',
          desc: 'Referring domains',
        },
        {
          method: 'GET',
          path: '/projects/{id}/providers/{provider}/rankings',
          desc: 'Organic keyword rankings',
        },
        {
          method: 'GET',
          path: '/projects/{id}/providers/{provider}/visibility',
          desc: 'Visibility history',
        },
        {
          method: 'GET',
          path: '/projects/{id}/providers/{provider}/top-pages',
          desc: 'Top pages with authority',
        },
      ],
    },
    {
      section: 'Google Search Console',
      endpoints: [
        { method: 'GET', path: '/projects/{id}/gsc/status', desc: 'GSC connection status' },
        { method: 'POST', path: '/projects/{id}/gsc/fetch', desc: 'Fetch GSC data' },
        { method: 'GET', path: '/projects/{id}/gsc/overview', desc: 'GSC overview stats' },
        { method: 'GET', path: '/projects/{id}/gsc/queries', desc: 'Search queries' },
        { method: 'GET', path: '/projects/{id}/gsc/pages', desc: 'Pages performance' },
        { method: 'GET', path: '/projects/{id}/gsc/timeline', desc: 'Clicks/impressions timeline' },
      ],
    },
    {
      section: 'System',
      endpoints: [
        { method: 'GET', path: '/health', desc: 'Health check' },
        { method: 'GET', path: '/server-info', desc: 'Server info (URL, auth)' },
        { method: 'GET', path: '/global-stats', desc: 'Global statistics' },
        { method: 'GET', path: '/system-stats', desc: 'System stats (CPU, memory, disk)' },
        { method: 'GET', path: '/storage-stats', desc: 'ClickHouse storage stats' },
        {
          method: 'POST',
          path: '/check-ip',
          desc: 'Check exit IP (optional: source_ip, force_ipv4)',
        },
        { method: 'GET', path: '/logs', desc: 'Application logs' },
      ],
    },
  ];

  function buildRefMarkdown() {
    const base = serverInfo?.api_url || 'http://localhost:9090/api';
    let md = `# CrawlObserver REST API\n\nBase URL: ${base}\n`;
    if (serverInfo?.has_auth) {
      md += `Auth: Basic auth (user: ${serverInfo.username}) or X-API-Key header\n`;
    }
    md += `\nAll responses are JSON. Replace {id}, {pid}, {sid}, {provider} with actual values.\n`;
    for (const group of apiRef) {
      md += `\n## ${group.section}\n`;
      for (const ep of group.endpoints) {
        md += `${ep.method} ${base}${ep.path} — ${ep.desc}\n`;
      }
    }
    return md;
  }

  async function handleCopyRef() {
    await copyToClipboard(buildRefMarkdown());
    copiedRef = true;
    setTimeout(() => (copiedRef = false), 2000);
  }

  loadAPIData();
  loadServerInfo();
</script>

<!-- Page header with subtitle -->
<div class="page-header">
  <h1>{t('sidebar.api')}</h1>
</div>
<p class="text-sm text-muted mb-lg api-subtitle">{t('api.subtitle')}</p>

<!-- API Endpoint -->
{#if serverInfo}
  <div class="card mb-lg api-endpoint-card">
    <div class="flex-center-gap api-endpoint-header">
      <h3 class="api-endpoint-title">{t('api.endpoint')}</h3>
      <span class="badge badge-success badge-xs">{t('common.running')}</span>
    </div>
    <div class="flex-center-gap api-url-row">
      <code class="text-sm word-break api-url-code">{serverInfo.api_url}</code>
      <button class="btn btn-sm" onclick={() => copyToClipboard(serverInfo.api_url)}
        >{t('common.copy')}</button
      >
    </div>
    {#if serverInfo.has_auth}
      <div class="text-xs text-muted mb-sm">
        {t('api.authInfo', { user: serverInfo.username, header: 'X-API-Key' })}
      </div>
    {/if}
    <details class="text-xs text-muted">
      <summary class="usage-summary">{t('api.usageExamples')}</summary>
      <div class="mt-sm flex-col gap-sm">
        <div>
          <strong>{t('api.curl')}</strong>
          <code class="code-example code-example-wrap"
            >curl {serverInfo.has_auth
              ? `-u ${serverInfo.username}:PASSWORD `
              : ''}{serverInfo.api_url}/sessions</code
          >
        </div>
        <div>
          <strong>{t('api.apiKeyLabel')}</strong>
          <code class="code-example code-example-wrap"
            >curl -H "X-API-Key: YOUR_KEY" {serverInfo.api_url}/sessions</code
          >
        </div>
        <div>
          <strong>{t('api.discoveryFile')}</strong>
          <code class="code-example">cat .crawlobserver-api.json</code>
        </div>
      </div>
    </details>
  </div>
{/if}

<!-- API Keys -->
<div class="page-header api-keys-header">
  <h1>{t('api.apiKeys')}</h1>
</div>

{#if createdKeyFull}
  <div class="card key-created-card">
    <div class="key-created-inner">
      <div class="flex-1">
        <strong>{t('api.keyCreated')}</strong>
        {t('api.copyKeyNow')}<br />
        <code class="word-break key-created-code">{createdKeyFull}</code>
      </div>
      <div class="key-created-actions">
        <button class="btn btn-sm" onclick={() => copyToClipboard(createdKeyFull)}
          >{t('common.copy')}</button
        >
        <button class="btn btn-sm" onclick={() => (createdKeyFull = null)}
          >{t('common.dismiss')}</button
        >
      </div>
    </div>
  </div>
{/if}

<div class="card mb-md">
  <div class="form-grid">
    <div class="form-group">
      <label for="key-name">{t('api.keyName')}</label>
      <input
        id="key-name"
        type="text"
        bind:value={newKeyName}
        placeholder={t('api.keyNamePlaceholder')}
      />
    </div>
    <div class="form-group">
      <label for="key-type">{t('api.keyType')}</label>
      <SearchSelect
        id="key-type"
        bind:value={newKeyType}
        options={[
          { value: 'general', label: t('api.generalAccess') },
          { value: 'project', label: t('api.projectReadOnly') },
        ]}
      />
    </div>
    {#if newKeyType === 'project'}
      <div class="form-group">
        <label for="key-project">{t('stats.project')}</label>
        <SearchSelect
          id="key-project"
          bind:value={newKeyProjectId}
          placeholder={t('api.selectProject')}
          options={[
            { value: '', label: t('api.selectProject') },
            ...projects.map((p) => ({ value: p.id, label: p.name })),
          ]}
          onsearch={projects.length > 20
            ? async (q) => {
                const lq = q.toLowerCase();
                return [
                  { value: '', label: t('api.selectProject') },
                  ...projects
                    .filter((p) => p.name.toLowerCase().includes(lq))
                    .map((p) => ({ value: p.id, label: p.name })),
                ];
              }
            : undefined}
        />
      </div>
    {/if}
  </div>
  <div class="mt-md">
    <button
      class="btn btn-primary"
      onclick={handleCreateAPIKey}
      disabled={!newKeyName.trim() || (newKeyType === 'project' && !newKeyProjectId)}
      >{t('api.createKey')}</button
    >
  </div>
</div>

{#if apiKeys.length === 0}
  <div class="card text-center text-muted empty-state">{t('api.noKeys')}</div>
{:else}
  <div class="card card-flush">
    {#each apiKeys as k}
      <div class="session-row">
        <div class="session-info">
          <div class="session-seed">{k.name}</div>
          <div class="session-meta">
            <span
              class="badge"
              class:badge-info={k.type === 'general'}
              class:badge-warning={k.type === 'project'}>{k.type}</span
            >
            {#if k.project_id}
              <span class="badge badge-accent"
                >{projects.find((p) => p.id === k.project_id)?.name || k.project_id}</span
              >
            {/if}
            <code class="key-prefix-code">{k.key_prefix}</code>
            <span>{new Date(k.created_at).toLocaleDateString()}</span>
            <span
              >{k.last_used_at
                ? t('api.used') + ' ' + timeAgo(k.last_used_at)
                : t('api.neverUsed')}</span
            >
          </div>
        </div>
        <div class="session-actions">
          <button class="btn btn-sm btn-danger" onclick={() => handleDeleteAPIKey(k.id)}
            >{t('api.revoke')}</button
          >
        </div>
      </div>
    {/each}
  </div>
{/if}

<!-- API Reference -->
<div class="page-header api-ref-header">
  <h1>{t('api.reference')}</h1>
  <button class="btn btn-primary" onclick={handleCopyRef}>
    {copiedRef ? t('api.copied') : t('api.copyForLLM')}
  </button>
</div>
<p class="text-xs text-muted mb-md api-subtitle">{t('api.referenceDesc')}</p>

<div class="card api-ref-card">
  {#if serverInfo}
    <div class="api-ref-base">
      Base URL: <code>{serverInfo.api_url}</code>
    </div>
  {/if}
  {#each apiRef as group, i}
    <details class="api-ref-group" open={i === 0}>
      <summary class="api-ref-group-summary">
        <span class="api-ref-group-title">{group.section}</span>
        <span class="api-ref-group-count">{group.endpoints.length}</span>
      </summary>
      <div class="api-ref-table-wrap">
        <table class="api-ref-table">
          <tbody>
            {#each group.endpoints as ep}
              <tr>
                <td class="api-ref-method-cell">
                  <span
                    class="api-ref-method"
                    class:method-get={ep.method === 'GET'}
                    class:method-post={ep.method === 'POST'}
                    class:method-put={ep.method === 'PUT'}
                    class:method-delete={ep.method === 'DELETE'}>{ep.method}</span
                  >
                </td>
                <td class="api-ref-path-cell"><code>{ep.path}</code></td>
                <td class="api-ref-desc-cell">{ep.desc}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </details>
  {/each}
</div>

{#if confirmState}<ConfirmModal
    message={confirmState.message}
    danger={confirmState.danger}
    confirmLabel={confirmState.confirmLabel}
    onconfirm={() => {
      confirmState.onConfirm();
      confirmState = null;
    }}
    oncancel={() => (confirmState = null)}
  />{/if}

<style>
  /* --- Endpoint card --- */
  .api-endpoint-card {
    border: 1px solid var(--border);
  }
  .api-endpoint-header {
    margin-bottom: 12px;
  }
  .api-endpoint-title {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
  .badge-xs {
    font-size: 11px;
  }
  .api-url-row {
    margin-bottom: 10px;
  }
  .api-url-code {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg-secondary);
    border-radius: 6px;
  }
  .usage-summary {
    cursor: pointer;
    user-select: none;
  }
  .code-example {
    display: block;
    padding: 6px 10px;
    background: var(--bg-secondary);
    border-radius: 4px;
    margin-top: 4px;
    font-size: 12px;
  }
  .code-example-wrap {
    white-space: pre-wrap;
  }

  /* --- API Keys --- */
  .api-keys-header {
    margin-top: 8px;
  }
  .key-created-card {
    border: 1px solid var(--success);
    background: var(--success-bg);
  }
  .key-created-inner {
    display: flex;
    align-items: flex-start;
    gap: 12px;
  }
  .key-created-code {
    font-size: 0.85rem;
    margin-top: 6px;
    display: inline-block;
  }
  .key-created-actions {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
  }
  .badge-accent {
    background: var(--accent-light);
    color: var(--accent);
  }
  .key-prefix-code {
    font-size: 0.8rem;
  }
  .empty-state {
    padding: 32px;
  }
  .flex-1 {
    flex: 1;
  }

  /* --- Shared row styles --- */
  .session-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border-light);
    transition: background 0.1s;
    gap: 16px;
  }
  .session-row:last-child {
    border-bottom: none;
  }
  .session-row:hover {
    background: var(--bg-hover);
  }
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

  /* --- Subtitle --- */
  .api-subtitle {
    margin-top: -8px;
  }

  /* --- API Reference --- */
  .api-ref-header {
    margin-top: 32px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .api-ref-header h1 {
    margin: 0;
  }

  .api-ref-card {
    border: 1px solid var(--border);
    padding: 0;
    overflow: hidden;
  }

  .api-ref-base {
    padding: 12px 20px;
    font-size: 12px;
    color: var(--text-muted);
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border-light);
  }
  .api-ref-base code {
    color: var(--text);
    font-weight: 600;
  }

  .api-ref-group {
    border-bottom: 1px solid var(--border-light);
  }
  .api-ref-group:last-child {
    border-bottom: none;
  }

  .api-ref-group-summary {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 20px;
    cursor: pointer;
    user-select: none;
    font-size: 13px;
    transition: background 0.1s;
  }
  .api-ref-group-summary:hover {
    background: var(--bg-hover);
  }

  .api-ref-group-title {
    font-weight: 600;
    color: var(--text);
  }

  .api-ref-group-count {
    font-size: 11px;
    color: var(--text-muted);
    background: var(--bg-secondary);
    padding: 1px 7px;
    border-radius: 10px;
    font-weight: 500;
  }

  .api-ref-table-wrap {
    padding: 0 20px 12px 20px;
    overflow-x: auto;
  }

  .api-ref-table {
    width: 100%;
    border-collapse: collapse;
  }
  .api-ref-table td {
    padding: 5px 8px;
    font-size: 12.5px;
    line-height: 1.5;
    vertical-align: baseline;
  }

  .api-ref-method-cell {
    width: 56px;
  }
  .api-ref-path-cell code {
    font-size: 12px;
    color: var(--text);
    white-space: nowrap;
  }
  .api-ref-desc-cell {
    color: var(--text-muted);
    font-size: 12px;
  }

  .api-ref-method {
    display: inline-block;
    font-size: 10px;
    font-weight: 700;
    padding: 2px 6px;
    border-radius: 3px;
    text-align: center;
    font-family: var(--font-mono, monospace);
    letter-spacing: 0.02em;
  }

  .method-get {
    background: #e8f5e9;
    color: #2e7d32;
  }
  .method-post {
    background: #e3f2fd;
    color: #1565c0;
  }
  .method-put {
    background: #fff3e0;
    color: #e65100;
  }
  .method-delete {
    background: #fce4ec;
    color: #c62828;
  }
  :global([data-theme='dark']) .method-get {
    background: #1b3a1e;
    color: #66bb6a;
  }
  :global([data-theme='dark']) .method-post {
    background: #0d2744;
    color: #64b5f6;
  }
  :global([data-theme='dark']) .method-put {
    background: #3e2000;
    color: #ffb74d;
  }
  :global([data-theme='dark']) .method-delete {
    background: #3e0a0a;
    color: #ef9a9a;
  }
</style>
