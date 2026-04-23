<script>
  import { onMount, onDestroy } from 'svelte';
  import { getAnnouncements, updateAnnouncementsSettings } from '../api.js';

  const POLL_MS = 10 * 60 * 1000;
  const DISMISSED_KEY = 'dismissed_announcements';
  const INSTALL_TS_KEY = 'announcements_install_ts';
  // Keys whose presence indicates the user has used the app before the
  // announcements feature existed — used to distinguish upgrades from fresh installs.
  const EXISTING_USER_HINT_KEYS = ['darkMode', 'locale'];
  const EPOCH_ISO = '1970-01-01T00:00:00.000Z';

  let message = $state(null);
  let dismissedIds = $state(loadDismissed());
  let installTs = $state(loadOrStampInstallTs());
  let pollTimer = null;

  function loadDismissed() {
    try {
      const raw = localStorage.getItem(DISMISSED_KEY);
      if (!raw) return new Set();
      const parsed = JSON.parse(raw);
      return new Set(Array.isArray(parsed) ? parsed : []);
    } catch {
      return new Set();
    }
  }

  function saveDismissed() {
    try {
      localStorage.setItem(DISMISSED_KEY, JSON.stringify([...dismissedIds]));
    } catch {
      // localStorage unavailable (private mode, quota) — fail silently
    }
  }

  // On first run, decide the "install cutoff" timestamp:
  //   - Existing users (any hint key present) → epoch, so they see the current message once.
  //   - Fresh installs → now, so stale pre-install messages stay hidden.
  // Corrupted values are rewritten to a sane default.
  function loadOrStampInstallTs() {
    try {
      const existing = localStorage.getItem(INSTALL_TS_KEY);
      if (existing && !Number.isNaN(Date.parse(existing))) return existing;

      const hasHint = EXISTING_USER_HINT_KEYS.some((k) => localStorage.getItem(k) !== null) ||
        localStorage.getItem(DISMISSED_KEY) !== null;
      const ts = hasHint ? EPOCH_ISO : new Date().toISOString();
      localStorage.setItem(INSTALL_TS_KEY, ts);
      return ts;
    } catch {
      return new Date().toISOString();
    }
  }

  async function fetchAnnouncement() {
    try {
      const data = await getAnnouncements();
      if (!data || !data.enabled) {
        message = null;
        return;
      }
      message = data.message || null;
    } catch {
      // Backend unreachable — silent fail
    }
  }

  function dismiss() {
    if (!message) return;
    dismissedIds = new Set([...dismissedIds, message.id]);
    saveDismissed();
    message = null;
  }

  async function optOut() {
    try {
      await updateAnnouncementsSettings(false);
      message = null;
    } catch (e) {
      console.warn('Failed to opt out of announcements:', e);
      // Leave the banner visible so the user knows the action didn't persist
    }
  }

  // Escape HTML then render a minimal, safe subset of markdown:
  //   **bold**, *italic*, [text](https://url)
  // Only https:// links are allowed.
  function renderBody(raw) {
    if (!raw) return '';
    let html = raw
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;');
    html = html.replace(
      /\[([^\]]+)\]\((https:\/\/[^\s)]+)\)/g,
      (_, text, url) => `<a href="${url}" target="_blank" rel="noopener noreferrer">${text}</a>`,
    );
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>');
    return html;
  }

  function isSafeCTA(url) {
    return typeof url === 'string' && url.startsWith('https://');
  }

  // Visibility rules (all must pass):
  //   - message exists and has an id
  //   - not already dismissed
  //   - published_at is a valid ISO date
  //   - published_at <= now (timed release)
  //   - published_at > installTs (hide pre-install messages for fresh installs)
  //   - show_until, if present and valid, is in the future
  function computeVisible(msg, dismissed, install) {
    if (!msg || !msg.id) return false;
    if (dismissed.has(msg.id)) return false;

    const now = Date.now();
    const publishedMs = Date.parse(msg.published_at);
    const installMs = Date.parse(install);
    if (Number.isNaN(publishedMs) || Number.isNaN(installMs)) return false;
    if (publishedMs > now) return false;
    if (publishedMs <= installMs) return false;

    if (msg.show_until) {
      const untilMs = Date.parse(msg.show_until);
      if (!Number.isNaN(untilMs) && untilMs < now) return false;
    }
    return true;
  }

  onMount(() => {
    fetchAnnouncement();
    pollTimer = setInterval(fetchAnnouncement, POLL_MS);
  });

  onDestroy(() => {
    if (pollTimer) clearInterval(pollTimer);
  });

  let visible = $derived(computeVisible(message, dismissedIds, installTs));
</script>

{#if visible}
  <div class="alert alert-info announcement">
    <div class="announcement-content">
      <div class="announcement-title">{message.title}</div>
      {#if message.body}
        <!-- eslint-disable-next-line svelte/no-at-html-tags -->
        <div class="announcement-body">{@html renderBody(message.body)}</div>
      {/if}
    </div>
    <div class="announcement-actions">
      {#if isSafeCTA(message.cta_url)}
        <a
          class="btn btn-sm btn-primary"
          href={message.cta_url}
          target="_blank"
          rel="noopener noreferrer"
        >
          {message.cta_label || 'En savoir plus'}
        </a>
      {/if}
      <button class="btn btn-sm btn-ghost" onclick={dismiss} title="Masquer ce message">
        ×
      </button>
      <button
        class="btn btn-sm btn-ghost announcement-optout"
        onclick={optOut}
        title="Ne plus afficher d'annonces"
      >
        Ne plus afficher
      </button>
    </div>
  </div>
{/if}

<style>
  .announcement {
    align-items: flex-start;
    gap: 16px;
  }
  .announcement-content {
    flex: 1;
    min-width: 0;
  }
  .announcement-title {
    font-weight: 600;
    margin-bottom: 2px;
  }
  .announcement-body {
    font-size: 13px;
    opacity: 0.9;
    line-height: 1.4;
  }
  .announcement-body :global(a) {
    color: inherit;
    text-decoration: underline;
  }
  .announcement-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }
  .announcement-optout {
    font-size: 12px;
    opacity: 0.7;
  }
  .announcement-optout:hover {
    opacity: 1;
  }
</style>
