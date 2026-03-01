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

// SVG path data for each tab icon (24x24 viewBox, stroke-based)
export const TAB_ICONS = {
  reports:    '<path d="M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2"/><rect x="9" y="3" width="6" height="4" rx="1"/><path d="M9 14l2 2 4-4"/>',
  pages:      '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/>',
  links:      '<path d="M15 7h3a5 5 0 0 1 0 10h-3m-6 0H6a5 5 0 0 1 0-10h3"/><line x1="8" y1="12" x2="16" y2="12"/>',
  resources:  '<line x1="16.5" y1="9.4" x2="7.5" y2="4.21"/><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/>',
  pagerank:   '<circle cx="11" cy="11" r="2.5" fill="currentColor" stroke="none"/><circle cx="13" cy="4" r="1.5" fill="currentColor" stroke="none"/><circle cx="20" cy="10" r="1.5" fill="currentColor" stroke="none"/><circle cx="16" cy="17" r="1.5" fill="currentColor" stroke="none"/><circle cx="5" cy="17" r="1.5" fill="currentColor" stroke="none"/><path d="M11 11l2-7M11 11l9-1M11 11l5 6M11 11l-6 6M20 10l-4 7M20 10l-7-6"/>',
  directives: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
  tests:      '<polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/>',
  authority:  '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M12 8v4m0 4h.01"/>',
};

export function getTabs() {
  return [
    { id: 'reports',    label: t('tabs.reports'),    icon: TAB_ICONS.reports },
    { id: 'pages',      label: t('tabs.pages'),      icon: TAB_ICONS.pages },
    { id: 'links',      label: t('tabs.links'),      icon: TAB_ICONS.links },
    { id: 'resources',  label: t('tabs.resources'),   icon: TAB_ICONS.resources },
    { id: 'pagerank',   label: t('tabs.pagerank'),    icon: TAB_ICONS.pagerank },
    { id: 'directives', label: t('tabs.directives'),  icon: TAB_ICONS.directives },
    { id: 'authority',  label: t('tabs.authority'),    icon: TAB_ICONS.authority },
    { id: 'tests',      label: t('tabs.tests'),       icon: TAB_ICONS.tests },
  ];
}

export const TAB_SUB_VIEWS = {
  reports:    ['overview', 'content', 'technical', 'links', 'structure', 'sitemaps', 'international'],
  pages:      ['all', 'titles', 'meta', 'headings', 'images', 'indexability', 'response'],
  links:      ['internal', 'external', 'checks'],
  resources:  ['summary', 'urls'],
  pagerank:   ['top', 'directory', 'distribution', 'table'],
  directives: ['robots', 'sitemaps'],
  authority:  null,
  tests:      null,
};

export const TAB_DEFAULT_SUB_VIEW = {
  reports:    'overview',
  pages:      'all',
  links:      'internal',
  resources:  'summary',
  pagerank:   'top',
  directives: 'robots',
  authority:  null,
  tests:      null,
};

// Map old flat tab IDs to { tab, subView } for URL compat
export const OLD_TAB_REDIRECT = {
  overview:      { tab: 'pages',      subView: 'all' },
  titles:        { tab: 'pages',      subView: 'titles' },
  meta:          { tab: 'pages',      subView: 'meta' },
  headings:      { tab: 'pages',      subView: 'headings' },
  images:        { tab: 'pages',      subView: 'images' },
  indexability:   { tab: 'pages',      subView: 'indexability' },
  response:      { tab: 'pages',      subView: 'response' },
  internal:      { tab: 'links',      subView: 'internal' },
  external:      { tab: 'links',      subView: 'external' },
  'ext-checks':  { tab: 'links',      subView: 'checks' },
  robots:        { tab: 'directives', subView: 'robots' },
  sitemaps:      { tab: 'directives', subView: 'sitemaps' },
};

// Keep TABS as alias for backward compatibility during migration
export const TABS = null; // Removed — use getTabs()
