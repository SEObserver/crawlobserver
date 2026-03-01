<script>
  import { getRobotsHosts, getRobotsContent, testRobotsUrls, simulateRobots } from '../api.js';
  import { a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { sessionId, onerror } = $props();

  let robotsHosts = $state([]);
  let robotsSelectedHost = $state(null);
  let robotsContent = $state(null);
  let robotsLoading = $state(false);
  let robotsTestUrls = $state('');
  let robotsTestUA = $state('');
  let robotsTestResults = $state(null);
  let robotsTestLoading = $state(false);

  // Sub-view state
  let robotsSubView = $state('content');

  // Simulator state
  let simulateContent = $state('');
  let simulateUA = $state('');
  let simulateLoading = $state(false);
  let simulateResults = $state(null);
  let showNewlyBlocked = $state(false);
  let showNewlyAllowed = $state(false);

  async function loadRobotsHosts() {
    robotsLoading = true;
    robotsSelectedHost = null;
    robotsContent = null;
    robotsTestResults = null;
    try {
      robotsHosts = await getRobotsHosts(sessionId);
    } catch (e) {
      robotsHosts = [];
    } finally {
      robotsLoading = false;
    }
  }

  async function selectRobotsHost(host) {
    robotsSelectedHost = host;
    robotsContent = null;
    robotsTestResults = null;
    simulateResults = null;
    simulateContent = '';
    robotsSubView = 'content';
    robotsLoading = true;
    try {
      const data = await getRobotsContent(sessionId, host);
      robotsContent = data.Content || data.content || '';
      simulateContent = robotsContent;
    } catch (e) {
      robotsContent = '(failed to load)';
    } finally {
      robotsLoading = false;
    }
  }

  async function runRobotsTest() {
    if (!robotsSelectedHost) return;
    const urls = robotsTestUrls
      .split('\n')
      .map((u) => u.trim())
      .filter(Boolean);
    if (urls.length === 0) return;
    robotsTestLoading = true;
    try {
      const data = await testRobotsUrls(sessionId, robotsSelectedHost, robotsTestUA || '*', urls);
      robotsTestResults = data.results;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      robotsTestLoading = false;
    }
  }

  async function runSimulation() {
    if (!robotsSelectedHost || !simulateContent.trim()) return;
    simulateLoading = true;
    simulateResults = null;
    try {
      simulateResults = await simulateRobots(
        sessionId,
        robotsSelectedHost,
        simulateUA || '*',
        simulateContent,
      );
      showNewlyBlocked = false;
      showNewlyAllowed = false;
    } catch (e) {
      onerror?.(e.message);
    } finally {
      simulateLoading = false;
    }
  }

  loadRobotsHosts();
</script>

<div class="robots-layout">
  <div class="robots-hosts">
    {#if robotsLoading && robotsHosts.length === 0}
      <p class="robots-empty-msg text-muted">{t('common.loading')}</p>
    {:else if robotsHosts.length === 0}
      <p class="robots-empty-msg text-muted">{t('robots.noData')}</p>
    {:else}
      <table>
        <thead>
          <tr
            ><th>{t('robots.host')}</th><th>{t('common.status')}</th><th>{t('robots.fetched')}</th
            ></tr
          >
        </thead>
        <tbody>
          {#each robotsHosts as h}
            <tr
              class="robots-host-row"
              class:robots-host-active={robotsSelectedHost === h.Host}
              role="button"
              tabindex="0"
              onclick={() => selectRobotsHost(h.Host)}
              onkeydown={a11yKeydown(() => selectRobotsHost(h.Host))}
            >
              <td class="font-medium">{h.Host}</td>
              <td
                ><span
                  class="badge {h.StatusCode === 200
                    ? 'badge-success'
                    : h.StatusCode >= 400
                      ? 'badge-error'
                      : 'badge-warning'}">{h.StatusCode}</span
                ></td
              >
              <td class="text-muted text-xs">{new Date(h.FetchedAt).toLocaleString()}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
  <div class="robots-detail">
    {#if robotsSelectedHost}
      <h3 class="robots-detail-title text-secondary font-semibold">
        robots.txt &mdash; {robotsSelectedHost}
      </h3>

      <div class="pr-subview-bar">
        <button
          class="pr-subview-btn {robotsSubView === 'content' ? 'pr-subview-active' : ''}"
          onclick={() => (robotsSubView = 'content')}>{t('robots.content')}</button
        >
        <button
          class="pr-subview-btn {robotsSubView === 'tester' ? 'pr-subview-active' : ''}"
          onclick={() => (robotsSubView = 'tester')}>{t('robots.urlTester')}</button
        >
        <button
          class="pr-subview-btn {robotsSubView === 'simulator' ? 'pr-subview-active' : ''}"
          onclick={() => (robotsSubView = 'simulator')}>{t('robots.simulator')}</button
        >
      </div>

      {#if robotsSubView === 'content'}
        {#if robotsContent !== null}
          <pre class="robots-content-pre">{robotsContent || t('robots.empty')}</pre>
        {:else}
          <p class="text-muted">{t('common.loading')}</p>
        {/if}
      {:else if robotsSubView === 'tester'}
        <div class="form-group mb-sm">
          <label>{t('robots.userAgentOptional')}</label>
          <input
            type="text"
            placeholder={t('robots.userAgentDefault')}
            bind:value={robotsTestUA}
            class="robots-ua-input"
          />
        </div>
        <div class="form-group mb-sm">
          <label>{t('robots.urlsToTest')}</label>
          <textarea
            rows="4"
            bind:value={robotsTestUrls}
            placeholder="/path/to/page&#10;/another/path"
            class="robots-mono-textarea"
          ></textarea>
        </div>
        <button
          class="btn btn-primary btn-sm"
          onclick={runRobotsTest}
          disabled={robotsTestLoading || !robotsTestUrls.trim()}
        >
          {robotsTestLoading ? t('robots.testing') : t('robots.test')}
        </button>

        {#if robotsTestResults}
          <div class="robots-test-results">
            {#each robotsTestResults as r}
              <div class="robots-test-result">
                <span class="badge {r.allowed ? 'badge-success' : 'badge-error'}"
                  >{r.allowed ? t('robots.allowed') : t('robots.blocked')}</span
                >
                <span class="text-sm robots-test-url">{r.url}</span>
              </div>
            {/each}
          </div>
        {/if}
      {:else}
        <p class="text-xs text-muted robots-sim-hint">
          {t('robots.simulatorDesc')}
        </p>
        <div class="form-group mb-sm">
          <label>{t('robots.userAgentOptional')}</label>
          <input
            type="text"
            placeholder={t('robots.userAgentDefault')}
            bind:value={simulateUA}
            class="robots-ua-input"
          />
        </div>
        <div class="form-group robots-sim-form-group">
          <label>{t('robots.proposedRobots')}</label>
          <textarea rows="12" bind:value={simulateContent} class="robots-sim-textarea"></textarea>
        </div>
        <button
          class="btn btn-primary btn-sm"
          onclick={runSimulation}
          disabled={simulateLoading || !simulateContent.trim()}
        >
          {simulateLoading ? t('robots.runningSimulation') : t('robots.runSimulation')}
        </button>

        {#if simulateResults}
          {@const res = simulateResults}
          {@const newlyBlockedCount = res.newly_blocked?.length || 0}
          {@const newlyAllowedCount = res.newly_allowed?.length || 0}
          {@const afterAllowed =
            (res.currently_allowed || 0) - newlyBlockedCount + newlyAllowedCount}
          {@const afterBlocked =
            (res.currently_blocked || 0) + newlyBlockedCount - newlyAllowedCount}
          <div class="sim-compare">
            <div class="sim-compare-header">
              <span></span>
              <span>{t('robots.before')}</span>
              <span></span>
              <span>{t('robots.after')}</span>
              <span>{t('robots.delta')}</span>
            </div>
            <div class="sim-compare-row">
              <span class="sim-compare-label">{t('robots.allowed')}</span>
              <span class="sim-compare-val sim-val-success"
                >{res.currently_allowed?.toLocaleString()}</span
              >
              <span class="sim-compare-arrow">→</span>
              <span class="sim-compare-val sim-val-success">{afterAllowed.toLocaleString()}</span>
              {#if newlyBlockedCount > 0 || newlyAllowedCount > 0}
                {@const delta = afterAllowed - (res.currently_allowed || 0)}
                <span
                  class="sim-compare-delta"
                  style="color: {delta > 0
                    ? 'var(--success)'
                    : delta < 0
                      ? 'var(--error)'
                      : 'var(--text-muted)'};">{delta > 0 ? '+' : ''}{delta.toLocaleString()}</span
                >
              {:else}
                <span class="sim-compare-delta text-muted">—</span>
              {/if}
            </div>
            <div class="sim-compare-row">
              <span class="sim-compare-label">{t('robots.blocked')}</span>
              <span class="sim-compare-val sim-val-error"
                >{res.currently_blocked?.toLocaleString()}</span
              >
              <span class="sim-compare-arrow">→</span>
              <span class="sim-compare-val sim-val-error">{afterBlocked.toLocaleString()}</span>
              {#if newlyBlockedCount > 0 || newlyAllowedCount > 0}
                {@const delta = afterBlocked - (res.currently_blocked || 0)}
                <span
                  class="sim-compare-delta"
                  style="color: {delta > 0
                    ? 'var(--error)'
                    : delta < 0
                      ? 'var(--success)'
                      : 'var(--text-muted)'};">{delta > 0 ? '+' : ''}{delta.toLocaleString()}</span
                >
              {:else}
                <span class="sim-compare-delta text-muted">—</span>
              {/if}
            </div>
            <div class="sim-compare-row sim-compare-total">
              <span class="sim-compare-label">{t('robots.total')}</span>
              <span class="sim-compare-val">{res.total_urls?.toLocaleString()}</span>
              <span class="sim-compare-arrow"></span>
              <span class="sim-compare-val">{res.total_urls?.toLocaleString()}</span>
              <span class="sim-compare-delta"></span>
            </div>
          </div>

          {#if res.newly_blocked?.length > 0}
            <button
              class="sim-change-header sim-url-blocked"
              onclick={() => (showNewlyBlocked = !showNewlyBlocked)}
            >
              <span class="badge badge-error">{res.newly_blocked.length}</span>
              <span>{t('robots.urlsNewlyBlocked')}</span>
              <span class="sim-toggle-icon">{showNewlyBlocked ? '▲' : '▼'}</span>
            </button>
            {#if showNewlyBlocked}
              <div class="sim-url-list">
                {#each res.newly_blocked as item}
                  <div class="sim-url-item">{item.url}</div>
                {/each}
              </div>
            {/if}
          {/if}

          {#if res.newly_allowed?.length > 0}
            <button
              class="sim-change-header sim-url-allowed"
              onclick={() => (showNewlyAllowed = !showNewlyAllowed)}
            >
              <span class="badge badge-success">{res.newly_allowed.length}</span>
              <span>{t('robots.urlsNewlyAllowed')}</span>
              <span class="sim-toggle-icon">{showNewlyAllowed ? '▲' : '▼'}</span>
            </button>
            {#if showNewlyAllowed}
              <div class="sim-url-list">
                {#each res.newly_allowed as item}
                  <div class="sim-url-item">{item.url}</div>
                {/each}
              </div>
            {/if}
          {/if}

          {#if (!res.newly_blocked || res.newly_blocked.length === 0) && (!res.newly_allowed || res.newly_allowed.length === 0)}
            <p class="mt-md text-muted text-sm">{t('robots.noChanges')}</p>
          {/if}
        {/if}
      {/if}
    {:else}
      <div class="robots-placeholder text-center text-muted">
        <p>{t('robots.selectHost')}</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .robots-layout {
    display: grid;
    grid-template-columns: 340px 1fr;
    min-height: 400px;
  }
  .robots-hosts {
    border-right: 1px solid var(--border);
    overflow-y: auto;
    max-height: 70vh;
  }
  .robots-host-active td {
    background: var(--accent-light);
    color: var(--accent);
  }
  .robots-detail {
    padding: 20px;
    overflow-y: auto;
    max-height: 70vh;
  }
  .robots-content-pre {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 16px;
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 12px;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-all;
    overflow: auto;
    color: var(--text);
  }
  .robots-test-result {
    display: flex;
    align-items: center;
    padding: 6px 0;
    border-bottom: 1px solid var(--border-light);
  }
  .robots-test-result:last-child {
    border-bottom: none;
  }
  .sim-compare {
    margin-top: 16px;
    margin-bottom: 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  .sim-compare-header {
    display: grid;
    grid-template-columns: 80px 1fr 28px 1fr 80px;
    gap: 0;
    padding: 8px 16px;
    background: var(--bg);
    border-bottom: 1px solid var(--border);
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    text-align: right;
  }
  .sim-compare-header span:first-child {
    text-align: left;
  }
  .sim-compare-header span:nth-child(3) {
    text-align: center;
  }
  .sim-compare-row {
    display: grid;
    grid-template-columns: 80px 1fr 28px 1fr 80px;
    gap: 0;
    padding: 10px 16px;
    border-bottom: 1px solid var(--border-light);
    align-items: center;
    font-variant-numeric: tabular-nums;
  }
  .sim-compare-row:last-child {
    border-bottom: none;
  }
  .sim-compare-total {
    border-top: 1px solid var(--border);
    background: var(--bg);
  }
  .sim-compare-label {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
  }
  .sim-compare-val {
    font-size: 18px;
    font-weight: 700;
    text-align: right;
    letter-spacing: -0.02em;
  }
  .sim-compare-arrow {
    text-align: center;
    color: var(--text-muted);
    font-size: 14px;
  }
  .sim-compare-delta {
    font-size: 13px;
    font-weight: 600;
    text-align: right;
  }
  .sim-change-header {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-card);
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    color: var(--text);
    margin-bottom: 2px;
    transition: background 0.15s;
    font-family: inherit;
  }
  .sim-change-header:hover {
    background: var(--bg-hover);
  }
  .sim-url-blocked {
    border-left: 3px solid var(--error);
  }
  .sim-url-allowed {
    border-left: 3px solid var(--success);
  }
  .sim-url-list {
    border: 1px solid var(--border);
    border-top: none;
    border-radius: 0 0 var(--radius-sm) var(--radius-sm);
    max-height: 300px;
    overflow-y: auto;
    margin-bottom: 8px;
  }
  .sim-url-item {
    padding: 6px 14px;
    font-size: 12px;
    font-family: 'SF Mono', 'Fira Code', monospace;
    border-bottom: 1px solid var(--border-light);
    color: var(--text-secondary);
    word-break: break-all;
  }
  .sim-url-item:last-child {
    border-bottom: none;
  }
  .robots-empty-msg {
    padding: 20px;
  }
  .robots-host-row {
    cursor: pointer;
  }
  .robots-detail-title {
    font-size: 14px;
    margin-bottom: 12px;
  }
  .robots-ua-input {
    max-width: 300px;
  }
  .robots-mono-textarea {
    font-family: 'SF Mono', monospace;
    font-size: 13px;
  }
  .robots-test-results {
    margin-top: 12px;
  }
  .robots-test-url {
    margin-left: 8px;
  }
  .robots-sim-hint {
    margin-bottom: 12px;
  }
  .robots-sim-form-group {
    margin-bottom: 12px;
  }
  .robots-sim-textarea {
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 12px;
    line-height: 1.6;
  }
  .sim-val-success {
    color: var(--success);
  }
  .sim-val-error {
    color: var(--error);
  }
  .sim-toggle-icon {
    margin-left: auto;
    font-size: 12px;
  }
  .robots-placeholder {
    padding: 40px;
  }
</style>
