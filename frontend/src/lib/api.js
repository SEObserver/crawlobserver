/**
 * @typedef {Object} Session
 * @property {string} ID
 * @property {string[]} SeedURLs
 * @property {string} Status
 * @property {string} ProjectID
 * @property {number} PagesCrawled
 * @property {string} StartedAt
 * @property {string} FinishedAt
 * @property {boolean} is_running
 */

/**
 * @typedef {Object} Page
 * @property {string} URL
 * @property {string} FinalURL
 * @property {number} StatusCode
 * @property {string} Title
 * @property {number} TitleLength
 * @property {string} MetaDescription
 * @property {number} MetaDescLength
 * @property {string} MetaKeywords
 * @property {string} MetaRobots
 * @property {string} Canonical
 * @property {boolean} CanonicalIsSelf
 * @property {boolean} IsIndexable
 * @property {string} IndexReason
 * @property {string} ContentType
 * @property {string} ContentEncoding
 * @property {number} BodySize
 * @property {number} WordCount
 * @property {number} FetchDurationMs
 * @property {number} Depth
 * @property {number} InternalLinksOut
 * @property {number} ExternalLinksOut
 * @property {number} ImagesCount
 * @property {number} ImagesNoAlt
 * @property {string[]} H1
 * @property {string[]} H2
 * @property {string} OGTitle
 * @property {number} PageRank
 */

/**
 * @typedef {Object} SessionStats
 * @property {number} total_pages
 * @property {number} internal_links
 * @property {number} external_links
 * @property {number} error_count
 * @property {number} avg_fetch_ms
 * @property {number} pages_per_second
 * @property {number} crawl_duration_sec
 * @property {Object<string, number>} status_codes
 * @property {Object<string, number>} depth_distribution
 */

/**
 * @typedef {Object} Link
 * @property {string} SourceURL
 * @property {string} TargetURL
 * @property {string} AnchorText
 * @property {string} Tag
 */

/**
 * @typedef {Object} Theme
 * @property {string} app_name
 * @property {string} logo_url
 * @property {string} accent_color
 * @property {string} mode
 */

/**
 * @typedef {Object} Project
 * @property {string} id
 * @property {string} name
 */

/**
 * @typedef {Object} ProgressEvent
 * @property {number} pages_crawled
 * @property {number} queue_size
 * @property {number} lost_pages
 */

/**
 * @typedef {Object} SSEHandle
 * @property {() => void} close
 */

const BASE = '/api';

export function buildApiPath(path, params = {}) {
  const qs = Object.entries(params)
    .filter(([, v]) => v !== '' && v != null)
    .map(([k, v]) => `${k}=${encodeURIComponent(v)}`)
    .join('&');
  return qs ? `${path}?${qs}` : path;
}

const DEFAULT_LIMIT = 100;
const PAGERANK_LIMIT = 50;
const PAGERANK_BUCKETS = 20;
const TREEMAP_DEPTH = 2;
const TREEMAP_MIN_PAGES = 1;

const SSE_RETRY_INIT_MS = 1000;
const SSE_RETRY_MAX_MS = 30000;
const SSE_MAX_RETRIES = 10;

/**
 * @param {string} path
 * @param {RequestInit} options
 * @returns {Promise<any>}
 */
async function fetchJSON(path, options = {}) {
  const res = await fetch(`${BASE}${path}`, options);
  if (!res.ok) {
    let errorMessage;
    try {
      errorMessage = (await res.json()).error;
    } catch {
      errorMessage = await res.text().catch(() => res.statusText);
    }
    throw new Error(errorMessage || `API error: ${res.status}`);
  }
  const text = await res.text();
  if (!text) return null;
  return JSON.parse(text);
}

