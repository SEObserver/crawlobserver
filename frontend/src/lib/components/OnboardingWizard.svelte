<script>
  import { t, setLocale, getLocale } from '../i18n/index.svelte.js';
  import { getSetupStatus, completeSetup } from '../api.js';

  let { oncomplete, startStep = 1 } = $props();

  const LANGUAGES = [
    { code: 'en', name: 'English' },
    { code: 'fr', name: 'Français' },
    { code: 'es', name: 'Español' },
    { code: 'pt', name: 'Português' },
    { code: 'nl', name: 'Nederlands' },
    { code: 'it', name: 'Italiano' },
    { code: 'de', name: 'Deutsch' },
    { code: 'ru', name: 'Русский' },
    { code: 'zh', name: '中文' },
    { code: 'ja', name: '日本語' },
    { code: 'he', name: 'עברית' },
    { code: 'ar', name: 'العربية' },
  ];

  function detectBrowserLanguage() {
    const langs = navigator.languages || [navigator.language || 'en'];
    for (const lang of langs) {
      const code = lang.split('-')[0].toLowerCase();
      if (LANGUAGES.some((l) => l.code === code)) return code;
    }
    return 'en';
  }

  let step = $state(startStep);
  let selectedLang = $state(detectBrowserLanguage());
  setLocale(selectedLang);
  let selectedDelay = $state('1s');
  let workers = $state(10);
  let telemetryEnabled = $state(true);
  let submitting = $state(false);

  // Download progress polling
  let downloadPercent = $state(0);
  let clickhouseReady = $state(false);
  let pollTimer = null;

  function pollStatus() {
    pollTimer = setInterval(async () => {
      try {
        const status = await getSetupStatus();
        if (status.download_progress) {
          downloadPercent = status.download_progress.percent || 0;
        }
        clickhouseReady = status.clickhouse_ready;
        if (clickhouseReady && pollTimer) {
          clearInterval(pollTimer);
          pollTimer = null;
        }
      } catch {
        // Server might not be fully ready yet
      }
    }, 1000);
  }
  pollStatus();

  function selectLang(lang) {
    selectedLang = lang;
    setLocale(lang);
  }

  function selectPreset(delay) {
    selectedDelay = delay;
  }

  function nextStep() {
    if (step < 3) step++;
  }

  async function finish() {
    submitting = true;
    try {
      await completeSetup({
        language: selectedLang,
        crawler_delay: selectedDelay,
        crawler_workers: workers,
        telemetry_enabled: telemetryEnabled,
      });

      // Wait for ClickHouse to be ready if it isn't yet
      if (!clickhouseReady) {
        await new Promise((resolve) => {
          const check = setInterval(async () => {
            try {
              const status = await getSetupStatus();
              if (status.clickhouse_ready) {
                clearInterval(check);
                resolve();
              }
            } catch {
              // keep trying
            }
          }, 1000);
        });
      }

      oncomplete?.();
    } catch (e) {
      console.error('Setup failed:', e);
      submitting = false;
    }
  }

  import { onDestroy } from 'svelte';
  onDestroy(() => {
    if (pollTimer) clearInterval(pollTimer);
  });
</script>

