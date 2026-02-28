import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { parseRoute, pushURL } from './router.js';

// Helper to set window.location for parseRoute tests
function setLocation(path, search = '') {
  Object.defineProperty(window, 'location', {
    value: { pathname: path, search, href: `http://localhost${path}${search}` },
    writable: true,
    configurable: true,
  });
}

describe('parseRoute', () => {
  it('returns home for /', () => {
    setLocation('/');
    expect(parseRoute()).toEqual({ page: 'home' });
  });

  it('parses /new-crawl', () => {
    setLocation('/new-crawl');
    expect(parseRoute()).toEqual({ page: 'new-crawl' });
  });

  it('parses /settings', () => {
    setLocation('/settings');
    expect(parseRoute()).toEqual({ page: 'settings' });
  });

  it('parses /stats', () => {
    setLocation('/stats');
    expect(parseRoute()).toEqual({ page: 'stats' });
  });

  it('parses /api', () => {
    setLocation('/api');
    expect(parseRoute()).toEqual({ page: 'api' });
  });

  it('parses /logs', () => {
    setLocation('/logs');
    expect(parseRoute()).toEqual({ page: 'logs' });
  });

  it('parses /compare with query params', () => {
    setLocation('/compare', '?a=sess1&b=sess2');
    expect(parseRoute()).toEqual({ page: 'compare', sessionA: 'sess1', sessionB: 'sess2' });
  });

  it('parses /compare with empty params', () => {
    setLocation('/compare', '');
    expect(parseRoute()).toEqual({ page: 'compare', sessionA: '', sessionB: '' });
  });

  it('parses /projects', () => {
    setLocation('/projects');
    expect(parseRoute()).toEqual({ page: 'all-projects' });
  });

  it('parses /projects/:id', () => {
    setLocation('/projects/5');
    expect(parseRoute()).toEqual({ page: 'project', projectId: '5', projectTab: 'sessions', projectSubView: null });
  });

  it('parses /projects/:id/:tab', () => {
    setLocation('/projects/5/gsc');
    expect(parseRoute()).toEqual({ page: 'project', projectId: '5', projectTab: 'gsc', projectSubView: null });
  });

  it('parses /projects/:id/:tab/:sub', () => {
    setLocation('/projects/5/gsc/queries');
    expect(parseRoute()).toEqual({ page: 'project', projectId: '5', projectTab: 'gsc', projectSubView: 'queries' });
  });

  it('parses /sessions/:id (overview)', () => {
    setLocation('/sessions/abc123');
    expect(parseRoute()).toEqual({ sessionId: 'abc123', tab: 'overview', subView: null, filters: {}, offset: 0 });
  });

  it('parses /sessions/:id/pages', () => {
    setLocation('/sessions/abc123/pages');
    expect(parseRoute()).toEqual({ sessionId: 'abc123', tab: 'pages', subView: null, filters: {}, offset: 0 });
  });

  it('parses /sessions/:id/pages with query params', () => {
    setLocation('/sessions/abc123/pages', '?status_code=404&offset=50');
    expect(parseRoute()).toEqual({ sessionId: 'abc123', tab: 'pages', subView: null, filters: { status_code: '404' }, offset: 50 });
  });

  it('parses /sessions/:id/url/... with URL decoding', () => {
    setLocation('/sessions/abc123/url/https%3A%2F%2Fexample.com%2Fpath');
    expect(parseRoute()).toEqual({
      sessionId: 'abc123',
      tab: 'url-detail',
      detailUrl: 'https://example.com/path',
      filters: {},
      offset: 0,
    });
  });

  it('ignores limit param in session routes', () => {
    setLocation('/sessions/abc123/pages', '?limit=50&offset=10&q=test');
    const route = parseRoute();
    expect(route.filters).toEqual({ q: 'test' });
    expect(route.offset).toBe(10);
  });
});

describe('pushURL', () => {
  let pushStateSpy;

  beforeEach(() => {
    setLocation('/');
    pushStateSpy = vi.spyOn(history, 'pushState').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('pushes simple path', () => {
    pushURL('/new-crawl');
    expect(pushStateSpy).toHaveBeenCalledWith(null, '', '/new-crawl');
  });

  it('appends non-empty filters as query params', () => {
    pushURL('/sessions/abc/pages', { status_code: '404', q: 'test' });
    const url = pushStateSpy.mock.calls[0][2];
    expect(url).toContain('status_code=404');
    expect(url).toContain('q=test');
  });

  it('ignores empty string and null filter values', () => {
    pushURL('/sessions/abc/pages', { status_code: '', q: null, real: 'yes' });
    const url = pushStateSpy.mock.calls[0][2];
    expect(url).not.toContain('status_code');
    expect(url).not.toContain('q=');
    expect(url).toContain('real=yes');
  });

  it('includes offset when > 0', () => {
    pushURL('/sessions/abc/pages', {}, 50);
    const url = pushStateSpy.mock.calls[0][2];
    expect(url).toContain('offset=50');
  });

  it('omits offset when 0', () => {
    pushURL('/sessions/abc/pages', {}, 0);
    const url = pushStateSpy.mock.calls[0][2];
    expect(url).not.toContain('offset');
  });

  it('is idempotent — does not pushState if same URL', () => {
    setLocation('/sessions/abc/pages', '?status_code=404');
    pushStateSpy.mockClear();
    pushURL('/sessions/abc/pages', { status_code: '404' });
    expect(pushStateSpy).not.toHaveBeenCalled();
  });
});
