import React, { useState, useEffect } from 'react';
import styles from './App.module.css';
import InteractiveDemo from './components/InteractiveDemo';

export function App() {
  const [copiedMac, setCopiedMac] = useState(false);
  const [copiedLinux, setCopiedLinux] = useState(false);
  const [copiedWin, setCopiedWin] = useState(false);
  
  const [is3DEnabled, setIs3DEnabled] = useState(false);
  const [canEnable3D, setCanEnable3D] = useState(true);
  const [ariaLiveMsg, setAriaLiveMsg] = useState('');

  useEffect(() => {
    const checkCapabilities = () => {
      if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        return false;
      }
      if (navigator.connection && navigator.connection.saveData) {
        return false;
      }
      try {
        const canvas = document.createElement('canvas');
        const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
        if (!gl) return false;
      } catch (e) {
        return false;
      }
      return true;
    };
    
    setCanEnable3D(checkCapabilities());
  }, []);

  const handleCopy = async (text, setter) => {
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        setter(true);
        setAriaLiveMsg('Copied to clipboard.');
        setTimeout(() => setter(false), 2000);
      } else {
        const textArea = document.createElement("textarea");
        textArea.value = text;
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand("Copy");
        textArea.remove();
        setter(true);
        setAriaLiveMsg('Copied to clipboard using fallback.');
        setTimeout(() => setter(false), 2000);
      }
    } catch (err) {
      setAriaLiveMsg('Failed to copy.');
    }
  };

  const enable3D = () => {
    setIs3DEnabled(true);
    setAriaLiveMsg('Live 3D enabled.');
  };

  const handle3DError = () => {
    setIs3DEnabled(false);
    setCanEnable3D(false);
    setAriaLiveMsg('3D mode failed. Falling back to static mode.');
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
          <a href="#main-content">Overview</a>
          <a href="#demo" onClick={scrollToDemo}>Flow</a>
          <a href="#install">Install</a>
          <a href="https://github.com/aklkbqx/agy-swap" target="_blank" rel="noopener noreferrer">GitHub</a>
        </nav>
        <button
          className={styles.navInstall}
          onClick={() => handleCopy('brew install aklkbqx/agy-swap/agy-swap', setCopiedMac)}
        >
          {copiedMac ? 'Copied' : 'Install agy-swap'}
        </button>
      </header>

      <main id="main-content" className={styles.main}>
        <section className={styles.hero} aria-labelledby="hero-heading">
          <div className={styles.heroTitleBlock}>
            <span className={styles.eyebrow}>AGY-SWAP</span>
            <h1 id="hero-heading" className={styles.headline}>
              Native Terminal<br/>
              Account Switcher<br/>
              And Quota Monitor<span className={styles.dot}>.</span>
            </h1>
          </div>
          <div className={styles.heroAside}>
            <p className={styles.heroSubtitle}>
              Swap accounts. Track remaining capacity.<br />Stay in flow.
            </p>
            <div className={`${styles.copyCode} ${styles.heroCommand}`}>
              <span><b aria-hidden="true">›</b> brew install aklkbqx/agy-swap/agy-swap</span>
              <button
                className={styles.copyBtn}
                onClick={() => handleCopy('brew install aklkbqx/agy-swap/agy-swap', setCopiedMac)}
                aria-label="Copy Homebrew install command"
              >
                {copiedMac ? 'Copied' : 'Copy'}
              </button>
            </div>
            <a href="#demo" className={styles.demoLink} onClick={scrollToDemo}>Explore interactive demo ↓</a>
          </div>
        </section>

        <section id="demo" tabIndex={-1} className={styles.demoSection} aria-labelledby="demo-heading">
          <div className={styles.demoHeader}>
            <div>
              <span className={styles.eyebrow}>LIVE PRODUCT FLOW</span>
              <h2 id="demo-heading">Interactive demo <span>· sample data</span></h2>
            </div>
            {canEnable3D && (
              <button
                className={styles.enable3dBtn}
                onClick={() => {
                  if (is3DEnabled) {
                    setIs3DEnabled(false);
                    setAriaLiveMsg('3D mode disabled.');
                  } else {
                    enable3D();
                  }
                }}
              >
                {is3DEnabled ? 'Switch to 2D' : 'Enable live 3D'}
              </button>
            )}
          </div>
          
          <div className={styles.demoWrapper}>
            <InteractiveDemo is3DEnabled={is3DEnabled} on3DError={handle3DError} />
          </div>
        </section>

        <section id="features" className={styles.featuresSection} aria-labelledby="features-heading">
          <div className={styles.featuresIntro}>
            <span className={styles.eyebrow}>BUILT FOR FLOW</span>
            <h2 id="features-heading">One tool.<br />Zero friction.</h2>
          </div>
          <div className={styles.featuresGrid}>
            <div>
              <h3 className={styles.featureTitle}>Instant Switching</h3>
              <p className={styles.featureDesc}>Change accounts without leaving your terminal. No context switching, no browser required.</p>
            </div>
            <div>
              <h3 className={styles.featureTitle}>Remaining-Capacity Visibility</h3>
              <p className={styles.featureDesc}>See remaining capacity and reset windows for every saved account directly in your workflow.</p>
            </div>
            <div>
              <h3 className={styles.featureTitle}>Local Safety & Backups</h3>
              <p className={styles.featureDesc}>Credentials stay in your local vault. Built-in doctor checks and easy backup/restore.</p>
            </div>
          </div>
        </section>

        <section id="install" className={styles.installSection} aria-labelledby="install-heading">
          <h2 id="install-heading" className={styles.installHeading}>Installation</h2>
          <div className={styles.installGrid}>
            
            <div className={styles.installBlock}>
              <h3>macOS (Homebrew)</h3>
              <div className={styles.copyCode}>
                <span>brew install aklkbqx/agy-swap/agy-swap</span>
                <button className={styles.copyBtn} onClick={() => handleCopy('brew install aklkbqx/agy-swap/agy-swap', setCopiedMac)} aria-label="Copy macOS Homebrew install command">
                  {copiedMac ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>

            <div className={styles.installBlock}>
              <h3>macOS / Linux (Binary)</h3>
              <div className={styles.copyCode}>
                <span>curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash</span>
                <button className={styles.copyBtn} onClick={() => handleCopy("curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash", setCopiedLinux)} aria-label="Copy macOS/Linux script install command">
                  {copiedLinux ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>

            <div className={styles.installBlock}>
              <h3>Windows (PowerShell)</h3>
              <div className={styles.copyCode}>
                <span>irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex</span>
                <button className={styles.copyBtn} onClick={() => handleCopy("irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex", setCopiedWin)} aria-label="Copy Windows PowerShell install command">
                  {copiedWin ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>
            
          </div>
        </section>
      </main>
      
      <footer className={styles.footer} role="contentinfo">
        <p>Interactive demo &middot; sample data</p>
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
