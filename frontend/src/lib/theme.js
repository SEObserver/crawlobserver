/**
 * Theme management — DOM manipulation for theme/dark mode.
 */
import { getTheme } from './api.js';

/** Resolve whether dark mode is active, considering 'auto' mode. */
function resolveDark(darkMode) {
  if (darkMode === 'auto') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  }
  return darkMode === true || darkMode === 'true';
}

/** Apply theme + dark mode to the document root. */
export function applyTheme(theme, darkMode) {
  const isDark = resolveDark(darkMode);
  document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light');
  if (theme.accent_color) {
    document.documentElement.style.setProperty('--accent', theme.accent_color);
    const hex = theme.accent_color.replace('#', '');
    const r = parseInt(hex.substring(0, 2), 16);
    const g = parseInt(hex.substring(2, 4), 16);
    const b = parseInt(hex.substring(4, 6), 16);
    const alpha = isDark ? 0.15 : 0.08;
    document.documentElement.style.setProperty('--accent-light', `rgba(${r},${g},${b},${alpha})`);
  }
}

/** Load theme from API + dark mode preference from localStorage. */
export async function loadThemeFromServer() {
  const t = await getTheme();
  const saved = localStorage.getItem('darkMode');
  let dark;
  if (saved === 'auto') {
    dark = 'auto';
  } else if (saved !== null) {
    dark = saved === 'true';
  } else {
    dark = t.mode === 'dark' ? true : t.mode === 'auto' ? 'auto' : false;
  }
  return { theme: t, darkMode: dark };
}

/** Persist dark mode preference to localStorage. */
export function saveDarkMode(darkMode) {
  if (darkMode === 'auto') {
    localStorage.setItem('darkMode', 'auto');
  } else {
    localStorage.setItem('darkMode', darkMode ? 'true' : 'false');
  }
}

/** Listen for OS color scheme changes and re-apply theme when in auto mode. */
export function listenColorScheme(getThemeState) {
  const mql = window.matchMedia('(prefers-color-scheme: dark)');
  mql.addEventListener('change', () => {
    const { theme, darkMode } = getThemeState();
    if (darkMode === 'auto') {
      applyTheme(theme, 'auto');
    }
  });
}
