<script>
  import { t } from '../i18n/index.svelte.js';
  import CustomTestsTab from './CustomTestsTab.svelte';
  import ExtractTab from './ExtractTab.svelte';

  let {
    sessionId,
    sessionConfig = null,
    initialSubView = 'tests',
    onpushurl,
    onerror,
  } = $props();

  let subView = $state(initialSubView);

  const SUB_VIEW_IDS = ['tests', 'extractions'];
  const SUB_VIEW_KEYS = {
    tests: 'tabs.tests',
    extractions: 'tabs.extract',
  };

  function switchSubView(id) {
    subView = id;
    onpushurl?.(`/sessions/${sessionId}/tools/${id}`);
  }
</script>

<div class="pr-container">
  <div class="pr-subview-bar">
    {#each SUB_VIEW_IDS as id}
      <button
        class="pr-subview-btn"
        class:pr-subview-active={subView === id}
        onclick={() => switchSubView(id)}
      >
        {t(SUB_VIEW_KEYS[id])}
      </button>
    {/each}
  </div>

  {#if subView === 'tests'}
    <CustomTestsTab {sessionId} onerror={(msg) => onerror?.(msg)} />
  {:else if subView === 'extractions'}
    <ExtractTab
      {sessionId}
      {sessionConfig}
      onerror={(msg) => onerror?.(msg)}
    />
  {/if}
</div>
