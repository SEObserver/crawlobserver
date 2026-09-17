import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, tick, unmount } from 'svelte';
import { checkIP, getExtractorSets, resumeCrawl, retryFailed, startCrawl } from '../api.js';
import CrawlForm from './CrawlForm.svelte';

vi.mock('../api.js', () => ({
  checkIP: vi.fn(),
  getExtractorSets: vi.fn(),
  resumeCrawl: vi.fn(),
  retryFailed: vi.fn(),
  startCrawl: vi.fn(),
}));

describe('CrawlForm', () => {
  let component;
  let target;

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
    vi.clearAllMocks();
    getExtractorSets.mockResolvedValue([]);
    startCrawl.mockResolvedValue({});
    resumeCrawl.mockResolvedValue({});
    retryFailed.mockResolvedValue({});
    checkIP.mockResolvedValue({ ip: '203.0.113.10' });
  });

  afterEach(async () => {
    if (component) await unmount(component);
    target.remove();
    component = null;
  });

  function render(props = {}) {
    component = mount(CrawlForm, {
      target,
      props: {
        mode: 'new',
        projects: [],
        ...props,
      },
    });
  }

  async function setInput(selector, value) {
    const input = target.querySelector(selector);
    input.value = value;
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await tick();
    return input;
  }

  async function setSelect(selector, value) {
    const select = target.querySelector(selector);
    select.value = value;
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await tick();
    return select;
  }

  function submitButton() {
    return target.querySelector('.form-actions .btn-primary');
  }

  it('normalizes seed URLs and starts a crawl with the form defaults', async () => {
    const onsubmit = vi.fn();
    render({ onsubmit });
    expect(submitButton().disabled).toBe(true);

    await setInput('#cf-seeds', 'example.com\nhttps://www.example.org/path');
    expect(submitButton().disabled).toBe(false);
    expect(target.querySelector('#cf-link-position option[value="default"]').textContent).toContain(
      'Use server default',
    );
    submitButton().click();

    await vi.waitFor(() => expect(startCrawl).toHaveBeenCalledOnce());
    expect(startCrawl).toHaveBeenCalledWith(
      ['http://example.com', 'https://www.example.org/path'],
      expect.objectContaining({
        workers: 10,
        delay: '1000ms',
        max_pages: 0,
        max_depth: 0,
        crawl_scope: 'host',
        fetch_sitemaps: true,
      }),
    );
    const options = startCrawl.mock.calls[0][1];
    expect(options).not.toHaveProperty('store_link_position');
    expect(options).not.toHaveProperty('max_link_positions_per_page');
    await vi.waitFor(() => expect(onsubmit).toHaveBeenCalledOnce());
  });

  it('sends an explicit false when link position recording is disabled', async () => {
    render();
    await setInput('#cf-seeds', 'example.com');
    await setSelect('#cf-link-position', 'disabled');

    submitButton().click();
    await vi.waitFor(() => expect(startCrawl).toHaveBeenCalledOnce());

    const options = startCrawl.mock.calls[0][1];
    expect(options.store_link_position).toBe(false);
    expect(options).not.toHaveProperty('max_link_positions_per_page');
  });

  it('requires a positive integer cap and sends a valid cap', async () => {
    render();
    await setInput('#cf-seeds', 'example.com');
    const limit = await setInput('#cf-link-position-limit', '0');

    expect(limit.getAttribute('aria-invalid')).toBe('true');
    expect(target.querySelector('#cf-link-position-error').textContent).toContain('positive');
    expect(submitButton().disabled).toBe(true);
    expect(startCrawl).not.toHaveBeenCalled();

    await setInput('#cf-link-position-limit', '250');
    expect(submitButton().disabled).toBe(false);
    submitButton().click();
    await vi.waitFor(() => expect(startCrawl).toHaveBeenCalledOnce());

    expect(startCrawl.mock.calls[0][1].max_link_positions_per_page).toBe(250);
  });

  it('restores persisted crawler settings when resuming a session', async () => {
    const session = {
      ID: 'session-1',
      SeedURLs: ['https://example.com'],
      ProjectID: 'project-1',
      Config: JSON.stringify({
        Crawler: {
          Workers: 7,
          Delay: 250_000_000,
          MaxPages: 500,
          MaxDepth: 4,
          CrawlScope: 'domain',
          StoreHTML: true,
          StoreLinkPosition: false,
          MaxLinkPositionsPerPage: 250,
        },
      }),
    };
    render({ mode: 'resume', session });

    expect(target.querySelector('#cf-workers').value).toBe('7');
    expect(target.querySelector('#cf-delay').value).toBe('250');
    expect(target.querySelector('#cf-seeds').disabled).toBe(true);
    expect(target.querySelector('#cf-link-position').value).toBe('disabled');
    expect(target.querySelector('#cf-link-position-limit').value).toBe('250');

    submitButton().click();
    await vi.waitFor(() => expect(resumeCrawl).toHaveBeenCalledOnce());
    expect(resumeCrawl).toHaveBeenCalledWith(
      'session-1',
      expect.objectContaining({
        workers: 7,
        delay: '250ms',
        max_pages: 500,
        max_depth: 4,
        crawl_scope: 'domain',
        project_id: 'project-1',
        store_html: true,
        store_link_position: false,
        max_link_positions_per_page: 250,
      }),
    );
  });

  it('leaves link position options omitted for an older session snapshot', async () => {
    const session = {
      ID: 'session-old',
      SeedURLs: ['https://example.com'],
      Config: JSON.stringify({
        Crawler: {
          Workers: 7,
          StoreHTML: true,
        },
      }),
    };
    render({ mode: 'resume', session });

    expect(target.querySelector('#cf-link-position').value).toBe('default');
    expect(target.querySelector('#cf-link-position-limit').value).toBe('');
    expect(target.querySelector('#cf-link-position option[value="default"]').textContent).toContain(
      'Keep session setting',
    );
    expect(target.querySelector('#cf-link-position-limit').placeholder).toBe(
      'Keep session setting',
    );
    submitButton().click();
    await vi.waitFor(() => expect(resumeCrawl).toHaveBeenCalledOnce());

    const options = resumeCrawl.mock.calls[0][1];
    expect(options).not.toHaveProperty('store_link_position');
    expect(options).not.toHaveProperty('max_link_positions_per_page');
  });

  it('uses the same explicit settings when retrying failed pages', async () => {
    const session = {
      ID: 'session-retry',
      SeedURLs: ['https://example.com'],
      Config: JSON.stringify({ Crawler: {} }),
    };
    render({ mode: 'retry', session, retryStatusCode: 500, retryCount: 2 });
    await setSelect('#cf-link-position', 'enabled');
    await setInput('#cf-link-position-limit', '100');

    submitButton().click();
    await vi.waitFor(() => expect(retryFailed).toHaveBeenCalledOnce());
    expect(retryFailed.mock.calls[0][2]).toEqual(
      expect.objectContaining({
        store_link_position: true,
        max_link_positions_per_page: 100,
      }),
    );
  });

  it('reports API failures without completing the form', async () => {
    const onerror = vi.fn();
    const onsubmit = vi.fn();
    startCrawl.mockRejectedValue(new Error('crawl failed'));
    render({ onerror, onsubmit });
    await setInput('#cf-seeds', 'example.com');

    submitButton().click();
    await vi.waitFor(() => expect(onerror).toHaveBeenCalledWith('crawl failed'));
    expect(onsubmit).not.toHaveBeenCalled();
    expect(submitButton().disabled).toBe(false);
  });
});
