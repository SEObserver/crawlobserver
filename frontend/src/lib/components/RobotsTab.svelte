<script>
  import { getRobotsHosts, getRobotsContent, testRobotsUrls } from '../api.js';
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
    robotsLoading = true;
    try {
      const data = await getRobotsContent(sessionId, host);
      robotsContent = data.Content || data.content || '';
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
      {#if robotsContent !== null}
        <pre class="robots-content-pre">{robotsContent || '(empty)'}</pre>
      {:else}
        <p style="color: var(--text-muted);">Loading...</p>
      {/if}

      <div class="robots-tester" style="margin-top: 20px;">
        <h4 style="font-size: 13px; font-weight: 600; margin-bottom: 8px;">URL Tester</h4>
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
      </div>
    {:else}
      <div style="padding: 40px; text-align: center; color: var(--text-muted);">
        <p>Select a host to view its robots.txt</p>
      </div>
    {/if}
  </div>
</div>