/** @returns {Promise<Session[]>} */
export async function getSessions() {
  return fetchJSON('/sessions');
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {Object<string, string>} filters
 * @returns {Promise<Page[]>}
 */
export async function getPages(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/pages?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {Object<string, string>} filters
 * @returns {Promise<Link[]>}
 */
export async function getExternalLinks(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/links?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {Object<string, string>} filters
 * @returns {Promise<Link[]>}
 */
export async function getInternalLinks(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/internal-links?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @returns {Promise<SessionStats>}
 */
export async function getStats(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/stats`);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function getAudit(sessionId) {
  return fetchJSON(`/sessions/${encodeURIComponent(sessionId)}/audit`);
}

/**
 * @param {string} sessionId
 * @param {string} url
 * @returns {Promise<{html: string}>}
 */
export async function getPageHTML(sessionId, url) {
  return fetchJSON(`/sessions/${sessionId}/page-html?url=${encodeURIComponent(url)}`);
}

/**
 * @param {string} sessionId
 * @param {string} url
 * @param {number} outLimit
 * @param {number} outOffset
 * @param {number} inLimit
 * @param {number} inOffset
 * @returns {Promise<Object>}
 */
export async function getPageDetail(
  sessionId,
  url,
  outLimit = DEFAULT_LIMIT,
  outOffset = 0,
  inLimit = DEFAULT_LIMIT,
  inOffset = 0,
) {
  return fetchJSON(
    `/sessions/${sessionId}/page-detail?url=${encodeURIComponent(url)}&out_limit=${outLimit}&out_offset=${outOffset}&in_limit=${inLimit}&in_offset=${inOffset}`,
  );
}

/** @returns {Promise<Object>} */
export async function getStorageStats() {
  return fetchJSON('/storage-stats');
}

/** @returns {Promise<Object>} */
export async function getGlobalStats() {
  return fetchJSON('/global-stats');
}

/** @returns {Promise<Object<string, number>>} */
export async function getSessionStorage() {
  return fetchJSON('/session-storage');
}

/** @returns {Promise<Object>} */
export async function getSystemStats() {
  return fetchJSON('/system-stats');
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function getProgress(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/progress`);
}

/** @returns {Promise<Object>} */
export async function getHealth() {
  return fetchJSON('/health');
}

/** @returns {Promise<Object>} */
export async function getServerInfo() {
  return fetchJSON('/server-info');
}

/** @returns {Promise<Theme>} */
export async function getTheme() {
  return fetchJSON('/theme');
}

/**
 * @param {Theme} theme
 * @returns {Promise<Theme>}
 */
export async function updateTheme(theme) {
  return fetchJSON('/theme', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(theme),
  });
}

/**
 * @param {string[]} seeds
 * @param {Object} options
 * @returns {Promise<{session_id: string}>}
 */
export async function startCrawl(seeds, options = {}) {
  return fetchJSON('/crawl', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ seeds, ...options }),
  });
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function stopCrawl(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/stop`, { method: 'POST' });
}

/**
 * @param {string} sessionId
 * @param {Object|null} options
 * @returns {Promise<Object>}
 */
export async function resumeCrawl(sessionId, options = null) {
  const opts = { method: 'POST' };
  if (options) {
    opts.headers = { 'Content-Type': 'application/json' };
    opts.body = JSON.stringify(options);
  }
  return fetchJSON(`/sessions/${sessionId}/resume`, opts);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function deleteSession(sessionId) {
  return fetchJSON(`/sessions/${sessionId}`, { method: 'DELETE' });
}

export async function deleteUnassignedSessions() {
  return fetchJSON('/sessions-unassigned', { method: 'DELETE' });
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function recomputeDepths(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/recompute-depths`, { method: 'POST' });
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function computePageRank(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/compute-pagerank`, { method: 'POST' });
}

/**
 * @param {string} sessionId
 * @param {number} statusCode
 * @param {Object|null} options
 * @returns {Promise<Object>}
 */
export async function retryFailed(sessionId, statusCode = 0, options = null) {
  const qs = statusCode ? `?status_code=${statusCode}` : '';
  const opts = { method: 'POST' };
  if (options) {
    opts.headers = { 'Content-Type': 'application/json' };
    opts.body = JSON.stringify(options);
  }
  return fetchJSON(`/sessions/${sessionId}/retry-failed${qs}`, opts);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object[]>}
 */
export async function getStatusTimeline(sessionId) {
  return fetchJSON(`/sessions/${encodeURIComponent(sessionId)}/status-timeline`);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object[]>}
 */
export async function getStatusTimelineRecent(sessionId) {
  return fetchJSON(`/sessions/${encodeURIComponent(sessionId)}/status-timeline-recent`);
}

/**
 * @param {string} sessionId
 * @param {number} buckets
 * @returns {Promise<Object[]>}
 */
export async function getPageRankDistribution(sessionId, buckets = PAGERANK_BUCKETS) {
  return fetchJSON(`/sessions/${sessionId}/pagerank-distribution?buckets=${buckets}`);
}

/**
 * @param {string} sessionId
 * @param {number} depth
 * @param {number} minPages
 * @returns {Promise<Object>}
 */
export async function getPageRankTreemap(
  sessionId,
  depth = TREEMAP_DEPTH,
  minPages = TREEMAP_MIN_PAGES,
) {
  return fetchJSON(`/sessions/${sessionId}/pagerank-treemap?depth=${depth}&min_pages=${minPages}`);
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {string} directory
 * @returns {Promise<Object[]>}
 */
export async function getPageRankTop(
  sessionId,
  limit = PAGERANK_LIMIT,
  offset = 0,
  directory = '',
) {
  let url = `/sessions/${sessionId}/pagerank-top?limit=${limit}&offset=${offset}`;
  if (directory) url += `&directory=${encodeURIComponent(directory)}`;
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object[]>}
 */
export async function getRobotsHosts(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/robots`);
}

