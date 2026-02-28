import { t } from './i18n/index.svelte.js';

export const PAGE_SIZE = 100;

export const TAB_FILTERS = {
  overview:     ['url', 'status_code', 'title', 'word_count', 'internal_links_out', 'external_links_out', 'body_size', 'fetch_duration_ms', 'depth', 'pagerank'],
  titles:       ['url', 'title', 'title_length', 'h1'],
  meta:         ['url', 'meta_description', 'meta_desc_length', 'meta_keywords', 'og_title'],
  headings:     ['url', 'h1', 'h2'],
  images:       ['url', 'images_count', 'images_no_alt', 'title', 'word_count'],
  indexability: ['url', 'is_indexable', 'index_reason', 'meta_robots', 'canonical', 'canonical_is_self'],
  response:     ['url', 'status_code', 'content_type', 'content_encoding', 'body_size', 'fetch_duration_ms'],
  internal:     ['source_url', 'target_url', 'anchor_text', 'tag'],
  external:     ['source_url', 'target_url', 'anchor_text', 'rel'],
};

export function getTabs() {
  return [
    { id: 'overview', label: t('tabs.allPages') },
    { id: 'titles', label: t('tabs.titles') },
    { id: 'meta', label: t('tabs.meta') },
    { id: 'headings', label: t('tabs.h1h2') },
    { id: 'images', label: t('tabs.images') },
    { id: 'indexability', label: t('tabs.indexability') },
    { id: 'response', label: t('tabs.response') },
    { id: 'internal', label: t('tabs.internalLinks') },
    { id: 'external', label: t('tabs.externalLinks') },
    { id: 'ext-checks', label: t('tabs.extChecks') },
    { id: 'resources', label: t('tabs.resources') },
    { id: 'pagerank', label: t('tabs.pagerank') },
    { id: 'robots', label: t('tabs.robotsTxt') },
    { id: 'sitemaps', label: t('tabs.sitemaps') },
    { id: 'reports', label: t('tabs.reports') },
    { id: 'tests', label: t('tabs.tests') },
  ];
}

// Keep TABS as alias for backward compatibility during migration
export const TABS = null; // Removed — use getTabs()
