const BASE = '/api';

async function fetchJSON(path, options = {}) {
  const res = await fetch(`${BASE}${path}`, options);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
  return res.json();
}

export async function getSessions() {
  return fetchJSON('/sessions');
}

export async function getPages(sessionId, limit = 100, offset = 0, filters = {}) {
  let url = `/sessions/${sessionId}/pages?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  return fetchJSON(url);
}

export async function getExternalLinks(sessionId, limit = 100, offset = 0, filters = {}) {
  let url = `/sessions/${sessionId}/links?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  return fetchJSON(url);
}

export async function getInternalLinks(sessionId, limit = 100, offset = 0, filters = {}) {
  let url = `/sessions/${sessionId}/internal-links?limit=${limit}&offset=${offset}`;
  for (const [k, v] of Object.entries(filters)) {
    if (v !== '' && v != null) url += `&${k}=${encodeURIComponent(v)}`;
  }
  return fetchJSON(url);
}

export async function getStats(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/stats`);
}

export async function getPageHTML(sessionId, url) {
  return fetchJSON(`/sessions/${sessionId}/page-html?url=${encodeURIComponent(url)}`);
}

export async function getPageDetail(sessionId, url, inLimit = 100, inOffset = 0) {
  return fetchJSON(`/sessions/${sessionId}/page-detail?url=${encodeURIComponent(url)}&in_limit=${inLimit}&in_offset=${inOffset}`);
}

export async function getStorageStats() {
  return fetchJSON('/storage-stats');
}

export async function getGlobalStats() {
  return fetchJSON('/global-stats');
}

export async function getSessionStorage() {
  return fetchJSON('/session-storage');
}

export async function getSystemStats() {
  return fetchJSON('/system-stats');
}

export async function getProgress(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/progress`);
}

export async function getHealth() {
  return fetchJSON('/health');
}

export async function getTheme() {
  return fetchJSON('/theme');
}

export async function updateTheme(theme) {
  return fetchJSON('/theme', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(theme),
  });
}

export async function startCrawl(seeds, options = {}) {
  return fetchJSON('/crawl', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ seeds, ...options }),
  });
}

export async function stopCrawl(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/stop`, { method: 'POST' });
}

export async function resumeCrawl(sessionId, options = null) {
  const opts = { method: 'POST' };
  if (options) {
    opts.headers = { 'Content-Type': 'application/json' };
    opts.body = JSON.stringify(options);
  }
  return fetchJSON(`/sessions/${sessionId}/resume`, opts);
}

export async function deleteSession(sessionId) {
  return fetchJSON(`/sessions/${sessionId}`, { method: 'DELETE' });
}

export async function recomputeDepths(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/recompute-depths`, { method: 'POST' });
}

export async function computePageRank(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/compute-pagerank`, { method: 'POST' });
}

export async function retryFailed(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/retry-failed`, { method: 'POST' });
}

export async function getPageRankDistribution(sessionId, buckets = 20) {
  return fetchJSON(`/sessions/${sessionId}/pagerank-distribution?buckets=${buckets}`);
}

export async function getPageRankTreemap(sessionId, depth = 2, minPages = 1) {
  return fetchJSON(`/sessions/${sessionId}/pagerank-treemap?depth=${depth}&min_pages=${minPages}`);
}

export async function getPageRankTop(sessionId, limit = 50, offset = 0, directory = '') {
  let url = `/sessions/${sessionId}/pagerank-top?limit=${limit}&offset=${offset}`;
  if (directory) url += `&directory=${encodeURIComponent(directory)}`;
  return fetchJSON(url);
}

export async function getRobotsHosts(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/robots`);
}

export async function getRobotsContent(sessionId, host) {
  return fetchJSON(`/sessions/${sessionId}/robots-content?host=${encodeURIComponent(host)}`);
}

export async function getSitemaps(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/sitemaps`);
}

export async function getSitemapURLs(sessionId, url, limit = 100, offset = 0) {
  return fetchJSON(`/sessions/${sessionId}/sitemap-urls?url=${encodeURIComponent(url)}&limit=${limit}&offset=${offset}`);
}

export async function testRobotsUrls(sessionId, host, userAgent, urls) {
  return fetchJSON(`/sessions/${sessionId}/robots-test`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ host, user_agent: userAgent, urls }),
  });
}

// --- Projects ---

export async function getProjects() {
  return fetchJSON('/projects');
}

export async function createProject(name) {
  return fetchJSON('/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
}

export async function renameProject(id, name) {
  return fetchJSON(`/projects/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
}

export async function deleteProject(id) {
  return fetchJSON(`/projects/${id}`, { method: 'DELETE' });
}

export async function associateSession(projectId, sessionId) {
  return fetchJSON(`/projects/${projectId}/sessions/${sessionId}`, { method: 'POST' });
}

export async function disassociateSession(projectId, sessionId) {
  return fetchJSON(`/projects/${projectId}/sessions/${sessionId}`, { method: 'DELETE' });
}

// --- API Keys ---

export async function getAPIKeys() {
  return fetchJSON('/api-keys');
}

export async function createAPIKey(name, type, projectId = null) {
  return fetchJSON('/api-keys', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, type, project_id: projectId }),
  });
}

export async function deleteAPIKey(id) {
  return fetchJSON(`/api-keys/${id}`, { method: 'DELETE' });
}

// --- Updates ---

export async function getUpdateStatus() {
  return fetchJSON('/update/status');
}

export async function applyUpdate() {
  return fetchJSON('/update/apply', { method: 'POST' });
}

// --- Backups ---

export async function getBackups() {
  return fetchJSON('/backups');
}

export async function createBackup() {
  return fetchJSON('/backups', { method: 'POST' });
}

export async function restoreBackup(filename) {
  return fetchJSON('/backups/restore', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filename }),
  });
}

export async function deleteBackup(name) {
  return fetchJSON(`/backups/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

// SSE for live progress
export function subscribeProgress(sessionId, onMessage, onDone) {
  const source = new EventSource(`${BASE}/sessions/${sessionId}/events`);
  source.onmessage = (e) => {
    try {
      onMessage(JSON.parse(e.data));
    } catch (err) { console.warn('SSE parse error:', err.message) }
  };
  source.addEventListener('done', () => {
    source.close();
    if (onDone) onDone();
  });
  source.onerror = () => {
    source.close();
    if (onDone) onDone();
  };
  return source;
}