/**
 * @param {string} sessionId
 * @param {string} host
 * @returns {Promise<{content: string}>}
 */
export async function getRobotsContent(sessionId, host) {
  return fetchJSON(`/sessions/${sessionId}/robots-content?host=${encodeURIComponent(host)}`);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object[]>}
 */
export async function getSitemaps(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/sitemaps`);
}

/**
 * @param {string} sessionId
 * @param {string} url
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getSitemapURLs(sessionId, url, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(
    `/sessions/${sessionId}/sitemap-urls?url=${encodeURIComponent(url)}&limit=${limit}&offset=${offset}`,
  );
}

export async function getSitemapCoverageURLs(sessionId, filter, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(
    `/sessions/${sessionId}/sitemap-coverage-urls?filter=${encodeURIComponent(filter)}&limit=${limit}&offset=${offset}`,
  );
}

/**
 * @param {string} sessionId
 * @param {string} host
 * @param {string} userAgent
 * @param {string[]} urls
 * @returns {Promise<Object>}
 */
export async function testRobotsUrls(sessionId, host, userAgent, urls) {
  return fetchJSON(`/sessions/${sessionId}/robots-test`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ host, user_agent: userAgent, urls }),
  });
}

/**
 * @param {string} sessionId
 * @param {string} host
 * @param {string} userAgent
 * @param {string} newContent
 * @returns {Promise<Object>}
 */
export async function simulateRobots(sessionId, host, userAgent, newContent) {
  return fetchJSON(`/sessions/${sessionId}/robots-simulate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ host, user_agent: userAgent, new_content: newContent }),
  });
}

// --- Projects ---

/** @returns {Promise<Project[]>} */
export async function getProjects() {
  return fetchJSON('/projects');
}

/**
 * @param {number} limit
 * @param {number} offset
 * @param {string} search
 * @returns {Promise<Object>}
 */
export async function getProjectsPaginated(limit = 30, offset = 0, search = '') {
  let url = `/projects?limit=${limit}&offset=${offset}`;
  if (search) url += `&search=${encodeURIComponent(search)}`;
  return fetchJSON(url);
}

/**
 * @param {number} limit
 * @param {number} offset
 * @param {Object} opts
 * @param {string} [opts.projectId]
 * @param {string} [opts.search]
 * @returns {Promise<Object>}
 */
export async function getSessionsPaginated(limit = 30, offset = 0, { projectId, search } = {}) {
  let url = `/sessions?limit=${limit}&offset=${offset}`;
  if (projectId) url += `&project_id=${encodeURIComponent(projectId)}`;
  if (search) url += `&search=${encodeURIComponent(search)}`;
  return fetchJSON(url);
}

/**
 * @param {string} name
 * @returns {Promise<Project>}
 */
export async function createProject(name) {
  return fetchJSON('/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
}

/**
 * @param {string} id
 * @param {string} name
 * @returns {Promise<Project>}
 */
export async function renameProject(id, name) {
  return fetchJSON(`/projects/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
}

/**
 * @param {string} id
 * @returns {Promise<Object>}
 */
export async function deleteProject(id) {
  return fetchJSON(`/projects/${id}`, { method: 'DELETE' });
}

export async function deleteProjectWithSessions(id) {
  return fetchJSON(`/projects/${id}/with-sessions`, { method: 'DELETE' });
}

/**
 * @param {string} projectId
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function associateSession(projectId, sessionId) {
  return fetchJSON(`/projects/${projectId}/sessions/${sessionId}`, { method: 'POST' });
}

/**
 * @param {string} projectId
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function disassociateSession(projectId, sessionId) {
  return fetchJSON(`/projects/${projectId}/sessions/${sessionId}`, { method: 'DELETE' });
}

/**
 * @param {string} sessionId
 * @param {string} label
 * @returns {Promise<Object>}
 */
export async function renameSession(sessionId, label) {
  return fetchJSON(`/sessions/${sessionId}/label`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ label }),
  });
}

/**
 * @param {string} projectId
 * @param {string[]} sessionIds
 * @returns {Promise<Object>}
 */
export async function batchAssignSessions(projectId, sessionIds) {
  return fetchJSON(`/projects/${projectId}/sessions/batch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_ids: sessionIds }),
  });
}

