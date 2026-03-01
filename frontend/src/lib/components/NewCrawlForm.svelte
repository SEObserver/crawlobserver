<script>
  import { t } from '../i18n/index.svelte.js';
  import { startCrawl, checkIP } from '../api.js';
  import SearchSelect from './SearchSelect.svelte';

  let { projects = [], initialProjectId = '', onstart, oncancel, onerror } = $props();

  let seedInput = $state('');
  let maxPages = $state(0);
  let maxDepth = $state(0);
  let workers = $state(10);
  let crawlDelay = $state('1s');
  let storeHtml = $state(false);
  let crawlScope = $state('host');
  let crawlProjectId = $state(initialProjectId);
  let checkExternalLinks = $state(true);
  let externalLinkWorkers = $state(3);
  let userAgentPreset = $state('');
  let userAgentCustom = $state('');
  let crawlSitemapOnly = $state(false);
  let tlsProfile = $state('');
  let jsRenderMode = $state('off');
  let jsRenderMaxPages = $state(4);
  let followJSLinks = $state(false);
  let sourceIP = $state('');
  let forceIPv4 = $state(false);
  let checkingIP = $state(false);
  let checkedIP = $state('');
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

  async function handleCheckIP() {
    checkingIP = true;
    checkedIP = '';
    try {
      const res = await checkIP(sourceIP, forceIPv4);
      checkedIP = res.ip;
    } catch (e) {
      checkedIP = '';
      onerror?.(e.message);
    } finally {
      checkingIP = false;
    }
  }

  async function handleStartCrawl() {
    const seeds = seedInput.split('\n').map(s => s.trim()).filter(Boolean);
    if (seeds.length === 0) return;
    starting = true;
    const ua = userAgentPreset === 'custom' ? userAgentCustom : userAgentPreset;
    try {
      await startCrawl(seeds, { max_pages: maxPages, max_depth: maxDepth, workers, delay: crawlDelay, store_html: storeHtml, crawl_scope: crawlScope, project_id: crawlProjectId || null, check_external_links: checkExternalLinks, external_link_workers: externalLinkWorkers, user_agent: ua || undefined, crawl_sitemap_only: crawlSitemapOnly, tls_profile: tlsProfile || undefined, source_ip: sourceIP || undefined, force_ipv4: forceIPv4 || undefined, js_render_mode: jsRenderMode !== 'off' ? jsRenderMode : undefined, js_render_max_pages: jsRenderMode !== 'off' ? jsRenderMaxPages : undefined, follow_js_links: jsRenderMode !== 'off' ? followJSLinks : undefined });
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
      <SearchSelect id="scope" bind:value={crawlScope} options={[
        { value: 'host', label: t('newCrawl.sameHost') },
        { value: 'domain', label: t('newCrawl.sameDomain') },
      ]} />
    </div>
    <div class="form-group">
      <label for="useragent">{t('newCrawl.userAgent')}</label>
      <SearchSelect id="useragent" bind:value={userAgentPreset} onchange={onUserAgentChange} options={userAgentPresets.map(p => ({ value: p.value, label: p.label }))} />
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
        <SearchSelect id="tlsprofile" bind:value={tlsProfile} options={[
          { value: '', label: t('newCrawl.tlsOff') },
          { value: 'chrome', label: t('newCrawl.tlsChrome') },
          { value: 'firefox', label: t('newCrawl.tlsFirefox') },
          { value: 'edge', label: t('newCrawl.tlsEdge') },
        ]} />
      </div>
    {/if}
    <div class="form-group">
      <label for="sourceip">{t('newCrawl.sourceIP')}</label>
      <div class="input-with-btn">
        <input id="sourceip" type="text" bind:value={sourceIP} placeholder={t('newCrawl.sourceIPPlaceholder')} />
        <button class="btn btn-sm" onclick={handleCheckIP} disabled={checkingIP}>
          {checkingIP ? t('newCrawl.checking') : t('newCrawl.checkIP')}
        </button>
        {#if checkedIP}
          <span class="badge badge-info">{checkedIP}</span>
        {/if}
      </div>
    </div>
    <div class="form-group form-checkbox-row">
      <input id="forceipv4" type="checkbox" bind:checked={forceIPv4} /><label for="forceipv4" class="form-checkbox-label">{t('newCrawl.forceIPv4')}</label>
    </div>
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
    <div class="form-group">
      <label for="jsrender">{t('newCrawl.jsRender')}</label>
      <SearchSelect id="jsrender" bind:value={jsRenderMode} options={[
        { value: 'off', label: t('newCrawl.jsRenderOff') },
        { value: 'auto', label: t('newCrawl.jsRenderAuto') },
        { value: 'always', label: t('newCrawl.jsRenderAlways') },
      ]} />
    </div>
    {#if jsRenderMode !== 'off'}
      <div class="form-group"><label for="jsworkers">{t('newCrawl.jsRenderWorkers')}</label><input id="jsworkers" type="number" bind:value={jsRenderMaxPages} min="1" max="8" /></div>
      <div class="form-group form-checkbox-row">
        <input id="followjs" type="checkbox" bind:checked={followJSLinks} /><label for="followjs" class="form-checkbox-label">{t('newCrawl.followJSLinks')}</label>
      </div>
    {/if}
    {#if projects.length > 0}
      <div class="form-group">
        <label for="crawl-project">{t('newCrawl.project')}</label>
        <SearchSelect id="crawl-project" bind:value={crawlProjectId}
          placeholder={t('newCrawl.noProject')}
          options={[{ value: '', label: t('newCrawl.noProject') }, ...projects.map(p => ({ value: p.id, label: p.name }))]}
          onsearch={projects.length > 20 ? async (q) => {
            const lq = q.toLowerCase();
            return [{ value: '', label: t('newCrawl.noProject') }, ...projects.filter(p => p.name.toLowerCase().includes(lq)).map(p => ({ value: p.id, label: p.name }))];
          } : undefined} />
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
  .input-with-btn {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .input-with-btn input { flex: 1; }
  .badge-info {
    font-size: 0.8rem;
    padding: 2px 8px;
    border-radius: 4px;
    background: var(--accent, #7c3aed);
    color: #fff;
    white-space: nowrap;
  }
  .form-actions {
    display: flex;
    gap: 8px;
    margin-top: 20px;
  }
</style>
