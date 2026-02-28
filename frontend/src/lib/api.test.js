import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { getSessions, exportSession, subscribeProgress } from './api.js';

// --- fetchJSON (tested via getSessions) ---

describe('fetchJSON', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns parsed JSON on success', async () => {
    globalThis.fetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([{ ID: '1' }]),
    });
    const result = await getSessions();
    expect(result).toEqual([{ ID: '1' }]);
    expect(globalThis.fetch).toHaveBeenCalledWith('/api/sessions', {});
  });

  it('throws with error message from JSON body on 404', async () => {
    globalThis.fetch.mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: () => Promise.resolve({ error: 'Session not found' }),
      text: () => Promise.resolve(''),
    });
    await expect(getSessions()).rejects.toThrow('Session not found');
  });

  it('throws with statusText when body is not JSON on 500', async () => {
    globalThis.fetch.mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: () => Promise.reject(new Error('parse error')),
      text: () => Promise.resolve('Internal Server Error'),
    });
    await expect(getSessions()).rejects.toThrow('Internal Server Error');
  });

  it('throws on network failure', async () => {
    globalThis.fetch.mockRejectedValue(new TypeError('Failed to fetch'));
    await expect(getSessions()).rejects.toThrow('Failed to fetch');
  });
});

// --- exportSession ---

describe('exportSession', () => {
  let openSpy;

  beforeEach(() => {
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('opens export URL without html param', () => {
    exportSession('sess1', false);
    expect(openSpy).toHaveBeenCalledWith(
      '/api/sessions/sess1/export?include_html=false',
      '_blank'
    );
  });

  it('opens export URL with html param', () => {
    exportSession('sess1', true);
    expect(openSpy).toHaveBeenCalledWith(
      '/api/sessions/sess1/export?include_html=true',
      '_blank'
    );
  });
});

// --- subscribeProgress ---

describe('subscribeProgress', () => {
  let MockEventSource;

  beforeEach(() => {
    vi.useFakeTimers();
    MockEventSource = vi.fn().mockImplementation(function (url) {
      this.url = url;
      this.onopen = null;
      this.onmessage = null;
      this.onerror = null;
      this.close = vi.fn();
      this._listeners = {};
      this.addEventListener = vi.fn((event, handler) => {
        this._listeners[event] = handler;
      });
      // Store for test access
      MockEventSource._lastInstance = this;
    });
    globalThis.EventSource = MockEventSource;
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('calls onMessage with parsed data on message event', () => {
    const onMessage = vi.fn();
    const onDone = vi.fn();
    subscribeProgress('sess1', onMessage, onDone);

    const es = MockEventSource._lastInstance;
    es.onmessage({ data: '{"pages_crawled":42}' });

    expect(onMessage).toHaveBeenCalledWith({ pages_crawled: 42 });
  });

  it('closes and calls onDone on done event', () => {
    const onMessage = vi.fn();
    const onDone = vi.fn();
    subscribeProgress('sess1', onMessage, onDone);

    const es = MockEventSource._lastInstance;
    // Trigger done event
    const doneHandler = es.addEventListener.mock.calls.find(c => c[0] === 'done')[1];
    doneHandler();

    expect(es.close).toHaveBeenCalled();
    expect(onDone).toHaveBeenCalledOnce();
  });

  it('retries with exponential backoff on error', () => {
    const onMessage = vi.fn();
    const onDone = vi.fn();
    subscribeProgress('sess1', onMessage, onDone);

    const es1 = MockEventSource._lastInstance;
    // Simulate error
    es1.onerror();
    expect(es1.close).toHaveBeenCalled();

    // Should not reconnect yet
    expect(MockEventSource).toHaveBeenCalledTimes(1);

    // Advance 1s — first retry delay
    vi.advanceTimersByTime(1000);
    expect(MockEventSource).toHaveBeenCalledTimes(2);

    // Second error
    const es2 = MockEventSource._lastInstance;
    es2.onerror();

    // Advance 2s — second retry delay
    vi.advanceTimersByTime(2000);
    expect(MockEventSource).toHaveBeenCalledTimes(3);
  });

  it('gives up after max retries and calls onDone', () => {
    const onMessage = vi.fn();
    const onDone = vi.fn();
    subscribeProgress('sess1', onMessage, onDone);

    // Trigger 11 errors (initial + 10 retries)
    for (let i = 0; i < 11; i++) {
      const es = MockEventSource._lastInstance;
      es.onerror();
      // Advance past any retry delay
      vi.advanceTimersByTime(60000);
    }

    expect(onDone).toHaveBeenCalledOnce();
  });

  it('close() cancels retry timer', () => {
    const onMessage = vi.fn();
    const onDone = vi.fn();
    const handle = subscribeProgress('sess1', onMessage, onDone);

    const es = MockEventSource._lastInstance;
    es.onerror(); // triggers a retry timer

    handle.close();

    // Advance past retry delay — should NOT reconnect
    vi.advanceTimersByTime(10000);
    // Only the initial connection
    expect(MockEventSource).toHaveBeenCalledTimes(1);
  });

  it('resets retry count on successful open', () => {
    const onMessage = vi.fn();
    const onDone = vi.fn();
    subscribeProgress('sess1', onMessage, onDone);

    // Error once
    const es1 = MockEventSource._lastInstance;
    es1.onerror();
    vi.advanceTimersByTime(1000);

    // Reconnect succeeds
    const es2 = MockEventSource._lastInstance;
    es2.onopen();

    // Error again — should start from 1s retry, not 2s
    es2.onerror();
    vi.advanceTimersByTime(1000);
    expect(MockEventSource).toHaveBeenCalledTimes(3);
  });
});
