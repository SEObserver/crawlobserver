<script>
  import { getRulesets, getRuleset, createRuleset, updateRuleset, deleteRuleset, runTests } from '../api.js';

  let { sessionId, onerror } = $props();

  const RULE_TYPES = [
    { value: 'string_contains', label: 'String contains', target: 'HTML' },
    { value: 'string_not_contains', label: 'String not contains', target: 'HTML' },
    { value: 'regex_match', label: 'Regex match', target: 'HTML' },
    { value: 'regex_not_match', label: 'Regex not match', target: 'HTML' },
    { value: 'header_exists', label: 'Header exists', target: 'Headers' },
    { value: 'header_not_exists', label: 'Header not exists', target: 'Headers' },
    { value: 'header_contains', label: 'Header contains', target: 'Headers' },
    { value: 'header_regex', label: 'Header regex', target: 'Headers' },
    { value: 'css_exists', label: 'CSS selector exists', target: 'HTML/CSS' },
    { value: 'css_not_exists', label: 'CSS selector not exists', target: 'HTML/CSS' },
    { value: 'css_extract_text', label: 'CSS extract text', target: 'HTML/CSS' },
    { value: 'css_extract_attr', label: 'CSS extract attribute', target: 'HTML/CSS' },
    { value: 'css_extract_all_text', label: 'CSS extract ALL text', target: 'HTML/CSS' },
    { value: 'css_extract_all_attr', label: 'CSS extract ALL attributes', target: 'HTML/CSS' },
    { value: 'regex_extract', label: 'Regex capture group', target: 'HTML' },
    { value: 'regex_extract_all', label: 'Regex capture ALL', target: 'HTML' },
    { value: 'xpath_extract', label: 'XPath extract', target: 'HTML/XPath' },
    { value: 'xpath_extract_all', label: 'XPath extract ALL', target: 'HTML/XPath' },
  ];

  function needsExtra(type) {
    return ['header_contains', 'header_regex', 'css_extract_attr', 'css_extract_all_attr'].includes(type);
  }

  function extraLabel(type) {
    if (type === 'header_contains' || type === 'header_regex') return 'Header value';
    if (type === 'css_extract_attr' || type === 'css_extract_all_attr') return 'Attribute name';
    return 'Extra';
  }

  function valueLabel(type) {
    if (type.startsWith('header_')) return 'Header name';
    if (type.startsWith('css_')) return 'CSS selector';
    if (type.startsWith('regex_')) return 'Regex pattern';
    if (type.startsWith('xpath_')) return 'XPath expression';
    return 'Search string';
  }

  function isAllRule(type) {
    return type.includes('_all_') || type.endsWith('_all');
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
    if (!editName.trim()) { onerror?.('Ruleset name is required'); return; }
    if (rules.length === 0) { onerror?.('At least one rule is required'); return; }

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

  async function removeRuleset(id) {
    if (!confirm('Delete this ruleset?')) return;
    try {
      await deleteRuleset(id);
      await loadRulesets();
    } catch (e) {
      onerror?.(e.message);
    }
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
  <div class="card" style="margin-top: 16px;">
    <div style="display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-bottom: 1px solid var(--border);">
      <h3 style="margin: 0; font-size: 15px; font-weight: 600;">Rulesets</h3>
      <button class="btn btn-primary btn-sm" onclick={newRuleset}>+ New Ruleset</button>
    </div>

    {#if loading}
      <div style="padding: 40px; text-align: center; color: var(--text-muted);">Loading...</div>
    {:else if rulesets.length === 0}
      <div style="padding: 40px; text-align: center; color: var(--text-muted);">
        No rulesets yet. Create one to start testing pages.
      </div>
    {:else}
      <table class="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Rules</th>
            <th>Updated</th>
            <th style="width: 200px;">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each rulesets as rs}
            <tr>
              <td><strong>{rs.name}</strong></td>
              <td>{rs.rules?.length ?? 0} rules</td>
              <td style="color: var(--text-muted); font-size: 13px;">{new Date(rs.updated_at).toLocaleDateString()}</td>
              <td>
                <button class="btn btn-sm" onclick={() => editRulesetById(rs.id)}>Edit</button>
                <button class="btn btn-primary btn-sm" onclick={() => runRuleset(rs.id)}
                  disabled={runningRulesetId === rs.id}>
                  {runningRulesetId === rs.id ? 'Running...' : 'Run'}
                </button>
                <button class="btn btn-sm btn-danger" onclick={() => removeRuleset(rs.id)}>Delete</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>

{:else if view === 'editor'}
  <div class="card" style="margin-top: 16px;">
    <div style="padding: 16px 20px; border-bottom: 1px solid var(--border);">
      <h3 style="margin: 0 0 12px 0; font-size: 15px; font-weight: 600;">
        {editId ? 'Edit Ruleset' : 'New Ruleset'}
      </h3>
      <div style="display: flex; gap: 8px; align-items: center;">
        <label style="font-size: 13px; font-weight: 500;">Name:</label>
        <input type="text" bind:value={editName} placeholder="Ruleset name"
          style="flex: 1; padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 13px;" />
      </div>
    </div>

    <div style="padding: 16px 20px;">
      {#if editRules.some(r => isAllRule(r.type))}
        <div style="background: #fff3cd; color: #856404; border: 1px solid #ffc107; border-radius: 6px; padding: 8px 12px; margin-bottom: 12px; font-size: 13px;">
          <strong>Warning:</strong> "Extract ALL" rules may produce large results on pages with many matching elements (capped at 20 items per page).
        </div>
      {/if}

      <table class="data-table" style="margin-bottom: 12px;">
        <thead>
          <tr>
            <th style="width: 200px;">Type</th>
            <th>Name</th>
            <th>Value</th>
            <th>Extra</th>
            <th style="width: 40px;"></th>
          </tr>
        </thead>
        <tbody>
          {#each editRules as rule, i}
            <tr>
              <td>
                <select bind:value={rule.type}
                  style="width: 100%; padding: 4px 6px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 12px;">
                  {#each RULE_TYPES as rt}
                    <option value={rt.value}>{rt.label}</option>
                  {/each}
                </select>
              </td>
              <td>
                <input type="text" bind:value={rule.name} placeholder="Rule label"
                  style="width: 100%; padding: 4px 6px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 12px;" />
              </td>
              <td>
                <input type="text" bind:value={rule.value} placeholder={valueLabel(rule.type)}
                  style="width: 100%; padding: 4px 6px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 12px;" />
              </td>
              <td>
                {#if needsExtra(rule.type)}
                  <input type="text" bind:value={rule.extra} placeholder={extraLabel(rule.type)}
                    style="width: 100%; padding: 4px 6px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 12px;" />
                {:else}
                  <span style="color: var(--text-muted); font-size: 12px;">-</span>
                {/if}
              </td>
              <td>
                {#if editRules.length > 1}
                  <button class="btn-ghost" style="padding: 2px 6px; font-size: 16px; color: var(--error);" onclick={() => removeRule(i)}>x</button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>

      <div style="display: flex; gap: 8px;">
        <button class="btn btn-sm" onclick={addRule}>+ Add Rule</button>
        <div style="flex: 1;"></div>
        <button class="btn btn-sm" onclick={() => { view = 'list'; }}>Cancel</button>
        <button class="btn btn-primary btn-sm" onclick={saveRuleset} disabled={loading}>
          {loading ? 'Saving...' : 'Save Ruleset'}
        </button>
      </div>
    </div>
  </div>

{:else if view === 'results' && testResult}
  <div class="card" style="margin-top: 16px;">
    <div style="display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-bottom: 1px solid var(--border);">
      <div>
        <h3 style="margin: 0; font-size: 15px; font-weight: 600;">
          Results: {testResult.ruleset_name}
        </h3>
        <span style="font-size: 13px; color: var(--text-muted);">{testResult.total_pages} pages tested</span>
      </div>
      <button class="btn btn-sm" onclick={() => { view = 'list'; }}>Back to Rulesets</button>
    </div>

    <!-- Summary -->
    <div style="display: flex; flex-wrap: wrap; gap: 12px; padding: 16px 20px; border-bottom: 1px solid var(--border);">
      {#each testResult.rules as rule}
        {@const count = testResult.summary[rule.id] ?? 0}
        {@const total = testResult.total_pages}
        {@const pct = total > 0 ? Math.round(count / total * 100) : 0}
        {@const isExtract = rule.type.includes('extract')}
        <div style="background: var(--bg-secondary); border-radius: 8px; padding: 10px 14px; min-width: 140px;">
          <div style="font-size: 12px; color: var(--text-muted); margin-bottom: 4px;">{rule.name}</div>
          {#if isExtract}
            <div style="font-size: 18px; font-weight: 600;">{count}/{total}</div>
            <div style="font-size: 11px; color: var(--text-muted);">have value</div>
          {:else}
            <div style="font-size: 18px; font-weight: 600; color: {pct >= 80 ? 'var(--success)' : pct >= 50 ? 'var(--warning)' : 'var(--error)'};">{pct}%</div>
            <div style="font-size: 11px; color: var(--text-muted);">{count}/{total} pass</div>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Results table -->
    <div style="overflow-x: auto;">
      <table class="data-table">
        <thead>
          <tr>
            <th>URL</th>
            {#each testResult.rules as rule}
              <th style="min-width: 100px;">{rule.name}</th>
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
                    <span class="badge badge-success">pass</span>
                  {:else if val === 'fail'}
                    <span class="badge badge-error">fail</span>
                  {:else}
                    <span style="font-size: 12px;" title={val}>{val.length > 60 ? val.slice(0, 60) + '...' : val || '-'}</span>
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
        <button class="btn btn-sm" disabled={resultsPage === 0} onclick={() => resultsPage--}>Previous</button>
        <span style="font-size: 13px; color: var(--text-muted);">
          {resultsPage * RESULTS_PAGE_SIZE + 1} - {Math.min((resultsPage + 1) * RESULTS_PAGE_SIZE, testResult.pages.length)} of {testResult.pages.length}
        </span>
        <button class="btn btn-sm" disabled={!hasMoreResults} onclick={() => resultsPage++}>Next</button>
      </div>
    {/if}
  </div>
{/if}
