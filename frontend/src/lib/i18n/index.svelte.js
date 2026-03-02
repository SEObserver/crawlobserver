import en from './en.json';
import fr from './fr.json';

const translations = { en, fr };

let locale = $state(detectLocale());

function detectLocale() {
  try {
    const saved = localStorage.getItem('locale');
    if (saved && translations[saved]) return saved;
    const nav = navigator.language?.slice(0, 2);
    if (nav && translations[nav]) return nav;
  } catch {}
  return 'en';
}

export function t(key, params) {
  const val = translations[locale]?.[key] ?? translations.en?.[key] ?? key;
  if (!params) return val;
  return val.replace(/\{(\w+)\}/g, (_, k) => params[k] ?? `{${k}}`);
}

export function setLocale(lang) {
  if (!translations[lang]) return;
  locale = lang;
  localStorage.setItem('locale', lang);
  document.documentElement.lang = lang;
}

export function getLocale() {
  return locale;
}
