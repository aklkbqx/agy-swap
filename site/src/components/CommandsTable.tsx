import { useState } from 'react';
import { motion } from 'framer-motion';
import { Copy, Check } from 'lucide-react';

const commandsList = [
  { cmd: "agy-swap", desc: "Launch interactive TUI session manager", detail: "Opens terminal interface with arrow key navigation" },
  { cmd: "agy-swap next", desc: "Auto-rotate to next ready account", detail: "Skips rate-limited accounts automatically (1-Click)" },
  { cmd: "agy-swap list", desc: "List all saved accounts & active status", detail: "Shows avatar badges and dual-model quota limits" },
  { cmd: "agy-swap switch <target>", desc: "Switch directly to an account by email or index", detail: "Fast 1-liner credential rotation" },
  { cmd: "agy-swap add", desc: "Log in & save a new Google account", detail: "OAuth browser flow + profile avatar fetching" },
  { cmd: "agy-swap update", desc: "Update agy-swap script to latest release", detail: "Checks remote GitHub version and self-upgrades" },
  { cmd: "agy-swap --version", desc: "Display current agy-swap version", detail: "Prints current semantic version string" }
];

export const CommandsTable = () => {
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null);

  const handleCopy = (cmdText: string, index: number) => {
    navigator.clipboard.writeText(cmdText);
    setCopiedIndex(index);
    setTimeout(() => setCopiedIndex(null), 2000);
  };

  return (
    <section id="commands" className="py-20 relative z-10">
      <div className="max-w-5xl mx-auto px-6">
        <div className="text-center mb-12">
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight mb-3">
            CLI Command Reference
          </h2>
          <p className="text-zinc-400 max-w-xl mx-auto text-base">
            Scriptable subcommands designed for automated workflows, terminal aliases, and CI pipelines.
          </p>
        </div>

        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5 }}
          className="glass-panel rounded-2xl border border-white/10 overflow-hidden shadow-2xl"
        >
          <div className="divide-y divide-zinc-800/80">
            {commandsList.map((item, idx) => (
              <div 
                key={item.cmd} 
                className="p-4 sm:px-6 sm:py-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4 hover:bg-zinc-900/40 transition-colors"
              >
                <div className="space-y-1">
                  <div className="flex items-center gap-3">
                    <span className="font-mono text-sm font-semibold text-orange-400 bg-orange-500/10 px-2.5 py-1 rounded-md border border-orange-500/20">
                      {item.cmd}
                    </span>
                  </div>
                  <div className="text-sm font-medium text-white">{item.desc}</div>
                  <div className="text-xs text-zinc-500">{item.detail}</div>
                </div>

                <button 
                  onClick={() => handleCopy(item.cmd, idx)}
                  className="self-start sm:self-center flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-900 border border-zinc-800 hover:border-zinc-700 text-zinc-400 hover:text-white text-xs font-mono transition-all"
                >
                  {copiedIndex === idx ? (
                    <>
                      <Check className="w-3.5 h-3.5 text-emerald-400" />
                      <span className="text-emerald-400">Copied</span>
                    </>
                  ) : (
                    <>
                      <Copy className="w-3.5 h-3.5" />
                      <span>Copy</span>
                    </>
                  )}
                </button>
              </div>
            ))}
          </div>
        </motion.div>
      </div>
    </section>
  );
};
