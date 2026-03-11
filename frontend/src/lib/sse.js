/**
 * SSE connection manager — tracks live crawl progress streams.
 */
import { subscribeProgress } from './api.js';

export function createSSEManager() {
  const connections = {};

  /**
   * Open an SSE connection for a running session.
   * @param {string} sessionId
   * @param {(data: object) => void} onProgress — called on each progress event
   * @param {(sessionId: string) => void} onComplete — called when stream ends
   * @param {() => void} [onStatsReady] — called when server signals new stats available
   */
  function connect(sessionId, onProgress, onComplete, onStatsReady) {
    if (connections[sessionId]) return;
    connections[sessionId] = subscribeProgress(sessionId, onProgress, () => {
      delete connections[sessionId];
      onComplete(sessionId);
    }, onStatsReady);
  }

  /** Close a single SSE connection. */
  function disconnect(sessionId) {
    if (connections[sessionId]) {
      connections[sessionId].close();
      delete connections[sessionId];
    }
  }

  /** Close all SSE connections (cleanup). */
  function disconnectAll() {
    for (const id of Object.keys(connections)) {
      connections[id].close();
      delete connections[id];
    }
  }

  /** Check if a session has an active SSE connection. */
  function isConnected(sessionId) {
    return !!connections[sessionId];
  }

  return { connect, disconnect, disconnectAll, isConnected };
}
