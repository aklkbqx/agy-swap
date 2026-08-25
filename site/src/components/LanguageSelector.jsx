import React, { useState, useRef, useEffect } from 'react';
import { useTranslation, SUPPORTED_LANGUAGES } from '../i18n/I18nContext';
import styles from '../App.module.css';

export default function LanguageSelector() {
  const { lang, setLang } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef(null);

  const currentLangObj = SUPPORTED_LANGUAGES.find((l) => l.code === lang) || SUPPORTED_LANGUAGES[0];

  useEffect(() => {
    function handleClickOutside(e) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target)) {
        setIsOpen(false);
      }
    }
    if (isOpen) {
      document.addEventListener('click', handleClickOutside);
    }
    return () => {
      document.removeEventListener('click', handleClickOutside);
    };
  }, [isOpen]);

  const handleContainerKeyDown = (e) => {
    if (e.key === 'Escape') {
      e.stopPropagation();
      setIsOpen(false);
    }
  };

  const handleSelect = (code) => {
    setLang(code);
    setIsOpen(false);
  };

  return (
    <div
      className={styles.langSelectorContainer}
      ref={dropdownRef}
      onKeyDown={handleContainerKeyDown}
    >
      <button
        type="button"
        className={styles.langTriggerBtn}
        onClick={() => setIsOpen((prev) => !prev)}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label={`Language selector. Current language: ${currentLangObj.label}`}
        title={`Change language (${currentLangObj.label})`}
      >
        <svg
          className={styles.langGlobeSvg}
          width="15"
          height="15"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="10" />
          <line x1="2" y1="12" x2="22" y2="12" />
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
        </svg>
        <span className={styles.langCurrentCode}>{currentLangObj.shortLabel}</span>
        <svg
          className={`${styles.langChevron} ${isOpen ? styles.langChevronOpen : ''}`}
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>

      {isOpen && (
        <ul className={styles.langDropdownMenu} role="listbox" aria-label="Available languages">
          {SUPPORTED_LANGUAGES.map((item) => {
            const isSelected = item.code === lang;
            return (
              <li
                key={item.code}
                role="option"
                aria-selected={isSelected}
                className={`${styles.langDropdownItem} ${isSelected ? styles.langItemSelected : ''}`}
                onClick={() => handleSelect(item.code)}
              >
                <span className={styles.langItemBadge}>{item.shortLabel}</span>
                <span className={styles.langItemLabel}>{item.label}</span>
                {isSelected && (
                  <svg
                    className={styles.langItemCheck}
                    width="14"
                    height="14"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="3"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
