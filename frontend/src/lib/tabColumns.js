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

export const TABS = [
  { id: 'overview', label: 'All Pages' },
  { id: 'titles', label: 'Titles' },
  { id: 'meta', label: 'Meta' },
  { id: 'headings', label: 'H1/H2' },
  { id: 'images', label: 'Images' },
  { id: 'indexability', label: 'Indexability' },
  { id: 'response', label: 'Response' },
  { id: 'internal', label: 'Internal Links' },
  { id: 'external', label: 'External Links' },
  { id: 'pagerank', label: 'PageRank' },
  { id: 'robots', label: 'Robots.txt' },
  { id: 'sitemaps', label: 'Sitemaps' },
  { id: 'gsc', label: 'Search Console' },
  { id: 'reports', label: 'Reports' },
  { id: 'tests', label: 'Tests' },
];
