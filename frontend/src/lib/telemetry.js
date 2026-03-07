let posthog = null;
let initialized = false;

const POSTHOG_KEY = 'phc_gbbCqXPOTdBLcN9HDvp7GYUUF7QZ4VDooPCxsJBwYcY';
const POSTHOG_HOST = 'https://eu.i.posthog.com';

/**
 * Initialize PostHog telemetry with the given instance ID.
 * Uses dynamic import so posthog-js is only loaded when telemetry is enabled.
 * @param {string} instanceId
 * @param {boolean} [sessionRecording=false] WARNING: records full browser sessions — all page content, URLs, and clicks are sent to PostHog
 */
export async function initTelemetry(instanceId, sessionRecording = false) {
  if (initialized) return;
  try {
    const mod = await import('posthog-js');
    posthog = mod.default;
    posthog.init(POSTHOG_KEY, {
      api_host: POSTHOG_HOST,
      ip: false,
      autocapture: false,
      capture_pageview: false,
      persistence: 'localStorage',
      disable_session_recording: !sessionRecording,
    });
    posthog.identify(instanceId);
    initialized = true;
  } catch (e) {
    console.warn('Telemetry init failed:', e);
  }
}

/**
 * Track a named event with optional properties.
 * @param {string} name
 * @param {Object} [props]
 */
export function trackEvent(name, props) {
  if (posthog && initialized) {
    posthog.capture(name, props);
  }
}

/**
 * Track a page view.
 * @param {string} page
 */
export function trackPageView(page) {
  if (posthog && initialized) {
    posthog.capture('$pageview', {
      $current_url: window.location.origin + page,
    });
  }
}

/**
 * Disable telemetry (opt out).
 */
export function disableTelemetry() {
  if (posthog) {
    posthog.opt_out_capturing();
  }
}

/**
 * Re-enable telemetry (opt in).
 */
export function enableTelemetry() {
  if (posthog) {
    posthog.opt_in_capturing();
  }
}
