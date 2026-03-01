<script>
  import { resumeCrawl } from '../api.js';
  import { a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';

  let { sessionId, sessions, onresume, onclose, onerror } = $props();

  // Init form from session config
  const sess = sessions.find((s) => s.ID === sessionId);
  let cfg = {};
  if (sess?.Config) {
    try {
      cfg = typeof sess.Config === 'string' ? JSON.parse(sess.Config) : sess.Config;
    } catch {}
  }

  let resumeMaxPages = $state(cfg.max_pages || 0);
  let resumeMaxDepth = $state(cfg.max_depth || 0);
  let resumeWorkers = $state(cfg.workers || 10);
  let resumeDelay = $state(cfg.delay || '1s');
  let resumeStoreHtml = $state(cfg.store_html || false);
  let resumeCrawlScope = $state(cfg.crawl_scope || 'host');
  let resuming = $state(false);

  const seedUrls = sess?.SeedURLs?.join('\n') || '';

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
      <h2>{t('resumeModal.title')}</h2>
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
      <div class="form-grid">
        <div class="form-group form-full-width">
          <label for="r-seeds">{t('newCrawl.seedUrls')}</label>
          <textarea id="r-seeds" rows="2" disabled value={seedUrls}></textarea>
        </div>
        <div class="form-group">
          <label for="r-workers">{t('newCrawl.workers')}</label><input
            id="r-workers"
            type="number"
            bind:value={resumeWorkers}
            min="1"
            max="100"
          />
        </div>
        <div class="form-group">
          <label for="r-delay">{t('newCrawl.delay')}</label><input
            id="r-delay"
            type="text"
            bind:value={resumeDelay}
            placeholder="1s"
          />
        </div>
        <div class="form-group">
          <label for="r-maxpages">{t('newCrawl.maxPages')}</label><input
            id="r-maxpages"
            type="number"
            bind:value={resumeMaxPages}
            min="0"
          />
        </div>
        <div class="form-group">
          <label for="r-maxdepth">{t('newCrawl.maxDepth')}</label><input
            id="r-maxdepth"
            type="number"
            bind:value={resumeMaxDepth}
            min="0"
          />
        </div>
        <div class="form-group">
          <label for="r-scope">{t('newCrawl.crawlScope')}</label>
          <SearchSelect
            id="r-scope"
            bind:value={resumeCrawlScope}
            options={[
              { value: 'host', label: t('resumeModal.sameHost') },
              { value: 'domain', label: t('resumeModal.sameDomain') },
            ]}
          />
        </div>
        <div class="form-group form-checkbox-row">
          <input id="r-storehtml" type="checkbox" bind:checked={resumeStoreHtml} /><label
            for="r-storehtml"
            class="form-checkbox-label">{t('resumeModal.storeHtml')}</label
          >
        </div>
      </div>
      <div class="form-actions">
        <button class="btn btn-primary" onclick={handleResume} disabled={resuming}>
          {resuming ? t('resumeModal.resuming') : t('resumeModal.resume')}
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
    max-width: 560px;
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
  }
  .modal-header h2 {
    font-size: 15px;
    font-weight: 600;
    margin: 0;
    color: var(--text);
  }
  .modal-body {
    padding: 20px;
  }
  .form-full-width {
    grid-column: 1 / -1;
  }
  .form-checkbox-row {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
    padding-top: 24px;
  }
  .form-checkbox-label {
    margin: 0;
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
