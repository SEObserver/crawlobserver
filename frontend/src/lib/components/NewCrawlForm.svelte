<script>
  import { t } from '../i18n/index.svelte.js';
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

  let userAgentPresets = $derived([
    { label: t('newCrawl.uaDefault'), value: '', tls: '' },
    { label: t('newCrawl.uaGooglebotDesktop'), value: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)', tls: '' },
    { label: t('newCrawl.uaGooglebotMobile'), value: 'Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.69 Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)', tls: '' },
    { label: t('newCrawl.uaBingbot'), value: 'Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)', tls: '' },
    { label: t('newCrawl.uaChromeDesktop'), value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36', tls: 'chrome' },
    { label: t('newCrawl.uaCustom'), value: 'custom', tls: '' },
  ]);

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
  <h1>{t('newCrawl.title')}</h1>
</div>
<div class="card">
  <div class="form-grid">
    <div class="form-group form-full-width">
      <label for="seeds">{t('newCrawl.seedUrls')}</label>
      <textarea id="seeds" bind:value={seedInput} rows="3" placeholder="https://example.com"></textarea>
    </div>
    <div class="form-group"><label for="workers">{t('newCrawl.workers')}</label><input id="workers" type="number" bind:value={workers} min="1" max="100" /></div>
    <div class="form-group"><label for="delay">{t('newCrawl.delay')}</label><input id="delay" type="text" bind:value={crawlDelay} placeholder="1s" /></div>
    <div class="form-group"><label for="maxpages">{t('newCrawl.maxPages')}</label><input id="maxpages" type="number" bind:value={maxPages} min="0" /></div>
    <div class="form-group"><label for="maxdepth">{t('newCrawl.maxDepth')}</label><input id="maxdepth" type="number" bind:value={maxDepth} min="0" /></div>
    <div class="form-group">
      <label for="scope">{t('newCrawl.crawlScope')}</label>
      <select id="scope" bind:value={crawlScope}>
        <option value="host">{t('newCrawl.sameHost')}</option>
        <option value="domain">{t('newCrawl.sameDomain')}</option>
      </select>
    </div>
    <div class="form-group">
      <label for="useragent">{t('newCrawl.userAgent')}</label>
      <select id="useragent" bind:value={userAgentPreset} onchange={onUserAgentChange}>
        {#each userAgentPresets as preset}
          <option value={preset.value}>{preset.label}</option>
        {/each}
      </select>
    </div>
    {#if userAgentPreset === 'custom'}
      <div class="form-group">
        <label for="useragent-custom">{t('newCrawl.customUserAgent')}</label>
        <input id="useragent-custom" type="text" bind:value={userAgentCustom} placeholder="Mozilla/5.0 ..." />
      </div>
    {/if}
    {#if userAgentPreset !== ''}
      <div class="form-group">
        <label for="tlsprofile">{t('newCrawl.tlsFingerprint')}</label>
        <select id="tlsprofile" bind:value={tlsProfile}>
          <option value="">{t('newCrawl.tlsOff')}</option>
          <option value="chrome">{t('newCrawl.tlsChrome')}</option>
          <option value="firefox">{t('newCrawl.tlsFirefox')}</option>
          <option value="edge">{t('newCrawl.tlsEdge')}</option>
        </select>
      </div>
    {/if}
    <div class="form-group form-checkbox-row">
      <input id="storehtml" type="checkbox" bind:checked={storeHtml} /><label for="storehtml" class="form-checkbox-label">{t('newCrawl.storeHtml')}</label>
    </div>
    <div class="form-group form-checkbox-row">
      <input id="checkext" type="checkbox" bind:checked={checkExternalLinks} /><label for="checkext" class="form-checkbox-label">{t('newCrawl.checkExternal')}</label>
    </div>
    <div class="form-group form-checkbox-row">
      <input id="sitemaponly" type="checkbox" bind:checked={crawlSitemapOnly} /><label for="sitemaponly" class="form-checkbox-label">{t('newCrawl.sitemapOnly')}</label>
    </div>
    {#if checkExternalLinks}
      <div class="form-group"><label for="extworkers">{t('newCrawl.extWorkers')}</label><input id="extworkers" type="number" bind:value={externalLinkWorkers} min="1" max="20" /></div>
    {/if}
    {#if projects.length > 0}
      <div class="form-group">
        <label for="crawl-project">{t('newCrawl.project')}</label>
        <select id="crawl-project" bind:value={crawlProjectId}>
          <option value="">{t('newCrawl.noProject')}</option>
          {#each projects as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
      </div>
    {/if}
  </div>
  <div class="form-actions">
    <button class="btn btn-primary" onclick={handleStartCrawl} disabled={starting || !seedInput.trim()}>
      {starting ? t('newCrawl.starting') : t('newCrawl.startCrawl')}
    </button>
    <button class="btn" onclick={oncancel}>{t('common.cancel')}</button>
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
