<script>
  import { getPageHTML } from '../api.js';
  import { a11yKeydown } from '../utils.js';

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
        <button class="btn btn-sm" class:btn-primary={htmlModalView === 'render'} onclick={() => htmlModalView = 'render'}>Render</button>
        <button class="btn btn-sm" class:btn-primary={htmlModalView === 'source'} onclick={() => htmlModalView = 'source'}>Source</button>
        <button class="btn btn-sm" title="Close" onclick={onclose}>
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>
    </div>
    <div class="html-modal-body">
      {#if loading}
        <p style="padding: 40px; color: var(--text-muted); text-align: center;">Loading...</p>
      {:else if !htmlModalData.body_html}
        <p style="padding: 40px; color: var(--text-muted); text-align: center;">No HTML stored for this page.</p>
      {:else if htmlModalView === 'render'}
        <iframe srcdoc={htmlModalData.body_html} title="Page render" class="html-modal-iframe" sandbox></iframe>
      {:else}
        <pre class="html-modal-source"><code>{htmlModalData.body_html}</code></pre>
      {/if}
    </div>
  </div>
</div>
