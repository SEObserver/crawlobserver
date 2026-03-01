import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { statusBadge, fmt, fmtSize, fmtN, trunc, timeAgo, a11yKeydown, squarify } from './utils.js';

describe('statusBadge', () => {
  it('returns badge-success for 2xx', () => {
    expect(statusBadge(200)).toBe('badge-success');
    expect(statusBadge(204)).toBe('badge-success');
    expect(statusBadge(299)).toBe('badge-success');
  });

  it('returns badge-info for 3xx', () => {
    expect(statusBadge(301)).toBe('badge-info');
    expect(statusBadge(304)).toBe('badge-info');
  });

  it('returns badge-warning for 4xx', () => {
    expect(statusBadge(400)).toBe('badge-warning');
    expect(statusBadge(404)).toBe('badge-warning');
    expect(statusBadge(499)).toBe('badge-warning');
  });

  it('returns badge-error for 5xx', () => {
    expect(statusBadge(500)).toBe('badge-error');
    expect(statusBadge(503)).toBe('badge-error');
  });

  it('returns badge-error for 0 (network error)', () => {
    expect(statusBadge(0)).toBe('badge-error');
  });
});

describe('fmt', () => {
  it('formats milliseconds below 1s', () => {
    expect(fmt(0)).toBe('0ms');
    expect(fmt(500)).toBe('500ms');
    expect(fmt(999)).toBe('999ms');
  });

  it('formats seconds for >= 1000ms', () => {
    expect(fmt(1000)).toBe('1.0s');
    expect(fmt(1500)).toBe('1.5s');
    expect(fmt(12345)).toBe('12.3s');
  });
});

describe('fmtSize', () => {
  it('formats bytes', () => {
    expect(fmtSize(0)).toBe('0B');
    expect(fmtSize(512)).toBe('512B');
    expect(fmtSize(1023)).toBe('1023B');
  });

  it('formats kilobytes', () => {
    expect(fmtSize(1024)).toBe('1.0KB');
    expect(fmtSize(1536)).toBe('1.5KB');
  });

  it('formats megabytes', () => {
    expect(fmtSize(1048576)).toBe('1.0MB');
    expect(fmtSize(5 * 1048576)).toBe('5.0MB');
  });

  it('formats gigabytes', () => {
    expect(fmtSize(1073741824)).toBe('1.00GB');
    expect(fmtSize(2.5 * 1073741824)).toBe('2.50GB');
  });
});

describe('fmtN', () => {
  it('formats numbers with locale separators', () => {
    expect(fmtN(0)).toBe('0');
    // fmtN uses Intl.NumberFormat, just verify it returns a string
    expect(typeof fmtN(1000)).toBe('string');
    expect(fmtN(1000).replace(/\D/g, '')).toBe('1000');
  });

  it('handles null/undefined', () => {
    // Intl.NumberFormat.format(null) → "0", format(undefined) → "NaN"
    expect(fmtN(null)).toBe('0');
  });
});

describe('trunc', () => {
  it('returns short strings unchanged', () => {
    expect(trunc('hello', 10)).toBe('hello');
  });

  it('truncates long strings with ellipsis', () => {
    expect(trunc('hello world', 5)).toBe('hello...');
  });

  it('returns dash for null/undefined/empty', () => {
    expect(trunc(null, 10)).toBe('-');
    expect(trunc(undefined, 10)).toBe('-');
    expect(trunc('', 10)).toBe('-');
  });
});

describe('timeAgo', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "just now" for < 60s ago', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);
    expect(timeAgo('2024-01-15T11:59:30Z')).toBe('just now');
  });

  it('returns minutes ago', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);
    expect(timeAgo('2024-01-15T11:55:00Z')).toBe('5m ago');
  });

  it('returns hours ago', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);
    expect(timeAgo('2024-01-15T09:00:00Z')).toBe('3h ago');
  });

  it('returns localized date for > 24h', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);
    const result = timeAgo('2024-01-10T12:00:00Z');
    // Should be a date string, not "Xh ago"
    expect(result).not.toContain('ago');
    expect(result).not.toBe('just now');
  });
});

describe('a11yKeydown', () => {
  it('calls callback on Enter', () => {
    const cb = vi.fn();
    const handler = a11yKeydown(cb);
    const event = { key: 'Enter', preventDefault: vi.fn() };
    handler(event);
    expect(cb).toHaveBeenCalledOnce();
    expect(event.preventDefault).toHaveBeenCalled();
  });

  it('calls callback on Space', () => {
    const cb = vi.fn();
    const handler = a11yKeydown(cb);
    const event = { key: ' ', preventDefault: vi.fn() };
    handler(event);
    expect(cb).toHaveBeenCalledOnce();
  });

  it('does not call callback on Tab', () => {
    const cb = vi.fn();
    const handler = a11yKeydown(cb);
    handler({ key: 'Tab', preventDefault: vi.fn() });
    expect(cb).not.toHaveBeenCalled();
  });

  it('does not call callback on other keys', () => {
    const cb = vi.fn();
    const handler = a11yKeydown(cb);
    handler({ key: 'Escape', preventDefault: vi.fn() });
    handler({ key: 'a', preventDefault: vi.fn() });
    expect(cb).not.toHaveBeenCalled();
  });
});

describe('squarify', () => {
  it('returns empty array for empty items', () => {
    expect(squarify([], 0, 0, 100, 100)).toEqual([]);
  });

  it('returns empty array for zero dimensions', () => {
    expect(squarify([{ value: 10 }], 0, 0, 0, 100)).toEqual([]);
    expect(squarify([{ value: 10 }], 0, 0, 100, 0)).toEqual([]);
  });

  it('single item covers the full area', () => {
    const rects = squarify([{ value: 100, label: 'a' }], 0, 0, 200, 100);
    expect(rects).toHaveLength(1);
    expect(rects[0].x).toBe(0);
    expect(rects[0].y).toBe(0);
    expect(rects[0].w).toBeCloseTo(200);
    expect(rects[0].h).toBeCloseTo(100);
    expect(rects[0].label).toBe('a');
  });

  it('multiple items: sum of areas equals total area', () => {
    const items = [
      { value: 60, label: 'a' },
      { value: 30, label: 'b' },
      { value: 10, label: 'c' },
    ];
    const rects = squarify(items, 0, 0, 400, 300);
    expect(rects).toHaveLength(3);
    const totalArea = 400 * 300;
    const sumAreas = rects.reduce((s, r) => s + r.w * r.h, 0);
    expect(sumAreas).toBeCloseTo(totalArea, 0);
  });

  it('all rects have reasonable aspect ratios', () => {
    const items = [{ value: 50 }, { value: 30 }, { value: 15 }, { value: 5 }];
    const rects = squarify(items, 0, 0, 200, 200);
    for (const r of rects) {
      expect(r.w).toBeGreaterThan(0);
      expect(r.h).toBeGreaterThan(0);
      const ratio = Math.max(r.w / r.h, r.h / r.w);
      expect(ratio).toBeLessThan(20);
    }
  });
});
