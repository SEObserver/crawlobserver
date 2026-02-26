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

export async function getLinks(sessionId, limit = 100, offset = 0) {
  return fetchJSON(`/sessions/${sessionId}/links?limit=${limit}&offset=${offset}`);
}

export async function getStats(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/stats`);
}

export async function getProgress(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/progress`);
}

export async function getHealth() {
  return fetchJSON('/health');
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
