import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { translations } from './translations.js';

export const SUPPORTED_LANGUAGES = [
  { code: 'en', label: 'English', flag: '🇺🇸', shortLabel: 'EN' },
  { code: 'th', label: 'ไทย', flag: '🇹🇭', shortLabel: 'TH' },
  { code: 'ja', label: '日本語', flag: '🇯🇵', shortLabel: 'JA' },
  { code: 'zh', label: '简体中文', flag: '🇨🇳', shortLabel: 'ZH' },
];

const I18nContext = createContext(null);

export function I18nProvider({ children }) {
  const [lang, setLangState] = useState(() => {
    try {
      const saved = typeof window !== 'undefined' ? localStorage.getItem('agy_lang') : null;
      if (saved && translations[saved]) return saved;
      if (typeof navigator !== 'undefined' && navigator.language) {
        const browserLang = navigator.language.toLowerCase();
        if (browserLang.startsWith('th')) return 'th';
        if (browserLang.startsWith('ja')) return 'ja';
        if (browserLang.startsWith('zh')) return 'zh';
      }
    } catch {
      // Ignore localStorage errors in private mode
    }
    return 'en';
  });

  const setLang = useCallback((newLang) => {
    if (translations[newLang]) {
      setLangState(newLang);
      try {
        localStorage.setItem('agy_lang', newLang);
      } catch {
        // Ignore
      }
    }
  }, []);

  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = lang;
    }
  }, [lang]);

  const t = useCallback((path, fallback = '') => {
    const keys = path.split('.');
    let current = translations[lang];
    for (const key of keys) {
      if (!current || typeof current !== 'object') {
        current = undefined;
        break;
      }
      current = current[key];
    }
    if (current !== undefined) return current;

    // Fallback to English if key is missing in chosen language
    let enCurrent = translations.en;
    for (const key of keys) {
      if (!enCurrent || typeof enCurrent !== 'object') {
        enCurrent = undefined;
        break;
      }
      enCurrent = enCurrent[key];
    }
    return enCurrent !== undefined ? enCurrent : (fallback || path);
  }, [lang]);

  return (
    <I18nContext.Provider value={{ lang, setLang, t, languages: SUPPORTED_LANGUAGES }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useTranslation() {
  const context = useContext(I18nContext);
  if (!context) {
    // Graceful fallback if rendered outside provider (e.g. in standalone test)
    return {
      lang: 'en',
      setLang: () => {},
      t: (path, fallback = '') => {
        const keys = path.split('.');
        let cur = translations.en;
        for (const k of keys) {
          if (!cur || typeof cur !== 'object') return fallback || path;
          cur = cur[k];
        }
        return cur !== undefined ? cur : (fallback || path);
      },
      languages: SUPPORTED_LANGUAGES,
    };
  }
  return context;
}
