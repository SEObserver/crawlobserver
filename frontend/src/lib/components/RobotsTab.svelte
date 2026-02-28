<script>
  import { getRobotsHosts, getRobotsContent, testRobotsUrls, simulateRobots } from '../api.js';
  import { a11yKeydown } from '../utils.js';

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
    const urls = robotsTestUrls.split('\n').map(u => u.trim()).filter(Boolean);
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
      simulateResults = await simulateRobots(sessionId, robotsSelectedHost, simulateUA || '*', simulateContent);
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
      <p style="padding: 20px; color: var(--text-muted);">Loading...</p>
    {:else if robotsHosts.length === 0}
      <p style="padding: 20px; color: var(--text-muted);">No robots.txt data. Run a crawl first.</p>
    {:else}
      <table>
        <thead>
          <tr><th>Host</th><th>Status</th><th>Fetched</th></tr>
        </thead>
        <tbody>
          {#each robotsHosts as h}
            <tr class:robots-host-active={robotsSelectedHost === h.Host} role="button" tabindex="0" style="cursor:pointer" onclick={() => selectRobotsHost(h.Host)} onkeydown={a11yKeydown(() => selectRobotsHost(h.Host))}>
              <td style="font-weight: 500;">{h.Host}</td>
              <td><span class="badge {h.StatusCode === 200 ? 'badge-success' : h.StatusCode >= 400 ? 'badge-error' : 'badge-warning'}">{h.StatusCode}</span></td>
              <td style="color: var(--text-muted); font-size: 12px;">{new Date(h.FetchedAt).toLocaleString()}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
  <div class="robots-detail">
    {#if robotsSelectedHost}
      <h3 style="font-size: 14px; font-weight: 600; margin-bottom: 12px; color: var(--text-secondary);">robots.txt &mdash; {robotsSelectedHost}</h3>

      <div class="pr-subview-bar">
        <button class="pr-subview-btn {robotsSubView === 'content' ? 'pr-subview-active' : ''}" onclick={() => robotsSubView = 'content'}>Content</button>
        <button class="pr-subview-btn {robotsSubView === 'tester' ? 'pr-subview-active' : ''}" onclick={() => robotsSubView = 'tester'}>URL Tester</button>
        <button class="pr-subview-btn {robotsSubView === 'simulator' ? 'pr-subview-active' : ''}" onclick={() => robotsSubView = 'simulator'}>Simulator</button>
      </div>

      {#if robotsSubView === 'content'}
        {#if robotsContent !== null}
          <pre class="robots-content-pre">{robotsContent || '(empty)'}</pre>
        {:else}
          <p style="color: var(--text-muted);">Loading...</p>
        {/if}

      {:else if robotsSubView === 'tester'}
        <div class="form-group" style="margin-bottom: 8px;">
          <label>User-Agent (optional)</label>
          <input type="text" placeholder="* (default)" bind:value={robotsTestUA} style="max-width: 300px;" />
        </div>
        <div class="form-group" style="margin-bottom: 8px;">
          <label>URLs to test (one per line)</label>
          <textarea rows="4" bind:value={robotsTestUrls} placeholder="/path/to/page&#10;/another/path" style="font-family: 'SF Mono', monospace; font-size: 13px;"></textarea>
        </div>
        <button class="btn btn-primary btn-sm" onclick={runRobotsTest} disabled={robotsTestLoading || !robotsTestUrls.trim()}>
          {robotsTestLoading ? 'Testing...' : 'Test'}
        </button>

        {#if robotsTestResults}
          <div style="margin-top: 12px;">
            {#each robotsTestResults as r}
              <div class="robots-test-result">
                <span class="badge {r.allowed ? 'badge-success' : 'badge-error'}">{r.allowed ? 'Allowed' : 'Blocked'}</span>
                <span style="font-size: 13px; margin-left: 8px;">{r.url}</span>
              </div>
            {/each}
          </div>
        {/if}

      {:else}
        <p style="font-size: 12px; color: var(--text-muted); margin-bottom: 12px;">
          Edit the robots.txt below, then run the simulation to see which crawled URLs would be newly blocked or allowed.
        </p>
        <div class="form-group" style="margin-bottom: 8px;">
          <label>User-Agent (optional)</label>
          <input type="text" placeholder="* (default)" bind:value={simulateUA} style="max-width: 300px;" />
        </div>
        <div class="form-group" style="margin-bottom: 12px;">
          <label>Proposed robots.txt</label>
          <textarea rows="12" bind:value={simulateContent} style="font-family: 'SF Mono', 'Fira Code', monospace; font-size: 12px; line-height: 1.6;"></textarea>
        </div>
        <button class="btn btn-primary btn-sm" onclick={runSimulation} disabled={simulateLoading || !simulateContent.trim()}>
          {simulateLoading ? 'Running...' : 'Run Simulation'}
        </button>

        {#if simulateResults}
          {@const res = simulateResults}
          {@const newlyBlockedCount = res.newly_blocked?.length || 0}
          {@const newlyAllowedCount = res.newly_allowed?.length || 0}
          {@const afterAllowed = (res.currently_allowed || 0) - newlyBlockedCount + newlyAllowedCount}
          {@const afterBlocked = (res.currently_blocked || 0) + newlyBlockedCount - newlyAllowedCount}
          <div class="sim-compare">
            <div class="sim-compare-header">
              <span></span>
              <span>Before</span>
              <span></span>
              <span>After</span>
              <span>Delta</span>
            </div>
            <div class="sim-compare-row">
              <span class="sim-compare-label">Allowed</span>
              <span class="sim-compare-val" style="color: var(--success);">{res.currently_allowed?.toLocaleString()}</span>
              <span class="sim-compare-arrow">→</span>
              <span class="sim-compare-val" style="color: var(--success);">{afterAllowed.toLocaleString()}</span>
              {#if newlyBlockedCount > 0 || newlyAllowedCount > 0}
                {@const delta = afterAllowed - (res.currently_allowed || 0)}
                <span class="sim-compare-delta" style="color: {delta > 0 ? 'var(--success)' : delta < 0 ? 'var(--error)' : 'var(--text-muted)'};">{delta > 0 ? '+' : ''}{delta.toLocaleString()}</span>
              {:else}
                <span class="sim-compare-delta" style="color: var(--text-muted);">—</span>
              {/if}
            </div>
            <div class="sim-compare-row">
              <span class="sim-compare-label">Blocked</span>
              <span class="sim-compare-val" style="color: var(--error);">{res.currently_blocked?.toLocaleString()}</span>
              <span class="sim-compare-arrow">→</span>
              <span class="sim-compare-val" style="color: var(--error);">{afterBlocked.toLocaleString()}</span>
              {#if newlyBlockedCount > 0 || newlyAllowedCount > 0}
                {@const delta = afterBlocked - (res.currently_blocked || 0)}
                <span class="sim-compare-delta" style="color: {delta > 0 ? 'var(--error)' : delta < 0 ? 'var(--success)' : 'var(--text-muted)'};">{delta > 0 ? '+' : ''}{delta.toLocaleString()}</span>
              {:else}
                <span class="sim-compare-delta" style="color: var(--text-muted);">—</span>
              {/if}
            </div>
            <div class="sim-compare-row sim-compare-total">
              <span class="sim-compare-label">Total</span>
              <span class="sim-compare-val">{res.total_urls?.toLocaleString()}</span>
              <span class="sim-compare-arrow"></span>
              <span class="sim-compare-val">{res.total_urls?.toLocaleString()}</span>
              <span class="sim-compare-delta"></span>
            </div>
          </div>

          {#if res.newly_blocked?.length > 0}
            <button class="sim-change-header sim-url-blocked" onclick={() => showNewlyBlocked = !showNewlyBlocked}>
              <span class="badge badge-error">{res.newly_blocked.length}</span>
              <span>URLs newly blocked</span>
              <span style="margin-left: auto; font-size: 12px;">{showNewlyBlocked ? '▲' : '▼'}</span>
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
            <button class="sim-change-header sim-url-allowed" onclick={() => showNewlyAllowed = !showNewlyAllowed}>
              <span class="badge badge-success">{res.newly_allowed.length}</span>
              <span>URLs newly allowed</span>
              <span style="margin-left: auto; font-size: 12px;">{showNewlyAllowed ? '▲' : '▼'}</span>
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
            <p style="margin-top: 16px; color: var(--text-muted); font-size: 13px;">No changes — all URLs keep their current status.</p>
          {/if}
        {/if}
      {/if}
    {:else}
      <div style="padding: 40px; text-align: center; color: var(--text-muted);">
        <p>Select a host to view its robots.txt</p>
      </div>
    {/if}
  </div>
</div>
