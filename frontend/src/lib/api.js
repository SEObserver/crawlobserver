const BASE = '/api';

async function fetchJSON(path) {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json();
}

export async function getSessions() {
  return fetchJSON('/sessions');
}

export async function getPages(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/pages`);
}

export async function getLinks(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/links`);
}

export async function getStats(sessionId) {
  return fetchJSON(`/sessions/${sessionId}/stats`);
}

export async function getHealth() {
  return fetchJSON('/health');
}
