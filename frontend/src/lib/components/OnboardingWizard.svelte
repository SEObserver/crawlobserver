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
    { code: 'tr', name: 'Türkçe' },
    { code: 'id', name: 'Bahasa Indonesia' },
    { code: 'ko', name: '한국어' },
    { code: 'pl', name: 'Polski' },
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
  let serverOS = $state('');
  let pollTimer = null;

  let winMethod = $state('docker');
  let copiedCmd = $state('');

  let showWindowsStep = $derived(serverOS === 'windows');
  let totalSteps = $derived(showWindowsStep ? 4 : 3);
  let crawlStep = $derived(showWindowsStep ? 3 : 2);
  let telemetryStep = $derived(showWindowsStep ? 4 : 3);

  // Auto-advance past Windows step when ClickHouse is detected
  $effect(() => {
    if (showWindowsStep && step === 2 && clickhouseReady) {
      setTimeout(() => {
        step = crawlStep;
      }, 1500);
    }
  });

  function pollStatus() {
    pollTimer = setInterval(async () => {
      try {
        const status = await getSetupStatus();
        if (status.os) serverOS = status.os;
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
    let next = step + 1;
    // Skip Windows step if CH is already ready
    if (showWindowsStep && next === 2 && clickhouseReady) {
      next = crawlStep;
    }
    if (next <= totalSteps) step = next;
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

  function copyCommand(text) {
    navigator.clipboard.writeText(text);
    // Determine which command was copied for visual feedback
    if (text.startsWith('docker')) copiedCmd = 'docker';
    else if (text === 'wsl --install') copiedCmd = 'wsl1';
    else copiedCmd = 'wsl3';
    setTimeout(() => (copiedCmd = ''), 2000);
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
    {:else if showWindowsStep && step === 2}
      <h1>{t('onboarding.windowsTitle')}</h1>
      <p class="subtitle">{t('onboarding.windowsSubtitle')}</p>

      <!-- Prerequisite checklist -->
      <div class="prereq-checklist">
        <div class="prereq-item" class:prereq-ok={clickhouseReady}>
          <span class="prereq-icon">
            {#if clickhouseReady}
              <svg viewBox="0 0 20 20" fill="currentColor"
                ><path
                  fill-rule="evenodd"
                  d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                  clip-rule="evenodd"
                /></svg
              >
            {:else}
              <span class="spinner-sm"></span>
            {/if}
          </span>
          <span class="prereq-label">ClickHouse</span>
          <span class="prereq-status" class:prereq-status-ok={clickhouseReady}>
            {clickhouseReady ? t('onboarding.winCheckOk') : t('onboarding.winCheckSearching')}
          </span>
        </div>
      </div>

      {#if !clickhouseReady}
        <p class="win-install-prompt">{t('onboarding.winInstallPrompt')}</p>

        <!-- Method selector tabs -->
        <div class="win-method-tabs">
          <button
            class="win-method-tab"
            class:win-method-active={winMethod === 'docker'}
            onclick={() => (winMethod = 'docker')}
          >
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.5"
              class="win-tab-icon"
            >
              <path d="M2 20h20V4H2v16zm2-2V6h16v12H4z" /><path
                d="M7 14c1-2 3-3 5-3s4 1 5 3"
              /><circle cx="9" cy="9" r="1" /><circle cx="15" cy="9" r="1" />
            </svg>
            Docker Desktop
          </button>
          <button
            class="win-method-tab"
            class:win-method-active={winMethod === 'wsl'}
            onclick={() => (winMethod = 'wsl')}
          >
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.5"
              class="win-tab-icon"
            >
              <rect x="2" y="3" width="20" height="16" rx="2" /><path d="M6 9h12M6 13h8" /><path
                d="M8 19l-2 2m10-2l2 2"
              />
            </svg>
            WSL
          </button>
        </div>

        {#if winMethod === 'docker'}
          <div class="win-steps">
            <div class="win-step">
              <span class="win-step-num">1</span>
              <div class="win-step-content">
                <p class="win-step-text">{t('onboarding.winDockerStep1')}</p>
                <a
                  class="win-step-link"
                  href="https://www.docker.com/products/docker-desktop/"
                  target="_blank"
                  rel="noopener"
                >
                  {t('onboarding.windowsDockerLink')} &#x2197;
                </a>
              </div>
            </div>
            <div class="win-step">
              <span class="win-step-num">2</span>
              <div class="win-step-content">
                <p class="win-step-text">{t('onboarding.winDockerStep2')}</p>
              </div>
            </div>
            <div class="win-step">
              <span class="win-step-num">3</span>
              <div class="win-step-content">
                <p class="win-step-text">{t('onboarding.winDockerStep3')}</p>
                <div class="win-cmd-block">
                  <code
                    >docker run -d --name clickhouse -p 9000:9000 clickhouse/clickhouse-server</code
                  >
                  <button
                    class="win-copy-btn"
                    onclick={() =>
                      copyCommand(
                        'docker run -d --name clickhouse -p 9000:9000 clickhouse/clickhouse-server',
                      )}
                    title={t('common.copy')}
                  >
                    {#if copiedCmd === 'docker'}
                      <svg viewBox="0 0 20 20" fill="currentColor"
                        ><path
                          fill-rule="evenodd"
                          d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                          clip-rule="evenodd"
                        /></svg
                      >
                    {:else}
                      <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"
                        ><rect x="6" y="6" width="10" height="10" rx="1.5" /><path
                          d="M4 14V4.5A.5.5 0 014.5 4H14"
                        /></svg
                      >
                    {/if}
                  </button>
                </div>
              </div>
            </div>
          </div>
        {:else}
          <div class="win-steps">
            <div class="win-step">
              <span class="win-step-num">1</span>
              <div class="win-step-content">
                <p class="win-step-text">{t('onboarding.winWslStep1')}</p>
                <div class="win-cmd-block">
                  <code>wsl --install</code>
                  <button
                    class="win-copy-btn"
                    onclick={() => copyCommand('wsl --install')}
                    title={t('common.copy')}
                  >
                    {#if copiedCmd === 'wsl1'}
                      <svg viewBox="0 0 20 20" fill="currentColor"
                        ><path
                          fill-rule="evenodd"
                          d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                          clip-rule="evenodd"
                        /></svg
                      >
                    {:else}
                      <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"
                        ><rect x="6" y="6" width="10" height="10" rx="1.5" /><path
                          d="M4 14V4.5A.5.5 0 014.5 4H14"
                        /></svg
                      >
                    {/if}
                  </button>
                </div>
              </div>
            </div>
            <div class="win-step">
              <span class="win-step-num">2</span>
              <div class="win-step-content">
                <p class="win-step-text">{t('onboarding.winWslStep2')}</p>
              </div>
            </div>
            <div class="win-step">
              <span class="win-step-num">3</span>
              <div class="win-step-content">
                <p class="win-step-text">{t('onboarding.winWslStep3')}</p>
                <div class="win-cmd-block">
                  <code>curl https://clickhouse.com/ | sh && ./clickhouse server</code>
                  <button
                    class="win-copy-btn"
                    onclick={() =>
                      copyCommand('curl https://clickhouse.com/ | sh && ./clickhouse server')}
                    title={t('common.copy')}
                  >
                    {#if copiedCmd === 'wsl3'}
                      <svg viewBox="0 0 20 20" fill="currentColor"
                        ><path
                          fill-rule="evenodd"
                          d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                          clip-rule="evenodd"
                        /></svg
                      >
                    {:else}
                      <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"
                        ><rect x="6" y="6" width="10" height="10" rx="1.5" /><path
                          d="M4 14V4.5A.5.5 0 014.5 4H14"
                        /></svg
                      >
                    {/if}
                  </button>
                </div>
              </div>
            </div>
          </div>
        {/if}
      {:else}
        <div class="win-success-box">
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            class="win-success-icon"
          >
            <path d="M22 11.08V12a10 10 0 11-5.93-9.14" /><polyline
              points="22 4 12 14.01 9 11.01"
            />
          </svg>
          <div>
            <p class="win-success-title">{t('onboarding.windowsDetected')}</p>
            <p class="win-success-sub">{t('onboarding.winReadyToContinue')}</p>
          </div>
        </div>
      {/if}
    {:else if step === crawlStep}
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
    {:else if step === telemetryStep}
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
      {#if !showWindowsStep}
        {#if clickhouseReady}
          <span class="progress-label progress-done">{t('onboarding.downloadComplete')}</span>
        {:else if downloadPercent > 0}
          <span class="progress-label"
            >{t('onboarding.downloadProgress', { percent: downloadPercent })}</span
          >
        {/if}
        <div class="progress-bar">
          <div
            class="progress-fill"
            style="width: {clickhouseReady ? 100 : downloadPercent}%"
          ></div>
        </div>
      {/if}
      <!-- Steps indicator -->
      <div class="steps">
        {#each Array.from({ length: totalSteps }, (_, i) => i + 1) as s}
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
    0% {
      transform: scale(1);
    }
    40% {
      transform: scale(1.03);
    }
    100% {
      transform: scale(1);
    }
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
    transition:
      border-color 0.2s ease,
      box-shadow 0.2s ease;
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
    transition:
      all 0.2s ease,
      transform 0.25s ease,
      box-shadow 0.2s ease;
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
    transition:
      transform 0.15s ease,
      box-shadow 0.15s ease;
  }
  .btn-lg:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(124, 58, 237, 0.25);
  }
  .btn-lg:active:not(:disabled) {
    transform: translateY(0);
  }

  /* Windows setup step — prerequisite checklist */
  .prereq-checklist {
    margin-bottom: 20px;
  }
  .prereq-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 16px;
    border-radius: 10px;
    background: var(--bg-card, #fff);
    border: 1px solid var(--border, #e0e0e0);
    font-size: 0.9rem;
  }
  .prereq-item.prereq-ok {
    border-color: var(--success, #22c55e);
    background: color-mix(in srgb, var(--success, #22c55e) 6%, var(--bg-card, #fff));
  }
  .prereq-icon {
    width: 20px;
    height: 20px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .prereq-icon svg {
    width: 18px;
    height: 18px;
    color: var(--success, #22c55e);
  }
  .prereq-label {
    font-weight: 600;
    color: var(--text-primary, #1a1a2e);
  }
  .prereq-status {
    margin-left: auto;
    font-size: 0.82rem;
    color: var(--text-muted, #888);
  }
  .prereq-status.prereq-status-ok {
    color: var(--success, #22c55e);
    font-weight: 600;
  }

  .win-install-prompt {
    font-size: 0.88rem;
    color: var(--text-secondary, #555);
    margin: 16px 0 20px;
  }

  /* Method selector tabs */
  .win-method-tabs {
    display: flex;
    gap: 8px;
    margin-bottom: 20px;
  }
  .win-method-tab {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 10px 16px;
    border: 2px solid var(--border, #e0e0e0);
    border-radius: 10px;
    background: var(--bg-card, #fff);
    cursor: pointer;
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--text-secondary, #555);
    transition: all 0.15s ease;
  }
  .win-method-tab:hover {
    border-color: var(--accent, #7c3aed);
    color: var(--text-primary, #1a1a2e);
  }
  .win-method-tab.win-method-active {
    border-color: var(--accent, #7c3aed);
    background: var(--accent, #7c3aed);
    color: #fff;
  }
  .win-tab-icon {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
  }

  /* Step-by-step instructions */
  .win-steps {
    text-align: left;
    margin-bottom: 24px;
  }
  .win-step {
    display: flex;
    gap: 12px;
    margin-bottom: 16px;
  }
  .win-step:last-child {
    margin-bottom: 0;
  }
  .win-step-num {
    width: 26px;
    height: 26px;
    border-radius: 50%;
    background: var(--accent, #7c3aed);
    color: #fff;
    font-size: 0.78rem;
    font-weight: 700;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    margin-top: 1px;
  }
  .win-step-content {
    flex: 1;
    min-width: 0;
  }
  .win-step-text {
    font-size: 0.88rem;
    color: var(--text-primary, #1a1a2e);
    line-height: 1.5;
    margin: 2px 0 0;
  }
  .win-step-link {
    display: inline-block;
    font-size: 0.82rem;
    color: var(--accent, #7c3aed);
    text-decoration: none;
    margin-top: 6px;
  }
  .win-step-link:hover {
    text-decoration: underline;
  }

  /* Command block with copy button */
  .win-cmd-block {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 8px;
    background: var(--bg, #f8f9fc);
    border: 1px solid var(--border, #e0e0e0);
    border-radius: 8px;
    padding: 8px 12px;
  }
  .win-cmd-block code {
    flex: 1;
    font-size: 0.78rem;
    font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
    color: var(--text-primary, #1a1a2e);
    word-break: break-all;
  }
  .win-copy-btn {
    flex-shrink: 0;
    width: 28px;
    height: 28px;
    padding: 4px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted, #888);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s;
  }
  .win-copy-btn:hover {
    background: var(--border, #e0e0e0);
    color: var(--text-primary, #1a1a2e);
  }
  .win-copy-btn svg {
    width: 16px;
    height: 16px;
  }

  /* Success state */
  .win-success-box {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 20px;
    border-radius: 12px;
    background: color-mix(in srgb, var(--success, #22c55e) 8%, var(--bg-card, #fff));
    border: 1px solid var(--success, #22c55e);
    margin-bottom: 24px;
    text-align: left;
  }
  .win-success-icon {
    width: 36px;
    height: 36px;
    color: var(--success, #22c55e);
    flex-shrink: 0;
  }
  .win-success-title {
    font-weight: 700;
    font-size: 0.95rem;
    color: var(--success, #22c55e);
    margin: 0 0 2px;
  }
  .win-success-sub {
    font-size: 0.82rem;
    color: var(--text-secondary, #555);
    margin: 0;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .spinner-sm {
    display: inline-block;
    width: 16px;
    height: 16px;
    border: 2px solid var(--border, #e0e0e0);
    border-top-color: var(--accent, #7c3aed);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
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
    transition:
      background 0.3s ease,
      transform 0.3s ease;
  }
  .step-dot.active {
    background: var(--accent, #7c3aed);
    transform: scale(1.3);
  }
  .step-dot.done {
    background: var(--success, #22c55e);
  }
</style>
