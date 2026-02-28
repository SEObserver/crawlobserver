<script>
  import { getPageHTML } from '../api.js';
  import { a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { sessionId, url: initialUrl, onclose, onerror } = $props();

  let htmlModalData = $state({ url: '', body_html: '' });
  let htmlModalView = $state('render');
  let loading = $state(true);

  async function load() {
    loading = true;
    htmlModalView = 'render';
    try {
      htmlModalData = await getPageHTML(sessionId, initialUrl);
    } catch (e) {
      htmlModalData = { url: initialUrl, body_html: '' };
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  load();
</script>

<div class="html-modal-overlay" role="button" tabindex="0" onclick={onclose} onkeydown={a11yKeydown(onclose)}>
  <div class="html-modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
    <div class="html-modal-header">
      <div class="html-modal-url" title={htmlModalData.url}>{htmlModalData.url}</div>
      <div class="html-modal-actions">
        <button class="btn btn-sm" class:btn-primary={htmlModalView === 'render'} onclick={() => htmlModalView = 'render'}>{t('htmlModal.render')}</button>
        <button class="btn btn-sm" class:btn-primary={htmlModalView === 'source'} onclick={() => htmlModalView = 'source'}>{t('htmlModal.source')}</button>
        <button class="btn btn-sm" title={t('common.close')} onclick={onclose}>
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>
    </div>
    <div class="html-modal-body">
      {#if loading}
        <p class="modal-placeholder">{t('common.loading')}</p>
      {:else if !htmlModalData.body_html}
        <p class="modal-placeholder">{t('htmlModal.noHtml')}</p>
      {:else if htmlModalView === 'render'}
        <iframe srcdoc={htmlModalData.body_html} title={t('htmlModal.pageRender')} class="html-modal-iframe" sandbox></iframe>
      {:else}
        <pre class="html-modal-source"><code>{htmlModalData.body_html}</code></pre>
      {/if}
    </div>
  </div>
</div>

<style>
  .html-modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.5);
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }
  .html-modal {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
    width: 100%;
    max-width: 1200px;
    height: 85vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .html-modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border);
    gap: 16px;
    flex-shrink: 0;
  }
  .html-modal-url {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    flex: 1;
  }
  .html-modal-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }
  .html-modal-body {
    flex: 1;
    overflow: auto;
    min-height: 0;
  }
  .html-modal-iframe {
    width: 100%;
    height: 100%;
    border: none;
    background: #fff;
  }
  .html-modal-source {
    margin: 0;
    padding: 20px;
    font-size: 12px;
    line-height: 1.6;
    background: var(--bg);
    color: var(--text);
    overflow: auto;
    height: 100%;
    white-space: pre-wrap;
    word-break: break-all;
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
  }
  .modal-placeholder {
    padding: 40px;
    color: var(--text-muted);
    text-align: center;
  }
</style>
