<script>
  import { resumeCrawl, retryFailed, checkIP, getExtractorSets } from '../api.js';
  import { a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';

  let {
    sessionId,
    sessions,
    projects = [],
    mode = 'resume',
    retryStatusCode = 0,
    retryCount = 0,
    onresume,
    onclose,
    onerror,
  } = $props();

  const isRetry = mode === 'retry';

  // Init form from session config
  const sess = sessions.find((s) => s.ID === sessionId);
  let crawlerCfg = {};
  if (sess?.Config) {
    try {
      const parsed = typeof sess.Config === 'string' ? JSON.parse(sess.Config) : sess.Config;
      crawlerCfg = parsed?.Crawler || {};
    } catch {}
  }

  // Convert Go time.Duration (nanoseconds) to human string
  function nsToDelay(ns) {
    if (!ns || ns <= 0) return '1s';
    const ms = ns / 1e6;
    if (ms < 1000) return `${Math.round(ms)}ms`;
    const s = ms / 1000;
    if (Number.isInteger(s)) return `${s}s`;
    return `${s.toFixed(1)}s`;
  }

  // --- Form state, pre-filled from original config ---
  let workers = $state(crawlerCfg.Workers || 10);
  let crawlDelay = $state(nsToDelay(crawlerCfg.Delay));
  let maxPages = $state(crawlerCfg.MaxPages || 0);
  let maxDepth = $state(crawlerCfg.MaxDepth || 0);
  let storeHtml = $state(crawlerCfg.StoreHTML || false);
  let crawlScope = $state(crawlerCfg.CrawlScope || 'host');
  let tlsProfile = $state(crawlerCfg.TLSProfile || '');
  let sourceIP = $state(crawlerCfg.SourceIP || '');
  let forceIPv4 = $state(crawlerCfg.ForceIPv4 || false);
  let jsRenderMode = $state(crawlerCfg.JSRender?.Mode || 'off');
  let jsRenderMaxPages = $state(crawlerCfg.JSRender?.MaxPages || 4);
  let followJSLinks = $state(false);
  let checkExternalLinks = $state(true);
  let externalLinkWorkers = $state(3);
  let crawlSitemapOnly = $state(false);
  let fetchSitemaps = $state(false);
  let extractorSetId = $state('');
  let extractorSets = $state([]);
  let crawlProjectId = $state(sess?.ProjectID || '');

  // UA preset detection
  const knownUAs = [
    { value: '', tls: '' },
    {
      value: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
      tls: '',
    },
    {
      value:
        'Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.69 Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
      tls: '',
    },
    {
      value: 'Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)',
      tls: '',
    },
    {
      value:
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
      tls: 'chrome',
    },
  ];
  const storedUA = crawlerCfg.UserAgent || '';
  const matchesKnown = knownUAs.some((p) => p.value === storedUA);
  let userAgentPreset = $state(matchesKnown ? storedUA : storedUA ? 'custom' : '');
  let userAgentCustom = $state(matchesKnown ? '' : storedUA);

  let userAgentPresets = $derived([
    { label: t('newCrawl.uaDefault'), value: '', tls: '' },
    {
      label: t('newCrawl.uaGooglebotDesktop'),
      value: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
      tls: '',
    },
    {
      label: t('newCrawl.uaGooglebotMobile'),
      value:
        'Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.69 Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
      tls: '',
    },
    {
      label: t('newCrawl.uaBingbot'),
      value: 'Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)',
      tls: '',
    },
    {
      label: t('newCrawl.uaChromeDesktop'),
      value:
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
      tls: 'chrome',
    },
    { label: t('newCrawl.uaCustom'), value: 'custom', tls: '' },
  ]);

  function onUserAgentChange() {
    const preset = userAgentPresets.find((p) => p.value === userAgentPreset);
    if (preset) tlsProfile = preset.tls;
  }

  let checkingIP = $state(false);
  let checkedIP = $state('');
  let submitting = $state(false);

  const seedUrls = sess?.SeedURLs?.join('\n') || '';

  getExtractorSets()
    .then((sets) => {
      extractorSets = sets;
    })
    .catch(() => {});

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

  function buildOptions() {
    const ua = userAgentPreset === 'custom' ? userAgentCustom : userAgentPreset;
    return {
      max_pages: maxPages,
      max_depth: maxDepth,
      workers,
      delay: crawlDelay,
      store_html: storeHtml,
      crawl_scope: crawlScope,
      user_agent: ua || undefined,
      tls_profile: tlsProfile || undefined,
      source_ip: sourceIP || undefined,
      force_ipv4: forceIPv4 || undefined,
      check_external_links: checkExternalLinks,
      external_link_workers: externalLinkWorkers,
      crawl_sitemap_only: crawlSitemapOnly,
      fetch_sitemaps: crawlSitemapOnly ? true : fetchSitemaps,
      js_render_mode: jsRenderMode !== 'off' ? jsRenderMode : undefined,
      js_render_max_pages: jsRenderMode !== 'off' ? jsRenderMaxPages : undefined,
      follow_js_links: jsRenderMode !== 'off' ? followJSLinks : undefined,
      extractor_set_id: extractorSetId || undefined,
    };
  }

  async function handleSubmit() {
    submitting = true;
    try {
      if (isRetry) {
        await retryFailed(sessionId, retryStatusCode, buildOptions());
      } else {
        await resumeCrawl(sessionId, buildOptions());
      }
      onresume?.();
    } catch (e) {
      onerror?.(e.message);
    } finally {
      submitting = false;
    }
  }
</script>

<div
  class="modal-overlay"
  role="button"
  tabindex="0"
  onclick={onclose}
  onkeydown={a11yKeydown(onclose)}
>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="modal-dialog"
    role="dialog"
    tabindex="-1"
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => e.stopPropagation()}
  >
    <div class="modal-header">
      <h2>
        {#if isRetry}
          {t('resumeModal.retryTitle')}
        {:else}
          {t('resumeModal.title')}
        {/if}
      </h2>
      <button class="btn btn-sm" title={t('common.close')} onclick={onclose}>
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg
        >
      </button>
    </div>
    <div class="modal-body">
      {#if isRetry && retryCount > 0}
        <div class="retry-info">
          {t('resumeModal.retryInfo', {
            count: retryCount,
            status: retryStatusCode || '0',
          })}
        </div>
      {/if}

      <!-- Seed URLs (read-only) -->
      <div class="form-group form-full-width">
        <label for="r-seeds">{t('newCrawl.seedUrls')}</label>
        <textarea id="r-seeds" rows="2" disabled value={seedUrls}></textarea>
      </div>

      <!-- Crawl parameters -->
      <div class="form-grid form-section">
        <div class="form-group">
          <label for="r-workers">{t('newCrawl.workers')}</label>
          <input id="r-workers" type="number" bind:value={workers} min="1" max="100" />
        </div>
        <div class="form-group">
          <label for="r-delay">{t('newCrawl.delay')}</label>
          <input id="r-delay" type="text" bind:value={crawlDelay} placeholder="1s" />
        </div>
        {#if !isRetry}
          <div class="form-group">
            <label for="r-maxpages">{t('newCrawl.maxPages')}</label>
            <input id="r-maxpages" type="number" bind:value={maxPages} min="0" />
          </div>
          <div class="form-group">
            <label for="r-maxdepth">{t('newCrawl.maxDepth')}</label>
            <input id="r-maxdepth" type="number" bind:value={maxDepth} min="0" />
          </div>
        {/if}
        <div class="form-group">
          <label for="r-scope">{t('newCrawl.crawlScope')}</label>
          <SearchSelect
            id="r-scope"
            bind:value={crawlScope}
            options={[
              { value: 'host', label: t('resumeModal.sameHost') },
              { value: 'domain', label: t('resumeModal.sameDomain') },
              { value: 'subdirectory', label: t('resumeModal.sameSubdirectory') },
            ]}
          />
        </div>
        <div class="form-group">
          <label for="r-useragent">{t('newCrawl.userAgent')}</label>
          <SearchSelect
            id="r-useragent"
            bind:value={userAgentPreset}
            onchange={onUserAgentChange}
            options={userAgentPresets.map((p) => ({ value: p.value, label: p.label }))}
          />
        </div>
        {#if userAgentPreset === 'custom'}
          <div class="form-group">
            <label for="r-useragent-custom">{t('newCrawl.customUserAgent')}</label>
            <input
              id="r-useragent-custom"
              type="text"
              bind:value={userAgentCustom}
              placeholder="Mozilla/5.0 ..."
            />
          </div>
        {/if}
        {#if userAgentPreset !== ''}
          <div class="form-group">
            <label for="r-tlsprofile">{t('newCrawl.tlsFingerprint')}</label>
            <SearchSelect
              id="r-tlsprofile"
              bind:value={tlsProfile}
              options={[
                { value: '', label: t('newCrawl.tlsOff') },
                { value: 'chrome', label: t('newCrawl.tlsChrome') },
                { value: 'firefox', label: t('newCrawl.tlsFirefox') },
                { value: 'edge', label: t('newCrawl.tlsEdge') },
              ]}
            />
          </div>
        {/if}
      </div>

      <!-- Outgoing IP -->
      <div class="form-section ip-section">
        <div class="form-group">
          <label for="r-sourceip">{t('newCrawl.sourceIP')}</label>
          <div class="input-with-btn">
            <input
              id="r-sourceip"
              type="text"
              bind:value={sourceIP}
              placeholder={t('newCrawl.sourceIPPlaceholder')}
            />
            <button class="btn btn-sm" onclick={handleCheckIP} disabled={checkingIP}>
              {checkingIP ? t('newCrawl.checking') : t('newCrawl.checkIP')}
            </button>
            {#if checkedIP}
              <span class="badge badge-info">{checkedIP}</span>
            {/if}
          </div>
        </div>
        <label class="inline-checkbox">
          <input type="checkbox" bind:checked={forceIPv4} />
          {t('newCrawl.forceIPv4')}
        </label>
      </div>

      <!-- Options -->
      <div class="form-section options-row">
        <label class="inline-checkbox">
          <input type="checkbox" bind:checked={storeHtml} />
          {t('newCrawl.storeHtml')}
        </label>
        <label class="inline-checkbox">
          <input type="checkbox" bind:checked={checkExternalLinks} />
          {t('newCrawl.checkExternal')}
        </label>
        {#if checkExternalLinks}
          <div class="form-group form-group-inline">
            <label for="r-extworkers">{t('newCrawl.extWorkers')}</label>
            <input
              id="r-extworkers"
              type="number"
              bind:value={externalLinkWorkers}
              min="1"
              max="20"
            />
          </div>
        {/if}
        {#if !isRetry}
          <label class="inline-checkbox">
            <input type="checkbox" bind:checked={crawlSitemapOnly} />
            {t('newCrawl.sitemapOnly')}
          </label>
          <label class="inline-checkbox">
            <input type="checkbox" bind:checked={fetchSitemaps} disabled={crawlSitemapOnly} />
            {t('newCrawl.fetchSitemaps')}
          </label>
        {/if}
      </div>

      <!-- JS Rendering / Extractors / Project -->
      <div class="form-grid form-section">
        <div class="form-group">
          <label for="r-jsrender">{t('newCrawl.jsRender')}</label>
          <SearchSelect
            id="r-jsrender"
            bind:value={jsRenderMode}
            options={[
              { value: 'off', label: t('newCrawl.jsRenderOff') },
              { value: 'auto', label: t('newCrawl.jsRenderAuto') },
              { value: 'always', label: t('newCrawl.jsRenderAlways') },
            ]}
          />
        </div>
        {#if jsRenderMode !== 'off'}
          <div class="form-group">
            <label for="r-jsworkers">{t('newCrawl.jsRenderWorkers')}</label>
            <input id="r-jsworkers" type="number" bind:value={jsRenderMaxPages} min="1" max="8" />
          </div>
          <label class="inline-checkbox inline-checkbox-align">
            <input type="checkbox" bind:checked={followJSLinks} />
            {t('newCrawl.followJSLinks')}
          </label>
        {/if}
        {#if extractorSets.length > 0}
          <div class="form-group">
            <label for="r-extractor-set">{t('newCrawl.extractorSet')}</label>
            <SearchSelect
              id="r-extractor-set"
              bind:value={extractorSetId}
              placeholder={t('newCrawl.noExtractorSet')}
              options={[
                { value: '', label: t('newCrawl.noExtractorSet') },
                ...extractorSets.map((es) => ({ value: es.id, label: es.name })),
              ]}
            />
          </div>
        {/if}
        {#if projects.length > 0}
          <div class="form-group">
            <label for="r-project">{t('newCrawl.project')}</label>
            <SearchSelect
              id="r-project"
              bind:value={crawlProjectId}
              placeholder={t('newCrawl.noProject')}
              options={[
                { value: '', label: t('newCrawl.noProject') },
                ...projects.map((p) => ({ value: p.id, label: p.name })),
              ]}
            />
          </div>
        {/if}
      </div>

      <div class="form-actions">
        <button class="btn btn-primary" onclick={handleSubmit} disabled={submitting}>
          {#if isRetry}
            {submitting ? t('resumeModal.retryingBtn') : t('resumeModal.retryBtn')}
          {:else}
            {submitting ? t('resumeModal.resuming') : t('resumeModal.resume')}
          {/if}
        </button>
        <button class="btn" onclick={onclose}>{t('common.cancel')}</button>
      </div>
    </div>
  </div>
</div>

<style>
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }
  .modal-dialog {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
    width: 100%;
    max-width: 640px;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .modal-header h2 {
    font-size: 15px;
    font-weight: 600;
    margin: 0;
    color: var(--text);
  }
  .modal-body {
    padding: 20px;
    overflow-y: auto;
  }
  .retry-info {
    background: var(--bg-hover);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 10px 14px;
    font-size: 13px;
    color: var(--text-secondary);
    margin-bottom: 16px;
  }
  .form-full-width {
    grid-column: 1 / -1;
  }
  .form-section {
    margin-top: 16px;
  }
  .ip-section {
    display: flex;
    align-items: flex-end;
    gap: 16px;
  }
  .ip-section .form-group {
    flex: 1;
  }
  .ip-section .inline-checkbox {
    margin-bottom: 8px;
  }
  .input-with-btn {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .input-with-btn input {
    flex: 1;
  }
  .options-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 20px;
  }
  .inline-checkbox {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--text-secondary);
    font-weight: 500;
    cursor: pointer;
    white-space: nowrap;
  }
  .inline-checkbox-align {
    align-self: flex-end;
    padding-bottom: 10px;
  }
  .form-group-inline {
    flex-direction: row;
    align-items: center;
  }
  .form-group-inline input {
    width: 70px;
  }
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
  textarea:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>