// --- API Keys ---

/** @returns {Promise<Object[]>} */
export async function getAPIKeys() {
  return fetchJSON('/api-keys');
}

/**
 * @param {string} name
 * @param {string} type
 * @param {string|null} projectId
 * @returns {Promise<Object>}
 */
export async function createAPIKey(name, type, projectId = null) {
  return fetchJSON('/api-keys', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, type, project_id: projectId }),
  });
}

/**
 * @param {string} id
 * @returns {Promise<Object>}
 */
export async function deleteAPIKey(id) {
  return fetchJSON(`/api-keys/${id}`, { method: 'DELETE' });
}

// --- Updates ---

/** @returns {Promise<Object>} */
export async function getUpdateStatus() {
  return fetchJSON('/update/status');
}

/** @returns {Promise<Object>} */
export async function applyUpdate() {
  return fetchJSON('/update/apply', { method: 'POST' });
}

// --- Export / Import ---

/**
 * @param {string} sessionId
 * @param {boolean} includeHTML
 */
export function exportSession(sessionId, includeHTML = false) {
  const url = `${BASE}/sessions/${sessionId}/export?include_html=${includeHTML}`;
  window.open(url, '_blank');
}

/**
 * @param {File} file
 * @returns {Promise<Object>}
 */
export async function importSession(file) {
  const form = new FormData();
  form.append('file', file);
  const res = await fetch(`${BASE}/sessions/import`, { method: 'POST', body: form });
  if (!res.ok) {
    let errorMessage;
    try {
      errorMessage = (await res.json()).error;
    } catch {
      errorMessage = await res.text().catch(() => res.statusText);
    }
    throw new Error(errorMessage || `API error: ${res.status}`);
  }
  return res.json();
}

/**
 * @param {File} file
 * @param {string} [projectId]
 * @returns {Promise<Object>}
 */
export async function importCSVSession(file, projectId = '') {
  const form = new FormData();
  form.append('file', file);
  if (projectId) form.append('project_id', projectId);
  const res = await fetch(`${BASE}/sessions/import/csv`, { method: 'POST', body: form });
  if (!res.ok) {
    let errorMessage;
    try {
      errorMessage = (await res.json()).error;
    } catch {
      errorMessage = await res.text().catch(() => res.statusText);
    }
    throw new Error(errorMessage || `API error: ${res.status}`);
  }
  return res.json();
}

// --- Backups ---

/** @returns {Promise<Object[]>} */
export async function getBackups() {
  return fetchJSON('/backups');
}

/** @returns {Promise<Object>} */
export async function createBackup() {
  return fetchJSON('/backups', { method: 'POST' });
}

/**
 * @param {string} filename
 * @returns {Promise<Object>}
 */
export async function restoreBackup(filename) {
  return fetchJSON('/backups/restore', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filename }),
  });
}

/**
 * @param {string} name
 * @returns {Promise<Object>}
 */
