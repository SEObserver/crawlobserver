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

export async function getPages(sessionId, limit = 100, offset = 0) {
  return fetchJSON(`/sessions/${sessionId}/pages?limit=${limit}&offset=${offset}`);
}

export async function getExternalLinks(sessionId, limit = 100, offset = 0) {
  return fetchJSON(`/sessions/${sessionId}/links?limit=${limit}&offset=${offset}`);
}

export async function getInternalLinks(sessionId, limit = 100, offset = 0, source = '', target = '') {
  let url = `/sessions/${sessionId}/internal-links?limit=${limit}&offset=${offset}`;
  if (source) url += `&source=${encodeURIComponent(source)}`;
  if (target) url += `&target=${encodeURIComponent(target)}`;
  return fetchJSON(url);
}

export async function getStats(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/stats`);
}

export async function getPageHTML(sessionId, url) {
  return fetchJSON(`/sessions/${sessionId}/page-html?url=${encodeURIComponent(url)}`);
}

export async function getStorageStats() {
  return fetchJSON('/storage-stats');
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

export async function resumeCrawl(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/resume`, { method: 'POST' });
}

export async function deleteSession(sessionId) {
  return fetchJSON(`/sessions/${sessionId}`, { method: 'DELETE' });
}

// SSE for live progress
export function subscribeProgress(sessionId, onMessage, onDone) {
  const source = new EventSource(`${BASE}/sessions/${sessionId}/events`);
  source.onmessage = (e) => {
    try {
      onMessage(JSON.parse(e.data));
    } catch {}
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
