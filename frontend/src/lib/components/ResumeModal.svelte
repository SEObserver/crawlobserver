<script>
  import { resumeCrawl } from '../api.js';
  import { a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { sessionId, sessions, onresume, onclose, onerror } = $props();

  // Init form from session config
  const sess = sessions.find(s => s.ID === sessionId);
  let cfg = {};
  if (sess?.Config) {
    try { cfg = typeof sess.Config === 'string' ? JSON.parse(sess.Config) : sess.Config; } catch {}
  }

  let resumeMaxPages = $state(cfg.max_pages || 0);
  let resumeMaxDepth = $state(cfg.max_depth || 0);
  let resumeWorkers = $state(cfg.workers || 10);
  let resumeDelay = $state(cfg.delay || '1s');
  let resumeStoreHtml = $state(cfg.store_html || false);
  let resumeCrawlScope = $state(cfg.crawl_scope || 'host');
  let resuming = $state(false);

  async function handleResume() {
    resuming = true;
    try {
      await resumeCrawl(sessionId, {
        max_pages: resumeMaxPages,
        max_depth: resumeMaxDepth,
        workers: resumeWorkers,
        delay: resumeDelay,
        store_html: resumeStoreHtml,
        crawl_scope: resumeCrawlScope,
      });
      onresume?.();
    } catch (e) {
      onerror?.(e.message);
    } finally {
      resuming = false;
    }
  }
</script>

<div class="html-modal-overlay" role="button" tabindex="0" onclick={onclose} onkeydown={a11yKeydown(onclose)}>
  <div class="html-modal resume-modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
    <div class="html-modal-header">
      <div class="html-modal-url">{t('resumeModal.title')}</div>
      <div class="html-modal-actions">
        <button class="btn btn-sm" title={t('common.close')} onclick={onclose}>
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>
    </div>
    <div class="modal-body">
      <div class="form-grid">
        <div class="form-group"><label for="r-maxpages">{t('resumeModal.maxPages')}</label><input id="r-maxpages" type="number" bind:value={resumeMaxPages} min="0" /></div>
        <div class="form-group"><label for="r-maxdepth">{t('resumeModal.maxDepth')}</label><input id="r-maxdepth" type="number" bind:value={resumeMaxDepth} min="0" /></div>
        <div class="form-group"><label for="r-workers">{t('resumeModal.workers')}</label><input id="r-workers" type="number" bind:value={resumeWorkers} min="1" max="100" /></div>
        <div class="form-group"><label for="r-delay">{t('resumeModal.delay')}</label><input id="r-delay" type="text" bind:value={resumeDelay} placeholder="1s" /></div>
        <div class="form-group">
          <label for="r-scope">{t('resumeModal.crawlScope')}</label>
          <select id="r-scope" bind:value={resumeCrawlScope}>
            <option value="host">{t('resumeModal.sameHost')}</option>
            <option value="domain">{t('resumeModal.sameDomain')}</option>
          </select>
        </div>
        <div class="form-group checkbox-row">
          <input id="r-storehtml" type="checkbox" bind:checked={resumeStoreHtml} /><label for="r-storehtml" class="mb-0">{t('resumeModal.storeHtml')}</label>
        </div>
      </div>
      <div class="modal-actions">
        <button class="btn btn-primary" onclick={handleResume} disabled={resuming}>
          {resuming ? t('resumeModal.resuming') : t('resumeModal.resume')}
        </button>
        <button class="btn" onclick={onclose}>{t('common.cancel')}</button>
      </div>
    </div>
  </div>
</div>

<style>
  .resume-modal {
    max-width: 480px;
    height: auto;
  }
  .modal-body {
    padding: 20px;
  }
  .checkbox-row {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
    padding-top: 24px;
  }
  .modal-actions {
    display: flex;
    gap: 8px;
    margin-top: 20px;
  }
</style>