export async function deleteBackup(name) {
  return fetchJSON(`/backups/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

// --- Google Search Console ---

/**
 * @param {string} projectId
 * @returns {Promise<Object>}
 */
export async function getGSCStatus(projectId) {
  return fetchJSON(`/projects/${projectId}/gsc/status`);
}

/**
 * @param {string} projectId
 * @returns {Promise<Object>}
 */
export function startGSCAuthorize(projectId) {
  return fetchJSON(`/gsc/authorize?project_id=${encodeURIComponent(projectId)}`);
}

/**
 * @param {string} projectId
 * @param {string} propertyUrl
 * @param {string} startDate
 * @param {string} endDate
 * @returns {Promise<Object>}
 */
export async function fetchGSCData(projectId, propertyUrl = '', startDate = '', endDate = '') {
  const body = {};
  if (propertyUrl) body.property_url = propertyUrl;
  if (startDate) body.start_date = startDate;
  if (endDate) body.end_date = endDate;
  return fetchJSON(`/projects/${projectId}/gsc/fetch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

/**
 * @param {string} projectId
 * @returns {Promise<Object>}
 */
export async function stopGSCFetch(projectId) {
  return fetchJSON(`/projects/${projectId}/gsc/stop`, { method: 'POST' });
}

/**
 * @param {string} projectId
 * @returns {Promise<Object>}
 */
export async function disconnectGSC(projectId) {
  return fetchJSON(`/projects/${projectId}/gsc/disconnect`, { method: 'DELETE' });
}

/**
 * @param {string} projectId
 * @returns {Promise<Object>}
 */
export async function getGSCOverview(projectId) {
  return fetchJSON(`/projects/${projectId}/gsc/overview`);
}

/**
 * @param {string} projectId
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getGSCQueries(projectId, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(`/projects/${projectId}/gsc/queries?limit=${limit}&offset=${offset}`);
}

/**
 * @param {string} projectId
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getGSCPages(projectId, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(`/projects/${projectId}/gsc/pages?limit=${limit}&offset=${offset}`);
}

/**
 * @param {string} projectId
 * @returns {Promise<Object[]>}
 */
export async function getGSCCountries(projectId) {
  return fetchJSON(`/projects/${projectId}/gsc/countries`);
}

/**
 * @param {string} projectId
 * @returns {Promise<Object[]>}
 */
export async function getGSCDevices(projectId) {
  return fetchJSON(`/projects/${projectId}/gsc/devices`);
}

/**
 * @param {string} projectId
 * @returns {Promise<Object[]>}
 */
export async function getGSCTimeline(projectId) {
  return fetchJSON(`/projects/${projectId}/gsc/timeline`);
}

/**
 * @param {string} projectId
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getGSCInspection(projectId, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(`/projects/${projectId}/gsc/inspection?limit=${limit}&offset=${offset}`);
}

// --- Setup & Telemetry ---

/** @returns {Promise<{setup_complete: boolean, download_progress: Object, clickhouse_ready: boolean}>} */
export async function getSetupStatus() {
  return fetchJSON('/setup/status');
}

/**
 * @param {Object} params
 * @returns {Promise<Object>}
 */
export async function completeSetup(params) {
  return fetchJSON('/setup/complete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  });
}

/** @returns {Promise<{enabled: boolean, instance_id: string}>} */
export async function getTelemetry() {
  return fetchJSON('/telemetry');
}

/**
 * @param {boolean} enabled
 * @returns {Promise<Object>}
 */
export async function updateTelemetry(enabled) {
  return fetchJSON('/telemetry', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  });
}

/**
 * @param {boolean} enabled
 * @returns {Promise<Object>}
 */
export async function updateSessionRecording(enabled) {
  return fetchJSON('/telemetry/session-recording', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  });
}

// --- External Link Checks ---

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {Object<string, string>} filters
 * @returns {Promise<Object[]>}
 */
export async function getExternalLinkChecks(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/external-checks?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {Object<string, string>} filters
 * @returns {Promise<Object[]>}
 */
export async function getExternalLinkCheckDomains(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/external-checks/domains?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getExpiredDomains(sessionId, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(
    `/sessions/${sessionId}/external-checks/expired-domains?limit=${limit}&offset=${offset}`,
  );
}

// --- Check IP ---

/**
 * @param {string} sourceIP
 * @param {boolean} forceIPv4
 * @returns {Promise<{ip: string}>}
 */
export async function checkIP(sourceIP = '', forceIPv4 = false) {
  return fetchJSON('/check-ip', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source_ip: sourceIP || undefined, force_ipv4: forceIPv4 || undefined }),
  });
}

// --- Page Resource Checks ---

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {Object<string, string>} filters
 * @returns {Promise<Object[]>}
 */
export async function getPageResourceChecks(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  filters = {},
) {
  let url = `/sessions/${sessionId}/resource-checks?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object>}
 */
export async function getPageResourceChecksSummary(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/resource-checks/summary`);
}

// --- Compare ---

/**
 * @param {string} a - Session ID A
 * @param {string} b - Session ID B
 * @returns {Promise<Object>}
 */
export async function getCompareStats(a, b) {
  return fetchJSON(`/compare/stats?a=${a}&b=${b}`);
}

