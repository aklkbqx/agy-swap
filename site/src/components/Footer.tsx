import { ArrowUpRight } from 'lucide-react';

export const Footer = () => {
  return (
    <footer className="border-t border-zinc-800/80 py-12 relative z-10 bg-[#070709]">
      <div className="max-w-6xl mx-auto px-6 flex flex-col sm:flex-row items-center justify-between gap-6">
        <div className="flex items-center gap-3">
          <div className="w-7 h-7 rounded-lg bg-orange-500 flex items-center justify-center text-black font-extrabold text-xs">
            A
          </div>
          <span className="font-bold text-white text-sm font-sans">AGY SWAP</span>
          <span className="text-zinc-600">|</span>
          <span className="text-xs text-zinc-500">Open-source MIT License</span>
        </div>

        <div className="flex items-center gap-6 text-xs text-zinc-400">
          <a 
            href="https://github.com/aklkbqx/agy-swap" 
            target="_blank" 
            rel="noopener noreferrer"
            className="hover:text-white flex items-center gap-1 transition-colors"
          >
            <span>GitHub Repository</span>
            <ArrowUpRight className="w-3 h-3 text-zinc-600" />
          </a>
          <a 
            href="https://github.com/aklkbqx" 
            target="_blank" 
            rel="noopener noreferrer"
            className="hover:text-white flex items-center gap-1 transition-colors"
          >
            <span>Created by @aklkbqx</span>
            <ArrowUpRight className="w-3 h-3 text-zinc-600" />
          </a>
        </div>
      </div>
    </footer>
  );
};
