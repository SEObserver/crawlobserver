/**
 * Theme management — DOM manipulation for theme/dark mode.
 */
import { getTheme } from './api.js';

/** Apply theme + dark mode to the document root. */
export function applyTheme(theme, darkMode) {
  document.documentElement.setAttribute('data-theme', darkMode ? 'dark' : 'light');
  if (theme.accent_color) {
    document.documentElement.style.setProperty('--accent', theme.accent_color);
    const hex = theme.accent_color.replace('#', '');
    const r = parseInt(hex.substring(0, 2), 16);
    const g = parseInt(hex.substring(2, 4), 16);
    const b = parseInt(hex.substring(4, 6), 16);
    const alpha = darkMode ? 0.15 : 0.08;
    document.documentElement.style.setProperty('--accent-light', `rgba(${r},${g},${b},${alpha})`);
  }
}

/** Load theme from API + dark mode preference from localStorage. */
export async function loadThemeFromServer() {
  const t = await getTheme();
  const saved = localStorage.getItem('darkMode');
  const dark = saved !== null ? saved === 'true' : t.mode === 'dark';
  return { theme: t, darkMode: dark };
}

/** Persist dark mode preference to localStorage. */
export function saveDarkMode(darkMode) {
  localStorage.setItem('darkMode', darkMode ? 'true' : 'false');
}
