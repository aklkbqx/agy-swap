import { useState } from 'react';
import { motion, useScroll, useTransform } from 'framer-motion';
import { Copy, Check, Terminal, Zap, Shield, ChevronRight, Activity } from 'lucide-react';

export const Hero = () => {
  const [copied, setCopied] = useState(false);
  const { scrollY } = useScroll();
  
  const yBackground = useTransform(scrollY, [0, 500], [0, 150]);
  const yHeroText = useTransform(scrollY, [0, 500], [0, -50]);
  const opacityHero = useTransform(scrollY, [0, 300], [1, 0.4]);

  const command = "curl -fsSL https://raw.githubusercontent.com/aklkbqx/agy-swap/main/agy-swap -o ~/.local/libexec/agy-swap && chmod +x ~/.local/libexec/agy-swap";

  const handleCopy = () => {
    navigator.clipboard.writeText(command);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <section className="relative pt-36 pb-20 overflow-hidden min-h-[90vh] flex flex-col justify-center">
      <div className="absolute inset-0 bg-[linear-gradient(to_right,#1f1f2e0f_1px,transparent_1px),linear-gradient(to_bottom,#1f1f2e0f_1px,transparent_1px)] bg-[size:4rem_4rem] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_0%,#000_70%,transparent_100%)] pointer-events-none" />

      <motion.div 
        style={{ y: yBackground }}
        className="absolute top-1/4 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[350px] bg-gradient-to-tr from-amber-600/15 to-orange-500/20 blur-[130px] rounded-full pointer-events-none"
      />

      <div className="max-w-5xl mx-auto px-6 relative z-10 text-center">
        <motion.div style={{ y: yHeroText, opacity: opacityHero }}>
          <motion.div 
            initial={{ opacity: 0, y: 15 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.5 }}
            className="inline-flex items-center gap-2.5 px-3.5 py-1.5 rounded-full bg-zinc-900/90 border border-zinc-800 text-xs text-zinc-300 mb-8 backdrop-blur-md shadow-inner"
          >
            <span className="flex h-2 w-2 rounded-full bg-orange-500 animate-pulse" />
            <span className="font-semibold text-zinc-200">v1.5.0 Released</span>
            <span className="text-zinc-600">|</span>
            <span className="text-zinc-400 font-mono">Smart Auto-Rotation Engine</span>
            <ChevronRight className="w-3.5 h-3.5 text-zinc-500" />
          </motion.div>

          <motion.h1 
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.1 }}
            className="text-4xl sm:text-6xl font-extrabold tracking-tight text-white mb-6 leading-[1.12]"
          >
            Multi-Account Switcher & Quota Intelligence for{' '}
            <span className="bg-gradient-to-r from-orange-400 via-amber-300 to-orange-500 bg-clip-text text-transparent">
              Google Antigravity CLI
            </span>
          </motion.h1>

          <motion.p 
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.2 }}
            className="text-lg sm:text-xl text-zinc-400 max-w-2xl mx-auto mb-10 leading-relaxed font-normal"
          >
            Bypass rate limits effortlessly. Monitor Claude vs Gemini limits in real time and auto-rotate Google credentials with zero distortion in a minimal terminal UI.
          </motion.p>

          <motion.div 
            initial={{ opacity: 0, scale: 0.96 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ duration: 0.5, delay: 0.3 }}
            className="max-w-2xl mx-auto mb-14"
          >
            <div 
              onClick={handleCopy}
              className="group relative flex items-center justify-between gap-4 p-3.5 sm:px-5 sm:py-4 rounded-xl bg-zinc-950/80 border border-zinc-800/90 hover:border-orange-500/40 transition-all duration-300 cursor-pointer shadow-2xl backdrop-blur-xl"
            >
              <div className="flex items-center gap-3 font-mono text-xs sm:text-sm text-zinc-300 overflow-x-auto whitespace-nowrap scrollbar-none">
                <span className="text-orange-500 font-bold select-none">$</span>
                <span className="select-all">{command}</span>
              </div>

              <button 
                className={`shrink-0 flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                  copied 
                    ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/40' 
                    : 'bg-zinc-900 border border-zinc-800 text-zinc-300 group-hover:border-zinc-700 group-hover:text-white'
                }`}
              >
                {copied ? (
                  <>
                    <Check className="w-3.5 h-3.5" />
                    <span>Copied</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3.5 h-3.5" />
                    <span>Copy</span>
                  </>
                )}
              </button>
            </div>
          </motion.div>
        </motion.div>

        <motion.div 
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.7, delay: 0.4 }}
          className="grid grid-cols-2 md:grid-cols-4 gap-4 max-w-4xl mx-auto"
        >
          <div className="glass-panel p-4 rounded-xl text-left border border-white/5">
            <div className="flex items-center gap-2 text-xs font-mono text-zinc-400 mb-1">
              <Zap className="w-3.5 h-3.5 text-orange-400" />
              <span>Switch Time</span>
            </div>
            <div className="text-xl font-bold text-white font-mono">&lt; 0.4s</div>
          </div>

          <div className="glass-panel p-4 rounded-xl text-left border border-white/5">
            <div className="flex items-center gap-2 text-xs font-mono text-zinc-400 mb-1">
              <Activity className="w-3.5 h-3.5 text-amber-400" />
              <span>Quota Precision</span>
            </div>
            <div className="text-xl font-bold text-white font-mono">100% Real-Time</div>
          </div>

          <div className="glass-panel p-4 rounded-xl text-left border border-white/5">
            <div className="flex items-center gap-2 text-xs font-mono text-zinc-400 mb-1">
              <Shield className="w-3.5 h-3.5 text-emerald-400" />
              <span>Security</span>
            </div>
            <div className="text-xl font-bold text-white font-mono">Native Keychain</div>
          </div>

          <div className="glass-panel p-4 rounded-xl text-left border border-white/5">
            <div className="flex items-center gap-2 text-xs font-mono text-zinc-400 mb-1">
              <Terminal className="w-3.5 h-3.5 text-cyan-400" />
              <span>Dependencies</span>
            </div>
            <div className="text-xl font-bold text-white font-mono">0 External</div>
          </div>
        </motion.div>
      </div>
    </section>
  );
};
