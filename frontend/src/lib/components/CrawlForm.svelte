<script>
  import { t } from '../i18n/index.svelte.js';
  import { startCrawl, resumeCrawl, retryFailed, checkIP, getExtractorSets } from '../api.js';
  import SearchSelect from './SearchSelect.svelte';

  let {
    mode = 'new',
    session = null,
    projects = [],
    initialProjectId = '',
    retryStatusCode = 0,
    retryCount = 0,
    onsubmit,
    oncancel,
    onerror,
  } = $props();

  const isNew = mode === 'new';
  const isRetry = mode === 'retry';

  // --- Init from session config (resume/retry) ---
  let crawlerCfg = {};
  if (!isNew && session?.Config) {
    try {
      const parsed = typeof session.Config === 'string' ? JSON.parse(session.Config) : session.Config;
      crawlerCfg = parsed?.Crawler || {};
    } catch {}
  }

  function nsToMs(ns) {
    if (ns == null || ns < 0) return 1000;
    return Math.round(ns / 1e6);
  }

  // --- UA preset detection (for resume/retry) ---
  const knownUAs = [
    { value: '', tls: '' },
    { value: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)', tls: '' },
    {
      value:
        'Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.69 Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
      tls: '',
    },
    { value: 'Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)', tls: '' },
    {
      value:
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
      tls: 'chrome',
    },
  ];

  function detectUAPreset() {
    if (isNew) return { preset: '', custom: '' };
    const storedUA = crawlerCfg.UserAgent || '';
    const matchesKnown = knownUAs.some((p) => p.value === storedUA);
    return {
      preset: matchesKnown ? storedUA : storedUA ? 'custom' : '',
      custom: matchesKnown ? '' : storedUA,
    };
  }

  const detectedUA = detectUAPreset();

  // --- Form state ---
  let seedInput = $state(isNew ? '' : (session?.SeedURLs?.join('\n') || ''));
  let workers = $state(isNew ? 10 : (crawlerCfg.Workers || 10));
  let crawlDelayMs = $state(isNew ? 1000 : nsToMs(crawlerCfg.Delay));
  let maxPages = $state(isNew ? 0 : (crawlerCfg.MaxPages || 0));
  let maxDepth = $state(isNew ? 0 : (crawlerCfg.MaxDepth || 0));
  let storeHtml = $state(isNew ? false : (crawlerCfg.StoreHTML || false));
  let crawlScope = $state(isNew ? 'host' : (crawlerCfg.CrawlScope || 'host'));
  let crawlProjectId = $state(isNew ? initialProjectId : (session?.ProjectID || ''));
  let checkExternalLinks = $state(true);
  let externalLinkWorkers = $state(3);
  let userAgentPreset = $state(detectedUA.preset);
  let userAgentCustom = $state(detectedUA.custom);
  let crawlSitemapOnly = $state(false);
  let fetchSitemaps = $state(isNew ? true : false);
  let tlsProfile = $state(isNew ? '' : (crawlerCfg.TLSProfile || ''));
  let jsRenderMode = $state(isNew ? 'off' : (crawlerCfg.JSRender?.Mode || 'off'));
  let jsRenderMaxPages = $state(isNew ? 4 : (crawlerCfg.JSRender?.MaxPages || 4));
  let followJSLinks = $state(false);
  let sourceIP = $state(isNew ? '' : (crawlerCfg.SourceIP || ''));
  let forceIPv4 = $state(isNew ? false : (crawlerCfg.ForceIPv4 || false));
  let ignoreRobots = $state(isNew ? false : false);
  let excludePatternsInput = $state(isNew ? '' : (crawlerCfg.ExcludePatterns || []).join('\n'));
  let extractorSetId = $state('');
  let extractorSets = $state([]);
  let checkingIP = $state(false);
  let checkedIP = $state('');
  let submitting = $state(false);

  // Load extractor sets on mount
  getExtractorSets()
    .then((sets) => { extractorSets = sets; })
    .catch(() => {});

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
      delay: `${crawlDelayMs}ms`,
      store_html: storeHtml,
      crawl_scope: crawlScope,
      project_id: crawlProjectId || null,
      check_external_links: checkExternalLinks,
      external_link_workers: externalLinkWorkers,
      user_agent: ua || undefined,
      crawl_sitemap_only: crawlSitemapOnly,
      fetch_sitemaps: crawlSitemapOnly ? true : fetchSitemaps,
      tls_profile: tlsProfile || undefined,
      source_ip: sourceIP || undefined,
      force_ipv4: forceIPv4 || undefined,
      js_render_mode: jsRenderMode !== 'off' ? jsRenderMode : undefined,
      js_render_max_pages: jsRenderMode !== 'off' ? jsRenderMaxPages : undefined,
      follow_js_links: jsRenderMode !== 'off' ? followJSLinks : undefined,
      extractor_set_id: extractorSetId || undefined,
      ignore_robots: ignoreRobots || undefined,
      exclude_patterns: excludePatternsInput.trim()
        ? excludePatternsInput.split('\n').map((s) => s.trim()).filter(Boolean)
        : undefined,
    };
  }

  async function handleSubmit() {
    submitting = true;
    try {
      if (isNew) {
        const seeds = seedInput
          .split('\n')
          .map((s) => s.trim())
          .filter(Boolean)
          .map((s) => (/^https?:\/\//i.test(s) ? s : `http://${s}`));
        if (seeds.length === 0) { submitting = false; return; }
        await startCrawl(seeds, buildOptions());
      } else if (isRetry) {
        await retryFailed(session.ID, retryStatusCode, buildOptions());
      } else {
        await resumeCrawl(session.ID, buildOptions());
      }
      onsubmit?.();
    } catch (e) {
      onerror?.(e.message);
    } finally {
      submitting = false;
    }
  }

  // Submit button labels
  let submitLabel = $derived(
    isNew
      ? (submitting ? t('newCrawl.starting') : t('newCrawl.startCrawl'))
      : isRetry
        ? (submitting ? t('resumeModal.retryingBtn') : t('resumeModal.retryBtn'))
        : (submitting ? t('resumeModal.resuming') : t('resumeModal.resume'))
  );

  let submitDisabled = $derived(submitting || (isNew && !seedInput.trim()));
</script>

{#if isNew}
  <div class="page-header">
    <h1>{t('newCrawl.title')}</h1>
  </div>
{/if}
<div class={isNew ? 'card' : ''}>
  {#if isRetry && retryCount > 0}
    <div class="retry-info">
      {t('resumeModal.retryInfo', { count: retryCount, status: retryStatusCode || '0' })}
    </div>
  {/if}

  <!-- Seed URLs -->
  <div class="form-group">
    <label for="cf-seeds">{t('newCrawl.seedUrls')}</label>
    {#if isNew}
      <textarea id="cf-seeds" bind:value={seedInput} rows="3" placeholder="https://example.com"></textarea>
    {:else}
      <textarea id="cf-seeds" rows="2" disabled value={seedInput}></textarea>
    {/if}
  </div>

  <!-- Main grid: Workers, Delay, MaxPages, MaxDepth, Scope, Project -->
  <div class="form-grid form-section">
    <div class="form-group">
      <label for="cf-workers">{t('newCrawl.workers')}</label>
      <input id="cf-workers" type="number" bind:value={workers} min="1" max="100" />
    </div>
    <div class="form-group">
      <label for="cf-delay">{t('newCrawl.delay')} (ms)</label>
      <input id="cf-delay" type="number" bind:value={crawlDelayMs} min="0" step="100" />
    </div>
    {#if !isRetry}
      <div class="form-group">
        <label for="cf-maxpages">{t('newCrawl.maxPages')}</label>
        <input id="cf-maxpages" type="number" bind:value={maxPages} min="0" />
      </div>
      <div class="form-group">
        <label for="cf-maxdepth">{t('newCrawl.maxDepth')}</label>
        <input id="cf-maxdepth" type="number" bind:value={maxDepth} min="0" />
      </div>
    {/if}
    <div class="form-group">
      <label for="cf-scope">{t('newCrawl.crawlScope')}</label>
      <SearchSelect
        id="cf-scope"
        bind:value={crawlScope}
        options={[
          { value: 'host', label: t('newCrawl.sameHost') },
          { value: 'domain', label: t('newCrawl.sameDomain') },
          { value: 'subdirectory', label: t('newCrawl.sameSubdirectory') },
        ]}
      />
    </div>
    {#if projects.length > 0}
      <div class="form-group">
        <label for="cf-project">{t('newCrawl.project')}</label>
        <SearchSelect
          id="cf-project"
          bind:value={crawlProjectId}
          placeholder={t('newCrawl.noProject')}
          options={[
            { value: '', label: t('newCrawl.noProject') },
            ...projects.map((p) => ({ value: p.id, label: p.name })),
          ]}
          onsearch={projects.length > 20
            ? async (q) => {
                const lq = q.toLowerCase();
                return [
                  { value: '', label: t('newCrawl.noProject') },
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

  <!-- Identity & Network -->
  <details class="collapsible">
    <summary>{t('crawlForm.identityNetwork')}</summary>
    <div class="collapsible-content">
      <div class="form-grid">
        <div class="form-group">
          <label for="cf-useragent">{t('newCrawl.userAgent')}</label>
          <SearchSelect
            id="cf-useragent"
            bind:value={userAgentPreset}
            onchange={onUserAgentChange}
            options={userAgentPresets.map((p) => ({ value: p.value, label: p.label }))}
          />
        </div>
        {#if userAgentPreset === 'custom'}
          <div class="form-group">
            <label for="cf-useragent-custom">{t('newCrawl.customUserAgent')}</label>
            <input
              id="cf-useragent-custom"
              type="text"
              bind:value={userAgentCustom}
              placeholder="Mozilla/5.0 ..."
            />
          </div>
        {/if}
        {#if userAgentPreset !== ''}
          <div class="form-group">
            <label for="cf-tlsprofile">{t('newCrawl.tlsFingerprint')}</label>
            <SearchSelect
              id="cf-tlsprofile"
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
      <div class="ip-section">
        <div class="form-group">
          <label for="cf-sourceip">{t('newCrawl.sourceIP')}</label>
          <div class="input-with-btn">
            <input
              id="cf-sourceip"
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
    </div>
  </details>

  <!-- Advanced Options -->
  <details class="collapsible">
    <summary>{t('crawlForm.advancedOptions')}</summary>
    <div class="collapsible-content">
      <div class="options-row">
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
            <label for="cf-extworkers">{t('newCrawl.extWorkers')}</label>
            <input id="cf-extworkers" type="number" bind:value={externalLinkWorkers} min="1" max="20" />
          </div>
        {/if}
        <label class="inline-checkbox">
          <input type="checkbox" bind:checked={crawlSitemapOnly} />
          {t('newCrawl.sitemapOnly')}
        </label>
        <label class="inline-checkbox">
          <input type="checkbox" bind:checked={fetchSitemaps} disabled={crawlSitemapOnly} />
          {t('newCrawl.fetchSitemaps')}
        </label>
        <label class="inline-checkbox">
          <input type="checkbox" bind:checked={ignoreRobots} />
          {t('newCrawl.ignoreRobots')}
        </label>
      </div>

      <div class="form-group" style="margin-top: 12px;">
        <label for="cf-excludePatterns">{t('newCrawl.excludePatterns')}</label>
        <textarea
          id="cf-excludePatterns"
          bind:value={excludePatternsInput}
          rows="3"
          placeholder={t('newCrawl.excludePatternsPlaceholder')}
        ></textarea>
        <p class="form-hint">{t('newCrawl.excludePatternsHint')}</p>
      </div>

      <div class="form-grid" style="margin-top: 12px;">
        <div class="form-group">
          <label for="cf-jsrender">{t('newCrawl.jsRender')}</label>
          <SearchSelect
            id="cf-jsrender"
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
            <label for="cf-jsworkers">{t('newCrawl.jsRenderWorkers')}</label>
            <input id="cf-jsworkers" type="number" bind:value={jsRenderMaxPages} min="1" max="8" />
          </div>
          <label class="inline-checkbox inline-checkbox-align">
            <input type="checkbox" bind:checked={followJSLinks} />
            {t('newCrawl.followJSLinks')}
          </label>
        {/if}
        {#if extractorSets.length > 0}
          <div class="form-group">
            <label for="cf-extractor-set">{t('newCrawl.extractorSet')}</label>
            <SearchSelect
              id="cf-extractor-set"
              bind:value={extractorSetId}
              placeholder={t('newCrawl.noExtractorSet')}
              options={[
                { value: '', label: t('newCrawl.noExtractorSet') },
                ...extractorSets.map((es) => ({ value: es.id, label: es.name })),
              ]}
            />
          </div>
        {/if}
      </div>
    </div>
  </details>

  <div class="form-actions">
    <button class="btn btn-primary" onclick={handleSubmit} disabled={submitDisabled}>
      {submitLabel}
    </button>
    <button class="btn" onclick={oncancel}>{t('common.cancel')}</button>
  </div>
</div>

<style>
  .form-section {
    margin-top: 16px;
  }
  .collapsible {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-top: 16px;
  }
  .collapsible summary {
    padding: 10px 14px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
    cursor: pointer;
    user-select: none;
  }
  .collapsible[open] summary {
    border-bottom: 1px solid var(--border);
  }
  .collapsible-content {
    padding: 16px;
  }
  .ip-section {
    display: flex;
    align-items: flex-end;
    gap: 16px;
    margin-top: 12px;
  }
  .ip-section .form-group {
    flex: 1;
    max-width: 400px;
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
  .retry-info {
    background: var(--bg-hover);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 10px 14px;
    font-size: 13px;
    color: var(--text-secondary);
    margin-bottom: 16px;
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
