<script>
  import { resumeCrawl } from '../api.js';
  import { a11yKeydown } from '../utils.js';

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
  <div class="html-modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} style="max-width: 480px; height: auto;">
    <div class="html-modal-header">
      <div class="html-modal-url">Resume Crawl</div>
      <div class="html-modal-actions">
        <button class="btn btn-sm" title="Close" onclick={onclose}>
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>
    </div>
    <div style="padding: 20px;">
      <div class="form-grid">
        <div class="form-group"><label for="r-maxpages">Max pages (0 = unlimited)</label><input id="r-maxpages" type="number" bind:value={resumeMaxPages} min="0" /></div>
        <div class="form-group"><label for="r-maxdepth">Max depth (0 = unlimited)</label><input id="r-maxdepth" type="number" bind:value={resumeMaxDepth} min="0" /></div>
        <div class="form-group"><label for="r-workers">Workers</label><input id="r-workers" type="number" bind:value={resumeWorkers} min="1" max="100" /></div>
        <div class="form-group"><label for="r-delay">Delay</label><input id="r-delay" type="text" bind:value={resumeDelay} placeholder="1s" /></div>
        <div class="form-group">
          <label for="r-scope">Crawl scope</label>
          <select id="r-scope" bind:value={resumeCrawlScope}>
            <option value="host">Same host only</option>
            <option value="domain">Same domain (incl. subdomains)</option>
          </select>
        </div>
        <div class="form-group" style="display: flex; flex-direction: row; align-items: center; gap: 8px; padding-top: 24px;">
          <input id="r-storehtml" type="checkbox" bind:checked={resumeStoreHtml} /><label for="r-storehtml" style="margin: 0;">Store raw HTML</label>
        </div>
      </div>
      <div style="display: flex; gap: 8px; margin-top: 20px;">
        <button class="btn btn-primary" onclick={handleResume} disabled={resuming}>
          {resuming ? 'Resuming...' : 'Resume'}
        </button>
        <button class="btn" onclick={onclose}>Cancel</button>
      </div>
    </div>
  </div>
</div>
