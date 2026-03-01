<script>
  import { a11yKeydown } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let {
    message,
    confirmLabel = null,
    cancelLabel = null,
    danger = false,
    onconfirm,
    oncancel,
  } = $props();
</script>

<div
  class="confirm-overlay"
  role="button"
  tabindex="0"
  onclick={oncancel}
  onkeydown={a11yKeydown(oncancel)}
>
  <div
    class="confirm-dialog"
    role="alertdialog"
    aria-modal="true"
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => {
      if (e.key === 'Escape') oncancel?.();
      e.stopPropagation();
    }}
  >
    <p class="confirm-message">{message}</p>
    <div class="confirm-actions">
      <button class="btn btn-sm" onclick={oncancel}
        >{cancelLabel ?? t('confirmModal.cancel')}</button
      >
      <button
        class="btn btn-sm"
        class:btn-primary={!danger}
        class:btn-danger={danger}
        onclick={onconfirm}>{confirmLabel ?? t('confirmModal.confirm')}</button
      >
    </div>
  </div>
</div>

<style>
  .confirm-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    z-index: 1100;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }
  .confirm-dialog {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
    padding: 24px;
    max-width: 420px;
    width: 100%;
  }
  .confirm-message {
    font-size: 14px;
    line-height: 1.5;
    color: var(--text);
    margin-bottom: 20px;
  }
  .confirm-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