/**
 * @param {string} a
 * @param {string} b
 * @param {string} type
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getComparePages(a, b, type = 'changed', limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(`/compare/pages?a=${a}&b=${b}&type=${type}&limit=${limit}&offset=${offset}`);
}

/**
 * @param {string} a
 * @param {string} b
 * @param {string} type
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getCompareLinks(a, b, type = 'added', limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(`/compare/links?a=${a}&b=${b}&type=${type}&limit=${limit}&offset=${offset}`);
}

// --- Rulesets (Custom Tests) ---

/** @returns {Promise<Object[]>} */
export async function getRulesets() {
  return fetchJSON('/rulesets');
}

/**
 * @param {string} id
 * @returns {Promise<Object>}
 */
export async function getRuleset(id) {
  return fetchJSON(`/rulesets/${id}`);
}

/**
 * @param {string} name
 * @param {Object[]} rules
 * @returns {Promise<Object>}
 */
export async function createRuleset(name, rules) {
  return fetchJSON('/rulesets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, rules }),
  });
}

/**
 * @param {string} id
 * @param {string} name
 * @param {Object[]} rules
 * @returns {Promise<Object>}
 */
export async function updateRuleset(id, name, rules) {
  return fetchJSON(`/rulesets/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, rules }),
  });
}

/**
 * @param {string} id
 * @returns {Promise<Object>}
 */
export async function deleteRuleset(id) {
  return fetchJSON(`/rulesets/${id}`, { method: 'DELETE' });
}

/**
 * @param {string} sessionId
 * @param {string} rulesetId
 * @returns {Promise<Object>}
 */
export async function runTests(sessionId, rulesetId) {
  return fetchJSON(`/sessions/${sessionId}/run-tests`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ruleset_id: rulesetId }),
  });
}

// --- Extractor Sets ---

export async function getExtractorSets() {
  return fetchJSON('/extractor-sets');
}

export async function getExtractorSet(id) {
  return fetchJSON(`/extractor-sets/${id}`);
}

export async function createExtractorSet(name, extractors) {
  return fetchJSON('/extractor-sets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, extractors }),
  });
}

export async function updateExtractorSet(id, name, extractors) {
  return fetchJSON(`/extractor-sets/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, extractors }),
  });
}

export async function deleteExtractorSet(id) {
  return fetchJSON(`/extractor-sets/${id}`, { method: 'DELETE' });
}

export async function getExtractions(sessionId, limit = 100, offset = 0) {
  return fetchJSON(`/sessions/${sessionId}/extractions?limit=${limit}&offset=${offset}`);
}

export async function runExtractions(sessionId, extractorSetId) {
  return fetchJSON(`/sessions/${sessionId}/run-extractions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ extractor_set_id: extractorSetId }),
  });
}

// --- Interlinking ---

export async function computeInterlinking(sessionId, options = {}) {
  return fetchJSON(`/sessions/${sessionId}/compute-interlinking`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(options),
  });
}

export async function getInterlinkingOpportunities(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  sort = '',
  order = '',
  filters = {},
) {
  let url = `/sessions/${sessionId}/interlinking-opportunities?limit=${limit}&offset=${offset}`;
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  for (const [key, value] of Object.entries(filters)) {
    if (value !== '' && value != null)
      url += `&${encodeURIComponent(key)}=${encodeURIComponent(value)}`;
  }
  return fetchJSON(url);
}

export async function simulateInterlinking(sessionId, links) {
  return fetchJSON(`/sessions/${sessionId}/simulate-interlinking`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ links }),
  });
}

export async function importVirtualLinks(sessionId, links) {
  return fetchJSON(`/sessions/${sessionId}/import-virtual-links`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ links }),
  });
}

export async function getInterlinkingSimulations(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/interlinking-simulations`);
}

