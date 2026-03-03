<script>
  import {
    getExtractorSets,
    getExtractorSet,
    createExtractorSet,
    updateExtractorSet,
    deleteExtractorSet,
    getExtractions,
    runExtractions,
  } from '../api.js';
  import ConfirmModal from './ConfirmModal.svelte';
  import UrlActions from './UrlActions.svelte';
  import { t } from '../i18n/index.svelte.js';
  import SearchSelect from './SearchSelect.svelte';

  let { sessionId, sessionConfig = '', onerror } = $props();

  let parsedConfig = $derived.by(() => {
    try {
      return typeof sessionConfig === 'string'
        ? JSON.parse(sessionConfig || '{}')
        : sessionConfig || {};
    } catch {
      return {};
    }
  });
  let hasStoredHtml = $derived(!!parsedConfig?.Crawler?.StoreHTML);

  const PRESETS = [
    {
      label: 'Prix',
      type: 'css_extract_text',
      selector: '[itemprop="price"], .price, .product-price',
      attribute: '',
      name: 'Prix',
    },
    { label: 'H1', type: 'css_extract_text', selector: 'h1', attribute: '', name: 'H1' },
    {
      label: 'Meta description',
      type: 'css_extract_attr',
      selector: 'meta[name="description"]',
      attribute: 'content',
      name: 'Meta Description',
    },
    {
      label: 'Canonical',
      type: 'css_extract_attr',
      selector: 'link[rel="canonical"]',
      attribute: 'href',
      name: 'Canonical',
    },
    {
      label: 'Hreflang',
      type: 'css_extract_all_attr',
      selector: 'link[rel="alternate"][hreflang]',
      attribute: 'hreflang',
      name: 'Hreflang',
    },
    {
      label: 'Schema.org @type',
      type: 'regex_extract_all',
      selector: '"@type"\\s*:\\s*"([^"]+)"',
      attribute: '',
      name: 'Schema Type',
    },
  ];

  function getExtractorTypes() {
    return [
      { value: 'css_extract_text', label: t('extract.cssExtractText') },
      { value: 'css_extract_attr', label: t('extract.cssExtractAttr') },
      { value: 'css_extract_all_text', label: t('extract.cssExtractAllText') },
      { value: 'css_extract_all_attr', label: t('extract.cssExtractAllAttr') },
      { value: 'regex_extract', label: t('extract.regexExtract') },
      { value: 'regex_extract_all', label: t('extract.regexExtractAll') },
      { value: 'xpath_extract', label: t('extract.xpathExtract') },
      { value: 'xpath_extract_all', label: t('extract.xpathExtractAll') },
    ];
  }

  function needsAttribute(type) {
    return ['css_extract_attr', 'css_extract_all_attr'].includes(type);
  }

  function getSelectorLabel(type) {
    if (type.startsWith('css_')) return t('extract.cssSelector');
    if (type.startsWith('regex_')) return t('extract.regexPattern');
    if (type.startsWith('xpath_')) return t('extract.xpathExpr');
    return t('extract.selector');
  }

  let confirmState = $state(null);

  function showConfirm(message, onConfirm, opts = {}) {
    confirmState = { message, onConfirm, ...opts };
  }

  // State
  let view = $state('list');
  let sets = $state([]);
  let loading = $state(false);

  // Editor state
  let editId = $state(null);
  let editName = $state('');
  let editExtractors = $state([]);

  // Results state
  let extractionResult = $state(null);
  let runningSetId = $state(null);
  let resultsPage = $state(0);
  const RESULTS_PAGE_SIZE = 100;

  async function loadSets() {
    loading = true;
    try {
      sets = await getExtractorSets();
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function newSet() {
    editId = null;
    editName = '';
    editExtractors = [
      { type: 'css_extract_text', name: '', selector: '', attribute: '', url_pattern: '' },
    ];
    view = 'editor';
  }

  function applyPreset(preset) {
    editExtractors = [
      ...editExtractors,
      {
        type: preset.type,
        name: preset.name,
        selector: preset.selector,
        attribute: preset.attribute,
        url_pattern: '',
      },
    ];
  }

  async function editSetById(id) {
    loading = true;
    try {
      const es = await getExtractorSet(id);
      editId = es.id;
      editName = es.name;
      editExtractors = es.extractors.map((e) => ({
        type: e.type,
        name: e.name,
        selector: e.selector,
        attribute: e.attribute || '',
        url_pattern: e.url_pattern || '',
      }));
      if (editExtractors.length === 0)
        editExtractors = [
          { type: 'css_extract_text', name: '', selector: '', attribute: '', url_pattern: '' },
        ];
      view = 'editor';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function saveSet() {
    const extractors = editExtractors.filter((e) => e.name && e.selector);
    if (!editName.trim()) {
      onerror?.(t('extract.nameRequired'));
      return;
    }
    if (extractors.length === 0) {
      onerror?.(t('extract.extractorRequired'));
      return;
    }

    loading = true;
    try {
      if (editId) {
        await updateExtractorSet(editId, editName, extractors);
      } else {
        await createExtractorSet(editName, extractors);
      }
      await loadSets();
      view = 'list';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function removeSet(id) {
    showConfirm(
      t('extract.deleteConfirm'),
      async () => {
        try {
          await deleteExtractorSet(id);
          await loadSets();
        } catch (e) {
          onerror?.(e.message);
        }
      },
      { danger: true, confirmLabel: t('common.delete') },
    );
  }

  async function runSet(setId) {
    runningSetId = setId;
    loading = true;
    try {
      extractionResult = await runExtractions(sessionId, setId);
      resultsPage = 0;
      view = 'results';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
      runningSetId = null;
    }
  }

  async function loadResults() {
    loading = true;
    try {
      extractionResult = await getExtractions(
        sessionId,
        RESULTS_PAGE_SIZE,
        resultsPage * RESULTS_PAGE_SIZE,
      );
      if (view !== 'results') view = 'results';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function addExtractor() {
    editExtractors = [
      ...editExtractors,
      { type: 'css_extract_text', name: '', selector: '', attribute: '', url_pattern: '' },
    ];
  }

  function removeExtractor(idx) {
    editExtractors = editExtractors.filter((_, i) => i !== idx);
  }

  function exportCsv() {
    if (!extractionResult || !extractionResult.pages.length) return;
    const names = extractionResult.extractors.map((e) => e.name);
    const header = ['URL', ...names].join(',');
    const rows = extractionResult.pages.map((p) => {
      const vals = names.map((n) => {
        const v = (p.values[n] || '').replace(/"/g, '""');
        return `"${v}"`;
      });
      return [`"${p.url}"`, ...vals].join(',');
    });
    const csv = [header, ...rows].join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `extractions-${sessionId}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  let pagedResults = $derived(extractionResult ? extractionResult.pages : []);
  let totalPages = $derived(extractionResult ? extractionResult.total_pages : 0);
  let hasMoreResults = $derived(
    extractionResult ? (resultsPage + 1) * RESULTS_PAGE_SIZE < totalPages : false,
  );
  let hasResults = $derived(totalPages > 0);

  // Load sets and check for existing results on mount
  async function init() {
    loading = true;
    try {
      const [setsData, resultsData] = await Promise.all([
        getExtractorSets(),
        getExtractions(sessionId, RESULTS_PAGE_SIZE, 0),
      ]);
      sets = setsData;
      extractionResult = resultsData;
      // Auto-show results if they exist
      if (resultsData && resultsData.total_pages > 0) {
        view = 'results';
      }
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  init();
</script>

{#if view === 'list'}
  <div class="card mt-md">
    <div class="ct-card-header">
      <h3 class="ct-title">{t('extract.sets')}</h3>
      <button class="btn btn-primary btn-sm" onclick={newSet}>{t('extract.newSet')}</button>
    </div>

    {#if loading}
      <div class="ct-empty">{t('common.loading')}</div>
    {:else if sets.length === 0}
      <div class="ct-empty">
        {t('extract.noSets')}
      </div>
    {:else}
      <table class="data-table">
        <thead>
          <tr>
            <th>{t('common.name')}</th>
            <th>{t('extract.extractors')}</th>
            <th>{t('tests.updated')}</th>
            <th class="ct-col-actions">{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {#each sets as es}
            <tr>
              <td><strong>{es.name}</strong></td>
              <td
                >{t('extract.extractorsCount', {
                  count: es.extractor_count ?? es.extractors?.length ?? 0,
                })}</td
              >
              <td class="text-muted text-sm">{new Date(es.updated_at).toLocaleDateString()}</td>
              <td class="ct-actions-cell">
                {#if hasResults}
                  <button class="btn btn-sm" onclick={loadResults}
                    >{t('extract.viewResults')}</button
                  >
                {/if}
                <button class="btn btn-sm" onclick={() => editSetById(es.id)}
                  >{t('common.edit')}</button
                >
                <button
                  class="btn btn-primary btn-sm"
                  onclick={() => runSet(es.id)}
                  disabled={!hasStoredHtml || runningSetId === es.id}
                  title={hasStoredHtml ? '' : t('extract.runRequiresHtml')}
                >
                  {runningSetId === es.id ? t('common.running') + '...' : t('tests.run')}
                </button>
                <button
                  class="btn-ghost ct-delete-btn"
                  onclick={() => removeSet(es.id)}
                  title={t('common.delete')}
                >
                  <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    class="ct-icon-sm"
                    ><polyline points="3 6 5 6 21 6" /><path
                      d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
                    /></svg
                  >
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
{:else if view === 'editor'}
  <div class="card mt-md">
    <div class="ct-editor-header">
      <h3 class="ct-title-mb">
        {editId ? t('extract.editSet') : t('extract.newSetTitle')}
      </h3>
      <div class="flex-center-gap">
        <label class="text-sm font-medium">{t('extract.setName')}</label>
        <input
          type="text"
          bind:value={editName}
          placeholder={t('extract.setNamePlaceholder')}
          class="ct-name-input"
        />
      </div>
    </div>

    <div class="ct-editor-body">
      <div class="ext-presets">
        <span class="text-sm text-muted">{t('extract.presets')}:</span>
        {#each PRESETS as preset}
          <button class="btn btn-sm" onclick={() => applyPreset(preset)}>{preset.label}</button>
        {/each}
      </div>

      <table class="data-table mb-sm">
        <thead>
          <tr>
            <th class="ct-col-type">{t('common.type')}</th>
            <th>{t('common.name')}</th>
            <th>{t('extract.selector')}</th>
            <th>{t('extract.attribute')}</th>
            <th>{t('extract.urlPattern')}</th>
            <th class="ct-col-remove"></th>
          </tr>
        </thead>
        <tbody>
          {#each editExtractors as ext, i}
            <tr>
              <td class="ct-td-type">
                <SearchSelect
                  small
                  bind:value={ext.type}
                  options={getExtractorTypes().map((et) => ({ value: et.value, label: et.label }))}
                />
              </td>
              <td>
                <input
                  type="text"
                  bind:value={ext.name}
                  placeholder={t('extract.extractorName')}
                  class="ct-input"
                />
              </td>
              <td>
                <input
                  type="text"
                  bind:value={ext.selector}
                  placeholder={getSelectorLabel(ext.type)}
                  class="ct-input"
                />
              </td>
              <td>
                {#if needsAttribute(ext.type)}
                  <input
                    type="text"
                    bind:value={ext.attribute}
                    placeholder={t('extract.attrName')}
                    class="ct-input"
                  />
                {:else}
                  <span class="text-muted text-xs">-</span>
                {/if}
              </td>
              <td>
                <input
                  type="text"
                  bind:value={ext.url_pattern}
                  placeholder={t('extract.urlPatternPlaceholder')}
                  class="ct-input"
                />
              </td>
              <td>
                {#if editExtractors.length > 1}
                  <button
                    class="btn-ghost ct-remove-btn"
                    onclick={() => removeExtractor(i)}
                    title={t('common.delete')}
                  >
                    <svg
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      class="ct-icon-sm"
                      ><polyline points="3 6 5 6 21 6" /><path
                        d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
                      /></svg
                    >
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>

      <div class="flex-center-gap">
        <button class="btn btn-sm" onclick={addExtractor}>{t('extract.addExtractor')}</button>
        <div class="ct-spacer"></div>
        <button
          class="btn btn-sm"
          onclick={() => {
            view = 'list';
          }}>{t('common.cancel')}</button
        >
        <button class="btn btn-primary btn-sm" onclick={saveSet} disabled={loading}>
          {loading ? t('common.saving') : t('extract.saveSet')}
        </button>
      </div>
    </div>
  </div>
{:else if view === 'results' && extractionResult}
  <div class="card mt-md">
    <div class="ct-card-header">
      <div>
        <h3 class="ct-title">{t('extract.results')}</h3>
        <span class="text-sm text-muted">{t('extract.pagesExtracted', { count: totalPages })}</span>
      </div>
      <div class="flex-center-gap">
        <button class="btn btn-sm" onclick={exportCsv} title={t('common.exportCsv')}>
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            ><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline
              points="7 10 12 15 17 10"
            /><line x1="12" y1="15" x2="12" y2="3" /></svg
          >
          {t('common.exportCsv')}
        </button>
        <button
          class="btn btn-sm"
          onclick={() => {
            view = 'list';
          }}>{t('extract.backToSets')}</button
        >
      </div>
    </div>

    {#if pagedResults.length === 0}
      <div class="ct-empty">{t('extract.noResults')}</div>
    {:else}
      <div class="overflow-auto">
        <table class="data-table">
          <thead>
            <tr>
              <th>{t('common.url')}</th>
              {#each extractionResult.extractors as ext}
                <th class="ct-col-rule">{ext.name}</th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each pagedResults as page}
              <tr>
                <td class="cell-url"
                  ><span class="cell-url-inner"
                    ><a href={page.url} target="_blank" rel="noopener">{page.url}</a><UrlActions
                      url={page.url}
                    /></span
                  ></td
                >
                {#each extractionResult.extractors as ext}
                  {@const val = page.values?.[ext.name] ?? ''}
                  <td>
                    <span class="text-xs" title={val}
                      >{val.length > 80 ? val.slice(0, 80) + '...' : val || '-'}</span
                    >
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if totalPages > RESULTS_PAGE_SIZE}
        <div class="pagination">
          <button
            class="btn btn-sm"
            disabled={resultsPage === 0}
            onclick={() => {
              resultsPage--;
              loadResults();
            }}>{t('common.previous')}</button
          >
          <span class="text-sm text-muted">
            {resultsPage * RESULTS_PAGE_SIZE + 1} - {Math.min(
              (resultsPage + 1) * RESULTS_PAGE_SIZE,
              totalPages,
            )}
            {t('common.of')}
            {totalPages}
          </span>
          <button
            class="btn btn-sm"
            disabled={!hasMoreResults}
            onclick={() => {
              resultsPage++;
              loadResults();
            }}>{t('common.next')}</button
          >
        </div>
      {/if}
    {/if}
  </div>
{/if}

{#if confirmState}<ConfirmModal
    message={confirmState.message}
    danger={confirmState.danger}
    confirmLabel={confirmState.confirmLabel}
    onconfirm={() => {
      confirmState.onConfirm();
      confirmState = null;
    }}
    oncancel={() => (confirmState = null)}
  />{/if}

<style>
  .ct-card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
  }
  .ct-title {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
  .ct-title-mb {
    margin: 0 0 12px 0;
    font-size: 15px;
    font-weight: 600;
  }
  .ct-empty {
    padding: 40px;
    text-align: center;
    color: var(--text-muted);
  }
  .ct-col-actions {
    width: 280px;
  }
  .ct-actions-cell {
    white-space: nowrap;
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .ct-col-type {
    width: 220px;
  }
  .ct-td-type {
    overflow: visible;
    position: relative;
  }
  .ct-col-remove {
    width: 40px;
  }
  .ct-col-rule {
    min-width: 100px;
  }
  .ct-editor-header {
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
  }
  .ct-editor-body {
    padding: 16px 20px;
  }
  .ct-name-input {
    flex: 1;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    font-size: 13px;
  }
  .ct-input {
    width: 100%;
    padding: 4px 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--text);
    font-size: 12px;
  }
  .ct-remove-btn {
    padding: 2px 6px;
    color: var(--text-muted);
  }
  .ct-remove-btn:hover {
    color: var(--text);
  }
  .ct-delete-btn {
    padding: 4px;
    color: var(--text-muted);
  }
  .ct-delete-btn:hover {
    color: var(--text);
  }
  .ct-icon-sm {
    width: 16px;
    height: 16px;
  }
  .ct-spacer {
    flex: 1;
  }
  .ext-presets {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
    margin-bottom: 12px;
  }
</style>
