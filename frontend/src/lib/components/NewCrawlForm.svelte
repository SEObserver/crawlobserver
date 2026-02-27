<script>
  import { startCrawl } from '../api.js';

  let { projects = [], onstart, oncancel, onerror } = $props();

  let seedInput = $state('');
  let maxPages = $state(0);
  let maxDepth = $state(0);
  let workers = $state(10);
  let crawlDelay = $state('1s');
  let storeHtml = $state(false);
  let crawlScope = $state('host');
  let crawlProjectId = $state('');
  let starting = $state(false);

  async function handleStartCrawl() {
    const seeds = seedInput.split('\n').map(s => s.trim()).filter(Boolean);
    if (seeds.length === 0) return;
    starting = true;
    try {
      await startCrawl(seeds, { max_pages: maxPages, max_depth: maxDepth, workers, delay: crawlDelay, store_html: storeHtml, crawl_scope: crawlScope, project_id: crawlProjectId || null });
      onstart?.();
    } catch (e) {
      onerror?.(e.message);
    } finally {
      starting = false;
    }
  }
</script>

<div class="page-header">
  <h1>New Crawl</h1>
</div>
<div class="card">
  <div class="form-grid">
    <div class="form-group" style="grid-column: 1 / -1;">
      <label for="seeds">Seed URLs (one per line)</label>
      <textarea id="seeds" bind:value={seedInput} rows="3" placeholder="https://example.com"></textarea>
    </div>
    <div class="form-group"><label for="workers">Workers</label><input id="workers" type="number" bind:value={workers} min="1" max="100" /></div>
    <div class="form-group"><label for="delay">Delay</label><input id="delay" type="text" bind:value={crawlDelay} placeholder="1s" /></div>
    <div class="form-group"><label for="maxpages">Max pages (0 = unlimited)</label><input id="maxpages" type="number" bind:value={maxPages} min="0" /></div>
    <div class="form-group"><label for="maxdepth">Max depth (0 = unlimited)</label><input id="maxdepth" type="number" bind:value={maxDepth} min="0" /></div>
    <div class="form-group">
      <label for="scope">Crawl scope</label>
      <select id="scope" bind:value={crawlScope}>
        <option value="host">Same host only</option>
        <option value="domain">Same domain (incl. subdomains)</option>
      </select>
    </div>
    <div class="form-group" style="display: flex; flex-direction: row; align-items: center; gap: 8px; padding-top: 24px;">
      <input id="storehtml" type="checkbox" bind:checked={storeHtml} /><label for="storehtml" style="margin: 0;">Store raw HTML</label>
    </div>
    {#if projects.length > 0}
      <div class="form-group">
        <label for="crawl-project">Project (optional)</label>
        <select id="crawl-project" bind:value={crawlProjectId}>
          <option value="">No project</option>
          {#each projects as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
      </div>
    {/if}
  </div>
  <div style="display: flex; gap: 8px; margin-top: 20px;">
    <button class="btn btn-primary" onclick={handleStartCrawl} disabled={starting || !seedInput.trim()}>
      {starting ? 'Starting...' : 'Start Crawl'}
    </button>
    <button class="btn" onclick={oncancel}>Cancel</button>
  </div>
</div>
