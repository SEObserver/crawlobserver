/**
 * Client-side router — pure functions, no framework dependency.
 */

/** Push a new URL to the browser history. */
export function pushURL(path, queryFilters = {}, offset = 0) {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(queryFilters)) {
    if (v !== '' && v != null) params.set(k, v);
  }
  if (offset > 0) params.set('offset', String(offset));
  const qs = params.toString();
  const full = qs ? `${path}?${qs}` : path;
  if (window.location.pathname + window.location.search !== full) {
    history.pushState(null, '', full);
  }
}

/** Parse the current URL into a route descriptor. */
export function parseRoute() {
  const path = window.location.pathname;
  const search = window.location.search;

  // Top-level pages
  if (path === '/new-crawl') return { page: 'new-crawl' };
  if (path === '/settings') return { page: 'settings' };
  if (path === '/stats') return { page: 'stats' };
  if (path === '/api') return { page: 'api' };
  if (path === '/logs') return { page: 'logs' };
  if (path === '/compare') {
    const sp = new URLSearchParams(search);
    return { page: 'compare', sessionA: sp.get('a') || '', sessionB: sp.get('b') || '' };
  }

  // All projects page
  if (path === '/projects') return { page: 'all-projects' };

  // Project view
  const projMatch = path.match(/^\/projects\/([^/]+)(?:\/([^/]+)(?:\/([^/]+))?)?/);
  if (projMatch) {
    return { page: 'project', projectId: projMatch[1], projectTab: projMatch[2] || 'sessions', projectSubView: projMatch[3] || null };
  }

  // URL detail
  const urlMatch = path.match(/^\/sessions\/([^/]+)\/url\/(.+)/);
  if (urlMatch) {
    return { sessionId: urlMatch[1], tab: 'url-detail', detailUrl: decodeURIComponent(urlMatch[2]), filters: {}, offset: 0 };
  }

  // Session detail
  const m = path.match(/^\/sessions\/([^/]+)(?:\/([^/]+)(?:\/([^/]+))?)?/);
  if (m) {
    const sp = new URLSearchParams(search);
    const routeFilters = {};
    let routeOffset = 0;
    for (const [k, v] of sp.entries()) {
      if (k === 'offset') {
        routeOffset = parseInt(v, 10) || 0;
      } else if (k !== 'limit') {
        routeFilters[k] = v;
      }
    }
    return { sessionId: m[1], tab: m[2] || 'overview', subView: m[3] || null, filters: routeFilters, offset: routeOffset };
  }

  // Home
  return { page: 'home' };
}
