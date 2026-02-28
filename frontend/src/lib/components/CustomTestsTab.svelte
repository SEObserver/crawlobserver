<script>
  import { getRulesets, getRuleset, createRuleset, updateRuleset, deleteRuleset, runTests } from '../api.js';
  import ConfirmModal from './ConfirmModal.svelte';
  import { t } from '../i18n/index.svelte.js';

  let { sessionId, onerror } = $props();

  function getRuleTypes() {
    return [
      { value: 'string_contains', label: t('tests.stringContains'), target: 'HTML' },
      { value: 'string_not_contains', label: t('tests.stringNotContains'), target: 'HTML' },
      { value: 'regex_match', label: t('tests.regexMatch'), target: 'HTML' },
      { value: 'regex_not_match', label: t('tests.regexNotMatch'), target: 'HTML' },
      { value: 'header_exists', label: t('tests.headerExists'), target: 'Headers' },
      { value: 'header_not_exists', label: t('tests.headerNotExists'), target: 'Headers' },
      { value: 'header_contains', label: t('tests.headerContains'), target: 'Headers' },
      { value: 'header_regex', label: t('tests.headerRegex'), target: 'Headers' },
      { value: 'css_exists', label: t('tests.cssExists'), target: 'HTML/CSS' },
      { value: 'css_not_exists', label: t('tests.cssNotExists'), target: 'HTML/CSS' },
      { value: 'css_extract_text', label: t('tests.cssExtractText'), target: 'HTML/CSS' },
      { value: 'css_extract_attr', label: t('tests.cssExtractAttr'), target: 'HTML/CSS' },
      { value: 'css_extract_all_text', label: t('tests.cssExtractAllText'), target: 'HTML/CSS' },
      { value: 'css_extract_all_attr', label: t('tests.cssExtractAllAttr'), target: 'HTML/CSS' },
      { value: 'regex_extract', label: t('tests.regexExtract'), target: 'HTML' },
      { value: 'regex_extract_all', label: t('tests.regexExtractAll'), target: 'HTML' },
      { value: 'xpath_extract', label: t('tests.xpathExtract'), target: 'HTML/XPath' },
      { value: 'xpath_extract_all', label: t('tests.xpathExtractAll'), target: 'HTML/XPath' },
    ];
  }

  function needsExtra(type) {
    return ['header_contains', 'header_regex', 'css_extract_attr', 'css_extract_all_attr'].includes(type);
  }

  function getExtraLabel(type) {
    if (type === 'header_contains' || type === 'header_regex') return t('tests.headerValue');
    if (type === 'css_extract_attr' || type === 'css_extract_all_attr') return t('tests.attributeName');
    return t('tests.extra');
  }

  function getValueLabel(type) {
    if (type.startsWith('header_')) return t('tests.headerName');
    if (type.startsWith('css_')) return t('tests.cssSelector');
    if (type.startsWith('regex_')) return t('tests.regexPattern');
    if (type.startsWith('xpath_')) return t('tests.xpathExpression');
    return t('tests.searchString');
  }

  function isAllRule(type) {
    return type.includes('_all_') || type.endsWith('_all');
  }

  let confirmState = $state(null);

  function showConfirm(message, onConfirm, opts = {}) {
    confirmState = { message, onConfirm, ...opts };
  }

  // State
  let view = $state('list'); // 'list' | 'editor' | 'results'
  let rulesets = $state([]);
  let loading = $state(false);

  // Editor state
  let editId = $state(null);
  let editName = $state('');
  let editRules = $state([]);

  // Results state
  let testResult = $state(null);
  let runningRulesetId = $state(null);
  let resultsPage = $state(0);
  const RESULTS_PAGE_SIZE = 100;

  async function loadRulesets() {
    loading = true;
    try {
      rulesets = await getRulesets();
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function newRuleset() {
    editId = null;
    editName = '';
    editRules = [{ type: 'string_contains', name: '', value: '', extra: '' }];
    view = 'editor';
  }

  async function editRulesetById(id) {
    loading = true;
    try {
      const rs = await getRuleset(id);
      editId = rs.id;
      editName = rs.name;
      editRules = rs.rules.map(r => ({ type: r.type, name: r.name, value: r.value, extra: r.extra || '' }));
      if (editRules.length === 0) editRules = [{ type: 'string_contains', name: '', value: '', extra: '' }];
      view = 'editor';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  async function saveRuleset() {
    const rules = editRules.filter(r => r.name && r.value);
    if (!editName.trim()) { onerror?.(t('tests.nameRequired')); return; }
    if (rules.length === 0) { onerror?.(t('tests.ruleRequired')); return; }

    loading = true;
    try {
      if (editId) {
        await updateRuleset(editId, editName, rules);
      } else {
        await createRuleset(editName, rules);
      }
      await loadRulesets();
      view = 'list';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  function removeRuleset(id) {
    showConfirm(t('tests.deleteConfirm'), async () => {
      try {
        await deleteRuleset(id);
        await loadRulesets();
      } catch (e) {
        onerror?.(e.message);
      }
    }, { danger: true, confirmLabel: t('common.delete') });
  }

  async function runRuleset(rulesetId) {
    runningRulesetId = rulesetId;
    loading = true;
    try {
      testResult = await runTests(sessionId, rulesetId);
      resultsPage = 0;
      view = 'results';
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
      runningRulesetId = null;
    }
  }

  function addRule() {
    editRules = [...editRules, { type: 'string_contains', name: '', value: '', extra: '' }];
  }

  function removeRule(idx) {
    editRules = editRules.filter((_, i) => i !== idx);
  }

  // Derived for results pagination
  let pagedResults = $derived(
    testResult ? testResult.pages.slice(resultsPage * RESULTS_PAGE_SIZE, (resultsPage + 1) * RESULTS_PAGE_SIZE) : []
  );
  let hasMoreResults = $derived(
    testResult ? (resultsPage + 1) * RESULTS_PAGE_SIZE < testResult.pages.length : false
  );

  // Load on mount
  loadRulesets();
</script>

{#if view === 'list'}
  <div class="card mt-md">
    <div class="ct-card-header">
      <h3 class="ct-title">{t('tests.rulesets')}</h3>
      <button class="btn btn-primary btn-sm" onclick={newRuleset}>{t('tests.newRuleset')}</button>
    </div>

    {#if loading}
      <div class="ct-empty">{t('common.loading')}</div>
    {:else if rulesets.length === 0}
      <div class="ct-empty">
        {t('tests.noRulesets')}
      </div>
    {:else}
      <table class="data-table">
        <thead>
          <tr>
            <th>{t('common.name')}</th>
            <th>{t('tests.rules')}</th>
            <th>{t('tests.updated')}</th>
            <th class="ct-col-actions">{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rulesets as rs}
            <tr>
              <td><strong>{rs.name}</strong></td>
              <td>{t('tests.rulesCount', { count: rs.rules?.length ?? 0 })}</td>
              <td class="text-muted text-sm">{new Date(rs.updated_at).toLocaleDateString()}</td>
              <td>
                <button class="btn btn-sm" onclick={() => editRulesetById(rs.id)}>{t('common.edit')}</button>
                <button class="btn btn-primary btn-sm" onclick={() => runRuleset(rs.id)}
                  disabled={runningRulesetId === rs.id}>
                  {runningRulesetId === rs.id ? t('common.running') + '...' : t('tests.run')}
                </button>
                <button class="btn btn-sm btn-danger" onclick={() => removeRuleset(rs.id)}>{t('common.delete')}</button>
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
        {editId ? t('tests.editRuleset') : t('tests.newRulesetTitle')}
      </h3>
      <div class="flex-center-gap">
        <label class="text-sm font-medium">{t('tests.rulesetName')}</label>
        <input type="text" bind:value={editName} placeholder={t('tests.rulesetNamePlaceholder')}
          class="ct-name-input" />
      </div>
    </div>

    <div class="ct-editor-body">
      {#if editRules.some(r => isAllRule(r.type))}
        <div class="ct-warning">
          <strong>{t('tests.warning')}</strong> {t('tests.extractAllWarning')}
        </div>
      {/if}

      <table class="data-table mb-sm">
        <thead>
          <tr>
            <th class="ct-col-type">{t('common.type')}</th>
            <th>{t('common.name')}</th>
            <th>{t('common.value')}</th>
            <th>{t('tests.extra')}</th>
            <th class="ct-col-remove"></th>
          </tr>
        </thead>
        <tbody>
          {#each editRules as rule, i}
            <tr>
              <td>
                <select bind:value={rule.type}
                  class="ct-input">
                  {#each getRuleTypes() as rt}
                    <option value={rt.value}>{rt.label}</option>
                  {/each}
                </select>
              </td>
              <td>
                <input type="text" bind:value={rule.name} placeholder={t('tests.ruleLabel')}
                  class="ct-input" />
              </td>
              <td>
                <input type="text" bind:value={rule.value} placeholder={getValueLabel(rule.type)}
                  class="ct-input" />
              </td>
              <td>
                {#if needsExtra(rule.type)}
                  <input type="text" bind:value={rule.extra} placeholder={getExtraLabel(rule.type)}
                    class="ct-input" />
                {:else}
                  <span class="text-muted text-xs">-</span>
                {/if}
              </td>
              <td>
                {#if editRules.length > 1}
                  <button class="btn-ghost ct-remove-btn" onclick={() => removeRule(i)}>x</button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>

      <div class="flex-center-gap">
        <button class="btn btn-sm" onclick={addRule}>{t('tests.addRule')}</button>
        <div class="ct-spacer"></div>
        <button class="btn btn-sm" onclick={() => { view = 'list'; }}>{t('common.cancel')}</button>
        <button class="btn btn-primary btn-sm" onclick={saveRuleset} disabled={loading}>
          {loading ? t('common.saving') : t('tests.saveRuleset')}
        </button>
      </div>
    </div>
  </div>

{:else if view === 'results' && testResult}
  <div class="card mt-md">
    <div class="ct-card-header">
      <div>
        <h3 class="ct-title">
          {t('tests.results', { name: testResult.ruleset_name })}
        </h3>
        <span class="text-sm text-muted">{t('tests.pagesTested', { count: testResult.total_pages })}</span>
      </div>
      <button class="btn btn-sm" onclick={() => { view = 'list'; }}>{t('tests.backToRulesets')}</button>
    </div>

    <!-- Summary -->
    <div class="ct-results-summary">
      {#each testResult.rules as rule}
        {@const count = testResult.summary[rule.id] ?? 0}
        {@const total = testResult.total_pages}
        {@const pct = total > 0 ? Math.round(count / total * 100) : 0}
        {@const isExtract = rule.type.includes('extract')}
        <div class="ct-result-card">
          <div class="text-xs text-muted mb-xs">{rule.name}</div>
          {#if isExtract}
            <div class="ct-result-value">{count}/{total}</div>
            <div class="ct-result-sublabel">{t('tests.haveValue')}</div>
          {:else}
            <div class="ct-result-value" style="color: {pct >= 80 ? 'var(--success)' : pct >= 50 ? 'var(--warning)' : 'var(--error)'};">{pct}%</div>
            <div class="ct-result-sublabel">{count}/{total} {t('tests.pass')}</div>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Results table -->
    <div class="overflow-auto">
      <table class="data-table">
        <thead>
          <tr>
            <th>{t('common.url')}</th>
            {#each testResult.rules as rule}
              <th class="ct-col-rule">{rule.name}</th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each pagedResults as page}
            <tr>
              <td class="cell-url" title={page.url}>{page.url}</td>
              {#each testResult.rules as rule}
                {@const val = page.results[rule.id] ?? ''}
                <td>
                  {#if val === 'pass'}
                    <span class="badge badge-success">{t('tests.pass')}</span>
                  {:else if val === 'fail'}
                    <span class="badge badge-error">{t('tests.fail')}</span>
                  {:else}
                    <span class="text-xs" title={val}>{val.length > 60 ? val.slice(0, 60) + '...' : val || '-'}</span>
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if testResult.pages.length > RESULTS_PAGE_SIZE}
      <div class="pagination">
        <button class="btn btn-sm" disabled={resultsPage === 0} onclick={() => resultsPage--}>{t('common.previous')}</button>
        <span class="text-sm text-muted">
          {resultsPage * RESULTS_PAGE_SIZE + 1} - {Math.min((resultsPage + 1) * RESULTS_PAGE_SIZE, testResult.pages.length)} of {testResult.pages.length}
        </span>
        <button class="btn btn-sm" disabled={!hasMoreResults} onclick={() => resultsPage++}>{t('common.next')}</button>
      </div>
    {/if}
  </div>
{/if}

{#if confirmState}<ConfirmModal message={confirmState.message} danger={confirmState.danger} confirmLabel={confirmState.confirmLabel} onconfirm={() => { confirmState.onConfirm(); confirmState = null; }} oncancel={() => confirmState = null} />{/if}

<style>
  .ct-card-header { display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-bottom: 1px solid var(--border); }
  .ct-title { margin: 0; font-size: 15px; font-weight: 600; }
  .ct-title-mb { margin: 0 0 12px 0; font-size: 15px; font-weight: 600; }
  .ct-empty { padding: 40px; text-align: center; color: var(--text-muted); }
  .ct-col-actions { width: 200px; }
  .ct-col-type { width: 200px; }
  .ct-col-remove { width: 40px; }
  .ct-col-rule { min-width: 100px; }
  .ct-editor-header { padding: 16px 20px; border-bottom: 1px solid var(--border); }
  .ct-editor-body { padding: 16px 20px; }
  .ct-name-input { flex: 1; padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 13px; }
  .ct-warning { background: #fff3cd; color: #856404; border: 1px solid #ffc107; border-radius: 6px; padding: 8px 12px; margin-bottom: 12px; font-size: 13px; }
  .ct-input { width: 100%; padding: 4px 6px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 12px; }
  .ct-remove-btn { padding: 2px 6px; font-size: 16px; color: var(--error); }
  .ct-spacer { flex: 1; }
  .ct-results-summary { display: flex; flex-wrap: wrap; gap: 12px; padding: 16px 20px; border-bottom: 1px solid var(--border); }
  .ct-result-card { background: var(--bg-secondary); border-radius: 8px; padding: 10px 14px; min-width: 140px; }
  .ct-result-value { font-size: 18px; font-weight: 600; }
  .ct-result-sublabel { font-size: 11px; color: var(--text-muted); }
</style>