<div class="onboarding">
  <div class="onboarding-card">
    {#if step === 1}
      <h1>{t('onboarding.step1Title')}</h1>
      <p class="subtitle">{t('onboarding.step1Subtitle')}</p>
      <div class="lang-select-wrapper">
        <select
          class="lang-select"
          value={selectedLang}
          onchange={(e) => selectLang(e.target.value)}
        >
          {#each LANGUAGES as lang}
            <option value={lang.code}>{lang.name}</option>
          {/each}
        </select>
      </div>
      <div class="actions">
        <button class="btn btn-primary btn-lg" onclick={nextStep}>{t('onboarding.next')}</button>
      </div>

    {:else if step === 2}
      <h1>{t('onboarding.step2Title')}</h1>
      <p class="subtitle">{t('onboarding.step2Subtitle')}</p>
      <div class="presets">
        <button
          class="preset"
          class:selected={selectedDelay === '1s'}
          onclick={() => selectPreset('1s')}
        >
          <span class="preset-name">{t('onboarding.presetRespectful')}</span>
          <span class="preset-desc">{t('onboarding.presetRespectfulDesc')}</span>
        </button>
        <button
          class="preset"
          class:selected={selectedDelay === '500ms'}
          onclick={() => selectPreset('500ms')}
        >
          <span class="preset-name">{t('onboarding.presetStandard')}</span>
          <span class="preset-desc">{t('onboarding.presetStandardDesc')}</span>
        </button>
        <button
          class="preset"
          class:selected={selectedDelay === '0s'}
          onclick={() => selectPreset('0s')}
        >
          <span class="preset-name">{t('onboarding.presetFast')}</span>
          <span class="preset-desc">{t('onboarding.presetFastDesc')}</span>
        </button>
      </div>
      <div class="workers-row">
        <label for="ob-workers">{t('onboarding.step2Workers')}</label>
        <input id="ob-workers" type="number" bind:value={workers} min="1" max="100" />
      </div>
      <div class="actions">
        <button class="btn btn-primary btn-lg" onclick={nextStep}>{t('onboarding.next')}</button>
      </div>

    {:else if step === 3}
      <h1>{t('onboarding.step3Title')}</h1>
      <p class="subtitle">{t('onboarding.step3Subtitle')}</p>
      <div class="telemetry-info">
        <div class="telemetry-col">
          <h3>{t('onboarding.telemetryCollect')}</h3>
          <p>{t('onboarding.telemetryCollectItems')}</p>
        </div>
        <div class="telemetry-col telemetry-never">
          <h3>{t('onboarding.telemetryNever')}</h3>
          <p>{t('onboarding.telemetryNeverItems')}</p>
        </div>
      </div>
      <label class="toggle-row">
        <input type="checkbox" bind:checked={telemetryEnabled} />
        <span>{t('onboarding.enableTelemetry')}</span>
      </label>
      <div class="actions">
        <button class="btn btn-primary btn-lg" onclick={finish} disabled={submitting}>
          {#if submitting}
            {t('onboarding.waitingForEngine')}
          {:else}
            {t('onboarding.finish')}
          {/if}
        </button>
      </div>
    {/if}

    <!-- Progress bar -->
    <div class="progress-section">
      {#if clickhouseReady}
        <span class="progress-label progress-done">{t('onboarding.downloadComplete')}</span>
      {:else if downloadPercent > 0}
        <span class="progress-label">{t('onboarding.downloadProgress', { percent: downloadPercent })}</span>
      {/if}
      <div class="progress-bar">
        <div class="progress-fill" style="width: {clickhouseReady ? 100 : downloadPercent}%"></div>
      </div>
      <!-- Steps indicator -->
      <div class="steps">
        {#each [1, 2, 3] as s}
          {#if s >= startStep}
            <div class="step-dot" class:active={step === s} class:done={step > s}></div>
          {/if}
        {/each}
      </div>
    </div>
  </div>
</div>

<style>
  .onboarding {
    position: fixed;
    inset: 0;
    z-index: 9999;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg, #f8f9fc);
  }

  .onboarding-card {
    max-width: 560px;
    width: 100%;
    padding: 48px 40px 32px;
    text-align: center;
  }

  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin-bottom: 8px;
    color: var(--text-primary, #1a1a2e);
  }

  .subtitle {
    color: var(--text-muted, #888);
    font-size: 0.95rem;
    margin-bottom: 32px;
  }

  @keyframes selectPop {
    0% { transform: scale(1); }
    40% { transform: scale(1.03); }
    100% { transform: scale(1); }
  }

  .lang-select-wrapper {
    position: relative;
    max-width: 320px;
    margin: 0 auto 32px;
  }

  .lang-select {
    width: 100%;
    padding: 14px 40px 14px 16px;
    font-size: 1.05rem;
    border: 2px solid var(--border, #e0e0e0);
    border-radius: 12px;
    background: var(--bg-card, #fff);
    color: var(--text-primary, #1a1a2e);
    appearance: none;
    cursor: pointer;
    transition: border-color 0.2s ease, box-shadow 0.2s ease;
  }

  .lang-select:focus {
    outline: none;
    border-color: var(--accent, #7c3aed);
    box-shadow: 0 0 0 3px rgba(124, 58, 237, 0.1);
  }

  .lang-select-wrapper::after {
    content: '';
    position: absolute;
    right: 16px;
    top: 50%;
    transform: translateY(-50%);
    width: 0;
    height: 0;
    border-left: 5px solid transparent;
    border-right: 5px solid transparent;
    border-top: 6px solid var(--text-muted, #888);
    pointer-events: none;
  }

  .presets {
    display: flex;
    gap: 12px;
    justify-content: center;
    margin-bottom: 24px;
  }

  .preset {
    flex: 1;
    padding: 20px 16px;
    border: 2px solid var(--border, #e0e0e0);
    border-radius: 12px;
    background: var(--bg-card, #fff);
    cursor: pointer;
    text-align: center;
    transition: all 0.2s ease, transform 0.25s ease, box-shadow 0.2s ease;
    color: var(--text-primary, #1a1a2e);
  }
  .preset:hover {
    border-color: var(--accent, #7c3aed);
    transform: translateY(-1px);
    box-shadow: 0 2px 8px rgba(124, 58, 237, 0.08);
  }
  .preset.selected {
    border-color: var(--accent, #7c3aed);
    background: var(--accent, #7c3aed);
    color: #fff;
    animation: selectPop 0.25s ease;
    box-shadow: 0 4px 12px rgba(124, 58, 237, 0.2);
  }

  .preset-name {
    display: block;
    font-weight: 600;
    font-size: 1rem;
    margin-bottom: 4px;
  }

  .preset-desc {
    display: block;
    font-size: 0.82rem;
    opacity: 0.8;
  }

  .workers-row {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-bottom: 32px;
  }
  .workers-row label {
    font-size: 0.9rem;
    color: var(--text-secondary, #555);
  }
  .workers-row input {
    width: 80px;
    padding: 6px 10px;
    border: 1px solid var(--border, #e0e0e0);
    border-radius: 6px;
    font-size: 0.95rem;
    text-align: center;
    background: var(--bg-card, #fff);
    color: var(--text-primary, #1a1a2e);
  }

  .telemetry-info {
    display: flex;
    gap: 24px;
    text-align: left;
    margin-bottom: 24px;
  }

  .telemetry-col {
    flex: 1;
    padding: 16px;
    border-radius: 10px;
    background: var(--bg-card, #fff);
    border: 1px solid var(--border, #e0e0e0);
  }
  .telemetry-col h3 {
    font-size: 0.85rem;
    font-weight: 600;
    margin-bottom: 8px;
    color: var(--text-primary, #1a1a2e);
  }
  .telemetry-col p {
    font-size: 0.82rem;
    color: var(--text-secondary, #555);
    line-height: 1.5;
  }
  .telemetry-never h3 {
    color: var(--success, #22c55e);
  }

  .toggle-row {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    margin-bottom: 32px;
    font-size: 0.95rem;
    cursor: pointer;
    color: var(--text-primary, #1a1a2e);
  }
  .toggle-row input[type='checkbox'] {
    width: 18px;
    height: 18px;
    accent-color: var(--accent, #7c3aed);
    transition: transform 0.15s ease;
  }
  .toggle-row input[type='checkbox']:checked {
    transform: scale(1.1);
  }

  .actions {
    margin-bottom: 32px;
  }

  .btn-lg {
    padding: 12px 48px;
    font-size: 1rem;
    border-radius: 10px;
    transition: transform 0.15s ease, box-shadow 0.15s ease;
  }
  .btn-lg:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(124, 58, 237, 0.25);
  }
  .btn-lg:active:not(:disabled) {
    transform: translateY(0);
  }

  .progress-section {
    border-top: 1px solid var(--border, #e0e0e0);
    padding-top: 20px;
  }

  .progress-label {
    display: block;
    font-size: 0.8rem;
    color: var(--text-muted, #888);
    margin-bottom: 8px;
  }
  .progress-done {
    color: var(--success, #22c55e);
  }

  .progress-bar {
    height: 4px;
    background: var(--border, #e0e0e0);
    border-radius: 2px;
    overflow: hidden;
    margin-bottom: 16px;
  }

  .progress-fill {
    height: 100%;
    background: var(--accent, #7c3aed);
    border-radius: 2px;
    transition: width 0.3s ease;
  }

  .steps {
    display: flex;
    justify-content: center;
    gap: 8px;
  }

  .step-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--border, #e0e0e0);
    transition: background 0.3s ease, transform 0.3s ease;
  }
  .step-dot.active {
    background: var(--accent, #7c3aed);
    transform: scale(1.3);
  }
  .step-dot.done {
    background: var(--success, #22c55e);
  }
</style>
