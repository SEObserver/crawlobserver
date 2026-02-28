import { describe, it, expect, vi, beforeEach } from 'vitest';
import { applyTheme, saveDarkMode } from './theme.js';

describe('applyTheme', () => {
  beforeEach(() => {
    document.documentElement.removeAttribute('data-theme');
    document.documentElement.style.cssText = '';
  });

  it('sets data-theme to light when darkMode is false', () => {
    applyTheme({ accent_color: '#3b82f6' }, false);
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
  });

  it('sets data-theme to dark when darkMode is true', () => {
    applyTheme({ accent_color: '#3b82f6' }, true);
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
  });

  it('sets --accent CSS variable from hex color', () => {
    applyTheme({ accent_color: '#ff6600' }, false);
    expect(document.documentElement.style.getPropertyValue('--accent')).toBe('#ff6600');
  });

  it('sets --accent-light with correct alpha for light mode', () => {
    applyTheme({ accent_color: '#ff6600' }, false);
    // ff=255, 66=102, 00=0, alpha=0.08
    expect(document.documentElement.style.getPropertyValue('--accent-light')).toBe('rgba(255,102,0,0.08)');
  });

  it('sets --accent-light with higher alpha for dark mode', () => {
    applyTheme({ accent_color: '#ff6600' }, true);
    // alpha=0.15
    expect(document.documentElement.style.getPropertyValue('--accent-light')).toBe('rgba(255,102,0,0.15)');
  });

  it('handles theme without accent_color', () => {
    applyTheme({}, false);
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    // No CSS variable set
    expect(document.documentElement.style.getPropertyValue('--accent')).toBe('');
  });
});

describe('saveDarkMode', () => {
  let store;

  beforeEach(() => {
    store = {};
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key) => store[key] ?? null),
        setItem: vi.fn((key, val) => { store[key] = String(val); }),
        removeItem: vi.fn((key) => { delete store[key]; }),
      },
      writable: true,
      configurable: true,
    });
  });

  it('saves true to localStorage', () => {
    saveDarkMode(true);
    expect(localStorage.setItem).toHaveBeenCalledWith('darkMode', 'true');
  });

  it('saves false to localStorage', () => {
    saveDarkMode(false);
    expect(localStorage.setItem).toHaveBeenCalledWith('darkMode', 'false');
  });
});
