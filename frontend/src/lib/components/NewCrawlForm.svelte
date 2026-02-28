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
  let checkExternalLinks = $state(true);
  let externalLinkWorkers = $state(3);
  let userAgentPreset = $state('');
  let userAgentCustom = $state('');
  let crawlSitemapOnly = $state(false);
  let tlsProfile = $state('');
  let starting = $state(false);

  const userAgentPresets = [
    { label: 'Default (CrawlObserver)', value: '', tls: '' },
    { label: 'Googlebot Desktop', value: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)', tls: '' },
    { label: 'Googlebot Mobile', value: 'Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.69 Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)', tls: '' },
    { label: 'Bingbot', value: 'Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)', tls: '' },
    { label: 'Chrome Desktop', value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36', tls: 'chrome' },
    { label: 'Custom', value: 'custom', tls: '' },
  ];

  function onUserAgentChange() {
    const preset = userAgentPresets.find(p => p.value === userAgentPreset);
    if (preset) tlsProfile = preset.tls;
  }

  async function handleStartCrawl() {
    const seeds = seedInput.split('\n').map(s => s.trim()).filter(Boolean);
    if (seeds.length === 0) return;
    starting = true;
    const ua = userAgentPreset === 'custom' ? userAgentCustom : userAgentPreset;
    try {
      await startCrawl(seeds, { max_pages: maxPages, max_depth: maxDepth, workers, delay: crawlDelay, store_html: storeHtml, crawl_scope: crawlScope, project_id: crawlProjectId || null, check_external_links: checkExternalLinks, external_link_workers: externalLinkWorkers, user_agent: ua || undefined, crawl_sitemap_only: crawlSitemapOnly, tls_profile: tlsProfile || undefined });
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
    <div class="form-group form-full-width">
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
    <div class="form-group">
      <label for="useragent">User-Agent</label>
      <select id="useragent" bind:value={userAgentPreset} onchange={onUserAgentChange}>
        {#each userAgentPresets as preset}
          <option value={preset.value}>{preset.label}</option>
        {/each}
      </select>
    </div>
    {#if userAgentPreset === 'custom'}
      <div class="form-group">
        <label for="useragent-custom">Custom User-Agent</label>
        <input id="useragent-custom" type="text" bind:value={userAgentCustom} placeholder="Mozilla/5.0 ..." />
      </div>
    {/if}
    {#if userAgentPreset !== ''}
      <div class="form-group">
        <label for="tlsprofile">TLS Fingerprint</label>
        <select id="tlsprofile" bind:value={tlsProfile}>
          <option value="">Off (Go default)</option>
          <option value="chrome">Chrome</option>
          <option value="firefox">Firefox</option>
          <option value="edge">Edge</option>
        </select>
      </div>
    {/if}
    <div class="form-group form-checkbox-row">
      <input id="storehtml" type="checkbox" bind:checked={storeHtml} /><label for="storehtml" class="form-checkbox-label">Store raw HTML</label>
    </div>
    <div class="form-group form-checkbox-row">
      <input id="checkext" type="checkbox" bind:checked={checkExternalLinks} /><label for="checkext" class="form-checkbox-label">Check external links</label>
    </div>
    <div class="form-group form-checkbox-row">
      <input id="sitemaponly" type="checkbox" bind:checked={crawlSitemapOnly} /><label for="sitemaponly" class="form-checkbox-label">Sitemap only (crawl only URLs found in sitemaps)</label>
    </div>
    {#if checkExternalLinks}
      <div class="form-group"><label for="extworkers">External link workers</label><input id="extworkers" type="number" bind:value={externalLinkWorkers} min="1" max="20" /></div>
    {/if}
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
  <div class="form-actions">
    <button class="btn btn-primary" onclick={handleStartCrawl} disabled={starting || !seedInput.trim()}>
      {starting ? 'Starting...' : 'Start Crawl'}
    </button>
    <button class="btn" onclick={oncancel}>Cancel</button>
  </div>
</div>

<style>
  .form-full-width { grid-column: 1 / -1; }
  .form-checkbox-row {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
    padding-top: 24px;
  }
  .form-checkbox-label { margin: 0; }
  .form-actions {
    display: flex;
    gap: 8px;
    margin-top: 20px;
  }
</style>
