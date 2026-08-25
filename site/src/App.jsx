import React, { useState, useEffect } from 'react';
import styles from './App.module.css';
import InteractiveDemo from './components/InteractiveDemo';
import LanguageSelector from './components/LanguageSelector';
import ThemeToggle from './components/ThemeToggle';
import GitHubIconBtn from './components/GitHubIconBtn';
import { useTranslation } from './i18n/I18nContext';

export function App() {
  const { t } = useTranslation();
  const [copiedMac, setCopiedMac] = useState(false);
  const [copiedLinux, setCopiedLinux] = useState(false);
  const [copiedWin, setCopiedWin] = useState(false);
  
  const [is3DEnabled, setIs3DEnabled] = useState(true);
  const [ariaLiveMsg, setAriaLiveMsg] = useState('');

  const handleCopy = async (text, setter) => {
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        setter(true);
        setAriaLiveMsg(t('install.copySuccess', 'Copied to clipboard.'));
        setTimeout(() => setter(false), 2000);
      } else {
        const textArea = document.createElement("textarea");
        textArea.value = text;
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand("Copy");
        textArea.remove();
        setter(true);
        setAriaLiveMsg(t('install.copySuccess', 'Copied to clipboard.'));
        setTimeout(() => setter(false), 2000);
      }
    } catch (err) {
      setAriaLiveMsg('Failed to copy.');
    }
  };

  const handle3DError = () => {
    setIs3DEnabled(false);
    setAriaLiveMsg('3D mode unavailable. Falling back to 2D.');
  };

  const scrollToDemo = (e) => {
    e.preventDefault();
    const demo = document.getElementById('demo');
    if (demo) {
      const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      demo.scrollIntoView({ behavior: prefersReduced ? 'auto' : 'smooth' });
      demo.focus({ preventScroll: true });
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.visuallyHidden} aria-live="polite">{ariaLiveMsg}</div>
      <a href="#main-content" className="skip-link">Skip to main content</a>
      
      <header className={styles.nav}>
        <a className={styles.logo} href="#main-content" aria-label="agy-swap home">agy-swap</a>
        <nav className={styles.navLinks} aria-label="Main Navigation">
          <a href="#main-content">{t('nav.overview', 'Overview')}</a>
          <a href="#demo" onClick={scrollToDemo}>{t('nav.flow', 'Flow')}</a>
          <a href="#features">{t('nav.features', 'Features')}</a>
          <a href="#install">{t('nav.install', 'Install')}</a>
        </nav>
        <div className={styles.navRightGroup}>
          <LanguageSelector />
          <ThemeToggle />
          <GitHubIconBtn />
        </div>
      </header>

      <main id="main-content" className={styles.main}>
        <section className={styles.hero} aria-labelledby="hero-heading">
          <div className={styles.heroTitleBlock}>
            <span className={styles.eyebrow}>{t('hero.eyebrow', 'AGY-SWAP')}</span>
            <h1 id="hero-heading" className={styles.headline}>
              {t('hero.headlinePart1', 'Native Terminal')}<br/>
              {t('hero.headlinePart2', 'Account Switcher')}<br/>
              {t('hero.headlinePart3', 'And Quota Monitor')}<span className={styles.dot}>.</span>
            </h1>
          </div>
          <div className={styles.heroAside}>
            <p className={styles.heroSubtitle}>
              {t('hero.subtitle', 'Instant account rotation and live Gemini quota monitoring for Google Antigravity.')}
            </p>
            <div className={`${styles.copyCode} ${styles.heroCommand}`}>
              <span><b aria-hidden="true">›</b> brew install aklkbqx/agy-swap/agy-swap</span>
              <button
                className={styles.copyBtn}
                onClick={() => handleCopy('brew install aklkbqx/agy-swap/agy-swap', setCopiedMac)}
                aria-label={t('hero.copyCommand', 'Copy Homebrew install command')}
              >
                {copiedMac ? t('hero.copied', 'Copied') : 'Copy'}
              </button>
            </div>
            <a href="#demo" className={styles.demoLink} onClick={scrollToDemo}>
              {t('hero.exploreDemo', 'Explore interactive demo')} ↓
            </a>
          </div>
        </section>

        <section id="demo" tabIndex={-1} className={styles.demoSection} aria-label={t('demo.headline', 'Interactive demo')}>
          <div className={styles.demoWrapper}>
            <InteractiveDemo is3DEnabled={is3DEnabled} on3DError={handle3DError} />
          </div>
        </section>

        <section id="features" className={styles.featuresSection} aria-labelledby="features-heading">
          <div className={styles.featuresIntro}>
            <span className={styles.eyebrow}>{t('features.eyebrow', 'BUILT FOR FLOW')}</span>
            <h2 id="features-heading">{t('features.headline', 'One tool. Zero friction.')}</h2>
          </div>
          <div className={styles.featuresGrid}>
            <div>
              <h3 className={styles.featureTitle}>{t('features.card1Title', 'Sub-Millisecond Switching')}</h3>
              <p className={styles.featureDesc}>{t('features.card1Desc', 'Switch active Google accounts instantaneously without restarting your terminal.')}</p>
            </div>
            <div>
              <h3 className={styles.featureTitle}>{t('features.card2Title', 'Live Gemini Quota Monitor')}</h3>
              <p className={styles.featureDesc}>{t('features.card2Desc', 'Precise real-time countdown of model rate limits and reset cooldowns.')}</p>
            </div>
            <div>
              <h3 className={styles.featureTitle}>{t('features.card3Title', 'Hardware OS Keychain Vault')}</h3>
              <p className={styles.featureDesc}>{t('features.card3Desc', 'Tokens and auth states are encrypted in your operating system secure enclave.')}</p>
            </div>
          </div>
        </section>

        <section id="install" className={styles.installSection} aria-labelledby="install-heading">
          <h2 id="install-heading" className={styles.installHeading}>{t('install.headline', 'Installation')}</h2>
          <div className={styles.installGrid}>
            
            <div className={styles.installBlock}>
              <h3>{t('install.macTab', 'macOS (Homebrew)')}</h3>
              <div className={styles.copyCode}>
                <span>brew install aklkbqx/agy-swap/agy-swap</span>
                <button className={styles.copyBtn} onClick={() => handleCopy('brew install aklkbqx/agy-swap/agy-swap', setCopiedMac)} aria-label="Copy macOS Homebrew install command">
                  {copiedMac ? t('hero.copied', 'Copied') : 'Copy'}
                </button>
              </div>
            </div>

            <div className={styles.installBlock}>
              <h3>{t('install.linuxTab', 'macOS / Linux (Binary)')}</h3>
              <div className={styles.copyCode}>
                <span>curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash</span>
                <button className={styles.copyBtn} onClick={() => handleCopy("curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash", setCopiedLinux)} aria-label="Copy macOS/Linux script install command">
                  {copiedLinux ? t('hero.copied', 'Copied') : 'Copy'}
                </button>
              </div>
            </div>

            <div className={styles.installBlock}>
              <h3>{t('install.winTab', 'Windows (PowerShell)')}</h3>
              <div className={styles.copyCode}>
                <span>irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex</span>
                <button className={styles.copyBtn} onClick={() => handleCopy("irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex", setCopiedWin)} aria-label="Copy Windows PowerShell install command">
                  {copiedWin ? t('hero.copied', 'Copied') : 'Copy'}
                </button>
              </div>
            </div>
            
          </div>
        </section>
      </main>
      
      <footer className={styles.footer} role="contentinfo">
        <p>{t('footer.tagline', 'Google Antigravity CLI Account Switcher & Quota Monitor.')} &middot; {t('footer.openSourceMIT', 'Open-source under MIT License.')}</p>
        <p className={styles.footerLinkWrapper}>
          <a href="https://github.com/aklkbqx/agy-swap" target="_blank" rel="noopener noreferrer" className={styles.footerLink}>
            github.com/aklkbqx/agy-swap
          </a>
        </p>
      </footer>
    </div>
  );
}

export default App;

