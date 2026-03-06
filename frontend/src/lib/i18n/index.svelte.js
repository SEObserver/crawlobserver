import en from './en.json';
import fr from './fr.json';
import es from './es.json';
import pt from './pt.json';
import nl from './nl.json';
import it from './it.json';
import de from './de.json';
import ru from './ru.json';
import zh from './zh.json';
import he from './he.json';
import ar from './ar.json';
import ja from './ja.json';
import tr from './tr.json';
import id from './id.json';
import ko from './ko.json';
import pl from './pl.json';

const translations = { en, fr, es, pt, nl, it, de, ru, zh, ja, tr, id, ko, pl, he, ar };

const rtlLocales = new Set(['he', 'ar']);

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

function applyDir(lang) {
  document.documentElement.dir = rtlLocales.has(lang) ? 'rtl' : 'ltr';
}

// Apply direction on initial load
applyDir(locale);

export function t(key, params) {
  let val = translations[locale]?.[key] ?? translations.en?.[key] ?? key;
  if (!params) return val;
  // Plural support: "singular|plural" picked by params.count
  if (params.count !== undefined && val.includes('|')) {
    const forms = val.split('|');
    val = (params.count === 1 ? forms[0] : forms[1]) ?? forms[0];
  }
  return val.replace(/\{(\w+)\}/g, (_, k) => params[k] ?? `{${k}}`);
}

export function setLocale(lang) {
  if (!translations[lang]) return;
  locale = lang;
  localStorage.setItem('locale', lang);
  document.documentElement.lang = lang;
  applyDir(lang);
}

export function getLocale() {
  return locale;
}
