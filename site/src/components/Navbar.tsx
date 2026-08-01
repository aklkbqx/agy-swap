import { useState, useEffect } from 'react';
import { ArrowUpRight } from 'lucide-react';
import { APP_VERSION } from '../version';

export const Navbar = () => {
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const handleScroll = () => setScrolled(window.scrollY > 20);
    window.addEventListener('scroll', handleScroll, { passive: true });
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  return (
    <header className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${
      scrolled ? 'bg-zinc-950 border-b border-zinc-800 py-3.5' : 'bg-zinc-950/95 py-5'
    }`}>
      <div className="max-w-6xl mx-auto px-6 flex items-center justify-between">
        <a href="#main-content" className="group flex items-center gap-3" aria-label="agy-swap home">
          <div className="flex h-9 w-9 items-center justify-center rounded border border-orange-400 bg-zinc-900 text-lg font-extrabold text-orange-400 transition-transform group-hover:scale-105">
            A
          </div>
          <div className="flex items-center gap-2.5">
            <span className="font-bold text-lg tracking-tight text-white font-sans">AGY SWAP</span>
            <span className="text-xs px-2 py-0.5 rounded-full font-mono bg-orange-500/10 text-orange-400 border border-orange-500/20 font-medium">
              v{APP_VERSION}
            </span>
          </div>
        </a>

        <nav className="hidden md:flex items-center gap-8">
          <a href="#features" className="text-sm text-zinc-400 hover:text-white transition-colors font-medium">Features</a>
          <a href="#simulator" className="text-sm text-zinc-400 hover:text-white transition-colors font-medium">Live Simulator</a>
          <a href="#calculator" className="text-sm text-zinc-400 hover:text-white transition-colors font-medium">Quota Logic</a>
          <a href="#commands" className="text-sm text-zinc-400 hover:text-white transition-colors font-medium">Commands</a>
        </nav>

        <div className="flex items-center gap-3">
          <a
            href="https://github.com/aklkbqx/agy-swap"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 text-xs font-semibold px-3.5 py-2 rounded-lg bg-zinc-900/90 border border-zinc-800 text-zinc-200 hover:bg-zinc-800 hover:border-zinc-700 hover:text-white transition-all shadow-sm"
          >
            <svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
            </svg>
            <span>GitHub</span>
            <ArrowUpRight aria-hidden="true" className="w-3.5 h-3.5 text-zinc-400" />
          </a>
        </div>
      </div>
    </header>
  );
};
