<script>
  import { getProjectsPaginated } from '../api.js';
  import { timeAgo } from '../utils.js';
  import { t } from '../i18n/index.svelte.js';

  let { onerror, onselectproject, oncreateproject } = $props();

  let projects = $state([]);
  let total = $state(0);
  let loading = $state(false);
  let offset = $state(0);
  const limit = 30;

  let searchInput = $state('');
  let search = $state('');

  async function loadProjects() {
    loading = true;
    try {
      const res = await getProjectsPaginated(limit, offset, search);
      projects = res.projects || [];
      total = res.total || 0;
    } catch (e) { onerror?.(e.message); }
    loading = false;
  }

  function applySearch() {
    search = searchInput;
    offset = 0;
    loadProjects();
  }

  function prevPage() {
    if (offset >= limit) { offset -= limit; loadProjects(); }
  }

  function nextPage() {
    if (offset + limit < total) { offset += limit; loadProjects(); }
  }

  loadProjects();
</script>

<div class="page-header">
  <h1>{t('projects.title')}</h1>
  <div class="flex-center-gap">
    <button class="btn btn-sm" onclick={loadProjects} disabled={loading}>
      {loading ? t('common.loading') : t('common.refresh')}
    </button>
    <button class="btn btn-sm btn-primary" onclick={() => oncreateproject?.()}>{t('projects.newProject')}</button>
  </div>
</div>

<div class="projects-filters">
  <form class="projects-search-form" onsubmit={(e) => { e.preventDefault(); applySearch(); }}>
    <input class="projects-search" type="text" placeholder={t('projects.searchProjects')} bind:value={searchInput} />
    <button class="btn btn-sm" type="submit">{t('common.search')}</button>
  </form>
  <span class="projects-count">{total} {t('projects.projectCount')}</span>
</div>

{#if loading && projects.length === 0}
  <div class="loading">{t('common.loading')}</div>
{:else}
  <div class="card card-flush">
    <table>
      <thead>
        <tr>
          <th>{t('common.name')}</th>
          <th class="col-created">{t('projects.created')}</th>
          <th class="col-actions"></th>
        </tr>
      </thead>
      <tbody>
        {#each projects as proj}
          <tr>
            <td>
              <a href={`/projects/${proj.id}`} onclick={(e) => { e.preventDefault(); onselectproject?.(proj); }}>
                {proj.name}
              </a>
            </td>
            <td class="nowrap text-muted text-sm">
              {proj.created_at ? timeAgo(proj.created_at) : '-'}
            </td>
            <td>
              <button class="btn btn-sm" onclick={() => onselectproject?.(proj)}>{t('common.view')}</button>
            </td>
          </tr>
        {:else}
          <tr><td colspan="3" class="empty-message">{t('projects.noProjects')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  {#if total > limit}
    <div class="pagination">
      <button class="btn btn-sm" onclick={prevPage} disabled={offset === 0}>{t('common.previous')}</button>
      <span class="pagination-info">{offset + 1}-{Math.min(offset + limit, total)} {t('common.of')} {total}</span>
      <button class="btn btn-sm" onclick={nextPage} disabled={offset + limit >= total}>{t('common.next')}</button>
    </div>
  {/if}
{/if}

<style>
  .projects-filters {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
    flex-wrap: wrap;
    align-items: center;
  }
  .projects-search-form {
    display: flex;
    gap: 4px;
    flex: 1;
    min-width: 200px;
  }
  .projects-search {
    flex: 1;
    padding: 6px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
  }
  .projects-count {
    font-size: 13px;
    color: var(--text-muted);
    white-space: nowrap;
  }
  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-top: 12px;
  }
  .pagination-info {
    font-size: 13px;
    color: var(--text-muted);
  }
  .col-created {
    width: 180px;
  }
  .col-actions {
    width: 100px;
  }
  .empty-message {
    text-align: center;
    padding: 24px;
    color: var(--text-muted);
  }
</style>
