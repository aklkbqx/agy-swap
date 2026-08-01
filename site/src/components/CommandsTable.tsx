import { useState } from 'react';
import { Copy, Check } from 'lucide-react';

const commandsList = [
  { cmd: "agy-swap", desc: "Launch interactive TUI session manager", detail: "Opens terminal interface with arrow key navigation" },
  { cmd: "agy-swap next", desc: "Rotate to an account without an observed cooldown", detail: "Falls back to the shortest known wait when all are limited" },
  { cmd: "agy-swap next --family claude", desc: "Rotate for one model family", detail: "Also accepts gemini or gpt without treating other-family cooldowns as account-wide" },
  { cmd: "agy-swap list", desc: "List all saved accounts & active status", detail: "Shows time-based cooldown bars without inventing remaining quota" },
  { cmd: "agy-swap switch <target>", desc: "Switch directly to an account by email or index", detail: "Fast 1-liner credential rotation" },
  { cmd: "agy-swap add", desc: "Log in & save a new Google account", detail: "OAuth browser flow + profile avatar fetching" },
  { cmd: "agy-swap limits --verbose", desc: "Show cooldown evidence", detail: "Includes observation time and sanitized source filename without exposing local paths" },
  { cmd: "agy-swap limit set 1 6d --group claude", desc: "Set a manual cooldown", detail: "Supports Claude, Gemini and GPT with durations up to seven days" },
  { cmd: "agy-swap logout", desc: "Remove the active session", detail: "Clears the OS credential and active OAuth files" },
  { cmd: "agy-swap --version", desc: "Display current agy-swap version", detail: "Prints current semantic version string" }
];

export const CommandsTable = () => {
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null);
  const [failedIndex, setFailedIndex] = useState<number | null>(null);

  const handleCopy = async (cmdText: string, index: number) => {
    try {
      await navigator.clipboard.writeText(cmdText);
      setCopiedIndex(index);
      setFailedIndex(null);
    } catch {
      setFailedIndex(index);
    }
    window.setTimeout(() => {
      setCopiedIndex(null);
      setFailedIndex(null);
    }, 2000);
  };

  return (
    <section id="commands" className="scroll-mt-24 py-20 relative z-10">
      <div className="max-w-5xl mx-auto px-6">
        <div className="text-center mb-12">
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight mb-3">
            CLI Command Reference
          </h2>
          <p className="text-zinc-400 max-w-xl mx-auto text-base">
            Scriptable subcommands designed for automated workflows, terminal aliases, and CI pipelines.
          </p>
        </div>

        <div className="overflow-hidden rounded-lg border border-zinc-800 bg-zinc-900/60 shadow-xl">
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
                  <div className="text-xs text-zinc-400">{item.detail}</div>
                </div>

                <button 
                  type="button"
                  onClick={() => handleCopy(item.cmd, idx)}
                  aria-label={`Copy command: ${item.cmd}`}
                  className="self-start sm:self-center flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-900 border border-zinc-800 hover:border-zinc-700 text-zinc-400 hover:text-white text-xs font-mono transition-all"
                >
                  {copiedIndex === idx ? (
                    <>
                      <Check aria-hidden="true" className="w-3.5 h-3.5 text-emerald-400" />
                      <span className="text-emerald-400">Copied</span>
                    </>
                  ) : (
                    <>
                      <Copy aria-hidden="true" className="w-3.5 h-3.5" />
                      <span>{failedIndex === idx ? 'Copy failed' : 'Copy'}</span>
                    </>
                  )}
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
};
