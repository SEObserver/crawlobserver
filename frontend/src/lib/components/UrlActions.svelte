<script>
  import { copyToClipboard } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { url } = $props();
  let copied = $state(false);

  function handleCopy(e) {
    e.stopPropagation();
    e.preventDefault();
    copyToClipboard(url);
    copied = true;
    setTimeout(() => (copied = false), 1500);
  }

  function handleOpen(e) {
    e.stopPropagation();
  }
</script>

<span class="url-actions">
  <button class="url-action-btn" title={t('common.copyUrl')} onclick={handleCopy}>
    {#if copied}
      <svg
        viewBox="0 0 24 24"
        width="16"
        height="16"
        fill="none"
        stroke="var(--success)"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg
      >
    {:else}
      <svg
        viewBox="0 0 24 24"
        width="16"
        height="16"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        ><rect x="9" y="9" width="13" height="13" rx="2" /><path
          d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"
        /></svg
      >
    {/if}
  </button>
  <a
    class="url-action-btn"
    href={url}
    target="_blank"
    rel="noopener"
    title={t('common.openUrl')}
    onclick={handleOpen}
  >
    <svg
      viewBox="0 0 24 24"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      ><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" /><polyline
        points="15 3 21 3 21 9"
      /><line x1="10" y1="14" x2="21" y2="3" /></svg
    >
  </a>
</span>
