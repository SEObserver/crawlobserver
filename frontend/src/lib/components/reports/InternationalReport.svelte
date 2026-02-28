<script>
  import { fmtN, a11yKeydown } from '../../utils.js';
  import { t } from '../../i18n/index.svelte.js';
  import HBarChart from '../charts/HBarChart.svelte';

  let { stats, audit, sessionId, onnavigate } = $props();

  function nav(tab, filters = {}) { onnavigate?.(`/sessions/${sessionId}/${tab}`, filters); }

  const intl = $derived(audit?.international);

  function langBars(i) {
    if (!i?.lang_distribution) return [];
    return i.lang_distribution.map(l => ({
      label: l.lang || '(none)',
      value: l.count,
      color: 'chart-bar-accent',
      onclick: () => nav('overview'),
    }));
  }

  function schemaBars(i) {
    if (!i?.schema_distribution) return [];
    return i.schema_distribution.map(s => ({
      label: s.schema_type,
      value: s.count,
      color: 'chart-bar-info',
      onclick: () => nav('overview'),
    }));
  }

  const lBars = $derived(langBars(intl));
  const sBars = $derived(schemaBars(intl));
</script>

{#if intl}
  <div class="report-section">
    <h3 class="chart-title">{t('report.international.schemaTypes')}</h3>
    {#if sBars.length > 0}
      <HBarChart bars={sBars} />
    {:else}
      <p class="chart-empty">{t('report.international.noSchema')}</p>
    {/if}
    <div class="stats-grid mt-md">
      <div class="stat-card stat-card-link" role="button" tabindex="0"
        onclick={() => nav('overview')} onkeydown={a11yKeydown(() => nav('overview'))}>
        <div class="stat-value">{fmtN(intl.pages_with_schema || 0)}</div><div class="stat-label">{t('report.international.pagesWithSchema')}</div>
      </div>
    </div>
  </div>

  <div class="report-section">
    <h3 class="chart-title">{t('report.international.langDist')}</h3>
    {#if lBars.length > 0}
      <HBarChart bars={lBars} />
    {:else}
      <p class="chart-empty">{t('report.international.noLang')}</p>
    {/if}
  </div>

  <div class="report-section">
    <h3 class="chart-title">{t('report.international.hreflang')}</h3>
    <div class="stats-grid">
      <div class="stat-card stat-card-link" role="button" tabindex="0"
        onclick={() => nav('overview')} onkeydown={a11yKeydown(() => nav('overview'))}>
        <div class="stat-value">{fmtN(intl.pages_with_hreflang || 0)}</div><div class="stat-label">{t('report.international.pagesWithHreflang')}</div>
      </div>
      <div class="stat-card stat-card-link" role="button" tabindex="0"
        onclick={() => nav('overview')} onkeydown={a11yKeydown(() => nav('overview'))}>
        <div class="stat-value">{fmtN(intl.pages_with_lang || 0)}</div><div class="stat-label">{t('report.international.pagesWithLang')}</div>
      </div>
    </div>
  </div>
{:else}
  <p class="chart-empty">{t('report.international.noData')}</p>
{/if}