export async function getInterlinkingSimulation(
  sessionId,
  simId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/interlinking-simulations/${simId}?limit=${limit}&offset=${offset}`;
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

// --- Application Logs ---

/**
 * @param {number} limit
 * @param {number} offset
 * @param {string} level
 * @param {string} component
 * @param {string} search
 * @returns {Promise<Object[]>}
 */
export async function getLogs(
  limit = DEFAULT_LIMIT,
  offset = 0,
  level = '',
  component = '',
  search = '',
) {
  let url = `/logs?limit=${limit}&offset=${offset}`;
  if (level) url += `&level=${encodeURIComponent(level)}`;
  if (component) url += `&component=${encodeURIComponent(component)}`;
  if (search) url += `&search=${encodeURIComponent(search)}`;
  return fetchJSON(url);
}

export function exportLogs() {
  window.open(`${BASE}/logs/export`, '_blank');
}

// --- External Data Providers ---

/**
 * @param {string} projectId
 * @returns {Promise<Object[]>}
 */
export async function getProviderConnections(projectId) {
  return fetchJSON(`/projects/${projectId}/providers`);
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @param {string} apiKey
 * @param {string} domain
 * @returns {Promise<Object>}
 */
export async function connectProvider(projectId, provider, apiKey, domain, limits = {}) {
  return fetchJSON(`/projects/${projectId}/providers/${provider}/connect`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      api_key: apiKey || '',
      domain,
      limit_backlinks: limits.backlinks || 0,
      limit_refdomains: limits.refdomains || 0,
      limit_rankings: limits.rankings || 0,
      limit_top_pages: limits.top_pages || 0,
    }),
  });
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @returns {Promise<Object>}
 */
export async function disconnectProvider(projectId, provider) {
  return fetchJSON(`/projects/${projectId}/providers/${provider}/disconnect`, { method: 'DELETE' });
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @returns {Promise<Object>}
 */
export async function getProviderStatus(projectId, provider) {
  return fetchJSON(`/projects/${projectId}/providers/${provider}/status`);
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @param {string[]} dataTypes
 * @returns {Promise<Object>}
 */
export async function fetchProviderData(projectId, provider, dataTypes = [], force = false) {
  const body = {};
  if (dataTypes.length > 0) body.data_types = dataTypes;
  if (force) body.force = true;
  return fetchJSON(`/projects/${projectId}/providers/${provider}/fetch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @returns {Promise<Object>}
 */
export async function stopProviderFetch(projectId, provider) {
  return fetchJSON(`/projects/${projectId}/providers/${provider}/stop`, { method: 'POST' });
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @returns {Promise<Object>}
 */
export async function getProviderMetrics(projectId, provider) {
  return fetchJSON(`/projects/${projectId}/providers/${provider}/metrics`);
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getProviderBacklinks(projectId, provider, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(
    `/projects/${projectId}/providers/${provider}/backlinks?limit=${limit}&offset=${offset}`,
  );
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getProviderRefDomains(
  projectId,
  provider,
  limit = DEFAULT_LIMIT,
  offset = 0,
) {
  return fetchJSON(
    `/projects/${projectId}/providers/${provider}/refdomains?limit=${limit}&offset=${offset}`,
  );
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @param {number} limit
 * @param {number} offset
 * @returns {Promise<Object[]>}
 */
export async function getProviderRankings(projectId, provider, limit = DEFAULT_LIMIT, offset = 0) {
  return fetchJSON(
    `/projects/${projectId}/providers/${provider}/rankings?limit=${limit}&offset=${offset}`,
  );
}

/**
 * @param {string} projectId
 * @param {string} provider
 * @returns {Promise<Object>}
 */
export async function getProviderVisibility(projectId, provider) {
  return fetchJSON(`/projects/${projectId}/providers/${provider}/visibility`);
}

/** @returns {Promise<{rows: Array, total: number}>} */
export async function getProviderTopPages(projectId, provider, limit = 100, offset = 0) {
  return fetchJSON(
    `/projects/${projectId}/providers/${provider}/top-pages?limit=${limit}&offset=${offset}`,
  );
}

/** @returns {Promise<{rows: Array, total: number}>} */
export async function getProviderData(
  projectId,
  provider,
  dataType,
  limit = 100,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/projects/${projectId}/providers/${provider}/data/${dataType}?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/** @returns {Promise<{rows: Array, total: number}>} */
export async function getProviderAPICalls(projectId, provider, limit = 50, offset = 0) {
  return fetchJSON(
    `/projects/${projectId}/providers/${provider}/api-calls?limit=${limit}&offset=${offset}`,
  );
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {number} threshold
 * @returns {Promise<{pairs: Array, total: number}>}
 */
export async function getNearDuplicates(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  threshold = 3,
) {
  return fetchJSON(
    `/sessions/${sessionId}/near-duplicates?limit=${limit}&offset=${offset}&threshold=${threshold}`,
  );
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @param {number} offset
 * @param {Object<string, string>} filters
 * @param {string} sort
 * @param {string} order
 * @returns {Promise<Object[]>}
 */
export async function getRedirectPages(
  sessionId,
  limit = DEFAULT_LIMIT,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/redirect-pages?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/**
 * @param {string} sessionId
 * @param {string} projectId
 * @param {number} limit
 * @param {number} offset
 * @param {string} directory
 * @returns {Promise<Object>}
 */
export async function getPageRankWeightedTop(
  sessionId,
  projectId,
  limit = PAGERANK_LIMIT,
  offset = 0,
  directory = '',
  sort = '',
  order = '',
) {
  let url = `/sessions/${sessionId}/pagerank-weighted-top?project_id=${encodeURIComponent(projectId)}&limit=${limit}&offset=${offset}`;
  if (directory) url += `&directory=${encodeURIComponent(directory)}`;
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  return fetchJSON(url);
}

/** @returns {Promise<{backlinks: Array, total: number}>} */
export async function getBacklinksTop(
  projectId,
  limit = 100,
  offset = 0,
  filters = {},
  sort = '',
  order = '',
) {
  let url = `/backlinks/top?project_id=${encodeURIComponent(projectId)}&limit=${limit}&offset=${offset}`;
  if (sort) url += `&sort=${encodeURIComponent(sort)}`;
  if (order) url += `&order=${encodeURIComponent(order)}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${encodeURIComponent(k)}=${encodeURIComponent(v)}`;
  }
  return fetchJSON(url);
}

/** @returns {Promise<{rows: Array, total: number}>} */
export async function getSessionAuthority(sessionId, projectId, limit = 100, offset = 0) {
  return fetchJSON(
    `/sessions/${sessionId}/authority?project_id=${projectId}&limit=${limit}&offset=${offset}`,
  );
}

/**
 * Subscribe to SSE progress events with automatic reconnection and exponential backoff.
 * @param {string} sessionId
 * @param {(data: ProgressEvent) => void} onMessage
 * @param {() => void} onDone
 * @returns {SSEHandle}
 */
export function subscribeProgress(sessionId, onMessage, onDone, onStatsReady) {
  let retryDelay = SSE_RETRY_INIT_MS;
  let retries = 0;
  let closed = false;
  let retryTimer = null;
  /** @type {EventSource|null} */
  let source = null;

  function connect() {
    if (closed) return;
    source = new EventSource(`${BASE}/sessions/${sessionId}/events`);

    source.onopen = () => {
      retryDelay = SSE_RETRY_INIT_MS;
      retries = 0;
    };

    source.onmessage = (e) => {
      try {
        onMessage(JSON.parse(e.data));
      } catch (_) {
        /* ignore malformed SSE frames */
      }
    };

    source.addEventListener('done', () => {
      closed = true;
      source.close();
      if (onDone) onDone();
    });

    source.addEventListener('stats_ready', () => {
      if (onStatsReady) onStatsReady();
    });

    source.onerror = () => {
      source.close();
      if (closed) return;
      retries++;
      if (retries > SSE_MAX_RETRIES) {
        closed = true;
        if (onDone) onDone();
        return;
      }
      retryTimer = setTimeout(connect, retryDelay);
      retryDelay = Math.min(retryDelay * 2, SSE_RETRY_MAX_MS);
    };
  }

  connect();

  return {
    close() {
      closed = true;
      if (retryTimer) clearTimeout(retryTimer);
      if (source) source.close();
    },
  };
}

// --- URL Patterns ---

/**
 * @param {string} sessionId
 * @param {number} depth
 * @returns {Promise<Object[]>}
 */
export async function getURLPatterns(sessionId, depth = 2) {
  return fetchJSON(`/sessions/${sessionId}/url-patterns?depth=${depth}`);
}

/**
 * @param {string} sessionId
 * @param {number} limit
 * @returns {Promise<Object[]>}
 */
export async function getURLParams(sessionId, limit = 100) {
  return fetchJSON(`/sessions/${sessionId}/url-params?limit=${limit}`);
}

/**
 * @param {string} sessionId
 * @param {number} depth
 * @param {number} minPages
 * @returns {Promise<Object[]>}
 */
export async function getURLDirectories(sessionId, depth = 2, minPages = 1) {
  return fetchJSON(`/sessions/${sessionId}/url-directories?depth=${depth}&min_pages=${minPages}`);
}

/**
 * @param {string} sessionId
 * @returns {Promise<Object[]>}
 */
export async function getURLHosts(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/url-hosts`);
}
